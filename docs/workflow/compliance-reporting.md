# Compliance & Reporting — Workflow

## Level 1: High-Level Architecture

```
                          ┌───────────────────────────────────────────────────────────────────────────┐
                          │                         EVENT SOURCE                                      │
                          │                                                                             │
                          │  Kafka: audit.trail (all audit events across all services)                  │
                          │  Produced by: Audit Service ingest, IAM outbox, Policy outbox, ...          │
                          └───────────────────────────┬───────────────────────────────────────────────┘
                                                       │
                                                       ▼
                          ┌───────────────────────────────────────────────────────────────────────────┐
                          │                   COMPLIANCE SERVICE (port 8088)                          │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │              KAFKA CONSUMER (audit.trail)                             │  │
                          │  │              Group: compliance-service-group                          │  │
                          │  │                                                                        │  │
                          │  │  Batch loop:                                                          │  │
                          │  │    FetchMessage → append to batch                                    │  │
                          │  │    When: len(batch) >= 5000 OR 2s elapsed                            │  │
                          │  │      → flush(batch):                                                  │  │
                          │  │         1. Deserialize events                                        │  │
                          │  │         2. Batch INSERT INTO ClickHouse (events table)                │  │
                          │  │         3. On success → commit Kafka offsets                          │  │
                          │  │         4. On failure → NO commit (replay on restart)                │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │              CLICKHOUSE DATA STORE (analytics)                       │  │
                          │  │                                                                        │  │
                          │  │  Table: events                                                         │  │
                          │  │    event_id     String   (ZSTD)                                       │  │
                          │  │    type         LowCardinality(String)                                 │  │
                          │  │    org_id       String   (ZSTD)                                       │  │
                          │  │    actor_id     String   (ZSTD)                                       │  │
                          │  │    actor_type   LowCardinality(String)                                 │  │
                          │  │    occurred_at  DateTime64(3)                                         │  │
                          │  │    source       LowCardinality(String)                                 │  │
                          │  │    payload      String   (ZSTD)                                       │  │
                          │  │                                                                        │  │
                          │  │  Engine: ReplacingMergeTree + TTL 2 YEAR                              │  │
                          │  │  Partition: by day (toYYYYMMDD)                                       │  │
                          │  │  Order: (org_id, type, occurred_at)                                   │  │
                          │  │                                                                        │  │
                          │  │  MV: event_counts_daily                                               │  │
                          │  │    Engine: SummingMergeTree                                           │  │
                          │  │    Pre-aggregated daily event counts by org, type                     │  │
                          │  │                                                                        │  │
                          │  │  Table: alert_stats                                                    │  │
                          │  │    Engine: SummingMergeTree                                           │  │
                          │  │    Alert counts + MTTR by org, day, severity                          │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │              REPORT GENERATION ENGINE                                │  │
                          │  │                                                                        │  │
                          │  │  POST /v1/compliance/reports → Create report job                      │  │
                          │  │    ├── "pending" → "generating" → "ready"                             │  │
                          │  │    │     (or "failed" on error)                                       │  │
                          │  │    │                                                                   │  │
                          │  │    ├── 1. Query ClickHouse (1-3 queries per framework)                │  │
                          │  │    ├── 2. Generate PDF via gofpdf                                     │  │
                          │  │    │      ├── Section 1: Executive Summary                           │  │
                          │  │    │      └── Section 2: Control Compliance Scores                   │  │
                          │  │    ├── 3. RSA-PSS sign PDF (if signing key configured)                │  │
                          │  │    ├── 4. Upload to S3/MinIO                                          │  │
                          │  │    │      reports/{orgID}/{jobID}.pdf                                │  │
                          │  │    │      reports/{orgID}/{jobID}.pdf.sig (optional)                 │  │
                          │  │    └── 5. Update status → "ready"                                    │  │
                          │  │                                                                        │  │
                          │  │  GET /v1/compliance/reports → List reports for org                    │  │
                          │  │  GET /v1/compliance/reports/{id} → Check status                       │  │
                          │  │  GET /v1/compliance/reports/{id}/download                             │  │
                          │  │    → 302 redirect to S3 presigned URL (1h TTL)                       │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │              BACKGROUND WORKER                                        │  │
                          │  │                                                                        │  │
                          │  │  Polls every 30s:                                                     │  │
                          │  │    SELECT * FROM reports WHERE status='pending'                       │  │
                          │  │    For each: dispatch via bulkhead (max 10 concurrent)                 │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │              DATA STORES                                             │  │
                          │  │                                                                        │  │
                          │  │  PostgreSQL: report_jobs table                                        │  │
                          │  │    id UUID PK, org_id UUID, framework TEXT,                           │  │
                          │  │    status TEXT (pending/generating/ready/failed),                     │  │
                          │  │    s3_key TEXT, s3_sig_key TEXT,                                      │  │
                          │  │    error_msg TEXT, created_at, updated_at                             │  │
                          │  │                                                                        │  │
                          │  │  S3/MinIO: compliance-reports bucket                                  │  │
                          │  │    reports/{orgID}/{jobID}.pdf                                       │  │
                          │  │    reports/{orgID}/{jobID}.pdf.sig (optional)                        │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          └───────────────────────────────────────────────────────────────────────────┘
```

---

## Level 2A: ClickHouse Ingestion Flow

```
  Kafka (audit.trail)        Compliance Consumer               ClickHouse
       │                            │                              │
       │  FetchMessage()           │                              │
       │───────────────────────────>│                              │
       │                            │                              │
       │  (accumulate batch)        │                              │
       │  ... until 5000 events     │                              │
       │       or 2 seconds         │                              │
       │                            │                              │
       │                     flush()│                              │
       │                            │                              │
       │  ┌─ Step 1: Deserialize ──│                              │
       │  │                         │                              │
       │  │  for each message:      │                              │
       │  │    json.Unmarshal(      │                              │
       │  │      m.Value, &event)   │                              │
       │  │    ↓                    │                              │
       │  │    repository.Event{    │                              │
       │  │      EventID           │                              │
       │  │      Type              │                              │
       │  │      OrgID             │                              │
       │  │      ActorID           │                              │
       │  │      ActorType         │                              │
       │  │      OccurredAt        │                              │
       │  │      Source            │                              │
       │  │      Payload           │                              │
       │  │    }                   │                              │
       │  │                         │                              │
       │  │  ┌─ Poison pill:        │                              │
       │  │  │  bad JSON → log+skip │                              │
       │  │  └─────────────────────│                              │
       │  │                         │                              │
       │  ├─ Step 2: Transform ────│                              │
       │  │                         │                              │
       │  │  If OccurredAt.IsZero() │                              │
       │  │    → OccurredAt = now() │                              │
       │  │                         │                              │
       │  │  All 8 fields are       │                              │
       │  │  directly mapped from   │                              │
       │  │  Kafka JSON → Event     │                              │
       │  │  struct (no field rename│                              │
       │  │  or reshaping)          │                              │
       │  │                         │                              │
       │  ├─ Step 3: Bulk Insert ──│                              │
       │  │                         │                              │
       │  │  PrepareBatch(          │                              │
       │  │   "INSERT INTO events") │                              │
       │  │                         │                              │
       │  │  for each event:        │                              │
       │  │    batch.Append(        │                              │
       │  │      EventID, Type,     │                              │
       │  │      OrgID, ActorID,    │                              │
       │  │      ActorType,         │                              │
       │  │      OccurredAt,        │                              │
       │  │      Source, Payload)   │                              │
       │  │                         │                              │
       │  │  batch.Send()           │                              │
       │  │──────────────────────────>                              │
       │  │                         │                              │
       │  │  ┌─ Success → Commit    │                              │
       │  │  │  offsets             │                              │
       │  │  └─────────────────────│                              │
       │  │                         │                              │
       │  │  ┌─ Failure → NO commit │                              │
       │  │  │  (messages replay    │                              │
       │  │  │   on restart)        │                              │
       │  │  └─────────────────────│                              │
       │  │                         │                              │
       │  │  ┌─ 0 valid events      │                              │
       │  │  │  (all poison):       │                              │
       │  │  │  commit offsets to   │                              │
       │  │  │  avoid re-processing │                              │
       │  │  │  undecodable msgs    │                              │
       │  │  └─────────────────────│                              │
       │  │                         │                              │
       │  └─────────────────────────┘                              │
       │                            │                              │
  ────────────────────────────────────────────────────────────────────
  ▼  Consumer Config                                                │
  ────────────────────────────                                      │
  Group:  compliance-service-group                                  │
  Topic:  audit.trail                                               │
  Batch:  5000 evts OR 2000ms                                       │
  Tuning: CLICKHOUSE_BULK_FLUSH_ROWS (env)                         │
          CLICKHOUSE_BULK_FLUSH_MS  (env)                          │
```

---

## Level 3A: Deserialize → Transform → Bulk Insert

### 3A.1 Kafka Message Envelope

The raw Kafka message on `audit.trail` contains a JSON byte payload. The compliance service unmarshals directly into `repository.Event`:

| Kafka JSON field | Go struct field | ClickHouse column | Type |
|---|---|---|---|
| `event_id` | `EventID` | `event_id` | String (ZSTD) |
| `type` | `Type` | `type` | LowCardinality(String) |
| `org_id` | `OrgID` | `org_id` | String (ZSTD) |
| `actor_id` | `ActorID` | `actor_id` | String (ZSTD) |
| `actor_type` | `ActorType` | `actor_type` | LowCardinality(String) |
| `occurred_at` | `OccurredAt` | `occurred_at` | DateTime64(3, 'UTC') |
| `source` | `Source` | `source` | LowCardinality(String) |
| `payload` | `Payload` | `payload` | String (ZSTD) |

**Note on field names:** Different upstream producers use different event field conventions:
- SDK `AuditEvent` uses `{"event_type": "...", "user_id": "...", "timestamp": "..."}`
- IAM outbox uses `{"event": "...", "user_id": "...", "timestamp": "..."}`
- Audit service uses `{"type": "...", "actor_id": "...", "occurred_at": "..."}`

The compliance service expects the **audit service convention** (`type`, `actor_id`, `occurred_at`). Events from other producers must be transformed before reaching `audit.trail` or the unmarshal will produce zero-value fields.

### 3A.2 Deserialization Logic (`flush()` in `clickhouse_writer.go`)

```
flush(ctx, messages []kafka.Message):
    events = []
    
    for m in messages:
        event = Event{}
        
        // Step 1: JSON Unmarshal
        err = json.Unmarshal(m.Value, &event)
        if err != nil:
            log.Error("failed to unmarshal event")
            continue              // ← skip poison pill
        
        // Step 2: Zero-time fallback
        if event.OccurredAt.IsZero():
            event.OccurredAt = time.Now()
        
        events.append(event)
    
    // Edge case: all messages were poison pills
    if len(events) == 0:
        commit offsets             // ← still commit to avoid
        return                     //    re-processing garbage
    
    // Step 3: Bulk insert
    err = repo.IngestEvents(ctx, events)
    if err != nil:
        log.Error("clickhouse ingest failed")
        return                     // ← do NOT commit offsets
    
    // Step 4: Commit on success
    err = reader.CommitMessages(ctx, messages...)
    if err != nil:
        log.Error("offset commit failed after successful ingest")
```

### 3A.3 Transformation Details

The transformation is intentionally **minimal** — a pass-through mapping:

1. **Field mapping:** Direct JSON-to-struct via struct tags (`ch:"event_id"`, `json:"event_id"`). No field renaming or reshaping occurs.
2. **Zero-time fallback:** If the upstream producer didn't set `occurred_at`, the consumer assigns `time.Now()`. This means events with missing timestamps get the ingestion time, not the original event time.
3. **No validation:** No schema validation, no type coercion beyond what `json.Unmarshal` provides. An empty `event_id` or missing `org_id` is accepted as-is.
4. **No deduplication:** Duplicate `event_id` values are allowed — ClickHouse `ReplacingMergeTree` handles dedup during `OPTIMIZE` or `SELECT ... FINAL`.

### 3A.4 ClickHouse Batch Insert (`IngestEvents()` in `repository.go`)

```go
func (r *Repository) IngestEvents(ctx context.Context, events []Event) error {
    batch, err := r.chConn.PrepareBatch(ctx, "INSERT INTO events")
    if err != nil {
        return err
    }

    for _, e := range events {
        if err := batch.Append(
            e.EventID, e.Type, e.OrgID,
            e.ActorID, e.ActorType,
            e.OccurredAt, e.Source, e.Payload,
        ); err != nil {
            return err
        }
    }

    return batch.Send()
}
```

**Key behaviors:**
- `PrepareBatch` creates a native ClickHouse batch insert — all rows sent in a single network round-trip
- `batch.Append` fails if any column type mismatches ClickHouse schema (e.g. string vs int)
- `batch.Send` flushes the batch; on failure, the entire batch is discarded — no partial insert
- The caller (ClickHouseWriter) controls offset commits: success → commit, failure → no commit

### 3A.5 Commit-or-Not Strategy

```
                          ┌──────────────────────┐
                          │   flush() called     │
                          │   (len(batch) > 0)   │
                          └──────────┬───────────┘
                                     │
                          ┌──────────▼───────────┐
                          │  Unmarshal events    │
                          │  from Kafka messages │
                          └──────────┬───────────┘
                                     │
                    ┌────────────────┼────────────────┐
                    │                │                  │
         ┌──────────▼───────┐  ┌────▼────────┐  ┌─────▼──────────┐
         │  0 valid events  │  │  Ingest OK  │  │  Ingest FAIL   │
         │  (all poison)    │  │             │  │                │
         └──────────┬───────┘  └────┬────────┘  └─────┬──────────┘
                    │               │                  │
         ┌──────────▼───────┐  ┌────▼────────┐  ┌─────▼──────────┐
         │  COMMIT offsets  │  │  COMMIT     │  │  NO COMMIT     │
         │  (avoid re-      │  │  offsets    │  │  (replay on    │
         │   processing     │  │             │  │   restart)     │
         │   undecodable    │  │             │  │                │
         │   forever)       │  │             │  │                │
         └──────────────────┘  └─────────────┘  └────────────────┘
```

**Why commit on all-poison?** Without this, a single bad message causes an infinite retry loop on restart — the consumer would fetch, fail to deserialize, fetch the same message again, etc. By committing offsets for poison batches, the consumer moves past undecodable messages at the cost of losing those events.

### 3A.6 Schema Initialization

The ClickHouse schema is auto-created at startup if it doesn't exist:

```sql
-- Table: events (ReplacingMergeTree, 2-year TTL)
CREATE TABLE IF NOT EXISTS events (
    event_id     String        CODEC(ZSTD(3)),
    type         LowCardinality(String),
    org_id       String        CODEC(ZSTD(3)),
    actor_id     String        CODEC(ZSTD(3)),
    actor_type   LowCardinality(String),
    occurred_at  DateTime64(3, 'UTC'),
    source       LowCardinality(String),
    payload      String        CODEC(ZSTD(3))
) ENGINE = ReplacingMergeTree(occurred_at)
  PARTITION BY toYYYYMMDD(occurred_at)
  ORDER BY (org_id, type, occurred_at, event_id)
  TTL toDateTime(occurred_at) + INTERVAL 2 YEAR
  SETTINGS index_granularity = 8192;

-- Materialized View: daily event counts (SummingMergeTree)
CREATE MATERIALIZED VIEW IF NOT EXISTS event_counts_daily
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (org_id, type, day)
AS SELECT org_id, type, toDate(occurred_at) AS day, count() AS cnt
FROM events GROUP BY org_id, type, day;

-- Table: alert_stats (SummingMergeTree)
CREATE TABLE IF NOT EXISTS alert_stats (
    org_id       String,
    day          Date,
    severity     LowCardinality(String),
    count        UInt64,
    mttr_seconds UInt64
) ENGINE = SummingMergeTree()
  ORDER BY (org_id, day, severity);
```

### 3A.7 Configuration

| Env Var | Default | Purpose |
|---|---|---|
| `CLICKHOUSE_BULK_FLUSH_ROWS` | `5000` | Max events per batch before flush |
| `CLICKHOUSE_BULK_FLUSH_MS` | `2000` | Max wait time before flush (ms) |
| `CLICKHOUSE_DB` | `default` | ClickHouse database name |
| `CLICKHOUSE_USER` | `default` | ClickHouse username |
| `CLICKHOUSE_PASSWORD` | (required) | ClickHouse password |

---

## Level 2B: Report Generation Flow

```
  Admin / Dashboard           Compliance Service              ClickHouse       S3          PostgreSQL
       │                            │                            │            │              │
       │  POST /v1/compliance/      │                            │            │              │
       │  reports                   │                            │            │              │
       │  { framework: "soc2",     │                            │            │              │
       │    org_id: "..." }         │                            │            │              │
       │───────────────────────────>│                            │            │              │
       │                            │                            │            │              │
       │                            │  Insert report job (status   │            │              │
       │                            │  = "pending")               │            │              │
       │                            │──────────────────────────────────────────────────────>│
       │                            │                            │            │              │
       │  <── 202 { job_id } ─────│                            │            │              │
       │                            │                            │            │              │
       │                            │  ┌─ Background processing   │            │              │
       │                            │  │                              │            │              │
       │                            │  │  Update status → "generating"             │              │
       │                            │  │──────────────────────────────────────────────────────>│
       │                            │  │                            │            │              │
       │                            │  │  Query posture:            │            │              │
       │                            │  │───────────────────────────>│            │              │
       │                            │  │  <── scores, counts ──────│            │              │
       │                            │  │                            │            │              │
       │                            │  │  Generate PDF:             │            │              │
       │                            │  │    A4, Arial               │            │              │
       │                            │  │    "{framework} Report"    │            │              │
       │                            │  │    Org: {orgID}            │            │              │
       │                            │  │    Generated: {now}        │            │              │
       │                            │  │    Section 1: Executive    │            │              │
       │                            │  │      Summary               │            │              │
       │                            │  │    Section 2: Compliance   │            │              │
       │                            │  │      Scores: X.XX%         │            │              │
       │                            │  │                            │            │              │
       │                            │  │  ┌─ Sign with RSA-PSS     │            │              │
       │                            │  │  │  (if key configured)    │            │              │
       │                            │  │  │                            │            │              │
       │                            │  │  Upload PDF:                │            │              │
       │                            │  │─────────────────────────────────────────>│              │
       │                            │  │  Upload .sig (optional)     │            │              │
       │                            │  │─────────────────────────────────────────>│              │
       │                            │  │                            │            │              │
       │                            │  │  Update status → "ready"   │            │              │
       │                            │  │──────────────────────────────────────────────────────>│
       │                            │  │                            │            │              │
       │                            │  │  ┌─ On any error:          │            │              │
       │                            │  │  │  status → "failed"      │            │              │
       │                            │  │  │  error_msg set          │            │              │
       │                            │  └────────────────────────────┘            │              │
       │                            │                                            │              │
       │  ┌─ Poll for completion     │                                            │              │
       │  │                            │                                            │              │
       │  GET /v1/compliance/         │                                            │              │
       │  reports/{job_id}          │                                            │              │
       │───────────────────────────>│                                            │              │
       │  <── { status: "ready",   │                                            │              │
       │         s3_key: "..." }   │                                            │              │
       │                            │                                            │              │
       │  GET /v1/compliance/       │                                            │              │
       │  reports/{id}/download    │                                            │              │
       │───────────────────────────>│                                            │              │
       │                            │  Generate presigned URL (1h TTL)           │              │
       │                            │─────────────────────────────────────────>  │              │
       │                            │<────────── presigned_url ──────────────────│              │
       │                            │                                            │              │
       │  <── 302 Redirect ────────│                                            │              │
       │       → presigned URL     │                                            │              │
       │                            │                                            │              │
       │  Download PDF directly     │                                            │              │
       │  from S3                  │                                            │              │
```

---

## Level 3: Compliance Scoring Logic

### Posture Calculation

```
  GetPosture(orgID):
    Query events (last 30 days):
      SELECT
        countIf(type LIKE 'auth.%')       as auth_events,
        countIf(type LIKE 'policy.%')     as policy_events,
        countIf(type LIKE 'data.access%') as access_events,
        countIf(type LIKE 'threat.%')     as threat_events
      FROM events FINAL
      WHERE org_id = $1
        AND occurred_at > now() - INTERVAL 30 DAY

    Scoring formulas (normalized, clamped at 100):

      GDPR_score  = min(100, auth_events*0.3  + access_events*0.7)
      SOC2_score  = min(100, auth_events*0.2  + policy_events*0.4  + threat_events*0.4)
      HIPAA_score = min(100, auth_events*0.2  + access_events*0.6  + threat_events*0.2)

    Return: { gdpr: X.XX, soc2: Y.YY, hipaa: Z.ZZ }
```

### Report Types

| Framework | Focus | Queries | Sections |
|-----------|-------|---------|----------|
| **GDPR** | Data access + auth events | Event counts, data access patterns | Executive summary, right-to-access, right-to-erasure, data processing records |
| **SOC2** | Auth + policy + threats | Policy changes, access grants, threat detections | Executive summary, access controls, monitoring, risk detection |
| **HIPAA** | Auth + data access + threats | PHI access events, auth attempts, threat alerts | Executive summary, access controls, audit controls, integrity controls |

---

## State Transitions

### Report Job State

```
                        ┌───────────┐
                        │  PENDING  │  (created by API or worker)
                        └─────┬─────┘
                              │
                        ┌─────▼──────┐
                        │ GENERATING │  (bulkhead acquired, PDF in progress)
                        └─────┬──────┘
                              │
                    ┌─────────┴──────────┐
                    │                    │
                    ▼                    ▼
              ┌──────────┐        ┌──────────┐
              │  READY   │        │  FAILED  │
              │ (S3 URL) │        │ (error)  │
              └──────────┘        └──────────┘
```

### Bulkhead State

```
  ┌─────────────────────────────────────────────────────┐
  │            BULKHEAD (report generation)              │
  │                                                     │
  │  Max concurrent: 10 (default)                       │
  │  Semaphore-based: TryAcquire (non-blocking)         │
  │                                                     │
  │  ┌─────────┐                                        │
  │  │  FREE   │  → Acquire → ┌──────────┐              │
  │  │ (N=10)  │             │ GENERATE │              │
  │  └─────────┘             │ (N-1)    │              │
  │          ↑               └──────────┘              │
  │          │                       ↓                  │
  │          │               ┌──────────┐              │
  │          └───────────────│ COMPLETE │              │
  │                          │ (Release)│              │
  │                          └──────────┘              │
  │                                                     │
  │  On full → HTTP 429 "Too Many Reports"              │
  │             Retry-After: 30                         │
  └─────────────────────────────────────────────────────┘
```

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Kafka (audit.trail)** | Durable event stream consumed by Compliance Service |
| **ClickHouse** | Long-term analytical storage with 2-year TTL and daily partitioning |
| **Compliance Service Consumer** | Batch ingestion from Kafka → ClickHouse with exactly-once semantics |
| **Compliance Service Handler** | Report generation orchestration, PDF creation, signing, S3 upload |
| **PostgreSQL** | Report job metadata tracking (not audit data) |
| **S3/MinIO** | Report PDF + signature storage with presigned URL access |
| **Angular Dashboard** | Displays posture scores, triggers report generation, downloads via redirect |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **ClickHouse down** | Compliance | Kafka offsets NOT committed; events replay on restart |
| **PostgreSQL down** | Compliance | Service crashes at startup (`os.Exit(1)`) |
| **S3/MinIO down** | Compliance | Report generation fails with "failed" status |
| **Kafka broker down** | Compliance | Consumer blocks on FetchMessage; no new events ingested |
| **Bulkhead full** | Compliance | HTTP 429 with Retry-After: 30 |
| **Poison pill (bad JSON)** | Compliance | Message skipped, offsets still committed (1 event lost) |
| **PDF generation OOM** | Compliance | Large PDF buffered in memory; no streaming to S3 |
| **RSA signing key missing** | Compliance | PDF uploaded unsigned (dev mode) |
| **Background worker panic** | Compliance | No recover() → service crash |
| **Many pending reports** | Compliance | GetPendingReports has no pagination → burst on recovery |
| **Presigned URL expires** | Compliance | Download link stale after 1h; user must regenerate |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_compliance_operations_total` | Counter | `operation`, `status` | Compliance Service |
| `openguard_report_bulkhead_rejected_total` | Counter | (none) | Compliance Service |
| `openguard_compliance_kafka_lag` | Gauge | `partition` | Compliance Service |
| `openguard_compliance_clickhouse_write_duration` | Histogram | (none) | Compliance Service |

### Key Traces (Jaeger)

- `compliance.kafka.consume` — from Kafka fetch to ClickHouse write
- `compliance.report.generate` — full report generation lifecycle
- `compliance.report.upload` — S3 upload + signing

### Audit Events

| Event | When | Payload |
|-------|------|---------|
| `compliance.report.created` | Report job inserted | job_id, framework, org_id |
| `compliance.report.completed` | Report ready on S3 | job_id, s3_key, duration |
| `compliance.report.failed` | Generation error | job_id, error, attempt |
| `compliance.report.downloaded` | Presigned URL accessed | job_id, org_id (via S3 logs) |

---

## Report PDF Structure

```
  ╔══════════════════════════════════════════╗
  ║        SOC2 Compliance Report           ║
  ║                                          ║
  ║  Organization: org-abc-123               ║
  ║  Report ID:   job-xyz-789                ║
  ║  Generated:   2026-01-15T10:30:00Z      ║
  ║  Framework:   SOC2                       ║
  ║                                          ║
  ║  ──────────────────────────────────────  ║
  ║  Section 1: Executive Summary            ║
  ║                                          ║
  ║  This report summarizes the security     ║
  ║  controls... (free text)                 ║
  ║                                          ║
  ║  ──────────────────────────────────────  ║
  ║  Section 2: Control Compliance           ║
  ║                                          ║
  ║  SOC2 Score: 87.50%                     ║
  ║                                          ║
  ║  ──────────────────────────────────────  ║
  ║                                          ║
  ║  Signed: [RSA-PSS signature block]      ║
  ╚══════════════════════════════════════════╝
```
