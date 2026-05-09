# Control Plane Gateway — Workflow

## Level 1: High-Level Architecture

```
  ┌─────────────────────────────────────────────────────────────────────────────────────┐
  │                              EXTERNAL CLIENTS                                        │
  │                                                                                       │
  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │
  │  │  Angular UI  │  │  SDK/Apps    │  │  SCIM IdP    │  │  curl/CLI    │             │
  │  │  (Dashboard) │  │  (external)  │  │  (Azure AD)  │  │              │             │
  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘             │
  │         │                 │                 │                 │                     │
  │         │  HTTPS (mTLS optional)            │                 │                     │
  │         ▼                 ▼                 ▼                 ▼                     │
  │  ┌────────────────────────────────────────────────────────────────────────────────┐ │
  │  │                   CONTROL PLANE (port 8081)                                     │ │
  │  │                                                                                 │ │
  │  │  ┌───────────────────────────────────────────────────────────────────────────┐ │ │
  │  │  │                     GLOBAL MIDDLEWARE STACK                                  │ │
  │  │  │                                                                             │ │
  │  │  │  1. chi.RequestID       → X-Request-ID header injection                    │ │
  │  │  │  2. chi.RealIP          → Trust X-Forwarded-For / X-Real-IP                │ │
  │  │  │  3. Correlation         → X-Correlation-ID (generates UUID if missing)     │ │
  │  │  │  4. Metrics             → Prometheus counter + latency histogram           │ │
  │  │  │  5. chi.Recoverer       → Panic recovery → 500                             │ │
  │  │  │  6. SecurityHeaders     → X-Content-Type-Options, X-Frame-Options, HSTS    │ │
  │  │  │  7. CORS                → Origin whitelist (3 hardcoded + env)             │ │
  │  │  │  8. DeprecationHeaders  → /v1/* routes: Deprecation: true (Jan 2027)       │ │
  │  │  └───────────────────────────────────────────────────────────────────────────┘ │ │
  │  │                                                                                 │ │
  │  │  ┌───────────────────────────────────────────────────────────────────────────┐ │ │
  │  │  │                     ROUTE TABLE + CIRCUIT BREAKERS                         │ │
  │  │  │                                                                             │ │
  │  │  │  Path                 Method     Target          Breaker      Half-Open    │ │
  │  │  │  /v1/policy/evaluate  POST       Policy (8083)   cbPolicy     3 probes     │ │
  │  │  │  /v1/policies         GET/POST   Policy (8083)   cbPolicy     3 probes     │ │
  │  │  │  /v1/policies/{id}    GET/PUT/DEL Policy (8083)  cbPolicy     3 probes     │ │
  │  │  │  /v1/events/ingest    POST       Audit (8085)    NONE         —            │ │
  │  │  │  /v1/scim/v2/Users    GET/POST   IAM (8082)      cbIAM        5 probes     │ │
  │  │  │  /v1/scim/v2/Users/{id} GET/PATCH IAM (8082)     cbIAM        5 probes     │ │
  │  │  │  /v1/logs              POST       Inline handler  —            —            │ │
  │  │  │  /health              GET        Inline handler  —            —            │ │
  │  │  │  /metrics              GET        Prometheus     —            —            │ │
  │  │  └───────────────────────────────────────────────────────────────────────────┘ │ │
  │  │                                                                                 │ │
  │  │  ┌───────────────────────────────────────────────────────────────────────────┐ │ │
  │  │  │                     PROXY LAYER                                            │ │
  │  │  │                                                                             │ │
  │  │  │  httputil.ReverseProxy with CircuitBreakerTransport:                       │ │
  │  │  │    - Standard Director rewrites URL → downstream service                   │ │
  │  │  │    - Strips X-Org-ID, X-Internal-Key, X-OpenGuard-Org-ID headers           │ │
  │  │  │    - Wraps http.DefaultTransport in gobreaker.CircuitBreaker               │ │
  │  │  └───────────────────────────────────────────────────────────────────────────┘ │ │
  │  └────────────────────────────────────────────────────────────────────────────────┘ │
  │                                                                                     │
  │  mTLS: Optional client cert verification (VerifyClientCertIfGiven)                 │
  │  Fallback: Plain HTTP if certs missing (dev mode)                                  │
  └─────────────────────────────────────────────────────────────────────────────────────┘
                                │            │            │
                    ┌───────────┼────────────┼────────────┼───────────┐
                    │           │            │            │           │
                    ▼           ▼            ▼            ▼           ▼
              ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
              │ IAM Svc  │ │ Policy   │ │ Audit    │ │ Angular  │ │ External │
              │ (8082)   │ │ (8083)   │ │ (8085)   │ │ (4200)   │ │ (CORS)   │
              └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘
```

---

## Level 2: Request Lifecycle (Detailed Sequence)

```
  Client                     Control Plane                              Downstream Service
    │                             │                                           │
    │  HTTPS Request              │                                           │
    │  (with optional client cert)│                                           │
    │────────────────────────────>│                                           │
    │                             │                                           │
    │                             │  ┌─ mTLS Termination                      │
    │                             │  │  VerifyClientCertIfGiven               │
    │                             │  │  If cert provided → verify vs CA       │
    │                             │  │  If no cert → allow (optional mTLS)    │
    │                             │  │                                           │
    │                             │  ┌─ Middleware Chain:                      │
    │                             │  │  1. chi.RequestID                       │
    │                             │  │     Generate X-Request-ID (UUID)        │
    │                             │  │                                         │
    │                             │  │  2. chi.RealIP                          │
    │                             │  │     Parse X-Forwarded-For → RemoteAddr  │
    │                             │  │                                         │
    │                             │  │  3. Correlation middleware              │
    │                             │  │     If X-Correlation-ID missing:        │
    │                             │  │       Generate UUID, inject into header │
    │                             │  │     Inject into slog context             │
    │                             │  │                                         │
    │                             │  │  4. Metrics middleware                  │
    │                             │  │     Start timer                          │
    │                             │  │     Defer: observe duration + status    │
    │                             │  │                                         │
    │                             │  │  5. chi.Recoverer                      │
    │                             │  │     defer recover() → log stack → 500  │
    │                             │  │                                         │
    │                             │  │  6. SecurityHeaders                     │
    │                             │  │     Set: X-Content-Type-Options: nosniff│
    │                             │  │     Set: X-Frame-Options: DENY          │
    │                             │  │     Set: X-XSS-Protection: 0            │
    │                             │  │     Set: Referrer-Policy: strict-origin │
    │                             │  │     Set: Strict-Transport-Security      │
    │                             │  │                                         │
    │                             │  │  7. CORS (inline)                      │
    │                             │  │     Vary: Origin                        │
    │                             │  │     Check origin whitelist              │
    │                             │  │     Set: Access-Control-Allow-*         │
    │                             │  │     If OPTIONS → 200 immediately        │
    │                             │  │                                         │
    │                             │  │  8. DeprecationHeaders (/v1/*)          │
    │                             │  │     Set: Deprecation: true              │
    │                             │  │     Set: Sunset: Jan 2027               │
    │                             │  │                                           │
    │                             │  ┌─ Route Matching                         │
    │                             │  │  e.g., POST /v1/policies                │
    │                             │  │                                           │
    │                             │  ┌─ Proxy Forwarding                       │
    │                             │  │  Create ReverseProxy to target           │
    │                             │  │                                           │
    │                             │  │  Director:                              │
    │                             │  │    - Rewrite URL scheme+host to target   │
    │                             │  │    - Strip internal headers:             │
    │                             │  │      X-Org-ID                            │
    │                             │  │      X-Internal-Key                      │
    │                             │  │      X-OpenGuard-Org-ID                  │
    │                             │  │                                           │
    │                             │  │  CircuitBreakerTransport:                 │
    │                             │  │    cb.Execute(func() → RoundTrip(req))   │
    │                             │  │                                           │
    │                             │  │  ┌─ Circuit CLOSED (normal):              │
    │                             │  │  │  → Forward request to downstream      │
    │                             │  │  │  → On 5xx / timeout → increment       │
    │                             │  │  │    failure count                       │
    │                             │  │  │                                           │
    │                             │  │  │  ┌─ Circuit OPEN (tripped):            │
    │                             │  │  │  │  → Bypass RoundTrip immediately     │
    │                             │  │  │  │  → Return ErrOpenState              │
    │                             │  │  │  │  → After 30s → HALF-OPEN            │
    │                             │  │  │  │                                       │
    │                             │  │  │  │  ┌─ HALF-OPEN (probing):             │
    │                             │  │  │  │  │  → Allow MaxRequests probes       │
    │                             │  │  │  │  │  → Success → CLOSED               │
    │                             │  │  │  │  │  → Failure → OPEN (30s more)      │
    │                             │  │  │  │  │                                       │
    │                             │  │  │  └────────────────────────────────────  │
    │                             │  │  │                                           │
    │  <──────────────────────────│──│──│── Response to client ───────────────────│
    │                             │  │  │                                           │
```

---

## Level 3: State Transitions

### Circuit Breaker State Machine

```
                        ┌──────────┐
          ┌────────────>│  CLOSED  │<──────────────┐
          │             │ (normal) │                │
          │             └────┬─────┘                │
          │                  │                      │
          │          5 consecutive failures          │
          │                  │                      │
          │             ┌────▼─────┐                │
          │             │   OPEN   │─────────────────┤
          │             │ (reject  │  30s timeout    │
          │             │  fast)   │─────> HALF-OPEN │
          │             └──────────┘                │
          │                  │                      │
          │             HALF-OPEN                   │
          │          ┌──────┴──────┐                │
          │          │             │                │
          │     ┌────▼────┐  ┌────▼────┐           │
          │     │ 1st req │  │ 2nd req │           │
          │     │ success │  │ failure │           │
          │     └────┬────┘  └────┬────┘           │
          │          │            │                 │
          │     ┌────▼────┐  ┌────▼─────┐          │
          │     │ CLOSED  │  │  OPEN    │──────────┘
          │     └─────────┘  │ (30s)    │
          │                  └──────────┘
          │
          └──── Policy:  MaxRequests=3, Interval=10s,
                         FailureThreshold=5, OpenDuration=30s
                IAM:     MaxRequests=5, rest same
```

### Connection State (mTLS)

```
                          ┌─────────────────────────────────────────────┐
                          │         mTLS NEGOTIATION                    │
                          │                                             │
                          │  Client → Server: CONNECT                   │
                          │  Server → Client: Certificate + CA list      │
                          │  Client → Server: Certificate (optional)    │
                          │                                             │
                          │  ┌─ Client sent cert:                       │
                          │  │  VerifyClientCertIfGiven:                 │
                          │  │  Validate against CA pool                │
                          │  │  ┌─ Valid → Full mTLS connection         │
                          │  │  └─ Invalid → TLS error (handshake fail)  │
                          │  │                                             │
                          │  └─ Client no cert:                         │
                          │     VerifyClientCertIfGiven:                 │
                          │     Allow connection (no mTLS)               │
                          │                                             │
                          │  Fallback: Plain HTTP if certs missing      │
                          │  (dev mode, logged as WARN)                 │
                          └─────────────────────────────────────────────┘
```

---

## Route Table

| Path | Method | Target Service | Circuit Breaker | Header Sanitization |
|------|--------|---------------|-----------------|-------------------|
| `/v1/policy/evaluate` | POST | Policy (8083) | cbPolicy (3 probes) | Yes |
| `/v1/policy/eval-logs` | GET | Policy (8083) | cbPolicy | Yes |
| `/v1/policies` | GET, POST | Policy (8083) | cbPolicy | Yes |
| `/v1/policies/{id}` | GET, PUT, DELETE | Policy (8083) | cbPolicy | Yes |
| `/v1/events/ingest` | POST | Audit (8085) | None | Yes |
| `/v1/scim/v2/Users` | GET, POST | IAM (8082) | cbIAM (5 probes) | Yes |
| `/v1/scim/v2/Users/{id}` | GET, PATCH | IAM (8082) | cbIAM | Yes |
| `/v1/logs` | POST | Inline (slog) | — | N/A |
| `/health` | GET | Inline (200 OK) | — | N/A |
| `/metrics` | GET | Prometheus | — | N/A |

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **External Client** | HTTPS with optional mTLS client certificate, JWT token management |
| **Control Plane** | Single ingress, middleware chain, route proxying, circuit breaking, header sanitization |
| **Shared Middleware** | Security headers, CORS, correlation IDs, Prometheus metrics |
| **Circuit Breaker** | Protects downstream services from cascading failures; fast-fail when unhealthy |
| **IAM Service** | Authenticates requests via JWT middleware at service level |
| **Policy Service** | Accepts proxied requests, authenticates via own JWT middleware |
| **Audit Service** | Accepts proxied ingest requests, authenticates via own JWT middleware |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Downstream service down** | Control Plane | CB accumulates failures → opens (30s) → requests fail fast with 500 |
| **Circuit breaker open** | Control Plane | RoundTrip returns ErrOpenState immediately (no downstream call) |
| **mTLS certs missing** | Control Plane | Server falls back to plain HTTP (dev mode) |
| **CA cert missing (server cert present)** | Control Plane | HTTPS works but client certs not verified |
| **Jaeger unavailable** | Control Plane | InitTracer logs error, startup continues (non-fatal) |
| **Panic in handler** | Control Plane | chi.Recoverer catches, logs stack, returns 500 |
| **Shutdown timeout** | Control Plane | 30s graceful shutdown; connections force-closed after |
| **Invalid proxy target URL** | Control Plane | NewProxy panics at startup (fail-early) |
| **Redis down (blocklist)** | Control Plane | N/A — control plane does not authenticate; downstream services handle |
| **CORS origin mismatch** | Control Plane | Browser preflight fails; server returns proper error headers |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `http_requests_total` | Counter | `method`, `path`, `status` | Control Plane |
| `http_request_duration_seconds` | Histogram | `method`, `path` | Control Plane |
| `openguard_circuit_breaker_state` | Gauge | `name`, `state` | Control Plane |
| `openguard_circuit_breaker_requests_total` | Counter | `name`, `result` | Shared resilience |

### Key Traces (Jaeger)

- `control-plane.proxy` — from request ingress to downstream response
- `control-plane.middleware` — cumulative middleware execution time

### Circuit Breaker Events (Logged)

| Event | When | Payload |
|-------|------|---------|
| `cb.state_changed` | State transition (any) | name, from_state, to_state |
| `cb.request_rejected` | Request while open | name, path |
| `cb.request_failed` | Downstream 5xx | name, path, status_code |

---

## Startup Sequence

```
  1. Init logger (JSON + SafeHandler redaction)
  2. Init OpenTelemetry Jaeger tracer
  3. Setup signal handler (SIGINT/SIGTERM)
  4. Build router (middleware + routes + circuit breakers)
  5. Load mTLS certs:
     ├── /certs/ca.crt exists?
     │   ├── Yes → load CA pool, verify client certs if given
     │   └── No → no client cert verification
     ├── /certs/control-plane.crt + .key exists?
     │   ├── Yes → HTTPS server
     │   └── No → HTTP server (dev mode WARN)
  6. Listen on :PORT (default 8080, mapped to 8081 in infra)
  7. Graceful shutdown on signal (30s timeout)
```
