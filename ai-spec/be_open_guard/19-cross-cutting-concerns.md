# §19 — Cross-Cutting Concerns

---

## 19.1 Structured Logging — Mandatory Fields

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

Use `SafeAttr` (§15.3) for all attributes whose key might contain sensitive keywords.

Log at the handler layer only. Service and repository layers return errors.

---

## 19.2 Distributed Tracing

Every service initializes OpenTelemetry on startup. Traces propagate via W3C `traceparent` header. The Outbox relay injects `trace_id` from the parent context into the `EventEnvelope.TraceID` field, so a trace spans from the original HTTP request through to the audit event in MongoDB.

Sampling: `OTEL_SAMPLING_RATE` (0.1 in production, 1.0 in development).

---

## 19.3 Graceful Shutdown (30-second window)

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

Kubernetes `terminationGracePeriodSeconds` must be set to **45 seconds** (30s for the app + 15s buffer). The Helm chart enforces this.

---

## 19.4 Health Checks

Every service exposes:

- `GET /health/live` — returns `200 {"status":"ok"}` immediately. Kubernetes liveness probe.
- `GET /health/ready` — checks PostgreSQL (ping), MongoDB (ping), Redis (ping), Kafka (metadata fetch). Returns `200` only if all dependencies pass. Returns `503 {"status":"not_ready","checks":{"postgres":"ok","mongo":"fail",...}}`. Kubernetes readiness probe.

Readiness check failures cause the pod to be removed from the load balancer, triggering circuit breaker state changes in calling services.

---

## 19.5 Idempotency

`POST` endpoints that create resources accept an `Idempotency-Key: <UUID>` header. Cached in Redis for 24h:
- Key: `"idempotent:{service}:{idempotency_key}"`
- Value: response status + body (max 64KB; not cached if larger — request proceeds but is not idempotent)
- On duplicate: return cached response with `Idempotency-Replayed: true` header

Excluded endpoints: list/GET endpoints, export download endpoints.

---

## 19.6 Request Validation

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

---

## 19.7 Testing Standards

| Layer | Tool | Requirement |
|---|---|---|
| Unit tests | `testing` + `testify` | ≥ 70% per package; deterministic; no `time.Sleep` |
| Integration tests | `testcontainers-go` | PostgreSQL + Redis + MongoDB real containers per service |
| Contract tests | Custom in `shared/` | Producer → consumer schema compatibility |
| API tests | `net/http/httptest` | Happy paths + key error paths |
| Load tests | k6 | All SLOs from §1.2 |
| Chaos tests (Phase 8+) | `toxiproxy` | Circuit breaker and outbox behavior under partition |

Mandatory CI flags:
```bash
go test ./... -race -count=1 -coverprofile=coverage.out -covermode=atomic -timeout 5m
```
