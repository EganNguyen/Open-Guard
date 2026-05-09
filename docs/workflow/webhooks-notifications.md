# Notifications & Webhooks — Workflow

## Level 1: High-Level Architecture

```
                          ┌───────────────────────────────────────────────────────────────────────────┐
                          │                        EVENT PRODUCERS                                    │
                          │                                                                             │
                          │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐         │
                          │  │  Alerting Svc    │  │  Connector       │  │  Any Service     │         │
                          │  │  (saga step 2)   │  │  Registry       │  │  (via Outbox)    │         │
                          │  │  → notification  │  │  → webhook       │  │  → webhook       │         │
                          │  │    .outbound     │  │    .delivery     │  │    .delivery     │         │
                          │  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘         │
                          │           │                     │                     │                     │
                          │           │  Each producer writes to outbox table,    │                     │
                          │           │  relay publishes to webhook.delivery      │                     │
                          │           └─────────────────┬─────────────────────────┘                     │
                          └─────────────────────────────┼───────────────────────────────────────────────┘
                                                         │
                                                         ▼
                          ┌───────────────────────────────────────────────────────────────────────────┐
                          │                         KAFKA: webhook.delivery                            │
                          │                         Consumer group: webhook-delivery-group              │
                          └─────────────────────────────┬───────────────────────────────────────────────┘
                                                         │
                                                         ▼
                          ┌───────────────────────────────────────────────────────────────────────────┐
                          │                   WEBHOOK DELIVERY SERVICE (port 8087)                      │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │                      KAFKA CONSUMER LOOP                              │  │
                          │  │                                                                        │
                          │  │  For each message:                                                    │
                          │  │    FetchMessage → deserialize → processMessage                        │  │
                          │  │                                                                        │
                          │  │    processMessage(msg):                                                │
                          │  │      ┌─ Retry loop (max 5 attempts, 1s..16s backoff) ──┐             │
                          │  │      │                                                      │          │
                          │  │      │   ← Deliver(ctx, target, payload, secret)          │          │
                          │  │      │      │                                              │          │
                          │  │      │      │  POST to target URL (SSRF-protected)         │          │
                          │  │      │      │  Headers:                                    │          │
                          │  │      │      │    Content-Type: application/json             │          │
                          │  │      │      │    X-OpenGuard-Signature: sha256=<hmac>      │          │
                          │  │      │      │    X-OpenGuard-Timestamp: <unix_epoch>       │          │
                          │  │      │      │    X-OpenGuard-Delivery: <delivery_id>      │          │
                          │  │      │      │                                              │          │
                          │  │      │      └── 2xx/3xx → SUCCESS                        │          │
                          │  │      │      └── 4xx/5xx → FAILURE (retry)                │          │
                          │  │      │      └── timeout/SSRF → FAILURE (retry)            │          │
                          │  │      │                                                      │          │
                          │  │      └── After 5 retries → PUBLISH TO DLQ                 │          │
                          │  │                                                              │          │
                          │  │  Commit Kafka offset                                        │          │
                          │  │                                                              │          │
                          │  │  Concurrency: max 50 goroutines (semaphore)                 │          │
                          │  └──────────────────────────────────────────────────────────────┘          │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │                     REPOSITORY (optional)                             │  │
                          │  │                                                                        │  │
                          │  │  webhook_deliveries table (PostgreSQL):                                │  │
                          │  │    id UUID PK          │ Status lifecycle:                            │  │
                          │  │    org_id UUID         │   pending → delivered / failed / dlq         │  │
                          │  │    connector_id UUID   │                                              │  │
                          │  │    event_id UUID       │  Falls back to in-memory if DB unavailable  │  │
                          │  │    target_url TEXT      │                                              │  │
                          │  │    payload JSONB       │                                              │  │
                          │  │    attempts INT        │                                              │  │
                          │  │    status TEXT         │                                              │  │
                          │  │    last_error TEXT     │                                              │  │
                          │  │    next_retry_at TS   │                                              │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │  API: GET /health, GET /metrics                                    │  │
                          │  │       GET /v1/webhook/deliveries (deprecated, returns [])           │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          └───────────────────────────────────────────────────────────────────────────┘
```

---

## Level 2: Webhook Delivery Sequence

```
  Upstream Service           Webhook Delivery                     Target Endpoint                  Kafka
  (Alerting Svc)             (port 8087)                          (SIEM / Customer)
       │                          │                                    │                           │
       │  (via Outbox → Kafka)    │                                    │                           │
       │─────────────────────────>│                                    │                           │
       │                          │                                    │                           │
       │                          │  FetchMessage()                    │                           │
       │                          │  WebhookDeliveryRequest:           │                           │
       │                          │    target: "https://customer.com/  │                           │
       │                          │             webhook"               │                           │
       │                          │    payload: '{"alert": {...}}'    │                           │
       │                          │    secret: "hmac-key-123"          │                           │
       │                          │    org_id: "org-456"               │                           │
       │                          │    event_id: "evt-789"             │                           │
       │                          │    connector_id: "conn-012"        │                           │
       │                          │                                    │                           │
       │                          │  processMessage()                  │                           │
       │                          │    repo.Create( status="pending" ) │                           │
       │                          │    (if DB configured)              │                           │
       │                          │                                    │                           │
       │                          │  ┌─ Retry 1/5 (backoff 1s) ──┐    │                           │
       │                          │  │  Deliver()                 │    │                           │
       │                          │  │    SSRF check: resolve     │    │                           │
       │                          │  │    hostname, validate IP   │    │                           │
       │                          │  │    → blocked? → fail       │    │                           │
       │                          │  │                             │    │                           │
       │                          │  │    Compute HMAC:            │    │                           │
       │                          │  │      ts := now()            │    │                           │
       │                          │  │      sig := HMAC-SHA256(    │    │                           │
       │                          │  │        secret, ts+"."+payload│   │                           │
       │                          │  │      )                      │    │                           │
       │                          │  │                             │    │                           │
       │                          │  │    POST target              │    │                           │
       │                          │  │    Headers:                 │    │                           │
       │                          │  │      X-OpenGuard-Signature  │    │                           │
       │                          │  │      X-OpenGuard-Timestamp  │    │                           │
       │                          │  │      X-OpenGuard-Delivery   │    │                           │
       │                          │  │─────────────────────────────────>│                           │
       │                          │  │                             │    │                           │
       │                          │  │  ┌─ 200 OK                  │    │                           │
       │                          │  │  │<──────────────────────────────│                           │
       │                          │  │  │ repo.Update(status="delivered")                           │
       │                          │  │  │ → SUCCESS, exit retry loop    │                           │
       │                          │  │  │                              │                           │
       │                          │  │  └─ 5xx / timeout / SSRF       │                           │
       │                          │  │    repo.Update(status="failed") │                           │
       │                          │  │    → continue retry loop        │                           │
       │                          │  │                                 │                           │
       │                          │  └─────────────────────────────────┘                           │
       │                          │                                    │                           │
       │                          │  ┌─ Retry 2/5 (backoff 2s) ──┐    │                           │
       │                          │  │  Deliver() → same flow      │    │                           │
       │                          │  └─────────────────────────────┘    │                           │
       │                          │                                    │                           │
       │                          │  (... up to 5 retries ...)          │                           │
       │                          │                                    │                           │
       │                          │  ┌─ All retries exhausted:          │                           │
       │                          │  │  repo.Update(status="dlq")       │                           │
       │                          │  │                                  │                           │
       │                          │  │  Publish to webhook.dlq topic    │                           │
       │                          │  │  { request, error, failed_at }   │                           │
       │                          │  │──────────────────────────────────────────────────────────>│   │
       │                          │  │                                  │                           │
       │                          │  └──────────────────────────────────┘                           │
       │                          │                                    │                           │
       │                          │  Commit Kafka offset               │                           │
```

---

## Level 3: State Transitions

### Delivery Status State Machine

```
                        ┌───────────┐
                        │  PENDING  │  (first fetched from Kafka)
                        └─────┬─────┘
                              │
                        ┌─────▼─────┐
                        │ IN_FLIGHT │  (goroutine processing)
                        └─────┬─────┘
                              │
                  ┌───────────┼───────────┐
                  │           │           │
                  ▼           ▼           ▼
            ┌──────────┐ ┌──────────┐ ┌──────────┐
            │DELIVERED │ │  FAILED  │ │ SSRF     │
            │ (success)│ │ (retry)  │ │ BLOCKED  │
            └──────────┘ └────┬─────┘ └──────────┘
                              │
                     ┌────────┴────────┐
                     │                 │
                     ▼                 ▼
               ┌──────────┐     ┌──────────┐
               │  RETRY   │     │   DLQ    │
               │ (×5 max) │     │ (perm.   │
               │ 1s..16s  │     │  failed) │
               └──────────┘     └──────────┘
```

### Retry Backoff Timeline

```
  Attempt 1:  │████░░░░░░░░░░░░░░░░░░░│  1s  → FAIL
  Attempt 2:  │████████░░░░░░░░░░░░░░░│  2s  → FAIL
  Attempt 3:  │████████████████░░░░░░░│  4s  → FAIL
  Attempt 4:  │██████████████████████░░│  8s  → FAIL
  Attempt 5:  │████████████████████████│ 16s  → FAIL → DLQ
                                            │
                                            ▼
                                     ┌──────────────┐
                                     │  DLQ publish │
                                     │  (topic:      │
                                     │  webhook.dlq)│
                                     └──────────────┘
```

---

## HMAC Signature Format

```
  Outgoing request headers:

    X-OpenGuard-Timestamp: 1715000000
    X-OpenGuard-Signature: sha256=a1b2c3d4e5f6...
    X-OpenGuard-Delivery:  <kafka-message-key>

  Signature computation:

    payload := request body (raw JSON string)
    ts      := strconv.FormatInt(time.Now().Unix(), 10)
    mac     := HMAC-SHA256(secret, ts + "." + payload)
    sig     := "sha256=" + hex.EncodeToString(mac)

  Target verification (pseudocode):

    func verify(payload, timestamp, sig, secret):
      expected = HMAC-SHA256(secret, timestamp + "." + payload)
      return constant_time_compare(sig, "sha256=" + hex(expected))

  Replay protection:
    - Target checks that timestamp is within ±5min of current time
    - Target stores X-OpenGuard-Delivery idempotency key (optional)
```

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Upstream Service** | Produces webhook delivery request via Transactional Outbox → `webhook.delivery` topic |
| **Kafka** | Durable queue with at-least-once delivery guarantees |
| **Webhook Delivery Service** | Consumes, retries with backoff, HMAC-signs, delivers, DLQs on exhaustion |
| **SSRF Guard** | Prevents delivery to internal/private IPs (cloud metadata, loopback, RFC-1918) |
| **PostgreSQL** | Optional delivery status tracking (`pending → delivered/failed/dlq`) |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Target unreachable (DNS failure)** | Webhook | Counts as failure → retry loop → DLQ after 5 |
| **Target returns 4xx/5xx** | Webhook | Counts as failure → retry loop → DLQ after 5 |
| **SSRF guard triggered** | Webhook | Returns "resolves to blocked IP" → retry loop → DLQ |
| **Kafka broker down** | Webhook | Consumer blocks on FetchMessage; retries internally |
| **DLQ publish fails** | Webhook | Offset NOT committed → message re-delivered on restart |
| **PostgreSQL unavailable** | Webhook | Status tracking degraded (in-memory only); delivery still works |
| **Panic in delivery goroutine** | Webhook | Recovered by recover(); offset may not commit reliably |
| **HMAC signing key wrong** | Webhook | Target receives invalid signature and rejects → retry → DLQ |
| **Target endpoint slow** | Webhook | 30s HTTP client timeout → retry → DLQ |
| **Semaphore full (50 goroutines)** | Webhook | processMessage blocks until slot available |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_webhook_delivery_attempts_total` | Counter | `operation`, `status` | Webhook Delivery |
| `openguard_webhook_delivery_duration_seconds` | Histogram | (none) | Webhook Delivery |
| `openguard_kafka_offset_commit_duration_seconds` | Histogram | (none) | Shared Kafka |

### Key Traces (Jaeger)

- `webhook.deliver` — from Kafka consume to HTTP response
- `webhook.dlq` — DLQ publish trace

### Audit Events

| Event | When | Payload |
|-------|------|---------|
| `webhook.delivery.started` | First delivery attempt | delivery_id, target, event_id |
| `webhook.delivery.succeeded` | Successful 2xx/3xx | delivery_id, attempts, status_code |
| `webhook.delivery.failed` | Failed attempt (retrying) | delivery_id, attempt, error, next_retry |
| `webhook.delivery.dlq` | All retries exhausted | delivery_id, error, total_attempts |

---

## Message Format

```
  WebhookDeliveryRequest (Kafka value):
  {
    "target":       "https://hooks.splunk.com/...",
    "payload":      "{\"alert\":{\"id\":\"...\",\"severity\":\"high\"}}",
    "secret":       "whsec_abc123...",
    "org_id":       "org-uuid",
    "event_id":     "event-uuid",
    "connector_id": "connector-uuid"
  }

  DLQ message (Kafka value, published to webhook.dlq):
  {
    "request":   { ... original WebhookDeliveryRequest ... },
    "error":     "target server error: 503",
    "failed_at": "2026-01-15T10:30:00Z"
  }
```
