# System Topology Map

## 1. High-Level Architecture

```mermaid
graph TD

    %% External
    subgraph External["External World"]
        BROWSER[Browser / API Clients]
        SIEM[SIEM / Splunk / Datadog]
        WEBHOOK_TARGET[Webhook Targets]
    end

    subgraph Frontend["Frontend Layer"]
        ANG[Angular Admin Dashboard :4200]
        REACT[React Example App :3000]
        REACT_BE[Example App Go Backend :3005]
    end

    subgraph Gateway["Gateway Layer"]
        NX["Nginx Reverse Proxy :8080"]
        CP["Control-Plane :8080/8081"]
    end

    subgraph audit_&_compliance["Audit & Compliance"]
        audit["Audit :8085"]
        compliance["Compliance :8088"]
    end

    subgraph detection_&_alerting["Detection & Alerting"]
        threat["Threat :8084"]
        alerting["Alerting :8086"]
    end

    subgraph identity_&_auth["Identity & Auth"]
        iam["IAM :8082"]
    end

    subgraph integration_layer["Integration Layer"]
        webhook_delivery["Webhook-Delivery :8087"]
        connector_registry["Connector-Registry :8090"]
    end

    subgraph policy_&_dlp["Policy & DLP"]
        policy["Policy :8083"]
        dlp["DLP :8089"]
    end

    subgraph Data["Data Tier"]
        PG[("PostgreSQL 15 :5432")]
        R[("Redis 7 :6379")]
        M[("MongoDB 6.0 :27017")]
        CH[("ClickHouse 23.3 :8123")]
        S3[("S3 / MinIO :4566")]
    end

    subgraph EventBus["Event Bus"]
        K[("Kafka 7.4 :9092")]
    end

    subgraph Monitoring["Observability Stack"]
        PROM[Prometheus :9090]
        GRAF[Grafana :3010]
        JAEG[Jaeger :16686]
        LOKI[Loki :3100]
    end

    subgraph Shared["Shared Library (shared/)"]
        SH_LIB["Crypto · Resilience · Middleware\nDB · Telemetry · Kafka"]
    end

    %% Frontend to Gateway
    BROWSER --> NX
    ANG --> NX
    REACT --> CP
    REACT --> REACT_BE
    REACT_BE -. SDK .-> policy

    %% Gateway to Services
    NX --> iam
    NX --> CP
    CP --> iam
    CP --> policy
    CP --> audit

    %% Services to Data Stores
    iam --> PG
    iam --> R
    policy --> PG
    policy --> R
    dlp --> PG
    dlp --> R
    threat --> M
    threat --> R
    alerting --> M
    alerting --> R
    audit --> M
    audit --> R
    compliance --> PG
    compliance --> CH
    compliance --> S3
    compliance --> R
    webhook_delivery --> PG
    connector_registry --> PG
    connector_registry --> R

    %% Transactional Outbox
    iam ==> PG
    policy ==> PG

    %% Kafka Producers
    iam --> K
    policy --> K
    dlp --> K
    threat --> K
    alerting --> K
    audit --> K
    webhook_delivery --> K
    control_plane --> K

    %% Kafka Consumers
    K --> alerting
    K --> audit
    K --> compliance
    K --> dlp
    K --> iam
    K --> threat
    K --> webhook_delivery

    %% External Integrations
    alerting -. "SIEM Webhook (HMAC-SHA256)" .-> SIEM
    webhook_delivery -. "Webhook Delivery\n(retry + DLQ)" .-> WEBHOOK_TARGET

    %% Monitoring
    iam --> PROM
    policy --> PROM
    dlp --> PROM
    threat --> PROM
    alerting --> PROM
    audit --> PROM
    compliance --> PROM
    webhook_delivery --> PROM
    connector_registry --> PROM
    control_plane --> PROM
    PROM --> GRAF
    iam --> JAEG
    policy --> JAEG
    dlp --> JAEG
    threat --> JAEG
    alerting --> JAEG
    audit --> JAEG
    compliance --> JAEG
    webhook_delivery --> JAEG
    connector_registry --> JAEG
    control_plane --> JAEG
    LOKI --> GRAF

    %% Shared Library
    iam -.-> SH_LIB
    policy -.-> SH_LIB
    dlp -.-> SH_LIB
    threat -.-> SH_LIB
    alerting -.-> SH_LIB
    audit -.-> SH_LIB
    compliance -.-> SH_LIB
    webhook_delivery -.-> SH_LIB
    connector_registry -.-> SH_LIB
    control_plane -.-> SH_LIB

```

---

## 2. Kafka Event Flow — Topic-Level Trace


All event-driven communication follows the **Transactional Outbox** pattern: services write to a local PostgreSQL `outbox_records` table within the same transaction as business logic, then an Outbox Relay polls and publishes to Kafka. Consumers read asynchronously.

```mermaid
graph LR

    subgraph Producers
        alerting[alerting]
        audit[audit]
        control_plane[control-plane]
        dlp[dlp]
        iam[iam]
        policy[policy]
        threat[threat]
        webhook_delivery[webhook-delivery]
    end

    subgraph Topics["Kafka Topics"]
        audit.trail["audit.trail (24p)"]
        auth.events["auth.events (12p)"]
        connector.events["connector.events (24p)"]
        control.plane.events["control.plane.events (3p)"]
        data.access["data.access (24p)"]
        dlp.dlq["dlp.dlq (1p)"]
        notifications.outbound["notifications.outbound (6p)"]
        outbox.dlq["outbox.dlq (3p)"]
        policy.changes["policy.changes (6p)"]
        saga.orchestration["saga.orchestration (12p)"]
        threat.alerts["threat.alerts (12p)"]
        webhook.delivery["webhook.delivery (12p)"]
        webhook.dlq["webhook.dlq (3p)"]
    end

    subgraph Consumers
        alerting_c[Alerting Saga]
        audit_c[Audit Service]
        compliance_c[Compliance Service]
        dlp_c[DLP Consumer]
        iam_c[IAM Saga Consumer]
        threat_c[Threat Detectors]
        webhook_delivery_c[Webhook-Delivery]
    end

    %% Producers to Topics
    audit ==> audit.trail
    iam ==> auth.events
    iam ==> connector.events
    control_plane ==> control.plane.events
    control_plane ==> data.access
    dlp ==> dlp.dlq
    alerting ==> notifications.outbound
    policy ==> policy.changes
    iam ==> saga.orchestration
    threat ==> threat.alerts
    alerting ==> webhook.delivery
    webhook_delivery ==> webhook.dlq

    %% Topics to Consumers
    audit.trail --> audit_c
    audit.trail --> compliance_c
    auth.events --> threat_c
    auth.events --> audit_c
    connector.events --> audit_c
    control.plane.events --> dlp_c
    data.access --> threat_c
    data.access --> audit_c
    data.access --> compliance_c
    policy.changes --> threat_c
    policy.changes --> audit_c
    saga.orchestration --> iam_c
    saga.orchestration --> audit_c
    threat.alerts --> alerting_c
    threat.alerts --> audit_c
    webhook.delivery --> webhook_delivery_c

    %% DLQ failure paths
    outbox_dlq -. "Outbox relay failures" .-> F_DLQ[Dead-Letter Queue]
    webhook_dlq -. "Webhook failures (3x backoff)" .-> F_DLQ
    dlp_dlq -. "DLP scan failures (5 consecutive)" .-> F_DLQ

    style F_DLQ fill:#ffcccc,stroke:#cc0000
    style outbox_dlq fill:#ffe0e0
    style webhook_dlq fill:#ffe0e0
    style dlp_dlq fill:#ffe0e0

```

| `audit.trail` | 24 | audit | audit, compliance | Canonical audit trail |
| `auth.events` | 12 | iam | threat, audit | Authentication, login, MFA events |
| `connector.events` | 24 | iam | audit | Connector lifecycle events |
| `control.plane.events` | 3 | control-plane | dlp | Events for DLP content scanning |
| `data.access` | 24 | control-plane | threat, audit, compliance | Data access events from protected apps |
| `dlp.dlq` | 1 | dlp | — | Dead letters: DLP scan failures |
| `notifications.outbound` | 6 | alerting | — | User notifications |
| `outbox.dlq` | 3 | outbox-relay | — | Dead letters: outbox publish failures |
| `policy.changes` | 6 | policy | threat, audit | Policy CRUD and assignment changes |
| `saga.orchestration` | 12 | iam | iam, audit | Provisioning saga coordination |
| `threat.alerts` | 12 | threat | alerting, audit | Detected threat alerts |
| `webhook.delivery` | 12 | alerting | webhook-delivery | Webhook dispatch queue |
| `webhook.dlq` | 3 | webhook-delivery | — | Dead letters: webhook delivery failures |

---

## 3. Layered System Stack


```mermaid
graph BT
    subgraph DataLayer["Data Tier"]
        PG[("PostgreSQL 15\nPrimary Store · RLS")]
        R[("Redis 7\nCache · Sessions · Rate-Limit")]
        M[("MongoDB 6.0\nAudit · Alerts · Threats")]
        CH[("ClickHouse 23.3\nCompliance Analytics")]
        S3[("S3 / MinIO\nReport Artifacts")]
    end

    subgraph ServiceLayer["Service Layer (Go 1.22+ / Chi / Gorilla Mux)"]
        IAM[IAM :8080\nIdentity · Auth · SCIM · SSO · MFA]
        POL[Policy :8080\nPolicy Engine · RBAC · Evaluation]
        DLP[DLP :8080\nContent Scanning · PII Detection]
        THREAT[Threat :8080\nReal-Time Detection · 6 Detectors]
        ALERT[Alerting :8080\nAlert Lifecycle · SIEM Delivery]
        AUDIT[Audit :8080\nLog Ingestion · CQRS · SSE Stream]
        COMP[Compliance :8080\nPosture · Reports · Aggregation]
        WH[Webhook-Delivery :8080\nDispatch · Retry · DLQ]
        CR[Connector-Registry :8090\n3rd-Party Connectors · SCIM Sync]
    end

    subgraph GatewayLayer["Gateway Layer"]
        CP[Control-Plane :8080\nAPI Gateway · Proxy · Circuit Breaker]
        NX[Nginx :8080\nReverse Proxy · TLS Termination]
    end

    subgraph FrontendLayer["Frontend Layer"]
        ANG[Angular 21 + Tailwind\nAdmin Dashboard :4200]
        REACT[Next.js 14 + React 18\nExample App :3000]
        REACT_BE[Go Chi Backend\nExample App :3005]
    end

    subgraph ExternalLayer["External"]
        EXT[External Clients / Browser]
        SIEM[SIEM / Datadog / Splunk]
        WEBHOOK[Webhook Targets]
    end

    %% Data to Services
    PG --- IAM
    R --- IAM
    PG --- POL
    R --- POL
    PG --- DLP
    R --- DLP
    PG --- CR
    R --- CR
    PG --- COMP
    CH --- COMP
    S3 --- COMP
    R --- COMP
    M --- THREAT
    R --- THREAT
    M --- AUDIT
    R --- AUDIT
    M --- ALERT

    %% Services to Gateway
    IAM --- CP
    POL --- CP
    AUDIT --- CP
    IAM --- NX
    CP --- NX

    %% Gateway to Frontend
    CP --- REACT_BE
    NX --- ANG
    CP --- REACT

    %% Frontend to External
    ANG --- EXT
    REACT --- EXT

    %% External integrations
    ALERT --- SIEM
    WH --- WEBHOOK
```

---

## 4. Service Registry


| # | Service | Port | Data Stores | Produces (Kafka) | Consumes (Kafka) | Router |
|:-:|---------|:----:|-------------|------------------|------------------|--------|
| 1 | **IAM** | 8082 | pg, redis | auth.events, saga.orchestration, connector.events | saga.orchestration | chi/v5 |
| 2 | **Policy** | 8083 | pg, redis | policy.changes | — | chi/v5 |
| 3 | **DLP** | 8089 | pg, redis | dlp.dlq | control.plane.events | gorilla/mux |
| 4 | **Threat** | 8084 | mongo, redis | threat.alerts | auth.events, policy.changes, data.access | chi/v5 |
| 5 | **Alerting** | 8086 | mongo, redis | notifications.outbound, webhook.delivery | threat.alerts | gorilla/mux |
| 6 | **Audit** | 8085 | mongo, redis | audit.trail | auth.events, policy.changes, data.access, threat.alerts, connector.events, saga.orchestration, audit.trail | std ServeMux |
| 7 | **Compliance** | 8088 | pg, clickhouse, s3, redis | — | audit.trail | gorilla/mux |
| 8 | **Webhook-Delivery** | 8087 | pg | webhook.dlq | webhook.delivery | gorilla/mux |
| 9 | **Connector-Registry** | 8090 | pg, redis | — | — | chi/v5 |
| 10 | **Control-Plane** | 8081 | — | control.plane.events | — | chi/v5 |

---

## 5. Port Map


| Component | Internal Port | External Port | Protocol |
|-----------|:------------:|:-------------:|:--------:|
| Nginx Gateway | 8080 | 8080 | HTTP / HTTPS |
| Control-Plane | 8080 | 8081 | HTTPS (mTLS) |
| IAM | 8080 / 8443 | 8082 | HTTPS (mTLS) |
| Policy | 8080 | 8083 | HTTPS (mTLS) |
| Threat | 8080 | 8084 | HTTPS (mTLS) |
| Audit | 8080 | 8085 | HTTPS (mTLS) |
| Alerting | 8080 | 8086 | HTTPS (mTLS) |
| Webhook-Delivery | 8080 | 8087 | HTTPS (mTLS) |
| Compliance | 8080 | 8088 | HTTPS (mTLS) |
| DLP | 8080 | 8089 | HTTPS (mTLS) |
| Connector-Registry | 8080 | 8090 | HTTPS (mTLS) |
| Example App Backend | 3005 | 3005 | HTTP |
| Example App Frontend | 3000 | 3000 | HTTP |
| Angular Dashboard | 80 (nginx) | 4200 | HTTP |
| PostgreSQL | 5432 | 5432 | PG wire |
| Redis | 6379 | 6379 | RESP |
| Kafka | 9092 | 9092 | Kafka protocol |
| ZooKeeper | 2181 | 2181 | ZK protocol |
| MongoDB | 27017 | 27017 | MongoDB wire |
| ClickHouse HTTP | 8123 | 8123 | HTTP |
| ClickHouse Native | 9000 | 9000 | TCP |
| LocalStack (S3) | 4566 | 4566 | HTTP |
| Prometheus | 9090 | 9090 | HTTP |
| Grafana | 3010 | 3010 | HTTP |
| Jaeger UI | 16686 | 16686 | HTTP |
| Jaeger gRPC | 4317 | 4317 | gRPC |
| Loki | 3100 | 3100 | HTTP |

---

## 6. Communication Patterns


| Pattern | Protocol | Examples | Characteristics |
|---------|----------|---------|----------------|
| **Sync REST** | HTTP/mTLS | Gateway → Policy, Gateway → IAM | Request/response, circuit-broken, correlation IDs |
| **Async Events** | Kafka (Transactional Outbox) | IAM → `auth.events`, Policy → `policy.changes` | Exactly-once delivery, at-least-once consumption |
| **SSE (Server-Sent Events)** | HTTP/Streaming | Audit SSE → Dashboard | Real-time audit log streaming to frontend |
| **SDK Inline** | gRPC/HTTP | Example App → Policy (SDK) | Synchronous policy evaluation with 60s TTL caching |
| **SIEM Outbound** | HTTPS POST (HMAC-SHA256) | Alerting → Splunk/Datadog/Sentinel | Signed payloads with replay protection |

---

## 7. Key Architectural Patterns


| Pattern | Where | Description |
|---------|-------|-------------|
| **Transactional Outbox** | IAM, Policy | Write to `outbox_records` in same PG transaction → Relay polls/publishes to Kafka |
| **RLS (Row-Level Security)** | PostgreSQL (all services) | `app.current_org_id` session variable enforces multi-tenant isolation |
| **CQRS** | Audit (MongoDB) | Write-optimized primary + read-optimized secondary for audit logs |
| **Circuit Breaker** | Control-Plane → Policy/IAM | gobreaker-based, 30s open duration, 5-failure threshold |
| **Bulkhead** | Compliance, Alerting | Max 10 concurrent report generations, 50 concurrent alert processing |
| **Fail-Closed SDK** | Example App | 60s TTL cache; deny access if control plane unreachable |
| **mTLS** | All service-to-service | Mutual TLS for encrypted + authenticated communication |
| **Dead-Letter Queue** | Outbox, Webhook, DLP | Failed messages routed to DLQ topics after retry exhaustion |

---

## 8. Threat Detectors (Internal Detail)


| Detector | Consumes From | Trigger | Action |
|----------|---------------|---------|--------|
| BruteForceDetector | `auth.events` | >11 failed logins in window | Publish `threat.alerts` |
| ImpossibleTravelDetector | `auth.events` | Geo-velocity > threshold (GeoLite2) | Publish `threat.alerts` |
| OffHoursDetector | `auth.events` | Access outside 22:00–06:00 | Publish `threat.alerts` |
| DataExfiltrationDetector | `data.access` | Anomalous data volume/patterns | Publish `threat.alerts` |
| AccountTakeoverDetector | `auth.events` | Behavioral anomaly (device/IP/geo) | Publish `threat.alerts` |
| PrivilegeEscalationDetector | `auth.events` + `policy.changes` | Suspicious role/privilege changes | Publish `threat.alerts` |

---

> **Legend:** ═══ thick/arrow line = transactional (outbox) write / Kafka publish, ─── thin line = direct connection / REST, - - - dashed = indirect/integration / SDK, (()) = data store