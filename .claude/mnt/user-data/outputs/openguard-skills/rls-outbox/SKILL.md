---
name: openguard-rls-outbox
description: >
  Use this skill for any task involving PostgreSQL Row-Level Security (RLS),
  the Transactional Outbox pattern, or multi-tenancy isolation in OpenGuard.
  Triggers include: "add RLS to a table", "create outbox record", "write a migration
  with tenant isolation", "atomic write with event", "business handler that publishes",
  "set org_id context", "outbox relay", "dual-write problem", or any SQL touching
  org-scoped tables. This skill is the authoritative reference for the most
  security-critical patterns in the codebase — RLS bugs are data breaches.
---

# OpenGuard RLS & Outbox Skill

These two patterns are the foundation of OpenGuard's security and audit guarantees.
RLS ensures zero cross-tenant data leakage at the database layer. The Outbox ensures
zero audit trail gaps. Both must be implemented exactly as specified — there are no
acceptable approximations.

---

## Part 1: Row-Level Security (RLS)

### 1.1 The Core Rule

**Every table storing org-scoped data MUST have:**
1. An explicit `org_id UUID NOT NULL` column
2. `ALTER TABLE <t> ENABLE ROW LEVEL SECURITY`
3. `ALTER TABLE <t> FORCE ROW LEVEL SECURITY` (applies to table owner too)
4. A policy using `NULLIF(current_setting('app.org_id', true), '')::UUID`

The `NULLIF` wrapper is mandatory. Without it, an empty string `org_id` in context
raises a cast error rather than silently returning zero rows, which can bypass RLS
depending on the PostgreSQL exception handler. This is a real security vulnerability.

### 1.2 Canonical RLS Migration Pattern

Apply this to every new org-scoped table. Never deviate from this exact form:

```sql
-- Step 1: Table with explicit org_id (never nullable)
CREATE TABLE <table> (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id   UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    -- ... other columns
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Step 2: Indexes (include WHERE for soft-delete tables)
CREATE INDEX idx_<table>_org_id ON <table>(org_id);

-- Step 3: Enable RLS (both statements required)
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;

-- Step 4: Policy — NULLIF is MANDATORY to handle empty string safely
CREATE POLICY <table>_org_isolation ON <table>
    USING (
        org_id = NULLIF(current_setting('app.org_id', true), '')::UUID
    )
    WITH CHECK (
        org_id = NULLIF(current_setting('app.org_id', true), '')::UUID
    );

-- Step 5: Grants (never superuser, never BYPASSRLS for app role)
GRANT SELECT, INSERT, UPDATE ON <table> TO openguard_app;
```

### 1.3 The Three DB Roles and What They Can Do

| Role | Purpose | BYPASSRLS? | DDL? |
|---|---|---|---|
| `openguard_migrate` | CI/CD migrations only | NO | YES |
| `openguard_app` | All service queries | NO | NO |
| `openguard_outbox` | Outbox relay only | YES (outbox table only) | NO |

The `openguard_app` role never has `BYPASSRLS`. The `openguard_migrate` role
never has `BYPASSRLS`. This is enforced at the DB level — no application code
workaround can bypass it.

### 1.4 OrgPool — Never Use Raw pgxpool.Pool

The `rls.OrgPool` wrapper sets `app.org_id` on every acquired connection automatically.
Using a raw `*pgxpool.Pool` in any service or repository is forbidden.

```go
// shared/rls/context.go

// WithOrgID stores the org_id in the request context.
// Called by auth middleware after verifying JWT or connector API key.
func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}

// SetSessionVar sets app.org_id on a pooled connection.
// Always use parameterized query — never interpolate orgID into SQL.
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    if orgID == "" {
        _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

// TxSetSessionVar sets RLS INSIDE an existing transaction.
// Use this instead of SetSessionVar when you're already in a transaction
// to avoid acquiring a second connection.
func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
    if orgID == "" {
        _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

// OrgPool wraps pgxpool.Pool and enforces RLS on every connection acquisition.
// Developers cannot get a connection without RLS being set — by construction.
type OrgPool struct{ pool *pgxpool.Pool }

func NewOrgPool(pool *pgxpool.Pool) *OrgPool { return &OrgPool{pool: pool} }

func (p *OrgPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
    conn, err := p.pool.Acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquire connection: %w", err)
    }
    if err := SetSessionVar(ctx, conn, OrgID(ctx)); err != nil {
        conn.Release()
        return nil, fmt.Errorf("set rls session var: %w", err)
    }
    return conn, nil
}

// BeginTx acquires a connection and begins a transaction with RLS set inside it.
// Returns a cleanup func that rolls back (no-op if committed) and releases the conn.
func (p *OrgPool) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, func(), error) {
    conn, err := p.pool.Acquire(ctx)
    if err != nil {
        return nil, nil, fmt.Errorf("acquire connection: %w", err)
    }
    tx, err := conn.BeginTx(ctx, opts)
    if err != nil {
        conn.Release()
        return nil, nil, fmt.Errorf("begin tx: %w", err)
    }
    if err := TxSetSessionVar(ctx, tx, OrgID(ctx)); err != nil {
        _ = tx.Rollback(ctx)
        conn.Release()
        return nil, nil, fmt.Errorf("set rls in tx: %w", err)
    }
    cleanup := func() {
        _ = tx.Rollback(ctx) // no-op if already committed
        conn.Release()
    }
    return tx, cleanup, nil
}
```

### 1.5 Middleware Chain (How org_id Gets into Context)

```
HTTP Request
  → SecurityHeaders middleware
  → RequestID middleware
  → JWT auth middleware    ← extracts org_id from verified JWT claim
  → RLS middleware         ← calls rls.WithOrgID(ctx, orgID)
  → Handler
     → svc.Method(ctx)    ← ctx carries org_id
        → pool.BeginTx(ctx)  ← OrgPool reads OrgID(ctx), calls TxSetSessionVar
           → SQL queries      ← app.org_id is set; RLS enforced by PostgreSQL
```

For connector API key auth: the `APIKeyMiddleware` calls `rls.WithOrgID` after
validating the connector's key against Redis/DB. The org_id comes from the connector
registry record, never from a client-supplied header.

### 1.6 Outbox Table RLS — Critical Difference

The outbox table has RLS on `org_id` (not on `key`). The Kafka partition key (`key`
column) is a routing concern and may differ from `org_id`. Never use `key` in RLS
policies:

```sql
-- WRONG — couples RLS to Kafka routing
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (key = current_setting('app.org_id', true)::UUID);  -- BUG

-- CORRECT — RLS always on org_id
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

-- The relay role bypasses RLS so it can read ALL tenants' pending records
GRANT BYPASSRLS ON TABLE outbox_records TO openguard_outbox;
```

---

## Part 2: Transactional Outbox

### 2.1 The Rule That Cannot Be Broken

**Every state-changing operation that must produce an audit event writes BOTH the
business row AND the outbox record in the same PostgreSQL transaction.**

There is NO acceptable reason to call Kafka's producer directly from a business
handler. The only path to Kafka is: PostgreSQL outbox → relay → Kafka.

```go
// WHY: If the process crashes between these two calls, the audit event is permanently lost
db.Exec("INSERT INTO users ...")     // succeeds
kafka.Publish("audit.trail", event)  // process crashes here — PERMANENT AUDIT GAP
```

### 2.2 Canonical Business Handler Pattern

This is the only acceptable pattern for writes that produce events. Copy it exactly:

```go
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (*models.User, error) {
    ctx, span := tracer.Start(ctx, "Service.CreateUser")
    defer span.End()

    // Step 1: Begin transaction (OrgPool sets RLS automatically)
    tx, cleanup, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return nil, fmt.Errorf("begin tx: %w", err)
    }
    defer cleanup() // Rollback + Release; no-op after Commit

    // Step 2: Business operation inside the transaction
    user, err := s.repo.CreateUserTx(ctx, tx, input)
    if err != nil {
        return nil, fmt.Errorf("create user: %w", err)
    }

    // Step 3: Write outbox record IN THE SAME TRANSACTION
    envelope := buildUserCreatedEnvelope(ctx, user)
    if err := s.outbox.Write(ctx, tx, kafka.TopicAuditTrail, user.OrgID, user.OrgID, envelope); err != nil {
        return nil, fmt.Errorf("write outbox: %w", err)
    }

    // Step 4: For saga participants, write saga event too (same transaction)
    sagaEnvelope := buildUserCreatedSagaEnvelope(ctx, user)
    if err := s.outbox.Write(ctx, tx, kafka.TopicSagaOrchestration, user.OrgID, user.OrgID, sagaEnvelope); err != nil {
        return nil, fmt.Errorf("write saga outbox: %w", err)
    }

    // Step 5: Commit — both business row and outbox records atomically
    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit: %w", err)
    }

    // Step 6: Return. NO direct Kafka publish here. Ever.
    // The relay publishes asynchronously. The handler does not wait for it.
    return user, nil
}
```

### 2.3 Outbox Writer

```go
// shared/kafka/outbox/writer.go
package outbox

// Writer inserts EventEnvelopes into the outbox within the caller's transaction.
// The transaction must already have the RLS session variable set (done by OrgPool.BeginTx).
// orgID is written explicitly to the org_id column — not derived from the Kafka key.
type Writer struct{}

func (w *Writer) Write(
    ctx      context.Context,
    tx       pgx.Tx,
    topic    string,
    key      string, // Kafka partition key — may differ from orgID
    orgID    string, // Explicit: written to org_id column for RLS
    envelope models.EventEnvelope,
) error {
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

### 2.4 Outbox Table Schema (Complete)

```sql
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,   -- For RLS. NOT the Kafka partition key.
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,   -- Kafka partition key. May differ from org_id.
    payload      BYTEA NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending | published | dead
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

-- Partial index on pending only — keeps index small as records are published
CREATE INDEX idx_outbox_pending ON outbox_records(created_at) WHERE status = 'pending';

ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

-- App role: can INSERT and UPDATE (to set status); cannot read other orgs' records
GRANT SELECT, INSERT, UPDATE ON outbox_records TO openguard_app;
-- Outbox role: BYPASSRLS so relay can read ALL tenants; can DELETE for cleanup job
GRANT SELECT, UPDATE, DELETE ON outbox_records TO openguard_outbox;
GRANT BYPASSRLS ON TABLE outbox_records TO openguard_outbox;

-- NOTIFY for immediate relay wake-up (no need to wait for 100ms poll interval)
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

### 2.5 Outbox Relay Key Behaviors

The relay runs in a separate goroutine (or separate process) as `openguard_outbox`.
These behaviors are non-negotiable:

- **LISTEN/NOTIFY for immediate wake-up** + **100ms polling fallback** (handles missed
  notifications and startup drain)
- **`FOR UPDATE SKIP LOCKED`** — safe to run multiple relay instances concurrently
- **At-most-once DB lock, at-least-once Kafka publish** — Kafka idempotent producer
  (`enable.idempotence=true`) makes this safe
- **Status updated AFTER Kafka ack** — never before; if update fails, record stays
  pending and is republished (idempotent)
- **Dead after 5 failures** — marked `dead`, sent to `TopicOutboxDLQ`
- **Cleanup job** — deletes `status='published'` records older than 24 hours

```go
// Relay poll loop — time.NewTicker, never time.Sleep
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
```

### 2.6 Building an EventEnvelope

```go
// Every event must populate all required fields. TraceID propagates the
// distributed trace from the originating HTTP request through to Kafka.
func buildUserCreatedEnvelope(ctx context.Context, user *models.User) models.EventEnvelope {
    spanCtx := trace.SpanFromContext(ctx).SpanContext()
    return models.EventEnvelope{
        ID:          uuid.New().String(),
        Type:        "user.created",
        OrgID:       user.OrgID,
        ActorID:     rls.OrgID(ctx), // or the authenticated user ID
        ActorType:   "user",
        OccurredAt:  time.Now().UTC(),
        Source:      "iam",
        EventSource: "internal",
        TraceID:     spanCtx.TraceID().String(),
        SpanID:      spanCtx.SpanID().String(),
        SchemaVer:   "1.0",
        Idempotent:  uuid.New().String(), // unique per event; used for Kafka dedup
        Payload:     mustMarshal(user),
    }
}
```

---

## Part 3: Common Mistakes Reference

| Mistake | Correct Pattern |
|---|---|
| RLS policy without `NULLIF` | `NULLIF(current_setting('app.org_id', true), '')::UUID` |
| Direct `kafka.Publish()` in handler | Write to outbox in same transaction |
| Using `key` column for RLS on outbox | Use `org_id` column for RLS |
| `*pgxpool.Pool` in repository | `*rls.OrgPool` always |
| `SET app.org_id` outside transaction | Use `TxSetSessionVar` inside tx; `SetSessionVar` outside |
| Calling `SetSessionVar` + then `BeginTx` (two connections) | Call `OrgPool.BeginTx` which does both on one connection |
| Outbox relay using `openguard_app` role | Must use `openguard_outbox` (BYPASSRLS) |
| Omitting `FORCE ROW LEVEL SECURITY` | Table owner bypasses RLS without it |
| Empty string org_id causing cast error | Always `NULLIF(..., '')` in policy |
