# §8 — Circuit Breakers & Resilience

---

## 8.1 Circuit Breaker Implementation

```go
// shared/resilience/breaker.go
package resilience

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
            // Emit metric: openguard_circuit_breaker_state{name, state}
        },
    })
}

// Call executes fn through the circuit breaker with a context timeout.
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
    if result == nil {
        var zero T
        return zero, nil
    }
    return result.(T), nil
}
```

---

## 8.2 bcrypt Worker Pool

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

---

## 8.3 Failure Mode Table (Non-Negotiable)

| Scenario | Required behavior | Rationale |
|---|---|---|
| Policy service unreachable | SDK uses cached decision up to 60s, then **denies** | Cache provides brief grace; after TTL, fail closed |
| IAM service unreachable | **Reject all logins**, return `503` | Cannot authenticate without IAM |
| Connector registry unreachable | **Deny all API key requests** after Redis cache misses | Cannot validate credential |
| Audit service unreachable | **Continue operation**, buffer via Outbox | Audit is observability, not a gate |
| Threat detection unreachable | **Continue operation**, log warning metric | Threat is advisory, not a gate |
| Redis unreachable | Rate limiting **fails open**; JWT `jti` blocklist **fails closed** (denies auth) | Auth blocklist is a security boundary; Redis MUST be deployed in HA topology |
| Kafka unreachable | **Outbox buffers events in PostgreSQL** | Kafka is not in the write path |
| ClickHouse unreachable | **Compliance reports fail with 503** | Analytics is read-only |
| Webhook delivery unreachable | **Retry via internal loop** with persistence in PostgreSQL | Delivery state survives restarts |
| DLP service unreachable (sync-block mode) | **Reject event ingest** for orgs with `dlp_mode=block` | Sync-block is an explicit opt-in |
| SCIM IdP sends wrong org_id | **Ignore header; derive from token config** | Prevents org_id spoofing |

### JWT `jti` blocklist — three-way return (critical)

```go
// CheckBlocklist returns the blocked status for a given jti.
//
// Return semantics (three-way):
//   blocked=true,  err=nil   → token is in blocklist; deny request
//   blocked=false, err=nil   → key not found in Redis; token not revoked; allow
//   blocked=false, err!=nil  → Redis error; state unknown; MUST deny (fail closed)
//
// Callers MUST treat (err != nil) as (blocked = true):
//   blocked, err := CheckBlocklist(ctx, redisClient, jti)
//   if err != nil || blocked {
//       return ErrTokenRevoked  // fail closed on any uncertainty
//   }
func CheckBlocklist(ctx context.Context, rdb *redis.Client, jti string) (blocked bool, err error) {
    result, err := rdb.Get(ctx, "jti:blocklist:"+jti).Result()
    if err == redis.Nil {
        return false, nil // definitive cache miss: token not revoked
    }
    if err != nil {
        return false, err // Redis unreachable: caller must treat as blocked
    }
    return result == "revoked", nil
}
```

---

## 8.4 Retry Policy

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
        timer := time.NewTimer(delay)
        select {
        case <-ctx.Done():
            timer.Stop()
            return ctx.Err()
        case <-timer.C:
        }
    }
    return lastErr
}
```

---

## 8.5 Bulkhead (Concurrency Limiter)

```go
// shared/resilience/bulkhead.go
package resilience

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

Bulkhead instances are created in `main.go` and injected via constructors. Never package-level variables.

---

## 8.6 PostgreSQL Outbox Write Latency Circuit Breaker

```go
// services/control-plane/pkg/service/ingest.go
//
// New env vars (add to .env.example):
// OUTBOX_INSERT_LATENCY_WARN_MS=100       # p99 threshold before shed (default: 100ms)
// OUTBOX_INSERT_LATENCY_WINDOW_SECONDS=30 # evaluation window (default: 30s)

func (s *IngestService) checkOutboxPressure() error {
    if s.latencyTracker.P99() > s.cfg.OutboxInsertLatencyWarnMs &&
        s.latencyTracker.WindowDuration() >= s.cfg.OutboxInsertLatencyWindowSecs {
        return fmt.Errorf("%w: PostgreSQL outbox write latency elevated (p99 %.0fms > %dms threshold)",
            models.ErrCircuitOpen,
            s.latencyTracker.P99(),
            s.cfg.OutboxInsertLatencyWarnMs,
        )
    }
    return nil
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
    if err := h.svc.checkOutboxPressure(); err != nil {
        w.Header().Set("Retry-After", "10")
        h.respondError(w, r, http.StatusServiceUnavailable, "OUTBOX_PRESSURE",
            "event ingest temporarily degraded, retry shortly")
        return
    }
    // ... normal ingest path
}
```

**Metrics:**

| Metric | Type | Labels |
|---|---|---|
| `openguard_outbox_insert_duration_seconds` | Histogram | `service` |
| `openguard_outbox_pressure_shed_total` | Counter | `service` |

**Alert:**
```yaml
  - alert: OutboxPressureHigh
    expr: histogram_quantile(0.99, rate(openguard_outbox_insert_duration_seconds_bucket[2m])) > 0.1
    for: 2m
    labels: { severity: warning }
    annotations:
      summary: "PostgreSQL outbox insert p99 > 100ms — load shedding may activate"
      runbook: "docs/runbooks/load-shedding.md"
```
