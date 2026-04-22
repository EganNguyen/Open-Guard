# §6 — Multi-Tenancy & RLS

Every table storing tenant data **must** have RLS enabled with an explicit `org_id UUID NOT NULL` column. The RLS policy always compares against the `org_id` column — never against any Kafka partition key or surrogate.

---

## 6.1 PostgreSQL Row-Level Security

### 6.1.1 Application DB Roles

```sql
-- DDL Role: Used by CI/CD migrations. DDL only, NO BYPASSRLS.
CREATE ROLE openguard_migrate LOGIN PASSWORD 'change-me';
GRANT ALL ON SCHEMA public TO openguard_migrate;

-- DML Role: Used by services. DML only, NO BYPASSRLS.
CREATE ROLE openguard_app LOGIN PASSWORD 'change-me';
GRANT USAGE ON SCHEMA public TO openguard_app;

-- Outbox Role: Has BYPASSRLS on outbox_records only.
CREATE ROLE openguard_outbox LOGIN PASSWORD 'change-me';
GRANT USAGE ON SCHEMA public TO openguard_outbox;
GRANT SELECT, UPDATE, DELETE ON outbox_records TO openguard_outbox;
GRANT BYPASSRLS ON outbox_records TO openguard_outbox;
```

### 6.1.2 RLS Setup (canonical pattern for every org-scoped table)

```sql
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;  -- applies to table owner too

CREATE POLICY <table>_org_isolation ON <table>
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

-- The 'true' flag makes current_setting return NULL instead of error when not set.
-- NULL::UUID != any org_id → no rows match → fail safe (zero rows, not error).
```

Apply to: `users`, `api_tokens`, `sessions`, `mfa_configs`, `policies`, `policy_assignments`, `outbox_records`, `dlp_policies`, `dlp_findings`, and any future tenant table.

### 6.1.3 Enforced RLS Wrapper

```go
// shared/rls/context.go
package rls

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type contextKey struct{}

func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}

func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    if orgID == "" {
        _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
    if orgID == "" {
        _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

// OrgPool wraps pgxpool.Pool and automatically sets the RLS session variable
// on every acquired connection. You cannot get a connection without RLS being set.
type OrgPool struct {
    pool *pgxpool.Pool
}

func NewOrgPool(pool *pgxpool.Pool) *OrgPool {
    return &OrgPool{pool: pool}
}

func (p *OrgPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
    orgID := OrgID(ctx)
    conn, err := p.pool.Acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquire connection: %w", err)
    }
    if err := SetSessionVar(ctx, conn, orgID); err != nil {
        conn.Release()
        return nil, fmt.Errorf("set rls session var: %w", err)
    }
    return conn, nil
}

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
        _ = tx.Rollback(ctx)
        conn.Release()
    }
    return tx, cleanup, nil
}

func (p *OrgPool) WithConn(ctx context.Context, fn func(conn *pgxpool.Conn) error) error {
    conn, err := p.Acquire(ctx)
    if err != nil {
        return err
    }
    defer conn.Release()
    return fn(conn)
}
```

Every repository in every service uses `*rls.OrgPool`, not `*pgxpool.Pool` directly.

### 6.1.4 Outbox Table RLS

```sql
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,     -- RLS is enforced on this column
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,     -- Kafka partition key; may differ from org_id
    payload      BYTEA NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);
```

The Outbox relay uses the `openguard_outbox` database role with `BYPASSRLS` on `outbox_records` only.

### 6.1.5 Outbox Cleanup Job

Background worker runs every 10 minutes:

```sql
DELETE FROM outbox_records
WHERE status = 'published'
  AND published_at < NOW() - INTERVAL '24 hours';
```

> **WARNING:** The cleanup job MUST NOT use `FOR UPDATE`. The relay's `processBatch` uses `FOR UPDATE SKIP LOCKED`. Using `FOR UPDATE` in the cleanup would cause deadlocks. The cleanup must simply `DELETE`.

Add a partial index for cleanup efficiency:
```sql
CREATE INDEX idx_outbox_published ON outbox_records(published_at)
    WHERE status = 'published';
```

### 6.1.6 API Key Middleware (Connector Auth)

Hot-path uses the two-tier Prefix/Secret scheme (§2.6):

```go
// shared/middleware/apikey.go
func APIKeyMiddleware(cache ConnectorCache, repo ConnectorReader) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if len(raw) < 16 {
                writeError(w, http.StatusUnauthorized, "INVALID_KEY", "invalid API key format", r)
                return
            }
            prefix := raw[0:8]
            secret := raw[8:]
            fastHash := sha256Hash(prefix)

            // 1. Redis lookup (O(microseconds))
            app, ok := cache.Get(r.Context(), fastHash)
            if ok {
                if app.Status != "active" {
                    writeError(w, http.StatusUnauthorized, "CONNECTOR_SUSPENDED", "connector is suspended", r)
                    return
                }
                if time.Since(app.LastVerifiedAt) > 5*time.Minute {
                    if !verifyPBKDF2(secret, app.KeyHash) {
                        writeError(w, http.StatusUnauthorized, "INVALID_KEY", "invalid secret", r)
                        return
                    }
                }
                ctx := rls.WithOrgID(r.Context(), app.OrgID)
                ctx = withConnectorID(ctx, app.ID)
                ctx = withConnectorScopes(ctx, app.Scopes)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }

            // 2. Cache miss → DB path (O(400ms))
            fullHash := pbkdf2Hash(raw)
            app, err := repo.GetByHash(r.Context(), fullHash)
            if err != nil {
                writeError(w, http.StatusUnauthorized, "INVALID_KEY", "unrecognized key", r)
                return
            }
            cache.Set(r.Context(), fastHash, app, 30*time.Second)

            if app.Status != "active" {
                writeError(w, http.StatusUnauthorized, "CONNECTOR_SUSPENDED", "connector is suspended", r)
                return
            }

            ctx := rls.WithOrgID(r.Context(), app.OrgID)
            ctx = withConnectorID(ctx, app.ID)
            ctx = withConnectorScopes(ctx, app.Scopes)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## 6.2 Per-Tenant Quotas

Three rate limit tiers using Redis sliding window (token bucket, 1-minute window):

```go
// shared/middleware/ratelimit.go
// Key schema:
//   Connector-level: "rl:connector:{connector_id}:{window_minute}"
//   Tenant-level:    "rl:org:{org_id}:{window_minute}"
//   SCIM-level:      "rl:scim:{org_id}:{window_minute}"
//
// Redis failure mode: FAIL OPEN with an in-process local Token Bucket backstop.
// On limit exceeded: return 429 with:
//   Retry-After: <seconds to next window>
//   X-RateLimit-Limit: <limit>
//   X-RateLimit-Remaining: 0
```

---

## 6.3 ClickHouse Multi-Tenancy Wrapper

Every ClickHouse query MUST go through `OrgClickHouseConn`:

```go
// shared/clickhouse/org_conn.go
package clickhouse

import (
    "context"
    "errors"
    "strings"
    "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type OrgClickHouseConn struct {
    conn  driver.Conn
    orgID string
}

func NewOrgConn(conn driver.Conn, orgID string) *OrgClickHouseConn {
    if conn == nil {
        panic("NewOrgConn: conn is required")
    }
    if orgID == "" {
        panic("NewOrgConn: orgID is required — use raw conn for system queries")
    }
    return &OrgClickHouseConn{conn: conn, orgID: orgID}
}

func (c *OrgClickHouseConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
    if !strings.Contains(strings.ToLower(query), "org_id") {
        return nil, errors.New("ClickHouse query missing org_id filter — potential cross-tenant leak")
    }
    return c.conn.Query(ctx, query, append([]any{c.orgID}, args...)...)
}

func (c *OrgClickHouseConn) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
    if !strings.Contains(strings.ToLower(query), "org_id") {
        panic("ClickHouse QueryRow missing org_id filter — potential cross-tenant leak")
    }
    return c.conn.QueryRow(ctx, query, append([]any{c.orgID}, args...)...)
}

// Raw returns the underlying driver.Conn for non-tenant queries (e.g., schema migrations).
// Callers using Raw() must add a comment explaining why org_id isolation is not needed.
func (c *OrgClickHouseConn) Raw() driver.Conn { return c.conn }
```

**Usage pattern:**
```go
orgConn := clickhouse.NewOrgConn(conn, orgID)
rows, err := orgConn.Query(ctx, `
    SELECT type, count() AS cnt
    FROM events FINAL
    WHERE org_id = ?
      AND occurred_at BETWEEN ? AND ?
    GROUP BY type
`, startDate, endDate)
```

> **Limitation:** The string-match guard (`strings.Contains`) does not parse SQL. It catches plain omissions but not a query that references `org_id` only in a comment. Code review and SQL lint are the complementary layers.
