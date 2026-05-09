# Audit & Event Pipeline — Workflow

## Level 1: High-Level Architecture

```
                          ┌─────────────────────────────────────────────────────────────────────────────┐
                          │                         EVENT PRODUCERS                                     │
                          │                                                                               │
                          │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
                          │  │  IAM Service  │  │ Policy Svc   │  │  SDK/Apps    │  │  Threat Svc  │    │
                          │  │  (port 8082) │  │ (port 8083)  │  │  (external)  │  │  (port 8084) │    │
                          │  │  auth.events  │  │policy.changes│  │  data.access │  │threat.alerts │    │
                          │  │  saga.orchest │  │              │  │  audit.trail │  │              │    │
                          │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │
                          │         │                 │                 │                 │            │
                          │         │   Each producer uses Transactional Outbox pattern   │            │
                          │         │   Write(tx) → pg_notify → Relay → Kafka             │            │
                          │         └─────────┬───────────────────────┬────────────────────┘            │
                          └───────────────────┼───────────────────────┼────────────────────────────────┘
                                              ▼                       ▼
                          ┌─────────────────────────────────────────────────────────────────────────────┐
                          │                           KAFKA EVENT BUS (7 topics consumed)               │
                          │                                                                               │
                          │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
                          │  │ auth.events  │  │policy.changes│  │ data.access  │  │ audit.trail  │    │
                          │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │
                          │         │                 │                 │                 │            │
                          │  ┌──────▼───────┐  ┌──────▼───────┐  ┌──────▼───────┐  ┌──────▼───────┐    │
                          │  │ threat.alerts │  │connector.ev  │  │saga.orchest. │  │              │    │
                          │  └──────────────┘  └──────────────┘  └──────────────┘  │              │    │
                          └─────────────────────────────────────────────────────────┼──────────────┘    │
                                                                                    │                    │
                          ┌─────────────────────────────────────────────────────────┼──────────────┐    │
                          │                     AUDIT SERVICE (port 8085)           │              │    │
                          │                                                         ▼              │    │
                          │  ┌────────────────────────────────────────────────────────────────────┐ │    │
                          │  │                   7 PARALLEL KAFKA CONSUMERS                       │ │    │
                          │  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐┐ │    │
                          │  │  │ auth.ev  │ │ policy   │ │ data.ac  │ │ threat   │ │ audit.t  ││ │    │
                          │  │  │ consumer │ │ consumer │ │ consumer │ │ consumer │ │ consumer ││ │    │
                          │  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘│ │    │
                          │  │  ┌──────────┐ ┌──────────┐                                      ││ │    │
                          │  │  │ connect  │ │ saga     │                                      ││ │    │
                          │  │  │ consumer │ │ consumer │                                      ││ │    │
                          │  │  └──────────┘ └──────────┘                                      ││ │    │
                          │  └──────────────────────────────┬───────────────────────────────────┘│ │    │
                          │                                 │                                    │ │    │
                          │                                 ▼                                    │ │    │
                          │  ┌────────────────────────────────────────────────────────────────────┐│ │    │
                          │  │               BATCH PROCESSOR (up to 500 events / 1s)              ││ │    │
                          │  │                                                                    ││ │    │
                          │  │  ┌────────────────────────────────────────────────────────────┐   ││ │    │
                          │  │  │  Step 1: ReserveSequence (atomic $inc in MongoDB)          │   ││ │    │
                          │  │  │  Step 2: Compute HMAC-SHA256 hash chain (event_id|prev)    │   ││ │    │
                          │  │  │  Step 3: Compare-And-Swap update chain head (CAS retry)    │   ││ │    │
                          │  │  │  Step 4: BulkWrite to MongoDB (ordered, idempotent)        │   ││ │    │
                          │  │  │  Step 5: Commit Kafka offsets                              │   ││ │    │
                          │  │  └────────────────────────────────────────────────────────────┘   ││ │    │
                          │  └────────────────────────────────────────────────────────────────────┘│ │    │
                          └────────────────────────────────┬───────────────────────────────────────┘ │    │
                                                           │                                         │    │
                              ┌────────────────────────────┼─────────────────────────────┐           │    │
                              │                            │                             │           │    │
                              ▼                            ▼                             ▼           │    │
                    ┌──────────────────┐          ┌──────────────────┐          ┌──────────────────┐  │    │
                    │  MongoDB (CQRS)  │          │  REST API + SSE  │          │  Kafka Ingest    │  │    │
                    │                  │          │                  │          │  Publisher       │  │    │
                    │  Write: primary  │          │  GET /v1/events  │          │  POST /v1/events │  │    │
                    │  (majority WC)   │          │  → secondary     │          │  /ingest         │  │    │
                    │                  │          │  GET /v1/events/ │          │  → publish to    │  │    │
                    │  Read: secondary │          │  stream (SSE)    │          │    audit.trail   │  │    │
                    │  (secondary pref)│          │  → Change Stream │          │                  │  │    │
                    │                  │          │                  │          │  202 Accepted    │  │    │
                    │  Collections:    │          │                  │          │  + DLP check     │  │    │
                    │  - audit_events  │          │                  │          │  (if block mode) │  │    │
                    │  - hash_chains   │          │                  │          │                  │  │    │
                    └──────────────────┘          └──────────────────┘          └──────────────────┘  │    │
```

---

## Level 2A: Event Ingest Flow (Synchronous)

```
                              INGEST ENDPOINT FLOW

  SDK/App                    Audit Service                        DLP Service            Kafka
    │                            │                                    │                    │
    │  POST /v1/events/ingest    │                                    │                    │
    │  { event payload }         │                                    │                    │
    │───────────────────────────>│                                    │                    │
    │                            │                                    │                    │
    │                            │  Extract org_id from JWT context   │                    │
    │                            │  Decode JSON event body            │                    │
    │                            │  Inject org_id, generate event_id  │                    │
    │                            │                                    │                    │
    │                            │  ┌─ DLP MODE == "block"?           │                    │
    │                            │  │  POST /v1/scan (2s timeout)     │                    │
    │                            │  │────────────────────────────────>│                    │
    │                            │  │                                 │                    │
    │                            │  │  ┌─ Findings found              │                    │
    │                            │  │  │  ←─── 422 ──────────────────│                    │
    │  <── 422 DLP Blocked ──────│──│──│──────────────────────────────│                    │
    │                            │  │  │                              │                    │
    │                            │  │  ┌─ No findings / DLP down     │                    │
    │                            │  │  │  ←─── 200 ──────────────────│                    │
    │                            │  │  │                              │                    │
    │                            │  └─ DLP MODE != "block" (skip)    │                    │
    │                            │                                    │                    │
    │                            │  Publish to Kafka (audit.trail)    │                    │
    │                            │  (RequireAll acks, sync publish)   │                    │
    │                            │──────────────────────────────────────────────────────>│
    │                            │                                    │                    │
    │  <── 202 Accepted ────────│────────────────────────────────────│                    │
```

---

## Level 2B: Async Consumer & Hash Chain Flow

```
                          BATCH PROCESSING SEQUENCE (exactly-once)

  Kafka                       Audit Consumer (per-topic)            MongoDB
    │                                │                                │
    │  FetchMessage (blocking)       │                                │
    │  ── accumulate into batch ──>  │                                │
    │  ── until 500 events or 1s ──> │                                │
    │                                │                                │
    │                     flush()    │                                │
    │                                │                                │
    │                                │  Step 1: ReserveSequence       │
    │                                │  ($inc: sequence by count)     │
    │                                │───────────────────────────────>│
    │                                │<── {startSeq, prevHash} ───────│
    │                                │                                │
    │                                │  Step 2: Compute hash chain    │
    │                                │  for each event(i):            │
    │                                │    chainInput = event_id + "|" │
    │                                │                 + prevHash     │
    │                                │    integrity_hash =            │
    │                                │      HMAC-SHA256(secret,       │
    │                                │                   chainInput)  │
    │                                │    prevHash = integrity_hash   │
    │                                │                                │
    │                                │  Step 3: CAS Update            │
    │                                │  UpdateHashChainCAS(orgID,     │
    │                                │    prevHash, newHash)          │
    │                                │───────────────────────────────>│
    │                                │<── ok=true/false ──────────────│
    │                                │                                │
    │                                │  ┌─ CAS failed (conflict)      │
    │                                │  │  → retry from Step 1       │
    │                                │  │  (max 5 retries)            │
    │                                │  │                                │
    │                                │  ┌─ CAS succeeded              │
    │                                │  │  Step 4: BulkWrite          │
    │                                │  │  (ordered=true, idempotent) │
    │                                │  │────────────────────────────>│
    │                                │  │<── success ─────────────────│
    │                                │  │                                │
    │                                │  │  Step 5: Commit Kafka offset│
    │  CommitMessages(batch...) <────│──│                            │
    │                                │  │                              │
    │                                │  Emit metrics                   │
    │                                │  (flush duration, batch size)  │
```

### Hash Chain Detail

```
                         HMAC-SHA256 SEQUENTIAL LINKING

  Chain definition:
    H(0)   = ""                                   (genesis)
    H(n)   = HMAC-SHA256(SecretKey, event_id(n) + "|" + H(n-1))

  Per-batch operation:
    ┌─────────────────────────────────────────────────────────────┐
    │                     BATCH N                                   │
    │                                                               │
    │  Previous state (MongoDB hash_chains):                       │
    │    { org_id: "...", sequence: 100, hash: "H(100)" }          │
    │                                                               │
    │  Step 1: ReserveSequence(10 events)                          │
    │    Returns: startSeq=101, prevHash="H(100)"                  │
    │                                                               │
    │  Step 2: Compute chain locally:                               │
    │    H(101) = HMAC(secret, "evt_101|H(100)")                   │
    │    H(102) = HMAC(secret, "evt_102|H(101)")                   │
    │    ...                                                        │
    │    H(110) = HMAC(secret, "evt_110|H(109)")                   │
    │                                                               │
    │  Step 3: CAS update:                                          │
    │    WHERE hash == "H(100)" SET hash = "H(110)"                │
    │    ┌─ Success → next batch chains from H(110)                 │
    │    └─ Failure → another consumer wrote a different H(110')    │
    │                → retry: re-reserve, re-chain from H(110')    │
    │                                                               │
    │  Step 4: BulkWrite events with their integrity_hash fields    │
    └─────────────────────────────────────────────────────────────┘

  Gap detection:
    If pod crashes between Step 3 and Step 4:
      - MongoDB hash_chains.sequence jumped by 10
      - But audit_events has a gap in sequence numbers
      - Detectable via: missing sequence N where H(N-1) exists
```

---

## Level 2C: SSE Streaming Flow

```
  Angular Dashboard          Audit Service                    MongoDB
    │                            │                              │
    │  GET /v1/events/stream     │                              │
    │  (JWT + org_id)            │                              │
    │───────────────────────────>│                              │
    │                            │  Auth middleware validates   │
    │                            │  JWT, extracts org_id        │
    │                            │                              │
    │                            │  coll.Watch(pipeline)        │
    │                            │  $match: org_id == orgID     │
    │                            │  Options: UpdateLookup       │
    │                            │─────────────────────────────>│
    │                            │<── change stream cursor ─────│
    │                            │                              │
    │  ┌─ SSE Loop ─────────────│                              │
    │  │                        │                              │
    │  │  <── event: id={event_id}, data={fullDocument} ───────│
    │  │                        │                              │
    │  │  <── event: ... (next change)                        │
    │  │                        │                              │
    │  └────────────────────────│                              │
```

---

## Level 3: State Transitions

### Hash Chain Head State

```
                          ┌─────────────────────────────────────────────────────┐
                          │              HASH CHAIN HEAD (per org)               │
                          │                                                     │
                          │  MongoDB: hash_chains collection                    │
                          │  { org_id, sequence, hash, created_at, updated_at } │
                          │                                                     │
                          │     ┌──────────────┐                               │
                          │     │  sequence:0  │  (org created, no events yet) │
                          │     │  hash: ""    │                               │
                          │     └──────┬───────┘                               │
                          │            │                                        │
                          │     ┌──────▼───────┐                               │
                          │     │  sequence:N   │  (N events ingested)          │
                          │     │  hash: H(N)   │                               │
                          │     └──────┬───────┘                               │
                          │            │                                        │
                          │     ┌──────▼───────┐                               │
                          │     │  sequence:N+M  │  (next batch of M events)    │
                          │     │  hash: H(N+M)  │                               │
                          │     └──────────────┘                               │
                          │                                                     │
                          │  CAS update: only succeeds if prevHash matches      │
                          │  If conflict → retry from ReserveSequence           │
                          └─────────────────────────────────────────────────────┘
```

### Kafka Consumer Offset State

```
                          ┌─────────────────────────────────────────────┐
                          │         CONSUMER OFFSET (per partition)     │
                          │                                             │
                          │     ┌────────────┐                         │
                          │     │  LAST      │  (offset of last         │
                          │     │  COMMITTED │   committed message)     │
                          │     └──────┬─────┘                         │
                          │            │                                │
                          │            │ flush() called                 │
                          │     ┌──────▼─────┐                         │
                          │     │  PROCESSING │  (batch in progress)     │
                          │     │  BATCH      │                         │
                          │     └──────┬─────┘                         │
                          │            │                                │
                          │      ┌─────┴──────┐                        │
                          │      │            │                         │
                          │      ▼            ▼                         │
                          │  ┌────────┐  ┌────────┐                    │
                          │  │COMMIT  │  │ ROLLBACK│  (crash → replay) │
                          │  │SUCCESS │  │(no ack) │                   │
                          │  └────────┘  └────────┘                    │
                          │                                             │
                          │  At-least-once: offsets committed only     │
                          │  after successful MongoDB write             │
                          └─────────────────────────────────────────────┘
```

---

## Consumer Group Mapping

| Topic | Consumer Group | Batch Size | Flush Interval | Purpose |
|-------|---------------|------------|----------------|---------|
| `auth.events` | `audit-service-auth.events` | 500 | 1s | Auth events → audit trail |
| `policy.changes` | `audit-service-policy.changes` | 500 | 1s | Policy changes → audit trail |
| `data.access` | `audit-service-data.access` | 500 | 1s | Data access → audit trail |
| `threat.alerts` | `audit-service-threat.alerts` | 500 | 1s | Threat alerts → audit trail |
| `connector.events` | `audit-service-connector.events` | 500 | 1s | Connector events → audit trail |
| `audit.trail` | `audit-service-audit.trail` | 500 | 1s | Audit events from SDK/ingest → audit trail |
| `saga.orchestration` | `audit-service-saga.orchestration` | 500 | 1s | Saga events → audit trail |

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Event Producers (IAM, Policy, SDK)** | Write events to outbox table within business logic transactions |
| **Outbox Relay** | Polls outbox table, publishes to Kafka, marks as published |
| **Kafka** | Persistent message bus with 7 topics consumed by Audit Service |
| **Audit Service (Ingest)** | Synchronous HTTPS endpoint for external SDKs; DLP block-mode check |
| **Audit Service (Consumers)** | 7 parallel goroutines consuming Kafka topics, batching, hashing, persisting |
| **MongoDB (Primary)** | Majority write concern for hash chain and event persistence |
| **MongoDB (Secondary)** | SecondaryPreferred reads for query performance |
| **Angular Dashboard** | SSE streaming via Change Streams for real-time event display |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Kafka broker down** (ingest) | Audit | Publisher returns error → ingest returns 500 (fail-closed) |
| **Kafka broker down** (consumer) | Audit | Consumer blocks on FetchMessage, retries with backoff |
| **MongoDB primary down** | Audit | BulkWrite fails → no offset commit → replay on restart |
| **Hash chain CAS conflict** | Audit | Retry up to 5× (100ms backoff); exhaustion → batch dropped (sequence gap) |
| **Pod crash** (after CAS, before BulkWrite) | Audit | Sequence incremented but events missing → detectable gap |
| **Pod crash** (after BulkWrite, before commit) | Audit | Events re-inserted (idempotent via event_id unique index) |
| **Missing AUDIT_SECRET_KEY** | Audit | Events written WITHOUT integrity_hash (degraded) |
| **Missing org_id in event** | Audit | Batch silently dropped (cannot route without tenant) |
| **DLP service down (block mode)** | Audit | Ingest returns 422 (fail-closed) |
| **SSE Change Stream error** | Audit | Connection drops; client reconnects with lastEventId |
| **Poison pill (bad JSON)** | Audit | Skip message, commit offset (1 event lost) |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_audit_events_ingested_total` | Counter | `org_id`, `topic` | Audit Service |
| `openguard_audit_batch_flush_duration_seconds` | Histogram | `status` | Audit Service |
| `openguard_audit_hash_chain_sequence` | Gauge | `org_id` | Audit Service |
| `openguard_audit_chain_integrity_failures_total` | Counter | `org_id` | Audit Service |
| `openguard_audit_kafka_bulk_insert_size` | Histogram | (none) | Audit Service |
| `openguard_kafka_offset_commit_duration_seconds` | Histogram | (none) | Shared Kafka |

### Key Traces (Jaeger)

- `ingest-audit-event` — from HTTP receive to Kafka publish
- `consume-audit-event` — per-event span (7 topics)
- `audit-batch-flush` — full flush cycle (sequence → hash → CAS → write → commit)

### Audit Log Events

| Event | When | Payload |
|-------|------|---------|
| `audit.event.ingested` | Event published to Kafka | event_id, org_id, type |
| `audit.batch.flushed` | Batch persisted to MongoDB | org_id, topic, count, sequence_range |
| `audit.chain.updated` | Hash chain CAS succeeded | org_id, sequence, new_hash |
| `audit.chain.conflict` | CAS conflict detected | org_id, expected_hash, actual_hash |
| `audit.stream.connected` | SSE client connected | org_id, client_ip |

---

## Data Flow Summary

```
  External                          Audit Service                              MongoDB
  ────────                          ────────────                              ──────

  SDK POST /v1/events/ingest
    │
    ├── DLP check (block mode)      ─→ DLP Service
    │
    └── Publish to kafka: audit.trail
                                    │
  IAM/Policy (via Outbox)           │
    └── Publish to: auth.events,    │
        policy.changes, ...         │
                                    │
  All 7 topics ────────────────────→│
                                    │
                                    │  ← ReserveSequence (MongoDB $inc)
                                    │  ← Compute HMAC-SHA256 chain
                                    │  ← CAS update hash chain head
                                    │  ← BulkWrite audit_events
                                    │  ← Commit Kafka offsets
                                    │
                                    ▼
                              ┌──────────────────┐
                              │ audit_events:     │
                              │  {event_id,       │
                              │   org_id,         │
                              │   type,           │
                              │   timestamp,      │
                              │   sequence,       │
                              │   integrity_hash, │
                              │   payload}        │
                              │                   │
                              │ hash_chains:      │
                              │  {org_id,         │
                              │   sequence,       │
                              │   hash,           │
                              │   created_at,     │
                              │   updated_at}     │
                              └──────────────────┘
                                    │
  Read Path                         │
    GET /v1/events   ───────────────┤ (secondary preferred)
    GET /v1/events/stream ──────────┤ (Change Stream, SSE)
```
