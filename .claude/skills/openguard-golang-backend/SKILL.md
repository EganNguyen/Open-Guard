---
name: openguard-golang-backend
description: >
  Use this skill whenever writing, reviewing, or extending any Golang backend
  service in the OpenGuard project. Covers all mandatory patterns: Transactional
  Outbox, RLS-enforced multi-tenancy, Circuit Breakers, Kafka manual-commit
  consumers, bcrypt worker pool, JWT keyring, SCIM v2, mTLS, CQRS, and
  choreography-based sagas. All rules below are CI-enforced — violation = PR blocked.
license: Internal — OpenGuard Engineering
---

# OpenGuard — Golang Backend Skill

> Read §0–§4 of the BE spec before writing any service code.
> Every code pattern here is canonical and CI-enforced.

---

## 0. Absolute Rules (CI-enforced, no exceptions)

```
✗ No direct Kafka producer calls from business handlers — use Outbox relay only
✗ No string concatenation in SQL — parameterized queries ($1, $2) always
✗ No time.Sleep anywhere in service code — use time.NewTicker inside select{}
✗ No interfaces defined in shared/ — define interfaces in the consuming package
✗ No raw goroutines for bcrypt — run inside bounded AuthWorkerPool
✗ No cross-service pkg/ imports — services never import each other's pkg/
✗ No shared/utils or shared/helpers packages — every package must have a real name
✗ No mutable package-level variables (except pre-compiled regexp and sentinel errors)
✗ No Kafka offset commit before successful downstream write
✗ No org_id derived from client-supplied headers in SCIM endpoints — from token only
✗ No _ = err — every error must be handled or explicitly logged
```

---

## 1. Package Design

### 1.1 Layout

Every service follows this internal layout:

```
services/<name>/
├── cmd/server/main.go          # wire everything, start HTTP + background workers
├── pkg/
│   ├── repository/             # DB access (type: Repository)
│   ├── service/                # business logic (type: Service)
│   ├── handlers/               # HTTP handlers (type: Handler)
│   ├── outbox/                 # outbox writer (type: Writer)  — if service publishes events
│   └── router/                 # chi/mux setup (type: Router)
└── migrations/
    ├── 001_create_foo.up.sql
    └── 001_create_foo.down.sql
```

`shared/` module (`github.com/openguard/shared`) holds only wire contracts:

```
shared/
├── kafka/          # EventEnvelope, topic constants, outbox writer/relay
├── models/         # canonical domain types, sentinel errors
├── rls/            # WithOrgID, OrgID, SetSessionVar, TxSetSessionVar
├── resilience/     # NewBreaker, Call[T], AuthWorkerPool
├── telemetry/      # structured logger setup, OTel tracer init
├── crypto/         # PBKDF2, fast-hash, TOTP helpers
└── validator/      # shared Zod-equivalent Go validators
```

### 1.2 Naming

| Package       | Canonical type name |
|---------------|---------------------|
| `repository/` | `Repository`        |
| `service/`    | `Service`           |
| `handlers/`   | `Handler`           |
| `outbox/`     | `Writer`            |
| `router/`     | `Router`            |

```go
// ✓ correct — caller writes repository.Repository
type Repository struct{}

// ✗ wrong — repeats package name
type UserRepository struct{}
```

Acceptable abbreviations **only**: `ctx`, `cfg`, `err`, `ok`, `id`, `tx`, `db`, `w`, `r`.

Sentinel errors use `Err` prefix:

```go
var (
    ErrNotFound      = errors.New("not found")
    ErrAlreadyExists = errors.New("already exists")
    ErrCircuitOpen   = errors.New("circuit breaker open")
    ErrRLSNotSet     = errors.New("RLS org_id context not set")
)
```

---

## 2. Error Handling

Wrap **once** at each layer boundary. Never discard errors silently.

```go
// Repository layer — map DB errors to sentinel errors
func (r *Repository) GetByID(ctx context.Context, id string) (*models.User, error) {
    var u models.User
    err := r.db.QueryRow(ctx, query, id).Scan(/* fields */)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound          // sentinel, not raw pgx error
        }
        return nil, fmt.Errorf("query user by id %s: %w", id, err)
    }
    return &u, nil
}

// Service layer — wrap with context
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
    u, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("get user: %w", err)
    }
    return u, nil
}

// Handler layer — map to HTTP
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    u, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "id"))
    if errors.Is(err, repository.ErrNotFound) {
        writeError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "user not found", r)
        return
    }
    if err != nil {
        writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", r)
        return
    }
    writeJSON(w, http.StatusOK, u)
}
```

Never close resources silently:

```go
// ✗ wrong
_ = db.Close()

// ✓ correct
if err := db.Close(); err != nil {
    slog.ErrorContext(ctx, "failed to close db", "error", err)
}
```

---

## 3. PostgreSQL & RLS

### 3.1 Every org-scoped table requires RLS

```sql
-- Required on every table with org_id column
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;   -- applies to table owner too

CREATE POLICY <table>_org_isolation ON <table>
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

-- The 'true' flag → NULL (not error) when setting is absent
-- NULL::UUID != any org_id → zero rows → fail safe
```

### 3.2 DB roles

| Role                 | Grants                                     |
|----------------------|--------------------------------------------|
| `openguard_migrate`  | DDL only. No BYPASSRLS. Run at CI/deploy.  |
| `openguard_app`      | DML only (SELECT/INSERT/UPDATE/DELETE). No BYPASSRLS. |
| `openguard_outbox`   | SELECT/UPDATE/DELETE on `outbox_records` + BYPASSRLS on that table only. |

### 3.3 RLS context wrapper

```go
// shared/rls/context.go

func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}

// Call before every transaction — sets the session variable PostgreSQL RLS reads
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
    _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}
```

### 3.4 Connection pool targets

| Service          | Min | Max | Lifetime |
|-----------------|-----|-----|----------|
| IAM             | 5   | 25  | 1800s    |
| Outbox Relay    | 2   | 10  | **60s** (fast failover recovery) |
| Control Plane   | 2   | 15  | 300s     |
| All services    | 5   | 20  | —  (Redis) |

### 3.5 Migrations

- Every `.up.sql` has a `.down.sql`
- Additive only in production: add nullable columns, add indexes, add tables — never drop or rename in same migration
- Every migration creating a table with `org_id` must include RLS setup inline
- Migrations run at service startup behind a **distributed Redis lock** (SET NX + heartbeat goroutine extending TTL every 10s)
- No DML in migrations — data backfills run separately

---

## 4. Transactional Outbox Pattern

**Rule**: Every Kafka publish goes through the outbox. No direct producer calls.

### 4.1 Outbox table (every service that publishes events)

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

-- NOTIFY trigger for immediate relay wake-up (avoids polling delay)
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

### 4.2 Business handler pattern

```go
// ✓ correct — atomic: business row + outbox record in same transaction
func (s *Service) CreateUser(ctx context.Context, u *models.User) error {
    return s.pool.BeginTxFunc(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
        // 1. Set RLS session variable
        if err := rls.TxSetSessionVar(ctx, tx, rls.OrgID(ctx)); err != nil {
            return fmt.Errorf("set rls: %w", err)
        }
        // 2. Business insert
        if err := s.repo.InsertUserTx(ctx, tx, u); err != nil {
            return fmt.Errorf("insert user: %w", err)
        }
        // 3. Outbox record (same transaction)
        evt := kafka.EventEnvelope{
            ID:          uuid.NewString(),
            Type:        "user.created",
            OrgID:       u.OrgID,
            ActorID:     u.ID,
            ActorType:   "user",
            OccurredAt:  time.Now(),
            Source:      "iam",
            EventSource: "internal",
            SchemaVer:   "1.0",
            Payload:     mustMarshal(u),
        }
        return s.outboxWriter.WriteTx(ctx, tx, models.TopicUserEvents, u.ID, evt)
    })
}

// ✗ wrong — race condition between DB write and Kafka publish
func (s *Service) CreateUserBad(ctx context.Context, u *models.User) error {
    s.repo.Insert(ctx, u)
    s.kafkaProducer.Publish("user.events", u) // crash here = permanent audit gap
    return nil
}
```

### 4.3 Relay query (SELECT FOR UPDATE SKIP LOCKED)

```go
const relayQuery = `
    SELECT id, topic, key, payload
    FROM outbox_records
    WHERE status = 'pending'
    ORDER BY created_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
`
```

---

## 5. Kafka — Event Bus

### 5.1 Kafka Envelope (canonical schema)

> The canonical EventEnvelope is defined in be_open_guard/03-shared-contracts.md §4.1. Do not redefine it here.

### 5.2 Manual offset commit — non-negotiable

```go
// ✓ correct — commit only after successful downstream write
for {
    msg, err := consumer.ReadMessage(ctx)
    if err != nil { /* handle */ continue }

    if err := s.processMessage(ctx, msg); err != nil {
        // do NOT commit — message will be reprocessed on restart
        log.ErrorContext(ctx, "failed to process message", "error", err)
        continue
    }

    // commit only on success
    if err := consumer.CommitMessage(ctx, msg); err != nil {
        log.ErrorContext(ctx, "failed to commit offset", "error", err)
    }
}

// ✗ wrong — commit before processing
consumer.CommitMessage(ctx, msg)
s.processMessage(ctx, msg)  // crash here = permanently lost message
```

### 5.3 Topic registry (canonical names — rename = major version bump)

> The canonical Kafka Topic Registry is defined in be_open_guard/03-shared-contracts.md §4.4. Do not redefine it here.

### 5.4 Consumer group naming

```
openguard-iam-v1
openguard-audit-v1
openguard-policy-v1
openguard-threat-v1
openguard-saga-v1
```

---

## 6. Resilience Patterns

### 6.1 Circuit breaker (wrap every inter-service HTTP call)

```go
// shared/resilience/breaker.go
type BreakerConfig struct {
    Name             string
    RequestTimeout   time.Duration
    MaxRequests      uint32        // requests allowed in half-open state
    Interval         time.Duration // stat collection window
    FailureThreshold uint32        // consecutive failures before opening
    OpenDuration     time.Duration // time before moving to half-open
}

func NewBreaker(cfg BreakerConfig, logger *slog.Logger) *gobreaker.CircuitBreaker {
    return gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        cfg.Name,
        MaxRequests: cfg.MaxRequests,
        Interval:    cfg.Interval,
        Timeout:     cfg.OpenDuration,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= cfg.FailureThreshold
        },
        OnStateChange: func(name string, from, to gobreaker.State) {
            logger.Warn("circuit breaker state changed",
                "name", name, "from", from.String(), "to", to.String())
            // emit metric: openguard_circuit_breaker_state{name, state}
        },
    })
}

// Call wraps fn with timeout + circuit breaker
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker,
    timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {

    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    result, err := cb.Execute(func() (any, error) { return fn(ctx) })
    if err != nil {
        var zero T
        if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
            return zero, fmt.Errorf("%w: %s", models.ErrCircuitOpen, cb.Name())
        }
        return zero, err
    }
    if result == nil {
        var zero T
        return zero, nil
    }
    return result.(T), nil
}
```

### 6.2 Failure mode table

| Service unavailable       | Behavior                                      |
|--------------------------|-----------------------------------------------|
| Policy engine            | Deny after 60s SDK cache TTL (fail closed)    |
| IAM                      | Reject all logins                             |
| DLP sync-block           | Reject events (per-org opt-in)                |
| Outbox relay             | Events queue in DB; relay resumes on restart  |
| Webhook delivery         | Retry up to WEBHOOK_MAX_ATTEMPTS → DLQ        |

### 6.3 bcrypt worker pool (never raw goroutines)

```go
// services/iam/pkg/service/auth.go
type AuthWorkerPool struct {
    jobs    chan bcryptJob
    workers int
}

type bcryptJob struct {
    password string
    hash     string
    result   chan error
}

func NewAuthWorkerPool(workers int) *AuthWorkerPool {
    p := &AuthWorkerPool{
        jobs:    make(chan bcryptJob, 100),
        workers: workers,        // use 2 * runtime.NumCPU()
    }
    for i := 0; i < workers; i++ {
        go p.worker()
    }
    return p
}

func (p *AuthWorkerPool) worker() {
    for job := range p.jobs {
        job.result <- bcrypt.CompareHashAndPassword([]byte(job.hash), []byte(job.password))
    }
}

func (p *AuthWorkerPool) Compare(ctx context.Context, password, hash string) error {
    result := make(chan error, 1)
    select {
    case p.jobs <- bcryptJob{password, hash, result}:
    case <-ctx.Done():
        return ctx.Err()
    }
    select {
    case err := <-result:
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**bcrypt capacity model**: at cost 12, ~350ms per operation. Pool of `2N` workers on N-core node → ~5.7N logins/s. Meeting 2,000 req/s SLO requires ~35 CPU cores (e.g. 6 pods × 6 cores).

---

## 7. Authentication & Cryptography

### 7.1 JWT keyring (zero-downtime rotation)

- All JWTs carry a `kid` claim
- Multiple valid keys coexist during rotation — verify with any matching key
- Validation order: (1) verify signature, (2) check `exp` → reject `ErrTokenExpired`, (3) check Redis jti blocklist
- Blocklist TTL = `exp - now()` (dynamic, not fixed)
- `iat` must not be more than 5 minutes in the future

### 7.2 Connector API key scheme

```
key = prefix (8 chars, base62) + secret (remaining chars)

Auth flow:
  1. prefix  = key[0:8]
  2. fastHash = SHA-256(prefix)
  3. Redis GET "connector:fasthash:{fastHash}"
     → hit:  deserialize ConnectedApp; verify secret vs stored PBKDF2 hash
             (skip PBKDF2 if last_verified_at < 5min ago → trust cache)
     → miss: PBKDF2-HMAC-SHA512(key, salt, 600000) → DB lookup
             → on hit: SET Redis with 30s TTL
  4. Check status == "active"
  5. rls.WithOrgID(ctx, connector.OrgID)
```

- Cache invalidation on suspension: SET to "suspended" sentinel (do not delete)
- Connector reactivation: overwrite Redis key with full record immediately (do not just delete)

### 7.3 SCIM authentication

```go
// shared/middleware/scim.go
// org_id is ALWAYS derived from the token map — never from client-supplied headers
func SCIMAuthMiddleware(tokens []SCIMToken) func(http.Handler) http.Handler {
    tokenMap := make(map[string]string, len(tokens))
    for _, t := range tokens {
        tokenMap[t.Token] = t.OrgID
    }
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            orgID, ok := tokenMap[raw]
            if !ok {
                writeError(w, http.StatusUnauthorized, "INVALID_SCIM_TOKEN", "invalid SCIM bearer token", r)
                return
            }
            ctx := rls.WithOrgID(r.Context(), orgID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### 7.4 Session risk signals → immediate revocation

On `SESSION_REVOKED_RISK` or `SESSION_COMPROMISED`:

```go
// Fetch all active jti values for user_id
// PIPELINE: SETEX jti:{jti} <remaining_ttl> "revoked" for each jti
// UPDATE sessions SET status='revoked' WHERE user_id = $1
// UPDATE users SET status='deprovisioned'  (for user.deleted saga)
```

---

## 8. Choreography-Based Sagas

### 8.1 SCIM user provisioning saga

```
IAM:        user.created (status=initializing)   → audit.trail + saga.orchestration
Policy:     [consumes user.created]              → assigns default org policies
            policy.assigned                      → audit.trail
Threat:     [consumes policy.assigned]           → initializes baseline profile
            threat.baseline.init                 → audit.trail
Alerting:   [consumes threat.baseline.init]      → configures notification preferences
            alert.prefs.init                     → audit.trail
IAM:        [consumes alert.prefs.init]          → UPDATE users SET status = 'active'
            saga.completed                       → audit.trail
```

**Compensation** (any step fails):

```
Policy:     policy.assignment.failed (compensation:true)
IAM:        [consumes] → sets user status=provisioning_failed
Threat:     [consumes user.provisioning.failed] → removes baseline profile
Alerting:   [consumes user.provisioning.failed] → removes notification preferences
```

### 8.2 User status transitions

| From               | To                   | Trigger                               |
|--------------------|----------------------|---------------------------------------|
| (new)              | `initializing`       | SCIM `POST /Users`                   |
| `initializing`     | `active`             | `saga.completed` consumed by IAM     |
| `initializing`     | `provisioning_failed`| `saga.timed_out` or compensation     |
| `provisioning_failed` | `initializing`    | Admin `POST /users/:id/reprovision`  |
| `active`           | `suspended`          | Admin `POST /users/:id/suspend`      |
| `suspended`        | `active`             | Admin `POST /users/:id/activate`     |
| any                | `deprovisioned`      | SCIM `DELETE /Users/:id`             |

IAM **must reject logins** for users in `initializing` status.

### 8.3 Saga timeout

```go
// deadline = SAGA_STEP_TIMEOUT_SECONDS (30s) + OUTBOX_MAX_LAG_SECONDS (10s) = 40s
// ZADD saga:deadlines <unix_deadline> <saga_id>
// Background watcher in consumer group openguard-saga-v1 polls every 10s
```

---

## 9. CQRS — Audit Log

### 9.1 Write path (Kafka consumer → MongoDB primary)

- Bulk insert up to 500 documents or 1-second flush interval, whichever comes first
- Commit Kafka offset **after** successful `BulkWrite()`
- Chain sequence assigned via batched atomic reservation (not per-document)
- Per-org HMAC hash chaining

### 9.2 Read path (HTTP handlers → MongoDB)

- `readPreference: secondaryPreferred` for all paginated queries
- `readPreference: secondary` for compliance report queries (5s acceptable staleness)
- **Exception**: `GET /audit/integrity` → `readPreference: primary` always

### 9.3 Idempotency

`event_id` unique index on MongoDB `audit_events` scoped to retention window. Enables safe Kafka message reprocessing after consumer restart.

---

## 10. Observability

### 10.1 Structured logging (log/slog)

```go
slog.InfoContext(ctx, "user login succeeded",
    "service",      "iam",
    "trace_id",     traceID(ctx),
    "span_id",      spanID(ctx),
    "org_id",       rls.OrgID(ctx),
    "user_id",      userID,
    "request_id",   requestID(r),
    "duration_ms",  time.Since(start).Milliseconds(),
)
```

Safe logger strips: tokens, passwords, api_key, secret, authorization header values.

### 10.2 Prometheus metrics (required per service)

```
openguard_http_request_duration_seconds{service, method, path, status}
openguard_kafka_consumer_lag{service, topic, partition}
openguard_outbox_depth{service}
openguard_circuit_breaker_state{name, state}   // 0=closed, 1=half-open, 2=open
openguard_bcrypt_pool_queue_depth{service}
```

### 10.3 Health endpoints (required on every service)

```
GET /healthz   → liveness  (always 200 if process is running)
GET /readyz    → readiness (checks DB connection, Redis ping, Kafka connectivity)
```

---

## 11. Goroutines & Concurrency

```go
// ✓ polling loop — ALWAYS use NewTicker + select
func (r *Relay) run(ctx context.Context) {
    ticker := time.NewTicker(r.interval)
    defer ticker.Stop()

    notify := r.listenOutboxNotify(ctx)   // pg_notify channel

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            r.drain(ctx)
        case <-notify:
            r.drain(ctx)   // immediate wake-up on new outbox record
        }
    }
}

// ✗ forbidden
time.Sleep(5 * time.Second)
```

Graceful shutdown:

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit

shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// drain in-flight HTTP requests
server.Shutdown(shutdownCtx)
// close Kafka consumer, DB pool, Redis pool
```

---

## 12. HTTP Standards

```go
// All error responses follow this shape
type ErrorResponse struct {
    Error struct {
        Code      string `json:"code"`        // e.g. "RESOURCE_NOT_FOUND"
        Message   string `json:"message"`
        RequestID string `json:"request_id"`
        TraceID   string `json:"trace_id"`
        Retryable bool   `json:"retryable"`
        Fields    []struct {
            Field   string `json:"field"`
            Message string `json:"message"`
        } `json:"fields,omitempty"`  // VALIDATION_ERROR only
    } `json:"error"`
}

// Idempotency-Key header required on all non-idempotent mutations
// API versioning: all routes under /v1/
// Breaking change: implement /v2/ alongside /v1/
//   Add Deprecation: true and Sunset: <date> headers to /v1/ responses
//   Maintain /v1/ for minimum 6 months after /v2/ GA
```

Security headers (every service):

```
Strict-Transport-Security: max-age=63072000; includeSubDomains
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'none'
```

---

## 13. Canonical SLOs (Phase 8 k6 verification required)

| Operation                                 | p99     | Throughput      |
|-------------------------------------------|---------|-----------------|
| `POST /v1/policy/evaluate` (uncached)     | 30ms    | 10,000 req/s    |
| `POST /v1/policy/evaluate` (Redis cached) | 5ms     | 10,000 req/s    |
| `POST /oauth/token`                       | 150ms   | 2,000 req/s     |
| `POST /v1/events/ingest`                  | 50ms    | 20,000 req/s    |
| Kafka event → audit DB insert             | 2s      | 50,000 events/s |
| `GET /audit/events`                       | 100ms   | 1,000 req/s     |
| Compliance report generation              | 30s     | 10 concurrent   |
| Connector registry lookup (Redis cached)  | 5ms     | —               |
| DLP async scan latency                    | 500ms   | —               |

A phase is **not complete** until its SLOs are met under k6 load.

---

## 14. Quick Reference — Forbidden Patterns

| Pattern                                    | Required alternative                              |
|--------------------------------------------|---------------------------------------------------|
| `kafka.Publish()` in business handler      | Transactional Outbox via `outbox.Writer`          |
| `fmt.Sprintf("... WHERE id='%s'", id)`     | Parameterized query `$1`                          |
| `time.Sleep(n)`                            | `time.NewTicker` inside `select{}`                |
| `go bcrypt.Compare(...)`                   | `authPool.Compare(ctx, ...)`                      |
| Interface in `shared/`                     | Interface in consuming package                    |
| `shared/utils` or `shared/helpers`         | Named package (e.g., `shared/crypto`)             |
| `_ = someErr`                              | Log or return the error                           |
| Direct cross-service pkg import            | HTTP call wrapped in circuit breaker              |
| `current_setting('app.org_id')` w/o `true`| `NULLIF(current_setting('app.org_id', true), '')`|
| `org_id` from SCIM client header           | Derive from SCIM bearer token map only            |
