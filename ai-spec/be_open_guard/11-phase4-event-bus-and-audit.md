# §12 — Phase 4: Event Bus & Audit Log

**Goal:** Kafka fully operational. Outbox relay running in all services. Audit Log consumes all events with manual-commit consumers, bulk inserts, atomic hash chaining, and CQRS read/write split.

---

## 12.1 Kafka Topic Configuration

```json
[
  { "name": "auth.events",            "partitions": 12, "replication": 3, "retention_ms": 604800000,  "compression": "lz4" },
  { "name": "policy.changes",         "partitions": 6,  "replication": 3, "retention_ms": 604800000,  "compression": "lz4" },
  { "name": "data.access",            "partitions": 24, "replication": 3, "retention_ms": 259200000,  "compression": "lz4" },
  { "name": "threat.alerts",          "partitions": 12, "replication": 3, "retention_ms": 2592000000, "compression": "lz4" },
  { "name": "audit.trail",            "partitions": 24, "replication": 3, "retention_ms": -1,         "compression": "lz4" },
  { "name": "notifications.outbound", "partitions": 6,  "replication": 3, "retention_ms": 86400000,   "compression": "lz4" },
  { "name": "saga.orchestration",     "partitions": 12, "replication": 3, "retention_ms": 604800000,  "compression": "lz4" },
  { "name": "outbox.dlq",             "partitions": 3,  "replication": 3, "retention_ms": -1,         "compression": "lz4" },
  { "name": "connector.events",       "partitions": 24, "replication": 3, "retention_ms": 259200000,  "compression": "lz4" },
  { "name": "webhook.delivery",       "partitions": 12, "replication": 3, "retention_ms": 86400000,   "compression": "lz4" },
  { "name": "webhook.dlq",            "partitions": 3,  "replication": 3, "retention_ms": -1,         "compression": "lz4" }
]
```

Replication factor 3 requires 3 brokers in staging/production. `create-topics.sh` detects broker count and adjusts automatically.

## 12.1.1 Kafka Consumer Group Tuning

All consumers MUST use these settings to minimize rebalance impact:

```yaml
session.timeout.ms: 45000
heartbeat.interval.ms: 3000
max.poll.interval.ms: 300000
partition.assignment.strategy: cooperative-sticky  # Incremental rebalance — mandatory
```

The `CooperativeStickyAssignor` is mandatory. With eager rebalancing (default), all consumers stop during rebalancing. With incremental rebalancing, only partitions that move are paused.

### 12.1.1.1 Library Note: Cooperative Sticky Assignor

kafka-go (`github.com/segmentio/kafka-go`) does not implement
CooperativeStickyAssignor. Two options:

**Option A (Recommended):** Migrate consumers to `github.com/confluentinc/confluent-kafka-go/v2`.
This is the only Go library with full cooperative-sticky support.
Trade-off: Requires librdkafka CGO dependency. Use the static build tag:
`CGO_ENABLED=1 go build -tags static`.

**Option B:** Accept eager rebalancing with kafka-go. Mitigate by:
- Setting `session.timeout.ms = 45000` (reduce rebalance window)
- Using `MaxWait = 1s` on FetchMessage to reduce pause during rebalance
- Monitoring `kafka_consumer_group_rebalances_total` metric

The Dockerfile base image must include build-essential for Option A.
The current codebase uses kafka-go. **Migration to confluent-kafka-go is
required before Phase 4 acceptance criteria can be met for cooperative rebalancing.**

---

## 12.2 Audit Log Service — CQRS Architecture

```
services/audit/pkg/
├── consumer/
│   ├── bulk_writer.go      # Buffers + bulk-inserts to MongoDB primary
│   └── hash_chain.go       # Atomic chain sequence + HMAC computation
├── repository/
│   ├── write.go            # Uses MONGO_URI_PRIMARY, write concern majority
│   └── read.go             # Uses MONGO_URI_SECONDARY, readPreference: secondaryPreferred
├── handlers/
│   ├── events.go           # GET /audit/events
│   └── export.go
└── integrity/
    └── verifier.go
```

### 12.2.1 Kafka Consumer (Manual Offset Commit)

Flow per batch:
1. Poll up to `AUDIT_BULK_INSERT_MAX_DOCS` messages (or wait `AUDIT_BULK_INSERT_FLUSH_MS`).
2. `BulkWriter.Flush()` → MongoDB `BulkWrite(ordered=false)` with `w:majority`.
3. On success: `kafkaConsumer.CommitOffsets()`.
4. On failure: do NOT commit, retry up to 5 times, then route to dead-letter collection.

### 12.2.2 Bulk Writer

```go
// Flush writes buffered documents to MongoDB as a single BulkWrite.
// ordered=false: continues on duplicate key errors (idempotent reprocessing).
// Retries up to 5 times with exponential backoff on primary failover errors.
// Contract: do not call Add() while Flush() and offset commit are in progress.
//
// On duplicate key errors (error code 11000): safe to commit offsets — the event
// was already written (this is the at-least-once replay path).
```

### 12.2.3 Atomic Hash Chain (Batched Reservation)

```go
// Atomic sequence assignment via findOneAndUpdate with $inc:
//   result = db.audit_chain_state.findOneAndUpdate(
//     { _id: orgID },
//     { $inc: { seq: batchSize } },
//     { upsert: true, returnDocument: "after" }
//   )
//   chain_seq_start = result.seq - batchSize + 1
//
// Consumer assigns sequence numbers to batch in memory, then bulk-inserts.
// O(1) DB ops per batch.

// ChainHash computes HMAC-SHA256 of: prev_hash + event_id + org_id + type + occurred_at.Unix()
// Key: AUDIT_HASH_CHAIN_SECRET
func ChainHash(secret, prevHash string, event AuditEvent) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(prevHash))
    mac.Write([]byte(event.EventID))
    mac.Write([]byte(event.OrgID))
    mac.Write([]byte(event.Type))
    mac.Write([]byte(strconv.FormatInt(event.OccurredAt.Unix(), 10)))
    return hex.EncodeToString(mac.Sum(nil))
}
```

### 12.2.4 MongoDB Schema

Collection: `audit_events`
```js
db.audit_events.createIndex({ org_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ org_id: 1, type: 1, occurred_at: -1 })
db.audit_events.createIndex({ actor_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ event_id: 1 }, { unique: true })  // dedup key
db.audit_events.createIndex({ org_id: 1, chain_seq: 1 })        // integrity checks
db.audit_events.createIndex({ occurred_at: 1 }, { expireAfterSeconds: <retention_seconds> })
```

Collection: `audit_chain_state` — `{ _id: org_id, seq: <int64>, last_hash: "<hex>" }`

### 12.2.5 Audit HTTP API

| Method | Path | Description |
|---|---|---|
| `GET` | `/audit/events` | List events (cursor paginated; reads from secondary) |
| `GET` | `/audit/events/:id` | Get single event |
| `POST` | `/audit/export` | Trigger async CSV/JSON export |
| `GET` | `/audit/export/:job_id` | Poll export job status |
| `GET` | `/audit/export/:job_id/download` | Stream download |
| `GET` | `/audit/integrity` | Verify hash chain for org (uses PRIMARY) |
| `GET` | `/audit/stats` | Event counts by type and day |

---

## 12.3 Phase 4 Acceptance Criteria

- [ ] Kafka consumer processes 50,000 events/s sustained.
- [ ] Bulk writer: each batch ≤ 500 docs, flush interval ≤ 1000ms.
- [ ] Kafka offsets committed only after successful MongoDB BulkWrite.
- [ ] Event from IAM login appears in MongoDB within p99 2s end-to-end.
- [ ] Duplicate `event_id`: second insert skipped, batch succeeds, offsets committed.
- [ ] Service crash before offset commit: events reprocessed on restart, duplicates silently skipped.
- [ ] `GET /audit/events` uses MongoDB secondary (verified with `explain()`).
- [ ] `GET /audit/integrity` returns `ok: true` on clean chain.
- [ ] Manually deleting a document → `GET /audit/integrity` reports a gap at the missing `chain_seq`.
- [ ] Chain sequence assignment: 100 concurrent events for the same org → all have unique, sequential `chain_seq` values.
