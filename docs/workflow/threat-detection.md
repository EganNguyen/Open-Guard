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
                                    │  │  │  │ Scorer   │   │Persister │──>│Kafka Publisher │ │                │ │
                                    │  │  │  │ (UNUSED  │   │ (MongoDB │   │ (threat.alerts │ │                │ │
                                    │  │  │  │  — dead  │   │  alerts) │   │   topic)       │ │                │ │
                                    │  │  │  │  code)   │   │          │   │                │ │                │ │
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
│  │    │     └─ (scorer.CompositeScore exists but NEVER called — dead code)    │
│  │    │     └─ Persist to MongoDB threats.alerts                              │
│  │    │     └─ Publish to threat.alerts Kafka topic                           │
│  │    │     └─ Cache in Redis with 24h TTL                                    │
│  │    │                                                                        │
│  │    │  5. Commit Kafka offset                                                │
│  │    │                                                                        │
│  │    │  6. Would emit metrics (NOT IMPLEMENTED):                              │
│  │    │     └─ openguard_threat_detections_total{detector,severity}           │
│  │    │     └─ processing_latency_seconds{detector}                           │
│  │    │     (metrics declared in telemetry/metrics.go, never .Inc()/.Observe())│
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
┌──────────────────────┬──────────────────┬──────────────────────────┬──────────────────┐
│       Detector       │   Kafka Topic    │ Group ID (actual)       │ Partition Assign │
├──────────────────────┼──────────────────┼──────────────────────────┼──────────────────┤
│ BruteForce           │ auth.events      │ threat-detector         │ Round-robin      │
│ ImpossibleTravel     │ auth.events      │ threat-detector         │ Round-robin      │
│ OffHoursAccess       │ auth.events      │ threat-detector         │ Round-robin      │
│ AccountTakeover      │ auth.events      │ threat-detector         │ Round-robin      │
│ DataExfiltration     │ data.access      │ threat-detector         │ Round-robin      │
│ PrivilegeEscalation  │ auth.events      │ threat-detector-auth    │ Round-robin      │
│ PrivilegeEscalation  │ policy.changes   │ threat-detector-policy  │ Round-robin      │
├──────────────────────┼──────────────────┼──────────────────────────┼──────────────────┤
│ Alerting Saga        │ threat.alerts    │ alert-saga              │ Round-robin      │
└──────────────────────┴──────────────────┴──────────────────────────┴──────────────────┘
```

**⚠️ Bug:** 5 detectors (BF, IT, OH, DE, ATO) share the same consumer group `threat-detector`, making them competing consumers — each partition message is delivered to only one detector. This is likely unintentional; each detector should have a unique group ID to receive all events. Configurable via env var `KAFKA_GROUP_ID` in `services/threat/main.go:63`.

```
Source: services/threat/main.go:63-66          (base groupID = "threat-detector")
       services/threat/pkg/detector/privilege_escalation.go:34,40  (+ "-auth" / "-policy")
```

---

## Code Anomalies

The following discrepancies between documented design and actual code were found during code exploration:

### 1. Scorer Package — Dead Code

**Location:** `services/threat/pkg/scorer/scorer.go` (47 lines + 73 lines of tests)

A `scorer` package exists defining:
- `Score` struct (`Value float64`, `Source string`, `OccurredAt time.Time`)
- `CompositeScore([]Score) float64` — recency-decayed max: `value * exp(-0.05 * minutes_ago)`
- `Severity(float64) string` — `≥0.95 → CRITICAL`, `≥0.8 → HIGH`, `≥0.5 → MEDIUM`, else `LOW`

**Problem:** This package is **never imported or used anywhere** in the codebase. All 6 detectors assign hardcoded inline scores:
- BruteForce: `Score: 0.9`
- ImpossibleTravel: `Score: 0.9`
- OffHours: `Score: 0.5`
- DataExfiltration: `Score: 0.7`
- AccountTakeover: `Score: 0.7`
- PrivilegeEscalation: `Score: 0.9`

There is no central aggregation step. No code calls `CompositeScore()` or `Severity()`. The scorer was written and tested but never wired into the detection pipeline.

### 2. Prometheus Metrics — Declared but Never Instrumented

**Location:** `services/threat/pkg/telemetry/metrics.go`

| Go Variable | Prometheus Metric | Type | Labels | Status |
|---|---|---|---|---|
| `ThreatsDetected` | `openguard_threat_detections_total` | CounterVec | `type`, `severity` | Never `.Inc()` or `.Add()` |
| `ProcessingLatency` | `openguard_threat_processing_duration_seconds` | HistogramVec | `type` | Never `.Observe()` |

Both metrics are `promauto`-registered (appear on `/metrics` endpoint) but have **zero `.Inc()` / `.Observe()` calls** anywhere in the codebase. They will always report zero values.

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

## Example & Demo Data Generation

The threat service can be driven with synthetic data through several mechanisms: the **Example App** (Attack Simulator), **pentest injection scripts**, **load test generators**, and the **Audit Ingest HTTP API**.

### Level 1: Example Data Flow Overview

```
                               ┌──────────────────────────────────────────────────┐
                               │              EXAMPLE DATA SOURCES                │
                               │                                                  │
                               │  ┌──────────────────┐  ┌─────────────────────┐  │
                               │  │ Attack Simulator  │  │ Pentest Inject      │  │
                               │  │ (React UI)        │  │ (kcat CLI)          │  │
                               │  │ apps/example/     │  │ pentest/scripts/    │  │
                               │  │                   │  │ kafka-inject.sh     │  │
                               │  │ 7 attack types:   │  │                     │  │
                               │  │ • SQLi            │  │ Injects raw JSON    │  │
                               │  │ • XSS             │  │ to Kafka topics:    │  │
                               │  │ • Rate Limit      │  │ • auth.events       │  │
                               │  │ • Bot UA          │  │ • policy.changes    │  │
                               │  │ • Path Traversal  │  │                     │  │
                               │  │ • Large Payload   │  │                     │  │
                               │  │ • Brute Force     │  │                     │  │
                               │  └────────┬─────────┘  └──────────┬──────────┘  │
                               │           │                       │              │
                               │           │                       │              │
                               │           ▼                       ▼              │
                               │  ┌────────────────────────────────────────────┐  │
                               │  │              k6 Load Tests                │  │
                               │  │                                            │  │
                               │  │  ┌─────────────────┐ ┌──────────────────┐ │  │
                               │  │  │ event-ingest.js │ │kafka-throughput.js│ │  │
                               │  │  │ 20k events/s    │ │ 50k events/s     │ │  │
                               │  │  │ HTTP POST       │ │ direct Kafka      │ │  │
                               │  │  │ /v1/events/     │ │ produce (xk6-     │ │  │
                               │  │  │ ingest          │ │ kafka)            │ │  │
                               │  │  └────────┬────────┘ └────────┬─────────┘ │  │
                               │  └───────────┼────────────────────┼───────────┘  │
                               └──────────────┼────────────────────┼──────────────┘
                                              │                    │
                    ┌─────────────────────────┼────────────────────┼─────────────────┐
                    │                         ▼                    ▼                  │
                    │  ┌────────────────────────────────────────────────────────────┐ │
                    │  │                    ENTRY POINTS                            │ │
                    │  │                                                            │ │
                    │  │  ┌──────────────────┐       ┌──────────────────────────┐  │ │
                    │  │  │ HTTP Ingest API  │       │ Direct Kafka Injection   │  │ │
                    │  │  │ POST /v1/events/ │       │ (kcat / k6)              │  │ │
                    │  │  │ ingest           │       │ Topics:                  │  │ │
                    │  │  │ (Audit Service)  │       │ • auth.events            │  │ │
                    │  │  │                  │       │ • policy.changes         │  │ │
                    │  │  │ Accepts custom   │       │ • data.access            │  │ │
                    │  │  │ topic in payload │       │ • audit.trail            │  │ │
                    │  │  └────────┬─────────┘       └───────────┬──────────────┘  │ │
                    │  └───────────┼─────────────────────────────┼────────────────┘ │
                    └──────────────┼─────────────────────────────┼──────────────────┘
                                   │                             │
                                   ▼                             ▼
                    ┌──────────────────────────────────────────────────────────────┐
                    │                    KAFKA EVENT BUS                           │
                    │  auth.events | policy.changes | data.access | audit.trail   │
                    └──────────────────────────┬───────────────────────────────────┘
                                               │
                                               ▼
                    ┌──────────────────────────────────────────────────────────────┐
                    │                   THREAT SERVICE                             │
                    │  6 detectors consume → produce alerts → MongoDB + Kafka      │
                    └──────────────────────────────────────────────────────────────┘
                                               │
                                               ▼
                    ┌──────────────────────────────────────────────────────────────┐
                    │            ANGULAR DASHBOARD (REST API polling)              │
                    │  GET /v1/threats/alerts → table, charts, stats              │
                    └──────────────────────────────────────────────────────────────┘
```

### Level 2: Example Data Source Details

#### A. Attack Simulator (React UI)

**Location:** `apps/example/client/src/components/AttackSimulator.tsx`

The Example App includes an **Attack Simulator** component with 7 predefined attack buttons. Each button triggers HTTP requests to the example app server (port 3001), which is protected by the OpenGuard middleware.

| Attack | Method | Path | Payload | Expected Detector |
|--------|--------|------|---------|-------------------|
| SQL Injection | GET | `/api/test/sqli` | `q: "1' UNION SELECT * FROM users--"` | `sql-injection` |
| XSS Payload | GET | `/api/test/xss` | `q: "<script>alert(1)</script>"` | `xss` |
| Rate Limit | GET | `/api/test/rate-limit` | (repeats 3×) | `rate-limiter` |
| Bot UA | GET | `/api/test/bot` | Header: `User-Agent: python-requests/2.28.0` | `bot-detection` |
| Path Traversal | GET | `/api/test/path` | `file: "../../etc/passwd"` | `path-traversal` |
| Large Payload | POST | `/api/comment` | 2MB body | `payload-size` |
| Brute Force | POST | `/api/login` | `{username: "admin", password: "wrong"}` (repeats 6×) | `auth-brute-force` |

**Data flow from Attack Simulator to Threat Service:**

```
┌──────────────────┐     HTTP      ┌──────────────────┐  guard events   ┌──────────────────┐
│ AttackSimulator  │──────────────>│ Example App      │────────────────>│ OpenGuard SDK     │
│ (React, port     │  GET/POST     │ (Express, port   │ onGuardBlock/   │ (openguard-client)│
│  3000)           │  /api/test/*  │  3001)           │ onGuardResult   │                   │
└──────────────────┘               └──────────────────┘                 └────────┬─────────┘
                                                                                  │
                                                                                  │ POST /v1/events/ingest
                                                                                  ▼
                                                                          ┌──────────────────┐
                                                                          │ Audit Service    │
                                                                          │ (IngestHandler)  │
                                                                          └────────┬─────────┘
                                                                                  │
                                                                                  │ Kafka publish
                                                                                  ▼
                                                                          ┌──────────────────┐
                                                                          │ auth.events /    │
                                                                          │ audit.trail      │
                                                                          └────────┬─────────┘
                                                                                  │
                                                                                  ▼
                                                                          ┌──────────────────┐
                                                                          │ Threat Detectors │
                                                                          └──────────────────┘
```

**Example App event ingestion code** (`apps/example/server/openguard-client.ts`): On every guard block or result, the SDK calls `ogClient.ingestEvent()` which POSTs to the Audit Service's `/v1/events/ingest` endpoint. The ingest handler can route to any Kafka topic (default: `audit.trail`, or override via `topic` field).

```typescript
// apps/example/server/index.ts — Guard block handler
globalEventEmitter.onGuardBlock((event) => {
  ogClient.ingestEvent({
    type: 'threat',
    actor_id: 'anonymous',
    action: 'BLOCK',
    status: 'detected',
    payload: { request: event.request, response: event.response },
  });
});
```

#### B. Pentest Kafka Injection

**Location:** `pentest/scripts/kafka-inject.sh`

Directly injects events into Kafka topics using `kcat`. This bypasses all HTTP API layers and feeds events straight to the threat detectors.

| Test | Kafka Topic | Sample Payload | Triggers Detector |
|------|------------|----------------|-------------------|
| Test 2 | `auth.events` | `{"type":"LOGIN","email":"test@evil.com","source_ip":"10.0.0.1"}` | BruteForce, ImpossibleTravel, OffHours, AccountTakeover |
| Test 3 | `policy.changes` | `{"action":"CREATE","policy_id":"pwned-policy","org_id":"alpha"}` | PrivilegeEscalation |
| Test 5 | Audit Ingest API | Sent via `curl POST /v1/events/ingest` with custom `topic:"policy.changes"` | PrivilegeEscalation |

**Usage:**
```bash
# From project root
cd pentest && ./scripts/kafka-inject.sh

# Or manual single injection:
echo '{"type":"LOGIN","email":"evil@test.com","source_ip":"1.2.3.4"}' | \
  kcat -P -b localhost:9092 -t "auth.events"
```

#### C. k6 Load Tests

**Location:** `tests/load/`

Two k6 scripts generate high-volume example data:

| Script | Rate | Method | Topic | Payload Pattern |
|--------|------|--------|-------|-----------------|
| `event-ingest.js` | 20,000/s | HTTP POST `/v1/events/ingest` | `audit.trail` (configurable) | `auth.login.success` with random IPs, user agents |
| `kafka-throughput.js` | 50,000/s | Direct Kafka produce (xk6-kafka) | `audit.trail` | `resource.read` with random orgs/users |

**Usage:**
```bash
make load-test
# Or run individually:
k6 run tests/load/event-ingest.js --env BASE_URL=http://localhost:8083
k6 run tests/load/kafka-throughput.js --env KAFKA_BROKERS=localhost:9092
```

The `make load-test` target runs all 7 k6 test suites sequentially (auth-login, policy-eval, event-ingest, audit-query, scim-users, compliance, kafka-throughput).

#### D. Audit Ingest HTTP API

**Location:** `services/audit/pkg/handlers/ingest.go`

The primary HTTP entry point for external example events. Accepts any JSON payload and publishes it to a configurable Kafka topic.

**Endpoint:** `POST /v1/events/ingest`

**Authentication:** JWT with org-scoped session (requires `X-Org-ID` or middleware-derived org).

**Payload structure:**
```json
{
  "event_id": "evt-1234567890",
  "type": "auth.login.success",
  "topic": "auth.events",
  "org_id": "org-1",
  "actor": { "id": "user-123", "type": "user" },
  "action": "login",
  "resource": "iam:session",
  "metadata": {
    "ip": "1.2.3.4",
    "user_agent": "Mozilla/5.0"
  }
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `topic` | No | `audit.trail` | Target Kafka topic for routing (e.g. `auth.events`, `policy.changes`, `data.access`) |
| `event_id` | No | Auto-generated | Unique event identifier |
| `org_id` | Auto-set | From JWT | Overridden by middleware to match authenticated tenant |

**DLP Integration:** If the Audit Service is in `block` mode (env: `DLP_MODE=block`), the ingest handler performs a synchronous DLP scan before publishing. If PII is detected, the request returns **422 Unprocessable Entity** and the event never reaches Kafka.

#### E. IAM Seed Data

**Location:** `services/iam/pkg/seed/seed.go`

The seeder creates baseline **database records only** (no Kafka events). Triggered via `make seed` or `SEED_DB=true` env var on IAM service startup.

**Seeded entities:**
- 2 organizations: `OpenGuard System`, `Acme Corp`
- 3 users with bcrypt-hashed passwords (admin, analyst, viewer roles)
- 1 OAuth connector (task management app)

**Example users created:**
| Username | Password (plain) | Org |
|----------|-----------------|-----|
| `admin` | `admin123` | OpenGuard System |
| `analyst` | `analyst123` | Acme Corp |
| `viewer` | `viewer123` | Acme Corp |

After seeding, you can log in via the Angular dashboard to generate real auth events that flow through the Transactional Outbox → Kafka → Threat Detector pipeline.

### Consumer Walkthrough: From Demo to Dashboard

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│           END-TO-END: CREATING A BRUTE FORCE THREAT ALERT WITH EXAMPLE DATA         │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                     │
│  1. Seed database                                                                   │
│     ┌──────────────┐     ┌───────────────────┐     ┌─────────────────────────────┐ │
│     │ make seed    │────>│ Creates orgs +     │────>│ Users: admin, analyst,     │ │
│     │              │     │ users in Postgres  │     │ viewer available in DB     │ │
│     └──────────────┘     └───────────────────┘     └─────────────────────────────┘ │
│                                                                                     │
│  2. Generate auth events via k6 or pentest                                          │
│     ┌──────────────┐     ┌───────────────────┐     ┌─────────────────────────────┐ │
│     │ k6 run event- │────>│ POST /v1/events/  │────>│ Kafka topic: auth.events   │ │
│     │ ingest.js     │     │ ingest (Audit Svc) │     │ (12 partitions)            │ │
│     └──────────────┘     └───────────────────┘     └─────────────────────────────┘ │
│                                                                                     │
│  Or inject directly:                                                                │
│     ┌──────────────┐     ┌───────────────────┐     ┌─────────────────────────────┐ │
│     │ kcat -P -t   │────>│ kcat publishes     │────>│ Kafka topic: auth.events   │ │
│     │ auth.events  │     │ raw JSON payload  │     │                             │ │
│     └──────────────┘     └───────────────────┘     └─────────────────────────────┘ │
│                                                                                     │
│  3. Threat detector processes                                                       │
│     ┌───────────────────────────────────────────────────────────────────────────┐   │
│     │ BruteForceDetector consumes from auth.events                              │   │
│     │  • Tracks failed attempts per IP in Redis (sorted set, 5min window)      │   │
│     │  • After 10+ failed attempts → alert created                              │   │
│     └───────────────────────────────────────────────────────────────────────────┘   │
│                                                                                     │
│  4. Alert persisted                                                                 │
│     ┌──────────────────────┐     ┌───────────────────┐     ┌────────────────────┐  │
│     │ MongoDB threats.     │<────│ store.CreateAlert │<────│ Detector fires     │  │
│     │ alerts collection    │     │                   │     │ HIGH severity      │  │
│     └──────────────────────┘     └───────────────────┘     └────────────────────┘  │
│                                                                                     │
│  5. View in dashboard                                                              │
│     ┌──────────────────────┐     ┌───────────────────┐                              │
│     │ Open /threats in     │────>│ GET /v1/threats/   │                              │
│     │ Angular Dashboard    │     │ alerts → see alert │                              │
│     └──────────────────────┘     └───────────────────┘                              │
│                                                                                     │
│  6. Interactive alert management                                                    │
│     ┌──────────────────────┐     ┌──────────────────────────────────────────────┐   │
│     │ Click "Acknowledge"  │────>│ POST /v1/threats/alerts/{id}/acknowledge    │   │
│     │ Click "Resolve"      │────>│ POST /v1/threats/alerts/{id}/resolve        │   │
│     │                      │     │ (MTTR computed automatically)               │   │
│     └──────────────────────┘     └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### Quickstart Commands

| Step | Command | Description |
|------|---------|-------------|
| 1 | `make seed` | Seed Postgres with orgs and users |
| 2 | `make dev` | Start all services (infra + Go services + Angular) |
| 3a | `make load-test` | Run full k6 load test suite (generates events across all topics) |
| 3b | `pentest/scripts/kafka-inject.sh` | Inject sample auth + policy events |
| 3c | Manual kcat | `echo '{"type":"LOGIN","email":"test@test.com","source_ip":"10.0.0.1"}' \| kcat -P -b localhost:9092 -t auth.events` |
| 3d | Attack Simulator UI | Open `http://localhost:3000` in Example App → click attack buttons |
| 4 | Dashboard | Open `http://localhost:4200/threats` to see alerts |

### Failure Modes for Example Data

| Failure | Symptom | Root Cause | Fix |
|---------|---------|------------|-----|
| **Attack simulator requests all pass** | No blocks registered | OpenGuard middleware not configured on example app routes | Check `guard.config.ts` and middleware setup |
| **kcat injection fails** | `kcat not available` | `kcat` (kafkacat) not installed | `brew install kcat` or use `kafka-console-producer` |
| **No alerts after injection** | Empty dashboard | Event format doesn't match detector expectations | Look at detector filter logic in `services/threat/pkg/detector/*.go` |
| **Ingest API returns 401** | `Unauthorized` | Missing or invalid JWT token | Use valid admin token or bypass auth in dev mode |
| **Ingest API returns 422** | DLP blocked the event | DLP in `block` mode detected PII-like content | Set `DLP_MODE=audit` or remove sensitive fields from payload |
| **k6 tests fail** | Connection refused | Target service not running | Run `make dev` first |
| **Dashboard empty after seed** | No threat alerts visible | Seed only creates DB records, not events | Run load test or attack simulator to generate events |

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source | Status |
|--------|------|--------|--------|--------|
| `openguard_threat_detections_total` | Counter | `detector`, `severity` | Threat Service | Declared, never `.Inc()` |
| `openguard_threat_processing_duration_seconds` | Histogram | `detector` | Threat Service | Declared, never `.Observe()` |
| `openguard_events_consumed_total` | Counter | `topic`, `consumer_group` | Threat Service | Unknown — not in codebase |
| `openguard_saga_step_duration_seconds` | Histogram | `step` | Alerting Service | Active |
| `openguard_saga_step_total` | Counter | `step`, `status` | Alerting Service | Active |
| `openguard_alerts_total` | Counter | `severity`, `status` | Alerting Service | Active |

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

---

## Data Storage

All persistent and transient data stores used by threat-related services, including schema, key patterns, TTLs, and ownership.

### Service ↔ Data Store Matrix

| Data Store | Service | Operation | Purpose |
|------------|---------|-----------|---------|
| MongoDB `threats.alerts` | Threat Service | **Write** (6 detectors) + **Read/Update** (HTTP API) | Alert persistence |
| MongoDB `alerting.alerts` | Alerting Service | **Write** (saga step 1) + **Read/Update** (saga + HTTP) | Saga state + enriched alerts |
| Redis (17 key patterns) | Threat Service | **Read/Write** per detector | Sliding windows, baselines, dedup, caches |
| PostgreSQL `outbox_records` (IAM) | IAM + Outbox Relay | **Insert** (app) → **Read/Delete** (relay) → Kafka `auth.events` | Transactional outbox for auth events |
| PostgreSQL `outbox_records` (Policy) | Policy + Outbox Relay | **Insert/Update** (app) → **Read/Delete** (relay) → Kafka `policy.changes` | Transactional outbox for policy events |
| ClickHouse `events` | Compliance | **Write** (ClickHouseWriter from `audit.trail`) + **Read** (stats/posture API) | Long-term audit event archive (2yr TTL) |
| ClickHouse `event_counts_daily` | Compliance | **Auto** (materialized view from events) | Pre-aggregated daily event counts |
| ClickHouse `alert_stats` | Compliance | **Schema only** (no write code yet) | Planned alert stat aggregation |
| Prometheus counters/histograms | Threat + Alerting + Kafka | **Write** on detect/publish/commit | Operational observability |

---

### Level 1: Storage Topology

```
                                  ┌──────────────────────────────────────────────────────────┐
                                  │                     REDIS (18 key patterns)               │
                                  │                                                          │
                                  │  BruteForce:                             1h TTL          │
                                  │   ┌──────────────────────────┐         ┌──────────────┐  │
                                  │   │ bruteforce:ip:{ip}       │ ZSET    │ travel:{uid} │  │
                                  │   │ bruteforce:user:{email}  │ ZSET    └──────────────┘  │
                                  │   │ alert_fired:*            │ STRING                    │
                                  │   └──────────────────────────┘       OffHours:            │
                                  │                                         7d TTL            │
                                  │  AccountTakeover:                     ┌──────────────────┐│
                                  │   ┌──────────────────────────┐        │ offhours:{org}:  ││
                                  │   │ ato:pwchange:{uid} STRING│        │ {uid}:{YYYY-MM-  ││
                                  │   │ ato:devices:{uid}   SET  │        │ DD}          STR ││
                                  │   └──────────────────────────┘        └──────────────────┘│
                                  │                                                          │
                                  │  DataExfiltration:                  PrivilegeEscalation: │
                                  │   ┌──────────────────────────┐      ┌──────────────────┐  │
                                  │   │ access:{org}:{uid}  ZSET │      │ privsec:login:{  │  │
                                  │   │ baseline:{org}:access_   │      │ uid}        STR  │  │
                                  │   │   mean / _stddev    STR  │      └──────────────────┘  │
                                  │   └──────────────────────────┘                            │
                                  │                                                          │
                                  │  Shared:                                                  │
                                  │   ┌──────────────────────────────────────────────────┐   │
                                  │   │ threat:* (alert cache, 24h TTL, all detectors)  │   │
                                  │   │ blocklist:{jti} (JWT revocation, middleware)     │   │
                                  │   └──────────────────────────────────────────────────┘   │
                                  └──────────────────────────────────────────────────────────┘

    ┌──────────────┐     ┌───────────────────┐    ┌───────────────────────┐    ┌──────────────────┐
    │  PostgreSQL   │     │     KAFKA         │    │   MONGODB             │    │   CLICKHOUSE     │
    │               │     │    (in motion)    │    │                       │    │                  │
    │ outbox_records│────>│ auth.events      │    │ threats.alerts        │    │ events (2yr TTL)│
    │ (IAM)         │     │ policy.changes   │    │  ┌─────────────────┐  │    │ event_counts_    │
    │  id UUID      │     │ data.access      │    │  │ _id ObjectID   │  │    │ daily (MV)       │
    │  org_id UUID  │     │ threat.alerts    │    │  │ org_id         │  │    │ alert_stats      │
    │  topic TEXT   │────>│ notifications.   │    │  │ user_id        │  │    │ (planned)        │
    │  payload BYTEA│     │ outbound         │    │  │ detector       │  │    └──────────────────┘
    │  status TEXT  │     │ audit.trail      │    │  │ score          │  │
    │               │     └───────────────────┘    │  │ severity       │  │
    │ outbox_records│                              │  │ status         │  │
    │ (Policy)      │────>                  ──────>│  │ created_at     │  │
    │  payload JSONB│                              │  │ resolved_at    │  │
    └──────────────┘                               │  │ mttr_seconds   │  │
                                                   │  │ metadata (BSON)│  │
                                                   │  └─────────────────┘  │
                                                   │                       │
                                                   │ alerting.alerts      │
                                                   │  ┌─────────────────┐  │
                                                   │  │ _id string     │  │
                                                   │  │ org_id         │  │
                                                   │  │ type           │  │
                                                   │  │ severity       │  │
                                                   │  │ risk_score     │  │
                                                   │  │ detector_id    │  │
                                                   │  │ raw_event (BSON)│  │
                                                   │  │ saga_steps[]   │  │
                                                   │  │ created_at     │  │
                                                   │  │ mttr_seconds   │  │
                                                   │  └─────────────────┘  │
                                                   └───────────────────────┘
```

---

### MongoDB

#### `threats.alerts` — Primary Alert Store

**Written by:** All 6 threat detectors (via `store.CreateAlert()`)
**Read by:** Threat service HTTP handlers (`ListAlerts`, `GetAlert`, `AcknowledgeAlert`, `ResolveAlert`, `GetStats`)
**Database:** `threats`, Collection: `alerts`

| Field | BSON Type | JSON Key | Constraints | Description |
|-------|-----------|----------|-------------|-------------|
| `_id` | `ObjectID` | `id` | Auto-generated, cursor pagination | Unique alert ID |
| `org_id` | `string` | `org_id` | Required | Organization tenant |
| `user_id` | `string` | `user_id` | Required | Affected user |
| `detector` | `string` | `type` | Required | `brute_force`, `impossible_travel`, `off_hours_access`, `data_exfiltration`, `account_takeover`, `privilege_escalation` |
| `score` | `float64` | `risk_score` | 0.0–1.0 | Risk score |
| `severity` | `string` | `severity` | `MEDIUM` / `HIGH` / `CRITICAL` | Severity classification |
| `status` | `string` | `status` | `open` / `acknowledged` / `resolved` | Default: `open` |
| `created_at` | `time.Time` | `created_at` | Auto-set | Alert creation timestamp |
| `resolved_at` | `*time.Time` | `resolved_at` | Omitted if null | Resolution timestamp |
| `mttr_seconds` | `*int64` | `mttr_seconds` | Omitted if null | Mean time to resolve |
| `metadata` | `map[string]interface{}` | `metadata` | Omitted if empty | Detector-specific context (IP, geo, device, etc.) |

**Query patterns:**
- List: `{org_id, status?, severity?, _id: {$lt: cursor}}` sorted by `{_id: -1}`
- Stats: `{$match: {org_id}} → {$group: {_id: "$severity", count, avg_mttr_sec}}`
- Update (acknowledge): `{$set: {status: "acknowledged"}}`
- Update (resolve): `{$set: {status: "resolved", resolved_at: now, mttr_seconds: ...}}`

#### `alerting.alerts` — Saga State Store

**Written by:** Alerting saga (step 1: persists alert)
**Read by:** Alerting HTTP handlers, saga step tracker
**Database:** `alerting`, Collection: `alerts`

| Field | BSON Type | Constraints | Description |
|-------|-----------|-------------|-------------|
| `_id` | `string` | Hex of threat service's ObjectID | Cross-reference key |
| `org_id` | `string` | Required | Organization tenant |
| `type` | `string` | Required | Alert type |
| `severity` | `string` | `LOW` / `MEDIUM` / `HIGH` / `CRITICAL` | Severity |
| `status` | `string` | `open` / `acknowledged` / `resolved` | Current status |
| `risk_score` | `float64` | 0.0–1.0 | Risk score |
| `detector_id` | `string` | Required | Detector name |
| `raw_event` | `bson.M` | Free-form | Full original alert payload |
| `saga_steps` | `[]SagaStep` | Array | Execution trace of each saga step |
| `created_at` | `time.Time` | Auto-set | Creation timestamp |
| `ack_at` | `*time.Time` | Omitted if null | Acknowledged timestamp |
| `resolved_at` | `*time.Time` | Omitted if null | Resolution timestamp |
| `mttr_seconds` | `float64` | Default 0 | Mean time to resolve |

**SagaStep sub-document:**
| Field | Type | Description |
|-------|------|-------------|
| `step` | `string` | `persist` / `notify` / `siem` / `audit` |
| `status` | `string` | `completed` / `failed` |
| `error` | `string` | Error message if failed |
| `at` | `time.Time` | Step execution timestamp |
| `retries` | `int` | Retry count for this step |

---

### Redis — Key Catalog

Every Redis key used across all 6 detectors and shared middleware, organized by namespace.

#### Brute Force Detector

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `bruteforce:ip:{ip}` | ZSET | 5 min | Member: UUID, Score: ms timestamp | `trackFailedAttempt()` | `trackFailedAttempt()` (ZCard) |
| `bruteforce:user:{email}` | ZSET | 5 min | Member: UUID, Score: ms timestamp | `trackFailedAttempt()` | `trackFailedAttempt()` (ZCard) |
| `alert_fired:bruteforce:ip:{ip}` | STRING | 5 min | `"1"` (SET NX) | `trackFailedAttempt()` | `trackFailedAttempt()` (Exists check) |
| `alert_fired:bruteforce:user:{email}` | STRING | 5 min | `"1"` (SET NX) | `trackFailedAttempt()` | `trackFailedAttempt()` (Exists check) |
| `threat:bruteforce:ip:{ip}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |
| `threat:bruteforce:user:{email}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |

#### Impossible Travel Detector

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `travel:{userID}` | STRING | 1 h | `{ip, lat, lon, timestamp}` JSON | Lua GETSET | Lua GETSET returns old value |
| `threat:travel:{userID}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |

#### Off-Hours Detector

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `offhours:{orgID}:{userID}:{YYYY-MM-DD}` | STRING | 7 d | `"1"` (in-hours access) | `processEvent()` on in-hours login | `processEvent()` checks last 3 days |
| `threat:offhours:{orgID}:{userID}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |

#### Data Exfiltration Detector

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `access:{orgID}:{userID}` | ZSET | 1 h | Member: UUID, Score: ms timestamp | `processEvent()` pipeline | `processEvent()` (ZCard) |
| `baseline:{orgID}:access_mean` | STRING | None | Float (mean access count) | External setter | `processEvent()` read for 3-sigma check |
| `baseline:{orgID}:access_stddev` | STRING | None | Float (stddev of access count) | External setter | `processEvent()` read for 3-sigma check |
| `threat:exfiltration:{orgID}:{userID}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |

#### Account Takeover Detector

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `ato:pwchange:{userID}` | STRING | 24 h | `"1"` | `processEvent()` on password.changed | `processEvent()` on login (Exists) |
| `ato:devices:{userID}` | SET | 30 d | Device fingerprint strings | `processEvent()` on login (SAdd) | `processEvent()` (SIsMember) |
| `threat:ato:{userID}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |

#### Privilege Escalation Detector

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `privsec:login:{userID}` | STRING | 1 h | `"1"` | `consumeAuth()` on login success | `consumePolicy()` on role.grant/policy.changed |
| `threat:privesc:{actorID}` | STRING | 24 h | JSON alert payload | `publishThreatEvent()` | Legacy cache |

#### Shared Middleware

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `blocklist:{jti}` | STRING | Remaining token lifetime | `"1"` | IAM service (logout/revoke) | JWT middleware (circuit-brokered) |

#### Fallback / Legacy

| Redis Key | Type | TTL | Data | Written By | Read By |
|-----------|------|-----|------|------------|---------|
| `threat:*` | Multiple | 24 h | All detector alert caches | All detectors (via `publishThreatEvent()`) | `GetThreats()` glob scan (expensive) |

---

### PostgreSQL

#### IAM Outbox (`services/iam/migrations/006_create_outbox.up.sql`)

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | UUID | PK, default `gen_random_uuid()` |
| `org_id` | UUID | NOT NULL |
| `topic` | TEXT | NOT NULL (e.g. `auth.events`) |
| `key` | TEXT | NOT NULL |
| `payload` | BYTEA | NOT NULL (binary serialized event) |
| `status` | TEXT | NOT NULL default `'pending'` |
| `attempts` | INT | NOT NULL default 0 |
| `last_error` | TEXT | Nullable |
| `created_at` | TIMESTAMPTZ | NOT NULL default `now()` |
| `published_at` | TIMESTAMPTZ | Nullable |
| `dead_at` | TIMESTAMPTZ | Nullable |

**Index:** `idx_outbox_pending ON outbox_records (created_at) WHERE status = 'pending'`
**Trigger:** `NOTIFY outbox_new` on INSERT wakes outbox relay
**RLS:** Row-Level Security with org isolation policy
**Relay:** `SELECT ... FOR UPDATE SKIP LOCKED` — moves to DLQ after 5 failed attempts

#### Policy Outbox (`services/policy/migrations/004_create_outbox.up.sql`)

| Column | Type | Constraints |
|--------|------|-------------|
| `id` | UUID | PK, default `gen_random_uuid()` |
| `org_id` | UUID | NOT NULL |
| `topic` | TEXT | NOT NULL (e.g. `policy.changes`) |
| `key` | TEXT | NOT NULL |
| `payload` | JSONB | NOT NULL (vs BYTEA in IAM) |
| `status` | TEXT | NOT NULL default `'pending'` |
| `attempts` | INT | NOT NULL default 0 |
| `last_error` | TEXT | Nullable |
| `dead_at` | TIMESTAMPTZ | Nullable |
| `published_at` | TIMESTAMPTZ | Nullable |
| `created_at` | TIMESTAMPTZ | NOT NULL default `now()` |

**Indexes:** `idx_outbox_status ON outbox_records (status) WHERE status = 'pending'`, `idx_outbox_org_id ON outbox_records (org_id)`
**Trigger:** `NOTIFY` on INSERT
**RLS:** Not defined on Policy outbox (vs IAM which has RLS)

---

### ClickHouse

#### `events` — Long-Term Audit Archive

**Engine:** `ReplacingMergeTree(occurred_at)`
**Partition:** `toYYYYMMDD(occurred_at)`
**Order Key:** `(org_id, type, occurred_at, event_id)`
**TTL:** `occurred_at + INTERVAL 2 YEAR`
**Written by:** Compliance ClickHouseWriter (consumes `audit.trail` Kafka topic)
**Read by:** Compliance `GetPosture()` and `GetStats()` APIs

| Column | Type | Codec |
|--------|------|-------|
| `event_id` | `String` | `ZSTD(3)` |
| `type` | `LowCardinality(String)` | — |
| `org_id` | `String` | `ZSTD(3)` |
| `actor_id` | `String` | `ZSTD(3)` |
| `actor_type` | `LowCardinality(String)` | — |
| `occurred_at` | `DateTime64(3, 'UTC')` | — |
| `source` | `LowCardinality(String)` | — |
| `payload` | `String` | `ZSTD(3)` |

**Threat-related posture query:**
```sql
SELECT countIf(type LIKE 'threat.%') AS threat_events,
       countIf(type LIKE 'auth.%')   AS auth_events
FROM events FINAL
WHERE org_id = ? AND occurred_at > now() - INTERVAL 30 DAY
```

#### `event_counts_daily` — Materialized View

**Engine:** `SummingMergeTree()` | **Partition:** `toYYYYMM(day)` | **Order Key:** `(org_id, type, day)`

```sql
CREATE MATERIALIZED VIEW event_counts_daily
AS SELECT org_id, type, toDate(occurred_at) AS day, count() AS cnt
FROM events GROUP BY org_id, type, day
```

#### `alert_stats` — Planned Aggregation

**Engine:** `SummingMergeTree()` | **Order Key:** `(org_id, day, severity)`
**Note:** Schema exists, no write code yet.

| Column | Type |
|--------|------|
| `org_id` | `String` |
| `day` | `Date` |
| `severity` | `LowCardinality(String)` |
| `count` | `UInt64` |
| `mttr_seconds` | `UInt64` |

---

### Kafka — Data in Motion

| Topic | Partitions | Produced By | Consumed By | Event Types |
|-------|------------|-------------|-------------|-------------|
| `auth.events` | 12 | IAM (outbox relay) | BruteForce, ImpossibleTravel, OffHours, AccountTakeover, PrivilegeEscalation | `auth.login.success`, `auth.login.failed`, `password.changed` |
| `policy.changes` | 6 | Policy (outbox relay) | PrivilegeEscalation | `role.grant`, `policy.changed` |
| `data.access` | 24 | External data service | DataExfiltration | `resource.read`, `resource.write` |
| `threat.alerts` | 12 | All 6 threat detectors | Alerting Saga | Alert JSON (scores, metadata, etc.) |
| `notifications.outbound` | 6 | Alerting Saga (step 2) | Notification service | Alert notification payloads |
| `audit.trail` | 24 | Alerting Saga (step 4) | Compliance ClickHouseWriter | Audit envelope `{event_id, type, org_id, payload}` |

**Publisher config:** `RequiredAcks = RequireAll`, `Async = false`, `BatchSize = 1`, `BatchTimeout = 0`, `AllowAutoTopicCreation = false`
