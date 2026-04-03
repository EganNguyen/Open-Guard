# §3 — Repository Layout

---

## 3.1 Full Directory Tree

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
│   ├── breaker.go              # Circuit breaker: defined failure modes (§3.2)
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

## 3.2 SDK Circuit Breaker Specification

`sdk/breaker.go` wraps all control plane calls. The failure modes are precisely defined:

```go
// sdk/breaker.go

// SDKBreaker wraps control plane HTTP calls with circuit-breaker semantics.
// Failure definition: HTTP 5xx, connection timeout, connection refused.
// HTTP 4xx are NOT failures — they are expected protocol responses.
// HTTP 429 (rate limit) IS a failure for circuit breaker purposes.
type SDKBreaker struct {
    cb *gobreaker.CircuitBreaker
}

// BreakerConfig for the SDK:
//   FailureThreshold: 5 consecutive failures
//   OpenDuration:     10s before moving to half-open
//   MaxRequests:      2 requests in half-open state
//   RequestTimeout:   SDK_POLICY_EVALUATE_TIMEOUT_MS (default 100ms)

// PolicyEvaluate calls POST /v1/policy/evaluate through the breaker.
// When the breaker is open:
//   - Returns (cachedDecision, nil) if a cached decision exists for the input.
//   - Returns (DenyDecision, ErrCircuitOpen) if cache is empty or expired.
// The SDK NEVER grants access when the breaker is open and the cache is cold.
func (b *SDKBreaker) PolicyEvaluate(ctx context.Context, req PolicyRequest) (PolicyDecision, error)

// EventIngest calls POST /v1/events/ingest through the breaker.
// When the breaker is open: buffer events locally up to SDK_OFFLINE_RETRY_LIMIT (default 500).
//
// Eviction policy: OLDEST events are evicted first (FIFO eviction).
// The buffer retains the N most recent events. For audit trail continuity,
// recent events carry the highest operational relevance.
// Eviction increments the `sdk.buffer_overflow` metric with label `eviction_policy=fifo`.
// On breaker recovery: flush buffered events in a background goroutine (oldest first).
func (b *SDKBreaker) EventIngest(ctx context.Context, event AuditEvent) error
```

## 3.3 Scope Middleware (Connector Authorization)

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

## 3.4 Go Workspace

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

## 3.5 Service Module Layout (canonical)

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
