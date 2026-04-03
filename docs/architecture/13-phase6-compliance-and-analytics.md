# §14 — Phase 6: Compliance & Analytics

**Goal:** ClickHouse receives bulk-inserted event stream. Report generation is concurrency-limited via injected Bulkhead. PDF output complete and signed. Analytics queries meet p99 < 100ms.

---

## 14.1 ClickHouse Schema

```sql
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
-- Do NOT partition by org_id — creates too many parts for 10k+ orgs.
-- org_id belongs only in ORDER BY. Daily partitioning prevents extreme partition sizes.
PARTITION BY toYYYYMMDD(occurred_at)
ORDER BY (org_id, type, occurred_at, event_id)
TTL occurred_at + INTERVAL 2 YEAR
SETTINGS index_granularity = 8192;

-- Materialized view for dashboard queries (O(1) aggregation)
CREATE MATERIALIZED VIEW IF NOT EXISTS event_counts_daily
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (org_id, type, day)
AS SELECT org_id, type, toDate(occurred_at) AS day, count() AS cnt
FROM events
GROUP BY org_id, type, day;

CREATE TABLE IF NOT EXISTS alert_stats (
    org_id       String,
    day          Date,
    severity     LowCardinality(String),
    count        UInt64,
    mttr_seconds UInt64
) ENGINE = SummingMergeTree(count, mttr_seconds)
ORDER BY (org_id, day, severity);
```

---

## 14.2 ClickHouse Bulk Insertion

```go
// pkg/consumer/clickhouse_writer.go
// Uses clickhouse-go v2 native batch API.
// Config: CLICKHOUSE_BULK_FLUSH_ROWS (5000), CLICKHOUSE_BULK_FLUSH_MS (2000)
// Manual Kafka offset commit after successful batch.Send().

func (w *ClickHouseWriter) Flush(ctx context.Context) error {
    batch, err := w.conn.PrepareBatch(ctx, "INSERT INTO events")
    if err != nil {
        return fmt.Errorf("prepare batch: %w", err)
    }
    for _, event := range w.buffer {
        if err := batch.Append(
            event.EventID, event.Type, event.OrgID, event.ActorID,
            event.ActorType, event.OccurredAt, event.Source, string(event.Payload),
        ); err != nil {
            return fmt.Errorf("append to batch: %w", err)
        }
    }
    return batch.Send()
}
```

### ClickHouse `FINAL` modifier — mandatory for compliance queries

`ReplacingMergeTree` deduplicates only during background merges. All compliance report queries targeting the `events` table **MUST** use the `FINAL` modifier:

```sql
-- CORRECT: forces deduplication at query time
SELECT type, count() AS cnt
FROM events FINAL
WHERE org_id = ? AND occurred_at BETWEEN ? AND ?
GROUP BY type
```

Do NOT use `FINAL` on `event_counts_daily` or `alert_stats` — the latency cost is not justified for dashboard queries.

---

## 14.3 Report Generation with Injected Bulkhead

```go
// main.go
bulkhead := resilience.NewBulkhead(config.DefaultInt("COMPLIANCE_REPORT_MAX_CONCURRENT", 10))
generator := reporter.NewGenerator(clickhouseClient, mongoClient, bulkhead)

// pkg/reporter/generator.go
type Generator struct {
    ch       *clickhouse.Client
    mongo    *mongo.Client
    bulkhead *resilience.Bulkhead  // injected, not package-level
}

func (g *Generator) Generate(ctx context.Context, report *Report) error {
    return g.bulkhead.Execute(ctx, func() error {
        return g.generate(ctx, report)
    })
}
```

When bulkhead is full: `ErrBulkheadFull` → `429` with `Retry-After: 30`.

### Report PDF Signing (RSA-PSS)

1. Generate PDF bytes.
2. Compute `SHA-256(pdf_bytes)`.
3. Sign: `sig = RSA-PSS-Sign(privateKey, sha256_hash)` (RSA-4096).
4. Store PDF as `{s3_key}.pdf` and detached signature as `{s3_key}.sig` in `compliance-reports` S3 bucket.
5. Store both keys in the PostgreSQL `reports` table.
6. Retrieval via pre-signed S3 URLs (TTL: 1 hour).

Add `COMPLIANCE_SIGNING_KEY_ARN` to `.env.example`.

---

## 14.4 Compliance API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/compliance/reports` | List reports |
| `POST` | `/v1/compliance/reports` | Trigger report (type: gdpr, soc2, hipaa) |
| `GET` | `/v1/compliance/reports/:id` | Status + download link |
| `GET` | `/v1/compliance/stats` | Compliance score and trends |
| `GET` | `/v1/compliance/posture` | Real-time posture vs controls |

---

## 14.5 Phase 6 Acceptance Criteria

- [ ] ClickHouse receives 10,000 events in ≤ 3 batches of ≤ 5,000 rows.
- [ ] Materialized view `event_counts_daily` populated automatically.
- [ ] `GET /compliance/stats` p99 < 100ms under load.
- [ ] GDPR report: 5 sections, valid PDF with ToC and page numbers.
- [ ] 11 concurrent report requests: 10 succeed, 11th returns 429.
- [ ] Bulkhead is injected via constructor (not package-level). Verified in unit test.
- [ ] Kafka offsets committed only after successful ClickHouse `batch.Send()`.
- [ ] ClickHouse partition by day only (no `org_id` partition). Verified in schema test.
