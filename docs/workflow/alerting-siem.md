# Alerting & SIEM — Workflow

## Level 1: High-Level Architecture

```
                          ┌───────────────────────────────────────────────────────────────────────────┐
                          │                         THREAT DETECTORS                                    │
                          │                                                                             │
                          │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
                          │  │ Brute Force   │  │ Impossible   │  │ Off Hours    │  │ Data Exfilt  │  │
                          │  │ Detector      │  │ Travel Det   │  │ Detector     │  │ ration Det   │  │
                          │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │
                          │         │                  │                 │                  │          │
                          │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                   │
                          │  │ Account       │  │ Privilege    │  │   DLP PII    │                   │
                          │  │ Takeover Det  │  │ Escalation   │  │   Scanner    │                   │
                          │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘                   │
                          │         │                  │                 │                            │
                          │         │     Each publishes to threat.alerts                           │
                          │         └──────────┬───────┴─────────────────┘                            │
                          └────────────────────┼─────────────────────────────────────────────────────┘
                                               │
                                               │ Kafka: threat.alerts (12 partitions)
                                               │
                          ┌────────────────────┼─────────────────────────────────────────────────────┐
                          │                    ▼                                                       │
                          │              ALERTING SERVICE (port 8086)                                  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │                    ALERT SAGA (saga.go)                              │  │
                          │  │                                                                        │  │
                          │  │  ┌─ FetchMessage → JSON unmarshal → processMessage ───────────────┐  │  │
                          │  │  │                                                                  │  │  │
                          │  │  │  ┌──────────────────────────────────────────────────────────┐   │  │  │
                          │  │  │  │  STEP 1: Persist to MongoDB                             │   │  │  │
                          │  │  │  │  repo.Create(ctx, alert)                                │   │  │  │
                          │  │  │  │  → saga_steps: [{step:"persist", status:"completed"}]   │   │  │  │
                          │  │  │  └──────────────────────────┬───────────────────────────────┘   │  │  │
                          │  │  │                             │                                   │  │  │
                          │  │  │  ┌──────────────────────────▼───────────────────────────────┐   │  │  │
                          │  │  │  │  STEP 2: Notify (Kafka)                                  │   │  │  │
                          │  │  │  │  publisher.Publish("notifications.outbound")             │   │  │  │
                          │  │  │  │  → saga_steps: [{step:"notify", status:"completed"}]     │   │  │  │
                          │  │  │  └──────────────────────────┬───────────────────────────────┘   │  │  │
                          │  │  │                             │                                   │  │  │
                          │  │  │  ┌──────────────────────────▼───────────────────────────────┐   │  │  │
                          │  │  │  │  STEP 3: SIEM Webhook                                    │   │  │  │
                          │  │  │  │  POST to Splunk / Datadog / Sentinel / Generic          │   │  │  │
                          │  │  │  │  HMAC-SHA256 signed, with replay protection             │   │  │  │
                          │  │  │  │  SSRF-protected HTTP client                             │   │  │  │
                          │  │  │  │  → saga_steps: [{step:"siem", status:"completed"}]      │   │  │  │
                          │  │  │  └──────────────────────────┬───────────────────────────────┘   │  │  │
                          │  │  │                             │                                   │  │  │
                          │  │  │  ┌──────────────────────────▼───────────────────────────────┐   │  │  │
                          │  │  │  │  STEP 4: Audit Trail                                    │   │  │  │
                          │  │  │  │  publisher.Publish("audit.trail")                       │   │  │  │
                          │  │  │  │  type: "threat.alert.created"                           │   │  │  │
                          │  │  │  │  → saga_steps: [{step:"audit", status:"completed"}]     │   │  │  │
                          │  │  │  └──────────────────────────────────────────────────────────┘   │  │  │
                          │  │  │                                                                  │  │  │
                          │  │  │  Each step retries 5× with exponential backoff (100ms→1.6s)    │  │  │
                          │  │  │  Failure recorded → saga continues (except Step 1 abort)       │  │  │
                          │  │  │                                                                  │  │  │
                          │  │  └──────────────────────────────────────────────────────────────────┘  │  │
                          │  │                                                                        │  │
                          │  │  Commit Kafka offset ◄─── at-least-once delivery                      │  │
                          │  │  Concurrency cap: 50 goroutines (semaphore)                           │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │                    REST API (router.go)                              │  │
                          │  │                                                                        │  │
                          │  │  GET    /v1/threats/alerts          → ListAlerts (cursor-paginated)  │  │
                          │  │  GET    /v1/threats/alerts/{id}     → GetAlert                       │  │
                          │  │  POST   /v1/threats/alerts/{id}/acknowledge → Acknowledge            │  │
                          │  │  POST   /v1/threats/alerts/{id}/resolve    → Resolve (MTTR computed) │  │
                          │  │  GET    /v1/threats/stats           → Severity counts + avg MTTR     │  │
                          │  │  GET    /v1/threats/detectors       → Active detectors (mocked)      │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          │                                                                             │
                          │  ┌──────────────────────────────────────────────────────────────────────┐  │
                          │  │                    MIDDLEWARE STACK                                  │  │
                          │  │  SecurityHeaders → RateLimiter → AuthJWT + Blocklist (circuit-broken) │  │
                          │  └──────────────────────────────────────────────────────────────────────┘  │
                          └──────────────────────────────────────────────────────────────────────────┘
                               │                    │                     │
                               ▼                    ▼                     ▼
                    ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
                    │  MongoDB         │  │  Redis           │  │  Kafka           │
                    │  alerting.alerts │  │  JWT blocklist   │  │  notifications.  │
                    │  saga_steps[]    │  │                  │  │  outbound        │
                    │  MTTR tracking   │  │                  │  │  audit.trail     │
                    └──────────────────┘  └──────────────────┘  └──────────────────┘
                                                                         │
                                                                         ▼
                                                              ┌──────────────────────┐
                                                              │  SIEM / External     │
                                                              │  (Splunk / Datadog   │
                                                              │   / Azure Sentinel)  │
                                                              └──────────────────────┘
```

---

## Level 2: Alert Lifecycle Sequence

```
  Threat Detector          Alerting Saga               MongoDB               Kafka                  SIEM
       │                        │                        │                    │                      │
       │  threat.alerts         │                        │                    │                      │
       │───────────────────────>│                        │                    │                      │
       │                        │                        │                    │                      │
       │                        │  ┌─ Step 1: Persist ──│                    │                      │
       │                        │  │  repo.Create(alert) │                    │                      │
       │                        │──────────────────────>│                    │                      │
       │                        │  │  { saga_steps:      │                    │                      │
       │                        │  │    [{step:"persist",│                    │                      │
       │                        │  │      status:"completed", retries: 0}]   │                      │
       │                        │  │<────────────────────│                    │                      │
       │                        │  │                     │                    │                      │
       │                        │  └── persist FAILS → return (saga aborted)                      │
       │                        │                        │                    │                      │
       │                        │  ┌─ Step 2: Notify ───│                    │                      │
       │                        │  │  Publish alert to  │                    │                      │
       │                        │  │  notifications.    │                    │                      │
       │                        │  │  outbound          │                    │                      │
       │                        │  │────────────────────────────────────────>│                      │
       │                        │  │  { saga_steps:      │                    │                      │
       │                        │  │    [{step:"notify", │                    │                      │
       │                        │  │      status:"completed", retries: 0}]   │                      │
       │                        │  │                     │                    │                      │
       │                        │  └── notify FAILS → log, step skipped (saga continues)            │
       │                        │                        │                    │                      │
       │                        │  ┌─ Step 3: SIEM ────│                    │                      │
       │                        │  │  Compute HMAC:      │                    │                      │
       │                        │  │    ts + "." + payload                   │                      │
       │                        │  │    sig = sha256=<hmac>                  │                      │
       │                        │  │                     │                    │                      │
       │                        │  │  POST (SSRF-protected)                  │                      │
       │                        │  │  X-OpenGuard-Signature                  │                      │
       │                        │  │  X-OpenGuard-Timestamp                  │                      │
       │                        │  │  X-OpenGuard-Delivery                   │                      │
       │                        │  │────────────────────────────────────────────────────────────────>│
       │                        │  │                     │                    │                      │
       │                        │  │  200 OK             │                    │                      │
       │                        │  │<────────────────────────────────────────────────────────────────│
       │                        │  │                     │                    │                      │
       │                        │  └── siem FAILS → retry 5×, else skip                           │
       │                        │                        │                    │                      │
       │                        │  ┌─ Step 4: Audit ───│                    │                      │
       │                        │  │  Publish to        │                    │                      │
       │                        │  │  audit.trail       │                    │                      │
       │                        │  │  type: threat.alert.created            │                      │
       │                        │  │────────────────────────────────────────>│                      │
       │                        │  │                     │                    │                      │
       │                        │  Commit Kafka offset   │                    │                      │
```

### Alert Lifecycle State Machine

```
                    ┌───────────┐
                    │   OPEN    │  Initial state (from threat detector)
                    └─────┬─────┘
                          │
                    ┌─────▼─────┐
                    │ACKNOWLEDGED│  Admin reviews alert
                    └─────┬─────┘
                          │
                    ┌─────▼─────┐
                    │ RESOLVED  │  Remediation complete (MTTR computed)
                    └───────────┘
```

### Saga Step Retry Backoff

```
  Step execution:

  Attempt 1:  │█░░░░░░░░░░░│  100ms  → FAIL
  Attempt 2:  │██░░░░░░░░░░│  200ms  → FAIL
  Attempt 3:  │████░░░░░░░░│  400ms  → FAIL
  Attempt 4:  │████████░░░░│  800ms  → FAIL
  Attempt 5:  │████████████│  1.6s   → FAIL → step recorded as failed
                         │
                         └── Total backoff per step: ~3.1s worst case
```

---

## Level 3: Internals

### SIEM Webhook Delivery

#### Payload Signing

```go
func Sign(payload []byte, secret string) (sig, delivery, ts string) {
    ts = strconv.FormatInt(time.Now().Unix(), 10)
    delivery = uuid.New().String()
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(ts + "." + string(payload)))
    sig = "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return
}
```

#### Replay Protection

```go
func Verify(payload []byte, secret, sig, ts string, tolerance int64) error {
    // 1. Check timestamp is within tolerance (default 300s / 5 min)
    if time.Now().Unix() - tsInt > tolerance {
        return fmt.Errorf("request too old (replay protection)")
    }
    // 2. Verify HMAC-SHA256 signature
    expected = "sha256=" + hex(HMAC-SHA256(secret, ts + "." + payload))
    if !hmac.Equal([]byte(sig), []byte(expected)) {
        return fmt.Errorf("invalid signature")
    }
    return nil
}
```

#### SIEM-Specific Formatting

| SIEM Type | POST URL | Payload Transformation | Auth Header |
|-----------|----------|----------------------|-------------|
| **Generic** | `https://customer-siem.example.com/webhook` | Raw JSON alert `{"event_id","org_id","type","severity",...}` | `X-OpenGuard-Signature: sha256=<hmac>` |
| **Splunk HEC** | Splunk HTTP Event Collector URL | Wrapped: `{"event": <alert>, "sourcetype": "openguard_alert"}` | `Authorization: Splunk <token>` |
| **Datadog** | Datadog API endpoint | Raw JSON alert | `DD-API-KEY: <api_key>` |
| **Azure Sentinel** | Logic App / Custom Log Ingestion URL | Raw JSON alert | `X-OpenGuard-Signature` + `Log-Type: OpenGuardAlert` |

**Common headers (all types):**
- `Content-Type: application/json`
- `X-OpenGuard-Signature: sha256=<hmac>`
- `X-OpenGuard-Timestamp: <unix_epoch_seconds>`
- `X-OpenGuard-Delivery: <uuid>` (idempotency key)

#### SSRF Protection

Outbound SIEM webhooks use `middleware.NewSafeHTTPClient(10s, nil)` which:
- Resolves hostname exactly once
- Validates all resolved IPs against blocked CIDRs (RFC-1918, loopback, link-local, cloud metadata)
- Pins connection to validated IP (prevents DNS rebinding)
- 10-second HTTP timeout

### Alert Document (MongoDB `alerting.alerts`)

```json
{
  "_id": "alert-123",
  "org_id": "org-456",
  "type": "brute_force",
  "severity": "HIGH",
  "status": "open",
  "risk_score": 0.85,
  "detector_id": "brute-force",
  "raw_event": { "event_type": "auth.login.failed", "count": 15, ... },
  "saga_steps": [
    { "step": "persist", "status": "completed", "at": "...", "retries": 0 },
    { "step": "notify",  "status": "completed", "at": "...", "retries": 1 },
    { "step": "siem",    "status": "completed", "at": "...", "retries": 0 },
    { "step": "audit",   "status": "completed", "at": "...", "retries": 0 }
  ],
  "created_at": "2026-01-15T10:30:00Z",
  "ack_at": null,
  "resolved_at": null,
  "mttr_seconds": 0
}
```

### Kafka Topics

| Topic | Partitions | Producer | Consumer | Purpose |
|-------|-----------|----------|----------|---------|
| `threat.alerts` | 12 | Threat Detectors | Alerting Saga, Audit | Raw threat alerts |
| `notifications.outbound` | 6 | Alerting Saga (step 2) | Notification Service | Alert notifications |
| `audit.trail` | 24 | Alerting Saga (step 4) | Audit, Compliance | Canonical audit trail |

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka brokers |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis (JWT blocklist) |
| `PORT` | `8080` | HTTP server port |
| `ALERTING_SIEM_WEBHOOK_URL` | `""` | SIEM webhook endpoint (validated at startup) |
| `ALERTING_SIEM_REPLAY_TOLERANCE_SECONDS` | `300` | Replay protection window |
| `IAM_JWT_KEYS` | dev fallback | JWT signing keys |

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Threat Detectors** | Produce `threat.alerts` from 6 detectors (brute force, impossible travel, off hours, data exfiltration, account takeover, privilege escalation) |
| **Alerting Saga** | Orchestrate alert lifecycle: persist → notify → SIEM → audit, with retry per step |
| **MongoDB** | Alert state + saga step tracking + MTTR computation |
| **Kafka** | Async delivery to notification service, audit trail, and (designed) webhook delivery |
| **SIEMDeliverer** | Format, sign, and deliver webhooks to SIEM platforms with SSRF/replay protection |
| **REST API** | Alert querying, acknowledgment, resolution, statistics |

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **MongoDB down** | Step 1 | Persist step fails → saga retries 5× → saga aborted (no further steps) |
| **Kafka broker down** | Step 2/4 | Notify/audit steps fail → retry 5× → step recorded as failed, saga continues |
| **SIEM endpoint down** | Step 3 | HTTP error or timeout → retry 5× with backoff → step recorded as failed, saga continues |
| **SIEM endpoint slow** | Step 3 | 10s HTTP timeout → counts as failure → retry |
| **HMAC secret wrong** | Step 3 | SIEM rejects with 401/403 → saga retries → eventually step marked failed |
| **Replay timestamp expired** | Step 3 | SIEM rejects (payload >5min old) → retry with fresh timestamp |
| **SSRF guard triggered** | Step 3 | Returns "resolves to blocked IP" → error → retry → step failed |
| **Panic in goroutine** | Saga | Recovered by goroutine recover; offset may not commit reliably |
| **SIEM URL not configured** | Step 3 | Step skipped entirely (siemURL is currently hardcoded `""`) |

## Deployment Status

> The alerting service is fully deployed. The 4-step saga runs for every alert (persist → notify → audit). The SIEM webhook step (step 3) is **structurally complete** (SIEMDeliverer supports Generic, Splunk, Datadog, Azure Sentinel with HMAC signing, replay protection, SSRF guard) but **currently disabled** — `siemURL = ""` is hardcoded in `saga.go:processMessage()`. The `ALERTING_SIEM_WEBHOOK_URL` env var validates the URL at startup but is never wired into the saga. Steps 1, 2, and 4 execute in production.

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_alerting_operations_total` | Counter | `operation`, `status` | Alerting Service |
| `openguard_kafka_offset_commit_duration_seconds` | Histogram | (none) | Shared Kafka |

### Key Traces (Jaeger)

- `alert.saga.process` — from Kafka consume to offset commit
- `alert.siem.deliver` — SIEM webhook HTTP call

### Audit Events

| Event | When | Payload |
|-------|------|---------|
| `threat.alert.created` | Alert persisted (saga step 4) | alert_id, org_id, severity, detector_id |
