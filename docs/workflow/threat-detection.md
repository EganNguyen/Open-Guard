# Threat Detection — Workflow

## Level 1: High-Level Architecture

```
                                    ┌────────────────────────────────────────────────────────────────────────────┐
                                    │                         EVENT SOURCES                                      │
                                    │                                                                              │
                                    │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
                                    │  │  IAM Service  │  │ Policy Svc   │  │  Audit Svc   │  │  SDK/Apps    │   │
                                    │  │  (port 8082) │  │ (port 8083)  │  │ (port 8085)  │  │  (external)  │   │
                                    │  │  auth.events  │  │policy.changes│  │ data.access  │  │  audit.trail │   │
                                    │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
                                    │         │                 │                 │                 │           │
                                    │         │   Transactional Outbox (pg_notify + poll relay)      │           │
                                    │         └─────────┬───────────────────────┬───────────────────┘           │
                                    └───────────────────┼───────────────────────┼───────────────────────────────┘
                                                         ▼                       ▼
                                    ┌────────────────────────────────────────────────────────────────────────────┐
                                    │                         KAFKA EVENT BUS                                    │
                                    │                                                                              │
                                    │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
                                    │  │ auth.events  │  │policy.changes│  │ data.access  │  │ audit.trail  │   │
                                    │  │ (12 part.)   │  │ (6 part.)    │  │ (24 part.)   │  │ (24 part.)   │   │
                                    │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
                                    │         │                 │                 │                 │           │
                                    │         └─────────────────┼─────────────────┼─────────────────┘           │
                                    │                           │                 │                               │
                                    └───────────────────────────┼─────────────────┼───────────────────────────────┘
                                                                │                 │
                                    ┌───────────────────────────┼─────────────────┼───────────────────────────────┐
                                    │                           ▼                 ▼                               │
                                    │  ┌────────────────────────────────────────────────────────────────────────┐ │
                                    │  │                      THREAT SERVICE (port 8084)                        │ │
                                    │  │                                                                        │ │
                                    │  │  ┌─────────────────────┐  ┌─────────────────────┐                      │ │
                                    │  │  │ BruteForceDetector   │  │ ImpossibleTravel    │                      │ │
                                    │  │  │ • auth.failed/login  │  │ Detector            │                      │ │
                                    │  │  │ • Redis sliding      │  │ • auth.login.success │                      │ │
                                    │  │  │   window (5min)      │  │ • GeoIP Haversine   │                      │ │
                                    │  │  │ • threshold: 10      │  │ • threshold: 500km  │                      │ │
                                    │  │  └──────────┬──────────┘  └──────────┬──────────┘                      │ │
                                    │  │             │                        │                                  │ │
                                    │  │  ┌──────────▼──────────┐  ┌──────────▼──────────┐                      │ │
                                    │  │  │ OffHoursDetector     │  │ AccountTakeover     │                      │ │
                                    │  │  │ • auth.login.success │  │ Detector            │                      │ │
                                    │  │  │ • 22:00-06:00 UTC   │  │ • password.changed   │                      │ │
                                    │  │  │ • 3-day baseline     │  │ • new device check   │                      │ │
                                    │  │  └──────────┬──────────┘  └──────────┬──────────┘                      │ │
                                    │  │             │                        │                                  │ │
                                    │  │  ┌──────────▼──────────┐  ┌──────────▼──────────┐                      │ │
                                    │  │  │ DataExfiltration     │  │ PrivilegeEscalation  │                      │ │
                                    │  │  │ Detector             │  │ Detector            │                      │ │
                                    │  │  │ • data.access events │  │ • auth + policy     │                      │ │
                                    │  │  │ • 3-sigma anomaly    │  │ • dual consumer     │                      │ │
                                    │  │  └──────────┬──────────┘  └──────────┬──────────┘                      │ │
                                    │  │             │                        │                                  │ │
                                    │  │             └──────────┬─────────────┘                                  │ │
                                    │  │                        ▼                                               │ │
                                    │  │  ┌─────────────────────────────────────────────────────┐                │ │
                                    │  │  │              Alert Pipeline                        │                │ │
                                    │  │  │  ┌──────────┐   ┌──────────┐   ┌────────────────┐ │                │ │
                                    │  │  │  │ Scorer   │──>│Persister │──>│Kafka Publisher │ │                │ │
                                    │  │  │  │ (compos. │   │ (MongoDB │   │ (threat.alerts │ │                │ │
                                    │  │  │  │  score)  │   │  alerts) │   │   topic)       │ │                │ │
                                    │  │  │  └──────────┘   └──────────┘   └───────┬────────┘ │                │ │
                                    │  │  └──────────────────────────────────────────┼────────┘                │ │
                                    │  │                                             │                          │ │
                                    │  │  ┌──────────────────────────────────────────┼────────┐                │ │
                                    │  │  │              Redis Cache                │        │                │ │
                                    │  │  │  threat:* alerts (24h TTL)              │        │                │ │
                                    │  │  └──────────────────────────────────────────┼────────┘                │ │
                                    │  └─────────────────────────────────────────────┼──────────────────────────┘ │
                                    └───────────────────────────────────────────────┼────────────────────────────┘
                                                                                    │
                                                                                    │ Kafka: threat.alerts
                                                                                    ▼
                                    ┌────────────────────────────────────────────────────────────────────────────┐
                                    │                     ALERTING SERVICE (port 8086)                          │
                                    │                                                                              │
                                    │  ┌────────────────────────────────────────────────────────────────────────┐ │
                                    │  │                        Alert Saga (4-step)                            │ │
                                    │  │                                                                        │ │
                                    │  │   Step 1                        Step 2                                 │ │
                                    │  │   ┌──────────────────┐         ┌──────────────────┐                   │ │
                                    │  │   │ Persist to       │  ───>   │ Enqueue          │                   │ │
                                    │  │   │ MongoDB          │         │ Notification     │                   │ │
                                    │  │   │ (alerting.alerts) │         │ (Kafka:           │                   │ │
                                    │  │   └──────────────────┘         │  notif.outbound)  │                   │ │
                                    │  │                                └──────────────────┘                   │ │
                                    │  │       │                                 │                              │ │
                                    │  │       ▼                                 ▼                              │ │
                                    │  │   Step 3                        Step 4                                 │ │
                                    │  │   ┌──────────────────┐         ┌──────────────────┐                   │ │
                                    │  │   │ Deliver to SIEM  │  ───>   │ Write Audit      │                   │ │
                                    │  │   │ (Splunk/Datadog/ │         │ Trail            │                   │ │
                                    │  │   │  Sentinel/Generic│         │ (Kafka:           │                   │ │
                                    │  │   │  + HMAC signing) │         │  audit.trail)     │                   │ │
                                    │  │   └──────────────────┘         └──────────────────┘                   │ │
                                    │  └────────────────────────────────────────────────────────────────────────┘ │
                                    └────────────────────────────────────────────────────────────────────────────┘
                                                                                    │
                                        ┌───────────────────────────────────────────┼───────────────────────────┐
                                        │                                           │                           │
                                        ▼                                           ▼                           ▼
                                  ┌──────────────┐                         ┌──────────────┐          ┌──────────────┐
                                  │   MongoDB    │                         │   SIEM       │          │   Kafka      │
                                  │  alerting DB │                         │  Webhook     │          │ audit.trail  │
                                  │  + threats   │                         │  (Splunk/    │          │              │
                                  │  .alerts     │                         │   Datadog)   │          │  → Compliance│
                                  └──────────────┘                         └──────────────┘          │  → Audit Svc │
                                                                                                    └──────────────┘
```

---

## Level 2A: Detection Pipeline — Event Processing Flow

```
                          ┌─────────────────── KAFKA ───────────────────┐
                          │                                              │
  ┌──────────────────┐    │  ┌──────────────┐  ┌──────────────┐         │
  │ IAM Service      │    │  │ auth.events  │  │policy.changes│         │
  │ (Login Attempt)  │───>│  │              │  │              │         │
  │                  │    │  └──────┬───────┘  └──────┬───────┘         │
  │ 1. User login    │    │         │                 │                 │
  │ 2. bcrypt verify │    │         │                 │                 │
  │ 3. Outbox insert │    │         │                 │                 │
  │ 4. pg_notify     │    │         │                 │                 │
  │ 5. Relay publish │    │         │                 │                 │
  └──────────────────┘    │         │                 │                 │
                          │         ▼                 ▼                 │
  ┌──────────────────┐    │  ┌──────────────────────────────────────┐   │
  │ Policy Service   │    │  │         THREAT SERVICE               │   │
  │ (Role Grant)     │───>│  │                                      │   │
  │                  │    │  │  ┌─ Per-detector goroutines ──────┐  │   │
  │ 1. Admin grants  │    │  │  │                                │  │   │
  │    role           │    │  │  │  Each detector:               │  │   │
  │ 2. DB transaction│    │  │  │  1. Consume from Kafka        │  │   │
  │ 3. Outbox insert │    │  │  │  2. Deserialize event         │  │   │
  │ 4. pg_notify     │    │  │  │  3. Check Redis state         │  │   │
  │ 5. Relay publish │    │  │  │  4. Apply detection logic     │  │   │
  └──────────────────┘    │  │   │  5. Score + Severity          │  │   │
                          │  │   │  6. Persist to MongoDB        │  │   │
  ┌──────────────────┐    │  │   │  7. Publish to threat.alerts  │  │   │
  │ Audit Service    │    │  │   │  8. Cache in Redis (24h)      │  │   │
  │ (SDK Event)      │───>│  │  └────────────────────────────────┘  │   │
  │                  │    │  └──────────────────────────────────────┘   │
  │ data.access topic│    └────────────────────────────────────────────┘
  └──────────────────┘
```

### Per-Detector Event Processing Detail

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         DETECTOR PROCESSING SEQUENCE                            │
│                                                                                 │
│  Kafka                         Threat Service                                   │
│  ┌────┐                                                                        │
│  │    │  ConsumerGroup.Goroutine                                               │
│  │    │                                                                        │
│  │    │  1. Receive event                                                      │
│  │    │     └─ Deserialize EventEnvelope                                       │
│  │    │        └─ Extract payload + trace context                              │
│  │    │                                                                        │
│  │    │  2. Fetch detector state from Redis                                    │
│  │    │     └─ Sorted set (brute force): ZCOUNT window                        │
│  │    │     └─ Key check (off hours): GET last_seen                           │
│  │    │     └─ Device set (account takeover): SMEMBERS user_devices           │
│  │    │     └─ Org baseline (exfiltration): GET org:<id>:baseline              │
│  │    │     └─ Login window (escalation): GET user:<id>:last_login            │
│  │    │                                                                        │
│  │    │  3. Apply detection logic                                              │
│  │    │     └─ Threshold check (count > limit?)                               │
│  │    │     └─ Geo distance calc (Haversine > threshold?)                     │
│  │    │     └─ Time window check (off-hours?)                                 │
│  │    │     └─ Statistical anomaly (3-sigma?)                                 │
│  │    │     └─ Device/password correlation                                    │
│  │    │                                                                        │
│  │    │  4. If threat detected:                                                │
│  │    │     └─ Build Alert struct                                              │
│  │    │        ├─ Detector name, UserID, OrgID                                │
│  │    │        ├─ Score (0.0 - 1.0)                                           │
│  │    │        ├─ Severity (MEDIUM/HIGH/CRITICAL)                             │
│  │    │        └─ Metadata (IP, device, location, etc.)                       │
│  │    │     └─ Compute composite score (time-decayed max)                     │
│  │    │     └─ Persist to MongoDB threats.alerts                              │
│  │    │     └─ Publish to threat.alerts Kafka topic                           │
│  │    │     └─ Cache in Redis with 24h TTL                                    │
│  │    │                                                                        │
│  │    │  5. Commit Kafka offset                                                │
│  │    │                                                                        │
│  │    │  6. Emit metrics:                                                      │
│  │    │     └─ openguard_threat_detections_total{detector,severity}           │
│  │    │     └─ processing_latency_seconds{detector}                           │
│  │    └────────────────────────────────────────────────────────────────────────┘
│  └────┘
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Level 2B: Alert Processing Saga (Alerting Service)

```
                         ALERTING SERVICE SAGA
                         (4-step, per-alert processing)

  threat.alerts          Alert Saga Worker                  MongoDB           Kafka              SIEM
  ┌──────────┐          ┌────────────────────────────────────────────────────────────────────┐
  │          │          │                                                                    │
  │          │  1.      │  ┌───────────────────┐                                             │
  │  Alert    │─────────>│  │ Step 1: Persist   │──── Persist alert ──────────────────────> │
  │  Event    │          │  │ to MongoDB         │<── success ───────────────────────────────┘
  │          │          │  └───────────────────┘                                             │
  │          │          │         │                                                          │
  │          │          │         │ success                                                  │
  │          │          │         ▼                                                          │
  │          │          │  ┌───────────────────┐                                             │
  │          │          │  │ Step 2: Enqueue   │──── Publish ──────────────────────────>     │
  │          │          │  │ Notification       │     (notifications.outbound)               │
  │          │          │  └───────────────────┘                                             │
  │          │          │         │                                                          │
  │          │          │         │ success                                                  │
  │          │          │         ▼                                                          │
  │          │          │  ┌───────────────────┐                                             │
  │          │          │  │ Step 3: Deliver   │──── POST webhook ─────────────────────>     │
  │          │          │  │ to SIEM            │     (HMAC-signed payload)                  │
  │          │          │  └───────────────────┘     <── 200 OK ──────────────────────       │
  │          │          │         │                                                          │
  │          │          │         │ success                                                  │
  │          │          │         ▼                                                          │
  │          │          │  ┌───────────────────┐                                             │
  │          │          │  │ Step 4: Write     │──── Publish ──────────────────────────>     │
  │          │          │  │ Audit Trail        │     (audit.trail topic)                    │
  │          │          │  └───────────────────┘                                             │
  │          │          │         │                                                          │
  │          │          │         │ all steps complete                                       │
  │          │          │         ▼                                                          │
  │          │          │  ┌───────────────────┐                                             │
  │          │          │  │ Saga Complete     │  (alert status: "open")                     │
  │          │          │  └───────────────────┘                                             │
  │          │          └────────────────────────────────────────────────────────────────────┘
  └──────────┘
```

---

## Level 3: State Transitions

### Alert Record State

```
                           ┌────────────────────────────────────────────────────────────┐
                           │                    ALERT LIFECYCLE                         │
                           │                                                            │
                           │                    ┌──────────┐                            │
                           │                    │  OPEN    │  (detected, persisted)      │
                           │                    └────┬─────┘                            │
                           │                         │                                  │
                           │                         │  Analyst reviews                 │
                           │                    ┌────▼─────┐                            │
                           │              ┌─────│ACKNOWLEDGED│───┐                      │
                           │              │     └────┬─────┘   │                      │
                           │              │          │         │                      │
                           │              │          │         │                      │
                           │         ┌────▼────┐ ┌───▼────┐ ┌──▼──────┐               │
                           │         │INVESTIG-│ │ RESOLVED│ │FALSE    │               │
                           │         │ATING    │ │ (actual │ │POSITIVE │               │
                           │         │          │ │ threat) │ │(dismiss)│               │
                           │         └─────────┘ └─────────┘ └─────────┘               │
                           │                                                            │
                           │  State transitions via:                                    │
                           │    PUT /v1/threats/{id}/acknowledge                        │
                           │    PUT /v1/threats/{id}/resolve                            │
                           │                                                            │
                           │  On resolve: MTTR computed as resolved_at - created_at     │
                           └────────────────────────────────────────────────────────────┘
```

### Alert Saga Step State

```
                           ┌─────────────────────────────────────────────┐
                           │          SAGA STEP STATE MACHINE           │
                           │                                             │
                           │           ┌──────────┐                     │
                           │           │ PENDING  │                     │
                           │           └────┬─────┘                     │
                           │                │                           │
                           │          ┌─────▼──────┐                    │
                           │          │ IN_PROGRESS │                   │
                           │          └─────┬──────┘                    │
                           │                │                           │
                           │          ┌─────┴──────┐                   │
                           │          │            │                    │
                           │          ▼            ▼                    │
                           │   ┌──────────┐ ┌──────────┐               │
                           │   │COMPLETED │ │ FAILED   │               │
                           │   └──────────┘ └────┬─────┘               │
                           │                     │                      │
                           │            ┌────────┴────────┐            │
                           │            │                 │            │
                           │            ▼                 ▼            │
                           │   ┌──────────────┐  ┌──────────────┐      │
                           │   │ Retry (×5)   │  │  ABANDONED   │      │
                           │   │ exponential  │  │ (max retries │      │
                           │   │ backoff      │  │  exhausted)  │      │
                           │   └──────────────┘  └──────────────┘      │
                           └─────────────────────────────────────────────┘
```

### Redis Sliding Window State (Brute Force Detector)

```
                           ┌─────────────────────────────────────────────┐
                           │     REDIS SORTED SET (sliding window)      │
                           │                                             │
                           │  Key:   brute_force:{org}:{user}           │
                           │  Score: Unix timestamp (ms)                 │
                           │  Value: event_id                           │
                           │                                             │
                           │  ┌──── Window: 5 minutes ──────────────┐   │
                           │  │                                      │   │
                           │  │  ZADD event1(ts1)  ZADD event2(ts2)  │   │
                           │  │  ZADD event3(ts3)                    │   │
                           │  │                                      │   │
                           │  │  ZREMRANGEBYSCORE -inf (now-5min)    │   │
                           │  │  ZCOUNT → if > 10 → THREAT          │   │
                           │  └──────────────────────────────────────┘   │
                           │                                             │
                           │  TTL: 10 minutes (gc safety margin)        │
                           └─────────────────────────────────────────────┘
```

---

## Consumer Group Mapping

```
┌──────────────────────┬──────────────────┬────────────┬──────────────────┐
│       Detector       │   Kafka Topic    │ Group ID   │ Partition Assign │
├──────────────────────┼──────────────────┼────────────┼──────────────────┤
│ BruteForce           │ auth.events      │ threat-bf  │ Round-robin      │
│ ImpossibleTravel     │ auth.events      │ threat-it  │ Round-robin      │
│ OffHoursAccess       │ auth.events      │ threat-oh  │ Round-robin      │
│ AccountTakeover      │ auth.events      │ threat-ato │ Round-robin      │
│ PrivilegeEscalation  │ auth.events      │ threat-pe  │ Round-robin      │
│ PrivilegeEscalation  │ policy.changes   │ threat-pe  │ Round-robin      │
│ DataExfiltration     │ data.access      │ threat-de  │ Round-robin      │
├──────────────────────┼──────────────────┼────────────┼──────────────────┤
│ Alerting Saga        │ threat.alerts    │ alert-saga │ Round-robin      │
└──────────────────────┴──────────────────┴────────────┴──────────────────┘
```

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Event Sources (IAM, Policy, Audit, SDK)** | Produce events via Transactional Outbox → Kafka |
| **Threat Service** | Consumes events, runs 6 parallel detectors, scores, persists, publishes alerts |
| **Redis** | Sliding windows, state tracking, geo cache, device fingerprints, 24h alert cache |
| **MongoDB (threats.alerts)** | Persistent alert storage (written by Threat Service) |
| **Kafka (threat.alerts)** | Async delivery of alerts to Alerting Service |
| **Alerting Service** | Runs 4-step saga: persist → notify → SIEM → audit trail |
| **SIEM Webhook** | Delivers normalized alerts to external SIEM (Splunk/Datadog/Sentinel) |
| **Compliance Service** | Long-term storage in ClickHouse for reporting (GDPR/SOC2/HIPAA) |
| **MongoDB (alerting.alerts)** | Saga state + per-step status for each alert |
| **Angular Dashboard** | Fetches alerts via Threat Service HTTP API, displays charts + list |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Kafka consumer offset commit fails** | Threat Svc | Next rebalance re-processes last batch (at-least-once) |
| **Redis unavailable** | Threat Svc | Detector falls back to conservative mode (no state = no detection) |
| **MongoDB write fails** | Threat Svc | Alert persisted to Kafka only; downstream alert-saga handles storage |
| **Kafka broker down** | Threat Svc | Consumer pauses, reconnects with backoff; events backlog on broker |
| **SIEM endpoint unreachable** | Alerting Svc | Step 3 retries ×5 with exponential backoff; saga stays IN_PROGRESS |
| **SIEM retries exhausted** | Alerting Svc | Step 3 marked ABANDONED; alert still persisted + notified (partial success) |
| **Threat.alerts topic not created** | Threat Svc | Publisher fails → detector retries with backoff on init |
| **Outbox relay dead** | Event Src | Events remain PENDING; DLQ after 5 retries; manual recovery needed |
| **GeoIP DB stale/missing** | Threat Svc | ImpossibleTravel falls back to "no detection" (conservative) |
| **Circuit breaker open (Redis)** | Threat Svc | JWT blocklist check skipped; rate limiting disabled |
| **Saga step timeout** | Alerting Svc | 30s per-step timeout; step marked FAILED → retry |
| **DLP block mode** | Audit Svc | If DLP unreachable & block mode → 422, event never reaches Kafka |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_threat_detections_total` | Counter | `detector`, `severity` | Threat Service |
| `detector_processing_latency_seconds` | Histogram | `detector` | Threat Service |
| `openguard_events_consumed_total` | Counter | `topic`, `consumer_group` | Threat Service |
| `openguard_saga_step_duration_seconds` | Histogram | `step` | Alerting Service |
| `openguard_saga_step_total` | Counter | `step`, `status` | Alerting Service |
| `openguard_alerts_total` | Counter | `severity`, `status` | Alerting Service |

### Key Traces (Jaeger)

- `threat.detect.{detector_name}` — from Kafka consume to alert publish
- `alert.saga.process` — from threat.alerts consume to saga completion
- `saga.step.{step_number}` — individual saga step with retry spans

### Audit Log Events

| Event | When | Payload |
|-------|------|---------|
| `threat.alert.created` | Alert persisted to MongoDB | Alert ID, detector, score, user_id |
| `threat.alert.acknowledged` | Analyst acknowledges | Alert ID, analyst_id, timestamp |
| `threat.alert.resolved` | Analyst resolves | Alert ID, analyst_id, MTTR, notes |
| `alert.saga.completed` | All 4 saga steps done | Alert ID, step statuses, total duration |
| `alert.siem.delivered` | SIEM webhook succeeded | Alert ID, SIEM type, response code |
| `alert.siem.failed` | SIEM webhook failed | Alert ID, error, retry count |
