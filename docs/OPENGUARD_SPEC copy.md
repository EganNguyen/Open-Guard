# OpenGuard — Enterprise Security Control Plane Specification v2

> **Document status:** Authoritative. Supersedes all prior versions.
> **Audience:** Implementing engineers, technical reviewers, security architects.
> **How to use:** Read Sections 0–4 in full before writing any code. Every code example satisfies the standards in Section 0. Where a standard has a narrow, named exception, the exception is stated at the point of use.
> **Changelog from v1:** See Appendix B for a full list of changes. Critical fixes include: PBKDF2 hot-path replacement, bcrypt worker pool, hash chain batch assignment, saga timeout, webhook delivery persistence, SCIM org_id spoofing fix, JWT jti revocation, MongoDB replica set init race, RLS NULL/empty-string handling, and ~25 additional corrections.

> **Mandatory rules (enforced by CI and code review):**
> - Every Kafka publish goes through the Outbox relay. No direct producer calls from business handlers.
> - Every table storing org-scoped data has RLS enabled with an explicit `org_id UUID` column.
> - Every inter-service HTTP call wraps a circuit breaker with a defined timeout and fallback.
> - Policy engine failure mode: **fail closed**. Cache grace period: 60s. After expiry: deny.
> - No string concatenation in SQL. Parameterized queries only.
> - No `time.Sleep` in service code. Use `time.NewTicker` inside `select{}` for all polling.
> - Interfaces are defined in the consuming package, never in `shared/`.
> - All canonical names (env vars, topic names, table names, error codes) are fixed. Rename = major version bump.
> - Kafka consumer offsets are committed only after successful downstream write (manual commit mode).
> - The connector registry lookup result is cached in Redis. Every `org_id` derivation hits cache, not DB.
> - **NEW:** PBKDF2 is used for DB storage only. The hot-path API key lookup uses a fast-hash prefix scheme.
> - **NEW:** bcrypt verification runs inside a bounded worker pool. Never in raw goroutines.
> - **NEW:** Webhook delivery state is persisted to PostgreSQL. Not held in memory.
> - **NEW:** SCIM org_id is derived from the bearer token configuration, never from client-supplied headers.

---

## Table of Contents

0. [Code Quality Standards](#0-code-quality-standards)
1. [Project Overview](#1-project-overview)
2. [Architecture Principles](#2-architecture-principles)
3. [Repository Layout](#3-repository-layout)
4. [Shared Contracts](#4-shared-contracts)
5. [Environment & Configuration](#5-environment--configuration)
6. [Multi-Tenancy & RLS](#6-multi-tenancy--rls)
7. [Transactional Outbox Pattern](#7-transactional-outbox-pattern)
8. [Circuit Breakers & Resilience](#8-circuit-breakers--resilience)
9. [Phase 1 — Foundation](#9-phase-1--foundation)
10. [Phase 2 — Policy Engine](#10-phase-2--policy-engine)
11. [Phase 3 — Event Bus & Audit Log](#11-phase-3--event-bus--audit-log)
12. [Phase 4 — Threat Detection & Alerting](#12-phase-4--threat-detection--alerting)
13. [Phase 5 — Compliance & Analytics](#13-phase-5--compliance--analytics)
14. [Phase 6 — Infra, CI/CD & Observability](#14-phase-6--infra-cicd--observability)
15. [Phase 7 — Security Hardening & Secret Rotation](#15-phase-7--security-hardening--secret-rotation)
16. [Phase 8 — Load Testing & Performance Tuning](#16-phase-8--load-testing--performance-tuning)
17. [Phase 9 — Documentation & Runbooks](#17-phase-9--documentation--runbooks)
18. [Phase 10 — Content Scanning & DLP](#18-phase-10--content-scanning--dlp)
19. [Cross-Cutting Concerns](#19-cross-cutting-concerns)
20. [Full-System Acceptance Criteria](#20-full-system-acceptance-criteria)

---

## 0. Code Quality Standards

> These standards are CI-enforced (linters, race detector, coverage gate, SQL lint). Every code example in this specification satisfies them. Named exceptions apply only where explicitly stated and scoped.

### 0.1 Philosophy

**Readability is a production concern.** Code is read ten times for every time it is written. Optimize for the on-call engineer at 3 AM.

**Boring code is good code.** Go is deliberately unexciting. Propose changes to this document; do not silently diverge in code.

**Consistency beats local optimality.** When the team has agreed on a pattern, use it.

### 0.2 Package Design

#### One coherent concept per package

If you cannot describe what a package does in one sentence without "and," it needs to be split.

#### Service layout

Every service uses `services/<name>/pkg/` for all sub-packages. Services never import each other's `pkg/` packages.

#### The `shared/` module

`github.com/openguard/shared` holds genuine cross-service wire contracts. Every package inside it has a real, descriptive name: `kafka/`, `models/`, `rls/`, `resilience/`, `telemetry/`, `crypto/`, `validator/`. Never add `shared/utils/` or `shared/helpers/`.

#### No package-level variables for mutable state

```go
// Bad — implicit global, test-order dependent
var defaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

// Good — explicit, injectable
func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{Timeout: timeout}
}
```

**Named exceptions (exhaustive list):**
- `shared/telemetry/logger.go` — `sensitiveKeys` is a read-only slice, initialized once, never mutated.
- `services/compliance/pkg/reporter/generator.go` — `reportBulkhead` is a `*resilience.Bulkhead` constructed in `main.go` and injected via `NewGenerator(bulkhead)`.
- Pre-compiled regular expressions (`var emailRE = regexp.MustCompile(...)`).
- `errors.New` sentinel errors.

#### No circular imports

The Go toolchain enforces this at compile time.

### 0.3 Naming Conventions

#### Names eliminate the need for comments

```go
// Bad
d := time.Since(start)

// Good
requestDuration := time.Since(start)
```

#### Exported names carry their package prefix — do not repeat it

```go
// In package repository — Bad
type UserRepository struct{}

// In package repository — Good
type Repository struct{}  // caller writes repository.Repository
```

**Canonical type names per package:**

| Package | Type name |
|---|---|
| `pkg/repository/` | `Repository` |
| `pkg/service/` | `Service` |
| `pkg/handlers/` | `Handler` |
| `pkg/outbox/` | `Writer` |
| `pkg/router/` | `Router` |

#### Interface names describe behavior

```go
type UserReader interface {
    GetByID(ctx context.Context, id string) (*models.User, error)
}
type UserWriter interface {
    Create(ctx context.Context, u *models.User) error
    Update(ctx context.Context, u *models.User) error
}
type UserRepository interface {
    UserReader
    UserWriter
}
```

#### Sentinel errors use `Err` prefix

```go
var (
    ErrNotFound      = errors.New("not found")
    ErrAlreadyExists = errors.New("already exists")
    ErrUnauthorized  = errors.New("unauthorized")
    ErrCircuitOpen   = errors.New("circuit breaker open")
    ErrBulkheadFull  = errors.New("bulkhead full")
    ErrRLSNotSet     = errors.New("RLS org_id context not set") // returned only when org_id
                                                                 // is required but absent;
                                                                 // empty string is valid for
                                                                 // system operations
)
```

#### Acceptable abbreviations

`ctx`, `cfg`, `err`, `ok`, `id`, `tx`, `db`, `w`, `r` (HTTP handlers). Nothing else abbreviated.

### 0.4 Error Handling

#### Never discard errors silently

```go
// Unacceptable
_ = db.Close()

// Required
if err := db.Close(); err != nil {
    slog.ErrorContext(ctx, "failed to close db connection", "error", err)
}
```

#### Wrap errors once at each layer boundary

```go
// Repository layer
func (r *Repository) GetByID(ctx context.Context, id string) (*models.User, error) {
    var u models.User
    err := r.db.QueryRow(ctx, query, id).Scan(/* fields */)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, fmt.Errorf("query user by id %s: %w", id, err)
    }
    return &u, nil
}

// Service layer
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("get user: %w", err)
    }
    return user, nil
}
```

#### Use `errors.Is` and `errors.As` — never string matching

```go
// Good
if errors.Is(err, ErrNotFound) {
    return http.StatusNotFound
}

// Bad
if strings.Contains(err.Error(), "not found") {}
```

#### Log or return — never both

Log at the outermost layer (HTTP handler or Kafka consumer). Service and repository layers return errors; they do not log them.

#### Panic only for programmer errors and startup invariants

```go
func NewService(repo Repository, events EventPublisher) *Service {
    if repo == nil {
        panic("NewService: repo is required")
    }
    return &Service{repo: repo, events: events}
}
```

#### Do not return `nil, nil`

Return `ErrNotFound` or an equivalent sentinel. Callers must never nil-check a returned pointer when the error is also nil.

### 0.5 Interfaces

#### The consuming package owns the interface

```go
// Bad — shared/ defines the interface; all services couple to it
// package shared/kafka
type Publisher interface { Publish(...) error }

// Good — IAM service defines exactly what it needs
// services/iam/pkg/service/user.go
type eventPublisher interface {
    Publish(ctx context.Context, topic, key string, payload []byte) error
}
```

`shared/` exports concrete types and structs. Interfaces over those types live in the consuming service's `pkg/service/` package.

#### Keep interfaces small. Compose when needed.

#### Do not add interfaces prematurely

Add an interface when: you have a second implementation, you need a test double, or you are crossing a significant layer boundary.

### 0.6 Concurrency

#### Every goroutine has an explicit owner and a termination path

```go
func (s *Service) Run(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)
    g.Go(func() error { return s.runEventLoop(ctx) })
    g.Go(func() error { return s.runCleanupLoop(ctx) })
    return g.Wait()
}

func (s *Service) runEventLoop(ctx context.Context) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := s.processBatch(ctx); err != nil {
                s.logger.ErrorContext(ctx, "batch processing failed", "error", err)
            }
        }
    }
}
```

#### `wg.Add` before the goroutine starts, `wg.Done` via `defer` as the first line

```go
// Bad — race: Add may not be called before Wait
go func(item Item) {
    wg.Add(1)
    defer wg.Done()
    process(item)
}(item)

// Good
wg.Add(1)
go func(item Item) {
    defer wg.Done()
    process(item)
}(item)
```

### 0.7 Context Discipline

#### `context.Context` is always the first parameter, never stored in a struct

The sole exception: a long-running background worker where the context is the worker's entire lifetime.

#### Never pass `context.Background()` inside a request handler

```go
// Bad — outlives the HTTP request
user, err := h.repo.GetByID(context.Background(), id)

// Good — cancelled when client disconnects
user, err := h.repo.GetByID(r.Context(), id)
```

#### Context values are for request-scoped metadata only

Context carries: trace IDs, request IDs, authenticated user IDs, `org_id` for RLS. Dependencies go in struct fields.

```go
func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}
```

### 0.8 Dependency Injection & Wiring

#### Constructor injection — always

```go
type Service struct {
    repo   userReader
    cache  Cache
    events eventPublisher
}

func NewService(repo userReader, cache Cache, events eventPublisher) *Service {
    if repo == nil {
        panic("NewService: repo is required")
    }
    return &Service{repo: repo, cache: cache, events: events}
}
```

#### `main.go` is the wiring file

All dependency construction belongs in `main.go`. The full dependency graph is visible in one place.

#### Use functional options for constructors with more than three parameters

```go
type ClientOption func(*clientOptions)

func WithTimeout(d time.Duration) ClientOption {
    return func(o *clientOptions) { o.timeout = d }
}
```

### 0.9 Configuration

#### Fail fast at startup — never discover bad config at request time

`config.MustLoad()` panics on any missing or malformed required variable. Use the `shared/config` helpers exclusively. Never call `os.Getenv` from business packages.

#### Typed sub-configs

```go
type Config struct {
    Addr     string
    Postgres PostgresConfig
    Redis    RedisConfig
    Kafka    KafkaConfig
    JWT      JWTConfig
}
```

### 0.10 Testing

#### Test behaviour, not implementation

Tests must not assert on internal state or call unexported methods.

#### Table-driven tests for input/output variation

```go
func TestValidateEmail(t *testing.T) {
    cases := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid",          "user@example.com", false},
        {"missing at",     "userexample.com",  true},
        {"missing domain", "user@",            true},
        {"empty",          "",                 true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            err := ValidateEmail(tc.input)
            if tc.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

#### `require` for fatal assertions, `assert` for non-fatal

#### Fakes over generated mocks for narrow interfaces

```go
type fakeUserRepo struct {
    users     map[string]*models.User
    createErr error
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*models.User, error) {
    if u, ok := f.users[id]; ok {
        return u, nil
    }
    return nil, ErrNotFound
}
```

Generated mocks (`mockery`) are acceptable only for interfaces with more than five methods.

#### Integration tests use `testcontainers-go` with real databases

```go
func TestRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    ctx := context.Background()
    pool := startPostgres(t, ctx)
    repo := repository.NewRepository(pool)

    t.Run("create and retrieve user", func(t *testing.T) {
        org := seedOrg(t, pool)
        created, err := repo.Create(ctx, CreateInput{OrgID: org.ID, Email: "test@example.com"})
        require.NoError(t, err)
        found, err := repo.GetByID(ctx, created.ID)
        require.NoError(t, err)
        assert.Equal(t, "test@example.com", found.Email)
    })
}
```

#### CI runs all tests with `-race`; coverage floor is 70% per package

### 0.11 Observability

#### Structured logging with `log/slog`, JSON in all non-dev environments

```go
// Bad
log.Printf("user %s logged in from %s", userID, ip)

// Good
slog.InfoContext(ctx, "user login success",
    "user_id",    userID,
    "ip_address", ip,
    "mfa_used",   mfaUsed,
)
```

#### `SafeAttr` for any attribute whose key might be sensitive

The `SafeAttr` function (Section 15.3) redacts values whose key contains any of: `password`, `secret`, `token`, `key`, `auth`, `credential`, `private`, `bearer`, `authorization`, `cookie`, `session`.

#### Distributed tracing: start a span at every service call boundary

```go
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
    ctx, span := tracer.Start(ctx, "Service.GetUser",
        trace.WithAttributes(attribute.String("user.id", id)),
    )
    defer span.End()

    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, fmt.Errorf("get user: %w", err)
    }
    return user, nil
}
```

### 0.12 HTTP Handler Rules

#### Handlers are thin: bind → validate → call service → respond. Centralize error-to-status-code mapping. Never expose internal error messages to callers. Always set explicit server timeouts. Validate `Content-Type: application/json` on all POST/PUT/PATCH handlers; return `415 Unsupported Media Type` otherwise (SCIM endpoints accept `application/scim+json`).

```go
server := &http.Server{
    Addr:              cfg.Addr,
    Handler:           router,
    ReadTimeout:       5 * time.Second,
    ReadHeaderTimeout: 2 * time.Second,
    WriteTimeout:      10 * time.Second,
    IdleTimeout:       120 * time.Second,
}
```

#### Centralize error-to-status-code mapping

```go
func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
    var valErr *ValidationError
    switch {
    case errors.Is(err, ErrNotFound):
        h.respondError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", err.Error())
    case errors.Is(err, ErrAlreadyExists):
        h.respondError(w, r, http.StatusConflict, "RESOURCE_CONFLICT", err.Error())
    case errors.Is(err, ErrUnauthorized):
        h.respondError(w, r, http.StatusForbidden, "FORBIDDEN", err.Error())
    case errors.Is(err, ErrCircuitOpen):
        h.respondError(w, r, http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE",
            "service temporarily unavailable")
    case errors.Is(err, ErrBulkheadFull):
        h.respondError(w, r, http.StatusTooManyRequests, "CAPACITY_EXCEEDED",
            "service at capacity, retry later")
    case errors.As(err, &valErr):
        h.respondError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", valErr.Error())
    default:
        slog.ErrorContext(r.Context(), "unhandled service error", "error", err)
        h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR",
            "an unexpected error occurred")
    }
}
```

#### Never expose internal error messages to callers

#### Always set explicit server timeouts

```go
server := &http.Server{
    Addr:              cfg.Addr,
    Handler:           router,
    ReadTimeout:       5 * time.Second,
    ReadHeaderTimeout: 2 * time.Second,
    WriteTimeout:      10 * time.Second,
    IdleTimeout:       120 * time.Second,
}
```

### 0.13 Forbidden Patterns

| Pattern | Why forbidden | Exception |
|---|---|---|
| `init()` for side effects | Uncontrollable execution order, no error return | Read-only sentinels, compiled regexes |
| `log.Fatal` / `os.Exit` outside `main` | Bypasses all deferred cleanup | `main.go` startup only |
| `any` / `interface{}` as parameter type | Turns compile-time errors into runtime panics | JSON marshal/unmarshal, `slog.Any` |
| `time.Sleep` in service code | Not context-cancellable, untestable | `scripts/re-encrypt-mfa.sh` (operational script only) |
| Shadowed `err` variables | Silent bugs | — |
| String concatenation in SQL | SQL injection vector | — |
| Kafka direct publish from business handler | Dual-write problem | — |
| `os.Getenv` from business packages | Bypasses typed config | — |
| Package-level mutable state | Test-order dependent, concurrent-unsafe | Named exceptions in §0.2 |
| PBKDF2 computed on every authenticated request | ~800ms/request; complete throughput bottleneck | — |
| bcrypt in an unbounded goroutine pool | CPU starvation; prevents meeting login SLO | — |

### 0.14 Code Review Checklist

**Package & structure**
- [ ] Package has a single, statable purpose
- [ ] No `utils`, `common`, `misc`, `helpers` package added to `shared/`
- [ ] No service imports another service's `pkg/` packages

**Errors**
- [ ] No discarded errors (`_ = ...`)
- [ ] Errors wrapped once at layer boundaries
- [ ] `errors.Is`/`errors.As` used — no string matching
- [ ] No log-and-return; no `nil, nil` returns; no shadowed `err`

**Concurrency**
- [ ] Every goroutine has a clear owner and termination path
- [ ] `wg.Add` called before goroutine starts
- [ ] No `time.Sleep` — `time.NewTicker` inside `select{}` for polling
- [ ] bcrypt called through worker pool, never in raw goroutine

**Context**
- [ ] `ctx` is first parameter on every I/O function
- [ ] `context.Background()` not used in request handlers
- [ ] Context values are typed (no raw string keys)

**Database**
- [ ] All SQL uses `$1`, `$2` parameters — no string interpolation
- [ ] `rls.SetSessionVar` called (via `db.WithOrgID`) before every PostgreSQL query
- [ ] RLS policy uses `NULLIF(current_setting('app.org_id', true), '')::UUID`
- [ ] Transactions defer `Rollback` and commit last
- [ ] No transaction held open across a network call
- [ ] Kafka offsets committed only after successful downstream write
- [ ] `version` column updated atomically on all SCIM-exposed resources

**HTTP**
- [ ] Handler only binds, validates, calls service, responds
- [ ] `Content-Type` validated before JSON decode; returns 415 if wrong
- [ ] `http.MaxBytesReader` applied to every request body
- [ ] Server configured with `ReadTimeout`, `WriteTimeout`, `IdleTimeout`

**Security**
- [ ] Webhook URLs re-validated at delivery time (not only at registration)
- [ ] `jti` included in JWT; blocklist checked on every authenticated request
- [ ] SCIM org_id derived from token config, not from `X-Org-ID` header
- [ ] Connector auth uses fast-hash prefix for cache lookup; PBKDF2 only on DB miss

**Observability**
- [ ] Sensitive fields passed through `SafeAttr`
- [ ] External calls wrapped in OTel spans
- [ ] Metrics label cardinality will not cause Prometheus explosion

**Interfaces & DI**
- [ ] Interfaces defined in the consuming package
- [ ] No `init()` for side effects; no `log.Fatal` / `os.Exit` outside `main`

---

## 1. Project Overview

### 1.1 What is OpenGuard?

OpenGuard is an open-source, self-hostable **centralized security control plane**. Connected applications register with OpenGuard and integrate via a lightweight SDK, SCIM 2.0, OIDC/SAML, and outbound webhooks. User traffic never flows *through* OpenGuard — it is a governance hub, not a proxy.

It operates at Fortune-500 scale: 100,000+ users, 10,000+ organizations, millions of audit events per day, cryptographic audit trail integrity, zero cross-tenant data leakage, and sub-100ms policy evaluation at p99.

**Core capabilities:**
- **Identity & Access Management:** OIDC/SAML IdP. SSO, SCIM 2.0, TOTP/WebAuthn MFA, API token lifecycle, session management.
- **Policy Engine:** Real-time RBAC evaluation via SDK. Fails closed. SDK caches decisions locally for up to 60 seconds during control plane unavailability.
- **Connector Registry:** Connected applications register and receive org-scoped API credentials. Credentials stored with PBKDF2 hash at rest; hot-path lookup uses a fast-hash prefix scheme against Redis.
- **Event Ingestion:** Connected apps push audit events to `POST /v1/events/ingest`. Events are normalized into the same Kafka-backed audit pipeline as internal events.
- **Threat Detection:** Streaming anomaly scoring — brute force, impossible travel, off-hours access, account takeover, privilege escalation.
- **Audit Log:** Append-only, HMAC hash-chained, cryptographically verifiable event trail with configurable retention.
- **Alerting & Webhooks:** Rule-based and ML-scored alerts with SIEM export and signed outbound webhook delivery.
- **Compliance Reporting:** GDPR, SOC 2, HIPAA report generation with PDF output.
- **Content Scanning / DLP:** Real-time PII, credential, and financial data detection.
- **Admin Dashboard:** Next.js 14 web console.

### 1.2 Performance Targets (Canonical SLOs)

These are hard targets. Phase 8 must verify each one with k6 load tests. A phase is not complete until its SLOs are met.

| Operation | p50 | p99 | p999 | Throughput |
|-----------|-----|-----|------|------------|
| `POST /oauth/token` (IAM OIDC) | 40ms | 150ms | 400ms | 2,000 req/s |
| `POST /v1/policy/evaluate` (uncached) | 5ms | 30ms | 80ms | 10,000 req/s |
| `POST /v1/policy/evaluate` (Redis cached) | 1ms | 5ms | 15ms | 10,000 req/s |
| SDK local cache hit (no network) | <1ms | <1ms | <1ms | unlimited |
| `GET /audit/events` (paginated) | 20ms | 100ms | 250ms | 1,000 req/s |
| Kafka event → audit DB insert | — | 2s | 5s | 50,000 events/s |
| Compliance report generation | — | 30s | 120s | 10 concurrent |
| `POST /v1/events/ingest` (connector push) | 10ms | 50ms | 150ms | 20,000 req/s |
| `GET /v1/scim/v2/Users` | 30ms | 500ms | 1,500ms | 500 req/s |
| Connector registry lookup (Redis cached) | 1ms | 5ms | 15ms | — |
| DLP async scan latency | — | 500ms | 2s | — |

**Notes on SLO feasibility:**
- `POST /oauth/token` at 150ms p99 requires bcrypt to run through a bounded worker pool (Section 9.3.9). At cost 12, bcrypt takes 250–400ms per operation. Without pooling, 2,000 req/s would require ~800 goroutines each blocking on bcrypt, causing CPU starvation. With a worker pool sized to `2 × NumCPU`, throughput is limited by CPU capacity; scale horizontally to meet the target.
- `POST /v1/events/ingest` at 50ms p99 at 20,000 req/s assumes backpressure shedding kicks in before PostgreSQL outbox writes become the bottleneck (Section 9.4.2).

### 1.3 Design Principles

| Principle | Implementation |
|-----------|---------------|
| **Fail closed** | Policy unavailable → SDK denies after 60s cache TTL. IAM unavailable → reject all logins. DLP sync-block unavailable → reject events (per-org opt-in). |
| **Exactly-once audit** | Every state-changing operation produces exactly one audit event via the Transactional Outbox. Connected-app events deduplicated by `event_id` (unique index, scoped to retention window). |
| **Zero cross-tenant leakage** | PostgreSQL RLS enforced at the DB layer. RLS policy uses `NULLIF(current_setting('app.org_id', true), '')::UUID` to handle NULL and empty string uniformly. |
| **Immutable audit trail** | Append-only MongoDB with per-org HMAC hash chaining. Batch chain assignment for throughput (Section 11.2.3). |
| **Least privilege (services)** | Each service has its own DB user with table-level grants. Migration runs as `openguard_migrate` (DDL only, no `BYPASSRLS` on data tables). |
| **Secret rotation without downtime** | JWT signing uses `kid`. Multiple valid keys coexist during rotation. Same pattern for MFA encryption keys. |
| **Access token revocation** | JWT `jti` claim; Redis blocklist checked on every authenticated request. |
| **mTLS between services** | All internal service-to-service calls use mTLS. |
| **Exactly-once Kafka delivery** | Idempotent Kafka producer. Consumer commits offsets only after successful downstream write. |
| **Cache-first connector auth** | Fast-hash prefix → Redis; PBKDF2 only on cache miss → DB. Sustains 20,000 req/s event ingest. |

---

## 2. Architecture Principles

### 2.1 The Dual-Write Problem

The root cause of most audit trail gaps in security systems:

```go
// WRONG — process crash between these two lines = permanent audit gap
db.Exec("INSERT INTO users ...")
kafka.Publish("audit.trail", event)
```

**The fix:** The Transactional Outbox Pattern (Section 7). The business row and the event record are committed atomically in the same PostgreSQL transaction. A separate relay process reads committed outbox records and publishes to Kafka.

```go
// CORRECT — atomic: both succeed or both fail
tx.Exec("INSERT INTO users ...")
tx.Exec("INSERT INTO outbox_records ...")
tx.Commit()
// Relay publishes asynchronously — no Kafka in the write path
```

### 2.2 Kafka Consumer Offset Commit Contract

This rule is non-negotiable. Every Kafka consumer in this system uses **manual offset commit mode**. An offset is committed only after the downstream write (MongoDB, ClickHouse, Redis, or PostgreSQL) has been confirmed.

```
Consumer reads message
  → Process (write to MongoDB, ClickHouse, etc.)
    → On success: commit offset
    → On failure: do NOT commit, retry or route to DLQ
```

The consequence: during bulk writes, if a batch of 500 documents is submitted to MongoDB but the service crashes before committing offsets, those 500 messages are reprocessed on restart. The `event_id` unique index on MongoDB `audit_events` and the Kafka idempotent producer together make this safe.

### 2.3 Multi-Tenancy Isolation

Three isolation tiers:

| Tier | Mechanism | Plan |
|------|-----------|------|
| **Shared** | PostgreSQL RLS on shared tables | Free / SMB |
| **Schema** | Dedicated PostgreSQL schema per org | Mid-market |
| **Shard** | Dedicated PostgreSQL instance per org | Enterprise / regulated |

This spec fully implements **Shared** (RLS) and scaffolds Schema/Shard as extension points. All application code is written RLS-first. The key requirement: every tenant table has an explicit `org_id UUID NOT NULL` column, and every outbox table has the same. The RLS policy always compares against this column — never against the Kafka partition key or any other proxy.

### 2.4 CQRS and Read/Write Split

The audit log has asymmetric load. Read/write split is enforced in the repository layer:

**MongoDB write path** (Kafka consumer → primary): Bulk insert up to 500 documents or 1-second flush interval. Offsets committed after successful `BulkWrite()`. Chain sequence assigned via batched atomic reservation (Section 11.2.3).

**MongoDB read path** (HTTP handlers → secondary): `readPreference: secondaryPreferred`. Compliance report queries use `readPreference: secondary` (acceptable staleness: 5s).

### 2.5 Choreography-Based Sagas

User provisioning via SCIM touches multiple services. OpenGuard uses choreography-based sagas via Kafka compensating events. Each step is idempotent and publishes the next step's trigger.

**SCIM `POST /scim/v2/Users` saga:**

```
IAM:        user.created          → audit.trail + saga.orchestration
Policy:     [consumes user.created] → assigns default org policies
            policy.assigned        → audit.trail
Threat:     [consumes user.created] → initializes baseline profile
            threat.baseline.init   → audit.trail
Alerting:   [consumes user.created] → configures notification preferences
            alert.prefs.init       → audit.trail
```

**Saga timeout:** When `user.created` is published, IAM writes a deadline record to a Redis sorted set: `ZADD saga:deadlines <unix_deadline> <saga_id>`. A background watcher (consumer group `openguard-saga-v1`) polls the sorted set every 10 seconds for expired entries. On expiry, it publishes `saga.timed_out` with `compensation: true`. Deadline: `SAGA_STEP_TIMEOUT_SECONDS` (default 30s) per step.

**Compensation (any step failure or timeout):**

```
Policy:     policy.assignment.failed (compensation:true, caused_by: <event_id>)
IAM:        [consumes policy.assignment.failed] → sets user status=provisioning_failed
            user.provisioning.failed → audit.trail
Threat:     [consumes user.provisioning.failed] → removes baseline profile
Alerting:   [consumes user.provisioning.failed] → removes notification preferences
```

Every service that participates in a saga must define compensation handlers for all previous steps. Consumer groups use `auto.offset.reset: earliest` so replays are safe.

### 2.6 App Registration and Credential Flow

**Key scheme:** API keys have two components:
- **Prefix** (first 8 chars, non-secret): used as the Redis cache lookup key via `SHA-256(prefix)`. O(microseconds).
- **Secret** (remaining chars): verified against the stored PBKDF2 hash only on cache miss (DB path). ~400ms, rare.

```
Admin       → POST /v1/admin/connectors           (JWT auth)
            ← { connector_id, api_key_plaintext }  (one-time; never stored)

ConnectedApp → POST /v1/events/ingest             (Bearer api_key_plaintext)

Control Plane auth flow:
  1. Parse key: prefix = key[0:8], secret = key[8:]
  2. fastHash = SHA-256(prefix)
  3. Lookup Redis: GET "connector:fasthash:{fastHash}"
     → Cache hit: deserialize ConnectedApp; verify secret matches stored PBKDF2 hash
       (full PBKDF2 verify only if last_verified_at > 5min ago; else trust cache)
     → Cache miss: PBKDF2-HMAC-SHA512(key, salt, 600000) → lookup DB by pbkdf2_hash
       → on hit: SET in Redis with 30s TTL
  4. Check status == "active"
  5. rls.WithOrgID(ctx, connector.OrgID)
  6. withConnectorScopes(ctx, connector.Scopes)
```

**Cache invalidation:** on `PATCH /v1/admin/connectors/:id`, call `DEL "connector:fasthash:{fastHash}"` before returning the HTTP response.

**Inbound:** Apps push to `POST /v1/events/ingest`. Events are normalized and written to the outbox.

**Outbound:** Webhook delivery service reads from `TopicWebhookDelivery`, signs with HMAC-SHA256, POSTs to connector URL. Delivery state is persisted in `webhook_deliveries` (PostgreSQL) so crash recovery does not restart from attempt 1. After `WEBHOOK_MAX_ATTEMPTS` exhaustion, the record is moved to `webhook.dlq` topic.

### 2.8 SCIM Authentication

SCIM provisioning is called by external identity providers (Okta, Azure AD). These callers authenticate with a per-org SCIM bearer token. **The org_id is derived from the token configuration, not from any client-supplied header.**

```go
// shared/middleware/scim.go
type SCIMToken struct {
    Token string `json:"token"`
    OrgID string `json:"org_id"`
}

// IAM_SCIM_TOKENS_JSON is a JSON array of SCIMToken objects.
// Each org that uses SCIM provisioning gets its own bearer token.
func SCIMAuthMiddleware(tokens []SCIMToken) func(http.Handler) http.Handler {
    // Build a map for O(1) lookup. Keys are token strings; values are org IDs.
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

### 2.9 Certificate Rotation

**Procedure (zero-downtime):**
1. Generate new cert: `scripts/gen-mtls-certs.sh --service <name> --renew`.
2. Update the target service's cert/key mounts. CA cert does not change.
3. Rolling deploy target service. Old and new pods share the same CA → mTLS succeeds across both.
4. CA rotation uses a dual-CA trust period documented in `docs/runbooks/ca-rotation.md`.

### 2.10 Connection Pooling Targets

| Service | DB | Pool min | Pool max | Max conn lifetime |
|---------|----|----------|----------|-------------------|
| IAM | PostgreSQL | 5 | 25 | 1800s |
| Control Plane | PostgreSQL (outbox only) | 2 | 15 | 300s |
| Outbox Relay | PostgreSQL (`openguard_outbox` role) | 2 | 10 | **60s** |
| Connector Registry | PostgreSQL | 2 | 10 | 1800s |
| Policy | PostgreSQL | 2 | 15 | 1800s |
| Audit (write) | MongoDB | 2 | 10 | — |
| Audit (read) | MongoDB | 5 | 30 | — |
| Compliance | ClickHouse | 2 | 8 | — |
| All services | Redis | 5 | 20 | — |

**Note on Outbox Relay pool lifetime:** Set to 60s so that after a PostgreSQL primary failover, stale connections to the old primary are recycled within 60 seconds, and the relay resumes draining the backlog promptly.

### 2.11 Tenant Offboarding

Triggered by `org.offboard` event:

1. IAM: Revoke all active sessions and API tokens. Set users to `status=deprovisioned`.
2. Control Plane: Suspend all connectors. Invalidate their Redis cache entries.
3. Policy: Archive all policies.
4. Webhook Delivery: Drain in-flight queue.
5. Audit: Finalize hash chain; write `org.offboarded` terminal event.
6. Compliance: Queue GDPR erasure export if requested.
7. Scheduler: After `ORG_DATA_RETENTION_DAYS`, hard-delete in this exact order (respects FK constraints):
   `outbox_records` → `dlp_findings` → `dlp_policies` → ... → `orgs`.

### 2.12 API Versioning Policy

All routes are prefixed `/v1/`. When a breaking change is required:
1. Implement `/v2/` route alongside `/v1/`.
2. Add `Deprecation: true` and `Sunset: <date>` headers to `/v1/` responses.
3. Maintain `/v1/` for a minimum of 6 months after `/v2/` GA.



---

## 3. Repository Layout

```
openguard/
├── .github/
│   └── workflows/
│       ├── ci.yml
│       ├── security.yml
│       └── release.yml
├── services/
│   ├── control-plane/
│   ├── connector-registry/
│   ├── iam/
│   ├── policy/
│   ├── threat/
│   ├── audit/
│   ├── alerting/
│   ├── webhook-delivery/
│   ├── compliance/
│   └── dlp/
├── sdk/
│   ├── go.mod                  # module: github.com/openguard/sdk
│   ├── policy/
│   │   ├── client.go           # Calls POST /v1/policy/evaluate
│   │   └── cache.go            # Local LRU cache; fail-closed after TTL
│   ├── events/
│   │   ├── publisher.go        # Batches and pushes to POST /v1/events/ingest
│   │   └── batcher.go          # Buffer: SDK_EVENT_BATCH_SIZE or SDK_EVENT_FLUSH_INTERVAL_MS
│   ├── breaker.go              # Circuit breaker: defined failure modes (Section 3.1)
│   └── client.go               # Root client; holds credentials and base URL
├── shared/
│   ├── go.mod                  # module: github.com/openguard/shared
│   ├── kafka/
│   │   ├── producer.go         # idempotent producer (enable.idempotence=true, acks=all)
│   │   ├── consumer.go         # manual offset commit mode
│   │   ├── topics.go
│   │   └── outbox/
│   │       ├── relay.go
│   │       └── poller.go
│   ├── middleware/
│   │   ├── apikey.go           # Connector API key auth + Redis cache
│   │   ├── scim.go             # SCIM bearer token auth (separate from API key)
│   │   ├── tenant.go           # Sets app.org_id for RLS
│   │   ├── ratelimit.go
│   │   ├── circuitbreaker.go
│   │   ├── logger.go
│   │   ├── security.go         # HTTP security headers
│   │   └── mtls.go
│   ├── models/
│   │   ├── event.go
│   │   ├── user.go
│   │   ├── policy.go
│   │   ├── connector.go
│   │   ├── errors.go           # Canonical sentinel errors
│   │   ├── outbox.go
│   │   └── saga.go
│   ├── rls/
│   │   └── context.go          # WithOrgID, OrgID, SetSessionVar, TxSetSessionVar
│   ├── resilience/
│   │   ├── breaker.go
│   │   ├── retry.go
│   │   └── bulkhead.go
│   ├── telemetry/
│   │   ├── otel.go
│   │   ├── metrics.go
│   │   └── logger.go           # SafeAttr
│   ├── crypto/
│   │   ├── jwt.go              # Multi-key keyring
│   │   ├── aes.go              # Multi-key AES-256-GCM keyring
│   │   ├── pbkdf2.go           # API key hashing: PBKDF2-HMAC-SHA512, 600k iterations
│   │   └── hmac.go             # HMAC-SHA256 for webhook signatures
│   └── validator/
│       └── validator.go
├── infra/
│   ├── docker/
│   │   └── docker-compose.yml
│   ├── k8s/
│   │   └── helm/openguard/
│   ├── kafka/
│   │   └── topics.json
│   ├── certs/
│   └── monitoring/
│       ├── prometheus.yml
│       ├── grafana/
│       └── alerts/
├── web/
│   ├── app/
│   │   └── (dashboard)/
│   │       ├── connectors/
│   │       ├── threats/
│   │       ├── audit/
│   │       └── compliance/
│   └── package.json
├── loadtest/
│   ├── auth.js
│   ├── policy-evaluate.js
│   ├── audit-query.js
│   ├── event-ingest.js
│   └── kafka-throughput.js
├── docs/
│   ├── architecture.md
│   ├── runbooks/
│   │   ├── kafka-consumer-lag.md
│   │   ├── circuit-breaker-open.md
│   │   ├── audit-hash-mismatch.md
│   │   ├── secret-rotation.md
│   │   ├── outbox-dlq.md
│   │   ├── postgres-rls-bypass.md
│   │   ├── load-shedding.md
│   │   ├── connector-suspension.md
│   │   ├── webhook-delivery-failure.md
│   │   └── ca-rotation.md
│   ├── contributing.md
│   └── api/
├── scripts/
│   ├── create-topics.sh
│   ├── migrate.sh
│   ├── seed.sh
│   ├── gen-mtls-certs.sh       # --service <name> [--renew] flags
│   └── rotate-jwt-keys.sh
├── go.work
├── .env.example
├── Makefile
└── README.md
```

### 3.1 SDK Circuit Breaker Specification

`sdk/breaker.go` wraps all control plane calls. The failure modes are precisely defined:

```go
// sdk/breaker.go

// SDKBreaker wraps control plane HTTP calls with circuit-breaker semantics.
// Failure definition: HTTP 5xx, connection timeout, connection refused.
// HTTP 4xx are NOT failures — they are expected protocol responses.
// HTTP 429 (rate limit) is a failure for circuit breaker purposes.
type SDKBreaker struct {
    cb *gobreaker.CircuitBreaker
}

// BreakerConfig for the SDK (driven by environment or SDK client options):
//   FailureThreshold: 5 consecutive failures
//   OpenDuration:     10s before moving to half-open
//   MaxRequests:      2 requests in half-open state (probe before full recovery)
//   RequestTimeout:   SDK_POLICY_EVALUATE_TIMEOUT_MS (default 100ms)

// PolicyEvaluate calls POST /v1/policy/evaluate through the breaker.
// When the breaker is open:
//   - Returns (cachedDecision, nil) if a cached decision exists for the input.
//   - Returns (DenyDecision, ErrCircuitOpen) if cache is empty or expired.
// The SDK NEVER grants access when the breaker is open and the cache is cold.
func (b *SDKBreaker) PolicyEvaluate(ctx context.Context, req PolicyRequest) (PolicyDecision, error)

// EventIngest calls POST /v1/events/ingest through the breaker.
// When the breaker is open: buffer events locally up to SDK_OFFLINE_RETRY_LIMIT (default 500).
// After 500 events, drop newest (tail drop) and publish `sdk.buffer_overflow` metric.
// On breaker recovery: flush buffered events in a background goroutine.
func (b *SDKBreaker) EventIngest(ctx context.Context, event AuditEvent) error
```

### 3.2 Scope Middleware (Connector Authorization)

Connector registry logic in `shared/middleware/scope.go` ensures a connector has the required permission for the endpoint.

```go
func RequiredScope(scope string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            scopes, ok := r.Context().Value(connectorScopesKey).([]string)
            if !ok || !contains(scopes, scope) {
                writeError(w, http.StatusForbidden, "INSUFFICIENT_SCOPE", 
                           fmt.Sprintf("required scope: %s", scope), r)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Canonical scopes: `events:write`, `policy:evaluate`, `audit:read`, `scim:write`, `dlp:scan`.

### 3.3 Go Workspace

```go
// go.work
go 1.22

use (
    ./shared
    ./sdk
    ./services/control-plane
    ./services/connector-registry
    ./services/iam
    ./services/policy
    ./services/threat
    ./services/audit
    ./services/alerting
    ./services/webhook-delivery
    ./services/compliance
    ./services/dlp
)
```

### 3.3 Service Module Layout (canonical)

```
services/<name>/
├── go.mod                      # module: github.com/openguard/<name>
├── main.go                     # wires everything, starts server + graceful shutdown
├── Dockerfile
├── migrations/
│   ├── 001_<name>.up.sql
│   └── 001_<name>.down.sql     # Required for every up migration
├── pkg/
│   ├── config/
│   │   └── config.go
│   ├── db/
│   │   ├── postgres.go         # pgxpool; enforced-RLS wrapper type
│   │   ├── mongo.go            # separate read + write clients
│   │   └── migrations.go       # golang-migrate with distributed lock
│   ├── outbox/
│   │   └── writer.go
│   ├── handlers/
│   │   └── <resource>.go
│   ├── service/
│   │   └── <resource>.go
│   ├── repository/
│   │   └── <resource>.go
│   └── router/
│       └── router.go
└── testdata/
    └── fixtures/
```

---

## 4. Shared Contracts

All types in this section live in `github.com/openguard/shared/models`. They are **immutable across phases** — rename requires a major version bump of the shared module and migration of all consumers.

### 4.1 Kafka Event Envelope

```go
package models

import (
    "encoding/json"
    "time"
)

// EventEnvelope is the wire format for every Kafka message on every topic.
// Consumers MUST validate SchemaVer before processing.
type EventEnvelope struct {
    ID          string          `json:"id"`           // UUIDv4, globally unique
    Type        string          `json:"type"`         // dot-separated: "auth.login.success"
    OrgID       string          `json:"org_id"`       // tenant identifier
    ActorID     string          `json:"actor_id"`     // user ID, service name, or "system"
    ActorType   string          `json:"actor_type"`   // "user" | "service" | "system"
    OccurredAt  time.Time       `json:"occurred_at"`  // event time, not processing time
    Source      string          `json:"source"`       // originating service: "iam", "policy", etc.
    EventSource string          `json:"event_source"` // "internal" | "connector:<connector_id>"
    TraceID     string          `json:"trace_id"`     // OTel W3C trace ID (from context at publish time)
    SpanID      string          `json:"span_id"`      // OTel span ID
    SchemaVer   string          `json:"schema_ver"`   // "1.0" — increment on breaking changes
    Idempotent  string          `json:"idempotent"`   // dedup key for consumers
    Payload     json.RawMessage `json:"payload"`      // event-specific struct, JSON encoded
}
```

### 4.2 Outbox Record

```go
package models

import "time"

// OutboxRecord is persisted in the same transaction as the business operation.
// The relay process reads pending records and publishes to Kafka.
// IMPORTANT: The `org_id` column is explicit (UUID, not NULL) and is used
// for RLS enforcement. It must match the org_id of the business operation.
// The `key` column is the Kafka partition key (typically the same as org_id,
// but may differ). Do not use `key` in RLS policies — use `org_id`.
type OutboxRecord struct {
    ID          string     `db:"id"`           // UUIDv4
    OrgID       string     `db:"org_id"`       // Explicit org_id for RLS — NOT the Kafka key
    Topic       string     `db:"topic"`        // Kafka topic name
    Key         string     `db:"key"`          // Kafka partition key (usually org_id, may differ)
    Payload     []byte     `db:"payload"`      // JSON-encoded EventEnvelope
    Status      string     `db:"status"`       // "pending" | "published" | "dead"
    Attempts    int        `db:"attempts"`
    LastError   string     `db:"last_error"`
    CreatedAt   time.Time  `db:"created_at"`
    PublishedAt *time.Time `db:"published_at"`
    DeadAt      *time.Time `db:"dead_at"`
}
```

### 4.3 Saga Event

```go
package models

// SagaEvent wraps an EventEnvelope with saga orchestration metadata.
type SagaEvent struct {
    EventEnvelope
    SagaID       string `json:"saga_id"`             // UUIDv4, same across all steps
    SagaType     string `json:"saga_type"`           // "user.provision" | "user.deprovision"
    SagaStep     int    `json:"saga_step"`           // 1-based step number
    Compensation bool   `json:"compensation"`        // true = rollback event
    CausedBy     string `json:"caused_by,omitempty"` // event ID that caused this step
}
```

### 4.4 Kafka Topic Registry

```go
// shared/kafka/topics.go
package kafka

const (
    TopicAuthEvents        = "auth.events"
    TopicPolicyChanges     = "policy.changes"
    TopicDataAccess        = "data.access"
    TopicThreatAlerts      = "threat.alerts"
    TopicAuditTrail        = "audit.trail"
    TopicNotificationsOut  = "notifications.outbound"
    TopicSagaOrchestration = "saga.orchestration"
    TopicOutboxDLQ         = "outbox.dlq"
    TopicConnectorEvents   = "connector.events"
    TopicWebhookDelivery   = "webhook.delivery"
    TopicWebhookDLQ        = "webhook.dlq"
)

const (
    GroupAudit           = "openguard-audit-v1"
    GroupThreat          = "openguard-threat-v1"
    GroupAlerting        = "openguard-alerting-v1"
    GroupCompliance      = "openguard-compliance-v1"
    GroupPolicy          = "openguard-policy-v1"
    GroupSaga            = "openguard-saga-v1"
    GroupWebhookDelivery = "openguard-webhook-delivery-v1"
)
```

### 4.5 Canonical User Model

```go
package models

import "time"

type User struct {
    ID                 string     `json:"id" db:"id"`
    OrgID              string     `json:"org_id" db:"org_id"`
    Email              string     `json:"email" db:"email"`
    DisplayName        string     `json:"display_name" db:"display_name"`
    Status             UserStatus `json:"status" db:"status"`
    MFAEnabled         bool       `json:"mfa_enabled" db:"mfa_enabled"`
    MFAMethod          string     `json:"mfa_method,omitempty" db:"mfa_method"` // "totp" | "webauthn"
    SCIMExternalID     string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
    ProvisioningStatus string     `json:"provisioning_status" db:"provisioning_status"`
    TierIsolation      string     `json:"tier_isolation" db:"tier_isolation"`
    CreatedAt          time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
    DeletedAt          *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type UserStatus string

const (
    UserStatusActive             UserStatus = "active"
    UserStatusSuspended          UserStatus = "suspended"
    UserStatusDeprovisioned      UserStatus = "deprovisioned"
    UserStatusProvisioningFailed UserStatus = "provisioning_failed"
)
```

### 4.6 Connected App Model

```go
package models

import "time"

type ConnectedApp struct {
    ID                string     `json:"id" db:"id"`
    OrgID             string     `json:"org_id" db:"org_id"`
    Name              string     `json:"name" db:"name"`
    WebhookURL        string     `json:"webhook_url" db:"webhook_url"`
    WebhookSecretHash string     `json:"-" db:"webhook_secret_hash"`
    APIKeyHash        string     `json:"-" db:"api_key_hash"` // PBKDF2-HMAC-SHA512, 600k iterations
    Scopes            []string   `json:"scopes" db:"scopes"`
    Status            string     `json:"status" db:"status"`  // "active" | "suspended" | "pending"
    CreatedAt         time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
    SuspendedAt       *time.Time `json:"suspended_at,omitempty" db:"suspended_at"`
}
```

### 4.7 Standard HTTP Contracts

**Error response:**
```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "User with id 'abc' not found",
    "request_id": "req_01j...",
    "trace_id": "4bf92f3577b34da6...",
    "retryable": false
  }
}
```

```go
package models

type APIError struct {
    Error APIErrorBody `json:"error"`
}

type APIErrorBody struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    RequestID string `json:"request_id"`
    TraceID   string `json:"trace_id"`
    Retryable bool   `json:"retryable"`
}
```

**Pagination envelope (all list endpoints):**
```json
{
  "data": [],
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 1024,
    "total_pages": 21,
    "next_cursor": "eyJpZCI6IjEyMyJ9"
  }
}
```

Cursor-based pagination for audit log and threat alert endpoints. Page-number pagination for user and policy lists.

**SCIM error responses** must follow RFC 7644 §3.12, not the `APIError` format:
```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
  "status": "404",
  "detail": "User not found"
}
```

The SCIM handler layer translates domain errors to SCIM error format before responding.

### 4.8 Canonical Sentinel Errors

```go
// shared/models/errors.go
package models

import "errors"

var (
    ErrNotFound       = errors.New("not found")
    ErrAlreadyExists  = errors.New("already exists")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrForbidden      = errors.New("forbidden")
    ErrCircuitOpen    = errors.New("circuit breaker open")
    ErrBulkheadFull   = errors.New("bulkhead full")
    ErrRetryable      = errors.New("retryable error")
    ErrSagaFailed     = errors.New("saga step failed")
    ErrRLSNotSet      = errors.New("RLS org_id context not set")
)
```
---

## 5. Environment & Configuration

### 5.1 `.env.example` (canonical — every variable required unless marked optional)

```dotenv
# ── App ──────────────────────────────────────────────────────────────
APP_ENV=development                    # development | staging | production
LOG_LEVEL=info                         # debug | info | warn | error
LOG_FORMAT=json                        # json | text (use json in non-dev)

# ── Control Plane ────────────────────────────────────────────────────
CONTROL_PLANE_PORT=8080
CONTROL_PLANE_API_KEY_SALT=change-me-32-bytes-hex
# PBKDF2-HMAC-SHA512, 600k iterations. Salt must be 32+ bytes.
CONTROL_PLANE_WEBHOOK_SIGNING_SECRET=change-me-32-bytes-hex
CONTROL_PLANE_POLICY_CACHE_TTL_SECONDS=60
CONTROL_PLANE_EVENT_INGEST_MAX_BATCH=500
CONTROL_PLANE_RATE_LIMIT_CONNECTOR=1000          # req/min per connector_id
CONTROL_PLANE_TENANT_QUOTA_RPM=5000              # req/min per org_id (all connectors)
CONTROL_PLANE_CONNECTOR_CACHE_TTL_SECONDS=30     # Redis TTL for connector auth cache
CONTROL_PLANE_MTLS_CERT_FILE=/certs/control-plane.crt
CONTROL_PLANE_MTLS_KEY_FILE=/certs/control-plane.key
CONTROL_PLANE_MTLS_CA_FILE=/certs/ca.crt

# ── Connector Registry ───────────────────────────────────────────────
CONNECTOR_REGISTRY_PORT=8090
CONNECTOR_REGISTRY_MTLS_CERT_FILE=/certs/connector-registry.crt
CONNECTOR_REGISTRY_MTLS_KEY_FILE=/certs/connector-registry.key
CONNECTOR_REGISTRY_MTLS_CA_FILE=/certs/ca.crt

# ── Webhook Delivery ─────────────────────────────────────────────────
WEBHOOK_DELIVERY_PORT=8091
WEBHOOK_MAX_ATTEMPTS=5
WEBHOOK_BACKOFF_BASE_MS=1000
WEBHOOK_BACKOFF_MAX_MS=60000
WEBHOOK_DELIVERY_TIMEOUT_MS=5000
WEBHOOK_DELIVERY_MTLS_CERT_FILE=/certs/webhook-delivery.crt
WEBHOOK_DELIVERY_MTLS_KEY_FILE=/certs/webhook-delivery.key
WEBHOOK_DELIVERY_MTLS_CA_FILE=/certs/ca.crt

# ── SDK ──────────────────────────────────────────────────────────────
SDK_CONTROL_PLANE_URL=https://api.openguard.example.com
SDK_POLICY_CACHE_TTL_SECONDS=60
SDK_POLICY_EVALUATE_TIMEOUT_MS=100
SDK_EVENT_BATCH_SIZE=100
SDK_EVENT_FLUSH_INTERVAL_MS=2000
SDK_OFFLINE_RETRY_LIMIT=500              # Max events buffered locally when breaker is open

# ── IAM ──────────────────────────────────────────────────────────────
IAM_PORT=8081
IAM_JWT_KEYS_JSON=[{"kid":"k1","secret":"change-me","algorithm":"HS256","status":"active"}]
IAM_JWT_EXPIRY_SECONDS=900             # 15 minutes
IAM_REFRESH_TOKEN_EXPIRY_DAYS=30
IAM_SAML_ENTITY_ID=https://openguard.example.com
IAM_SAML_IDP_METADATA_URL=https://idp.example.com/metadata
IAM_OIDC_ISSUER=https://accounts.example.com
IAM_OIDC_CLIENT_ID=openguard
IAM_OIDC_CLIENT_SECRET=change-me
IAM_SCIM_TOKENS_JSON=[{"token":"scim-t1","org_id":"00000000-0000-0000-0000-000000000000"}]
IAM_MFA_TOTP_ISSUER=OpenGuard
IAM_MFA_ENCRYPTION_KEY_JSON=[{"kid":"mk1","key":"base64-encoded-32-bytes","status":"active"}]
IAM_WEBAUTHN_RPID=openguard.example.com
IAM_WEBAUTHN_RPORIGIN=https://openguard.example.com
IAM_MTLS_CERT_FILE=/certs/iam.crt
IAM_MTLS_KEY_FILE=/certs/iam.key
IAM_MTLS_CA_FILE=/certs/ca.crt
# Bounded worker pool for bcrypt verification to meet login SLO
IAM_BCRYPT_WORKER_COUNT=8                # Default: NumCPU / 2

# ── Policy ───────────────────────────────────────────────────────────
POLICY_PORT=8082
POLICY_CACHE_TTL_SECONDS=30
POLICY_MTLS_CERT_FILE=/certs/policy.crt
POLICY_MTLS_KEY_FILE=/certs/policy.key
POLICY_MTLS_CA_FILE=/certs/ca.crt

# ── Threat ───────────────────────────────────────────────────────────
THREAT_PORT=8083
THREAT_ANOMALY_WINDOW_MINUTES=60
THREAT_MAX_FAILED_LOGINS=10
THREAT_GEO_CHANGE_THRESHOLD_KM=500
THREAT_MAXMIND_DB_PATH=/data/GeoLite2-City.mmdb
THREAT_MTLS_CERT_FILE=/certs/threat.crt
THREAT_MTLS_KEY_FILE=/certs/threat.key
THREAT_MTLS_CA_FILE=/certs/ca.crt

# ── Audit ────────────────────────────────────────────────────────────
AUDIT_PORT=8084
AUDIT_RETENTION_DAYS=730
AUDIT_HASH_CHAIN_SECRET=change-me-32-bytes-hex
AUDIT_BULK_INSERT_MAX_DOCS=500
AUDIT_BULK_INSERT_FLUSH_MS=1000
AUDIT_MTLS_CERT_FILE=/certs/audit.crt
AUDIT_MTLS_KEY_FILE=/certs/audit.key
AUDIT_MTLS_CA_FILE=/certs/ca.crt

# ── Alerting ─────────────────────────────────────────────────────────
ALERTING_PORT=8085
ALERTING_SLACK_WEBHOOK_URL=            # optional
ALERTING_SMTP_HOST=smtp.example.com
ALERTING_SMTP_PORT=587
ALERTING_SMTP_USER=
ALERTING_SMTP_PASS=
ALERTING_SIEM_WEBHOOK_URL=             # optional
ALERTING_SIEM_WEBHOOK_HMAC_SECRET=change-me
ALERTING_MTLS_CERT_FILE=/certs/alerting.crt
ALERTING_MTLS_KEY_FILE=/certs/alerting.key
ALERTING_MTLS_CA_FILE=/certs/ca.crt

# ── Compliance ───────────────────────────────────────────────────────
COMPLIANCE_PORT=8086
COMPLIANCE_REPORT_MAX_CONCURRENT=10
COMPLIANCE_MTLS_CERT_FILE=/certs/compliance.crt
COMPLIANCE_MTLS_KEY_FILE=/certs/compliance.key
COMPLIANCE_MTLS_CA_FILE=/certs/ca.crt

# ── DLP ──────────────────────────────────────────────────────────────
DLP_PORT=8087
DLP_ENTROPY_THRESHOLD=4.5
DLP_MIN_CREDENTIAL_LENGTH=24
DLP_SYNC_BLOCK_TIMEOUT_MS=30           # Sync check timeout during ingest
DLP_MTLS_CERT_FILE=/certs/dlp.crt
DLP_MTLS_KEY_FILE=/certs/dlp.key
DLP_MTLS_CA_FILE=/certs/ca.crt

# ── PostgreSQL ───────────────────────────────────────────────────────
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=openguard_app
POSTGRES_PASSWORD=change-me
POSTGRES_DB=openguard
POSTGRES_SSLMODE=verify-full
POSTGRES_SSLROOTCERT=/certs/postgres-ca.crt
POSTGRES_POOL_MIN_CONNS=5
POSTGRES_POOL_MAX_CONNS=25
POSTGRES_OUTBOX_USER=openguard_outbox    # DML only, constrained role
POSTGRES_OUTBOX_PASSWORD=change-me
POSTGRES_MIGRATE_USER=openguard_migrate  # DDL only role
POSTGRES_MIGRATE_PASSWORD=change-me

# ── MongoDB ──────────────────────────────────────────────────────────
MONGO_URI_PRIMARY=mongodb://localhost:27017
MONGO_URI_SECONDARY=mongodb://localhost:27018
MONGO_DB=openguard
MONGO_AUTH_SOURCE=admin
MONGO_TLS_CA_FILE=/certs/mongo-ca.crt
MONGO_WRITE_POOL_MAX=10
MONGO_READ_POOL_MAX=30

# ── Redis ────────────────────────────────────────────────────────────
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=change-me
REDIS_DB=0
REDIS_TLS_CERT_FILE=/certs/redis.crt
REDIS_POOL_SIZE=20

# ── Kafka ────────────────────────────────────────────────────────────
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=openguard
KAFKA_TLS_CA_FILE=/certs/kafka-ca.crt
KAFKA_SASL_USER=openguard
KAFKA_SASL_PASSWORD=change-me
KAFKA_PRODUCER_IDEMPOTENT=true

# ── ClickHouse ───────────────────────────────────────────────────────
CLICKHOUSE_ADDR=localhost:9000
CLICKHOUSE_USER=openguard
CLICKHOUSE_PASSWORD=change-me
CLICKHOUSE_DB=openguard
CLICKHOUSE_TLS_CA_FILE=/certs/clickhouse-ca.crt

# ── Circuit Breakers ─────────────────────────────────────────────────
CB_POLICY_TIMEOUT_MS=50
CB_POLICY_FAILURE_THRESHOLD=5
CB_POLICY_OPEN_DURATION_MS=10000
CB_IAM_TIMEOUT_MS=200
CB_IAM_FAILURE_THRESHOLD=5
CB_IAM_OPEN_DURATION_MS=15000

# ── OpenTelemetry ────────────────────────────────────────────────────
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SAMPLING_RATE=0.1

# ── Org Lifecycle ────────────────────────────────────────────────────
ORG_DATA_RETENTION_DAYS=2555           # 7 years (compliance baseline)
ORG_OFFBOARDING_GRACE_PERIOD_DAYS=30   # Suspension before hard delete

# ── Frontend ─────────────────────────────────────────────────────────
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me
```

### 5.2 Config Loading Pattern

```go
// shared/config/config.go
package config

import (
    "encoding/json"
    "fmt"
    "os"
    "strconv"
    "time"
)

func Must(key string) string {
    v := os.Getenv(key)
    if v == "" {
        panic(fmt.Sprintf("required env var %q not set", key))
    }
    return v
}

func Default(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func MustInt(key string) int {
    v := Must(key)
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
    }
    return n
}

func DefaultInt(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
    }
    return n
}

func MustDuration(key string) time.Duration {
    return time.Duration(MustInt(key)) * time.Millisecond
}

func MustJSON(key string, dest any) {
    v := Must(key)
    if err := json.Unmarshal([]byte(v), dest); err != nil {
        panic(fmt.Sprintf("env var %q is not valid JSON: %v", key, err))
    }
}
```

---

## 6. Multi-Tenancy & RLS

### 6.1 PostgreSQL Row-Level Security

Every table storing tenant data **must** have RLS enabled with an explicit `org_id UUID NOT NULL` column. The migration runner refuses to apply any migration that creates a new table with an `org_id` column without also enabling RLS. The RLS policy always compares against the `org_id` column — never against any Kafka partition key or surrogate.

#### 6.1.1 Application DB Roles

```sql
-- DDL Role: Used by CI/CD migrations. DDL only, NO BYPASSRLS.
CREATE ROLE openguard_migrate LOGIN PASSWORD 'change-me';
GRANT ALL ON SCHEMA public TO openguard_migrate;

-- DML Role: Used by services. DML only, NO BYPASSRLS.
CREATE ROLE openguard_app LOGIN PASSWORD 'change-me';
GRANT USAGE ON SCHEMA public TO openguard_app;

-- Outbox Role: Used by relay only. Has BYPASSRLS on outbox_records only.
CREATE ROLE openguard_outbox LOGIN PASSWORD 'change-me';
GRANT USAGE ON SCHEMA public TO openguard_outbox;
GRANT SELECT, UPDATE, DELETE ON outbox_records TO openguard_outbox;
GRANT BYPASSRLS ON outbox_records TO openguard_outbox;
```

#### 6.1.2 RLS Setup (canonical pattern for every org-scoped table)

```sql
-- Apply to: users, api_tokens, sessions, mfa_configs, policies, policy_assignments,
-- outbox_records, dlp_policies, dlp_findings, and any future tenant table.
ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <table> FORCE ROW LEVEL SECURITY;  -- applies to table owner too

CREATE POLICY <table>_org_isolation ON <table>
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

-- The 'true' flag makes current_setting return NULL instead of error when not set.
-- NULL::UUID != any org_id → no rows match → fail safe (zero rows, not error).
```

#### 6.1.3 Enforced RLS Wrapper

A raw `*pgxpool.Pool` is dangerous because developers can call `pool.QueryRow(ctx, ...)` without setting the session variable. Instead, every service wraps the pool in an `OrgPool` type that enforces RLS on every connection acquisition:

```go
// shared/rls/context.go
package rls

import (
    "context"
    "fmt"
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

// SetSessionVar sets the PostgreSQL session variable for RLS on a pooled connection.
// orgID is always passed as a query parameter ($1), never string-interpolated.
// orgID originates from a verified JWT claim or connector registry lookup.
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    if orgID == "" {
        _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

// TxSetSessionVar sets the RLS variable within an existing transaction.
// Use this (not SetSessionVar) inside transaction blocks to avoid
// acquiring a second connection.
func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
    if orgID == "" {
        _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}

// OrgPool wraps pgxpool.Pool and automatically sets the RLS session variable
// on every acquired connection. This prevents the "forgot to call SetSessionVar"
// bug class entirely — you cannot get a connection without RLS being set.
type OrgPool struct {
    pool *pgxpool.Pool
}

func NewOrgPool(pool *pgxpool.Pool) *OrgPool {
    return &OrgPool{pool: pool}
}

// Acquire acquires a connection and sets the RLS session variable from context.
// Returns ErrRLSNotSet if org_id is not in the context (defensive: should never
// happen if middleware is correctly configured, but must not silently leak data).
func (p *OrgPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
    orgID := OrgID(ctx)
    // Note: empty orgID is valid for system operations (SCIM admin, outbox relay).
    // The empty string causes RLS to return zero rows for tenant tables, which is
    // the correct fail-safe behavior.
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

// BeginTx acquires a connection, sets RLS, and begins a transaction.
// TxSetSessionVar is called inside the transaction (same connection, no double-acquire).
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

Every repository in every service uses `*rls.OrgPool`, not `*pgxpool.Pool` directly.

#### 6.1.4 Outbox Table RLS — Corrected

The previous version of the RLS policy used `key = current_setting(...)`, which relies on the Kafka partition key matching org_id — a fragile implicit contract. The corrected version:

```sql
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,     -- Explicit; RLS is enforced on this column
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
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

The Outbox relay uses the `openguard_outbox` database role, which has `BYPASSRLS` on `outbox_records` only. This allows the relay to read all tenants' pending records without setting a tenant context.

#### 6.1.5 Outbox Cleanup Job

To prevent `outbox_records` from growing unbounded, a background worker runs every 10 minutes to delete published records older than 24 hours. This job also runs as `openguard_outbox`.

```sql
DELETE FROM outbox_records
WHERE status = 'published'
  AND published_at < NOW() - INTERVAL '24 hours';
```

#### 6.1.6 API Key Middleware (Connector Auth)

Hot-path API key lookup uses the two-tier Prefix/Secret scheme (Section 2.6).

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
                // 2. Slow-path verification (only every 5 minutes per connector)
                if time.Since(app.LastVerifiedAt) > 5 * time.Minute {
                    if !verifyPBKDF2(secret, app.KeyHash) {
                        writeError(w, http.StatusUnauthorized, "INVALID_KEY", "invalid secret", r)
                        return
                    }
                }
                ctx := rls.WithOrgID(r.Context(), app.OrgID)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }

            // 3. Cache miss -> DB path (O(400ms))
            // Only hits if the prefix was never seen or cache expired.
            fullHash := pbkdf2Hash(raw)
            app, err := repo.GetByHash(r.Context(), fullHash)
            if err != nil {
                writeError(w, http.StatusUnauthorized, "INVALID_KEY", "unrecognized key", r)
                return
            }
            cache.Set(r.Context(), fastHash, app, 30 * time.Second)
            ctx := rls.WithOrgID(r.Context(), app.OrgID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```
            } else {
                var err error
                connector, err = connectorRepo.GetByKeyHash(r.Context(), keyHash)
                if err != nil {
                    writeError(w, http.StatusUnauthorized, "INVALID_API_KEY", "invalid or unknown API key", r)
                    return
                }
                cache.Set(r.Context(), keyHash, connector, cacheTTL)
            }

            if connector.Status != "active" {
                writeError(w, http.StatusUnauthorized, "CONNECTOR_SUSPENDED", "connector is suspended", r)
                return
            }

            ctx := rls.WithOrgID(r.Context(), connector.OrgID)
            ctx = withConnectorID(ctx, connector.ID)
            ctx = withConnectorScopes(ctx, connector.Scopes)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### 6.2 Per-Tenant Quotas

Two rate limit tiers using Redis sliding window (token bucket, 1-minute window):

```go
// shared/middleware/ratelimit.go
// Key schema:
//   Connector-level: "rl:connector:{connector_id}:{window_minute}"
//   Tenant-level:    "rl:org:{org_id}:{window_minute}"
//
// Both are checked. Request is rejected if either limit is exceeded.
// Redis failure mode: FAIL OPEN (allow requests, log error metric).
// Rationale: availability over rate limiting when Redis is degraded.
//
// On limit exceeded: return 429 with:
//   Retry-After: <seconds to next window>
//   X-RateLimit-Limit: <limit>
//   X-RateLimit-Remaining: 0
```

---

## 7. Transactional Outbox Pattern

### 7.1 Outbox Table (every service that publishes events)

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
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
GRANT BYPASSRLS ON TABLE outbox_records TO openguard_outbox;

-- Application user grants (never superuser)
GRANT SELECT, INSERT, UPDATE ON outbox_records TO openguard_app;
GRANT SELECT, UPDATE ON outbox_records TO openguard_outbox;

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

### 7.2 Outbox Writer

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
// The transaction must already have the RLS session variable set (via rls.TxSetSessionVar).
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

### 7.3 Outbox Relay

```go
// shared/kafka/outbox/relay.go
package outbox

import (
    "context"
    "log/slog"
    "time"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/openguard/shared/kafka"
    "golang.org/x/sync/errgroup"
)

// Relay reads pending outbox records and publishes them to Kafka.
//
// Architecture:
//   - Uses PostgreSQL LISTEN/NOTIFY to wake up immediately on new records.
//   - Falls back to polling every 100ms (time.NewTicker, never time.Sleep).
//   - Uses FOR UPDATE SKIP LOCKED for safe concurrent relay instances.
//   - Multiple relay instances are safe: row-level locking prevents double-publish.
//
// Delivery guarantee:
//   - At-least-once delivery to Kafka.
//   - Kafka idempotent producer (enable.idempotence=true, acks=all) prevents duplicates.
//   - Records are marked "published" only after Kafka ack (sync produce).
//   - Records failing 5 times are marked "dead" and sent to outbox.dlq.
//
// PostgreSQL failover behavior:
//   - The relay reconnects automatically via pgxpool's built-in reconnection logic.
//   - Pending records buffered in PostgreSQL are published after reconnection.
//   - The 100ms polling fallback ensures no records are missed if LISTEN/NOTIFY
//     was interrupted during the failover window.
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

    // Goroutine 1: LISTEN for immediate notification
    g.Go(func() error { return r.listenNotify(ctx) })

    // Goroutine 2: Polling fallback (handles missed notifications and startup drain)
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
// publishes each to Kafka synchronously (blocking until ack), then updates status.
// The entire status update is in a single transaction per batch.
// If Kafka publish succeeds but the status UPDATE fails (rare DB error),
// the record remains "pending" and is republished on next batch — safe because
// the Kafka producer is idempotent (enable.idempotence=true).
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
        ORDER BY created_at
        LIMIT 100
        FOR UPDATE SKIP LOCKED
    `)
    if err != nil {
        return 0, fmt.Errorf("select outbox records: %w", err)
    }
    defer rows.Close()

    type record struct {
        id       string
        orgID    string
        topic    string
        key      string
        payload  []byte
        attempts int
    }
    var records []record
    for rows.Next() {
        var rec record
        if err := rows.Scan(&rec.id, &rec.orgID, &rec.topic, &rec.key, &rec.payload, &rec.attempts); err != nil {
            return 0, fmt.Errorf("scan outbox record: %w", err)
        }
        records = append(records, rec)
    }
    rows.Close()

    published := 0
    for _, rec := range records {
        pubErr := r.producer.Produce(ctx, rec.topic, rec.key, rec.payload)
        if pubErr != nil {
            newAttempts := rec.attempts + 1
            if newAttempts >= r.maxDead {
                if _, err := tx.Exec(ctx,
                    `UPDATE outbox_records SET status='dead', attempts=$1, last_error=$2, dead_at=NOW() WHERE id=$3`,
                    newAttempts, pubErr.Error(), rec.id,
                ); err != nil {
                    r.logger.ErrorContext(ctx, "failed to mark record dead", "id", rec.id, "error", err)
                }
                // Publish to DLQ (best-effort; DLQ failure is logged, not fatal)
                if dlqErr := r.producer.Produce(ctx, kafka.TopicOutboxDLQ, rec.orgID, rec.payload); dlqErr != nil {
                    r.logger.ErrorContext(ctx, "failed to publish to DLQ", "id", rec.id, "error", dlqErr)
                }
            } else {
                if _, err := tx.Exec(ctx,
                    `UPDATE outbox_records SET attempts=$1, last_error=$2 WHERE id=$3`,
                    newAttempts, pubErr.Error(), rec.id,
                ); err != nil {
                    r.logger.ErrorContext(ctx, "failed to increment attempts", "id", rec.id, "error", err)
                }
            }
            continue
        }
        if _, err := tx.Exec(ctx,
            `UPDATE outbox_records SET status='published', published_at=NOW() WHERE id=$1`,
            rec.id,
        ); err != nil {
            // Kafka message was published but status update failed.
            // Record remains pending; will be republished.
            // Idempotent producer prevents Kafka duplicates.
            r.logger.ErrorContext(ctx, "failed to mark record published (will republish)", "id", rec.id, "error", err)
        } else {
            published++
        }
    }

    if err := tx.Commit(ctx); err != nil {
        return 0, fmt.Errorf("commit relay batch: %w", err)
    }
    return published, nil
}
```

### 7.4 Business Handler Pattern (Canonical)

```go
// Canonical write handler. Every service write that produces an event follows this pattern.
// Do not deviate. All steps must be in one transaction.
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

---

## 8. Circuit Breakers & Resilience

### 8.1 Circuit Breaker Implementation

```go
// shared/resilience/breaker.go
package resilience

import (
    "context"
    "fmt"
    "log/slog"
    "time"
    "github.com/sony/gobreaker"
    "github.com/openguard/shared/models"
)

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
                "name", name,
                "from", from.String(),
                "to", to.String(),
            )
            // Emit metric: openguard_circuit_breaker_state{name, state}
        },
    })
}

// Call executes fn through the circuit breaker with a context timeout.
// The type parameter T prevents the unchecked type assertion bug:
// if cb.Execute returns nil on certain gobreaker error paths, the
// nil check before the type assertion prevents a panic.
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    result, err := cb.Execute(func() (any, error) {
        return fn(ctx)
    })
    if err != nil {
        var zero T
        if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
            return zero, fmt.Errorf("%w: %s", models.ErrCircuitOpen, cb.Name())
        }
        return zero, err
    }
    // Safe nil check before type assertion
    if result == nil {
        var zero T
        return zero, nil
    }
    return result.(T), nil
}
```

### 8.2 bcrypt Worker Pool

bcrypt is deliberately expensive (~300ms/op). In a high-traffic IAM service, raw goroutines per login would starve the CPU. OpenGuard uses a bounded worker pool for all bcrypt operations.

```go
// services/iam/pkg/service/auth.go
type AuthWorkerPool struct {
    jobs    chan bcryptJob
    workers int
}

func NewAuthWorkerPool(workers int) *AuthWorkerPool {
    p := &AuthWorkerPool{
        jobs:    make(chan bcryptJob, 100),
        workers: workers,
    }
    for i := 0; i < workers; i++ {
        go p.worker()
    }
    return p
}

func (p *AuthWorkerPool) Verify(ctx context.Context, hash, password string) error {
    res := make(chan error, 1)
    select {
    case p.jobs <- bcryptJob{hash, password, res}:
        return <-res
    case <-ctx.Done():
        return ctx.Err()
    default:
        return models.ErrBulkheadFull // backpressure: too many logins in flight
    }
}
```

Configured via `IAM_BCRYPT_WORKER_COUNT`. Recommended size: `2 × NumCPU`.

### 8.3 Failure Mode Table (Non-Negotiable)

| Scenario | Required behavior | Rationale |
|---|---|---|
| Policy service unreachable | SDK uses cached decision up to 60s, then **denies**. | Cache provides brief grace; after TTL, fail closed. |
| IAM service unreachable | **Reject all logins**, return `503`. | Cannot authenticate without IAM. |
| Connector registry unreachable | **Deny all API key requests** after Redis cache misses; return `503`. | Cannot validate credential. Cache still serves recent lookups. |
| Audit service unreachable | **Continue operation**, buffer via Outbox. | Audit is observability, not a gate. |
| Threat detection unreachable | **Continue operation**, log warning metric. | Threat is advisory, not a gate. |
| Redis unreachable | Rate limiting **fails open**; log error metric. | Availability over rate limiting. |
| Kafka unreachable | **Outbox buffers events in PostgreSQL**. Writes succeed; events queue. | Kafka is not in the write path. |
| ClickHouse unreachable | **Compliance reports fail with 503**. | Analytics is read-only. |
| Webhook delivery unreachable | **Retry via internal loop** with persistence in PostgreSQL. | Delivery state survives service restarts. |
| DLP service unreachable (sync-block mode) | **Reject event ingest** for orgs with `dlp_mode=block`. | Sync-block is an explicit opt-in with latency tradeoff. |
| SCIM IdP sends wrong org_id | **Ignore header; derive from token config**. | Prevents org_id spoofing (Section 2.8). |

### 8.4 Retry Policy

```go
// shared/resilience/retry.go
package resilience

type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    Retryable   func(error) bool
}

var DefaultRetry = RetryConfig{
    MaxAttempts: 5,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    10 * time.Second,
    Retryable: func(err error) bool {
        return errors.Is(err, models.ErrRetryable)
    },
}

// Do executes fn with retries.
// Backoff: exponential with full jitter: sleep = rand(0, min(MaxDelay, BaseDelay * 2^attempt))
// Respects context cancellation between attempts.
func Do(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
    var lastErr error
    for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
        if err := ctx.Err(); err != nil {
            return err
        }
        lastErr = fn(ctx)
        if lastErr == nil {
            return nil
        }
        if !cfg.Retryable(lastErr) {
            return lastErr
        }
        delay := jitter(cfg.BaseDelay, cfg.MaxDelay, attempt)
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
        }
    }
    return lastErr
}
```

### 8.5 Bulkhead (Concurrency Limiter)

```go
// shared/resilience/bulkhead.go
package resilience

import (
    "context"
    "fmt"
    "github.com/openguard/shared/models"
)

type Bulkhead struct {
    sem chan struct{}
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
    if maxConcurrent <= 0 {
        panic("NewBulkhead: maxConcurrent must be positive")
    }
    return &Bulkhead{sem: make(chan struct{}, maxConcurrent)}
}

func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
    select {
    case b.sem <- struct{}{}:
        defer func() { <-b.sem }()
        return fn()
    case <-ctx.Done():
        return fmt.Errorf("%w: bulkhead full", models.ErrBulkheadFull)
    }
}
```

Bulkhead instances are created in `main.go` and injected via constructors. They are never package-level variables initialized from env vars.
---

## 9. Phase 1 — Foundation

**Goal:** Running skeleton with enterprise-grade auth and working control plane. JWT multi-key rotation, RLS enforced, Outbox in place, circuit breakers configured, connector registration operational. At the end of Phase 1: an app can register, receive an API key, and call the control plane; a user can log in via OIDC and receive a JWT; every write publishes via the Outbox.

### 9.1 Prerequisites (produce before any service code)

The infra and CI setup must be established before service code begins. This is not "Phase 6" work — it is the foundation:

1. `infra/docker/docker-compose.yml` (see Section 14.1 for full spec).
2. `scripts/gen-mtls-certs.sh` — generates CA and per-service certs. Includes: `control-plane`, `connector-registry`, `iam`, `policy`, `threat`, `audit`, `alerting`, `webhook-delivery`, `compliance`, `dlp`.
3. `scripts/create-topics.sh` — idempotent topic creation from `infra/kafka/topics.json`. Detects broker count and adjusts replication factor.
4. `Makefile` with targets: `dev`, `test`, `lint`, `build`, `migrate`, `seed`, `load-test`, `certs`.
5. `.env.example` as defined in Section 5.1.
6. `.github/workflows/ci.yml` — the CI pipeline (Section 14.2) must be operational from the first commit.

### 9.2 Migration Strategy

Use `golang-migrate/migrate` with these invariants:

- Every `.up.sql` must have a corresponding `.down.sql`.
- Migrations are **additive only** in production: add nullable columns, add indexes, add tables. Never drop or rename in the same migration as adding.
- Every migration that creates a table with an `org_id` column must include the RLS setup for that table.
- The migration runner verifies checksums and refuses to apply a modified historical migration.
- Migrations run at service startup with a distributed lock to prevent concurrent runs in multi-replica deployments.

```go
// pkg/db/migrations.go (in each service)
// Distributed lock implementation using Redis SET NX with heartbeat goroutine.
// The heartbeat extends the lock TTL every 10s. If the process crashes,
// the heartbeat stops, the TTL expires (30s), and other replicas can proceed.
// This is safer than a fixed TTL, which can expire before a long migration completes.
func RunMigrations(ctx context.Context, dsn string, redisClient *redis.Client, serviceName string) error {
    lockKey := fmt.Sprintf("migrate-lock:%s", serviceName)
    lockTTL := 30 * time.Second

    // 1. SET NX with TTL
    acquired, err := redisClient.SetNX(ctx, lockKey, "locked", lockTTL).Result()
    if err != nil || !acquired {
        // Another replica holds the lock; wait and retry for up to 2 minutes
        return waitForMigration(ctx, redisClient, lockKey)
    }

    // 2. Start heartbeat goroutine to extend lock while migration runs
    heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
    defer cancelHeartbeat()
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-heartbeatCtx.Done():
                return
            case <-ticker.C:
                redisClient.Expire(ctx, lockKey, lockTTL)
            }
        }
    }()

    // 3. Run migrations as openguard_migrate role (DDL only)
    defer redisClient.Del(ctx, lockKey)
    m, err := migrate.New("file://migrations", dsn)
    if err != nil {
        return fmt.Errorf("create migrator: %w", err)
    }
    if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
        return fmt.Errorf("run migrations: %w", err)
    }
    return nil
}
```

### 9.3 IAM Service

#### 9.3.1 Database Schema

**001_create_orgs.up.sql**
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE orgs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL,
    slug           TEXT NOT NULL UNIQUE,
    plan           TEXT NOT NULL DEFAULT 'free',
    isolation_tier TEXT NOT NULL DEFAULT 'shared',
    mfa_required   BOOLEAN NOT NULL DEFAULT FALSE,
    sso_required   BOOLEAN NOT NULL DEFAULT FALSE,
    max_users      INT,
    max_sessions   INT NOT NULL DEFAULT 5,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE orgs FORCE ROW LEVEL SECURITY;
-- Orgs table: app user can only see its own org. System/admin operations use BYPASSRLS.
CREATE POLICY orgs_self_read ON orgs FOR SELECT
    USING (id = current_setting('app.org_id', true)::UUID);
```

**002_create_users.up.sql**
```sql
CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email               TEXT NOT NULL,
    display_name        TEXT NOT NULL DEFAULT '',
    password_hash       TEXT,            -- bcrypt, cost 12
    status              TEXT NOT NULL DEFAULT 'active',
    mfa_enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method          TEXT,
    scim_external_id    TEXT,
    provisioning_status TEXT NOT NULL DEFAULT 'complete',
    tier_isolation      TEXT NOT NULL DEFAULT 'shared',
    version             INT NOT NULL DEFAULT 1,  -- Atomic increment for SCIM ETags
    last_login_at       TIMESTAMPTZ,
    last_login_ip       INET,
    failed_login_count  INT NOT NULL DEFAULT 0,
    locked_until        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_users_org_id   ON users(org_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_email    ON users(email)  WHERE deleted_at IS NULL;
CREATE INDEX idx_users_scim_ext ON users(org_id, scim_external_id) WHERE scim_external_id IS NOT NULL;

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY users_org_isolation ON users
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT, UPDATE ON users TO openguard_app;
```

**003_create_sessions.up.sql**
```sql
CREATE TABLE sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id           UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    refresh_hash     TEXT NOT NULL UNIQUE,
    prev_refresh_hash TEXT,              -- Grace window: old hash valid for IAM_REFRESH_TOKEN_GRACE_SECONDS
    prev_hash_expiry  TIMESTAMPTZ,
    ip_address       INET,
    user_agent       TEXT,
    country_code     TEXT,
    city             TEXT,
    lat              DECIMAL(9,6),
    lng              DECIMAL(9,6),
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked          BOOLEAN NOT NULL DEFAULT FALSE,
    revoke_reason    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id) WHERE revoked = FALSE;
CREATE INDEX idx_sessions_org_id  ON sessions(org_id)  WHERE revoked = FALSE;

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY sessions_org_isolation ON sessions
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT, UPDATE ON sessions TO openguard_app;
```

**004_create_api_tokens.up.sql**
```sql
CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_tokens_org_id  ON api_tokens(org_id);
CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id);

ALTER TABLE api_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY api_tokens_org_isolation ON api_tokens
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT, UPDATE ON api_tokens TO openguard_app;
```

**005_create_mfa_configs.up.sql**
```sql
CREATE TABLE mfa_configs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    org_id            UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type              TEXT NOT NULL DEFAULT 'totp',  -- 'totp' | 'webauthn'
    encrypted_secret  TEXT NOT NULL,    -- Format: "mk1:<base64(nonce+ciphertext)>"
    -- Backup codes: NOT stored as bcrypt array. Stored as HMAC-SHA256 under
    -- IAM_MFA_BACKUP_CODE_HMAC_SECRET. Lookup is O(1) not O(N * bcrypt_cost).
    backup_code_hashes TEXT[] NOT NULL DEFAULT '{}',
    verified          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- WebAuthn credentials stored separately (one user can have multiple authenticators)
CREATE TABLE webauthn_credentials (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id           UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    credential_id    BYTEA NOT NULL UNIQUE,      -- WebAuthn credential ID
    public_key       BYTEA NOT NULL,             -- COSE-encoded public key
    sign_count       BIGINT NOT NULL DEFAULT 0,  -- Replay attack prevention
    aaguid           UUID,                       -- Authenticator type
    name             TEXT NOT NULL DEFAULT '',   -- User-assigned name
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at     TIMESTAMPTZ
);

ALTER TABLE mfa_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE mfa_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY mfa_configs_org_isolation ON mfa_configs
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

ALTER TABLE webauthn_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE webauthn_credentials FORCE ROW LEVEL SECURITY;
CREATE POLICY webauthn_credentials_org_isolation ON webauthn_credentials
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON mfa_configs TO openguard_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON webauthn_credentials TO openguard_app;
```

**006_create_outbox.up.sql** — standard outbox table (Section 7.1).

#### 9.3.2 MFA Backup Code Storage

Backup codes must be O(1) to look up, not O(N × bcrypt_cost). The correct scheme:

```go
// pkg/service/mfa.go
// Backup code generation:
//   1. Generate 8 random 8-character codes (e.g., "ABCD-1234")
//   2. For each code, compute HMAC-SHA256(code, IAM_MFA_BACKUP_CODE_HMAC_SECRET)
//   3. Store the array of hex-encoded HMACs in mfa_configs.backup_code_hashes
//
// Backup code verification:
//   1. Compute HMAC-SHA256(input_code, IAM_MFA_BACKUP_CODE_HMAC_SECRET)
//   2. Check if the result is in backup_code_hashes (O(1) with a DB query on the array)
//   3. If found, remove it from the array (single-use)
//
// Security: HMAC prevents brute-force enumeration of backup codes. The HMAC secret
// must be rotated separately from passwords and JWT keys.
```

#### 9.3.3 MFA Encryption (AES-256-GCM Multi-Key)

```go
// shared/crypto/aes.go
package crypto

type EncryptionKey struct {
    Kid    string `json:"kid"`
    Key    string `json:"key"`    // base64-encoded 32-byte key
    Status string `json:"status"` // "active" | "verify_only"
}

type EncryptionKeyring struct{ keys []EncryptionKey }

// Encrypt uses the first active key.
// Output: "<kid>:<base64(nonce+ciphertext)>"
func (k *EncryptionKeyring) Encrypt(plaintext []byte) (string, error)

// Decrypt parses kid from prefix, finds the matching key (active OR verify_only), decrypts.
func (k *EncryptionKeyring) Decrypt(ciphertext string) ([]byte, error)
```

#### 9.3.4 JWT Multi-Key Keyring

```go
// shared/crypto/jwt.go
package crypto

type JWTKey struct {
    Kid       string `json:"kid"`
    Secret    string `json:"secret"`
    Algorithm string `json:"algorithm"` // "HS256" | "RS256"
    Status    string `json:"status"`    // "active" | "verify_only"
}

type JWTKeyring struct{ keys []JWTKey }

// Sign uses the first key with status="active". Includes kid in JWT header.
func (k *JWTKeyring) Sign(claims jwt.Claims) (string, error)

// Verify extracts kid from header, finds matching key (active or verify_only),
// verifies signature and expiry.
// Returns ErrTokenExpired, ErrTokenInvalid, or nil.
func (k *JWTKeyring) Verify(tokenString string) (jwt.MapClaims, error)
```

#### 9.3.5 Risk-Based Session Protection

Applied at `/auth/refresh`. Scores are additive:

| Factor | Score | Definition |
|---|---|---|
| User agent family change | 60 | Chrome → Firefox, Safari → Chrome |
| IP subnet change (/16) | 40 | Different /16 subnet |
| IP host change (same /16) | 15 | Same /16, different host |
| UA version change (same family) | 20 | Chrome 119 → Chrome 122 |

**Thresholds:**
- Score ≥ 80: Revoke session immediately. Return `401 SESSION_REVOKED_RISK`. Publish `auth.session.revoked_risk` event via outbox.
- Score < 80: Accept. Rotate refresh token. Update session with new IP/UA.

**Refresh token concurrent request race condition:** The `sessions` table stores both `refresh_hash` (current) and `prev_refresh_hash` (previous, with `prev_hash_expiry`). On successful refresh:
1. New refresh token generated; `prev_refresh_hash = old refresh_hash`; `prev_hash_expiry = NOW() + IAM_REFRESH_TOKEN_GRACE_SECONDS`.
2. A concurrent refresh using the `prev_refresh_hash` within the grace window is accepted and returns the same new token (idempotent).
3. After `prev_hash_expiry`, the previous hash is no longer valid.

#### 9.3.6 WebAuthn Implementation

Use `github.com/go-webauthn/webauthn`. Configuration:

```go
// pkg/service/webauthn.go
func newWebAuthnConfig(cfg config.IAMConfig) *webauthn.WebAuthn {
    wConfig := &webauthn.Config{
        RPDisplayName: "OpenGuard",
        RPID:          cfg.WebAuthnRPID,
        RPOrigins:     []string{cfg.WebAuthnRPOrigin},
        // Attestation: "none" for most deployments. "indirect" for regulated environments.
        AttestationPreference: protocol.PreferNoAttestation,
        // Require resident key (passkey-style) for better UX
        AuthenticatorSelection: protocol.AuthenticatorSelection{
            RequireResidentKey: protocol.ResidentKeyRequirementRequired,
            UserVerification:   protocol.VerificationRequired,
        },
    }
    w, err := webauthn.New(wConfig)
    if err != nil {
        panic(fmt.Sprintf("failed to initialize WebAuthn: %v", err))
    }
    return w
}
```

WebAuthn challenge state is stored in Redis (TTL: 5 minutes) keyed by `webauthn:challenge:{user_id}:{session_id}`, not in the database. The challenge is deleted after successful verification.

#### 9.3.7 SCIM v2 Implementation

SCIM endpoints are exposed through the control plane at `/v1/scim/v2/*` and proxied to IAM via mTLS.

IAM implements the SCIM 2.0 protocol correctly:

**SCIM `ListResponse` envelope:**
```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:ListResponse"],
  "totalResults": 100,
  "startIndex": 1,
  "itemsPerPage": 50,
  "Resources": []
}
```

**SCIM `PATCH` with JSON Patch (RFC 6902):**
```go
// IAM handles SCIM PATCH operations which use JSONPatch operations,
// not standard JSON merge-patch.
type SCIMPatchOp struct {
    Schemas    []string        `json:"schemas"`
    Operations []SCIMOperation `json:"Operations"`
}

type SCIMOperation struct {
    Op    string          `json:"op"`    // "add" | "remove" | "replace"
    Path  string          `json:"path"`  // SCIM attribute path
    Value json.RawMessage `json:"value"`
}
```

**SCIM `ETag` support:** Every SCIM resource response includes `ETag: "{version}"`. Conditional updates with `If-Match` are enforced.

**SCIM error format** (RFC 7644 §3.12): The SCIM handler layer translates all domain errors to SCIM error format (see Section 4.7). OpenGuard's `APIError` format is never returned on SCIM endpoints.

#### 9.3.8 IAM HTTP Endpoints

**OIDC/SAML IdP** (public, standard TLS):

| Method | Path | Description |
|---|---|---|
| `GET` | `/oauth/authorize` | OIDC authorization endpoint |
| `POST` | `/oauth/token` | OIDC token (password, auth_code, refresh_token grants) |
| `GET` | `/oauth/userinfo` | OIDC userinfo |
| `GET` | `/oauth/jwks` | JSON Web Key Set |
| `GET` | `/oauth/.well-known/openid-configuration` | OIDC discovery document |
| `POST` | `/saml/acs` | SAML Assertion Consumer Service |
| `GET` | `/saml/metadata` | SAML SP metadata |

**Internal management API** (mTLS, called by control plane only):

| Method | Path | Description |
|---|---|---|
| `POST` | `/auth/register` | Create org + admin user (single transaction) |
| `POST` | `/auth/login` | Password login → JWT + session |
| `POST` | `/auth/refresh` | Rotate refresh token with risk scoring |
| `POST` | `/auth/logout` | Revoke session |
| `POST` | `/auth/mfa/enroll` | Begin TOTP enrollment |
| `POST` | `/auth/mfa/verify` | Complete TOTP enrollment |
| `POST` | `/auth/mfa/challenge` | Verify TOTP at login |
| `POST` | `/auth/webauthn/register/begin` | Begin WebAuthn credential registration |
| `POST` | `/auth/webauthn/register/finish` | Complete WebAuthn registration |
| `POST` | `/auth/webauthn/login/begin` | Begin WebAuthn authentication |
| `POST` | `/auth/webauthn/login/finish` | Complete WebAuthn authentication |
| `GET` | `/users` | List users (cursor paginated) |
| `POST` | `/users` | Create user |
| `GET` | `/users/:id` | Get user |
| `PATCH` | `/users/:id` | Update user |
| `DELETE` | `/users/:id` | Soft-delete |
| `POST` | `/users/:id/suspend` | Suspend user |
| `POST` | `/users/:id/activate` | Activate user |
| `GET` | `/users/:id/sessions` | List active sessions |
| `DELETE` | `/users/:id/sessions/:sid` | Revoke session |
| `DELETE` | `/users/:id/sessions` | Revoke all sessions |
| `GET` | `/users/:id/tokens` | List API tokens |
| `POST` | `/users/:id/tokens` | Create API token |
| `DELETE` | `/users/:id/tokens/:tid` | Revoke token |
| `POST` | `/users/bulk` | Bulk create/update (SCIM internal) |
| `GET` | `/orgs/me` | Get current org |
| `PATCH` | `/orgs/me` | Update org settings |

**SCIM v2** (SCIM bearer token auth, proxied from control plane):

| Method | Path | Description |
|---|---|---|
| `GET` | `/scim/v2/Users` | List users with SCIM ListResponse |
| `POST` | `/scim/v2/Users` | Provision user (triggers saga) |
| `GET` | `/scim/v2/Users/:id` | Get user (SCIM Resource format) |
| `PUT` | `/scim/v2/Users/:id` | Full update (with ETag check) |
| `PATCH` | `/scim/v2/Users/:id` | Partial update (RFC 6902 JSON Patch) |
| `DELETE` | `/scim/v2/Users/:id` | Deprovision user (triggers saga) |

#### 9.3.9 bcrypt worker pool integration

The `AuthWorkerPool` (Section 8.2) is initialized in `main.go` and injected into the IAM service. The `PostMethod` for `/oauth/token` calls `p.Verify()` instead of `bcrypt.CompareHashAndPassword()` directly. This ensures login latency remains predictable even under high load.

#### 9.3.10 IAM Kafka Events (via Outbox)

| Event type | Topic | Saga topic? |
|---|---|---|
| `auth.login.success` | `auth.events` | — |
| `auth.login.failure` | `auth.events` | — |
| `auth.login.locked` | `auth.events` | — |
| `auth.logout` | `auth.events` | — |
| `auth.mfa.enrolled` | `auth.events` | — |
| `auth.webauthn.registered` | `auth.events` | — |
| `auth.token.created` | `auth.events` | — |
| `user.created` | `audit.trail` | `saga.orchestration` |
| `user.deleted` | `audit.trail` | `saga.orchestration` |
| `user.scim.provisioned` | `audit.trail` | `saga.orchestration` |

### 9.4 Control Plane Foundation

#### 9.4.1 Route Table

The control plane's connector registry uses the two-tier Prefix/Secret scheme (Section 2.6).

| Method | Path | Required Scope | Circuit Breaker |
|---|---|---|---|
| `POST` | `/v1/policy/evaluate` | `policy:evaluate` | `cb-policy` |
| `POST` | `/v1/events/ingest` | `events:write` | — |
| `GET` | `/v1/scim/v2/Users` | `scim:read` | `cb-iam` |
| `POST` | `/v1/scim/v2/Users` | `scim:write` | `cb-iam` |
| `GET` | `/v1/scim/v2/Users/:id` | `scim:read` | `cb-iam` |
| `PATCH` | `/v1/scim/v2/Users/:id` | `scim:write` | `cb-iam` |

**Admin API (mTLS + JWT):**
- `POST /v1/admin/connectors`: Generates prefix (8 chars) + plaintext secret (24 chars). Hashes secret with PBKDF2.
- `PATCH /v1/admin/connectors/:id`: Invalidates Redis cache entry (`DEL connector:fasthash:{hash}`) on status or scope change.

#### 9.4.2 Connector Registry Schema

```sql
CREATE TABLE connector_registry (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    api_key_hash       TEXT NOT NULL, -- full PBKDF2 hash
    api_key_prefix     TEXT NOT NULL, -- first 8 chars for fast-hash lookup
    webhook_url        TEXT,
    webhook_secret_hash TEXT,
    scopes             TEXT[] NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'active',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_prefix ON connector_registry(api_key_prefix, status);
ALTER TABLE connector_registry ENABLE ROW LEVEL SECURITY;
GRANT SELECT ON connector_registry TO openguard_app;
```

### 9.5 DLQ Inspector (Phase 1 Operations)

A CLI tool for inspecting and replaying failed events from `TopicOutboxDLQ` and `TopicWebhookDLQ`.

```bash
# openguard-admin dlq list --topic outbox.dlq --org_id <uuid>
# openguard-admin dlq replay --id <uuid> --target audit.trail
```

### 9.6 Phase 1 Release Acceptance Criteria

1.  **IAM Security:**
    - [ ] `POST /oauth/token` passes with valid credentials; fails with 401 on invalid.
    - [ ] bcrypt cost is 12 (verified by unit test logging iteration count).
    - [ ] JWT contains `jti`, `iat`, `exp`, and `org_id`.
    - [ ] Redis blocklist correctly revokes `jti` on session logout.
2.  **Multi-Tenancy:**
    - [ ] Org A cannot see Org B's users via API (tested via curl).
    - [ ] `SELECT` on `users` table via `psql` as `openguard_app` returns 0 rows without `set_config`.
    - [ ] `openguard_migrate` role is used for all table creations.
3.  **Audit Integrity:**
    - [ ] User creation produces exactly one `outbox_record` in the same transaction.
    - [ ] Outbox relay publishes to Kafka and marks as `published`.
    - [ ] `idempotent` key in Kafka message matches PostgreSQL `id`.
4.  **Resilience & SLO:**
    - [ ] `POST /oauth/token` p99 < 150ms at 500 req/s with bcrypt worker pool enabled.
    - [ ] Outbox relay resumes draining backlog within 60s of PostgreSQL primary failover.
5.  **SCIM 2.0:**
    - [ ] `GET /v1/scim/v2/Users` returns correct resource counts and schema.
    - [ ] SCIM auth rejects requests with `X-Org-ID` header; only accepts token-based derivation.
    - [ ] `version` column increments on user patch.
6.  **Observability:**
    - [ ] Login failures log `ActorID` and `ActorType` to `slog` with `SafeAttr` redaction.
    - [ ] Every request includes `X-Request-ID` and OTel trace propagation.
    - [ ] Metrics for outbox lag and login failure rate are available in Prometheus.


---

## 10. Phase 2 — Policy Engine

**Goal:** p99 < 30ms for `POST /v1/policy/evaluate` (uncached); p99 < 5ms (Redis cached). Two-tier cache: SDK LRU (client-side) + Redis (server-side). Fail closed.

### 10.1 Database Schema

Standard policy tables plus:

**001_create_policies.up.sql**
```sql
CREATE TABLE policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    version      INT NOT NULL DEFAULT 1,  -- Atomic increment for ETag
    logic        JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**003_create_policy_eval_log.up.sql**
```sql
CREATE TABLE policy_eval_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    user_id      UUID NOT NULL,
    action       TEXT NOT NULL,
    resource     TEXT NOT NULL,
    result       BOOLEAN NOT NULL,
    policy_ids   UUID[] NOT NULL DEFAULT '{}',
    latency_ms   INT NOT NULL,
    cache_hit    TEXT NOT NULL DEFAULT 'none',  -- 'none' | 'redis' | 'sdk'
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_eval_org_user ON policy_eval_log(org_id, user_id, evaluated_at DESC);

ALTER TABLE policy_eval_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_eval_log FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_eval_org_isolation ON policy_eval_log
    USING (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT ON policy_eval_log TO openguard_app;
```

Also: standard outbox table.

### 10.2 Redis Caching for Evaluate

**Cache key:**
```
"policy:eval:{org_id}:{sha256(sorted_json(action, resource, user_id, user_groups))}"
```

**Cache value:**
```json
{ "permitted": true, "matched_policies": ["uuid1"], "reason": "RBAC match", "evaluated_at": "..." }
```

**TTL:** `POLICY_CACHE_TTL_SECONDS` (default: 30).

**Cache invalidation on policy change:** The policy service subscribes to `TopicPolicyChanges` (consumer group `GroupPolicy`) and deletes all cached evaluation keys for the affected org. The deletion uses Redis `SCAN` with a per-org key index:

```go
// Correct O(M) cache invalidation — not O(total keyspace)
//
// On every cache SET, also add the key to a Redis Set:
//   SADD "policy:eval:org:{org_id}:keys" "<full_cache_key>"
//   EXPIRE "policy:eval:org:{org_id}:keys" <TTL>
//
// On policy.changes event for org_id:
//   1. SMEMBERS "policy:eval:org:{org_id}:keys"   → get all cached keys for this org
//   2. DEL <each key>                              → O(M) where M = keys for this org
//   3. DEL "policy:eval:org:{org_id}:keys"         → remove the index
//
// This is O(M) for the affected org, not O(total keyspace) like SCAN.
```

### 10.3 Policy Service Architecture

The control plane calls the policy service via mTLS when handling `POST /v1/policy/evaluate` from the SDK. The SDK also maintains a local LRU cache.

**Evaluation flow:**
1. SDK sends `POST /v1/policy/evaluate` to control plane.
2. Control plane's `cb-policy` circuit breaker wraps the call to the policy service.
3. Policy service checks Redis cache first.
4. Cache miss: policy service queries PostgreSQL (RLS-scoped), evaluates RBAC rules, writes result to Redis, logs to `policy_eval_log` via outbox.
5. Control plane returns result to SDK.
6. SDK stores result in local LRU cache with TTL = `SDK_POLICY_CACHE_TTL_SECONDS`.

**Second SDK call with same inputs:** SDK local cache hit. Zero network requests. `cache_hit: "sdk"` in the eval log (SDK sends this flag in the request when it has a local hit and is refreshing in background — optional background refresh pattern).

**Circuit breaker open:**
- Control plane returns `503 POLICY_SERVICE_UNAVAILABLE`.
- SDK uses its local cache if available.
- After SDK cache TTL expires with no successful re-fetch: SDK returns `DenyDecision`.
- The SDK never grants access after cache expiry when it cannot reach the policy service.

### 10.4 Policy Webhook to Connectors

When a policy changes, connected apps with scope `policy:read` receive a signed outbound webhook within 5 seconds. The flow: `policy.changes` Kafka event → audit service consumes → webhook delivery service reads `webhook.delivery` topic → POSTs to connector URL.

### 10.5 Policy Management API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/policies` | List policies |
| `POST` | `/v1/policies` | Create policy |
| `GET` | `/v1/policies/:id` | Get policy |
| `PUT` | `/v1/policies/:id` | Update policy (publishes `policy.changes` via outbox) |
| `DELETE` | `/v1/policies/:id` | Delete policy |
| `POST` | `/v1/policy/evaluate` | Real-time evaluation (SDK entry point) |
| `GET` | `/v1/policy/eval-logs` | Evaluation history |

### 10.6 Phase 2 Acceptance Criteria

- [ ] `POST /v1/policy/evaluate` p99 < 30ms (uncached) under 500 concurrent requests.
- [ ] `POST /v1/policy/evaluate` p99 < 5ms (Redis cached) under 500 concurrent requests.
- [ ] SDK local cache hit: second identical call produces 0 outbound HTTP requests.
- [ ] Policy change → Redis cache invalidated → next evaluate returns fresh result within 1s.
- [ ] Policy change → webhook delivered to connector with `policy:write` scope within 5s.
- [ ] Policy service circuit breaker open → `503` → SDK falls back to local cache → after TTL: SDK denies.
- [ ] `version` increments on policy update; returns correct `ETag` header.
- [ ] `policy_eval_log` records `cache_hit: "redis"` for cache hits, `"none"` for misses.

---

## 11. Phase 3 — Event Bus & Audit Log

**Goal:** Kafka fully operational. Outbox relay running in all services. Audit Log consumes all events with manual-commit consumers, bulk inserts, atomic hash chaining, and CQRS read/write split.

### 11.1 Kafka Topic Configuration

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

Replication factor 3 requires 3 brokers in staging/production. Docker Compose uses single-broker (replication=1) for local dev. `create-topics.sh` detects broker count and adjusts replication factor automatically.

### 11.2 Audit Log Service — CQRS Architecture

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
│   └── export.go           # Export jobs
└── integrity/
    └── verifier.go         # Hash chain verification
```

#### 11.2.1 Kafka Consumer (Manual Offset Commit)

```go
// pkg/consumer/consumer.go
// The audit consumer uses manual offset commit mode.
// An offset is committed ONLY after the MongoDB BulkWrite succeeds.
//
// Flow per batch:
//   1. Poll up to AUDIT_BULK_INSERT_MAX_DOCS messages (or wait AUDIT_BULK_INSERT_FLUSH_MS)
//   2. BulkWriter.AddBatch(docs)
//   3. BulkWriter.Flush() → MongoDB BulkWrite (ordered=false for throughput)
//   4. On success: kafkaConsumer.CommitOffsets()
//   5. On failure: do NOT commit, retry batch up to 5 times, then route to dead-letter collection
//
// Consequence of crash before commit:
//   The batch is reprocessed on restart. The event_id unique index in MongoDB
//   causes duplicate InsertOne operations to fail with a duplicate key error.
//   BulkWrite with ordered=false continues on duplicate key errors, logs them,
//   and does not fail the entire batch. This provides exactly-once semantics
//   in the audit log.
```

#### 11.2.2 Bulk Writer with Correct Flush Semantics

```go
// pkg/consumer/bulk_writer.go
type BulkWriter struct {
    coll       *mongo.Collection  // primary write client
    buffer     []mongo.WriteModel
    mu         sync.Mutex
    maxDocs    int
    flushAfter time.Duration
}

// Add appends a document and flushes if maxDocs reached.
// Does NOT flush automatically on timer — the consumer's Run loop owns the timer.
func (b *BulkWriter) Add(doc AuditEvent) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.buffer = append(b.buffer, mongo.NewInsertOneModel().SetDocument(doc))
}

// Flush writes all buffered documents to MongoDB as a single BulkWrite.
// ordered=false: continues on duplicate key errors (idempotent reprocessing).
// Returns error only for non-duplicate failures.
// Called by the consumer after reaching maxDocs or flushAfter interval.
// The consumer commits Kafka offsets AFTER this function returns nil.
func (b *BulkWriter) Flush(ctx context.Context) error {
    b.mu.Lock()
    if len(b.buffer) == 0 {
        b.mu.Unlock()
        return nil
    }
    docs := b.buffer
    b.buffer = make([]mongo.WriteModel, 0, b.maxDocs)
    b.mu.Unlock()

    opts := options.BulkWrite().SetOrdered(false)
    result, err := b.coll.BulkWrite(ctx, docs, opts)
    if err != nil {
        var bulkErr mongo.BulkWriteException
        if errors.As(err, &bulkErr) {
            // Log individual failures; ignore duplicate key errors (E11000)
            for _, we := range bulkErr.WriteErrors {
                if we.Code != 11000 { // not duplicate key
                    // log genuine failures; return error to prevent offset commit
                    return fmt.Errorf("bulk write non-duplicate error: %w", err)
                }
            }
            // All failures were duplicate keys — safe to commit offsets
            return nil
        }
        return fmt.Errorf("bulk write failed: %w", err)
    }
    _ = result
    return nil
}
```

#### 11.2.3 Atomic Hash Chain (Batched Reservation)

To prevent MongoDB write lock contention on every audit event, the audit service reserves chain sequences in batches of 100 per org.

```go
// pkg/consumer/hash_chain.go
// 1. Consumer gets batch of Kafka messages.
// 2. Increments mongo.audit_counters.last_seq by len(messages) in one atomic update.
// 3. Assigns reserved seqs to messages in-memory.
// 4. Bulk-inserts signed events into mongo.audit_trail.
```
// pkg/consumer/hash_chain.go

// ChainState is stored in a separate MongoDB collection: audit_chain_state
// Document format: { _id: org_id, seq: <int64>, last_hash: "<hex string>" }
//
// Atomic sequence assignment:
//   result = db.audit_chain_state.findOneAndUpdate(
//     { _id: orgID },
//     { $inc: { seq: 1 } },
//     { upsert: true, returnDocument: "after" }
//   )
//   chain_seq = result.seq
//   prev_hash = result.last_hash (before the $inc, captured in a pipeline update)
//
// This serializes chain assignments per org, which is correct for chain integrity.
// For high-throughput orgs (>10k events/s), consider batched chain assignment:
//   reserve a range of seq numbers atomically, then assign to the batch in order.

// ChainHash computes HMAC-SHA256 of concatenated fields.
// Key: AUDIT_HASH_CHAIN_SECRET
// Input: prev_hash + event_id + org_id + type + occurred_at.Unix()
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

#### 11.2.4 MongoDB Schema

Collection: `audit_events`
```js
db.audit_events.createIndex({ org_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ org_id: 1, type: 1, occurred_at: -1 })
db.audit_events.createIndex({ actor_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ event_id: 1 }, { unique: true })  // dedup key
db.audit_events.createIndex({ org_id: 1, chain_seq: 1 })        // integrity checks
db.audit_events.createIndex({ occurred_at: 1 }, { expireAfterSeconds: <retention_seconds> })
```

Collection: `audit_chain_state`
```js
db.audit_chain_state.createIndex({ _id: 1 })  // org_id is _id
```

#### 11.2.5 Audit HTTP API

| Method | Path | Description |
|---|---|---|
| `GET` | `/audit/events` | List events (cursor paginated; reads from secondary) |
| `GET` | `/audit/events/:id` | Get single event |
| `POST` | `/audit/export` | Trigger async CSV/JSON export |
| `GET` | `/audit/export/:job_id` | Poll export job status |
| `GET` | `/audit/export/:job_id/download` | Stream download |
| `GET` | `/audit/integrity` | Verify hash chain for org |
| `GET` | `/audit/stats` | Event counts by type and day |

### 11.3 Phase 3 Acceptance Criteria

- [ ] Kafka consumer processes 50,000 events/s sustained (k6 + producer load test).
- [ ] Bulk writer: each batch ≤ 500 docs, flush interval ≤ 1000ms.
- [ ] Kafka offsets committed only after successful MongoDB BulkWrite.
- [ ] Event from IAM login appears in MongoDB within p99 2s end-to-end.
- [ ] Duplicate `event_id`: second insert skipped (duplicate key error), batch succeeds, offsets committed.
- [ ] Service crash before offset commit: events reprocessed on restart, duplicates silently skipped.
- [ ] `GET /audit/events` uses MongoDB secondary (verified with `explain()`).
- [ ] `GET /audit/integrity` returns `ok: true` on clean chain.
- [ ] Manually deleting a document → `GET /audit/integrity` reports a gap at the missing `chain_seq`.
- [ ] Chain hash breaks are reported in Prometheus `openguard_audit_chain_integrity_failures_total`.
- [ ] Chain sequence assignment: 100 concurrent events for the same org → all have unique, sequential `chain_seq` values.

---

## 12. Phase 4 — Threat Detection & Alerting

**Goal:** Real-time detection via Redis-backed counters. Composite risk scoring. Saga-based alert lifecycle. SIEM payloads signed with HMAC and replay-protected.

### 12.1 Threat Detectors

All detectors consume from `TopicAuthEvents`, `TopicPolicyChanges`, or `TopicConnectorEvents`. Each maintains state in Redis.

| Detector | Signal | Threshold | Risk Score |
|---|---|---|---|
| Brute force | `auth.login.failure` for same `email` within window | `THREAT_MAX_FAILED_LOGINS` in `THREAT_ANOMALY_WINDOW_MINUTES` | 0.8 |
| Impossible travel | `auth.login.success` from IP1 then IP2 with distance > `THREAT_GEO_CHANGE_THRESHOLD_KM` within 1hr | Physical impossibility of travel | 0.9 |
| Off-hours access | `auth.login.success` outside 06:00–22:00 org local time for 3+ consecutive days previously all in-hours | Historical pattern deviation | 0.5 |
| Data exfiltration | `data.access` event count for single user exceeds org baseline by 3σ within 1hr | Statistical anomaly | 0.7 |
| Account takeover (ATO) | `auth.login.success` from new device fingerprint within 24hr of password change | New device + recent credential change | 0.7 |
| Privilege escalation | `policy.changes` with `role.grant` for a user who logged in within 60min | Login → immediate admin grant | 0.9 |

**Composite scoring:** `max(individual_scores)` weighted by recency. Score ≥ 0.5 → alert. Score ≥ 0.8 → HIGH. Score ≥ 0.95 → CRITICAL.

### 12.2 Alert Lifecycle Saga

```
threat.alert.created   →  Step 1: persist alert in MongoDB
                       →  Step 2: enqueue notification (notifications.outbound)
                       →  Step 3: fire SIEM webhook (if configured)
                       →  Step 4: write audit event (audit.trail)
threat.alert.acknowledged → update alert status, write audit event
threat.alert.resolved  → update status, compute MTTR, write audit event
```

All steps must succeed or compensate. MTTR (mean time to resolve) is tracked per org per severity.

### 12.3 SIEM Webhook Signing and Replay Protection

Every SIEM webhook POST includes:
```
X-OpenGuard-Signature: sha256=<hmac-sha256-hex>
X-OpenGuard-Delivery: <uuid>
X-OpenGuard-Timestamp: <unix seconds>
```

HMAC is computed over `"<timestamp>.<payload_bytes>"` using `ALERTING_SIEM_WEBHOOK_HMAC_SECRET`. Replay protection: reject requests where `abs(now - timestamp) > ALERTING_SIEM_REPLAY_TOLERANCE_SECONDS` (default 300s). Receivers must implement the same check.

Outgoing SIEM webhook URLs are validated at startup and on update for SSRF (must be HTTPS, must not resolve to RFC 1918 / loopback addresses).

### 12.4 Threat & Alerting API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/threats/alerts` | List alerts (status, severity filters, cursor paginated) |
| `GET` | `/v1/threats/alerts/:id` | Alert detail + saga step status |
| `POST` | `/v1/threats/alerts/:id/acknowledge` | Mark acknowledged |
| `POST` | `/v1/threats/alerts/:id/resolve` | Mark resolved (computes MTTR) |
| `GET` | `/v1/threats/stats` | Alert counts and MTTR |
| `GET` | `/v1/threats/detectors` | Active detectors and weights |

### 12.5 Phase 4 Acceptance Criteria

- [ ] 11 failed logins within window → HIGH alert in MongoDB within 3s.
- [ ] Privilege escalation detector fires within 5s of role grant event.
- [ ] SIEM webhook includes valid HMAC signature. Receiver can verify.
- [ ] Webhook with timestamp 6 minutes old → rejected (replay protection).
- [ ] Alert saga: all 4 steps produce audit events in `audit.trail`.
- [ ] MTTR computed correctly on resolution.
- [ ] ATO detector fires when login from new device follows password change within 24h.
- [ ] SSRF: SIEM URL `http://169.254.169.254/latest/meta-data/` rejected at configuration time.

---

## 13. Phase 5 — Compliance & Analytics

**Goal:** ClickHouse receives bulk-inserted event stream. Report generation is concurrency-limited via injected Bulkhead. PDF output complete and signed. Analytics queries meet p99 < 100ms.

### 13.1 ClickHouse Schema

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
) ENGINE = MergeTree()
-- Correct multi-tenant partition strategy: time-only partitioning.
-- Do NOT partition by org_id — this creates too many parts for 10k+ orgs
-- and degrades INSERT performance. org_id belongs only in ORDER BY.
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (org_id, type, occurred_at)
TTL occurred_at + INTERVAL 2 YEAR
SETTINGS index_granularity = 8192;

-- Materialized view for dashboard queries (O(1) aggregation, not full scan)
CREATE MATERIALIZED VIEW IF NOT EXISTS event_counts_daily
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (org_id, type, day)
AS SELECT
    org_id,
    type,
    toDate(occurred_at) AS day,
    count() AS cnt
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

### 13.2 ClickHouse Bulk Insertion

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
            event.EventID,
            event.Type,
            event.OrgID,
            event.ActorID,
            event.ActorType,
            event.OccurredAt,
            event.Source,
            string(event.Payload),
        ); err != nil {
            return fmt.Errorf("append to batch: %w", err)
        }
    }
    return batch.Send()
}
```

### 13.3 Report Generation with Injected Bulkhead

The `reportBulkhead` is not a package-level variable. It is constructed in `main.go` and injected:

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

func NewGenerator(ch *clickhouse.Client, mongo *mongo.Client, bulkhead *resilience.Bulkhead) *Generator {
    if bulkhead == nil {
        panic("NewGenerator: bulkhead is required")
    }
    return &Generator{ch: ch, mongo: mongo, bulkhead: bulkhead}
}

func (g *Generator) Generate(ctx context.Context, report *Report) error {
    return g.bulkhead.Execute(ctx, func() error {
        return g.generate(ctx, report)
    })
}
```

When bulkhead is full: `ErrBulkheadFull` → handler maps to `429` with `Retry-After: 30`.

### 13.4 Compliance API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/compliance/reports` | List reports |
| `POST` | `/v1/compliance/reports` | Trigger report (type: gdpr, soc2, hipaa) |
| `GET` | `/v1/compliance/reports/:id` | Status + download link |
| `GET` | `/v1/compliance/stats` | Compliance score and trends |
| `GET` | `/v1/compliance/posture` | Real-time posture vs controls |

### 13.5 Phase 5 Acceptance Criteria

- [ ] ClickHouse receives 10,000 events in ≤ 3 batches of ≤ 5,000 rows.
- [ ] Materialized view `event_counts_daily` populated automatically.
- [ ] `GET /compliance/stats` p99 < 100ms under load.
- [ ] GDPR report: 5 sections, valid PDF with ToC and page numbers.
- [ ] 11 concurrent report requests: 10 succeed, 11th returns 429.
- [ ] Bulkhead is injected via constructor (not package-level var). Verified in unit test.
- [ ] Kafka offsets committed only after successful ClickHouse `batch.Send()`.
- [ ] ClickHouse partition by month only (no `org_id` partition). Verified in schema test.
---

## 14. Phase 6 — Infra, CI/CD & Observability

> Note on ordering: Infrastructure and CI are established as prerequisites in Phase 1 (Section 9.1). This section specifies the full detail of the Docker Compose file, CI pipeline, metrics, and Helm chart — the reference against which Phase 1's prerequisites are built.

### 14.1 Docker Compose

```yaml
# infra/docker/docker-compose.yml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    volumes: [postgres-data:/var/lib/postgresql/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $$POSTGRES_USER"]
      interval: 5s
      timeout: 5s
      retries: 10

  mongo-primary:
    image: mongo:7
    command: mongod --replSet rs0 --bind_ip_all
    volumes: [mongo-primary-data:/data/db]
    healthcheck:
      test: ["CMD", "mongosh", "--eval", "db.adminCommand('ping')"]
      interval: 5s
      retries: 10

  mongo-secondary-1:
    image: mongo:7
    command: mongod --replSet rs0 --bind_ip_all
    volumes: [mongo-secondary1-data:/data/db]
    depends_on: [mongo-primary]

  mongo-secondary-2:
    image: mongo:7
    command: mongod --replSet rs0 --bind_ip_all
    volumes: [mongo-secondary2-data:/data/db]
    depends_on: [mongo-primary]

  mongo-init:
    image: mongo:7
    depends_on:
      mongo-primary: { condition: service_healthy }
    restart: "no"
    command: >
      bash -c "
        mongosh --host mongo-primary --eval \"
          rs.initiate({_id:'rs0',members:[
            {_id:0,host:'mongo-primary:27017',priority:2},
            {_id:1,host:'mongo-secondary-1:27017',priority:1},
            {_id:2,host:'mongo-secondary-2:27017',priority:1}
          ]});
        \"
      "

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes: [redis-data:/data]
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 5s
      retries: 10

  zookeeper:
    image: bitnami/zookeeper:3.9
    environment: [ALLOW_ANONYMOUS_LOGIN=yes]
    volumes: [zookeeper-data:/bitnami/zookeeper]
    healthcheck:
      test: ["CMD-SHELL", "zkServer.sh status"]
      interval: 10s
      retries: 5

  kafka:
    image: bitnami/kafka:3.6
    depends_on:
      zookeeper: { condition: service_healthy }
    environment:
      KAFKA_CFG_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_CFG_NUM_PARTITIONS: 12
      KAFKA_CFG_DEFAULT_REPLICATION_FACTOR: 1  # 1 for dev; create-topics.sh detects and adjusts
      ALLOW_PLAINTEXT_LISTENER: "yes"
      KAFKA_CFG_ENABLE_IDEMPOTENCE: "true"
    volumes: [kafka-data:/bitnami/kafka]
    healthcheck:
      test: ["CMD-SHELL", "kafka-topics.sh --list --bootstrap-server localhost:9092"]
      interval: 10s
      retries: 10

  clickhouse:
    image: clickhouse/clickhouse-server:24
    volumes: [clickhouse-data:/var/lib/clickhouse]
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]
      interval: 5s
      retries: 10

  jaeger:
    image: jaegertracing/all-in-one:latest
    ports: ["16686:16686", "4317:4317"]

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus

  grafana:
    image: grafana/grafana:latest
    volumes:
      - grafana-data:/var/lib/grafana
      - ./monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards
    environment: [GF_SECURITY_ADMIN_PASSWORD=admin]
    ports: ["3001:3000"]

  control-plane:
    build: { context: ../../services/control-plane }
    ports: ["8080:8080"]
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
      kafka: { condition: service_healthy }
    env_file: [../../.env]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health/live"]
      interval: 10s
      retries: 5

  connector-registry:
    build: { context: ../../services/connector-registry }
    ports: ["8090:8090"]
    depends_on:
      postgres: { condition: service_healthy }
    env_file: [../../.env]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health/live"]

  iam:
    build: { context: ../../services/iam }
    ports: ["8081:8081"]
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
    env_file: [../../.env]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8081/health/live"]

  policy:
    build: { context: ../../services/policy }
    ports: ["8082:8082"]
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
    env_file: [../../.env]

  threat:
    build: { context: ../../services/threat }
    ports: ["8083:8083"]
    depends_on:
      kafka: { condition: service_healthy }
    env_file: [../../.env]

  audit:
    build: { context: ../../services/audit }
    ports: ["8084:8084"]
    depends_on:
      kafka: { condition: service_healthy }
      mongo-primary: { condition: service_healthy }
    env_file: [../../.env]

  alerting:
    build: { context: ../../services/alerting }
    ports: ["8085:8085"]
    depends_on:
      kafka: { condition: service_healthy }
    env_file: [../../.env]

  webhook-delivery:
    build: { context: ../../services/webhook-delivery }
    ports: ["8091:8091"]
    depends_on:
      kafka: { condition: service_healthy }
      postgres: { condition: service_healthy }
    env_file: [../../.env]

  compliance:
    build: { context: ../../services/compliance }
    ports: ["8086:8086"]
    depends_on:
      clickhouse: { condition: service_healthy }
      kafka: { condition: service_healthy }
    env_file: [../../.env]

  dlp:
    build: { context: ../../services/dlp }
    ports: ["8087:8087"]
    depends_on:
      postgres: { condition: service_healthy }
      kafka: { condition: service_healthy }
    env_file: [../../.env]

  web:
    build: { context: ../../web }
    ports: ["3000:3000"]
    depends_on:
      control-plane: { condition: service_healthy }
      iam: { condition: service_healthy }
    env_file: [../../.env]

volumes:
  postgres-data: {}
  mongo-primary-data: {}
  mongo-secondary-data: {}
  redis-data: {}
  zookeeper-data: {}
  kafka-data: {}
  clickhouse-data: {}
  prometheus-data: {}
  grafana-data: {}
```

### 14.2 GitHub Actions CI

```yaml
# .github/workflows/ci.yml
name: CI
on:
  pull_request:
  push:
    branches: [main]

jobs:
  go-test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env: { POSTGRES_PASSWORD: test, POSTGRES_DB: openguard_test }
        options: --health-cmd pg_isready --health-interval 5s --health-retries 10
      redis:
        image: redis:7-alpine
        options: --health-cmd "redis-cli ping" --health-interval 5s
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22', cache: true }
      - run: go work sync
      - run: go test ./... -race -coverprofile=coverage.out -covermode=atomic -timeout 5m
      - run: go vet ./...
      - name: Coverage gate (70% floor per package)
        run: |
          go tool cover -func=coverage.out | awk '
            /^total:/ { next }
            { split($3, a, "%"); if (a[1]+0 < 70) { print "FAIL: " $1 " coverage " $3 " < 70%"; fail=1 } }
            END { exit fail }
          '

  go-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v4
        with: { version: latest, args: --timeout 5m }

  sql-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          go install github.com/ryanprior/go-sqllint@latest
          find services shared -name "*.go" | xargs go-sqllint
          # Fails on string concatenation in SQL queries

  next-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: web/package-lock.json }
      - run: cd web && npm ci && npm run build && npm run lint

  contract-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go test ./shared/... -run TestContract -v
        # Verifies: EventEnvelope produced by IAM is parseable by audit consumer
        # Verifies: Policy evaluate request/response schema
        # Verifies: SCIM response envelope matches RFC 7643 schema

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
      - uses: aquasecurity/trivy-action@master
        with: { scan-type: fs, severity: CRITICAL,HIGH, exit-code: 1 }
      - run: go mod verify  # fails if go.sum is not up to date
```

### 14.3 Prometheus Metrics

| Metric | Type | Labels |
|---|---|---|
| `openguard_outbox_pending_records` | Gauge | `service` |
| `openguard_outbox_relay_duration_seconds` | Histogram | `service`, `result` |
| `openguard_circuit_breaker_state` | Gauge | `name`, `state` (0=closed, 1=half-open, 2=open) |
| `openguard_rls_session_set_duration_seconds` | Histogram | `service` |
| `openguard_kafka_bulk_insert_size` | Histogram | `service` |
| `openguard_kafka_consumer_lag` | Gauge | `topic`, `group` |
| `openguard_kafka_offset_commit_duration_seconds` | Histogram | `topic`, `group` |
| `openguard_audit_chain_integrity_failures_total` | Counter | `org_id` |
| `openguard_policy_cache_hits_total` | Counter | `layer` (`sdk`\|`redis`) |
| `openguard_policy_cache_misses_total` | Counter | `layer` |
| `openguard_threat_detections_total` | Counter | `detector`, `severity` |
| `openguard_report_generation_duration_seconds` | Histogram | `type`, `format` |
| `openguard_report_bulkhead_rejected_total` | Counter | — |
| `openguard_connector_auth_total` | Counter | `result` (`ok`\|`invalid`\|`suspended`\|`cache_hit`) |
| `openguard_events_ingested_total` | Counter | `connector_id` |
| `openguard_webhook_delivery_duration_seconds` | Histogram | `result` |
| `openguard_webhook_delivery_attempts_total` | Counter | `result` |
| `openguard_webhook_dlq_total` | Counter | — |
| `openguard_dlp_scan_duration_seconds` | Histogram | `mode` (`sync`\|`async`) |
| `openguard_dlp_findings_total` | Counter | `type` (`pii`\|`credential`\|`financial`) |

### 14.4 Alertmanager Rules

```yaml
# infra/monitoring/alerts/openguard.yml
groups:
- name: openguard
  rules:
  - alert: OutboxLagHigh
    expr: openguard_outbox_pending_records > 1000
    for: 2m
    labels: { severity: warning }
    annotations:
      summary: "Outbox relay lagging ({{ $value }} pending records in {{ $labels.service }})"
      runbook: "docs/runbooks/outbox-dlq.md"

  - alert: CircuitBreakerOpen
    expr: openguard_circuit_breaker_state{state="2"} == 1
    for: 30s
    labels: { severity: critical }
    annotations:
      summary: "Circuit breaker {{ $labels.name }} is open"
      runbook: "docs/runbooks/circuit-breaker-open.md"

  - alert: KafkaConsumerLagHigh
    expr: openguard_kafka_consumer_lag > 50000
    for: 5m
    labels: { severity: warning }
    annotations:
      runbook: "docs/runbooks/kafka-consumer-lag.md"

  - alert: AuditChainIntegrityFailure
    expr: increase(openguard_audit_chain_integrity_failures_total[5m]) > 0
    labels: { severity: critical }
    annotations:
      summary: "Audit chain integrity violation for org {{ $labels.org_id }}"
      runbook: "docs/runbooks/audit-hash-mismatch.md"

  - alert: PolicyServiceDown
    expr: up{job="policy"} == 0
    for: 30s
    labels: { severity: critical }
    annotations:
      summary: "Policy service down — all evaluations failing closed after SDK cache TTL"

  - alert: KafkaOffsetCommitLag
    expr: histogram_quantile(0.99, openguard_kafka_offset_commit_duration_seconds_bucket) > 5
    for: 2m
    labels: { severity: warning }
    annotations:
      summary: "Kafka offset commits are slow (p99 {{ $value }}s) — potential consumer stall"
```

### 14.5 Helm Chart

`infra/k8s/helm/openguard/` with:
- `Deployment` per service with `minReadySeconds: 30` and `RollingUpdate` strategy.
- `PodDisruptionBudget` per service: `minAvailable: 1`.
- `HorizontalPodAutoscaler` for `control-plane`, `iam`, `policy`, `audit`: scale on CPU 70% and `openguard_kafka_consumer_lag`.
- `NetworkPolicy`:
  - Internal services (`iam`, `policy`, `audit`, `threat`, `alerting`, `compliance`, `dlp`) accept inbound only from `control-plane` (mTLS).
  - `connector-registry` accepts inbound only from `control-plane`.
  - `control-plane` has an externally reachable `LoadBalancer` Service.
  - IAM's OIDC endpoints (`/oauth/*`) have a separate `Ingress` (public TLS, no client cert) for browser and OIDC flows.
- `ServiceAccount` per service with least-privilege RBAC.
- `Secret` references via `external-secrets.io` for production; plain secrets for dev.
- `topologySpreadConstraints`: spread pods across 3 AZs.

### 14.6 Connected Apps Admin UI (`/connectors`)

**Page: `/connectors`** — list view:
- Table: name, status badge, scopes, created date, last event timestamp, event volume (30d).
- "Register app" button → registration modal.
- Per-row actions: view, suspend, activate, delete.

**Registration modal:**
- Fields: App name, Webhook URL, Scopes (multi-select).
- On success: API key displayed in a one-time reveal panel (masked by default, click to reveal, copy button). Prominent warning: "This key will not be shown again."

**Detail page (`/connectors/:id`):**
- Metadata and status.
- Edit webhook URL and scopes.
- Webhook delivery log: last 100 deliveries with timestamp, event type, HTTP status, latency, retry count.
- Event volume chart: events/day for last 30 days (from ClickHouse `event_counts_daily`).
- "Send test webhook" button.
- Danger zone: suspend / delete.

### 14.7 Phase 6 Acceptance Criteria

- [ ] `docker compose up` starts all services healthy with MongoDB replica set initialized.
- [ ] MongoDB init service: if primary not ready on first attempt, retries until healthy (tested by adding `sleep 10` delay to primary startup).
- [ ] `go test ./... -race` passes in CI. Coverage gate enforced per package.
- [ ] `govulncheck ./...` reports no CRITICAL vulnerabilities.
- [ ] SQL lint catches deliberately injected string concatenation (test file).
- [ ] Contract test: IAM `EventEnvelope` is parseable by audit consumer.
- [ ] All 11 services scraped by Prometheus. All `openguard_*` metrics appear in Grafana.
- [ ] `OutboxLagHigh` alert fires when relay is stopped for 2+ minutes.
- [ ] `CircuitBreakerOpen` alert fires when policy service is killed.
- [ ] `helm lint` and `helm template` pass without warnings.
- [ ] Connected app registration UI flow end-to-end: register → copy key → authenticate → verify in delivery log.

---

## 15. Phase 7 — Security Hardening & Secret Rotation

### 15.1 HTTP Security Headers

```go
// shared/middleware/security.go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Content-Security-Policy", "default-src 'none'")
        w.Header().Set("Referrer-Policy", "no-referrer")
        w.Header().Set("X-Request-ID", generateRequestID())
        next.ServeHTTP(w, r)
    })
}
```

Applied to every service router.

### 15.2 SSRF Protection

All outgoing webhook URLs (SIEM, connector webhook) are validated at configuration time (startup and on PATCH):

```go
func validateWebhookURL(raw string) error {
    u, err := url.Parse(raw)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }
    if u.Scheme != "https" {
        return errors.New("webhook URL must use HTTPS")
    }
    ips, err := net.LookupHost(u.Hostname())
    if err != nil {
        return fmt.Errorf("DNS resolution failed: %w", err)
    }
    for _, ip := range ips {
        parsed := net.ParseIP(ip)
        if parsed == nil {
            continue
        }
        if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() ||
            parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() {
            return fmt.Errorf("webhook URL resolves to restricted IP %s (SSRF blocked)", ip)
        }
    }
    return nil
}
```

### 15.3 Safe Logger

```go
// shared/telemetry/logger.go

// sensitiveKeys: read-only, initialized once, never mutated (named exception per §0.2)
var sensitiveKeys = []string{
    "password", "secret", "token", "key", "auth", "credential",
    "private", "bearer", "authorization", "cookie", "session",
}

func SafeAttr(key string, value any) slog.Attr {
    keyLower := strings.ToLower(key)
    for _, s := range sensitiveKeys {
        if strings.Contains(keyLower, s) {
            return slog.String(key, "[REDACTED]")
        }
    }
    return slog.Any(key, value)
}
```

### 15.4 Secret Rotation Runbooks

Document in `docs/runbooks/secret-rotation.md`:

**JWT key rotation (zero-downtime):**
1. `scripts/rotate-jwt-keys.sh new` — generates new key, outputs JSON snippet.
2. Update `IAM_JWT_KEYS_JSON`: add new key as `active`, set old to `verify_only`.
3. Rolling deploy IAM. New tokens signed with new key; old tokens still verify.
4. Wait `IAM_JWT_EXPIRY_SECONDS` seconds.
5. Remove old key from JSON. Rolling deploy IAM.

**MFA encryption key rotation (zero-downtime):**
1. Add new key to `IAM_MFA_ENCRYPTION_KEY_JSON` as `active`, old as `verify_only`.
2. Deploy IAM.
3. Run `scripts/re-encrypt-mfa.sh` — reads all `mfa_configs`, decrypts with old key, re-encrypts with new key. Batches of 100, `time.Sleep(50ms)` between batches (operational script, named exception per §0.13). Progress logged to stdout.
4. Remove old key. Deploy IAM.

**Connector API key rotation (with maintenance window):**
1. Call `DELETE /v1/admin/connectors/:id/api-key` — invalidates existing key (Redis cache cleared immediately).
2. Call `POST /v1/admin/connectors/:id/api-key` — issues new key.
3. Update connected app's configuration.
4. Verify: `GET /v1/admin/connectors/:id` → status `active`.
5. Note: no dual-key support for connector API keys. Schedule during maintenance window or coordinate with connector operator.

**mTLS certificate rotation:** See Section 2.9.

**Kafka SASL credential rotation:**
1. Add new credential to Kafka ACLs.
2. Update `KAFKA_SASL_PASSWORD`. Rolling deploy all services.
3. Remove old credential.

### 15.5 Idempotency Key Constraints

Idempotency keys (Section 19.5) have these constraints:
- Maximum replay cache entry size: 64KB. Entries larger than 64KB are not cached (the request proceeds but is not idempotent).
- List endpoints (`GET *`) and export download endpoints are excluded from idempotency key support.
- Redis key: `"idempotent:{service}:{idempotency_key}"`, TTL 24 hours.

### 15.6 Phase 7 Acceptance Criteria

- [ ] Security headers on every response from every service (verified with `curl -I`).
- [ ] SSRF: connector webhook URL `http://localhost/internal` rejected at registration.
- [ ] SSRF: SIEM URL `http://169.254.169.254/latest/meta-data/` rejected at startup.
- [ ] Safe logger: log entry containing `password=secret123` → value appears as `[REDACTED]`.
- [ ] JWT rotation runbook executed end-to-end: old tokens rejected after rotation.
- [ ] MFA re-encryption: TOTP codes valid before and after re-encryption.
- [ ] `go mod verify` passes in CI.
- [ ] `govulncheck ./...` and `npm audit --audit-level=high` report zero issues.
- [ ] Idempotency: POST with same `Idempotency-Key` twice → second response is identical to first with `Idempotency-Replayed: true` header.
- [ ] Idempotency replay cache entry > 64KB is not cached (next request re-executes).

---

## 16. Phase 8 — Load Testing & Performance Tuning

### 16.1 k6 Test Scripts

**`auth.js`** — OIDC token endpoint throughput:
```js
export const options = {
    stages: [
        { duration: '1m', target: 500 },
        { duration: '3m', target: 2000 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        'http_req_duration': ['p(99)<150'],
        'http_req_failed': ['rate<0.01'],
    },
};
// POST /oauth/token with grant_type=password
// Pre-seeded users: 10k users across 100 orgs
```

**`policy-evaluate.js`** — policy evaluation (most critical SLO):
```js
// Two scenarios:
// Scenario 1: repeated inputs → Redis cache hits → p99 < 5ms
// Scenario 2: unique resource per VU → cache misses → p99 < 30ms
// Total: 10,000 req/s
// SDK local cache verification: inject Jaeger trace checker to assert
//   second call from same VU produces no new spans to policy service
```

**`event-ingest.js`** — event push throughput:
```js
// POST /v1/events/ingest with batch of 10 events per request
// 2,000 req/s = 20,000 events/s
// p99 < 50ms
// Post-run: verify all events in audit log within 5s (separate verification script)
```

**`audit-query.js`** — read path:
```js
// GET /audit/events with various filter combinations
// 1,000 req/s
// p99 < 100ms
// Verify MongoDB readPreference=secondaryPreferred via explain()
```

**`kafka-throughput.js`** — event bus capacity:
```js
// xk6-kafka extension; direct Kafka producer
// 50,000 events/s to audit.trail
// Monitor: openguard_kafka_consumer_lag must stay < 10,000
```

### 16.2 Tuning Table

| SLO failing | Probable cause | Action |
|---|---|---|
| Login p99 > 150ms | bcrypt CPU-bound under load | Add IAM replicas; bcrypt cannot be parallelized per request |
| Policy p99 > 30ms (uncached) | Cold DB query | Ensure indexes on `policies(org_id, resource, action)` |
| Policy p99 > 5ms (cached) | Redis latency | Check Redis memory; tune `REDIS_POOL_SIZE` |
| Event ingest p99 > 50ms | Outbox write contention | Increase control-plane replicas; tune `POSTGRES_POOL_MAX_CONNS` |
| Audit query p99 > 100ms | Missing MongoDB index | Run `explain()`, add compound index |
| Kafka consumer lag growing | Bulk writer too slow | Increase `AUDIT_BULK_INSERT_MAX_DOCS`; ensure MongoDB write concern is `w:1` (not majority) for audit |
| Connector auth p99 > 5ms (cached) | Redis pool exhausted | Increase `REDIS_POOL_SIZE` |
| Webhook delivery backlog | Delivery service under-scaled | Increase `webhook-delivery` replicas |
| MongoDB OOM | Bulk write buffer too large | Reduce `AUDIT_BULK_INSERT_MAX_DOCS`; tune MongoDB `wiredTiger.engineConfig.cacheSizeGB` |

### 16.3 Phase 8 Acceptance Criteria

- [ ] `auth.js`: p99 < 150ms at 2,000 req/s, error rate < 1%.
- [ ] `policy-evaluate.js`: p99 < 5ms (Redis cached), p99 < 30ms (uncached) at 10,000 req/s.
- [ ] SDK local cache: second call produces 0 spans to policy service (Jaeger verification).
- [ ] `event-ingest.js`: p99 < 50ms at 20,000 req/s. All events in audit within 5s.
- [ ] `audit-query.js`: p99 < 100ms at 1,000 req/s.
- [ ] Kafka consumer lag < 10,000 during 50,000 events/s burst.
- [ ] Connector auth p99 < 5ms (Redis cached) at 20,000 req/s.
- [ ] All k6 HTML reports committed to `loadtest/results/`.
- [ ] Grafana screenshots showing all SLOs met under load committed to `docs/`.

---

## 17. Phase 9 — Documentation & Runbooks

### 17.1 Required Documents

**`README.md`** must contain:
- One-sentence description.
- Architecture diagram (Mermaid) showing control plane model: connected apps calling OpenGuard, not traffic flowing through it.
- SLO table from Section 1.2.
- Quick start: `git clone`, `cp .env.example .env`, `make dev` — working in < 5 minutes on a clean machine.
- License and contributing links.

**`docs/architecture.md`** must contain:
- C4 Level 2 component diagram (Mermaid) showing control plane, connector registry, IAM OIDC IdP, SDK as distinct components.
- Connector registration and API key authentication flow (including Redis cache path).
- Event ingest flow (internal outbox path and connected app push path).
- Transactional Outbox flow.
- Outbound webhook delivery flow.
- RLS enforcement flow (including OrgPool wrapper).
- Circuit breaker state machine.
- SDK cache layering (local LRU → Redis → DB).
- Saga choreography (user provisioning + compensation).
- MongoDB hash chain integrity model.
- Database ER diagram for each service (Mermaid erDiagram).

**`docs/contributing.md`** must contain:
- Local dev setup.
- Adding a new Kafka consumer (manual commit requirements).
- Adding a new threat detector (template).
- Adding a new compliance report type.
- Adding a new RLS-protected table (checklist: `org_id UUID NOT NULL`, RLS policy, `BYPASSRLS` for outbox role, app role grants).
- Adding a new control plane route (scope, middleware chain, circuit breaker, OpenAPI spec update).
- PR requirements: tests, lint, contract test if schema changes.

**OpenAPI specs** (`docs/api/<service>.openapi.json`) for all services, valid OpenAPI 3.1, passing `redocly lint`. Includes `control-plane.openapi.json` with SCIM endpoints documented separately from connector API endpoints.

### 17.2 Operational Runbooks

| File | Scenario |
|---|---|
| `kafka-consumer-lag.md` | Consumer lag > 50k. Check bulk writer, scale consumers, check MongoDB write saturation. |
| `circuit-breaker-open.md` | Breaker fired. Identify failing service, check health endpoints, manual reset procedure. |
| `audit-hash-mismatch.md` | Integrity check fails. Identify affected org, time range, gap analysis, escalation. |
| `secret-rotation.md` | Full rotation for: JWT keys, MFA keys, connector API keys, webhook secrets, Kafka SASL, mTLS certs. |
| `outbox-dlq.md` | Messages in `outbox.dlq`. Inspect, replay, investigate root cause. |
| `postgres-rls-bypass.md` | Cross-tenant data returned. Incident response. Verify RLS policies. |
| `load-shedding.md` | Extreme load. Increase rate limits, scale services, shed non-critical consumers. |
| `connector-suspension.md` | Suspend misbehaving connector. `PATCH /v1/admin/connectors/:id`, verify 401, investigate event log. |
| `webhook-delivery-failure.md` | Connector not receiving webhooks. Check delivery log, DLQ, verify URL reachable. |
| `ca-rotation.md` | Rotate the mTLS CA. Dual-CA trust period. Rehearse in staging first. |

### 17.3 Phase 9 Acceptance Criteria

- [ ] `make dev` works on a clean machine following only `README.md`.
- [ ] All OpenAPI specs pass `redocly lint`.
- [ ] Architecture Mermaid diagrams render in GitHub Markdown.
- [ ] All 10 runbooks present.
- [ ] Following `docs/contributing.md`: adding a new detector produces a passing test.
- [ ] Following `docs/contributing.md`: adding a new control plane route produces correct scope enforcement.

---

## 18. Phase 10 — Content Scanning & DLP

**Goal:** Detect and mitigate sensitive data leakage in real-time. Scan latency p99 < 50ms for sync mode (per-org opt-in). Default mode is async.

### 18.1 Database Schema

**001_create_dlp_policies.up.sql**
```sql
CREATE TABLE dlp_policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    name         TEXT NOT NULL,
    rules        JSONB NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    mode         TEXT NOT NULL DEFAULT 'monitor',  -- 'monitor' | 'block'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlp_policies_org ON dlp_policies(org_id) WHERE enabled = TRUE;

ALTER TABLE dlp_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE dlp_policies FORCE ROW LEVEL SECURITY;
CREATE POLICY dlp_policies_org_isolation ON dlp_policies
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON dlp_policies TO openguard_app;
```

**002_create_dlp_findings.up.sql**
```sql
CREATE TABLE dlp_findings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    event_id      UUID NOT NULL,
    rule_id       UUID REFERENCES dlp_policies(id),
    finding_type  TEXT NOT NULL,    -- 'pii' | 'credential' | 'financial'
    finding_kind  TEXT NOT NULL,    -- 'email' | 'ssn' | 'credit_card' | 'api_key' | 'high_entropy'
    json_path     TEXT NOT NULL,    -- JSONPath to the matched field (for masking)
    action_taken  TEXT NOT NULL,    -- 'monitor' | 'mask' | 'block'
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlp_findings_event  ON dlp_findings(event_id);
CREATE INDEX idx_dlp_findings_org    ON dlp_findings(org_id, occurred_at DESC);

ALTER TABLE dlp_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE dlp_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY dlp_findings_org_isolation ON dlp_findings
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

GRANT SELECT, INSERT ON dlp_findings TO openguard_app;
```

### 18.2 Scanning Engine

**Tier 1 — Regex (PII and Financial):**

| Finding kind | Pattern | Validation |
|---|---|---|
| `email` | RFC 5322 simplified | None |
| `ssn` | `\b\d{3}-\d{2}-\d{4}\b` | None |
| `credit_card` | Visa/MC/Amex patterns | Luhn check |
| `phone_us` | `\b\+?1?[-.\s]?\(?\d{3}\)?[-.\s]\d{3}[-.\s]\d{4}\b` | None |

**Tier 2 — Entropy (Credentials):**

```go
// High-entropy string detection
func shannonEntropy(s string) float64 {
    if len(s) == 0 {
        return 0
    }
    freq := make(map[rune]int)
    for _, c := range s {
        freq[c]++
    }
    entropy := 0.0
    for _, count := range freq {
        p := float64(count) / float64(len(s))
        entropy -= p * math.Log2(p)
    }
    return entropy
}

// A string is flagged as a credential if:
//   len(s) >= DLP_MIN_CREDENTIAL_LENGTH (24) AND
//   shannonEntropy(s) >= DLP_ENTROPY_THRESHOLD (4.5) AND
//   not in common false-positive list (UUIDs, base64 of low-entropy data)
```

**Known prefixes (immediate credential flag regardless of entropy):**
`sk_live_`, `sk_test_`, `AIza`, `AKIA`, `ghp_`, `github_pat_`, `xoxb-`, `xoxp-`

### 18.3 Integration Flow

```
Default (dlp_mode=monitor):
  Connected app → POST /v1/events/ingest → accepted immediately
  → Outbox relay → Kafka (connector.events, audit.trail)
  → DLP service consumes connector.events ASYNC
  → Finds PII → dlp.finding.created event → audit service masks field in MongoDB

Opt-in (dlp_mode=block, per-org policy):
  Connected app → POST /v1/events/ingest
  → Control Plane: org has dlp_mode=block? YES
  → Sync call to DLP service (mTLS, cb-dlp, DLP_SYNC_BLOCK_TIMEOUT_MS=30ms)
  → DLP: finds credit card → returns Block decision
  → Control Plane: 422 DLP_POLICY_VIOLATION, event NOT written to outbox
  → DLP service unavailable (cb-dlp open): reject event (fail closed)

Masking flow (monitor mode finding):
  DLP service → consumes connector.events
  → Finds SSN at json_path "$.payload.form_data.social_security"
  → Writes dlp_finding record (PostgreSQL, RLS-scoped)
  → Publishes dlp.finding.created (via outbox) to audit.trail
  → Audit service: consumes dlp.finding.created
  → Updates audit_events document: replaces matched value with "[REDACTED:ssn]"
    using event_id + json_path from the finding
```

### 18.4 DLP API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/dlp/policies` | List DLP policies |
| `POST` | `/v1/dlp/policies` | Create DLP policy |
| `GET` | `/v1/dlp/policies/:id` | Get policy |
| `PUT` | `/v1/dlp/policies/:id` | Update policy |
| `DELETE` | `/v1/dlp/policies/:id` | Delete policy |
| `GET` | `/v1/dlp/findings` | List findings (cursor paginated) |
| `GET` | `/v1/dlp/findings/:id` | Finding detail + json_path |
| `GET` | `/v1/dlp/stats` | Finding counts by type |

### 18.5 Phase 10 Acceptance Criteria

- [ ] Regex scanner identifies email and SSN in JSON payloads.
- [ ] Luhn scanner identifies valid Visa credit card numbers; ignores random digit strings.
- [ ] Entropy scanner detects `AKIAIOSFODNN7EXAMPLE` (AWS access key) correctly.
- [ ] Sync block (`dlp_mode=block`): `POST /v1/events/ingest` with cleartext credit card → `422 DLP_POLICY_VIOLATION`.
- [ ] Sync block with DLP service down → request rejected (`503 DLP_UNAVAILABLE` for blocking orgs).
- [ ] Monitor mode: event accepted → SSN detected → audit log field masked within 5s.
- [ ] DLP finding auto-creates HIGH threat alert for `credential` finding type.
- [ ] `openguard_dlp_findings_total` metric incremented per finding.

---

## 19. Cross-Cutting Concerns

### 19.1 Structured Logging — Mandatory Fields

All services use `log/slog` with JSON output in non-dev environments. These fields are injected by the logger middleware and `slog.With` base attributes. Individual callsites do not repeat them.

| Field | Source |
|---|---|
| `service` | Hardcoded service name constant |
| `env` | `APP_ENV` |
| `level` | Log level |
| `msg` | Human-readable message |
| `trace_id` | OTel W3C trace ID from context |
| `span_id` | OTel span ID from context |
| `request_id` | `X-Request-ID` header |
| `org_id` | RLS context (omit for system operations) |
| `user_id` | JWT claim (omit for unauthenticated) |
| `duration_ms` | `time.Since(start).Milliseconds()` on completion |

Use `SafeAttr` (Section 15.3) for all attributes whose key might contain sensitive keywords.

Log at the handler layer only. Service and repository layers return errors.

### 19.2 Distributed Tracing

Every service initializes OpenTelemetry on startup. Traces propagate via W3C `traceparent` header. The Outbox relay injects `trace_id` from the parent context into the `EventEnvelope.TraceID` field, so a trace spans from the original HTTP request through to the audit event in MongoDB.

Sampling: `OTEL_SAMPLING_RATE` (0.1 in production, 1.0 in development).

### 19.3 Graceful Shutdown (30-second window)

```go
// main.go pattern — every service
func main() {
    // ... setup ...

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Shutdown order:
    // 1. Stop accepting new HTTP requests
    // 2. Stop Kafka consumers (no new messages)
    // 3. Flush Outbox relay (publish buffered records)
    // 4. Flush bulk writers (MongoDB, ClickHouse)
    // 5. Close DB connections
    _ = server.Shutdown(ctx)
    kafkaConsumer.Close()
    outboxRelay.Flush(ctx)
    bulkWriter.Flush(ctx)
    dbPool.Close()
    if mongoClient != nil {
        _ = mongoClient.Disconnect(ctx)
    }
}
```

Kubernetes `terminationGracePeriodSeconds` must be set to 45 seconds (30s for the app + 15s buffer). The Helm chart enforces this.

### 19.4 Health Checks

Every service exposes:
- `GET /health/live` — returns `200 {"status":"ok"}` immediately. Kubernetes liveness probe.
- `GET /health/ready` — checks PostgreSQL (ping), MongoDB (ping), Redis (ping), Kafka (metadata fetch). Returns `200` only if all dependencies pass. Returns `503 {"status":"not_ready","checks":{"postgres":"ok","mongo":"fail",...}}`. Kubernetes readiness probe.

Readiness check failures cause the pod to be removed from the load balancer (via `readinessProbe.failureThreshold`), triggering circuit breaker state changes in calling services.

### 19.5 Idempotency

`POST` endpoints that create resources accept an `Idempotency-Key: <UUID>` header. Cached in Redis for 24h:
- Key: `"idempotent:{service}:{idempotency_key}"`
- Value: response status + body (max 64KB; not cached if larger)
- On duplicate: return cached response with `Idempotency-Replayed: true` header

Excluded endpoints: list/GET endpoints, export download endpoints.

### 19.6 Request Validation

Use `github.com/go-playground/validator/v10`. Every handler binds request body to a typed struct and calls `validate.Struct()` before passing to the service layer.

Validation error response:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "fields": [
      { "field": "email", "message": "must be a valid email address" },
      { "field": "password", "message": "must be at least 12 characters" }
    ]
  }
}
```

### 19.7 Testing Standards

| Layer | Tool | Requirement |
|---|---|---|
| Unit tests | `testing` + `testify` | ≥ 70% per package; deterministic; no `time.Sleep` |
| Integration tests | `testcontainers-go` | PostgreSQL + Redis + MongoDB real containers per service |
| Contract tests | Custom in `shared/` | Producer → consumer schema compatibility |
| API tests | `net/http/httptest` | Happy paths + key error paths |
| Load tests | k6 | All SLOs from Section 1.2 |
| Chaos tests (Phase 8+) | `toxiproxy` | Circuit breaker and outbox behavior under partition |

Mandatory CI flags:
```bash
go test ./... -race -count=1 -coverprofile=coverage.out -covermode=atomic -timeout 5m
```

---

## 20. Full-System Acceptance Criteria

The following end-to-end scenario must execute without manual intervention. Run as a CI job on every release candidate.

```
1.  docker compose up -d                                  → all services healthy
2.  POST /auth/register                                   → org "Acme" + admin user; single transaction
3.  POST /oauth/token (IAM OIDC, password grant)          → access_token + refresh_token; kid in JWT header
4.  POST /v1/admin/connectors (admin JWT)                 → connector "AcmeApp" with scopes [policy:evaluate, events:write]
                                                            → one-time API key returned (Prefix/Secret scheme)
5.  POST /v1/admin/connectors (second, scim:read only)     → connector "AcmeApp2"
6.  POST /v1/policies (admin JWT)                         → IP allowlist policy created
7.  POST /v1/policy/evaluate (AcmeApp key)                → blocked IP: permitted:false; cache_hit:none
8.  POST /v1/policy/evaluate (same inputs, AcmeApp key)   → permitted:false; cache_hit:redis
9.  POST /v1/policy/evaluate (AcmeApp2 key)               → 403 INSUFFICIENT_SCOPE
10. POST /v1/events/ingest (AcmeApp, 50 events)           → 200; 50 outbox records in one transaction
                                                            → all 50 in GET /audit/events within 5s
                                                            → EventSource="connector:<id>" on each
11. Simulate 11 failed login events via POST /v1/events/ingest
                                                          → HIGH alert in MongoDB within 5s
12. GET /v1/threats/alerts                                → alert visible; severity=high
13. Verify SIEM webhook mock received payload             → HMAC signature valid
14. GET /audit/events                                     → all events from steps 2-11 present
15. GET /audit/integrity                                  → ok:true; no chain gaps
16. POST /compliance/reports {type:"gdpr"}                → report job created
17. Poll GET /compliance/reports/:id                      → status=completed within 60s
18. GET /compliance/reports/:id/download                  → valid PDF; all 5 GDPR sections present
19. POST /v1/events/ingest (event containing SSN field, AcmeApp, dlp_mode=monitor org)
                                                          → 200; event accepted
                                                          → audit log field masked within 5s
20. PATCH /v1/admin/connectors/:id2 {status:"suspended"}  → AcmeApp2 suspended
                                                            → connector cache invalidated immediately
21. POST /v1/events/ingest (AcmeApp2 key)                 → 403 INSUFFICIENT_SCOPE (after cache miss)
22. POST /v1/admin/connectors/:id/test                    → test webhook delivered; HMAC valid
23. GET /v1/admin/connectors/:id/deliveries               → delivery log shows test + policy-change webhooks
24. POST /auth/refresh (valid refresh token)              → new token issued; old token invalid after grace window
25. POST /auth/refresh (same client, high-risk UA change) → 401 SESSION_REVOKED_RISK
26. JWT key rotation: add new key → deploy IAM            → old tokens still verify
27. JWT key rotation: remove old key → deploy IAM         → old tokens return 401
28. Kill policy service                                   → SDK falls back to local cache (60s)
29. After TTL: SDK /v1/policy/evaluate returns DenyDecision
30. Restart policy service                                → circuit breaker resets; evaluate succeeds
31. Kill Kafka                                            → POST /v1/events/ingest succeeds; outbox pending
32. Restart Kafka                                         → outbox records published within 30s
33. Crash audit consumer before offset commit             → on restart: events reprocessed;
34. MongoDB duplicate key errors skipped                  → audit log has no duplicate event_ids
35. go test ./... -race                                   → all tests pass
36. k6 run loadtest/auth.js                               → p99 < 150ms at 2,000 req/s
37. k6 run loadtest/policy-evaluate.js                    → p99 < 5ms (cached); p99 < 30ms (uncached)
38. SDK local cache: second call produces 0 spans         → verified via Jaeger
39. docker compose down                                   → clean shutdown; no data loss
```

Every step is a CI assertion. The release pipeline does not publish unless all 39 steps pass.

---

## Appendix A — Known Trade-offs

This section documents explicit design decisions where a trade-off was made, so future engineers understand why the current design was chosen.

| Decision | Alternatives | Reason |
|---|---|---|
| Connector auth via Redis cache | DB lookup per request | 20,000 req/s event ingest would require 20,000 DB lookups/s. Cache reduces to ~50 DB lookups/s (cache miss rate ~0.25%) at 30s TTL. |
| Per-org key index for policy cache invalidation | Redis SCAN on pattern | SCAN on 5M+ keys is O(N) and blocks Redis event loop. Per-org key set is O(M) where M = keys for that org. |
| MongoDB hash chain via `findOneAndUpdate` | In-application sequence | In-application sequence has TOCTOU race between reads. `findOneAndUpdate` with `$inc` is atomic at the MongoDB layer. |
| Separate `org_id` column on outbox table | Use Kafka partition key for RLS | Partition key is a routing concern; RLS is a security concern. Coupling them creates correctness bugs if routing changes. |
| HMAC for MFA backup codes | bcrypt array | bcrypt array is O(N × bcrypt_cost) = ~3s for 10 codes. HMAC lookup is O(1). Backup codes are not passwords; brute-force enumeration is protected by rate limiting, not hashing cost. |
| Manual Kafka offset commit | Auto-commit | Auto-commit acknowledges messages before they are written to MongoDB/ClickHouse. A crash after auto-commit but before the write permanently loses the audit event. Manual commit provides exactly-once semantics with the `event_id` unique index. |
| Refresh token grace window | Strict single-use | Strict single-use causes valid clients to be logged out on network retries (the first request succeeds and rotates the token; the retry uses the old token and gets 401). Grace window prevents this while maintaining security. |
| ClickHouse partitioned by month only | Partition by (month, org_id) | ClickHouse does not efficiently handle per-org partitioning at 10k+ orgs. Each org-month pair becomes a separate part, causing part explosion and INSERT degradation. `org_id` in ORDER BY is sufficient for query performance. |
