# OpenGuard — Enterprise Implementation

> **For the implementing engineer:** This is the single authoritative document for OpenGuard. It contains both the system architecture specification and the Go clean-code standards that apply to every line of implementation. Read the entire document before writing any code. The code quality standards in **Section 0** apply to every section that follows — they are not optional and are enforced by CI.
>
> This version supersedes v1.0 and incorporates fixes for: the dual-write / Transactional Outbox problem, PostgreSQL Row-Level Security, circuit breakers, the Saga pattern for distributed operations, multi-tenancy isolation, read/write split (CQRS), secret rotation, mTLS, ClickHouse bulk-insert batching, load performance targets, structured migration guarantees, and Go clean-code consistency throughout all code examples.

> **Non-negotiable rules:**
> - Every Kafka publish goes through the Outbox relay — never a direct producer call from a business handler.
> - Every table that holds org data has RLS enabled and enforced.
> - Every inter-service HTTP call wraps a circuit breaker.
> - Failure mode for the policy engine is **fail closed**: deny all access when unavailable.
> - No string concatenation in SQL — parameterized queries only, enforced by linter in CI.
> - No `time.Sleep` in service code paths — use `time.NewTicker` for all polling loops.
> - Interfaces are defined in the consuming service's package, not in `shared/`.
> - All canonical names (env vars, topic names, table names, error codes) are fixed — do not rename.

---

## Table of Contents

0. [Code Quality Standards](#0-code-quality-standards)
1. [Project Overview](#1-project-overview)
2. [Enterprise Architecture Principles](#2-enterprise-architecture-principles)
3. [Repository Layout](#3-repository-layout)
4. [Shared Contracts](#4-shared-contracts)
5. [Environment & Configuration](#5-environment--configuration)
6. [Multi-Tenancy Model](#6-multi-tenancy-model)
7. [Transactional Outbox Pattern](#7-transactional-outbox-pattern)
8. [Circuit Breakers & Resilience](#8-circuit-breakers--resilience)
9. [Phase 1 — Foundation (IAM + Control Plane API)](#9-phase-1--foundation-iam--control-plane-api)
10. [Phase 2 — Policy Engine](#10-phase-2--policy-engine)
11. [Phase 3 — Event Bus, Outbox Relay & Audit Log](#11-phase-3--event-bus-outbox-relay--audit-log)
12. [Phase 4 — Threat Detection & Alerting](#12-phase-4--threat-detection--alerting)
13. [Phase 5 — Compliance & Analytics](#13-phase-5--compliance--analytics)
14. [Phase 6 — Infra, CI/CD & Observability](#14-phase-6--infra-cicd--observability)
15. [Phase 7 — Security Hardening & Secret Rotation](#15-phase-7--security-hardening--secret-rotation)
16. [Phase 8 — Load Testing & Performance Tuning](#16-phase-8--load-testing--performance-tuning)
17. [Phase 9 — Documentation & Runbooks](#17-phase-9--documentation--runbooks)
18. [Phase 10 — Content Scanning & DLP](#18-phase-10--content-scanning--dlp)
19. [Cross-Cutting Concerns](#19-cross-cutting-concerns)
20. [Acceptance Criteria (Full System)](#20-acceptance-criteria-full-system)

---

## 0. Code Quality Standards

> These standards are not optional guidelines. They are enforced by CI (linters, race detector, coverage gate, SQL lint) and by code review. Every code example in this specification is written to satisfy them. Where a rule has a narrow exception specific to OpenGuard, that exception is stated explicitly and applies only where stated.

---

### 0.1 Philosophy

**Readability is a production concern.** Code is read ten times for every one time it is written. In an on-call rotation at 3 AM, unclear code costs real reliability. Optimize for the reader — the engineer who adds a feature here in six months, the SRE debugging a memory leak, the new hire extending this service in their second week.

**Boring code is good code.** Go is deliberately unexciting. Resist the urge to be clever. A `for` loop readable in five seconds beats a channel-of-channels construction that requires a design doc. The most important metric for a code review is: can a competent engineer understand this without asking you questions?

**Consistency beats local optimality.** When the team has agreed on a pattern, use it — even if you personally know a marginally better one. Propose changes to this document; do not silently diverge in code.

---

### 0.2 Package Design

#### One coherent concept per package

A package should encapsulate a single coherent concept. If you cannot describe what a package does in one sentence without the word "and," it probably needs to be split.

#### Service layout: `pkg/` is this project's `internal/`

Every service uses `services/<name>/pkg/` for all sub-packages. Because all seven services live in the same Go workspace and no service ever imports another service's packages directly, `pkg/` provides the same isolation guarantee as `internal/`. Nothing under `pkg/` is ever imported by a different service. The style guide's `internal/` rule applies to any standalone library or service built outside this workspace.

#### The `shared/` module is a justified exception to the no-generic-names rule

`github.com/openguard/shared` holds genuine cross-service wire contracts. Every package inside it has a real, descriptive name: `kafka/`, `models/`, `rls/`, `resilience/`, `telemetry/`, `crypto/`, `validator/`. Do not add packages to `shared/` whose own name is generic (e.g., `shared/utils/`, `shared/helpers/`).

#### No package-level variables for mutable state

Package-level `var` creates implicit global state that makes tests order-dependent and parallel execution treacherous.

```go
// Bad — test A may corrupt test B's state
var defaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

// Good — explicit, injectable, testable
func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{Timeout: timeout}
}
```

**Justified exceptions in this codebase:**
- `shared/telemetry/logger.go` — `sensitiveKeys` is a read-only slice initialized once at startup. Safe.
- `services/compliance/pkg/reporter/generator.go` — `reportBulkhead` is a process-lifetime concurrency limiter initialized from config at startup. Safe because it is never mutated after construction.
- Compiled regular expressions (`var emailRE = regexp.MustCompile(...)`).

#### No circular imports

If package A imports package B and package B imports package A, one of two things is true: they should be merged, or a third package should hold the shared type. The Go toolchain enforces this at compile time.

---

### 0.3 Naming Conventions

#### The name should eliminate the need for a comment

```go
// Bad
d := time.Since(start) // duration of the request

// Good
requestDuration := time.Since(start)
```

#### Exported names carry their package prefix — do not repeat it

The caller writes `repository.Repository`, not `repository.UserRepository`. This rule applies to all type names within their own package. Spec code examples reference types from the caller's perspective using their full `package.Type` form; when implementing, define the type without the package prefix.

```go
// In package repository — Bad
type UserRepository struct{}   // caller sees repository.UserRepository — redundant

// In package repository — Good
type Repository struct{}       // caller sees repository.Repository
```

**Concrete OpenGuard mapping:**

| Package | Type name inside the package |
|---|---|
| `pkg/repository/` | `Repository` |
| `pkg/service/` | `Service` |
| `pkg/handlers/` | `Handler` |
| `pkg/outbox/` | `Writer` |
| `pkg/router/` | `Router` |

#### Interface names describe the behavior

```go
// Bad — names the implementor, not the capability
type UserStore interface{}

// Good — names what you can do with it
type UserReader interface {
    GetByID(ctx context.Context, id string) (*models.User, error)
}

type UserWriter interface {
    Create(ctx context.Context, u *models.User) error
    Update(ctx context.Context, u *models.User) error
}

// Compose when both are needed
type UserRepository interface {
    UserReader
    UserWriter
}
```

#### Sentinel errors use the `Err` prefix

```go
var (
    ErrNotFound      = errors.New("not found")
    ErrAlreadyExists = errors.New("already exists")
    ErrUnauthorized  = errors.New("unauthorized")
)
```

Use sentinel errors for simple cases. Use typed errors when callers need to inspect fields:

```go
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation failed on %s: %s", e.Field, e.Message)
}
```

#### Acceptable abbreviations

`ctx`, `cfg`, `err`, `ok`, `id`, `tx`, `db`, `w`, `r` (in HTTP handlers) are universally understood. `usrMgr`, `svcCnfg`, `rqstHndlr` are not acceptable.

---

### 0.4 Error Handling

#### Never discard errors silently

```go
// Unacceptable — always handle the error
_ = db.Close()

// Correct
if err := db.Close(); err != nil {
    slog.ErrorContext(ctx, "failed to close db connection", "error", err)
}
```

#### Wrap errors once at each layer boundary

```go
// Repository layer: translate DB errors to domain errors
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

// Service layer: one wrap with service-level context
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
// Good — traverses the full error chain
if errors.Is(err, ErrNotFound) {
    return http.StatusNotFound
}

var valErr *ValidationError
if errors.As(err, &valErr) {
    respondWithFieldError(w, valErr.Field, valErr.Message)
}

// Bad — brittle and breaks with wrapped errors
if strings.Contains(err.Error(), "not found") {}
```

#### Log or return — never both

Logging an error and then returning it causes double-logging. Log at the outermost layer that has full request context (the HTTP handler or Kafka consumer). Service and repository layers return errors; they do not log them.

```go
// Bad — logs here AND the caller logs again
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) error {
    if err := s.repo.Create(ctx, input); err != nil {
        slog.ErrorContext(ctx, "repo create failed", "error", err) // double-log
        return fmt.Errorf("create user: %w", err)
    }
    return nil
}

// Good — return only; the handler logs with full request context
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) error {
    if err := s.repo.Create(ctx, input); err != nil {
        return fmt.Errorf("create user: %w", err)
    }
    return nil
}
```

#### Panic only for programmer errors and startup invariants

`panic` is appropriate when a nil dependency is passed to a constructor (programmer error) or when a required configuration is absent at startup. Never panic on runtime errors — return them.

```go
// Acceptable — startup invariant
func NewService(repo Repository, events EventPublisher) *Service {
    if repo == nil {
        panic("NewService: repo is required")
    }
    return &Service{repo: repo, events: events}
}
```

#### Do not return `nil, nil`

A nil error alongside a nil value is ambiguous. Return `ErrNotFound` or an equivalent sentinel. Callers should never need to nil-check a returned pointer when the error is also nil.

---

### 0.5 Interfaces

#### The consuming package owns the interface

Define interfaces in the package that uses them, not in the package that implements them. This keeps the interface narrow (only what the consumer actually needs), makes it trivial to swap implementations in tests, and avoids creating shared dependencies between packages.

```go
// Bad — shared/ defines the interface; all services are coupled to it
// package shared/kafka
type Publisher interface {
    Publish(ctx context.Context, topic, key string, payload []byte) error
}

// Good — IAM service defines the interface it needs; shared/kafka satisfies it implicitly
// services/iam/pkg/service/user.go
type eventPublisher interface {
    Publish(ctx context.Context, topic, key string, payload []byte) error
}

type Service struct {
    events eventPublisher // satisfied at runtime by *kafka.Producer
}
```

**OpenGuard rule:** `shared/` exports concrete types and structs (`EventEnvelope`, `OutboxRecord`, `kafka.Producer`, `outbox.Writer`). The interfaces over those types live in the consuming service's `pkg/service/` package, never in `shared/`.

#### Keep interfaces small

Every method added narrows the set of types that can satisfy the interface. Prefer single-method or two-method interfaces. Compose them when needed.

#### Do not add interfaces prematurely

An interface with one implementation and no tests that substitute it is engineering overhead. Add it when you have a second implementation, when you need a test double, or when crossing a significant layer boundary.

---

### 0.6 Concurrency

#### Every goroutine has an explicit owner and a termination path

If you cannot answer "who starts this goroutine," "when does it stop," and "who waits for it," the goroutine will leak. Use `errgroup` for all structured concurrency.

```go
// Good — lifecycle is explicit and tied to context cancellation
func (s *Service) Run(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error { return s.runEventLoop(ctx) })
    g.Go(func() error { return s.runCleanupLoop(ctx) })

    return g.Wait()
}

func (s *Service) runEventLoop(ctx context.Context) error {
    ticker := time.NewTicker(100 * time.Millisecond) // never time.Sleep
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

#### Use `errgroup` for parallel fan-out

```go
func (s *Service) enrichUser(ctx context.Context, userID string) (*EnrichedUser, error) {
    var (
        profile  *Profile
        roles    []Role
        sessions []Session
    )

    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error {
        var err error
        profile, err = s.profileSvc.Get(ctx, userID)
        return err
    })
    g.Go(func() error {
        var err error
        roles, err = s.policySvc.GetRoles(ctx, userID)
        return err
    })
    g.Go(func() error {
        var err error
        sessions, err = s.sessionSvc.List(ctx, userID)
        return err
    })

    if err := g.Wait(); err != nil {
        return nil, fmt.Errorf("enrich user %s: %w", userID, err)
    }
    return &EnrichedUser{Profile: profile, Roles: roles, Sessions: sessions}, nil
}
```

#### Use `sync.Mutex` for protecting shared state, channels for transferring ownership

A mutex is right when multiple goroutines read and write the same memory. A channel is right when one goroutine hands a value to another. Do not reach for channels where a mutex is simpler.

#### `wg.Add` before the goroutine starts, `wg.Done` via `defer` as the first line

```go
// Bad — race: goroutine may not have called Add before Wait
for _, item := range items {
    go func(item Item) {
        wg.Add(1) // wrong place
        defer wg.Done()
        process(item)
    }(item)
}

// Good
for _, item := range items {
    wg.Add(1)
    go func(item Item) {
        defer wg.Done()
        process(item)
    }(item)
}
wg.Wait()
```

---

### 0.7 Context Discipline

#### `context.Context` is always the first parameter, never stored in a struct

```go
// Bad — hides lifecycle, makes cancellation invisible
type Service struct {
    ctx context.Context
}

// Good — context flows explicitly through every call
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {}
```

The only exception is a long-running background worker where the context is the worker's entire lifetime, passed to `Run(ctx)` at construction.

#### Never pass `context.Background()` inside a request handler

```go
// Bad — query outlives the HTTP request; client disconnect is ignored
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    user, err := h.repo.GetByID(context.Background(), chi.URLParam(r, "id"))
}

// Good — query is cancelled when client disconnects
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    user, err := h.repo.GetByID(r.Context(), chi.URLParam(r, "id"))
}
```

#### Context values are for request-scoped metadata only

Context is not a dependency injection mechanism. It carries: trace IDs, request IDs, authenticated user IDs, and `org_id` for RLS. Dependencies (repositories, event publishers, loggers) go in struct fields set by the constructor.

```go
// Bad — dependencies via context
ctx = context.WithValue(ctx, dbKey{}, db)

// Good — typed context values for metadata only
func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}
```

#### Propagate context cancellation; do not mask it

When `ctx.Err()` is non-nil after an operation, return the context error. Do not substitute a generic error — callers distinguish "the operation failed" from "the caller cancelled."

---

### 0.8 Dependency Injection & Wiring

#### Constructor injection — always

Every type with external dependencies receives them through its constructor. No type reaches for a global variable, singleton, or `sync.Once`-initialized dependency inside business logic.

```go
// Bad — hidden global dependency
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
    return globalDB.QueryUser(ctx, id)
}

// Good — explicit dependency, testable
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

All dependency construction and wiring belongs in `main.go`. Business packages never construct their own dependencies. The full dependency graph of the service is visible in one place.

```go
// main.go — the entire wiring is visible here
func main() {
    cfg := config.MustLoad()

    pgPool    := mustOpenPostgres(ctx, cfg.Postgres)
    mongoWrite, mongoRead := mustOpenMongo(ctx, cfg.Mongo)
    redisClient := mustOpenRedis(ctx, cfg.Redis)
    kafkaProd   := mustOpenKafkaProducer(ctx, cfg.Kafka)

    outboxWriter := outbox.NewWriter()
    userRepo     := repository.NewRepository(pgPool)
    userSvc      := service.NewService(userRepo, redisClient, outboxWriter, kafkaProd)
    userHandler  := handlers.NewHandler(userSvc, validator.New())

    router := router.New(userHandler)
    server := &http.Server{
        Addr:              cfg.Addr,
        Handler:           router,
        ReadTimeout:       5 * time.Second,
        ReadHeaderTimeout: 2 * time.Second,
        WriteTimeout:      10 * time.Second,
        IdleTimeout:       120 * time.Second,
    }
    runServer(ctx, server)
}
```

#### Use functional options for constructors with more than three parameters

```go
type clientOptions struct {
    timeout   time.Duration
    retryMax  int
    userAgent string
}

type ClientOption func(*clientOptions)

func WithTimeout(d time.Duration) ClientOption {
    return func(o *clientOptions) { o.timeout = d }
}

func NewHTTPClient(baseURL string, opts ...ClientOption) *HTTPClient {
    o := &clientOptions{timeout: 10 * time.Second, retryMax: 3}
    for _, opt := range opts {
        opt(o)
    }
    return &HTTPClient{baseURL: baseURL, opts: o}
}
```

---

### 0.9 Configuration

#### Fail fast at startup — never discover bad config at request time

`config.MustLoad()` panics on any missing or malformed required variable. A service that starts with bad config fails immediately, loudly, and predictably. A service that discovers the missing config on the first request that exercises the broken path fails in production, silently, and unpredictably.

The `shared/config/config.go` helpers (`Must`, `MustInt`, `MustDuration`, `MustJSON`) are the canonical implementation (Section 5.2). Use them exclusively — do not call `os.Getenv` from inside business packages.

#### Typed sub-configs prevent stringly-typed mistakes

```go
type Config struct {
    Addr     string
    Postgres PostgresConfig
    Redis    RedisConfig
    Kafka    KafkaConfig
    JWT      JWTConfig
}

type JWTConfig struct {
    Keys   []crypto.JWTKey
    Expiry time.Duration
    Issuer string
}
```

Pass sub-configs (or constructed clients) to constructors. Never scatter `os.Getenv` calls across packages.

---

### 0.10 Testing

#### Test behaviour, not implementation

Tests that assert on internal state, call unexported methods, or are coupled to implementation data structures break on every refactor. Tests describe what the system does.

#### Use table-driven tests for input/output variation

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

`require.NoError` stops the test immediately. `assert.Equal` collects all failures. Use `require` when the test cannot meaningfully continue after a failure; use `assert` otherwise so you see all failures at once.

#### Fakes over generated mocks for most cases

Hand-written fakes are more readable, type-safe, and maintainable than generated mocks for narrow interfaces.

```go
// A fake that lives alongside the test
type fakeUserRepo struct {
    users     map[string]*models.User
    createErr error
    getCalled bool
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*models.User, error) {
    f.getCalled = true
    if u, ok := f.users[id]; ok {
        return u, nil
    }
    return nil, ErrNotFound
}

func (f *fakeUserRepo) Create(_ context.Context, u *models.User) error {
    if f.createErr != nil {
        return f.createErr
    }
    if f.users == nil {
        f.users = make(map[string]*models.User)
    }
    f.users[u.ID] = u
    return nil
}
```

#### Integration tests use `testcontainers-go` with real databases

Unit tests with fakes cover business logic. Integration tests with real PostgreSQL and MongoDB containers cover the SQL, schema, pgx driver behaviour, and index usage.

```go
func TestRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    ctx := context.Background()
    container, dsn := startPostgres(t, ctx)
    defer container.Terminate(ctx)

    pool := mustOpenPool(t, dsn)
    runMigrations(t, dsn)

    repo := repository.NewRepository(pool)

    t.Run("create and retrieve user", func(t *testing.T) {
        org := seedOrg(t, pool)
        created, err := repo.Create(ctx, CreateInput{OrgID: org.ID, Email: "test@example.com"})
        require.NoError(t, err)
        require.NotEmpty(t, created.ID)

        found, err := repo.GetByID(ctx, created.ID)
        require.NoError(t, err)
        assert.Equal(t, "test@example.com", found.Email)
    })
}
```

#### CI runs all tests with `-race`; coverage floor is 70%

```bash
go test ./... -race -count=1 -coverprofile=coverage.out -timeout 5m
```

The CI pipeline (Section 15.2) enforces ≥ 70% coverage per package and rejects any PR that drops below this threshold. 70% is the floor; aim higher in new packages.

---

### 0.11 Observability

#### Structured logging with `log/slog`, JSON in all non-dev environments

```go
// Bad — impossible to query or alert on
log.Printf("user %s logged in from %s", userID, ip)

// Good — every field is queryable in any log aggregator
slog.InfoContext(ctx, "user login success",
    "user_id",    userID,
    "ip_address", ip,
    "country",    country,
    "mfa_used",   mfaUsed,
)
```

#### Mandatory fields on every log entry

These fields are injected automatically by the logger middleware and `slog.With` base attributes. Individual log callsites do not repeat them.

| Field | Source |
|---|---|
| `service` | Hardcoded service name constant |
| `env` | `APP_ENV` |
| `trace_id` | OTel W3C trace ID from context |
| `span_id` | OTel span ID from context |
| `request_id` | `X-Request-ID` header |
| `org_id` | RLS context (omit for system operations) |
| `user_id` | JWT claim (omit for unauthenticated requests) |
| `duration_ms` | For request-scoped completion logs |

#### `SafeAttr` for any attribute whose key might be sensitive

The `SafeAttr` function (Section 16.3) redacts values whose key contains: `password`, `secret`, `token`, `key`, `auth`, `credential`, `private`, `bearer`, `authorization`, `cookie`, `session`. Use it for every log attribute passed from user-controlled or secret-adjacent data.

#### Log at the handler layer; service and repository layers return errors

This is the enforcement of the log-or-return rule at the architectural level. Handlers have the full request context (trace ID, user ID, org ID, request path) needed for a meaningful error log. Repository and service methods do not log — they return wrapped errors.

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

Sampling rate is controlled by `OTEL_SAMPLING_RATE` (0.1 in production, 1.0 in development). The Outbox relay injects `trace_id` from the parent context into each `EventEnvelope` so traces span from HTTP request through to the audit event in MongoDB.

---

### 0.12 HTTP Handler Rules

#### Handlers are thin: bind → validate → call service → respond

Business logic never lives in a handler. A handler has exactly four jobs.

```go
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit — always

    var input service.CreateUserInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        h.respondError(w, r, http.StatusBadRequest, "INVALID_JSON", err.Error())
        return
    }
    if err := h.validator.Struct(input); err != nil {
        h.respondValidationError(w, r, err)
        return
    }

    user, err := h.svc.CreateUser(r.Context(), input)
    if err != nil {
        h.handleServiceError(w, r, err)
        return
    }
    h.respond(w, http.StatusCreated, user)
}
```

#### Centralize error-to-status-code mapping

One `handleServiceError` method on the handler. Not scattered per endpoint.

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

The response body must never reveal database error strings, file paths, internal service names, or stack traces. Log the full internal error; return a sanitized message. Matches the `APIError` contract defined in Section 4.6.

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

A server without `ReadTimeout` is vulnerable to slow-client attacks. A server without `WriteTimeout` accumulates goroutines for clients that have silently disconnected.

---

### 0.13 Performance Discipline

#### Measure before optimizing — always

The Go profiler (`pprof`) tells you exactly where CPU time and allocations are going. Intuitions about hot paths are almost always wrong.

```bash
go test -bench=. -benchmem -cpuprofile=cpu.prof ./pkg/policy/...
go tool pprof -http=:8080 cpu.prof
```

#### `sync.Pool` for frequently allocated temporary objects

```go
var envelopePool = sync.Pool{
    New: func() any { return new(bytes.Buffer) },
}

func marshalEnvelope(env *models.EventEnvelope) ([]byte, error) {
    buf := envelopePool.Get().(*bytes.Buffer)
    buf.Reset()
    defer envelopePool.Put(buf)
    if err := json.NewEncoder(buf).Encode(env); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}
```

#### Pre-allocate slices when the length is known

```go
// Bad — repeated reallocation on every append
var events []AuditEvent
for rows.Next() {
    var e AuditEvent
    rows.Scan(...)
    events = append(events, e)
}

// Good — single allocation
events := make([]AuditEvent, 0, expectedCount)
for rows.Next() {
    var e AuditEvent
    rows.Scan(...)
    events = append(events, e)
}
```

#### Avoid reflection in hot paths

`reflect` is expensive. The policy evaluation path (`POST /policies/evaluate`, p99 target 30ms) and JWT validation path (p99 target 5ms) must not use reflection. Prefer code generation or explicit type-specific implementations.

---

### 0.14 Forbidden Patterns

The following patterns are banned in all service code. CI linters are configured to catch most of them. Code review must catch the rest.

#### No `init()` for side effects

`init()` runs at package load time in import-graph order with no way to inject dependencies, return errors, or control execution. It makes startup bugs mysterious and tests order-dependent.

```go
// Banned
func init() {
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    globalDB = db
}

// Correct — explicit wiring in main.go
func main() {
    cfg := config.MustLoad()
    pool := mustOpenPostgres(ctx, cfg.Postgres)
    // pass pool to constructors
}
```

**Exception:** Initializing truly read-only package-level values that cannot be `const` — pre-compiled regexes, `errors.New` sentinels.

#### No `log.Fatal` or `os.Exit` outside `main`

`log.Fatal` calls `os.Exit(1)`, bypassing all deferred functions. The service gets no chance to flush buffers, close connections, or release locks.

```go
// Banned in service/repository/handler packages
func (s *Server) Start() {
    if err := s.db.Ping(); err != nil {
        log.Fatalf("db unreachable: %v", err) // kills process with no cleanup
    }
}

// Correct — return the error; main.go decides whether to exit
func (s *Server) Start() error {
    if err := s.db.Ping(); err != nil {
        return fmt.Errorf("db ping: %w", err)
    }
    return nil
}
```

#### No `any` / `interface{}` as function parameter types

`any` parameters bypass the type system, force type assertions, and turn compile-time errors into runtime panics.

```go
// Banned
func Process(payload any) error {
    event, ok := payload.(*models.EventEnvelope)
    if !ok {
        return errors.New("unexpected type") // panics waiting to happen
    }
}

// Correct — the compiler enforces the contract
func Process(event *models.EventEnvelope) error {}
```

Accepted uses: JSON marshaling/unmarshaling (`json.Unmarshal(data, &dest)`), `slog.Any`, generic containers where the type is genuinely unknown.

#### No `time.Sleep` in service code paths

`time.Sleep` blocks the goroutine without responding to context cancellation. It makes code untestable and unshuttable.

```go
// Banned in all service, consumer, and relay code
for {
    s.processBatch()
    time.Sleep(100 * time.Millisecond) // not cancellable
}

// Correct — responds to context cancellation and supports graceful shutdown
ticker := time.NewTicker(100 * time.Millisecond)
defer ticker.Stop()
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-ticker.C:
        if err := s.processBatch(ctx); err != nil {
            s.logger.ErrorContext(ctx, "batch failed", "error", err)
        }
    }
}
```

**Exception:** The `scripts/re-encrypt-mfa.sh` migration script uses `time.Sleep(50ms)` between batches to throttle DB load. That is a one-off operational script, not a service code path. This exception does not extend to any code inside `services/`.

#### No shadowed `err` variables

```go
// Bug — inner err is a different variable; outer err stays nil
err := doFirst()
if err != nil { return err }
if condition {
    err := doSecond() // shadows outer err; this err is discarded
}
return err // always nil — bug

// Correct — assign to the outer err
err := doFirst()
if err != nil { return err }
if condition {
    err = doSecond()
}
return err
```

#### No string concatenation in SQL

Parameterized queries only. No exceptions. The SQL linter in CI (`go-sqllint`) catches this automatically, but it is also a code review gate.

```go
// Banned — SQL injection vector
query := "SELECT * FROM users WHERE email = '" + email + "'"

// Required
const query = `SELECT id, email FROM users WHERE email = $1`
row := pool.QueryRow(ctx, query, email)
```

---

### 0.15 Code Review Checklist

Use this checklist on every PR. All items must pass before merge.

**Package & structure**
- [ ] Package has a single, statable purpose
- [ ] No package named `utils`, `common`, `misc`, or `helpers` added to `shared/`
- [ ] No service imports another service's `pkg/` packages
- [ ] No circular imports

**Errors**
- [ ] No discarded errors (`_ = ...`)
- [ ] Errors wrapped once at layer boundaries, not at every callsite
- [ ] `errors.Is` / `errors.As` used for inspection — no string matching
- [ ] No log-and-return (pick one)
- [ ] No `nil, nil` returns
- [ ] No shadowed `err`

**Concurrency**
- [ ] Every goroutine has a clear owner and termination path
- [ ] `wg.Add` called before goroutine starts
- [ ] No `time.Sleep` in service code — `time.NewTicker` used for polling
- [ ] Tests run with `-race` in CI

**Context**
- [ ] `ctx` is the first parameter on every function that touches I/O
- [ ] `context.Background()` not used inside request handlers
- [ ] Context values are typed (no raw string keys)
- [ ] Cancellation propagated — not swallowed

**Database**
- [ ] All SQL uses parameterized queries (`$1`, `$2`) — no string interpolation
- [ ] Transactions defer `Rollback` and commit last
- [ ] No transaction held open across a network or external service call
- [ ] `rls.SetSessionVar` called before every PostgreSQL query on a pooled connection

**HTTP**
- [ ] Handler only binds, validates, calls service, responds
- [ ] Error-to-status mapping in centralized `handleServiceError`
- [ ] Internal errors not exposed in response body
- [ ] `http.MaxBytesReader` applied to every request body
- [ ] Server configured with `ReadTimeout`, `WriteTimeout`, `IdleTimeout`

**Observability**
- [ ] All log entries use `slog` with `slog.InfoContext` / `slog.ErrorContext`
- [ ] Sensitive fields passed through `SafeAttr`
- [ ] External calls wrapped in OTel spans
- [ ] Metrics use label cardinality that will not cause Prometheus cardinality explosion

**Interfaces & DI**
- [ ] Interfaces defined in the consuming package, not in `shared/`
- [ ] Dependencies injected via constructor — no global state accessed from business logic
- [ ] No `init()` for side effects
- [ ] No `log.Fatal` / `os.Exit` outside `main`

---
## 1. Project Overview

### 1.1 What is OpenGuard?

OpenGuard is an open-source, self-hostable **centralized security control plane** inspired by Atlassian Guard. Rather than sitting inline as a reverse proxy, OpenGuard operates as a governance hub that connected applications register with and call out to. User traffic never flows *through* OpenGuard; instead, connected apps integrate via SCIM 2.0 provisioning, OIDC/SAML as an IdP, a policy evaluation SDK, and outbound webhook/event delivery.

It is designed to operate at Fortune-500 scale: 100,000+ users, 10,000+ organizations, millions of audit events per day, with cryptographic audit trail integrity, zero cross-tenant data leakage, and sub-100ms policy evaluation at the 99th percentile.

Core capabilities:
- **Identity & Access Management (IAM):** Acts as an OIDC/SAML IdP. SSO (SAML 2.0 / OIDC), SCIM 2.0 provisioning, TOTP/WebAuthn MFA, API token lifecycle, session management. Exposes standard `/authorize`, `/token`, `/userinfo`, and `/jwks` OIDC endpoints.
- **Policy Engine:** Real-time RBAC evaluation called by the embedded SDK in connected apps. Data security rules, IP allowlists, session limits. Fails closed. Apps call `POST /v1/policy/evaluate`; the SDK caches decisions locally for up to 60 seconds during control plane unavailability.
- **Connector Registry:** Connected applications register with OpenGuard and receive org-scoped API credentials. The registry stores webhook URLs, scopes, and credential hashes per connector.
- **Event Ingestion:** Connected apps push audit events to `POST /v1/events/ingest`. The control plane normalizes and routes these into the same Kafka-backed audit pipeline as internal events.
- **Threat Detection:** Streaming anomaly scoring — brute force, impossible travel, off-hours access, data exfiltration. Fed by both internal IAM events and events pushed by connected apps.
- **Audit Log:** Append-only, hash-chained, cryptographically verifiable event trail with configurable retention.
- **Alerting & Webhooks:** Rule-based + ML-scored alerts, SIEM webhook export, Slack/email delivery. Policy changes and security events are pushed to connected apps via signed outbound webhooks.
- **Compliance Reporting:** GDPR, SOC 2, HIPAA report generation with PDF output.
- **Admin Dashboard:** Next.js 14 web console including a Connected Apps management section.

### 1.2 Performance Targets (Canonical SLOs)

These are hard targets. Phase 8 must verify each one with k6 load tests. No phase is complete until its SLOs are met.

| Operation | p50 | p99 | p999 | Throughput |
|-----------|-----|-----|------|------------|
| `POST /auth/login` (IAM OIDC token) | 40ms | 150ms | 400ms | 2,000 req/s |
| `POST /v1/policy/evaluate` (SDK call) | 5ms | 30ms | 80ms | 10,000 req/s |
| `GET /audit/events` (paginated) | 20ms | 100ms | 250ms | 1,000 req/s |
| Kafka event → audit DB insert | — | 2s | 5s | 50,000 events/s |
| Compliance report generation | — | 30s | 120s | 10 concurrent |
| `POST /v1/events/ingest` (connector push) | 10ms | 50ms | 150ms | 20,000 req/s |
| `GET /v1/scim/v2/Users` (provisioning) | 30ms | 500ms | 1,500ms | 500 req/s |

### 1.3 Design Principles

| Principle | Implication |
|-----------|-------------|
| Fail safe | Policy engine unavailable → SDK returns cached decision for up to 60s, then denies. For high-stakes decisions (account suspension, MFA enrollment changes) the SDK always calls sync with no cache grace. IAM unavailable → reject all logins. Never fail open on security decisions. |
| Exactly-once audit | Every state-changing operation produces exactly one audit event. The Transactional Outbox guarantees this for internal events; connected-app ingest is deduplicated by `event_id`. |
| Zero cross-tenant leakage | PostgreSQL RLS enforced at the DB layer. Bug in application code cannot expose another org's data. |
| Immutable audit trail | Append-only MongoDB collection with hash chaining. Tampering is detectable. |
| Least privilege (services) | Each service has its own DB user with table-level grants. No service can read another service's tables. |
| Least privilege (tenants) | Tenant quotas enforced at the control plane. A noisy tenant cannot starve others. |
| Secret rotation without downtime | JWT signing uses key IDs (kid). Multiple valid keys coexist during rotation. |
| mTLS between services | All internal service-to-service calls use mTLS. A compromised service cannot impersonate another. |
| Structured migrations | golang-migrate with checksums. Down migrations required. Blue/green compatible (additive only in prod). |
| Observable by default | Every service emits traces, metrics, and structured logs from day one. No retrofitting. |

---

## 2. Enterprise Architecture Principles

### 2.1 The Dual-Write Problem and Why It Matters

The v1 spec had this pattern in every service handler:

```go
// WRONG — DO NOT DO THIS
db.Exec("INSERT INTO users ...")     // step 1
kafka.Publish("audit.trail", event) // step 2 — process crashes here = silent data loss
```

If the process crashes, OOM-kills, or loses network between step 1 and step 2:
- The user is created in PostgreSQL.
- No audit event is ever published.
- The audit log has a permanent gap.
- For a security platform, this is a compliance violation.

**The fix is the Transactional Outbox Pattern.** See Section 7 for the complete implementation contract.

### 2.2 Multi-Tenancy Isolation Levels

OpenGuard supports three isolation tiers selectable per organization plan:

| Tier | Mechanism | Use case |
|------|-----------|----------|
| **Shared** | PostgreSQL RLS on shared tables | SMB, free tier |
| **Schema** | Dedicated PostgreSQL schema per org | Mid-market |
| **Shard** | Dedicated PostgreSQL instance per org | Enterprise, regulated |

The spec implements **Shared** (RLS) fully and scaffolds **Schema** and **Shard** as extension points. All application code must be written to support RLS from day one — the schema and shard tiers slot in without changing handler logic.

### 2.3 CQRS and Read/Write Split

The audit log has asymmetric load: writes are high-throughput streaming (Kafka consumer); reads are ad-hoc queries from the dashboard and compliance exports.

MongoDB write path (Kafka consumer → primary):
- Consumer writes to the MongoDB **primary** only.
- Uses bulk insert with a buffer of up to 500 documents or 1 second, whichever comes first.

MongoDB read path (HTTP handlers → secondary):
- All `GET /audit/events` queries use `readPreference: secondaryPreferred`.
- Compliance report queries use `readPreference: secondary` (acceptable staleness: 5s).

This is enforced in the repository layer. See Section 11.

### 2.4 Saga Pattern for Distributed Operations

User provisioning via SCIM touches multiple services and must be atomic from the caller's perspective. OpenGuard uses **choreography-based sagas** via Kafka compensating events.

Example: SCIM `POST /scim/v2/Users`

```
IAM: user.created (org_id, user_id, scim_external_id) → audit.trail
Policy: consumes user.created → assigns default org policies → policy.assigned → audit.trail
Threat: consumes user.created → initializes baseline profile
Alerting: consumes user.created → configures notification preferences
```

If policy assignment fails, Policy publishes `policy.assignment.failed` with a `compensation: true` flag. IAM consumes this and sets the user to `status: provisioning_failed`, publishes `user.provisioning.failed` for the SCIM caller to poll.

Each saga step is idempotent. Consumer groups use `auto.offset.reset: earliest` so replays are safe.

### 2.5 Connection Pooling Targets

| Service | DB | Pool min | Pool max | Rationale |
|---------|----|----------|----------|-----------|
| IAM | PostgreSQL | 5 | 25 | Login burst |
| Policy | PostgreSQL | 2 | 15 | Short-lived evaluate queries |
| Audit (write) | MongoDB | 2 | 10 | Bulk inserts, low concurrency |
| Audit (read) | MongoDB | 5 | 30 | Dashboard queries |
| Compliance | ClickHouse | 2 | 8 | Long-running aggregations |
| All services | Redis | 5 | 20 | Rate limit + session |

These are configured via env vars (see Section 5) and enforced in the `db` package of each service.

### 2.6 App Registration Model

Every protected application must register with OpenGuard before it can call the control plane API. Registration produces an org-scoped API credential that the connector uses for all subsequent calls.

Registration flow:
1. An org admin calls `POST /v1/connectors` from the admin dashboard or API, supplying the app name, webhook URL, and the scopes it needs (e.g., `["audit:write", "policy:read", "users:read"]`).
2. The connector registry creates a `ConnectedApp` record and returns a one-time plaintext API key. The registry stores only the PBKDF2 hash of the key — the plaintext is never stored and cannot be retrieved again.
3. The connected app includes the key in all API calls as `Authorization: Bearer <key>`.
4. The control plane's API key middleware hashes the inbound key, looks it up in the connector registry, and derives `org_id` from the stored record. No `X-Org-ID` header injection from a proxy is required.
5. Connectors can be suspended (`PATCH /v1/connectors/:id {status: "suspended"}`) which immediately revokes all API key access without requiring credential rotation.

Scopes are enforced at the control plane middleware layer. A connector with scope `audit:write` cannot call `POST /v1/policy/evaluate`, and vice versa. Scope violations return `403 INSUFFICIENT_SCOPE`.

### 2.7 Push/Pull Event Flow

OpenGuard supports two event directions between itself and connected apps:

**Inbound (connected app → OpenGuard):** Apps push audit and access events to `POST /v1/events/ingest`. The control plane validates the connector credential, normalizes the event into an `EventEnvelope` (adding `event_source: "connector:<connector_id>"`), and writes it to the outbox. The relay publishes to Kafka, and the audit and threat services consume it identically to internally generated events.

**Outbound (OpenGuard → connected app):** When OpenGuard produces a security-relevant event (user suspended, policy changed, alert fired), the webhook delivery service reads from `TopicWebhookDelivery`, signs the payload with the connector's `webhook_secret`, and POSTs it to the connector's registered webhook URL. Delivery is at-least-once with exponential backoff. Failed deliveries go to `webhook.dlq` after 5 attempts.

This push/pull model replaces the previous architecture where the gateway passively intercepted all traffic. The outbox pattern still underpins the outbound webhook path — no webhook is delivered without a corresponding committed outbox record.

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
│   ├── control-plane/          # Public-facing API for connected apps (replaces gateway)
│   ├── connector-registry/     # Stores registered apps, API key hashes, scopes, webhook config
│   ├── iam/
│   ├── policy/
│   ├── threat/
│   ├── audit/
│   ├── alerting/
│   ├── webhook-delivery/       # Reads TopicWebhookDelivery, delivers to connector webhook URLs
│   ├── compliance/
│   └── dlp/                    # Phase 10: Scanning service for PII, credentials, financial data
├── sdk/                        # Embeddable Go client library for connected apps
│   ├── go.mod                  # module: github.com/openguard/sdk
│   ├── policy/
│   │   ├── client.go           # Calls POST /v1/policy/evaluate
│   │   └── cache.go            # Local LRU cache with TTL, used during control plane unavailability
│   ├── events/
│   │   ├── publisher.go        # Batches and pushes events to POST /v1/events/ingest
│   │   └── batcher.go          # Buffer: SDK_EVENT_BATCH_SIZE or SDK_EVENT_FLUSH_INTERVAL_MS
│   ├── breaker.go              # Local circuit breaker around control plane calls
│   └── client.go               # Root SDK client, holds credentials and base URL
├── shared/                     # go module: github.com/openguard/shared
│   ├── go.mod
│   ├── kafka/
│   │   ├── producer.go
│   │   ├── consumer.go
│   │   ├── topics.go
│   │   └── outbox/
│   │       ├── relay.go        # Outbox → Kafka relay
│   │       └── poller.go
│   ├── middleware/
│   │   ├── apikey.go           # API key auth — replaces proxy-injected X-Org-ID
│   │   ├── tenant.go           # Sets app.org_id for RLS (derives from API key, not header)
│   │   ├── ratelimit.go
│   │   ├── circuitbreaker.go
│   │   ├── logger.go
│   │   └── mtls.go
│   ├── models/
│   │   ├── event.go
│   │   ├── user.go
│   │   ├── policy.go
│   │   ├── connector.go        # ConnectedApp model
│   │   ├── errors.go
│   │   ├── outbox.go
│   │   └── saga.go
│   ├── rls/
│   │   └── context.go          # Sets + reads app.org_id from Go context
│   ├── resilience/
│   │   ├── breaker.go          # Circuit breaker wrapper
│   │   ├── retry.go            # Exponential backoff with jitter
│   │   └── bulkhead.go         # Concurrency limiter
│   ├── telemetry/
│   │   ├── otel.go
│   │   ├── metrics.go
│   │   └── logger.go
│   ├── crypto/
│   │   ├── jwt.go
│   │   ├── aes.go
│   │   └── apikey.go           # PBKDF2 API key hashing
│   └── validator/
│       └── validator.go
├── infra/
│   ├── docker/
│   │   └── docker-compose.yml
│   ├── k8s/
│   │   └── helm/openguard/
│   ├── kafka/
│   │   └── topics.json
│   ├── certs/                  # generated by gen-mtls-certs.sh
│   └── monitoring/
│       ├── prometheus.yml
│       ├── grafana/
│       └── alerts/
├── web/                        # Next.js 14 admin dashboard
│   ├── app/
│   │   └── (dashboard)/
│   │       ├── connectors/     # Connected Apps management UI (new)
│   │       ├── threats/
│   │       ├── audit/
│   │       └── compliance/
│   └── package.json
├── loadtest/
│   ├── auth.js
│   ├── policy-evaluate.js
│   ├── audit-query.js
│   ├── event-ingest.js         # New: load test for POST /v1/events/ingest
│   └── kafka-throughput.js
├── docs/
│   ├── architecture.md
│   ├── runbooks/
│   │   ├── kafka-consumer-lag.md
│   │   ├── circuit-breaker-open.md
│   │   ├── audit-hash-mismatch.md
│   │   ├── secret-rotation.md
│   │   ├── connector-suspension.md    # New
│   │   └── webhook-delivery-failure.md # New
│   ├── contributing.md
│   └── api/
├── scripts/
│   ├── create-topics.sh
│   ├── migrate.sh
│   ├── seed.sh
│   ├── gen-mtls-certs.sh
│   └── rotate-jwt-keys.sh
├── go.work
├── .env.example
├── Makefile
└── README.md
```


### 3.1 Go Workspace

```
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

### 3.2 Service Module Layout (canonical — every service follows this)

```
services/<name>/
├── go.mod                          # module: github.com/openguard/<name>
├── main.go                         # wires everything, starts server + graceful shutdown
├── Dockerfile
├── migrations/
│   ├── 001_<name>.up.sql
│   └── 001_<name>.down.sql         # Required for every up migration
├── pkg/
│   ├── config/
│   │   └── config.go               # env-var loading using shared pattern
│   ├── db/
│   │   ├── postgres.go             # pgxpool setup, RLS session var injection
│   │   ├── mongo.go                # separate read + write clients
│   │   └── migrations.go           # golang-migrate runner
│   ├── outbox/
│   │   └── writer.go               # writes to local outbox table (same TX as business data)
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
    Type        string          `json:"type"`         // dot-separated, e.g. "auth.login.success"
    OrgID       string          `json:"org_id"`       // tenant identifier
    ActorID     string          `json:"actor_id"`     // user ID, service name, or "system"
    ActorType   string          `json:"actor_type"`   // "user" | "service" | "system"
    OccurredAt  time.Time       `json:"occurred_at"`  // event time, not processing time
    Source      string          `json:"source"`       // originating service: "iam", "policy", etc.
    EventSource string          `json:"event_source"` // "internal" | "connector:<connector_id>"
    TraceID     string          `json:"trace_id"`     // OpenTelemetry W3C trace ID
    SpanID      string          `json:"span_id"`      // OpenTelemetry span ID
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
type OutboxRecord struct {
    ID          string    `db:"id"`           // UUIDv4
    Topic       string    `db:"topic"`        // Kafka topic name
    Key         string    `db:"key"`          // Kafka partition key (usually org_id)
    Payload     []byte    `db:"payload"`      // JSON-encoded EventEnvelope
    Status      string    `db:"status"`       // "pending" | "published" | "dead"
    Attempts    int       `db:"attempts"`     // number of publish attempts
    LastError   string    `db:"last_error"`   // last error message
    CreatedAt   time.Time `db:"created_at"`
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
    SagaID       string `json:"saga_id"`              // UUIDv4, same across all steps
    SagaType     string `json:"saga_type"`            // "user.provision", "user.deprovision"
    SagaStep     int    `json:"saga_step"`            // 1-based step number
    Compensation bool   `json:"compensation"`         // true = this is a rollback event
    CausedBy     string `json:"caused_by,omitempty"` // event ID that caused this step
}
```

### 4.4 Kafka Topic Registry

```go
// shared/kafka/topics.go — canonical topic names, never hardcode strings
package kafka

const (
    TopicAuthEvents        = "auth.events"
    TopicPolicyChanges     = "policy.changes"
    TopicDataAccess        = "data.access"
    TopicThreatAlerts      = "threat.alerts"
    TopicAuditTrail        = "audit.trail"
    TopicNotificationsOut  = "notifications.outbound"
    TopicSagaOrchestration = "saga.orchestration"
    TopicOutboxDLQ         = "outbox.dlq"           // dead-letter for relay failures
    TopicConnectorEvents   = "connector.events"     // inbound events from connected apps
    TopicWebhookDelivery   = "webhook.delivery"     // outbound webhook queue
    TopicWebhookDLQ        = "webhook.dlq"          // failed webhook deliveries
)

// ConsumerGroups — canonical consumer group IDs
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
    ID              string     `json:"id" db:"id"`
    OrgID           string     `json:"org_id" db:"org_id"`
    Email           string     `json:"email" db:"email"`
    DisplayName     string     `json:"display_name" db:"display_name"`
    Status          UserStatus `json:"status" db:"status"`
    MFAEnabled      bool       `json:"mfa_enabled" db:"mfa_enabled"`
    MFAMethod       string     `json:"mfa_method,omitempty" db:"mfa_method"` // "totp" | "webauthn"
    SCIMExternalID  string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
    ProvisioningStatus string  `json:"provisioning_status" db:"provisioning_status"` // "complete" | "pending" | "failed"
    TierIsolation   string     `json:"tier_isolation" db:"tier_isolation"` // "shared" | "schema" | "shard"
    CreatedAt       time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
    DeletedAt       *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
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

// ConnectedApp represents an application registered with OpenGuard.
// Each app receives org-scoped API credentials used to authenticate
// all control plane API calls.
type ConnectedApp struct {
    ID                string     `json:"id" db:"id"`
    OrgID             string     `json:"org_id" db:"org_id"`
    Name              string     `json:"name" db:"name"`
    WebhookURL        string     `json:"webhook_url" db:"webhook_url"`
    WebhookSecretHash string     `json:"-" db:"webhook_secret_hash"` // HMAC secret, never returned
    APIKeyHash        string     `json:"-" db:"api_key_hash"`        // PBKDF2 hash, never returned
    Scopes            []string   `json:"scopes" db:"scopes"`
    Status            string     `json:"status" db:"status"`         // "active" | "suspended" | "pending"
    CreatedAt         time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
    SuspendedAt       *time.Time `json:"suspended_at,omitempty" db:"suspended_at"`
}
```

### 4.7 Standard HTTP Contracts

**Error response (all services):**
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
    Retryable bool   `json:"retryable"` // clients use this to decide whether to retry
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

Cursor-based pagination is used for audit log and threat alert endpoints (high volume). Page-number pagination is acceptable for user and policy lists.

---

## 5. Environment & Configuration

### 5.1 `.env.example` (canonical — every variable required)

```dotenv
# ── App ──────────────────────────────────────────────────────────────
APP_ENV=development                   # development | staging | production
LOG_LEVEL=info                        # debug | info | warn | error
LOG_FORMAT=json                       # json | text (use json in non-dev)

# ── Control Plane ────────────────────────────────────────────────────
CONTROL_PLANE_PORT=8080
CONTROL_PLANE_API_KEY_SALT=change-me              # PBKDF2 salt for API key hashing
CONTROL_PLANE_WEBHOOK_SIGNING_SECRET=change-me    # HMAC-SHA256 secret for outbound webhook signing
CONTROL_PLANE_POLICY_CACHE_TTL_SECONDS=60         # SDK local cache grace period during unavailability
CONTROL_PLANE_EVENT_INGEST_MAX_BATCH=500          # Max events per POST /v1/events/ingest call
CONTROL_PLANE_RATE_LIMIT_CONNECTOR=1000           # req/min per connector_id
CONTROL_PLANE_TENANT_QUOTA_RPM=5000               # req/min per org_id (all connectors combined)
CONTROL_PLANE_MTLS_CERT_FILE=/certs/control-plane.crt
CONTROL_PLANE_MTLS_KEY_FILE=/certs/control-plane.key
CONTROL_PLANE_MTLS_CA_FILE=/certs/ca.crt

# ── Connector Registry ───────────────────────────────────────────────
CONNECTOR_REGISTRY_PORT=8090
CONNECTOR_REGISTRY_MTLS_CERT_FILE=/certs/connector-registry.crt
CONNECTOR_REGISTRY_MTLS_KEY_FILE=/certs/connector-registry.key
CONNECTOR_REGISTRY_MTLS_CA_FILE=/certs/ca.crt

# ── Webhook Delivery Service ─────────────────────────────────────────
WEBHOOK_DELIVERY_PORT=8091
WEBHOOK_MAX_ATTEMPTS=5                            # Attempts before moving to webhook.dlq
WEBHOOK_BACKOFF_BASE_MS=1000                      # Base delay for exponential backoff
WEBHOOK_BACKOFF_MAX_MS=60000                      # Max delay cap
WEBHOOK_DELIVERY_TIMEOUT_MS=5000                  # Per-delivery HTTP timeout
WEBHOOK_DELIVERY_MTLS_CERT_FILE=/certs/webhook-delivery.crt
WEBHOOK_DELIVERY_MTLS_KEY_FILE=/certs/webhook-delivery.key
WEBHOOK_DELIVERY_MTLS_CA_FILE=/certs/ca.crt

# ── SDK Defaults (embedded in sdk/config.go) ─────────────────────────
SDK_CONTROL_PLANE_URL=https://api.openguard.example.com
SDK_POLICY_CACHE_TTL_SECONDS=60
SDK_EVENT_BATCH_SIZE=100
SDK_EVENT_FLUSH_INTERVAL_MS=2000

# ── IAM Service (OIDC/SAML IdP) ─────────────────────────────────────
IAM_PORT=8081
IAM_JWT_KEYS_JSON=[{"kid":"k1","secret":"change-me","algorithm":"HS256","status":"active"}]
# JWT_KEYS_JSON is an array — supports multiple keys for zero-downtime rotation.
# "status": "active" = sign + verify. "status": "verify_only" = verify only (rotation window).
IAM_JWT_EXPIRY_SECONDS=3600
IAM_REFRESH_TOKEN_EXPIRY_DAYS=30
IAM_SAML_ENTITY_ID=https://openguard.example.com
IAM_SAML_IDP_METADATA_URL=https://idp.example.com/metadata
IAM_OIDC_ISSUER=https://accounts.example.com
IAM_OIDC_CLIENT_ID=openguard
IAM_OIDC_CLIENT_SECRET=change-me
IAM_SCIM_BEARER_TOKEN=change-me
IAM_MFA_TOTP_ISSUER=OpenGuard
IAM_MFA_ENCRYPTION_KEY_JSON=[{"kid":"mk1","key":"base64-encoded-32-bytes","status":"active"}]
# Encryption keys follow the same rotation pattern as JWT keys.
IAM_WEBAUTHN_RPID=openguard.example.com
IAM_WEBAUTHN_RPORIGIN=https://openguard.example.com
IAM_MTLS_CERT_FILE=/certs/iam.crt
IAM_MTLS_KEY_FILE=/certs/iam.key
IAM_MTLS_CA_FILE=/certs/ca.crt

# ── Policy Service ───────────────────────────────────────────────────
POLICY_PORT=8082
POLICY_CACHE_TTL_SECONDS=30           # Redis cache TTL for evaluated policies
POLICY_MTLS_CERT_FILE=/certs/policy.crt
POLICY_MTLS_KEY_FILE=/certs/policy.key
POLICY_MTLS_CA_FILE=/certs/ca.crt

# ── Threat Detection ─────────────────────────────────────────────────
THREAT_PORT=8083
THREAT_ANOMALY_WINDOW_MINUTES=60
THREAT_MAX_FAILED_LOGINS=10
THREAT_GEO_CHANGE_THRESHOLD_KM=500
THREAT_MAXMIND_DB_PATH=/data/GeoLite2-City.mmdb
THREAT_MTLS_CERT_FILE=/certs/threat.crt
THREAT_MTLS_KEY_FILE=/certs/threat.key
THREAT_MTLS_CA_FILE=/certs/ca.crt

# ── Audit Service ────────────────────────────────────────────────────
AUDIT_PORT=8084
AUDIT_RETENTION_DAYS=730
AUDIT_HASH_CHAIN_SECRET=change-me     # HMAC secret for audit chain integrity
AUDIT_BULK_INSERT_MAX_DOCS=500        # Max documents per bulk insert
AUDIT_BULK_INSERT_FLUSH_MS=1000       # Max ms before forced flush
AUDIT_MTLS_CERT_FILE=/certs/audit.crt
AUDIT_MTLS_KEY_FILE=/certs/audit.key
AUDIT_MTLS_CA_FILE=/certs/ca.crt

# ── Alerting Service ─────────────────────────────────────────────────
ALERTING_PORT=8085
ALERTING_SLACK_WEBHOOK_URL=
ALERTING_SMTP_HOST=smtp.example.com
ALERTING_SMTP_PORT=587
ALERTING_SMTP_USER=
ALERTING_SMTP_PASS=
ALERTING_SIEM_WEBHOOK_URL=
ALERTING_SIEM_WEBHOOK_HMAC_SECRET=change-me  # HMAC-SHA256 signature on SIEM payloads
ALERTING_MTLS_CERT_FILE=/certs/alerting.crt
ALERTING_MTLS_KEY_FILE=/certs/alerting.key
ALERTING_MTLS_CA_FILE=/certs/ca.crt

# ── Compliance Service ───────────────────────────────────────────────
COMPLIANCE_PORT=8086
COMPLIANCE_REPORT_MAX_CONCURRENT=10
COMPLIANCE_MTLS_CERT_FILE=/certs/compliance.crt
COMPLIANCE_MTLS_KEY_FILE=/certs/compliance.key
COMPLIANCE_MTLS_CA_FILE=/certs/ca.crt

# ── PostgreSQL ───────────────────────────────────────────────────────
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=openguard_app           # application user — limited grants
POSTGRES_PASSWORD=change-me
POSTGRES_DB=openguard
POSTGRES_SSLMODE=verify-full          # never "disable" in staging/production
POSTGRES_SSLROOTCERT=/certs/postgres-ca.crt
POSTGRES_POOL_MIN_CONNS=5
POSTGRES_POOL_MAX_CONNS=25
POSTGRES_POOL_MAX_CONN_IDLE_SECS=300
POSTGRES_POOL_MAX_CONN_LIFETIME_SECS=3600
# Outbox relay uses a dedicated superuser-equivalent for LISTEN/NOTIFY
POSTGRES_OUTBOX_USER=openguard_outbox
POSTGRES_OUTBOX_PASSWORD=change-me

# ── MongoDB ──────────────────────────────────────────────────────────
MONGO_URI_PRIMARY=mongodb://localhost:27017        # write path
MONGO_URI_SECONDARY=mongodb://localhost:27018      # read path
MONGO_DB=openguard
MONGO_AUTH_SOURCE=admin
MONGO_TLS_CA_FILE=/certs/mongo-ca.crt
MONGO_WRITE_POOL_MIN=2
MONGO_WRITE_POOL_MAX=10
MONGO_READ_POOL_MIN=5
MONGO_READ_POOL_MAX=30

# ── Redis ────────────────────────────────────────────────────────────
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=change-me
REDIS_DB=0
REDIS_TLS_CERT_FILE=/certs/redis.crt
REDIS_POOL_SIZE=20
REDIS_MIN_IDLE_CONNS=5

# ── Kafka ────────────────────────────────────────────────────────────
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=openguard
KAFKA_TLS_CA_FILE=/certs/kafka-ca.crt
KAFKA_SASL_MECHANISM=SCRAM-SHA-512
KAFKA_SASL_USER=openguard
KAFKA_SASL_PASSWORD=change-me
KAFKA_PRODUCER_MAX_MESSAGE_BYTES=1048576   # 1MB
KAFKA_CONSUMER_SESSION_TIMEOUT_MS=45000
KAFKA_CONSUMER_HEARTBEAT_MS=3000
KAFKA_CONSUMER_MAX_POLL_RECORDS=500

# ── ClickHouse ───────────────────────────────────────────────────────
CLICKHOUSE_ADDR=localhost:9000
CLICKHOUSE_USER=openguard
CLICKHOUSE_PASSWORD=change-me
CLICKHOUSE_DB=openguard
CLICKHOUSE_TLS_CA_FILE=/certs/clickhouse-ca.crt
CLICKHOUSE_BULK_FLUSH_ROWS=5000
CLICKHOUSE_BULK_FLUSH_MS=2000

# ── Circuit Breakers ─────────────────────────────────────────────────
CB_POLICY_TIMEOUT_MS=50             # policy evaluate request timeout (called by SDK)
CB_POLICY_FAILURE_THRESHOLD=5       # failures before opening
CB_POLICY_OPEN_DURATION_MS=10000    # ms before moving to half-open
CB_IAM_TIMEOUT_MS=200
CB_IAM_FAILURE_THRESHOLD=5
CB_IAM_OPEN_DURATION_MS=15000
CB_CONNECTOR_REGISTRY_TIMEOUT_MS=100
CB_CONNECTOR_REGISTRY_FAILURE_THRESHOLD=5
CB_CONNECTOR_REGISTRY_OPEN_DURATION_MS=10000

# ── OpenTelemetry ────────────────────────────────────────────────────
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SERVICE_NAMESPACE=openguard
OTEL_SAMPLING_RATE=0.1              # 10% in production, 1.0 in development

# ── Frontend (Next.js) ───────────────────────────────────────────────
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me
```

### 5.2 Config Loading Pattern (shared, implement once)

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

// MustJSON parses a JSON env var into dest.
func MustJSON(key string, dest any) {
    v := Must(key)
    if err := json.Unmarshal([]byte(v), dest); err != nil {
        panic(fmt.Sprintf("env var %q is not valid JSON: %v", key, err))
    }
}
```

---

## 6. Multi-Tenancy Model

### 6.1 PostgreSQL Row-Level Security

Every table that stores tenant data **must** have RLS enabled. This is enforced by the migration runner — it refuses to apply any migration that creates a new table with an `org_id` column without also enabling RLS.

#### 6.1.1 Application DB User

The application uses a dedicated PostgreSQL user with no superuser access:

```sql
-- Run once, not in migrations
CREATE ROLE openguard_app LOGIN PASSWORD 'change-me';
GRANT CONNECT ON DATABASE openguard TO openguard_app;
-- Tables are granted individually per migration (see below)
```

#### 6.1.2 RLS Setup (apply to every org-scoped table)

```sql
-- Example: users table
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY; -- applies to table owner too

CREATE POLICY users_org_isolation ON users
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

-- org_id can be empty for system-level queries (e.g. SCIM provisioning before org is set)
-- The 'true' flag makes current_setting return NULL instead of error when not set
-- A NULL org_id means no rows match — fail safe
```

Apply this pattern to: `users`, `api_tokens`, `sessions`, `mfa_configs`, `policies`, `policy_assignments`, `outbox_records` (IAM), `outbox_records` (Policy).

#### 6.1.3 Setting the Tenant Context

The `shared/rls` package manages the tenant context:

```go
// shared/rls/context.go
package rls

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5/pgxpool"
)

type contextKey struct{}

// WithOrgID stores the org ID in the Go context.
func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

// OrgID retrieves the org ID from context. Returns "" if not set.
func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}

// SetSessionVar sets the PostgreSQL session variable for RLS.
// Must be called before every query on a pooled connection.
//
// IMPORTANT: orgID is always passed as a query parameter ($1), never interpolated
// into the SQL string. orgID originates from a JWT claim; string interpolation here
// would be a SQL injection vector. Parameterized form is mandatory per §0.14.
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    if orgID == "" {
        // Unset the variable — this results in no rows for RLS-protected tables
        _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
    return err
}
```

Every repository method that executes a PostgreSQL query must:
1. Acquire a connection from the pool.
2. Call `rls.SetSessionVar(ctx, conn, rls.OrgID(ctx))`.
3. Execute the query.
4. Release the connection.

```go
// Example repository method pattern.
// Naming note (§0.3): within package `repository` the struct is named `Repository`,
// not `UserRepository`. Spec examples use the caller-perspective name
// (repository.UserRepository) for clarity. Implement as `type Repository struct{}`.
func (r *Repository) GetByID(ctx context.Context, id string) (*models.User, error) {
    conn, err := r.pool.Acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquire connection: %w", err)
    }
    defer conn.Release()

    if err := rls.SetSessionVar(ctx, conn, rls.OrgID(ctx)); err != nil {
        return nil, fmt.Errorf("set rls context: %w", err)
    }

    var u models.User
    err = conn.QueryRow(ctx,
        `SELECT id, org_id, email, display_name, status, mfa_enabled,
                scim_external_id, provisioning_status, created_at, updated_at, deleted_at
         FROM users WHERE id = $1 AND deleted_at IS NULL`,
        id,
    ).Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Status,
           &u.MFAEnabled, &u.SCIMExternalID, &u.ProvisioningStatus,
           &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt)
    if err != nil {
        return nil, fmt.Errorf("query user: %w", err)
    }
    return &u, nil
}
```

#### 6.1.4 Tenant Middleware (HTTP)

In the control plane model, `org_id` is never injected by a proxy via a header. Instead it is derived from the connector's API key credential. The `shared/middleware/apikey.go` middleware authenticates the incoming Bearer token, looks up the `ConnectedApp` record in the connector registry, and sets the `org_id` in the Go context:

```go
// shared/middleware/apikey.go
package middleware

import (
    "net/http"
    "strings"
    "github.com/openguard/shared/rls"
)

type ConnectorReader interface {
    GetByKeyHash(ctx context.Context, keyHash string) (*models.ConnectedApp, error)
}

// APIKeyMiddleware authenticates the request using the Bearer API key.
// It derives org_id from the stored ConnectedApp record and sets it in context.
// This replaces the previous X-Org-ID header pattern that required an inline proxy.
func APIKeyMiddleware(connectorRepo ConnectorReader, hasher KeyHasher) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if raw == "" {
                writeError(w, http.StatusUnauthorized, "MISSING_CREDENTIALS", "API key required", r)
                return
            }
            keyHash := hasher.Hash(raw)
            connector, err := connectorRepo.GetByKeyHash(r.Context(), keyHash)
            if err != nil {
                writeError(w, http.StatusUnauthorized, "INVALID_API_KEY", "invalid or unknown API key", r)
                return
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

Downstream repository methods call `rls.SetSessionVar` using `rls.OrgID(ctx)` exactly as before. The only change is how `org_id` enters the context — from a verified credential record rather than an untrusted header.

### 6.2 Per-Tenant Quotas

The control plane enforces two rate limit tiers using Redis sliding window. User traffic no longer passes through OpenGuard, so IP-based and user-based limits are no longer applicable at this layer. Quotas are keyed on connector identity and org aggregate:

```go
// shared/middleware/ratelimit.go

// Two limit keys per request:
// 1. Connector-based: key = "rl:connector:{connector_id}"
// 2. Tenant-based (org aggregate): key = "rl:org:{org_id}"

// Both are checked. The most restrictive applies.
// Tenant quota prevents a single org's connectors from consuming all capacity.
```

Limits are configurable via `CONTROL_PLANE_RATE_LIMIT_CONNECTOR` and `CONTROL_PLANE_TENANT_QUOTA_RPM`.

On quota exceeded: return `429` with body:
```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Request rate limit exceeded",
    "retryable": true,
    "request_id": "...",
    "trace_id": "..."
  }
}
```
And headers: `Retry-After: <seconds>`, `X-RateLimit-Limit: <limit>`, `X-RateLimit-Remaining: 0`.

---

## 7. Transactional Outbox Pattern

This section is the most critical in the document. Every service that publishes Kafka events must implement the Outbox pattern. No exceptions.

### 7.1 Outbox Table (add to every service that publishes events)

```sql
-- Add to each service's PostgreSQL migrations
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,          -- Kafka partition key
    payload      BYTEA NOT NULL,         -- JSON-encoded EventEnvelope
    status       TEXT NOT NULL DEFAULT 'pending', -- pending | published | dead
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

-- RLS on outbox
ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (key = current_setting('app.org_id', true));

CREATE INDEX idx_outbox_status_created ON outbox_records(status, created_at)
    WHERE status = 'pending';

-- NOTIFY trigger so relay wakes up immediately instead of polling
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

// Writer writes events to the outbox table within the caller's transaction.
type Writer struct{}

// Write inserts an EventEnvelope into the outbox within the provided transaction.
// The transaction must already have the RLS session variable set.
func (w *Writer) Write(ctx context.Context, tx pgx.Tx, topic, key string, envelope models.EventEnvelope) error {
    payload, err := json.Marshal(envelope)
    if err != nil {
        return fmt.Errorf("marshal envelope: %w", err)
    }
    _, err = tx.Exec(ctx,
        `INSERT INTO outbox_records (topic, key, payload)
         VALUES ($1, $2, $3)`,
        topic, key, payload,
    )
    return err
}
```

### 7.3 Outbox Relay

```go
// shared/kafka/outbox/relay.go
package outbox

// Relay reads pending outbox records and publishes them to Kafka.
// It uses PostgreSQL LISTEN/NOTIFY to wake up immediately on new records,
// and falls back to polling every 100ms to handle missed notifications.
// The 100ms fallback is implemented with time.NewTicker(100*time.Millisecond)
// inside a select{} — never with time.Sleep (see §0.14).
//
// Guarantees:
// - At-least-once delivery to Kafka (Kafka's idempotent producer handles dedup)
// - Records are marked "published" only after Kafka ack
// - Records that fail 5 times are marked "dead" and sent to outbox.dlq
// - The relay is safe to run as multiple instances (row-level locking via FOR UPDATE SKIP LOCKED)

type Relay struct {
    pool     *pgxpool.Pool
    producer kafka.Producer
    logger   *slog.Logger
}

func (r *Relay) Run(ctx context.Context) error {
    // 1. Start LISTEN on "outbox_new" channel
    // 2. On notification OR 100ms tick, call processBatch
    // 3. processBatch: SELECT ... FOR UPDATE SKIP LOCKED LIMIT 100 WHERE status='pending'
    // 4. For each record: produce to Kafka with idempotent producer
    // 5. On success: UPDATE status='published', published_at=NOW()
    // 6. On failure: UPDATE attempts=attempts+1, last_error=...
    //    If attempts >= 5: UPDATE status='dead', dead_at=NOW(), publish to outbox.dlq
    // 7. All updates in a single transaction per batch
}

// processBatch implements the core relay loop.
// Must use FOR UPDATE SKIP LOCKED to prevent multiple relay instances from double-publishing.
func (r *Relay) processBatch(ctx context.Context) (int, error) {
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return 0, err
    }
    defer tx.Rollback(ctx)

    rows, err := tx.Query(ctx, `
        SELECT id, topic, key, payload, attempts
        FROM outbox_records
        WHERE status = 'pending'
        ORDER BY created_at
        LIMIT 100
        FOR UPDATE SKIP LOCKED
    `)
    // ... publish each, update status, commit
}
```

### 7.4 Business Handler Pattern (with Outbox)

This is the canonical pattern every handler must follow:

```go
// Canonical write handler pattern — do NOT deviate from this.
// Naming note (§0.3): within package `service` the struct is named `Service`,
// not `UserService`. Implement as `type Service struct{}`.
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (*models.User, error) {
    // 1. Acquire connection
    conn, err := s.pool.Acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquire conn: %w", err)
    }
    defer conn.Release()

    // 2. Begin transaction
    tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return nil, fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx) // no-op if committed

    // 3. Set RLS context within the transaction
    if _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", rls.OrgID(ctx)); err != nil {
        return nil, fmt.Errorf("set rls: %w", err)
    }

    // 4. Business operation — write to users table
    user, err := s.repo.CreateUserTx(ctx, tx, input)
    if err != nil {
        return nil, fmt.Errorf("create user: %w", err)
    }

    // 5. Write to outbox IN THE SAME TRANSACTION
    envelope := buildUserCreatedEnvelope(ctx, user)
    if err := s.outboxWriter.Write(ctx, tx, kafka.TopicAuditTrail, user.OrgID, envelope); err != nil {
        return nil, fmt.Errorf("write outbox: %w", err)
    }

    // 6. Commit — both the user row and the outbox record are committed atomically
    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit: %w", err)
    }

    // 7. The relay publishes the outbox record to Kafka asynchronously
    // There is NO direct Kafka.Publish() call here
    return user, nil
}
```

---

## 8. Circuit Breakers & Resilience

### 8.1 Circuit Breaker Implementation

Use `github.com/sony/gobreaker` wrapped in `shared/resilience/breaker.go`:

```go
// shared/resilience/breaker.go
package resilience

import (
    "context"
    "fmt"
    "time"
    "github.com/sony/gobreaker"
    "github.com/openguard/shared/models"
)

type BreakerConfig struct {
    Name            string
    Timeout         time.Duration // request timeout
    MaxRequests     uint32        // max requests in half-open state
    Interval        time.Duration // stat collection window
    FailureThreshold uint32       // failures before opening
    OpenDuration    time.Duration // time before moving to half-open
}

// NewBreaker creates a circuit breaker with standard OpenGuard defaults.
func NewBreaker(cfg BreakerConfig) *gobreaker.CircuitBreaker {
    return gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        cfg.Name,
        MaxRequests: cfg.MaxRequests,
        Interval:    cfg.Interval,
        Timeout:     cfg.OpenDuration,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= cfg.FailureThreshold
        },
        OnStateChange: func(name string, from, to gobreaker.State) {
            // Emit metric: openguard_circuit_breaker_state_change{name, from, to}
            // Emit structured log: level=warn msg="circuit breaker state changed"
        },
    })
}

// Call executes fn through the circuit breaker with a context timeout.
// Returns models.ErrCircuitOpen if the breaker is open.
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
    return result.(T), nil
}
```

### 8.2 Failure Modes (Canonical)

These failure mode decisions are non-negotiable for a security platform:

| Scenario | Required behaviour | Rationale |
|----------|--------------------|-----------|
| Policy service unreachable | **SDK uses cached decision for up to 60s, then denies** — control plane returns `503 POLICY_SERVICE_UNAVAILABLE` | SDK cache provides a brief grace period; after TTL expiry the SDK fails closed |
| IAM service unreachable | **Reject all logins** — return `503` | Cannot authenticate without IAM |
| Connector registry unreachable | **Deny all API key requests** — circuit breaker returns `503` | Cannot validate connector credential without registry |
| Audit service unreachable | **Continue operation, buffer via Outbox** — events will publish when audit recovers | Audit is observability, not a gate |
| Threat detection unreachable | **Continue operation, log warning** | Threat is advisory, not a gate |
| Redis unreachable | **Rate limiting fails open** — allow requests, log error | Availability over rate limiting when Redis is down |
| Kafka unreachable | **Outbox buffers events in PostgreSQL** — writes succeed, events queue | Kafka is not in the write path |
| ClickHouse unreachable | **Compliance reports fail with 503** — no writes blocked | Analytics is read-only |
| Webhook delivery unreachable | **Retry via outbox DLQ** — outbound webhooks queue and retry with backoff | Webhook delivery is async, not in the request path |

### 8.3 Retry Policy

```go
// shared/resilience/retry.go
package resilience

// RetryConfig defines exponential backoff with full jitter.
type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    Retryable   func(error) bool // returns true if the error warrants a retry
}

// DefaultRetry is the standard retry config for idempotent operations.
var DefaultRetry = RetryConfig{
    MaxAttempts: 5,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    10 * time.Second,
    Retryable: func(err error) bool {
        // Retry on network errors, 429, 503
        // Do NOT retry on 400, 401, 403, 404, 409
        return errors.Is(err, models.ErrRetryable)
    },
}

// Do executes fn with retries according to cfg.
// Uses exponential backoff with full jitter: sleep = rand(0, min(MaxDelay, BaseDelay * 2^attempt))
func Do(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error
```

### 8.4 Bulkhead (Concurrency Limiter)

Compliance report generation and audit CSV exports are expensive. Limit concurrency to prevent these from consuming all goroutines:

```go
// shared/resilience/bulkhead.go
package resilience

// Bulkhead limits concurrent executions of a function.
type Bulkhead struct {
    sem chan struct{}
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
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

---

## 9. Phase 1 — Foundation (IAM + Control Plane API)

**Goal:** Running skeleton with enterprise-grade auth and a working control plane API surface. JWT multi-key rotation on the IAM OIDC IdP, RLS enforced, Outbox in place, circuit breakers configured, connector registration operational. At the end of Phase 1: an app can register as a connector, receive an API key, and call the control plane. A user can log in via the IAM OIDC token endpoint and receive a JWT. Every write publishes via the Outbox, not directly to Kafka.

### 9.1 Prerequisites (produce before any service code)

1. `infra/docker/docker-compose.yml` — PostgreSQL 16, MongoDB 7 (primary + secondary replica), Redis 7, Kafka 3.6 + Zookeeper, ClickHouse 24, Jaeger, Prometheus, Grafana.
2. `scripts/gen-mtls-certs.sh` — generates a CA and per-service client certificates using `openssl`. Outputs to `infra/certs/`. Must include certs for: `control-plane`, `connector-registry`, `iam`, `policy`, `threat`, `audit`, `alerting`, `webhook-delivery`, `compliance`.
3. `scripts/create-topics.sh` — idempotent topic creation from `infra/kafka/topics.json`. Must include the three new topics: `connector.events`, `webhook.delivery`, `webhook.dlq`.
4. `Makefile` with targets: `dev`, `test`, `lint`, `build`, `migrate`, `seed`, `load-test`, `certs`.
5. `.env.example` as defined in Section 5.1.

### 9.2 Migration Strategy

Use `golang-migrate/migrate` with these rules:

- Every `.up.sql` must have a corresponding `.down.sql`.
- Migrations are **additive only** in production: add columns (nullable), add indexes, add tables. Never drop or rename in the same migration as adding.
- Every migration that creates a table with `org_id` must include the RLS setup for that table.
- The migration runner verifies checksums — it will refuse to apply a modified historical migration.
- Migration runner at service startup (not as a separate job) — use `migrate.Up()` on startup with a distributed lock (Redis `SET NX` with 30s TTL) to prevent concurrent runs in multi-replica deployments.

```go
// pkg/db/migrations.go (in each service)
func RunMigrations(ctx context.Context, dsn string, redisClient *redis.Client) error {
    // 1. Acquire distributed lock: "migrate-lock:<service-name>"
    // 2. Run golang-migrate Up()
    // 3. Release lock
    // Lock timeout: 120s (long enough for large migrations)
}
```

### 9.3 Control Plane API (`services/control-plane`)

The control plane is the public-facing HTTP service that connected apps call. It replaces the previous inline reverse proxy (`services/gateway`). It does not forward or proxy user traffic — it exposes a governed API surface for security operations.

#### 9.3.1 JWT Multi-Key Rotation

JWT keys are now owned by the IAM service (the OIDC IdP), not the control plane. Keys are stored in `IAM_JWT_KEYS_JSON`. Each key has:
- `kid` — key identifier, included in JWT header.
- `secret` — the signing secret.
- `algorithm` — `HS256` | `RS256`.
- `status` — `active` (sign + verify) | `verify_only` (verify old tokens during rotation window).

```go
// shared/crypto/jwt.go — unchanged; now used by IAM, not by a gateway
package crypto

type JWTKey struct {
    Kid       string `json:"kid"`
    Secret    string `json:"secret"`
    Algorithm string `json:"algorithm"`
    Status    string `json:"status"` // "active" | "verify_only"
}

type JWTKeyring struct {
    keys []JWTKey
}

// Sign uses the first key with status="active".
func (k *JWTKeyring) Sign(claims jwt.Claims) (string, error)

// Verify tries all keys, matching on kid from the token header.
// Returns ErrTokenExpired, ErrTokenInvalid, or nil.
func (k *JWTKeyring) Verify(tokenString string) (jwt.MapClaims, error)
```

**Key rotation procedure** (documented in `docs/runbooks/secret-rotation.md`):
1. Generate new key, add to `IAM_JWT_KEYS_JSON` with `status: "active"`. Set old key to `status: "verify_only"`.
2. Deploy IAM — new tokens are signed with the new key. Old tokens still verify.
3. Wait for `IAM_JWT_EXPIRY_SECONDS` (default 900s / 15min) — all old tokens have expired.
4. Remove the old key from the JSON array.
5. Deploy IAM again.

The script `scripts/rotate-jwt-keys.sh` automates steps 1 and 4.

#### 9.3.2 Control Plane Route Table

The control plane exposes two groups of routes: connector-authenticated routes (Bearer API key) for connected apps, and admin-JWT-authenticated routes for the dashboard.

**Connector API** (authenticated via `APIKeyMiddleware`, all under `/v1/`):

| Method | Path | Auth | Circuit breaker | Description |
|--------|------|------|----------------|-------------|
| `POST` | `/v1/policy/evaluate` | API key (`policy:read`) | `cb-policy` | SDK policy check |
| `POST` | `/v1/events/ingest` | API key (`audit:write`) | — | Batch event push from connected app |
| `GET` | `/v1/scim/v2/Users` | API key (`users:read`) | `cb-iam` | SCIM user list |
| `POST` | `/v1/scim/v2/Users` | API key (`users:write`) | `cb-iam` | SCIM provision user |
| `GET` | `/v1/scim/v2/Users/:id` | API key (`users:read`) | `cb-iam` | SCIM get user |
| `PUT` | `/v1/scim/v2/Users/:id` | API key (`users:write`) | `cb-iam` | SCIM update user |
| `DELETE` | `/v1/scim/v2/Users/:id` | API key (`users:write`) | `cb-iam` | SCIM deprovision user |

**Admin API** (authenticated via JWT bearer from IAM OIDC, all under `/v1/admin/`):

| Method | Path | Circuit breaker | Description |
|--------|------|----------------|-------------|
| `GET` | `/v1/admin/connectors` | `cb-connector-registry` | List registered apps |
| `POST` | `/v1/admin/connectors` | `cb-connector-registry` | Register new connected app |
| `GET` | `/v1/admin/connectors/:id` | `cb-connector-registry` | Get connector detail |
| `PATCH` | `/v1/admin/connectors/:id` | `cb-connector-registry` | Update webhook URL or scopes |
| `DELETE` | `/v1/admin/connectors/:id` | `cb-connector-registry` | Remove connector |
| `POST` | `/v1/admin/connectors/:id/suspend` | `cb-connector-registry` | Suspend connector |
| `POST` | `/v1/admin/connectors/:id/activate` | `cb-connector-registry` | Reactivate connector |
| `GET` | `/v1/admin/connectors/:id/deliveries` | — | Webhook delivery log |
| `POST` | `/v1/admin/connectors/:id/test` | — | Send a test webhook |

**Scope enforcement:** The `ScopeMiddleware` checks `connector.Scopes` (stored in context by `APIKeyMiddleware`) against the required scope for each route. Violation returns `403 INSUFFICIENT_SCOPE`.

When a circuit breaker is open:
```json
{ "error": { "code": "UPSTREAM_UNAVAILABLE", "message": "Service temporarily unavailable", "retryable": true } }
```
With `Retry-After: 10` header.

**Special rule for policy circuit breaker:** When `cb-policy` is open and the SDK calls `/v1/policy/evaluate`, the control plane returns `503 POLICY_SERVICE_UNAVAILABLE`. The SDK falls back to its local cache. Once the cache TTL (`CONTROL_PLANE_POLICY_CACHE_TTL_SECONDS`) expires without a successful re-fetch, the SDK denies the request. It never grants access after cache expiry when it cannot reach the policy service.

#### 9.3.3 Event Ingest Handler

```go
// services/control-plane/pkg/handlers/ingest.go

// IngestRequest is the request body for POST /v1/events/ingest.
type IngestRequest struct {
    Events []IngestEvent `json:"events" validate:"required,min=1,max=500,dive"`
}

type IngestEvent struct {
    ID         string          `json:"id" validate:"required,uuid4"`   // idempotency key
    Type       string          `json:"type" validate:"required"`
    OccurredAt time.Time       `json:"occurred_at" validate:"required"`
    ActorID    string          `json:"actor_id" validate:"required"`
    ActorType  string          `json:"actor_type" validate:"required,oneof=user service system"`
    Payload    json.RawMessage `json:"payload" validate:"required"`
}

func (h *Handler) IngestEvents(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB limit for batches

    var req IngestRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, r, http.StatusBadRequest, "INVALID_JSON", err.Error())
        return
    }
    if err := h.validator.Struct(req); err != nil {
        h.respondValidationError(w, r, err)
        return
    }

    result, err := h.svc.IngestEvents(r.Context(), req)
    if err != nil {
        h.handleServiceError(w, r, err)
        return
    }
    // Returns accepted count and any per-event validation failures
    h.respond(w, http.StatusOK, result)
}
```

Each accepted event is normalized into an `EventEnvelope` with `EventSource: "connector:<connector_id>"` and written to the outbox within a single transaction. The outbox relay then publishes to `TopicConnectorEvents`, which the audit service consumes.

#### 9.3.4 mTLS Between Internal Services

The control plane communicates with upstream internal services (IAM, policy, connector registry) using mTLS. All inbound connections from connected apps use standard TLS (no client cert required — API key is the credential). Internal service-to-service calls continue to require mTLS.

```go
// shared/middleware/mtls.go — unchanged
func NewMTLSServer(certFile, keyFile, caFile string) (*tls.Config, error) {
    // Load service cert + key
    // Load CA for client verification
    // Return tls.Config with ClientAuth: tls.RequireAndVerifyClientCert
}

func NewMTLSClient(certFile, keyFile, caFile string) (*http.Client, error) {
    // Load client cert + key
    // Load CA for server verification
    // Return *http.Client with TLS config
}
```

### 9.4 IAM Service

#### 9.4.1 Database Schema

**001_create_orgs.up.sql**
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE orgs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    slug              TEXT NOT NULL UNIQUE,
    plan              TEXT NOT NULL DEFAULT 'free',    -- free | pro | enterprise
    isolation_tier    TEXT NOT NULL DEFAULT 'shared',  -- shared | schema | shard
    mfa_required      BOOLEAN NOT NULL DEFAULT FALSE,
    sso_required      BOOLEAN NOT NULL DEFAULT FALSE,
    max_users         INT,                             -- NULL = unlimited
    max_sessions      INT NOT NULL DEFAULT 5,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Orgs table is NOT org-scoped — it's a cross-tenant table
-- Only system role can read all orgs; app user can only read its own org
CREATE POLICY orgs_self_read ON orgs FOR SELECT
    USING (id = current_setting('app.org_id', true)::UUID);
ALTER TABLE orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE orgs FORCE ROW LEVEL SECURITY;
```

**002_create_users.up.sql**
```sql
CREATE TABLE users (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id               UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email                TEXT NOT NULL,
    display_name         TEXT NOT NULL DEFAULT '',
    password_hash        TEXT,
    status               TEXT NOT NULL DEFAULT 'active',
    mfa_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method           TEXT,                          -- 'totp' | 'webauthn' | NULL
    scim_external_id     TEXT,
    provisioning_status  TEXT NOT NULL DEFAULT 'complete',
    tier_isolation       TEXT NOT NULL DEFAULT 'shared',
    last_login_at        TIMESTAMPTZ,
    last_login_ip        INET,
    failed_login_count   INT NOT NULL DEFAULT 0,
    locked_until         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_users_org_id    ON users(org_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_email     ON users(email)  WHERE deleted_at IS NULL;
CREATE INDEX idx_users_scim_ext  ON users(org_id, scim_external_id) WHERE scim_external_id IS NOT NULL;

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY users_org_isolation ON users
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**003_create_api_tokens.up.sql**
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
```

**004_create_sessions.up.sql**
```sql
CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    refresh_hash TEXT NOT NULL UNIQUE,
    ip_address   INET,
    user_agent   TEXT,
    country_code TEXT,
    city         TEXT,
    lat          DECIMAL(9,6),
    lng          DECIMAL(9,6),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id) WHERE revoked = FALSE;
CREATE INDEX idx_sessions_org_id  ON sessions(org_id)  WHERE revoked = FALSE;

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY sessions_org_isolation ON sessions
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**005_create_mfa_configs.up.sql**
```sql
CREATE TABLE mfa_configs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    org_id            UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type              TEXT NOT NULL DEFAULT 'totp',
    encrypted_secret  TEXT NOT NULL,    -- AES-256-GCM, includes key ID prefix: "mk1:base64..."
    backup_codes_hash TEXT[] NOT NULL DEFAULT '{}',  -- bcrypt hashes of backup codes
    verified          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE mfa_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE mfa_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY mfa_configs_org_isolation ON mfa_configs
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**006_create_outbox.up.sql**
```sql
-- Outbox table for IAM events
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

CREATE OR REPLACE FUNCTION notify_outbox() RETURNS trigger AS $$
BEGIN PERFORM pg_notify('outbox_new', NEW.id::text); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
    AFTER INSERT ON outbox_records FOR EACH ROW EXECUTE FUNCTION notify_outbox();
```

#### 9.4.2 MFA Encryption (Key Versioning)

TOTP secrets are encrypted with AES-256-GCM. The ciphertext is stored with a key ID prefix so the correct decryption key can be selected during rotation:

```go
// shared/crypto/aes.go
package crypto

type EncryptionKey struct {
    Kid    string `json:"kid"`
    Key    string `json:"key"`    // base64-encoded 32-byte key
    Status string `json:"status"` // "active" | "verify_only"
}

type EncryptionKeyring struct{ keys []EncryptionKey }

// Encrypt encrypts plaintext using the active key.
// Output format: "<kid>:<base64(nonce+ciphertext)>"
func (k *EncryptionKeyring) Encrypt(plaintext []byte) (string, error)

// Decrypt parses the kid prefix, finds the matching key, and decrypts.
// Works for all valid keys (active or verify_only).
func (k *EncryptionKeyring) Decrypt(ciphertext string) ([]byte, error)
```

#### 9.4.3 HTTP API

IAM now serves two distinct roles: the internal user management API (called by the control plane's admin routes via mTLS), and the public-facing OIDC/SAML IdP endpoints (called directly by connected apps and browsers). Both surface areas are hosted on the same service but on separate router groups.

**OIDC/SAML IdP endpoints** (public — standard TLS, no mTLS from client):

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/oauth/authorize` | OIDC authorization endpoint |
| `POST` | `/oauth/token` | OIDC token endpoint (issues JWT + refresh) |
| `GET` | `/oauth/userinfo` | OIDC userinfo endpoint |
| `GET` | `/oauth/jwks` | JSON Web Key Set — public keys for JWT verification |
| `GET` | `/oauth/.well-known/openid-configuration` | OIDC discovery document |
| `POST` | `/saml/acs` | SAML Assertion Consumer Service |
| `GET` | `/saml/metadata` | SAML SP metadata |

**Internal management API** (mTLS, called by control plane only):

| Method | Path | Description | New in v2 |
|--------|------|-------------|-----------|
| `POST` | `/auth/register` | Create org + admin user | — |
| `POST` | `/auth/login` | Password login → JWT + session cookie | — |
| `POST` | `/auth/refresh` | Use session cookie to issue new JWT + reset idle clock | Yes |
| `POST` | `/auth/logout` | Revoke session + clear cookie | — |
| `POST` | `/auth/mfa/enroll` | Begin TOTP/WebAuthn enrollment | WebAuthn |
| `POST` | `/auth/mfa/verify` | Complete enrollment | — |
| `POST` | `/auth/mfa/challenge` | Verify TOTP at login | — |
| `POST` | `/auth/webauthn/register` | Begin WebAuthn registration | Yes |
| `POST` | `/auth/webauthn/register/finish` | Complete WebAuthn registration | Yes |
| `POST` | `/auth/webauthn/login` | Begin WebAuthn login | Yes |
| `POST` | `/auth/webauthn/login/finish` | Complete WebAuthn login | Yes |
| `GET` | `/users` | List users (cursor paginated) | Cursor |
| `POST` | `/users` | Create user | — |
| `GET` | `/users/:id` | Get user | — |
| `PATCH` | `/users/:id` | Update user | — |
| `DELETE` | `/users/:id` | Soft-delete | — |
| `POST` | `/users/:id/suspend` | Suspend | — |
| `POST` | `/users/:id/activate` | Activate | — |
| `GET` | `/users/:id/sessions` | List active sessions | — |
| `DELETE` | `/users/:id/sessions/:sid` | Revoke session | — |
| `DELETE` | `/users/:id/sessions` | Revoke all sessions | Yes |
| `GET` | `/users/:id/tokens` | List API tokens | — |
| `POST` | `/users/:id/tokens` | Create API token | — |
| `DELETE` | `/users/:id/tokens/:tid` | Revoke token | — |
| `POST` | `/users/bulk` | Bulk create/update (SCIM internal) | Yes |
| `GET` | `/orgs/me` | Get current org settings | Yes |
| `PATCH` | `/orgs/me` | Update org settings | Yes |
| `GET` | `/v1/connectors` | List registered connectors | Yes |
| `POST` | `/v1/connectors` | Register new connector | — |
| `GET` | `/v1/connectors/:id` | Get connector details | — |
| `PATCH` | `/v1/connectors/:id` | Update/Suspend connector | — |
| `DELETE` | `/v1/connectors/:id` | Delete connector | — |

**SCIM v2:** Exposed through the control plane at `/v1/scim/v2/*` (proxied to IAM via mTLS). IAM handles the logic. Add `ETag` header support for conditional updates.

#### 9.4.4 Kafka Events (via Outbox)

All events written to `outbox_records` table, relay publishes to Kafka. Payload is `EventEnvelope` with appropriate `Type`:

| Event type | Topic | Payload key fields |
|------------|-------|-------------------|
| `auth.login.success` | `auth.events` | `user_id`, `ip`, `country`, `mfa_used` |
| `auth.login.failure` | `auth.events` | `email`, `ip`, `reason` |
| `auth.login.locked` | `auth.events` | `user_id`, `locked_until` |
| `auth.logout` | `auth.events` | `user_id`, `session_id` |
| `auth.mfa.enrolled` | `auth.events` | `user_id`, `method` |
| `auth.mfa.failed` | `auth.events` | `user_id`, `ip` |
| `auth.token.created` | `auth.events` | `user_id`, `token_id`, `scopes` |
| `auth.token.revoked` | `auth.events` | `user_id`, `token_id` |
| `user.created` | `audit.trail` + `saga.orchestration` | Full user object |
| `user.updated` | `audit.trail` | Changed fields diff |
| `user.deleted` | `audit.trail` + `saga.orchestration` | `user_id`, `org_id` |
| `user.suspended` | `audit.trail` | `user_id`, `reason` |
| `user.scim.provisioned` | `audit.trail` + `saga.orchestration` | SCIM payload |
| `user.scim.deprovisioned` | `audit.trail` + `saga.orchestration` | `user_id`, `scim_id` |

### 9.5 Phase 1 Acceptance Criteria

- [ ] `POST /auth/register` creates org + admin user. Both writes are in one DB transaction with an outbox record.
- [ ] `POST /oauth/token` (IAM OIDC token endpoint) returns a JWT signed with `kid` in header. Refresh token stored as `refresh_hash` (SHA-256).
- [ ] JWT verified by the control plane's admin JWT middleware using multi-key keyring. Token from removed key returns 401.
- [ ] New key added alongside old → old tokens still verify. Old key removed → old tokens return 401.
- [ ] `POST /v1/admin/connectors` registers a connected app and returns a one-time plaintext API key. The key is not retrievable after the response.
- [ ] `Authorization: Bearer <key>` on any `/v1/` connector route authenticates and sets `org_id` via the connector registry lookup.
- [ ] Suspended connector (`PATCH /v1/admin/connectors/:id {status:"suspended"}`) → subsequent requests with that API key return `401 CONNECTOR_SUSPENDED`.
- [ ] `POST /v1/events/ingest` with 10 events → all 10 appear as outbox records in a single transaction; relay publishes within 200ms.
- [ ] Connector with scope `audit:write` calling `POST /v1/policy/evaluate` returns `403 INSUFFICIENT_SCOPE`.
- [ ] RLS enforced: querying `users` with `app.org_id=''` returns zero rows.
- [ ] RLS enforced: two orgs' users are never visible to each other even with a broken `WHERE` clause omitted.
- [ ] Outbox relay publishes events to Kafka within 200ms of commit.
- [ ] Relay handles PostgreSQL restart: events buffered in `outbox_records` are published when relay reconnects.
- [ ] Relay marks records `dead` after 5 failures and publishes to `outbox.dlq`.
- [ ] mTLS: request from non-mTLS client to IAM internal management API is rejected.
- [ ] Passwords hashed with bcrypt cost 12. Raw passwords never appear in logs.
- [ ] TOTP secret stored as `"mk1:base64..."` format, decryptable only with correct key.
- [ ] `go test ./... -race` passes.
- [ ] `docker compose up` starts all infra and services healthy.

---

## 10. Phase 2 — Policy Engine

**Goal:** Policy evaluation is the most latency-sensitive path in the system. Target: p99 < 30ms for the SDK's `POST /v1/policy/evaluate` call. Results must be cached in Redis (server-side) and in the SDK's local LRU cache (client-side). The service fails closed when unavailable — after the SDK's local cache TTL expires it denies requests.

### 10.1 Database Schema

Same as v1, plus outbox table (same pattern as IAM 006 migration), plus:

**003_create_policy_cache.up.sql**
```sql
-- Policy evaluation cache log for audit purposes
CREATE TABLE policy_eval_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    user_id      UUID NOT NULL,
    action       TEXT NOT NULL,
    resource     TEXT NOT NULL,
    result       BOOLEAN NOT NULL,
    policy_ids   UUID[] NOT NULL DEFAULT '{}',
    latency_ms   INT NOT NULL,
    cached       BOOLEAN NOT NULL DEFAULT FALSE,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_eval_org_user ON policy_eval_log(org_id, user_id, evaluated_at DESC);

ALTER TABLE policy_eval_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_eval_log FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_eval_org_isolation ON policy_eval_log
    USING (org_id = current_setting('app.org_id', true)::UUID);
```

### 10.2 Redis Caching for Evaluate

Policy evaluation results are cached in Redis:

```
Key:   "policy:eval:{org_id}:{sha256(action+resource+user_id+user_groups)}"
Value: JSON { "permitted": bool, "matched_policies": [...], "reason": "..." }
TTL:   POLICY_CACHE_TTL_SECONDS (default: 30)
```

Cache is invalidated on any `policy.changes` Kafka event for the org. The policy service subscribes to its own topic as a consumer and calls `DEL` on all keys matching `policy:eval:{org_id}:*` (use Redis `SCAN` not `KEYS`).

### 10.3 Evaluator Interface

The evaluator must be called within the circuit breaker. The control plane calls the policy service via mTLS HTTP when handling `POST /v1/policy/evaluate` from the SDK. The SDK also maintains a local LRU cache (keyed on `org_id + sha256(principal+action+resource+context)`) with TTL = `SDK_POLICY_CACHE_TTL_SECONDS`. A cache hit on the SDK side never reaches the control plane. A cache hit on the Redis (server) side skips the policy engine's DB query. Policy service does not expose gRPC in Phase 2 (scaffold it in Phase 6).

### 10.4 Policy Management API (Admin Surface)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/policies` | List all policies (RBAC, IP, etc.) |
| `POST` | `/v1/policies` | Create new policy |
| `GET` | `/v1/policies/:id` | Get policy rules |
| `PUT` | `/v1/policies/:id` | Update policy rules |
| `DELETE` | `/v1/policies/:id` | Delete policy |
| `GET` | `/v1/policy/eval-logs` | List recent evaluation history |

### 10.5 Policy Evaluation API (Public / Connector Surface)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/policy/evaluate` | Sub-30ms real-time decision |

### 10.4 Phase 2 Acceptance Criteria

- [ ] `POST /v1/policy/evaluate` p99 < 30ms under 500 concurrent requests (k6 test).
- [ ] Second evaluate call for same inputs returns `cached: true` in eval log (Redis hit).
- [ ] SDK local cache hit does not reach the control plane (verify with zero requests to `/v1/policy/evaluate` on repeated identical calls within TTL).
- [ ] Policy change invalidates Redis cache: evaluate returns fresh result within 1s.
- [ ] Policy service circuit breaker open → control plane returns `503 POLICY_SERVICE_UNAVAILABLE`; SDK falls back to local cache, then denies after TTL.
- [ ] All policy writes go through outbox. Cache invalidation via Kafka consumer.
- [ ] Policy push: a webhook is delivered to registered connectors with scope `policy:read` within 5s of a policy change.

---

## 11. Phase 3 — Event Bus, Outbox Relay & Audit Log

**Goal:** Kafka is fully operational. The Outbox relay runs in every service. The Audit Log consumes all events with bulk inserts, hash chaining, and CQRS read/write split.

The audit log has two ingest paths:

1. **Internal events** (IAM, policy, threat, alerting services) — written to each service's `outbox_records` table within the same DB transaction as the business operation. The Outbox relay publishes to the appropriate Kafka topic (`auth.events`, `policy.changes`, `audit.trail`, etc.). The audit service consumes these.

2. **Connected app events** (`POST /v1/events/ingest`) — validated and normalized by the control plane, written to its outbox table with `EventSource: "connector:<connector_id>"`, relayed to `TopicConnectorEvents`. The audit service consumes `connector.events` with the same bulk writer and hash chain logic as internal events.

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

Replication factor 3 requires 3 Kafka brokers in staging/production. Docker Compose uses single-broker (replication=1) for local dev. The `create-topics.sh` script detects broker count and adjusts replication factor accordingly.

### 11.2 Audit Log Service — CQRS Architecture

```
services/audit/
├── pkg/
│   ├── consumer/
│   │   ├── bulk_writer.go      # Buffers + bulk-inserts to MongoDB primary
│   │   └── hash_chain.go       # Computes and stores hash chain
│   ├── repository/
│   │   ├── write.go            # Uses MONGO_URI_PRIMARY
│   │   └── read.go             # Uses MONGO_URI_SECONDARY
│   ├── handlers/
│   │   ├── events.go           # GET /audit/events (reads from secondary)
│   │   └── export.go           # Export jobs (reads from secondary)
│   └── integrity/
│       └── verifier.go         # Hash chain verification
```

#### 11.2.1 Bulk Insert with Backpressure

```go
// pkg/consumer/bulk_writer.go
type BulkWriter struct {
    coll        *mongo.Collection    // primary
    buffer      []mongo.WriteModel
    mu          sync.Mutex
    maxDocs     int                  // AUDIT_BULK_INSERT_MAX_DOCS (default: 500)
    flushAfter  time.Duration        // AUDIT_BULK_INSERT_FLUSH_MS (default: 1000ms)
    metrics     *BulkWriterMetrics
}

// Add appends a document to the buffer. Flushes if maxDocs reached.
func (b *BulkWriter) Add(ctx context.Context, doc AuditEvent) error

// flush writes the buffer to MongoDB as a bulk write (ordered=false for throughput).
// Called by Add when buffer is full, or by the ticker on flushAfter interval.
func (b *BulkWriter) flush(ctx context.Context) error {
    // mongo.Collection.BulkWrite with options.BulkWrite().SetOrdered(false)
    // Ordered=false: inserts continue even if one document fails (e.g. duplicate event_id)
    // Log failed documents individually, don't fail the entire batch
}
```

#### 11.2.2 Hash Chain Integrity

Each audit event stores a chain hash linking it to the previous event for the same `org_id`. This makes tampering with or deleting an event detectable.

```go
// pkg/consumer/hash_chain.go

// ChainHash computes HMAC-SHA256 of: prev_hash + event_id + org_id + type + occurred_at
// Key: AUDIT_HASH_CHAIN_SECRET
func ChainHash(secret, prevHash string, event AuditEvent) string

// AuditEvent in MongoDB includes:
type AuditEvent struct {
    // ... standard fields ...
    ChainHash     string    `bson:"chain_hash"`      // hash of this event
    PrevChainHash string    `bson:"prev_chain_hash"` // hash of previous event for this org
    ChainSeq      int64     `bson:"chain_seq"`       // monotonically increasing per org
}

// The integrity verifier (GET /audit/integrity) recomputes the chain
// and reports any gaps or hash mismatches.
```

#### 11.2.3 MongoDB Schema

Collection: `audit_events`

```js
db.audit_events.createIndex({ org_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ org_id: 1, type: 1, occurred_at: -1 })
db.audit_events.createIndex({ actor_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ event_id: 1 }, { unique: true })
db.audit_events.createIndex({ org_id: 1, chain_seq: 1 })  // for integrity checks
db.audit_events.createIndex({ occurred_at: 1 }, { expireAfterSeconds: <retention> })
```

#### 11.2.4 HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/audit/events` | List events (cursor paginated, all filters) |
| `GET` | `/audit/events/:id` | Get single event |
| `POST` | `/audit/export` | Trigger async CSV/JSON export |
| `GET` | `/audit/export/:job_id` | Poll export job status |
| `GET` | `/audit/export/:job_id/download` | Stream download when complete |
| `GET` | `/audit/integrity` | Verify hash chain for org (returns gaps/mismatches) |
| `GET` | `/audit/stats` | Event counts by type and day (for dashboard) |

### 11.3 Phase 3 Acceptance Criteria

- [ ] Kafka consumer processes 50,000 events/s (k6 + Kafka producer load test).
- [ ] Bulk writer flushes ≤ 500 docs per batch, ≤ 1000ms flush interval.
- [ ] Event from IAM login appears in MongoDB audit within p99 2s.
- [ ] Duplicate `event_id` skipped, no error.
- [ ] Dead-letter documents in `audit_dead_letter` after 5 consumer failures.
- [ ] `GET /audit/events` reads from MongoDB secondary (verify with `explain()`).
- [ ] `GET /audit/integrity` returns `ok: true` on a clean chain.
- [ ] Manually deleting a document causes `GET /audit/integrity` to report a gap.
- [ ] Hash chain breaks are reported in Prometheus metric `audit_chain_integrity_failures_total`.

---

## 12. Phase 4 — Threat Detection & Alerting

**Goal:** Real-time detection via Redis-backed counters. Composite risk scoring. Saga-based alert lifecycle. SIEM payloads signed with HMAC.

### 12.1 Threat Detection Service

Identical detector interfaces to v1, with these additions:

**Account takeover detector** (`ato.go`):
- Detects: login success from a new device/browser fingerprint after a recent password change.
- Signal: `auth.login.success` events where `user_agent` doesn't match any session in the last 30 days AND `password_changed_within_24h = true` in the payload.
- Risk score: 0.7.

**Privilege escalation detector** (`priv_escalation.go`):
- Detects: user granted admin role within 60 minutes of a new login.
- Signal: `policy.changes` event with action `role.grant` where `target_user_id` logged in within 60 minutes.
- Risk score: 0.9.

All detector results feed the composite scorer (same formula as v1). Composite score ≥ 0.5 → alert. Composite ≥ 0.8 → HIGH severity. Composite ≥ 0.95 → CRITICAL.

### 12.2 Alert Lifecycle (Saga)

Alert creation, acknowledgement, and resolution form a saga published on `saga.orchestration`:

```
threat.alert.created   → saga step 1: alert persisted in MongoDB
                       → saga step 2: notification enqueued (notifications.outbound)
                       → saga step 3: SIEM webhook fired (if configured)
                       → saga step 4: audit event written (audit.trail)
threat.alert.acknowledged → updates alert status, publishes audit event
threat.alert.resolved     → updates alert status, computes MTTR, publishes audit event
```

MTTR (mean time to resolve) is tracked per org per severity for the compliance dashboard.

### 12.3 Threat & Alerting API (Admin Surface)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/threats/alerts` | List security alerts (status, severity filters) |
| `GET` | `/v1/threats/alerts/:id` | Get alert details + saga status |
| `POST` | `/v1/threats/alerts/:id/acknowledge` | Mark alert as acknowledged |
| `POST` | `/v1/threats/alerts/:id/resolve` | Mark alert as resolved |
| `GET` | `/v1/threats/stats` | Alert counts and MTTR per org |
| `GET` | `/v1/threats/detectors` | List active detectors and risk weights |

### 12.3 SIEM Webhook Signing

Every SIEM webhook POST includes:
```
X-OpenGuard-Signature: sha256=<hmac-sha256-hex>
X-OpenGuard-Delivery: <uuid>
X-OpenGuard-Timestamp: <unix seconds>
```

HMAC is computed over `timestamp.payload` using `ALERTING_SIEM_WEBHOOK_HMAC_SECRET`. Replay protection: reject requests where `abs(now - timestamp) > 300` seconds. Document in `docs/api/webhooks.md`.

### 12.4 Phase 4 Acceptance Criteria

- [ ] 11 failed logins within window produce HIGH alert in MongoDB within 3s.
- [ ] Privilege escalation detector fires within 5s of role grant event.
- [ ] SIEM webhook POST includes valid HMAC signature.
- [ ] Alert saga completes all 4 steps; all steps produce audit events.
- [ ] MTTR is computed and stored on alert resolution.
- [ ] `GET /threats/stats` returns correct open count by severity.

---

## 13. Phase 5 — Compliance & Analytics

**Goal:** ClickHouse receives a bulk-inserted event stream. Report generation is concurrency-limited. PDF output is complete and signed. Analytics queries meet p99 < 100ms.

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
PARTITION BY (toYYYYMM(occurred_at), org_id)
ORDER BY (org_id, type, occurred_at)
TTL occurred_at + INTERVAL 2 YEAR
SETTINGS index_granularity = 8192;

-- Materialized view for fast dashboard queries
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

The compliance consumer **must not** insert one row per Kafka message. Use a buffered writer:

```go
// Bulk insert config:
// CLICKHOUSE_BULK_FLUSH_ROWS = 5000 (rows per batch)
// CLICKHOUSE_BULK_FLUSH_MS   = 2000 (max ms before forced flush)

// Use ClickHouse's native batch API (clickhouse-go v2):
batch, err := conn.PrepareBatch(ctx, "INSERT INTO events")
for _, event := range bufferedEvents {
    batch.Append(event.EventID, event.Type, ...)
}
batch.Send()
```

Throughput target: 100,000 rows/second. Verify in Phase 8.

### 13.3 Compliance API (Admin Surface)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/compliance/reports` | List generated compliance reports (PDF/JSON) |
| `POST` | `/v1/compliance/reports` | Trigger new report generation (SOC2, GDPR) |
| `GET` | `/v1/compliance/reports/:id` | Get report metadata + download link |
| `GET` | `/v1/compliance/stats` | Global compliance score and daily trends |
| `GET` | `/v1/compliance/posture` | Real-time posture assessment against controls |

### 13.4 Report Generation with Bulkhead

```go
// pkg/reporter/generator.go

// reportBulkhead is a justified package-level var (see §0.2):
// it is a process-lifetime concurrency limiter, initialized once from config
// at startup and never mutated. It is not mutable shared state.
var reportBulkhead = resilience.NewBulkhead(
    config.DefaultInt("COMPLIANCE_REPORT_MAX_CONCURRENT", 10),
)

func (g *Generator) Generate(ctx context.Context, report *Report) error {
    return reportBulkhead.Execute(ctx, func() error {
        return g.generate(ctx, report)
    })
}
```

When bulkhead is full: return `429` with `Retry-After: 30`.

### 13.4 Phase 5 Acceptance Criteria

- [ ] ClickHouse receives 10,000 events in ≤ 3 batches of ≤ 5,000 rows each.
- [ ] Materialized view `event_counts_daily` is populated automatically.
- [ ] `GET /compliance/stats?metric=logins&granularity=day` p99 < 100ms.
- [ ] GDPR report includes all 5 sections and is valid PDF with ToC and page numbers.
- [ ] 11 concurrent report requests: 10 succeed, 11th returns 429.
- [ ] Report includes a digital timestamp (generation time + org name + hash of report content).

---

            "script-src 'self' 'unsafe-inline'",  // Next.js requires unsafe-inline
            "style-src 'self' 'unsafe-inline'",
            "img-src 'self' data: blob:",
            "connect-src 'self' https://api.openguard.example.com",
            "frame-ancestors 'none'",
        ].join('; '),
    },
];
```

### 14.4 Connected Apps Management (`/connectors`)

New admin section for managing registered connected applications. This is the primary UI surface for the control plane's connector registry.

**Page: `/connectors`** — list view:
- Table of registered connectors: name, status badge, scopes, created date, last event timestamp.
- "Register app" button → opens registration modal.
- Per-row actions: view detail, suspend, activate, delete.

**Registration modal (`/connectors/new`):**
- Fields: App name, Webhook URL, Scopes (multi-select checkboxes).
- On submit: `POST /v1/admin/connectors`.
- On success: display API key in a one-time reveal panel with a copy button and a prominent warning that the key will not be shown again. The key must be masked by default (click to reveal).

**Detail page (`/connectors/:id`):**
- Connector metadata and status.
- Edit webhook URL and scopes (`PATCH /v1/admin/connectors/:id`).
- Webhook delivery log: table of recent deliveries (timestamp, event type, HTTP status, latency, retry count). Fetched from `GET /v1/admin/connectors/:id/deliveries`.
- "Send test webhook" button → `POST /v1/admin/connectors/:id/test`.
- Event volume chart: events received per day (last 30 days) from ClickHouse.
- Danger zone: suspend / delete connector.

```ts
// app/(dashboard)/connectors/page.tsx
// app/(dashboard)/connectors/new/page.tsx
// app/(dashboard)/connectors/[id]/page.tsx
```

### 14.5 Authentication Flow Update

The dashboard authenticates via the IAM OIDC IdP rather than posting credentials directly to a proxied endpoint. Use NextAuth.js with a custom OIDC provider pointing to `IAM_OIDC_ISSUER`:

```ts
// app/api/auth/[...nextauth]/route.ts
import NextAuth from 'next-auth';
import { OAuthConfig } from 'next-auth/providers';

const openguardProvider: OAuthConfig<any> = {
    id: 'openguard',
    name: 'OpenGuard',
    type: 'oauth',
    issuer: process.env.IAM_OIDC_ISSUER,
    wellKnown: `${process.env.IAM_OIDC_ISSUER}/.well-known/openid-configuration`,
    clientId: process.env.IAM_OIDC_CLIENT_ID,
    clientSecret: process.env.IAM_OIDC_CLIENT_SECRET,
    checks: ['pkce', 'state'],
};

export const { handlers, auth, signIn, signOut } = NextAuth({
    providers: [openguardProvider],
});
```

### 14.6 Phase 6 Acceptance Criteria

- [ ] Dashboard loads in < 2s on 3G throttled connection (Lighthouse).
- [ ] Threat alert SSE stream delivers new alert to browser within 1s of creation.
- [ ] Audit log table handles 10,000 rows without browser jank (virtual scroll).
- [ ] All security headers present on every response (verify with `curl -I`).
- [ ] Lighthouse accessibility score ≥ 90.
- [ ] Connected app registration flow: register → copy API key → API key authenticates a connector call → API key is not retrievable on page refresh.
- [ ] Connector suspension from dashboard: connector API key returns `401 CONNECTOR_SUSPENDED` within 1s.
- [ ] Webhook delivery log shows last 100 deliveries with correct status and latency.
- [ ] Event volume chart renders correctly for a connector with 0 events and one with 1,000+ events.

---

## 14. Phase 6 — Infra, CI/CD & Observability

### 15.1 Docker Compose

```yaml
# infra/docker/docker-compose.yml
services:
  postgres:
    image: postgres:16-alpine
    environment: [POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB]
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

  mongo-secondary:
    image: mongo:7
    command: mongod --replSet rs0 --bind_ip_all
    volumes: [mongo-secondary-data:/data/db]
    depends_on: [mongo-primary]

  mongo-init:
    image: mongo:7
    depends_on: [mongo-primary, mongo-secondary]
    command: >
      mongosh --host mongo-primary --eval
      "rs.initiate({_id:'rs0', members:[{_id:0,host:'mongo-primary:27017'},{_id:1,host:'mongo-secondary:27017',priority:0}]})"

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes: [redis-data:/data]

  zookeeper:
    image: bitnami/zookeeper:3.9
    environment: [ALLOW_ANONYMOUS_LOGIN=yes]
    volumes: [zookeeper-data:/bitnami/zookeeper]

  kafka:
    image: bitnami/kafka:3.6
    depends_on: [zookeeper]
    environment:
      - KAFKA_CFG_ZOOKEEPER_CONNECT=zookeeper:2181
      - KAFKA_CFG_NUM_PARTITIONS=12
      - KAFKA_CFG_DEFAULT_REPLICATION_FACTOR=1
      - ALLOW_PLAINTEXT_LISTENER=yes
    volumes: [kafka-data:/bitnami/kafka]

  clickhouse:
    image: clickhouse/clickhouse-server:24
    volumes: [clickhouse-data:/var/lib/clickhouse]
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]

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

  # ── OpenGuard services ───────────────────────────────────────────────

  control-plane:
    build: { context: ../../services/control-plane }
    ports: ["8080:8080"]
    depends_on: [postgres, redis, kafka, connector-registry, iam, policy]
    env_file: [../../.env]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health/live"]

  connector-registry:
    build: { context: ../../services/connector-registry }
    ports: ["8090:8090"]
    depends_on: [postgres]
    env_file: [../../.env]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/health/live"]

  iam:
    build: { context: ../../services/iam }
    ports: ["8081:8081"]
    depends_on: [postgres, redis]
    env_file: [../../.env]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8081/health/live"]

  policy:
    build: { context: ../../services/policy }
    ports: ["8082:8082"]
    depends_on: [postgres, redis]
    env_file: [../../.env]

  threat:
    build: { context: ../../services/threat }
    ports: ["8083:8083"]
    depends_on: [kafka, mongo-primary]
    env_file: [../../.env]

  audit:
    build: { context: ../../services/audit }
    ports: ["8084:8084"]
    depends_on: [kafka, mongo-primary]
    env_file: [../../.env]

  alerting:
    build: { context: ../../services/alerting }
    ports: ["8085:8085"]
    depends_on: [kafka, mongo-primary]
    env_file: [../../.env]

  webhook-delivery:
    build: { context: ../../services/webhook-delivery }
    ports: ["8091:8091"]
    depends_on: [kafka, postgres]
    env_file: [../../.env]

  compliance:
    build: { context: ../../services/compliance }
    ports: ["8086:8086"]
    depends_on: [clickhouse, kafka]
    env_file: [../../.env]

  web:
    build: { context: ../../web }
    ports: ["3000:3000"]
    depends_on: [control-plane, iam]
    env_file: [../../.env]

volumes:
  postgres-data: mongo-primary-data: mongo-secondary-data: redis-data:
  zookeeper-data: kafka-data: clickhouse-data: prometheus-data: grafana-data:
```

### 15.2 GitHub Actions CI

**`.github/workflows/ci.yml`:**

```yaml
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
      - name: Check coverage
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Coverage $COVERAGE% is below 70% threshold"
            exit 1
          fi

  go-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout 5m

  sql-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          go install github.com/ryanprior/go-sqllint@latest
          find services -name "*.go" | xargs go-sqllint
          # Fails on any string concatenation in SQL queries

  next-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: web/package-lock.json }
      - run: cd web && npm ci && npm run build && npm run lint

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Go dependency audit
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
      - name: Container scan
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: fs
          severity: CRITICAL,HIGH
          exit-code: 1

  contract-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - name: Run contract tests
        run: go test ./shared/... -run TestContract -v
        # Contract tests verify: EventEnvelope produced by IAM is parseable by Audit consumer
        # Contract tests verify: Policy evaluate request/response shape
```

**`.github/workflows/security.yml`** — runs weekly:
```yaml
name: Security Audit
on:
  schedule: [{ cron: '0 3 * * 1' }]  # Monday 3am UTC
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: govulncheck ./...
      - run: cd web && npm audit --audit-level=high
      - uses: aquasecurity/trivy-action@master
        with: { scan-type: fs, format: sarif, output: trivy.sarif }
      - uses: github/codeql-action/upload-sarif@v3
        with: { sarif_file: trivy.sarif }
```

### 15.3 Prometheus Metrics (Extended)

Every service exposes these metrics in addition to the standard HTTP metrics:

| Metric | Type | Labels | Service |
|--------|------|--------|---------|
| `openguard_outbox_pending_records` | Gauge | `service` | All |
| `openguard_outbox_relay_duration_seconds` | Histogram | `service`, `result` | All |
| `openguard_circuit_breaker_state` | Gauge | `name`, `state` | Control Plane |
| `openguard_rls_session_set_duration_seconds` | Histogram | `service` | All (Postgres) |
| `openguard_kafka_bulk_insert_size` | Histogram | `service` | Audit, Compliance |
| `openguard_kafka_consumer_lag` | Gauge | `topic`, `group` | All consumers |
| `openguard_audit_chain_integrity_failures_total` | Counter | `org_id` | Audit |
| `openguard_policy_cache_hits_total` | Counter | `layer` (`sdk`\|`redis`) | Policy |
| `openguard_policy_cache_misses_total` | Counter | `layer` (`sdk`\|`redis`) | Policy |
| `openguard_threat_detections_total` | Counter | `detector`, `severity` | Threat |
| `openguard_report_generation_duration_seconds` | Histogram | `type`, `format` | Compliance |
| `openguard_report_bulkhead_rejected_total` | Counter | — | Compliance |
| `openguard_connector_api_key_auth_total` | Counter | `result` (`ok`\|`invalid`\|`suspended`) | Control Plane |
| `openguard_events_ingested_total` | Counter | `connector_id`, `org_id` | Control Plane |
| `openguard_webhook_delivery_duration_seconds` | Histogram | `connector_id`, `result` | Webhook Delivery |
| `openguard_webhook_delivery_attempts_total` | Counter | `connector_id`, `result` | Webhook Delivery |
| `openguard_webhook_dlq_total` | Counter | `connector_id` | Webhook Delivery |

### 15.4 Alertmanager Rules

In `infra/monitoring/alerts/openguard.yml`:

```yaml
groups:
- name: openguard
  rules:
  - alert: OutboxLagHigh
    expr: openguard_outbox_pending_records > 1000
    for: 2m
    labels: { severity: warning }
    annotations:
      summary: "Outbox relay is lagging ({{ $value }} pending records)"

  - alert: CircuitBreakerOpen
    expr: openguard_circuit_breaker_state{state="open"} == 1
    for: 30s
    labels: { severity: critical }
    annotations:
      summary: "Circuit breaker {{ $labels.name }} is open"

  - alert: KafkaConsumerLagHigh
    expr: openguard_kafka_consumer_lag > 50000
    for: 5m
    labels: { severity: warning }

  - alert: AuditChainIntegrityFailure
    expr: increase(openguard_audit_chain_integrity_failures_total[5m]) > 0
    labels: { severity: critical }
    annotations:
      summary: "Audit chain integrity violation detected for org {{ $labels.org_id }}"

  - alert: PolicyServiceDown
    expr: up{job="policy"} == 0
    for: 30s
    labels: { severity: critical }
    annotations:
      summary: "Policy service is down — all policy evaluations are failing closed"
```

### 15.5 Helm Chart

`infra/k8s/helm/openguard/` with:
- `Deployment` per service with `minReadySeconds: 30` and `RollingUpdate` strategy.
- `PodDisruptionBudget` per service: `minAvailable: 1`.
- `HorizontalPodAutoscaler` for control-plane, IAM, policy, threat: scale on CPU 70% AND custom metric `openguard_kafka_consumer_lag`.
- `NetworkPolicy`: each internal service (IAM, policy, audit, threat, alerting, compliance) only accepts inbound traffic from the control-plane (via mTLS). The connector-registry only accepts inbound traffic from the control-plane. The control-plane is the only service with an externally reachable `LoadBalancer` Service. IAM's OIDC endpoints (`/oauth/*`) get a separate `Ingress` resource (public TLS, no client cert) to serve browser and connected-app OIDC flows.
- `ServiceAccount` per service with least-privilege RBAC.
- `Secret` references via `external-secrets.io` annotations for production. Plain secrets for dev.
- `topologySpreadConstraints`: spread pods across 3 AZs.

### 14.6 Phase 6 Acceptance Criteria

- [ ] `docker compose up` starts all infra healthy with MongoDB replica set initialized.
- [ ] `go test ./... -race` passes in CI with ≥ 70% coverage.
- [ ] `govulncheck ./...` reports no CRITICAL vulnerabilities.
- [ ] SQL lint catches a deliberately injected string concatenation in a test file.
- [ ] Contract test verifies IAM event is parseable by Audit consumer.
- [ ] Prometheus scrapes all 8 services. All `openguard_*` metrics appear in Grafana.
- [ ] `OutboxLagHigh` alert fires when relay is artificially stopped.
- [ ] `CircuitBreakerOpen` alert fires when policy service is killed.
- [ ] `helm lint` and `helm template` pass without warnings.

---

## 15. Phase 7 — Security Hardening & Secret Rotation

### 16.1 HTTP Security Middleware (all services)

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

### 16.2 SSRF Protection

The SIEM webhook URL (`ALERTING_SIEM_WEBHOOK_URL`) is validated on startup and on update to prevent SSRF:

```go
// pkg/service/alerting.go
func validateWebhookURL(raw string) error {
    u, err := url.Parse(raw)
    if err != nil {
        return err
    }
    if u.Scheme != "https" {
        return errors.New("webhook URL must use HTTPS")
    }
    // Resolve to IP and block private ranges
    ips, err := net.LookupHost(u.Hostname())
    for _, ip := range ips {
        parsed := net.ParseIP(ip)
        if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
            return fmt.Errorf("webhook URL resolves to private IP %s — SSRF blocked", ip)
        }
    }
    return nil
}
```

### 16.3 Safe Logger (no secret leakage)

```go
// shared/telemetry/logger.go

// sensitiveKeys is a justified package-level var (see §0.2):
// it is a read-only slice initialized once at package load, never mutated,
// and referenced by every service that calls SafeAttr. It contains no state.
var sensitiveKeys = []string{
    "password", "secret", "token", "key", "auth", "credential",
    "private", "bearer", "authorization", "cookie", "session",
}

// SafeAttr returns a slog.Attr with the value redacted if the key is sensitive.
// Use for every log attribute whose key might intersect with secrets (see §0.11).
func SafeAttr(key string, value any) slog.Attr {
    keyLower := strings.ToLower(key)
    for _, sensitive := range sensitiveKeys {
        if strings.Contains(keyLower, sensitive) {
            return slog.String(key, "[REDACTED]")
        }
    }
    return slog.Any(key, value)
}
```

### 16.4 Secret Rotation Runbook

Document in `docs/runbooks/secret-rotation.md`:

**JWT key rotation (zero-downtime):**
1. Generate new key: `scripts/rotate-jwt-keys.sh new`.
2. Update `IAM_JWT_KEYS_JSON` to include both old (`verify_only`) and new (`active`) keys.
3. Rolling deploy IAM. New tokens signed with new key; old tokens still verify.
4. Wait `IAM_JWT_EXPIRY_SECONDS` seconds.
5. Update env to remove old key.
6. Rolling deploy IAM.

**MFA encryption key rotation:**
1. Add new key to `IAM_MFA_ENCRYPTION_KEY_JSON` as `active`, set old to `verify_only`.
2. Deploy IAM.
3. Run `scripts/re-encrypt-mfa.sh` — reads all `mfa_configs`, decrypts with old key, re-encrypts with new key. Runs in batches of 100, waits 50ms between batches to avoid DB overload.
4. Remove old key from JSON. Deploy IAM.

**Connector API key rotation:**
1. Call `DELETE /v1/admin/connectors/:id/api-key` — invalidates the existing key.
2. Call `POST /v1/admin/connectors/:id/api-key` — issues a new key.
3. Update the connected app's configuration with the new key.
4. Verify connectivity: `GET /v1/admin/connectors/:id` → status `active`.
Note: During the window between steps 1 and 3, the connector cannot authenticate. Schedule during a maintenance window or use zero-downtime rotation (issue new key before revoking old) when the connected app supports dual-key configuration.

**Webhook signing secret rotation:**
1. Update `CONTROL_PLANE_WEBHOOK_SIGNING_SECRET` in env (new value alongside old is not possible — this is a single secret per deployment).
2. Deploy control-plane and webhook-delivery services.
3. Notify all connector operators: all webhooks will now carry new signature. Connected apps must update their verification logic before the deploy.
4. Rotate `ALERTING_SIEM_WEBHOOK_HMAC_SECRET` separately on the same schedule.

**Kafka SASL credential rotation:**
1. Add new credential to Kafka ACL without removing old.
2. Update `KAFKA_SASL_PASSWORD` in env. Rolling deploy all services.
3. Remove old credential from Kafka ACL.

### 16.5 Dependency Pinning

`go.sum` must be committed and CI must fail if `go.sum` is not up to date (`go mod verify`). Node dependencies pinned with `package-lock.json` (exact versions, not ranges). `dependabot.yml` configured for weekly auto-PRs for Go and Node.

### 15.6 Phase 7 Acceptance Criteria

- [ ] Security headers on every HTTP response from every service.
- [ ] SSRF: webhook URL `http://localhost/internal` is rejected at configuration time.
- [ ] Safe logger: `password=secret123` does not appear in structured log output.
- [ ] JWT rotation runbook executed end-to-end: old tokens rejected after rotation complete.
- [ ] MFA re-encryption script runs without data loss (verify before/after spot check).
- [ ] `go mod verify` passes in CI.
- [ ] `govulncheck` and `npm audit --audit-level=high` report zero issues.

---

## 16. Phase 8 — Load Testing & Performance Tuning

**Goal:** Verify every SLO from Section 1.2. No phase is "done" until SLOs are met.

### 17.1 k6 Test Scripts

Produce these scripts in `loadtest/`:

**`auth.js`** — login throughput via IAM OIDC token endpoint:
```js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '1m', target: 500 },
        { duration: '3m', target: 2000 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        'http_req_duration{scenario:default}': ['p(99)<150'],
        'http_req_failed': ['rate<0.01'],
    },
};

export default function () {
    const res = http.post(`${__ENV.IAM_URL}/oauth/token`, JSON.stringify({
        grant_type: 'password',
        client_id: __ENV.OIDC_CLIENT_ID,
        username: `user-${__VU}@load-test.example.com`,
        password: 'Load-test-password-123!',
    }), { headers: { 'Content-Type': 'application/json' } });

    check(res, {
        'status is 200': (r) => r.status === 200,
        'has access_token': (r) => JSON.parse(r.body).access_token !== undefined,
    });
    sleep(0.5);
}
```

**`policy-evaluate.js`** — policy evaluation latency via control plane (most critical):
```js
// 10,000 req/s with p99 < 30ms
// Connector Bearer token pre-seeded with scope policy:read
// Test both cache-hit and cache-miss paths
// Cache-hit: same principal+action+resource, p99 target 5ms (Redis hit)
// Cache-miss: unique resource per VU, p99 target 30ms
// SDK local cache hit: same VU second call, 0 network requests
```

**`event-ingest.js`** — connected app event push throughput:
```js
// 20,000 req/s at POST /v1/events/ingest
// Each request: batch of 10 events
// p99 < 50ms
// Verify: all events appear in audit log within 5s
```

**`audit-query.js`** — read path under load:
```js
// 1,000 req/s GET /audit/events with various filters
// p99 < 100ms
// Verify MongoDB explains show secondaryPreferred
```

**`kafka-throughput.js`** — event bus capacity:
```js
// Direct Kafka producer (use k6 xk6-kafka extension)
// Produce 50,000 events/s to audit.trail
// Verify consumer lag stays below 10,000 messages
```

### 17.2 Tuning Targets and Actions

Run `make load-test` and address failures:

| SLO failing | Likely cause | Tuning action |
|-------------|-------------|---------------|
| Login p99 > 150ms | bcrypt too slow under load | Increase IAM replicas; bcrypt is CPU-bound |
| Policy evaluate p99 > 30ms | Cache miss on cold start | Pre-warm Redis cache on deployment |
| SDK local cache miss | TTL too short | Increase `SDK_POLICY_CACHE_TTL_SECONDS` |
| Event ingest p99 > 50ms | Outbox write contention | Increase control-plane replicas; tune `POSTGRES_POOL_MAX_CONNS` |
| Audit query p99 > 100ms | Missing MongoDB index | Add compound index, analyze with `explain()` |
| Kafka consumer lag growing | Bulk writer too slow | Increase `AUDIT_BULK_INSERT_MAX_DOCS`, tune MongoDB write concern |
| Memory OOM on IAM pod | Connection pool too large | Reduce `POSTGRES_POOL_MAX_CONNS` per replica, add replicas instead |
| Webhook delivery backlog | Delivery service under-scaled | Increase `webhook-delivery` replicas; tune `WEBHOOK_DELIVERY_TIMEOUT_MS` |

### 16.3 Phase 8 Acceptance Criteria

- [ ] `auth.js` k6 run: p99 login < 150ms at 2,000 req/s, error rate < 1%.
- [ ] `policy-evaluate.js`: p99 < 5ms (Redis cached), p99 < 30ms (uncached) at 10,000 req/s.
- [ ] SDK local cache: second identical call produces 0 outbound HTTP requests (verify with Jaeger traces).
- [ ] `event-ingest.js`: p99 < 50ms at 20,000 req/s, all events appear in audit within 5s.
- [ ] `audit-query.js`: p99 < 100ms at 1,000 req/s.
- [ ] Kafka consumer lag stays < 10,000 during 50,000 events/s burst.
- [ ] All k6 HTML reports committed to `loadtest/results/`.
- [ ] Grafana dashboards show all SLOs met under load (screenshot in docs).

---

## 17. Phase 9 — Documentation & Runbooks

### 18.1 Required Documents

**`README.md`** — must contain:
- One-sentence project description.
- Feature matrix (what OpenGuard does vs Atlassian Guard).
- Quick start: `git clone`, `cp .env.example .env`, `make dev` — working in < 5 minutes.
- Architecture diagram (Mermaid) — must show the control plane model: connected apps calling OpenGuard, not traffic flowing through it.
- SLO table (from Section 1.2).
- License and contributing links.

**`docs/architecture.md`** — must contain:
- Component diagram (Mermaid C4 level 2) — showing control plane, connector registry, IAM OIDC IdP, and SDK as distinct components.
- Connector registration and API key authentication flow diagram.
- Event ingest flow diagram (both internal outbox path and connected app push path).
- Transactional Outbox flow diagram.
- Outbound webhook delivery flow diagram (outbox → Kafka → webhook-delivery service → connector).
- RLS enforcement diagram.
- Circuit breaker state machine diagram.
- SDK local cache + server-side Redis cache layering diagram.
- Saga choreography diagram (user provisioning).
- Database ER diagram (Mermaid erDiagram) for each service.

**`docs/contributing.md`** — must contain:
- Local dev setup (Docker Compose).
- Makefile targets explained.
- Adding a new Kafka consumer.
- Adding a new threat detector (with template).
- Adding a new compliance report type.
- Adding a new RLS-protected table (checklist).
- Adding a new control plane route (scope, middleware chain, circuit breaker).
- PR requirements: tests, lint, contract test if schema changes.
- Commit format: Conventional Commits.

**OpenAPI specs** — `docs/api/<service>.openapi.json` for all services, valid OpenAPI 3.1, passing `redocly lint`. Must include `control-plane.openapi.json` and `connector-registry.openapi.json`.

### 18.2 Operational Runbooks

`docs/runbooks/` must contain:

| File | Scenario |
|------|----------|
| `kafka-consumer-lag.md` | Consumer lag > 50k. Steps: check bulk writer, scale consumers, check MongoDB write saturation. |
| `circuit-breaker-open.md` | Circuit breaker fired. Steps: identify failing service, check health endpoints, manual reset procedure. |
| `audit-hash-mismatch.md` | Integrity check fails. Steps: identify affected org, time range, gap analysis, escalation path. |
| `secret-rotation.md` | Full rotation procedures for all secret types including connector API keys and webhook signing secrets. |
| `outbox-dlq.md` | Messages in `outbox.dlq`. Steps: inspect, replay, investigate root cause. |
| `postgres-rls-bypass.md` | If a query returns cross-tenant data (must never happen). Incident response. |
| `load-shedding.md` | Under extreme load. Steps: increase rate limits temporarily, scale services, shed non-critical consumers. |
| `connector-suspension.md` | How to suspend a misbehaving connector immediately. Steps: `PATCH /v1/admin/connectors/:id`, verify 401 responses, investigate event log for abuse pattern. |
| `webhook-delivery-failure.md` | Connector not receiving webhooks. Steps: check delivery log, inspect DLQ, verify connector URL is reachable, re-enable after fix. |

### 17.3 Phase 9 Acceptance Criteria

- [ ] `make dev` runs to a working state on a clean machine following `README.md` only.
- [ ] All OpenAPI specs pass `redocly lint` including `control-plane.openapi.json`.
- [ ] Architecture Mermaid diagrams render in GitHub Markdown.
- [ ] All 9 runbooks are present and reviewed by a second engineer (or simulated review by a second LLM pass).
- [ ] `docs/contributing.md` — adding a new detector by following the guide produces a passing test.
- [ ] `docs/contributing.md` — adding a new control plane route by following the guide produces a route with correct scope enforcement and circuit breaker.

---

---

## 18. Phase 10 — Content Scanning & DLP

**Goal:** Detect and mitigate sensitive data leakage (PII, credentials, financial data) in real-time. Target: scan latency p99 < 50ms. High-risk findings trigger immediate alerts and can optionally block event ingestion or mask data in the audit log.

### 18.1 Database Schema (services/dlp)

**001_create_dlp_policies.up.sql**
```sql
CREATE TABLE dlp_policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    name         TEXT NOT NULL,
    rules        JSONB NOT NULL,    -- Array of rule objects: {type: "pii", kind: "email", action: "mask"}
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlp_policies_org ON dlp_policies(org_id);
ALTER TABLE dlp_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE dlp_policies FORCE ROW LEVEL SECURITY;
CREATE POLICY dlp_org_isolation ON dlp_policies
    USING (org_id = current_setting('app.org_id', true)::UUID);
```

**002_create_dlp_findings.up.sql**
```sql
CREATE TABLE dlp_findings (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    event_id     UUID NOT NULL,
    rule_id      UUID REFERENCES dlp_policies(id),
    finding_type TEXT NOT NULL,    -- PII | CREDENTIAL | FINANCIAL
    finding_kind TEXT NOT NULL,    -- email | ssn | credit_card | api_key
    action_taken TEXT NOT NULL,    -- monitor | mask | block
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dlp_findings_event ON dlp_findings(event_id);
ALTER TABLE dlp_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE dlp_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY dlp_findings_org_isolation ON dlp_findings
    USING (org_id = current_setting('app.org_id', true)::UUID);
```

### 18.2 Scanning Engine logic

The DLP service consumes from `TopicAuthEvents`, `TopicConnectorEvents`, and `TopicPolicyChanges`. It uses a two-tier scanning approach:

1.  **Regex-based (PII & Financial):**
    - Email: Standard RFC 5322.
    - SSN: `\b\d{3}-\d{2}-\d{4}\b`.
    - Credit Card: Luhn-validated patterns (Visa, MC, Amex).
2.  **Entropy-based (Credentials):**
    - High-entropy strings (>4.5 shannon entropy) for 24+ characters.
    - Known prefixes: `sk_live_`, `AIza...`, etc.

### 18.3 DLP API (Admin Surface)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/dlp/policies` | List all DLP policies (PII, Credentials, etc.) |
| `POST` | `/v1/dlp/policies` | Create new DLP policy |
| `GET` | `/v1/dlp/policies/:id` | Get DLP policy rules |
| `PUT` | `/v1/dlp/policies/:id` | Update DLP rules |
| `DELETE` | `/v1/dlp/policies/:id` | Delete DLP policy |
| `GET` | `/v1/dlp/findings` | List all sensitive data findings |
| `GET` | `/v1/dlp/findings/:id` | Get finding details + JSON path |
| `GET` | `/v1/dlp/stats` | Findings count by type (PII vs Credential) |

### 18.4 Integration Flow

1.  **Ingestion:** SDK/Connector pushes events to Control Plane.
2.  **Sync Scan (Optional):** If policy is set to "Block", the Control Plane calls the DLP service synchronously via mTLS before accepting the event.
3.  **Async Scan (Default):** The DLP service consumes Kafka events, scans in the background, and publishes `dlp.finding.created` if sensitive data is found.
4.  **Masking:** If "Mask" is active, the Audit service consumes the DLP finding and redacts the sensitive fields in MongoDB using the `event_id` and JSON path.

### 18.5 Phase 10 Acceptance Criteria

- [ ] Regex scanner correctly identifies test email and SSN in JSON payloads.
- [ ] Luhn scanner identifies valid credit card numbers and ignores random numbers.
- [ ] Entropy scanner detects AWS secret keys with 100% precision in test data.
- [ ] Sync "Block" policy rejects a `POST /v1/events/ingest` call containing a cleartext credit card.
- [ ] Async "Mask" policy redacts sensitive data in the audit log within 5s of ingestion.
- [ ] DLP finding triggers a HIGH threat alert automatically.

## 19. Cross-Cutting Concerns

### 19.1 Structured Logging

> The full logging standard — slog setup, JSON configuration, the log-or-return rule, `SafeAttr` usage, and the logger middleware pattern — is defined in **§0.11**. This section records the OpenGuard-specific mandatory field set that §0.11 references.

All services use `log/slog` with JSON output in non-dev environments. These fields are **mandatory** on every log entry. They are injected automatically by the logger middleware and `slog.With` base attributes; individual callsites do not repeat them.

| Field | Source | Notes |
|---|---|---|
| `service` | Hardcoded service name constant | e.g. `"iam"`, `"policy"` |
| `env` | `APP_ENV` | `development` \| `staging` \| `production` |
| `level` | Log level | Set by `LOG_LEVEL` env var |
| `msg` | Human-readable message | Required on every entry |
| `trace_id` | OpenTelemetry W3C trace ID | From context via OTel middleware |
| `span_id` | OpenTelemetry span ID | From context via OTel middleware |
| `request_id` | `X-Request-ID` header | Generated by the control plane or the calling SDK, propagated downstream |
| `org_id` | RLS context | Omit for system-level operations |
| `user_id` | JWT claim | Omit for unauthenticated requests |
| `duration_ms` | `time.Since(start).Milliseconds()` | For request-scoped completion logs |

Use `SafeAttr` (Section 16.3) for **all** log attributes whose key might intersect with secrets. Never log raw values for keys matching: `password`, `secret`, `token`, `key`, `auth`, `credential`, `private`, `bearer`, `authorization`, `cookie`, `session`.

### 19.2 Distributed Tracing

Every service initializes OpenTelemetry on startup. Traces propagate via W3C `traceparent` header between services. The Outbox relay injects `trace_id` from the parent context into the `EventEnvelope`, so you can trace from an HTTP request all the way to the audit event in MongoDB.

Sampling rate: `OTEL_SAMPLING_RATE` (0.1 in production, 1.0 in development).

> For the canonical span instrumentation pattern — how to start a span, record errors, and set status codes — see **§0.11 (Distributed Tracing)**.

### 19.3 Graceful Shutdown (30-second window)

```go
// main.go pattern for every service
func main() {
    // ... setup ...

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Order matters:
    // 1. Stop accepting new HTTP requests (server.Shutdown)
    // 2. Stop Kafka consumer (no new messages)
    // 3. Flush Outbox relay (publish buffered records)
    // 4. Flush bulk writer (write buffered MongoDB/ClickHouse docs)
    // 5. Close DB connections
    server.Shutdown(ctx)
    kafkaConsumer.Close()
    outboxRelay.Flush(ctx)
    bulkWriter.Flush(ctx)
    dbPool.Close()
    mongoClient.Disconnect(ctx)
}
```

### 19.4 Health Checks

Every service:
- `GET /health/live` — returns `200 {"status":"ok"}` immediately. Used by Kubernetes liveness probe.
- `GET /health/ready` — checks PostgreSQL, MongoDB, Redis, Kafka connectivity. Returns `200` only if all pass. Returns `503 {"status":"not_ready","checks":{...}}` with per-dependency detail. Used by Kubernetes readiness probe.

Readiness probe failure should trigger circuit breaker state change if the service is a dependency.

### 19.5 Idempotency

All `POST` endpoints that create resources accept an `Idempotency-Key` header (UUID). Cached in Redis for 24 hours: key = `idempotent:{service}:{idempotency-key}`, value = response status + body. On duplicate key: return cached response with `Idempotency-Replayed: true` header.

### 19.6 Request Validation

Use `github.com/go-playground/validator/v10` for struct-level validation. Every handler binds the request body to a typed struct and calls `validate.Struct()` before passing to the service layer. Validation errors return `422 VALIDATION_ERROR` with per-field detail:

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

> The detailed testing patterns — table-driven tests, `require` vs `assert` discipline, fake construction, `testcontainers-go` integration test structure, and the reasoning behind each — are defined in **§0.10**. This section records the required test layers and the CI enforcement thresholds.

| Layer | Tool | Requirement |
|---|---|---|
| Unit tests | `testing` + `testify` | ≥ 70% per package, deterministic, no `time.Sleep` |
| Integration tests | `testcontainers-go` | PostgreSQL + Redis + MongoDB containers, 1 per service |
| Contract tests | Custom (in `shared/`) | Verify producer → consumer schema compatibility |
| API tests | `net/http/httptest` | All happy paths + key error paths |
| Load tests | k6 | All SLOs from Section 1.2 |
| Chaos tests (Phase 8+) | `toxiproxy` | Verify circuit breaker and outbox behavior under network partition |

**Mandatory CI flags:**
```bash
go test ./... -race -count=1 -coverprofile=coverage.out -covermode=atomic -timeout 5m
```

**Coverage gate:** The CI pipeline fails if any package falls below 70% coverage (see Section 15.2). This is a floor; new packages should target higher.

**Test doubles:** Prefer hand-written fakes over generated mocks for narrow interfaces (see §0.10). Generated mocks (`mockery`) are acceptable only for interfaces with more than five methods where a fake would be burdensome to maintain.

**Test behaviour, not implementation:** Tests must not assert on unexported fields, call private methods via reflection, or be coupled to internal data structures. If a refactor breaks a test that is testing the right thing, the test design needs fixing, not the refactor.

---

## 18. Acceptance Criteria (Full System)

When all 10 phases are complete, the following end-to-end scenario must execute without manual intervention. Run it as a CI job on every release.

```
1.  docker compose up -d                               → all services healthy
2.  POST /auth/register                                → org "Acme" + admin user created
3.  POST /oauth/token (IAM OIDC)                       → access token + refresh token issued, kid in JWT header
4.  POST /v1/admin/connectors (admin JWT)              → connector "AcmeApp" registered
                                                         → one-time API key returned (plaintext)
                                                         → GET /v1/admin/connectors/:id returns status=active
5.  POST /v1/admin/connectors (second connector)       → connector "AcmeApp2" registered with scope audit:write only
6.  POST /v1/policies (admin JWT)                      → IP allowlist policy created for org
7.  POST /v1/policy/evaluate (AcmeApp API key)         → blocked IP returns permitted:false
8.  POST /v1/policy/evaluate (same inputs, same key)   → returns permitted:false, cached:true (Redis hit)
9.  POST /v1/policy/evaluate (AcmeApp2 API key)        → returns 403 INSUFFICIENT_SCOPE (scope=audit:write only)
10. POST /v1/events/ingest (AcmeApp API key, 50 events)
                                                       → 200 OK, accepted:50
                                                       → all 50 visible in GET /audit/events within 5s
                                                       → EventSource="connector:<id>" on each event
11. Simulate 11 failed login events via POST /v1/events/ingest
                                                       → HIGH alert in MongoDB within 5s
12. GET /v1/threats/alerts                             → alert visible, severity=high
13. Verify Slack webhook mock received payload         → HMAC signature valid
14. GET /audit/events                                  → all events from steps 2-11 present
15. GET /audit/integrity                               → ok:true, no chain gaps
16. POST /compliance/reports {type:gdpr}               → report job created
17. Poll GET /compliance/reports/:id                   → status=completed within 60s
18. GET /compliance/reports/:id/download               → valid PDF, all 5 GDPR sections
19. PATCH /v1/admin/connectors/:id2 {status:suspended} → AcmeApp2 suspended
20. POST /v1/events/ingest (AcmeApp2 API key)          → 401 CONNECTOR_SUSPENDED
21. POST /v1/admin/connectors/:id/test                 → test webhook delivered, HMAC valid
22. GET /v1/admin/connectors/:id/deliveries            → delivery log shows test + policy-change webhooks
23. JWT key rotation: add new IAM key, deploy IAM      → old tokens still verify
24. JWT key rotation: remove old IAM key, deploy IAM   → old tokens return 401
25. Kill policy service                                → SDK falls back to local cache (60s grace)
                                                         → /v1/policy/evaluate returns 503 POLICY_SERVICE_UNAVAILABLE after TTL
26. Restart policy service                             → circuit breaker resets, evaluate succeeds
27. Kill Kafka                                         → POST /v1/events/ingest succeeds, outbox records pending
28. Restart Kafka                                      → outbox records published within 30s
29. go test ./... -race                                → all tests pass
30. k6 run loadtest/auth.js                            → p99 < 150ms at 2,000 req/s
31. k6 run loadtest/policy-evaluate.js                 → p99 < 30ms uncached, p99 < 5ms Redis cached at 10,000 req/s
32. k6 run loadtest/event-ingest.js                    → p99 < 50ms at 20,000 req/s
33. docker compose down                                → clean shutdown, no data loss
```

Every step is a CI assertion. The release pipeline does not publish unless all 33 steps pass.
