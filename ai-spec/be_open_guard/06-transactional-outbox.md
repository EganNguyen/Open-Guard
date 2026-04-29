# §7 — Transactional Outbox Pattern

---

## 7.1 Outbox Table (every service that publishes events)

```sql
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,
    payload      BYTEA NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON outbox_records(created_at) WHERE status = 'pending';

ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);
GRANT BYPASSRLS ON TABLE outbox_records TO openguard_outbox;

GRANT SELECT, INSERT, UPDATE ON outbox_records TO openguard_app;
GRANT SELECT, UPDATE, DELETE ON outbox_records TO openguard_outbox;

-- NOTIFY trigger for immediate relay wake-up
CREATE OR REPLACE FUNCTION notify_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('outbox_new', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
    AFTER INSERT ON outbox_records
    FOR EACH ROW EXECUTE FUNCTION notify_outbox();
```

---

## 7.2 Outbox Writer

```go
// shared/kafka/outbox/writer.go
package outbox

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/jackc/pgx/v5"
    "github.com/openguard/shared/models"
)

type Writer struct{}

// Write inserts an EventEnvelope into the outbox within the provided transaction.
// orgID is passed explicitly and written to the org_id column for correct RLS enforcement.
// This is separate from the Kafka partition key (key parameter) which may be different.
func (w *Writer) Write(ctx context.Context, tx pgx.Tx, topic, key, orgID string, envelope models.EventEnvelope) error {
    payload, err := json.Marshal(envelope)
    if err != nil {
        return fmt.Errorf("marshal envelope: %w", err)
    }
    _, err = tx.Exec(ctx,
        `INSERT INTO outbox_records (org_id, topic, key, payload)
         VALUES ($1, $2, $3, $4)`,
        orgID, topic, key, payload,
    )
    return err
}
```

---

## 7.3 Outbox Relay

```go
// shared/kafka/outbox/relay.go
package outbox

// Relay reads pending outbox records and publishes them to Kafka.
//
// Architecture:
//   - Uses PostgreSQL LISTEN/NOTIFY to wake up immediately on new records.
//   - Falls back to polling every 100ms (time.NewTicker, never time.Sleep).
//   - Uses FOR UPDATE SKIP LOCKED for safe concurrent relay instances.
//
// Delivery guarantee:
//   - At-least-once delivery to Kafka.
//   - Kafka idempotent producer prevents duplicates.
//   - Records marked "published" only after Kafka ack (sync produce).
//   - Records failing 5 times are marked "dead" and sent to outbox.dlq.
type Relay struct {
    pool     *pgxpool.Pool // uses openguard_outbox role (BYPASSRLS on outbox_records)
    producer kafka.SyncProducer
    logger   *slog.Logger
    maxDead  int // default 5
}

func NewRelay(pool *pgxpool.Pool, producer kafka.SyncProducer, logger *slog.Logger) *Relay {
    if pool == nil {
        panic("NewRelay: pool is required")
    }
    return &Relay{pool: pool, producer: producer, logger: logger, maxDead: 5}
}

func (r *Relay) Run(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)
    g.Go(func() error { return r.listenNotify(ctx) })
    g.Go(func() error { return r.pollLoop(ctx) })
    return g.Wait()
}

func (r *Relay) pollLoop(ctx context.Context) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if _, err := r.processBatch(ctx); err != nil {
                r.logger.ErrorContext(ctx, "outbox relay batch failed", "error", err)
            }
        }
    }
}

// processBatch selects up to 100 pending records with FOR UPDATE SKIP LOCKED,
// publishes each to Kafka synchronously, then updates status.
//
// SECURITY: relayTotalInstances and relayInstanceIndex MUST be validated as
// non-negative integers before use. They are passed as query parameters ($1, $2),
// NOT via fmt.Sprintf, to prevent SQL injection from config manipulation.
func (r *Relay) processBatch(ctx context.Context) (int, error) {
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return 0, fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx)

    rows, err := tx.Query(ctx, `
        SELECT id, org_id, topic, key, payload, attempts
        FROM outbox_records
        WHERE status = 'pending'
          AND ($1::int = 1 OR MOD(hashtext(id::text), $1::int) = $2::int)
        ORDER BY created_at
        LIMIT 100
        FOR UPDATE SKIP LOCKED
    `, relayTotalInstances, relayInstanceIndex)
    if err != nil {
        return 0, fmt.Errorf("select outbox records: %w", err)
    }
    defer rows.Close()

    // ... scan, publish, update status (see full impl in §7.3 source)

    if err := tx.Commit(ctx); err != nil {
        return 0, fmt.Errorf("commit relay batch: %w", err)
    }
    return published, nil
}
```

---

## 7.4 Hybrid Pull/Push Strategy
The relay combines two wakeup mechanisms:
1. `pg_notify` on channel `outbox_new` — low-latency push on insert
2. Polling ticker (configurable, default 1s) — fallback for missed notifications

---

## 7.5 Business Handler Pattern (Canonical)

```go
// Every service write that produces an event follows this pattern exactly.
// All steps must be in one transaction.
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (*models.User, error) {
    tx, cleanup, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return nil, fmt.Errorf("begin tx: %w", err)
    }
    defer cleanup() // Rollback + Release; no-op if committed

    // Business operation
    user, err := s.repo.CreateUserTx(ctx, tx, input)
    if err != nil {
        return nil, fmt.Errorf("create user: %w", err)
    }

    // Write to outbox IN THE SAME TRANSACTION
    envelope := buildUserCreatedEnvelope(ctx, user)
    if err := s.outboxWriter.Write(ctx, tx, kafka.TopicAuditTrail, user.OrgID, user.OrgID, envelope); err != nil {
        return nil, fmt.Errorf("write outbox: %w", err)
    }

    // For SCIM saga: also write saga event
    sagaEnvelope := buildUserCreatedSagaEnvelope(ctx, user)
    if err := s.outboxWriter.Write(ctx, tx, kafka.TopicSagaOrchestration, user.OrgID, user.OrgID, sagaEnvelope); err != nil {
        return nil, fmt.Errorf("write saga outbox: %w", err)
    }

    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit: %w", err)
    }

    // The relay publishes the outbox records to Kafka asynchronously.
    // There is NO direct Kafka.Publish() call here. Ever.
    return user, nil
}
```
