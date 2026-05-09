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
       │  FetchMessage()            │                              │
       │───────────────────────────>│                              │
       │                            │                              │
       │  (accumulate batch)        │                              │
       │  ... until 5000 events     │                              │
       │       or 2 seconds         │                              │
       │                            │                              │
       │                     flush()│                              │
       │                            │                              │
       │                            │  Deserialize each event      │
       │                            │  Map to Event struct:        │
       │                            │    event_id, type, org_id,   │
       │                            │    actor_id, actor_type,     │
       │                            │    occurred_at, source,      │
       │                            │    payload                   │
       │                            │                              │
       │                            │  PrepareBatch → append       │
       │                            │──────────────────────────────>│
       │                            │                              │
       │                            │  ┌─ Success → Commit offsets │
       │  CommitMessages() <───────│──│                          │
       │                            │  │                              │
       │                            │  └─ Failure → NO commit        │
       │                            │     (messages replay on        │
       │                            │      restart)                  │
```

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
