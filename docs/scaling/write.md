# CQRS & Write Scaling at Open Guard

> **Target Audience:** Senior Backend Engineers & System Architects
> **Version:** 1.0 | **Classification:** Engineering Reference

## Table of Contents

1. [CQRS Architecture Overview](#1-cqrs-architecture-overview)
2. [Transactional Outbox: The Core Write Pattern](#2-transactional-outbox-the-core-write-pattern)
3. [MongoDB CQRS Split (Audit Service)](#3-mongodb-cqrs-split-audit-service)
4. [Service-Level Write Models](#4-service-level-write-models)
5. [Command vs. Query Repositories](#5-command-vs-query-repositories)
6. [Write Consistency Guarantees](#6-write-consistency-guarantees)
7. [Batching & Flush Strategies](#7-batching--flush-strategies)
8. [Dead Letter Queues & Failure Modes](#8-dead-letter-queues--failure-modes)
9. [Scaling Write Capacity](#9-scaling-write-capacity)
10. [CQRS Decision Matrix](#10-cqrs-decision-matrix)
11. [Operational Considerations](#11-operational-considerations)

---

## 1. CQRS Architecture Overview

Open Guard implements a **hybrid CQRS** model. Rather than enforcing strict command/query separation at every layer, we apply it where it matters most: separating **write-optimized** storage from **read-optimized** storage for the high-volume audit pipeline, and using **event-driven state transfer** (Transactional Outbox → Kafka) to atomically propagate writes to downstream read models.

### High-Level Write Topology

```
                         WRITE PATH                           READ PATH
                    ┌──────────────────┐              ┌──────────────────┐
                    │  PostgreSQL       │              │  Redis (L1+L2)   │
                    │  (Source of Truth)│◄────────────►│  (Cache)         │
                    │  IAM, Policy, DLP │  read/write  │                  │
                    └─────┬────────────┘              └──────────────────┘
                          │
                    [Transactional Outbox]
                          │
                    ┌─────▼────────────┐
                    │  Kafka           │
                    │  (Event Bus)     │
                    └──┬───────────┬───┘
                       │           │
              ┌────────▼──┐  ┌─────▼──────────┐
              │ MongoDB    │  │ ClickHouse     │
              │ (Audit)    │  │ (Compliance)   │
              │ Write-Opt. │  │ Analytics-Opt. │
              └────────────┘  └────────────────┘

Write Repos (command side)         Read Repos (query side)
───────────────                     ──────────────
PostgreSQL (IAM, Policy, DLP)       Redis cache (hot reads)
MongoDB Primary (Audit writes)      MongoDB Secondary (Audit reads)
Kafka (event publication)           ClickHouse (analytics queries)
                                    PostgreSQL (direct reads: policy eval,
                                                 user listing)
```

### The CQRS Spectrum

| Level | Pattern | Where Used |
|-------|---------|------------|
| **Database** | Separate primary/secondary MongoDB connections | `services/audit/main.go:71-105` |
| **Event** | Transactional Outbox → Kafka → consumer writes to read store | IAM, Policy → Kafka → Audit, Compliance, Threat |
| **Service** | Command handlers vs. query handlers in same binary | Policy Service: `CreatePolicy` vs. `Evaluate` |
| **Storage** | Write-optimized (MongoDB) vs. analytics-optimized (ClickHouse) | Audit trail vs. Compliance reporting |

---

## 2. Transactional Outbox: The Core Write Pattern

### Problem

When a service must write to PostgreSQL AND publish a Kafka event (e.g., "policy created" + "notify detectors"), a naive dual-write risks:
- Kafka published but DB write fails → phantom event
- DB write succeeds but Kafka publish fails → lost event
- Partial failure during service crash

### Solution: Outbox Within the Same DB Transaction

**Location:** `shared/kafka/outbox/writer.go`

```go
func (w *Writer) WriteTx(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error {
    query := `INSERT INTO %s (org_id, topic, key, payload) VALUES ($1, $2, $3, $4)`
    _, err := tx.Exec(ctx, query, orgID, topic, key, payload)
    return err
}
```

The write happens inside the **same PostgreSQL transaction** as the business logic:

**IAM `RegisterUser`** (`services/iam/pkg/service/users.go:39-68`):
```go
tx, err := s.userRepo.BeginTx(ctx)
// ... create user in DB ...
s.outboxRepo.CreateOutboxEvent(ctx, tx, req.OrgID, "saga.orchestration", userID, payload)
tx.Commit(ctx)
// Both succeed or neither — atomic.
```

**Policy `CreatePolicy`** (`services/policy/pkg/service/service.go:431`):
```go
tx, err := s.repo.Pool().Begin(ctx)
p, err := s.repo.CreatePolicyTx(ctx, tx, orgID, ...)
s.PublishPolicyChange(ctx, tx, orgID, p.ID, "policy.created")
tx.Commit(ctx)
```

### Outbox Relay: Transactional Publisher

**Location:** `shared/kafka/outbox/relay.go`

The relay reads from the outbox table and publishes to Kafka. It combines:

- **Push:** `LISTEN "outbox_new"` via `pg_notify` trigger — immediate wakeup on INSERT
- **Pull:** 5s polling ticker — fallback for missed notifications and retries

```go
func (r *Relay) drain(ctx context.Context) {
    query := `SELECT id, topic, key, payload, attempts
              FROM outbox_records
              WHERE (status = 'pending' OR (status = 'failed' AND attempts < 5))
                AND dead_at IS NULL
              ORDER BY created_at ASC
              FOR UPDATE SKIP LOCKED LIMIT 100`
    // Publish each record to Kafka
    // On success: UPDATE SET status = 'published'
    // On failure: increment attempts; if >= 5, mark 'dead'
}
```

`FOR UPDATE SKIP LOCKED` allows **horizontal scaling** of the relay — multiple instances can drain concurrently without double-processing.

### Schema

**IAM** (`services/iam/migrations/006_create_outbox.up.sql`) and **Policy** (`services/policy/migrations/004_create_outbox.up.sql`):

```sql
CREATE TABLE outbox_records (
    id          UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id      UUID NOT NULL,
    topic       TEXT NOT NULL,
    key         TEXT NOT NULL,
    payload     JSONB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | published | failed | dead
    attempts    INT NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at     TIMESTAMPTZ
);

-- pg_notify trigger for instant relay wakeup
CREATE FUNCTION notify_outbox() RETURNS TRIGGER AS $$
BEGIN PERFORM pg_notify('outbox_new', NEW.id::TEXT); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
    AFTER INSERT ON outbox_records
    FOR EACH ROW EXECUTE FUNCTION notify_outbox();
```

---

## 3. MongoDB CQRS Split (Audit Service)

The Audit service is the most CQRS-native component — it separates the write connection from the read connection at the driver level.

**Location:** `services/audit/main.go:71-105`

```go
// ── MongoDB CQRS Split ───────────────────────────────────────────────────

// Primary (Writes): majority write concern
wc := writeconcern.Majority()
writeOpts := options.Client().ApplyURI(primaryURI).SetWriteConcern(wc)
writeClient, _ := mongo.Connect(ctx, writeOpts)

// Secondary (Reads): route to secondaries for read scaling
rp := readpref.SecondaryPreferred()
readOpts := options.Client().ApplyURI(secondaryURI).SetReadPreference(rp)
readClient, _ := mongo.Connect(ctx, readOpts)

writeRepo := repository.NewAuditWriteRepository(writeClient, "openguard_audit")
readRepo := repository.NewAuditReadRepository(readClient, "openguard_audit")
```

### Write Repository

The write repository processes Kafka consumer batches and writes to MongoDB:

- Kafka consumer accumulates up to **500 events** or **1 second** window
- Writes via `BulkWrite` with `ordered: false` for parallel insert
- Idempotent via `event_id` unique index — exactly-once semantics
- Kafka offset committed **only after** MongoDB write succeeds

### Read Repository

The read repository serves API queries:

- `SecondaryPreferred()` — reads from replica set secondaries when available
- No writes ever touch this repository
- This separation prevents heavy analytical queries from competing with write capacity on the primary

---

## 4. Service-Level Write Models

### 4a. IAM Service (Command Side)

**Location:** `services/iam/pkg/service/`

| Command | Write Target | Outbox Event | Outbox Topic |
|---------|-------------|--------------|--------------|
| `RegisterUser` | PostgreSQL `users` table | `user.created` | `saga.orchestration` |
| `ReprovisionUser` | PostgreSQL `users` table | `user.reprovisioned` | `saga.orchestration` |
| `DeleteUser` | PostgreSQL `users` table + Redis blocklist | `user.deleted` | `saga.orchestration` |
| `PatchUser` | PostgreSQL `users` table | `user.updated` | `saga.orchestration` |
| `OffboardOrg` | PostgreSQL (all org resources) | `org.offboarded` | `saga.orchestration` |

**Read side** (`GetUser`, `ListUsers`, `GetCurrentUser`): queries PostgreSQL directly, no Kafka.

### 4b. Policy Service (Command Side)

**Location:** `services/policy/pkg/service/service.go`

| Command | Write Target | Outbox Event | Outbox Topic |
|---------|-------------|--------------|--------------|
| `CreatePolicy` | PostgreSQL `policies` table | `policy.created` | `policy.changes` |
| `UpdatePolicy` | PostgreSQL `policies` table | `policy.updated` | `policy.changes` |
| `DeletePolicy` | PostgreSQL `policies` table | `policy.deleted` | `policy.changes` |

**Read side** (`Evaluate`): Tiered cache (Redis → PostgreSQL with singleflight), no Kafka.

### 4c. Audit Ingestion (Direct Kafka Write)

**Location:** `services/audit/pkg/handlers/ingest.go`

Unlike IAM/Policy, the Audit ingest path skips the outbox and publishes **directly to Kafka**:

```go
func (h *IngestHandler) Ingest(w http.ResponseWriter, r *http.Request) {
    // ... parse event ...
    h.publisher.Publish(r.Context(), topic, eventID, payload)
    w.WriteHeader(http.StatusAccepted)
}
```

**Rationale:** Audit events are not business state — they are fire-and-forget observations. The trade-off is lower latency at the cost of no atomicity guarantee. If Kafka is down, the request is rejected (fail-closed).

### 4d. Threat Detectors (Dual Write)

**Location:** `services/threat/pkg/detector/brute_force.go`

When a detector fires, it **dual-writes**:

```go
// 1. Direct MongoDB write (immediate queryability)
d.store.CreateAlert(ctx, alert)

// 2. Kafka publish (downstream consumers: Alerting, Audit)
d.pub.Publish(ctx, "threat.alerts", alertID, payload)
```

### 4e. Compliance Service (Kafka → ClickHouse)

**Location:** `services/compliance/pkg/consumer/clickhouse_writer.go`

Batch-oriented write model:

- Accumulates **5000 events** or **2 second** window
- Writes to ClickHouse — columnar, analytics-optimized
- Commits Kafka offset **only after** ClickHouse write succeeds (at-least-once)
- Idempotent consumers provide exactly-once semantics

### 4f. DLP Service (Kafka → PostgreSQL)

**Location:** `services/dlp/pkg/consumer/consumer.go`

- Consumes from configured Kafka topic
- Scans content for PII patterns
- Writes findings directly to PostgreSQL
- DLQ after 5 consecutive failures

### 4g. Alerting Saga (Kafka → MongoDB + Kafka)

**Location:** `services/alerting/pkg/saga/saga.go`

Multi-step write saga from `threat.alerts`:

1. Persist alert to MongoDB
2. Publish to `notifications.outbound` (Kafka)
3. Fire SIEM webhook (HTTP)
4. Publish to `audit.trail` (Kafka)

---

## 5. Command vs. Query Repositories

### 5a. MongoDB (Audit Service)

| Aspect | Write Repo | Read Repo |
|--------|-----------|-----------|
| Connection | Primary MongoDB node | Secondary/replica nodes |
| Write Concern | `Majority` | N/A (read-only) |
| Read Preference | Primary | `SecondaryPreferred` |
| Access Pattern | Batch writes from Kafka consumer | Paginated API queries |
| Index Strategy | Minimal (event_id unique) | Query-pattern indexes (org_id, actor, time) |

### 5b. PostgreSQL (IAM, Policy, DLP)

No explicit repository split — the same PostgreSQL connection handles both reads and writes. Separation is at the **service method** level:

| Service | Write Methods | Read Methods | Cache Layer |
|---------|-------------|-------------|-------------|
| IAM | `RegisterUser`, `DeleteUser`, etc. | `GetUser`, `ListUsers` | Redis for blocklist |
| Policy | `CreatePolicy`, `UpdatePolicy` | `Evaluate`, `GetPolicy` | Redis (L1+L2) |
| DLP | `SaveFinding` | `GetFindings` | None |

### 5c. ClickHouse (Compliance Service)

ClickHouse is a **write-optimized** store by design. There is no read-repository separation because:

- All writes are batch inserts from Kafka
- All reads are analytical queries (aggregations, grouped by tenant/time)
- ClickHouse's MergeTree engine handles both efficiently

---

## 6. Write Consistency Guarantees

### 6a. Outbox-Backed Writes (IAM, Policy)

```
── Write ──► Begin TX ──► DB Write ──► Outbox Write ──► Commit ──► pg_notify ──► Relay ──► Kafka

Isolation: serializable at DB level (via RLS + transaction)
Durability: PostgreSQL WAL (sync commit) + Kafka acks=all
Atomicity: all-or-nothing within the DB transaction
```

### 6b. Direct Kafka Publish (Audit Ingest)

```
── Write ──► Kafka Publish (sync, RequireAll acks)

Consistency: eventual (consumers may lag)
Durability: Kafka replication factor 3, min ISR 2
Failure mode: HTTP 500 if Kafka unavailable (fail-closed)
```

### 6c. Kafka Consumer → Data Store (Audit, Compliance, DLP)

```
Kafka ──► Batch ──► Write to Store ──► Commit Offset

Philosophy: "store first, commit second"
- If store write fails: offset NOT committed → message redelivered on restart
- If commit fails after successful write: message reprocessed (idempotent key prevents duplicates)
- Exactly-once via event_id/dedup key unique index in target store
```

### 6d. Consistency Matrix

| Write Path | Consistency Level | RPO | RTO | Failure Atomicity |
|-----------|------------------|-----|-----|-------------------|
| Outbox (PG → Kafka) | Strong within PG, eventual at Kafka | < 5s | < 60s | Atomic (same TX) |
| Direct Kafka (Audit Ingest) | Eventual | < 1s | < 1s | None (fire-and-forget with ack) |
| Kafka → MongoDB (Audit consumer) | Eventual | < 100ms | < 5s | At-least-once + idempotent |
| Kafka → ClickHouse (Compliance) | Eventual | < 2s | < 10s | At-least-once + batch |
| Direct PG (IAM, Policy reads) | Strong (read-your-writes) | — | — | — |

---

## 7. Batching & Flush Strategies

Each Kafka consumer tunes batch size and flush interval independently:

| Service | Max Batch | Flush Interval | Target Throughput |
|---------|-----------|----------------|-------------------|
| Audit (MongoDB) | 500 events | 1000ms | 50K events/s per consumer |
| Compliance (ClickHouse) | 5000 events | 2000ms | 250K events/s per consumer |
| DLP (PostgreSQL) | 100 events | 1000ms | 10K events/s per consumer |
| Webhook Delivery | 50 concurrent | N/A (worker pool) | 50 deliveries/s |

**Key insight:** Batching is the single most important lever for write throughput. A single ClickHouse consumer writing 5000-event batches can sustain higher throughput than 10 MongoDB consumers writing 500-event batches.

---

## 8. Dead Letter Queues & Failure Modes

### 8a. Outbox DLQ (PostgreSQL level)

After **5 failed publish attempts**, an outbox record moves to `status = 'dead'` with a `dead_at` timestamp. DLQ topics:
- `outbox.dlq` (Kafka, 3 partitions) — for manual inspection and replay
- Dead records remain in PostgreSQL for forensic analysis

### 8b. Kafka Consumer DLQ (application level)

DLP consumer — after **5 consecutive processing failures**, the message is published to a dedicated DLQ topic.

### 8c. Webhook DLQ

After **5 delivery attempts** with exponential backoff (1s, 2s, 4s, 8s, 16s), the webhook delivery is moved to `webhook.dlq`.

### 8d. Failure Flow

```
                    ┌──────────────┐
                    │ Write Fails  │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
          Retryable?   Transient?   Permanent?
              │            │            │
              ▼            ▼            ▼
          Increment    Retry with    Move to DLQ
          attempts     backoff       (dead status /
              │            │         DLQ topic)
              ▼            │
          Max retries ─────┘
          reached?
              │
              ▼
          Dead record /
          DLQ topic
```

---

## 9. Scaling Write Capacity

### 9a. Horizontal Outbox Relay Scaling

The outbox relay uses `FOR UPDATE SKIP LOCKED` — multiple relay instances can safely drain the same outbox table:

```
3 relay pods → each picks up 100 pending records per drain cycle → 300 records/cycle → ~3000 records/s at 10 cycles/s
```

Scale linearly by adding more relay pods (up to the PG connection limit).

### 9b. Kafka Partition Parallelism

- `saga.orchestration`: 12 partitions → up to 12 parallel consumers per group
- `policy.changes`: 6 partitions → up to 6 parallel consumers
- `audit.trail`: 24 partitions → up to 24 parallel consumers
- `threat.alerts`: 12 partitions → up to 12 parallel consumers

Each consumer can run in its own goroutine or process.

### 9c. Independent Detector Goroutines (Threat Service)

All 6+ detectors run as independent goroutines, each with its own Kafka reader:

```
BruteForceDetector       ── goroutine
ImpossibleTravelDetector ── goroutine
OffHoursDetector         ── goroutine
AccountTakeoverDetector  ── goroutine
PrivilegeEscalationDetector ── goroutine
DataExfiltrationDetector ── goroutine
```

One slow detector never blocks others.

### 9d. ClickHouse Write Scaling

MergeTree engine + batch writes enable linear scaling with node count:

- 3-node ClickHouse cluster: sustained **500K+ events/s** at p99 < 10ms write latency
- 2s flush window absorbs traffic bursts without backpressure
- Writes are asynchronous from the consumer perspective

### 9e. MongoDB Write Scaling

- Primary handles all writes (MongoDB's single-writer limitation)
- Secondary reads offload query traffic but do not increase write capacity
- At scale: consider sharded MongoDB cluster (shard key: `org_id` + `timestamp`)
- Immediate path: increase batch size and tune `writeconcern.Majority()` to `journaled` for lower latency

### 9f. Read Scalability (The Other Side of CQRS)

While writes scale via partitioning and batching, reads scale via:

| Layer | Mechanism | Hit Rate Target |
|-------|-----------|----------------|
| L1: In-process cache (per pod) | Policy decisions, config | 70% |
| L2: Redis Cluster | Sessions, blocklist, rate limits | 25% |
| L3: Database | Source of truth | 5% |

Read scalability is documented in detail at `docs/scaling/read.md` and `docs/scaling/redis.md`.

---

## 10. CQRS Decision Matrix

| Operation Type | Command Model | Query Model | Why |
|---------------|--------------|-------------|-----|
| User registration | PostgreSQL (outbox → Kafka) | PostgreSQL (direct) | ACID required for identity, eventual for notifications |
| Policy CRUD | PostgreSQL (outbox → Kafka) | Redis → PostgreSQL | Cache beats DB for evaluation latency |
| Audit ingestion | Kafka (direct) | MongoDB (secondary) | Write-specialized (Kafka) + read-isolated (MongoDB replica) |
| Threat detection | Kafka + MongoDB | MongoDB (secondary) | Dual-write for reliability + queryability |
| Compliance analytics | ClickHouse (batch) | ClickHouse (direct) | Columnar storage optimizes both write and analytical read |
| Webhook delivery | PostgreSQL (status) | PostgreSQL (direct) | Simple CRUD, no read scaling pressure |
| DLP scanning | PostgreSQL (findings) | PostgreSQL (direct) | Low volume, direct reads |
| Policy evaluation | N/A (read-only hot path) | Redis → PostgreSQL | Heavy read optimization, no write side |

### When We Skip CQRS

Not every write path justifies CQRS separation:

| Skip Reason | Examples |
|-------------|----------|
| Low write volume (< 100/s) | Webhook delivery status, DLP findings |
| Same-node read after write | IAM user listing (reads from same PG as writes) |
| Simple CRUD with no downstream consumers | Connector configuration, tenant settings |

---

## 11. Operational Considerations

### Tracing Writes Through the Pipeline

Every Kafka message carries OpenTelemetry context via propagated headers:

```go
// shared/kafka/publisher.go
otel.GetTextMapPropagator().Inject(ctx, kafkaHeaderCarrier{headers: &msg.Headers})
```

The trace follows: `HTTP handler → Outbox write → Kafka publish → Consumer → DB write`. This enables end-to-end observability of write latency and failures.

### Monitoring Write Health

| Metric | What It Tells You | Target |
|--------|-------------------|--------|
| `outbox_records WHERE status = 'failed'` | Relay publishing issues | < 0.01% of total |
| `outbox_records WHERE status = 'dead'` | Permanent failures needing manual DLQ review | 0 |
| `kafka_consumer_lag{group}` | Consumer falling behind producer | < 1000 |
| `kafka_write_latency` | Kafka broker performance | p99 < 10ms |
| `mongodb_bulk_write_duration` | MongoDB insert performance | p99 < 100ms |
| `clickhouse_insert_duration` | ClickHouse insert performance | p99 < 50ms |
| `outbox_relay_drain_duration` | Relay processing time per cycle | p99 < 500ms |

### Recovery Procedures

| Failure | Recovery | RTO |
|---------|----------|-----|
| Outbox relay crash | New goroutine picks up `SKIP LOCKED` within 5s | < 5s |
| Kafka broker failure | Rebalance to remaining brokers (min ISR = 2) | < 30s |
| MongoDB primary failure | Replica set election | < 10s |
| ClickHouse node failure | Distributed table routes to remaining nodes | < 1s |
| DLQ accumulation | Replay via admin API or manual Kafka republish | N/A |

### CQRS Migration Path

If a service outgrows single-node writes:

```
Phase 1: Same-node CQRS (current for IAM, Policy)
         PostgreSQL handles both reads and writes

Phase 2: Read replica offload
         Route analytical/listing queries to PG read replicas

Phase 3: Outbox → specialized read store (already done for Audit)
         PostgreSQL (write) → Kafka → MongoDB/ClickHouse (read)

Phase 4: Event-sourced read model
         Rebuild read model from Kafka event stream (replay)
```

---

## Key Architectural Principles

1. **Write to your source of truth, emit events as a side effect.** The Transactional Outbox ensures atomicity between business state and event publication — no dual-write problem.

2. **Consumers own their read models.** Each consumer (Audit, Compliance, DLP, Threat) is responsible for writing to its own optimized store. There is no shared write path.

3. **Store first, commit offset second.** Kafka consumers write to the target data store before committing the offset. This guarantees at-least-once delivery. Idempotent keys (event_id unique index) provide exactly-once.

4. **Batch for throughput.** Write throughput is inversely proportional to the number of round-trips. Every consumer is designed around batch accumulation and bulk writes.

5. **Fail closed on the write path.** If the write pipeline is unhealthy (Kafka down, DB unreachable), the system rejects writes rather than silently losing data.

---

## References

| Document | What It Covers |
|----------|---------------|
| `docs/scaling/read.md` | Read path scaling, caching hierarchy |
| `docs/scaling/scale.md` | Overall scaling strategy, sharding, multi-tenancy |
| `docs/scaling/eda.md` | Event-driven architecture, outbox relay details |
| `docs/scaling/redis.md` | Redis scaling, cluster migration, bloom filters |
| `docs/index/ARCHITECTURE.md` | Core design patterns |
| `docs/index/INTENT_MAP.md` | Architectural decision log |
| `shared/kafka/outbox/` | Outbox writer + relay implementation |
| `shared/kafka/publisher.go` | Kafka publisher with trace propagation |
| `shared/kafka/envelope.go` | EventEnvelope wire format |
