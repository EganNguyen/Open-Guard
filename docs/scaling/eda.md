# Event-Driven Architecture (EDA) for Scaling

## Core Pattern: Hybrid Pull/Push Transactional Outbox

**Location:** `shared/kafka/outbox/`

Services write events atomically within the same PG transaction as business logic. No dual-write problem, exactly-once delivery.

### Outbox Schema (`outbox_records`)

```
id          UUID PRIMARY KEY
org_id      UUID NOT NULL          -- RLS-enforced tenant isolation
topic       TEXT NOT NULL           -- Kafka topic
key         TEXT NOT NULL           -- Partition key
payload     BYTEA/JSONB            -- Serialized event
status      TEXT DEFAULT 'pending'  -- pending | published | failed | dead
attempts    INT DEFAULT 0
last_error  TEXT
created_at  TIMESTAMPTZ
published_at TIMESTAMPTZ
dead_at     TIMESTAMPTZ
```

Features: RLS-enabled, `pg_notify` trigger on INSERT, index on pending records.

### Relay (`shared/kafka/outbox/relay.go`)

Combines two wakeup strategies:

- **Push:** `LISTEN "outbox_new"` via dedicated PG connection — instant notification on INSERT
- **Pull:** 5s polling ticker — fallback for missed notifications, retries failed records

`drain()` selects up to 100 pending/failed records with `FOR UPDATE SKIP LOCKED`, publishes to Kafka, marks published/failed/dead.

## Kafka Topic Topology

| Topic | Producer | Consumers |
|-------|----------|-----------|
| `auth.events` | SDKs, Example App | Audit, Threat (brute force, impossible travel, off-hours, account takeover) |
| `data.access` | SDKs, Example App | Audit, Threat (data exfiltration) |
| `policy.changes` | Policy Service (outbox) | Audit, Threat (privilege escalation) |
| `saga.orchestration` | IAM Service (outbox), Saga Watcher | IAM Saga Consumer |
| `audit.trail` | Audit Ingest Handler | Compliance (ClickHouse writer), Threat |
| `threat.alerts` | Threat Detectors | Audit, Alerting Saga |
| `connector.events` | Connector infrastructure | Audit |
| `notifications.outbound` | Alerting Saga | Webhook Delivery |
| `webhook.dlq` | Webhook Delivery | — (DLQ) |

## Scaling Mechanisms

### 1. Horizontal Producer Scaling
Multiple relay instances run safely — `FOR UPDATE SKIP LOCKED` ensures each record processed by exactly one instance. More replicas = higher outbox throughput.

### 2. Kafka Consumer Groups
Each service uses a unique `groupID`:
- Audit: `audit-service-{topic}`
- Threat: `threat-detector`
- Compliance: `compliance-service-group`

Partitions enable N consumers per group to read in parallel.

### 3. Independent Detector Goroutines (Threat Service)
All 6+ detectors (`BruteForceDetector`, `ImpossibleTravelDetector`, `OffHoursDetector`, `AccountTakeoverDetector`, `PrivilegeEscalationDetector`, `DataExfiltrationDetector`) run as independent goroutines, each with its own Kafka reader. One slow detector never blocks others.

### 4. Batch Processing Tuning
Each consumer tunes independently:
| Consumer | Max Batch | Flush Interval |
|----------|-----------|----------------|
| Audit | 500 | 1000ms |
| Compliance (ClickHouse) | 5000 | 2000ms |

### 5. Webhook Worker Pool
`services/webhook-delivery`: limits concurrent deliveries to 50 via buffered channel semaphore.

## Dead Letter Queue (DLQ)

Three DLQ paths:

1. **Outbox DLQ:** After 5 failed publish attempts → `status = 'dead'`, `dead_at` timestamp
2. **Kafka Consumer DLQ:** DLP consumer → DLQ topic after 5 consecutive failures
3. **Webhook DLQ:** After 5 retries (1s, 2s, 4s, 8s, 16s backoff) → `webhook.dlq` topic

## Shared EDA Library

```
shared/kafka/
├── envelope.go         # EventEnvelope wire format (ID, Type, OrgID, ActorID, ...)
├── publisher.go        # Kafka publisher (segmentio/kafka-go, sync, RequireAll acks)
├── telemetry.go        # Prometheus OffsetCommitDuration histogram
└── outbox/
    ├── relay.go        # Hybrid push/pull outbox relay
    └── writer.go       # Writes outbox records within DB transactions
```

## Key Architectural Properties

- **Async auditing:** Requests are never blocked by audit trail writes
- **Kafka-decoupled producers:** If Kafka is down, events queue in Postgres and drain on recovery
- **CQRS on Mongo:** Audit service uses separate primary (write) and secondary (read) connections
- **Exactly-once semantics:** Outbox guarantees at-least-once; idempotent consumers (event_id unique index on Mongo) provide exactly-once
