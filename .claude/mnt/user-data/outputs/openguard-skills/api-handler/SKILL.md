---
name: openguard-api-handler
description: >
  Use this skill when writing or reviewing HTTP handlers, middleware, circuit
  breakers, or resilience patterns in OpenGuard. Triggers: "add a route",
  "write a handler", "add middleware", "circuit breaker", "rate limit",
  "validate request", "error response", "scope check", "content-type validation",
  "SCIM endpoint", "health check", "bcrypt worker pool", "bulkhead", or any
  code in services/*/pkg/handlers/, services/*/pkg/router/, or shared/middleware/.
  This skill enforces the thin-handler rule, centralized error mapping, and the
  complete resilience pattern stack.
---

# OpenGuard API Handler & Resilience Skill

Handlers in OpenGuard are deliberately thin. Their only job is: bind → validate →
call service → respond. All business logic lives in the service layer. All
resilience (circuit breakers, bulkheads, retries) is wired in `main.go` and
injected into services.

---

## 1. Handler Structure (Thin Handler Rule)

```go
// pkg/handlers/users.go
package handlers

type Handler struct {
    svc    userService  // interface defined in THIS package
    logger *slog.Logger
    v      *validator.Validate
}

func NewHandler(svc userService, logger *slog.Logger) *Handler {
    if svc == nil {
        panic("NewHandler: svc is required")
    }
    return &Handler{svc: svc, logger: logger, v: validator.New()}
}

// CreateUser: bind → validate → call service → respond.
// Nothing else. No business logic. No DB calls. No Kafka calls.
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    // 1. Validate Content-Type (required on all POST/PUT/PATCH)
    if !isJSONContentType(r) {
        h.respondError(w, r, http.StatusUnsupportedMediaType,
            "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
        return
    }

    // 2. Limit body size (prevent OOM on large payloads)
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit

    // 3. Bind request body
    var req CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
        return
    }

    // 4. Validate (go-playground/validator)
    if err := h.v.Struct(req); err != nil {
        h.respondValidationError(w, r, err)
        return
    }

    // 5. Call service (single call; service owns all business logic)
    user, err := h.svc.CreateUser(r.Context(), service.CreateUserInput{
        Email:       req.Email,
        DisplayName: req.DisplayName,
    })
    if err != nil {
        h.handleServiceError(w, r, err)
        return
    }

    // 6. Respond
    h.respond(w, r, http.StatusCreated, toUserResponse(user))
}
```

---

## 2. Centralized Error Mapping

All error-to-status-code mapping lives in one place. Handlers call `h.handleServiceError`.
Never check errors in individual handlers:

```go
func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
    var valErr *ValidationError
    switch {
    case errors.Is(err, models.ErrNotFound):
        h.respondError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "resource not found")
    case errors.Is(err, models.ErrAlreadyExists):
        h.respondError(w, r, http.StatusConflict, "RESOURCE_CONFLICT", "resource already exists")
    case errors.Is(err, models.ErrUnauthorized):
        h.respondError(w, r, http.StatusForbidden, "FORBIDDEN", "access denied")
    case errors.Is(err, models.ErrCircuitOpen):
        h.respondError(w, r, http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE",
            "service temporarily unavailable")
    case errors.Is(err, models.ErrBulkheadFull):
        w.Header().Set("Retry-After", "30")
        h.respondError(w, r, http.StatusTooManyRequests, "CAPACITY_EXCEEDED",
            "service at capacity, retry later")
    case errors.As(err, &valErr):
        h.respondValidationError(w, r, valErr)
    default:
        // Log unhandled errors at handler layer only — never in service/repo
        slog.ErrorContext(r.Context(), "unhandled service error", "error", err)
        h.respondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR",
            "an unexpected error occurred")
    }
}

// respondError writes the canonical APIError format. Never expose internal messages.
func (h *Handler) respondError(w http.ResponseWriter, r *http.Request,
    status int, code, msg string) {
    h.respond(w, r, status, models.APIError{
        Error: models.APIErrorBody{
            Code:      code,
            Message:   msg,
            RequestID: requestIDFromContext(r.Context()),
            TraceID:   traceIDFromContext(r.Context()),
            Retryable: isRetryable(status),
        },
    })
}
```

---

## 3. Request/Response Helpers

```go
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, status int, body any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if err := json.NewEncoder(w).Encode(body); err != nil {
        slog.ErrorContext(r.Context(), "failed to encode response", "error", err)
    }
}

func isJSONContentType(r *http.Request) bool {
    if r.Method == http.MethodGet || r.Method == http.MethodDelete {
        return true // GET/DELETE have no body
    }
    ct := r.Header.Get("Content-Type")
    return strings.HasPrefix(ct, "application/json")
}

// SCIM endpoints use application/scim+json, not application/json
func isSCIMContentType(r *http.Request) bool {
    ct := r.Header.Get("Content-Type")
    return strings.HasPrefix(ct, "application/scim+json") ||
           strings.HasPrefix(ct, "application/json")
}
```

---

## 4. Router Setup (Middleware Stack Order)

```go
// pkg/router/router.go
func NewRouter(h *handlers.Handler, cfg config.Config) http.Handler {
    r := chi.NewRouter()

    // Global middleware — applied to all routes
    r.Use(shared_middleware.SecurityHeaders)   // HSTS, X-Frame-Options, CSP, etc.
    r.Use(shared_middleware.RequestID)         // X-Request-ID header
    r.Use(shared_middleware.Logger(logger))    // structured request logging
    r.Use(shared_middleware.OTelTracing)       // distributed tracing

    // Health endpoints — no auth required
    r.Get("/health/live",  h.Liveness)
    r.Get("/health/ready", h.Readiness)

    // Connector-authenticated routes (API key + scope check)
    r.Group(func(r chi.Router) {
        r.Use(shared_middleware.APIKey(connectorCache, connectorRepo))
        r.Use(shared_middleware.RateLimit(redisClient, cfg.RateLimits))

        r.With(shared_middleware.RequiredScope("events:write")).
            Post("/v1/events/ingest", h.IngestEvents)

        r.With(shared_middleware.RequiredScope("policy:evaluate")).
            Post("/v1/policy/evaluate", h.EvaluatePolicy)
    })

    // Admin routes (JWT auth)
    r.Group(func(r chi.Router) {
        r.Use(shared_middleware.JWTAuth(keyring))
        r.Use(shared_middleware.RequireAdmin)

        r.Post("/v1/admin/connectors",        h.CreateConnector)
        r.Patch("/v1/admin/connectors/{id}",  h.UpdateConnector)
        r.Delete("/v1/admin/connectors/{id}", h.DeleteConnector)
    })

    // SCIM routes (SCIM bearer token — org_id derived from token, not header)
    r.Group(func(r chi.Router) {
        r.Use(shared_middleware.SCIMAuth(scimTokens))

        r.Get("/v1/scim/v2/Users",           h.SCIMListUsers)
        r.Post("/v1/scim/v2/Users",          h.SCIMCreateUser)
        r.Get("/v1/scim/v2/Users/{id}",      h.SCIMGetUser)
        r.Put("/v1/scim/v2/Users/{id}",      h.SCIMReplaceUser)
        r.Patch("/v1/scim/v2/Users/{id}",    h.SCIMPatchUser)
        r.Delete("/v1/scim/v2/Users/{id}",   h.SCIMDeleteUser)
    })

    return r
}
```

---

## 5. HTTP Server Configuration (Non-Negotiable Timeouts)

```go
server := &http.Server{
    Addr:              cfg.Addr,
    Handler:           router,
    ReadTimeout:       5 * time.Second,    // time to read full request body
    ReadHeaderTimeout: 2 * time.Second,    // time to read headers (Slowloris protection)
    WriteTimeout:      10 * time.Second,   // time to write response
    IdleTimeout:       120 * time.Second,  // keep-alive connection idle timeout
}
```

All four timeouts must be set. Omitting any creates a potential DoS vector.

---

## 6. Health Check Endpoints

```go
// GET /health/live — Kubernetes liveness probe
// Returns 200 immediately. If this is slow, the pod has a deeper problem.
func (h *Handler) Liveness(w http.ResponseWriter, r *http.Request) {
    h.respond(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /health/ready — Kubernetes readiness probe
// Returns 200 only if ALL dependencies are reachable.
// Returns 503 with per-dependency status if any fail.
func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
    checks := map[string]string{
        "postgres": h.checkPostgres(r.Context()),
        "redis":    h.checkRedis(r.Context()),
        "kafka":    h.checkKafka(r.Context()),
    }
    allOK := true
    for _, status := range checks {
        if status != "ok" {
            allOK = false
            break
        }
    }
    if allOK {
        h.respond(w, r, http.StatusOK, map[string]any{"status": "ready", "checks": checks})
    } else {
        h.respond(w, r, http.StatusServiceUnavailable,
            map[string]any{"status": "not_ready", "checks": checks})
    }
}
```

---

## 7. Circuit Breaker (Wrapping Upstream Calls)

Circuit breakers are constructed in `main.go` and injected into services. The service
wraps upstream HTTP calls through the breaker. Never create a circuit breaker inside
a handler or repository.

```go
// shared/resilience/breaker.go

// Call executes fn through the circuit breaker with a context timeout.
// The type parameter T prevents unchecked type assertion panics.
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker,
    timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {

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
    if result == nil {
        var zero T
        return zero, nil
    }
    return result.(T), nil
}

// Usage in service layer:
func (s *PolicyService) Evaluate(ctx context.Context, req PolicyRequest) (PolicyDecision, error) {
    return resilience.Call(ctx, s.policyBreaker, s.policyTimeout,
        func(ctx context.Context) (PolicyDecision, error) {
            return s.policyClient.Evaluate(ctx, req)
        },
    )
}
```

**Breaker configuration (from env, wired in main.go):**

```go
policyBreaker := resilience.NewBreaker(resilience.BreakerConfig{
    Name:             "cb-policy",
    RequestTimeout:   time.Duration(cfg.CBPolicyTimeoutMs) * time.Millisecond,
    FailureThreshold: uint32(cfg.CBPolicyFailureThreshold),
    OpenDuration:     time.Duration(cfg.CBPolicyOpenDurationMs) * time.Millisecond,
    MaxRequests:      2, // probe requests in half-open state
}, logger)
```

**Failure definition:** HTTP 5xx, connection timeout, connection refused.
HTTP 4xx are NOT failures. HTTP 429 IS a failure.

---

## 8. Bulkhead (Concurrency Limiter)

Use bulkheads to protect expensive operations (report generation, compliance exports)
from exhausting system resources:

```go
// shared/resilience/bulkhead.go
type Bulkhead struct{ sem chan struct{} }

func NewBulkhead(max int) *Bulkhead {
    if max <= 0 {
        panic("NewBulkhead: max must be positive")
    }
    return &Bulkhead{sem: make(chan struct{}, max)}
}

// Execute acquires a slot, runs fn, releases the slot.
// If no slot is available and the context is cancelled: ErrBulkheadFull.
func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
    select {
    case b.sem <- struct{}{}:
        defer func() { <-b.sem }()
        return fn()
    case <-ctx.Done():
        return fmt.Errorf("%w", models.ErrBulkheadFull)
    }
}
```

Bulkheads are constructed in `main.go` with size from env config. They are injected
into services via constructors. Never create a bulkhead inside a handler or as a
package-level variable.

---

## 9. bcrypt Worker Pool (IAM Service Only)

bcrypt at cost 12 takes ~300ms. Unbounded goroutines per login starvex the CPU.
The worker pool bounds concurrency:

```go
// services/iam/pkg/service/auth.go
type bcryptJob struct {
    hash     string
    password string
    result   chan error
}

type AuthWorkerPool struct {
    jobs    chan bcryptJob
    size    int
}

func NewAuthWorkerPool(size int) *AuthWorkerPool {
    if size <= 0 {
        panic("NewAuthWorkerPool: size must be positive")
    }
    p := &AuthWorkerPool{
        jobs: make(chan bcryptJob, 100),
        size: size,
    }
    for i := 0; i < size; i++ {
        go p.worker()
    }
    return p
}

func (p *AuthWorkerPool) worker() {
    for job := range p.jobs {
        job.result <- bcrypt.CompareHashAndPassword([]byte(job.hash), []byte(job.password))
    }
}

// Verify submits a bcrypt comparison to the pool.
// Returns ErrBulkheadFull if the job queue is full (backpressure).
// Returns ctx.Err() if the context is cancelled while waiting.
func (p *AuthWorkerPool) Verify(ctx context.Context, hash, password string) error {
    res := make(chan error, 1)
    select {
    case p.jobs <- bcryptJob{hash, password, res}:
        select {
        case err := <-res:
            return err
        case <-ctx.Done():
            return ctx.Err()
        }
    case <-ctx.Done():
        return ctx.Err()
    default:
        return models.ErrBulkheadFull
    }
}
```

Recommended size: `IAM_BCRYPT_WORKER_COUNT` (default `2 × runtime.NumCPU()`).
This pool is constructed in `main.go` and injected into `service.NewAuthService`.
Never call `bcrypt.CompareHashAndPassword` directly in a goroutine.

---

## 10. SCIM Error Format

SCIM endpoints return RFC 7644 §3.12 error format — NOT the standard `APIError` format.
The SCIM handler layer translates all domain errors before responding:

```go
func (h *Handler) scimError(w http.ResponseWriter, r *http.Request, status int, detail string) {
    w.Header().Set("Content-Type", "application/scim+json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]any{
        "schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
        "status":  strconv.Itoa(status),
        "detail":  detail,
    })
}

// SCIM domain error translation
func (h *Handler) handleSCIMError(w http.ResponseWriter, r *http.Request, err error) {
    switch {
    case errors.Is(err, models.ErrNotFound):
        h.scimError(w, r, http.StatusNotFound, "Resource not found")
    case errors.Is(err, models.ErrAlreadyExists):
        h.scimError(w, r, http.StatusConflict, "Resource already exists")
    default:
        h.scimError(w, r, http.StatusInternalServerError, "Internal server error")
    }
}
```

---

## 11. Idempotency Key Middleware

```go
// shared/middleware/idempotency.go
// Applies to POST endpoints that create resources.
// Excluded: GET, DELETE, list endpoints, export download endpoints.
func IdempotencyKey(redisClient *redis.Client, svcName string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            key := r.Header.Get("Idempotency-Key")
            if key == "" {
                next.ServeHTTP(w, r)
                return
            }
            cacheKey := fmt.Sprintf("idempotent:%s:%s", svcName, key)

            // Check cache
            cached, err := redisClient.Get(r.Context(), cacheKey).Bytes()
            if err == nil {
                // Replay cached response
                var resp cachedResponse
                if json.Unmarshal(cached, &resp) == nil {
                    w.Header().Set("Idempotency-Replayed", "true")
                    w.Header().Set("Content-Type", "application/json")
                    w.WriteHeader(resp.Status)
                    w.Write(resp.Body)
                    return
                }
            }

            // Capture the response for caching
            rec := httptest.NewRecorder()
            next.ServeHTTP(rec, r)

            // Cache if response body ≤ 64KB
            if rec.Body.Len() <= 64<<10 {
                data, _ := json.Marshal(cachedResponse{
                    Status: rec.Code,
                    Body:   rec.Body.Bytes(),
                })
                redisClient.Set(r.Context(), cacheKey, data, 24*time.Hour)
            }

            // Write the captured response to the real writer
            for k, vs := range rec.Header() {
                for _, v := range vs {
                    w.Header().Add(k, v)
                }
            }
            w.WriteHeader(rec.Code)
            w.Write(rec.Body.Bytes())
        })
    }
}
```

---

## 12. Handler Checklist

Before submitting any handler code:

- [ ] Handler is thin: bind → validate → service call → respond only
- [ ] `Content-Type: application/json` validated on POST/PUT/PATCH (returns 415)
- [ ] `http.MaxBytesReader` applied to request body
- [ ] All errors routed through `handleServiceError` (never inline status code logic)
- [ ] Internal error messages never exposed to callers
- [ ] Server has all four timeouts: ReadTimeout, ReadHeaderTimeout, WriteTimeout, IdleTimeout
- [ ] SCIM endpoints return RFC 7644 error format, not APIError
- [ ] Circuit breaker constructed in main.go, injected into service
- [ ] Bulkhead constructed in main.go, injected into service
- [ ] bcrypt calls go through AuthWorkerPool, never raw goroutine
- [ ] Idempotency key middleware on state-mutating POST endpoints
- [ ] Health endpoints at `/health/live` and `/health/ready`
