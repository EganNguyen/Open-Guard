---
name: openguard-go-service
description: >
  Use this skill whenever scaffolding, extending, or reviewing any Go microservice
  in the OpenGuard project. Triggers include: "add a new service", "create a handler",
  "wire up dependencies", "add a repository", "create a constructor", "add config",
  "write a migration", or any request touching services/*, shared/*, or main.go wiring.
  Enforces the mandatory code quality standards from the spec (Section 0) and the
  canonical service layout (Section 3.3). Do NOT use for Kafka consumer loops, RLS
  setup, or HTTP handler patterns — those have dedicated skills.
---

# OpenGuard Go Service Skill

You are implementing production Go code for OpenGuard, a Fortune-500-scale security
control plane. Every decision you make must satisfy the mandatory rules in the spec.
Non-compliance is a CI failure, not a style preference.

---

## 1. Before You Write a Single Line

Answer these questions first:

1. **Which service?** (`services/<name>/`) Does it already exist? If new, scaffold the
   full layout before writing business logic.
2. **Which layer?** Handler → Service → Repository → DB. Never skip layers.
3. **Does shared/ already have this?** Check `shared/kafka/`, `shared/resilience/`,
   `shared/crypto/`, `shared/rls/`, `shared/models/` before writing anything new.
4. **What events does this operation produce?** Every state-changing operation must
   write to the outbox in the same transaction. No exceptions.

---

## 2. Canonical Service Layout

Every service MUST follow this exact structure. Do not deviate:

```
services/<name>/
├── go.mod                    # module: github.com/openguard/<name>
├── main.go                   # ONLY wiring + graceful shutdown. Zero business logic.
├── Dockerfile
├── migrations/
│   ├── 001_<name>.up.sql
│   └── 001_<name>.down.sql   # Required. Every up has a down.
└── pkg/
    ├── config/config.go      # Typed config struct. MustLoad() panics on bad config.
    ├── db/
    │   ├── postgres.go       # Returns *rls.OrgPool, NOT *pgxpool.Pool
    │   ├── mongo.go          # Separate read + write clients
    │   └── migrations.go     # golang-migrate with distributed Redis lock
    ├── outbox/writer.go      # Wraps shared/kafka/outbox.Writer
    ├── handlers/<resource>.go
    ├── service/<resource>.go
    ├── repository/<resource>.go
    └── router/router.go
```

**Package naming rules (non-negotiable):**

| Package | Exported type name |
|---|---|
| `pkg/repository/` | `Repository` |
| `pkg/service/` | `Service` |
| `pkg/handlers/` | `Handler` |
| `pkg/outbox/` | `Writer` |
| `pkg/router/` | `Router` |

Exported names do NOT repeat the package name. `repository.Repository`, not
`repository.UserRepository`.

---

## 3. Constructor Pattern (Mandatory)

Every constructor must:
- Accept dependencies as interfaces (defined in the consuming package, NOT in shared/)
- Panic on nil required dependencies
- Use functional options only when parameter count exceeds 3

```go
// CORRECT
type Service struct {
    repo   userReader      // interface defined in THIS package
    cache  Cache           // interface defined in THIS package
    events eventPublisher  // interface defined in THIS package
    logger *slog.Logger
}

func NewService(repo userReader, cache Cache, events eventPublisher, logger *slog.Logger) *Service {
    if repo == nil {
        panic("NewService: repo is required")
    }
    if logger == nil {
        panic("NewService: logger is required")
    }
    return &Service{repo: repo, cache: cache, events: events, logger: logger}
}

// WRONG — never do this
var globalService *Service  // package-level mutable state
func init() { globalService = &Service{} }  // init() for side effects
```

**Interface definition rule:** Interfaces belong in the consuming package, always.

```go
// WRONG — shared/ defines the interface
// package shared/kafka
type Publisher interface { Publish(...) error }

// CORRECT — service package defines exactly what it needs
// services/iam/pkg/service/user.go
type eventPublisher interface {
    Publish(ctx context.Context, topic, key string, payload []byte) error
}
```

---

## 4. main.go Wiring (The Only Place Dependencies Are Constructed)

`main.go` does exactly four things: load config, construct dependency graph, start
servers/workers, handle graceful shutdown. Business logic never lives here.

```go
func main() {
    cfg := config.MustLoad()

    logger := telemetry.NewLogger(cfg.AppEnv, cfg.LogLevel)
    tp := telemetry.InitTracer(cfg.OTELEndpoint, cfg.ServiceName)
    defer tp.Shutdown(context.Background())

    // Infrastructure
    pgPool   := db.NewOrgPool(db.MustConnectPostgres(cfg.Postgres))
    redisClient := db.MustConnectRedis(cfg.Redis)
    kafkaProducer := kafka.NewSyncProducer(cfg.Kafka)

    // Run migrations (distributed lock via Redis)
    if err := db.RunMigrations(context.Background(), cfg.Postgres.DSN, redisClient, cfg.ServiceName); err != nil {
        logger.Error("migration failed", "error", err)
        os.Exit(1)
    }

    // Business layer
    outboxWriter := outbox.NewWriter()
    repo         := repository.NewRepository(pgPool)
    svc          := service.NewService(repo, redisClient, outboxWriter, logger)
    h            := handlers.NewHandler(svc, logger)
    router       := router.NewRouter(h, cfg)

    server := &http.Server{
        Addr:              cfg.Addr,
        Handler:           router,
        ReadTimeout:       5 * time.Second,
        ReadHeaderTimeout: 2 * time.Second,
        WriteTimeout:      10 * time.Second,
        IdleTimeout:       120 * time.Second,
    }

    // Graceful shutdown (30-second window)
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

    g, ctx := errgroup.WithContext(context.Background())
    g.Go(func() error { return server.ListenAndServe() })
    g.Go(func() error {
        <-quit
        shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        return server.Shutdown(shutCtx)
    })

    if err := g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        logger.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

---

## 5. Configuration (Typed, Fail-Fast)

```go
// pkg/config/config.go
package config

import "github.com/openguard/shared/config"

type Config struct {
    Addr        string
    AppEnv      string
    ServiceName string
    Postgres    config.PostgresConfig
    Redis       config.RedisConfig
    Kafka       config.KafkaConfig
    JWT         config.JWTConfig
    // ... service-specific fields
}

func MustLoad() Config {
    return Config{
        Addr:        config.Default("IAM_PORT", "8081"),
        AppEnv:      config.Must("APP_ENV"),
        ServiceName: "iam",
        Postgres: config.PostgresConfig{
            Host:     config.Must("POSTGRES_HOST"),
            MaxConns: config.DefaultInt("POSTGRES_POOL_MAX_CONNS", 25),
            // ...
        },
    }
}
```

Rules:
- `config.Must()` for required vars — panics at startup if missing
- `config.Default()` for optional vars with documented defaults
- Never call `os.Getenv` from service, handler, or repository packages
- All numeric env vars use `config.MustInt()` / `config.DefaultInt()`

---

## 6. Error Handling Rules

```go
// Repository: translate DB errors to domain sentinels, wrap with context
func (r *Repository) GetByID(ctx context.Context, id string) (*models.User, error) {
    var u models.User
    err := r.pool.QueryRow(ctx, `SELECT ... FROM users WHERE id = $1`, id).Scan(&u.ID, &u.Email)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, models.ErrNotFound   // sentinel, not wrapped
        }
        return nil, fmt.Errorf("query user by id %s: %w", id, err)  // wrap once
    }
    return &u, nil
}

// Service: wrap repository errors with operation context
func (s *Service) GetUser(ctx context.Context, id string) (*models.User, error) {
    user, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("get user: %w", err)  // wrap once more
    }
    return user, nil
}

// Handler: log + map to HTTP status. Never log AND return.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
    user, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "id"))
    if err != nil {
        h.handleError(w, r, err)  // logs here, once
        return
    }
    h.respond(w, r, http.StatusOK, user)
}
```

**Absolute prohibitions:**
- `_ = someFunc()` — never discard errors
- `log.Printf(...)` + `return err` in the same function — log OR return
- `strings.Contains(err.Error(), "not found")` — use `errors.Is`/`errors.As`
- `return nil, nil` — return a sentinel error instead

---

## 7. Concurrency Rules

```go
// Every goroutine has an owner and a termination path
func (s *Service) Run(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)
    g.Go(func() error { return s.runEventLoop(ctx) })
    g.Go(func() error { return s.runCleanup(ctx) })
    return g.Wait()
}

// Polling: ALWAYS time.NewTicker inside select{}, NEVER time.Sleep
func (s *Service) runEventLoop(ctx context.Context) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := s.processBatch(ctx); err != nil {
                s.logger.ErrorContext(ctx, "batch failed", "error", err)
                // do NOT return — keep running unless context cancelled
            }
        }
    }
}

// WaitGroup: Add BEFORE goroutine starts, Done via defer as first line
wg.Add(1)
go func(item Item) {
    defer wg.Done()  // first line inside goroutine
    process(item)
}(item)
```

---

## 8. Context Discipline

```go
// context.Context is ALWAYS first parameter on I/O functions
func (r *Repository) Create(ctx context.Context, input CreateInput) (*models.User, error)

// Context values use typed keys — never raw strings
type contextKey struct{}
func WithOrgID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, contextKey{}, id)
}

// NEVER use context.Background() inside a request handler
// BAD:
user, err := h.repo.GetByID(context.Background(), id)
// GOOD:
user, err := h.repo.GetByID(r.Context(), id)
```

---

## 9. Observability (Required on Every Service Call Boundary)

```go
func (s *Service) CreateUser(ctx context.Context, input CreateInput) (*models.User, error) {
    ctx, span := tracer.Start(ctx, "Service.CreateUser",
        trace.WithAttributes(attribute.String("org.id", rls.OrgID(ctx))),
    )
    defer span.End()

    user, err := s.repo.Create(ctx, input)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, fmt.Errorf("create user: %w", err)
    }

    slog.InfoContext(ctx, "user created",
        telemetry.SafeAttr("user_id", user.ID),
        "org_id", rls.OrgID(ctx),
    )
    return user, nil
}
```

Use `telemetry.SafeAttr` for any log attribute whose key might contain:
`password`, `secret`, `token`, `key`, `auth`, `credential`, `private`, `bearer`,
`authorization`, `cookie`, `session`.

---

## 10. Database Connection — Always OrgPool, Never Raw Pool

```go
// pkg/db/postgres.go

// WRONG — raw pool lets developers forget RLS
func NewPostgres(cfg PostgresConfig) *pgxpool.Pool { ... }

// CORRECT — OrgPool enforces RLS on every acquired connection
func NewOrgPool(cfg PostgresConfig) *rls.OrgPool {
    pool, err := pgxpool.New(context.Background(), cfg.DSN())
    if err != nil {
        panic(fmt.Sprintf("connect postgres: %v", err))
    }
    return rls.NewOrgPool(pool)
}
```

The `rls.OrgPool` calls `SET app.org_id` on every connection acquisition. This means
it is impossible to query tenant tables without the RLS variable being set. Any query
without an org_id in context will return zero rows — the correct fail-safe behavior.

---

## 11. Migration Rules

Every migration file must:
1. Have a corresponding `.down.sql`
2. If creating a table with `org_id`, include the full RLS setup (see RLS skill)
3. Be additive only — no DROP or RENAME of existing columns in the same migration as new columns
4. Use `$1`, `$2` parameters — never string interpolation

```sql
-- 001_create_users.up.sql
CREATE TABLE users (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id  UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email   TEXT NOT NULL,
    -- ... other columns
    UNIQUE (org_id, email)
);

-- Indexes always include WHERE clause for partial indexes on soft-deleted tables
CREATE INDEX idx_users_org_id ON users(org_id) WHERE deleted_at IS NULL;

-- RLS is MANDATORY on every table with org_id
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY users_org_isolation ON users
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

GRANT SELECT, INSERT, UPDATE ON users TO openguard_app;
```

---

## 12. Forbidden Patterns Checklist

Before submitting any code, verify:

- [ ] No `init()` with side effects
- [ ] No `log.Fatal` / `os.Exit` outside `main.go`
- [ ] No `interface{}` / `any` as parameter type (except JSON marshal/unmarshal)
- [ ] No `time.Sleep` in service code
- [ ] No shadowed `err` variables
- [ ] No string concatenation in SQL
- [ ] No `os.Getenv` from business packages
- [ ] No package-level mutable state (unless in named exception list)
- [ ] No direct Kafka publish from business handlers — always via Outbox
- [ ] No `*pgxpool.Pool` in service or repository — always `*rls.OrgPool`
- [ ] No interface defined in `shared/` — always in consuming package
- [ ] No `utils`, `helpers`, `common`, `misc` packages added to `shared/`
