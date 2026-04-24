# §0 — Code Quality Standards

> These standards are CI-enforced (linters, race detector, coverage gate, SQL lint). Every code example in this specification satisfies them. Named exceptions apply only where explicitly stated and scoped.

---

## 0.1 Philosophy

**Readability is a production concern.** Code is read ten times for every time it is written. Optimize for the on-call engineer at 3 AM.

**Boring code is good code.** Go is deliberately unexciting. Propose changes to this document; do not silently diverge in code.

**Consistency beats local optimality.** When the team has agreed on a pattern, use it.

---

## 0.2 Package Design

### One coherent concept per package

If you cannot describe what a package does in one sentence without "and," it needs to be split.

### Service layout

Every service uses `services/<name>/pkg/` for all sub-packages. Services never import each other's `pkg/` packages.

### The `shared/` module

`github.com/openguard/shared` holds genuine cross-service wire contracts. Every package inside it has a real, descriptive name: `kafka/`, `models/`, `rls/`, `resilience/`, `telemetry/`, `crypto/`, `validator/`. Never add `shared/utils/` or `shared/helpers/`.

### No package-level variables for mutable state

```go
// Bad — implicit global, test-order dependent
var defaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

// Good — explicit, injectable
func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{Timeout: timeout}
}
```

**Named exceptions (exhaustive list):**
- Pre-compiled regular expressions (`var emailRE = regexp.MustCompile(...)`).
- `errors.New` sentinel errors.

### No circular imports

The Go toolchain enforces this at compile time.

---

## 0.3 Naming Conventions

### Names eliminate the need for comments

```go
// Bad
d := time.Since(start)

// Good
requestDuration := time.Since(start)
```

### Exported names carry their package prefix — do not repeat it

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

### Interface names describe behavior

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

### Sentinel errors use `Err` prefix

```go
var (
    ErrNotFound      = errors.New("not found")
    ErrAlreadyExists = errors.New("already exists")
    ErrUnauthorized  = errors.New("unauthorized")
    ErrCircuitOpen   = errors.New("circuit breaker open")
    ErrBulkheadFull  = errors.New("bulkhead full")
    ErrRLSNotSet     = errors.New("RLS org_id context not set")
)
```

### Acceptable abbreviations

`ctx`, `cfg`, `err`, `ok`, `id`, `tx`, `db`, `w`, `r` (HTTP handlers). Nothing else abbreviated.

---

## 0.4 Error Handling

### Never discard errors silently

```go
// Unacceptable
_ = db.Close()

// Required
if err := db.Close(); err != nil {
    slog.ErrorContext(ctx, "failed to close db connection", "error", err)
}
```

### Wrap errors once at each layer boundary

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

### Use `errors.Is` and `errors.As` — never string matching

```go
// Good
if errors.Is(err, ErrNotFound) {
    return http.StatusNotFound
}

// Bad
if strings.Contains(err.Error(), "not found") {}
```

### Log or return — never both

Log at the outermost layer (HTTP handler or Kafka consumer). Service and repository layers return errors; they do not log them.

### Panic only for programmer errors and startup invariants

```go
func NewService(repo Repository, events EventPublisher) *Service {
    if repo == nil {
        panic("NewService: repo is required")
    }
    return &Service{repo: repo, events: events}
}
```

### Do not return `nil, nil`

Return `ErrNotFound` or an equivalent sentinel. Callers must never nil-check a returned pointer when the error is also nil.

---

## 0.5 Interfaces

### The consuming package owns the interface

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

### Keep interfaces small. Compose when needed.

### Do not add interfaces prematurely

Add an interface when: you have a second implementation, you need a test double, or you are crossing a significant layer boundary.

---

## 0.6 Concurrency

### Every goroutine has an explicit owner and a termination path

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

### `wg.Add` before the goroutine starts, `wg.Done` via `defer` as the first line

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

---

## 0.7 Context Discipline

### `context.Context` is always the first parameter, never stored in a struct

The sole exception: a long-running background worker where the context is the worker's entire lifetime.

### Never pass `context.Background()` inside a request handler

```go
// Bad — outlives the HTTP request
user, err := h.repo.GetByID(context.Background(), id)

// Good — cancelled when client disconnects
user, err := h.repo.GetByID(r.Context(), id)
```

### Context values are for request-scoped metadata only

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

---

## 0.8 Dependency Injection & Wiring

### Constructor injection — always

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

### `main.go` is the wiring file

All dependency construction belongs in `main.go`. The full dependency graph is visible in one place.

### Use functional options for constructors with more than three parameters

```go
type ClientOption func(*clientOptions)

func WithTimeout(d time.Duration) ClientOption {
    return func(o *clientOptions) { o.timeout = d }
}
```

---

## 0.9 Configuration

### Fail fast at startup — never discover bad config at request time

`config.MustLoad()` panics on any missing or malformed required variable. Use the `shared/config` helpers exclusively. Never call `os.Getenv` from business packages.

### Typed sub-configs

```go
type Config struct {
    Addr     string
    Postgres PostgresConfig
    Redis    RedisConfig
    Kafka    KafkaConfig
    JWT      JWTConfig
}
```

---

## 0.10 Testing

### Test behaviour, not implementation

Tests must not assert on internal state or call unexported methods.

### Table-driven tests for input/output variation

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

### `require` for fatal assertions, `assert` for non-fatal

### Fakes over generated mocks for narrow interfaces

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

### Integration tests use `testcontainers-go` with real databases

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

### CI runs all tests with `-race`; coverage floor is 70% per package

---

## 0.11 Observability

### Structured logging with `log/slog`, JSON in all non-dev environments

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

### `SafeAttr` for any attribute whose key might be sensitive

The `SafeAttr` function (§15.3) redacts values whose key contains any of: `password`, `secret`, `token`, `key`, `auth`, `credential`, `private`, `bearer`, `authorization`, `cookie`, `session`.

### Distributed tracing: start a span at every service call boundary

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

---

## 0.12 HTTP Handler Rules

Handlers are thin: **bind → validate → call service → respond**. Centralize error-to-status-code mapping. Never expose internal error messages to callers. Always set explicit server timeouts. Validate `Content-Type: application/json` on all POST/PUT/PATCH handlers; return `415 Unsupported Media Type` otherwise (SCIM endpoints accept `application/scim+json`).

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

### Centralize error-to-status-code mapping

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

---

## 0.13 Forbidden Patterns

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

---

## 0.14 Code Review Checklist

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

## 0.15 Context Key Convention

All context values MUST use typed unexported keys to prevent collisions:
```go
// CORRECT
type contextKey string
const userIDKey contextKey = "user_id"
ctx = context.WithValue(ctx, userIDKey, userID)

// WRONG — string key collides with any package using "user_id"
ctx = context.WithValue(ctx, "user_id", userID)
```
This applies to all services including example apps. The shared middleware
package provides `GetUserID(ctx)` and `GetOrgID(ctx)` helpers — use these
instead of reading from context directly.
