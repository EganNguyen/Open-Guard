"""Generate the full SYSTEM_MAP.md from BLAST_RADIUS.json + architectural constants."""

import json
import os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, ".."))
BLAST_RADIUS_PATH = os.path.join(REPO_DIR, "docs/index/BLAST_RADIUS.json")
OUTPUT_PATH = os.path.join(REPO_DIR, "docs/index/SYSTEM_MAP.md")


def load_data():
    with open(BLAST_RADIUS_PATH) as f:
        return json.load(f)


def service_id(label):
    """Normalize a service label to a valid Mermaid node ID."""
    return label.replace("-", "_").replace(" ", "_").lower()


def generate_architecture_diagram(data):
    """Generate the L1 High-Level Architecture diagram."""
    services = data["services"]
    topics = data["event_flow"]

    groups = {}
    for svc in services:
        g = svc.get("group", "Other")
        groups.setdefault(g, []).append(svc)

    lines = [
        "```mermaid",
        "graph TD",
        "",
        "    %% External",
        '    subgraph External["External World"]',
        "        BROWSER[Browser / API Clients]",
        "        SIEM[SIEM / Splunk / Datadog]",
        "        WEBHOOK_TARGET[Webhook Targets]",
        "    end",
        "",
        '    subgraph Frontend["Frontend Layer"]',
        "        ANG[Angular Admin Dashboard :4200]",
        "        REACT[React Example App :3000]",
        "        REACT_BE[Example App Go Backend :3005]",
        "    end",
        "",
        '    subgraph Gateway["Gateway Layer"]',
        '        NX["Nginx Reverse Proxy :8080"]',
        '        CP["Control-Plane :8080/8081"]',
        "    end",
        "",
    ]

    # Service group subgraphs (skip control-plane — already in hardcoded Gateway subgraph)
    skip_services = {"control-plane"}
    for g, svcs in sorted(groups.items()):
        filtered = [s for s in svcs if s["id"] not in skip_services]
        if not filtered:
            continue
        gid = service_id(g)
        lines.append(f'    subgraph {gid}["{g}"]')
        for svc in filtered:
            sid = service_id(svc["id"])
            port_info = svc["port_ext"]
            lines.append(f'        {sid}["{svc["name"]} :{port_info}"]')
        lines.append("    end")
        lines.append("")

    # Data tier
    lines.extend([
        '    subgraph Data["Data Tier"]',
        '        PG[("PostgreSQL 15 :5432")]',
        '        R[("Redis 7 :6379")]',
        '        M[("MongoDB 6.0 :27017")]',
        '        CH[("ClickHouse 23.3 :8123")]',
        '        S3[("S3 / MinIO :4566")]',
        "    end",
        "",
        '    subgraph EventBus["Event Bus"]',
        '        K[("Kafka 7.4 :9092")]',
        "    end",
        "",
        '    subgraph Monitoring["Observability Stack"]',
        "        PROM[Prometheus :9090]",
        "        GRAF[Grafana :3010]",
        "        JAEG[Jaeger :16686]",
        "        LOKI[Loki :3100]",
        "    end",
        "",
        '    subgraph Shared["Shared Library (shared/)"]',
        '        SH_LIB["Crypto · Resilience · Middleware\\nDB · Telemetry · Kafka"]',
        "    end",
        "",
    ])

    # Edges: Frontend to Gateway
    lines.append("    %% Frontend to Gateway")
    lines.append("    BROWSER --> NX")
    lines.append("    ANG --> NX")
    lines.append("    REACT --> CP")
    lines.append("    REACT --> REACT_BE")
    lines.append("    REACT_BE -. SDK .-> policy")
    lines.append("")

    # Edges: Gateway to Services
    lines.append("    %% Gateway to Services")
    lines.append("    NX --> iam")
    lines.append("    NX --> CP")
    lines.append("    CP --> iam")
    lines.append("    CP --> policy")
    lines.append("    CP --> audit")
    lines.append("")

    # Edges: Services to Data Stores
    ds_map = {
        "pg": "PG", "redis": "R", "mongo": "M",
        "clickhouse": "CH", "s3": "S3",
    }
    lines.append("    %% Services to Data Stores")
    for svc in services:
        sid = service_id(svc["id"])
        for ds in svc.get("data_stores", []):
            ds_node = ds_map.get(ds)
            if ds_node:
                lines.append(f"    {sid} --> {ds_node}")
    lines.append("")

    # Transactional Outbox (services with outbox)
    outbox_services = ["iam", "policy"]
    lines.append("    %% Transactional Outbox")
    for s in outbox_services:
        lines.append(f"    {s} ==> PG")
    lines.append("")

    # Kafka producers (services that produce to Kafka)
    lines.append("    %% Kafka Producers")
    for svc in services:
        if svc.get("kafka_produces"):
            sid = service_id(svc["id"])
            lines.append(f"    {sid} --> K")
    lines.append("")

    # Kafka consumers (services that consume from Kafka)
    lines.append("    %% Kafka Consumers")
    seen_consumers = set()
    for topic, flow in topics.items():
        for c in flow.get("consumers", []):
            cid = service_id(c)
            if cid not in seen_consumers and c != "outbox-relay":
                seen_consumers.add(cid)
    for cid in sorted(seen_consumers):
        lines.append(f"    K --> {cid}")
    lines.append("")

    # External integrations
    lines.append("    %% External Integrations")
    lines.append('    alerting -. "SIEM Webhook (HMAC-SHA256)" .-> SIEM')
    lines.append('    webhook_delivery -. "Webhook Delivery\\n(retry + DLQ)" .-> WEBHOOK_TARGET')
    lines.append("")

    # Monitoring
    lines.append("    %% Monitoring")
    for svc in services:
        sid = service_id(svc["id"])
        lines.append(f"    {sid} --> PROM")
    lines.append("    PROM --> GRAF")
    for svc in services:
        sid = service_id(svc["id"])
        lines.append(f"    {sid} --> JAEG")
    lines.append("    LOKI --> GRAF")
    lines.append("")

    # Shared Library
    lines.append("    %% Shared Library")
    for svc in services:
        sid = service_id(svc["id"])
        lines.append(f"    {sid} -.-> SH_LIB")

    lines.append("")
    lines.append("```")
    return "\n".join(lines)


def generate_kafka_flow_diagram(data):
    """Generate the detailed Kafka Event Flow diagram."""
    topics = data["event_flow"]

    # Collect all unique producers and consumers
    all_producers = set()
    all_consumers = set()
    for topic, flow in topics.items():
        p = flow["producer"]
        if p and p != "outbox-relay":
            all_producers.add(p)
        for c in flow.get("consumers", []):
            if c and c != "outbox-relay":
                all_consumers.add(c)

    lines = [
        "```mermaid",
        "graph LR",
        "",
        "    subgraph Producers",
    ]
    for p in sorted(all_producers):
        pid = service_id(p)
        lines.append(f"        {pid}[{p}]")
    lines.append("    end")
    lines.append("")

    # Topic nodes
    lines.append('    subgraph Topics["Kafka Topics"]')
    for topic, flow in sorted(topics.items()):
        tid = service_id(topic)
        parts = flow.get("partitions", "")
        label = f"{topic} ({parts}p)" if parts else topic
        lines.append(f'        {tid}["{label}"]')
    lines.append("    end")
    lines.append("")

    # Consumer groups
    consumer_labels = {
        "threat": "Threat Detectors",
        "audit": "Audit Service",
        "alerting": "Alerting Saga",
        "compliance": "Compliance Service",
        "webhook-delivery": "Webhook-Delivery",
        "iam": "IAM Saga Consumer",
        "dlp": "DLP Consumer",
    }

    lines.append("    subgraph Consumers")
    for c in sorted(all_consumers):
        cid = service_id(c)
        label = consumer_labels.get(c, c)
        lines.append(f"        {cid}_c[{label}]")
    lines.append("    end")
    lines.append("")

    # Producers to Topics
    lines.append("    %% Producers to Topics")
    for topic, flow in sorted(topics.items()):
        p = flow["producer"]
        if p and p != "outbox-relay":
            pid = service_id(p)
            tid = service_id(topic)
            lines.append(f"    {pid} ==> {tid}")
    lines.append("")

    # Topics to Consumers
    lines.append("    %% Topics to Consumers")
    for topic, flow in sorted(topics.items()):
        tid = service_id(topic)
        for c in flow.get("consumers", []):
            if c and c != "outbox-relay":
                cid = service_id(c)
                lines.append(f"    {tid} --> {cid}_c")
    lines.append("")

    # DLQ failure annotation
    lines.append("    %% DLQ failure paths")
    lines.append('    outbox_dlq -. "Outbox relay failures" .-> F_DLQ[Dead-Letter Queue]')
    lines.append('    webhook_dlq -. "Webhook failures (3x backoff)" .-> F_DLQ')
    lines.append('    dlp_dlq -. "DLP scan failures (5 consecutive)" .-> F_DLQ')
    lines.append("")
    lines.append("    style F_DLQ fill:#ffcccc,stroke:#cc0000")
    lines.append("    style outbox_dlq fill:#ffe0e0")
    lines.append("    style webhook_dlq fill:#ffe0e0")
    lines.append("    style dlp_dlq fill:#ffe0e0")

    lines.append("")
    lines.append("```")
    return "\n".join(lines)


def generate_kafka_table(data):
    """Generate the Kafka topic reference table."""
    topics = data["event_flow"]
    rows = []
    for topic, flow in sorted(topics.items()):
        parts = flow.get("partitions", "")
        prod = flow["producer"] if flow["producer"] else "—"
        consumers = flow.get("consumers", [])
        cons_str = ", ".join(c for c in consumers) if consumers else "—"
        desc = flow.get("description", "")
        rows.append(f"| `{topic}` | {parts} | {prod} | {cons_str} | {desc} |")
    return "\n".join(rows)


def generate_service_table(data):
    """Generate the Service Registry table."""
    services = data["services"]
    rows = []
    for i, svc in enumerate(services, 1):
        sid = svc["id"]
        port = str(svc["port_ext"])
        ds = ", ".join(svc.get("data_stores", [])) or "—"
        produces = ", ".join(svc.get("kafka_produces", [])) or "—"
        consumes = ", ".join(svc.get("kafka_consumes", [])) or "—"
        router = svc.get("router", "")
        rows.append(
            f"| {i} | **{svc['name']}** | {port} | {ds} | {produces} | {consumes} | {router} |"
        )
    return "\n".join(rows)


def generate_layered_stack():
    """Static layered system stack diagram."""
    return """```mermaid
graph BT
    subgraph DataLayer["Data Tier"]
        PG[("PostgreSQL 15\\nPrimary Store · RLS")]
        R[("Redis 7\\nCache · Sessions · Rate-Limit")]
        M[("MongoDB 6.0\\nAudit · Alerts · Threats")]
        CH[("ClickHouse 23.3\\nCompliance Analytics")]
        S3[("S3 / MinIO\\nReport Artifacts")]
    end

    subgraph ServiceLayer["Service Layer (Go 1.22+ / Chi / Gorilla Mux)"]
        IAM[IAM :8080\\nIdentity · Auth · SCIM · SSO · MFA]
        POL[Policy :8080\\nPolicy Engine · RBAC · Evaluation]
        DLP[DLP :8080\\nContent Scanning · PII Detection]
        THREAT[Threat :8080\\nReal-Time Detection · 6 Detectors]
        ALERT[Alerting :8080\\nAlert Lifecycle · SIEM Delivery]
        AUDIT[Audit :8080\\nLog Ingestion · CQRS · SSE Stream]
        COMP[Compliance :8080\\nPosture · Reports · Aggregation]
        WH[Webhook-Delivery :8080\\nDispatch · Retry · DLQ]
        CR[Connector-Registry :8090\\n3rd-Party Connectors · SCIM Sync]
    end

    subgraph GatewayLayer["Gateway Layer"]
        CP[Control-Plane :8080\\nAPI Gateway · Proxy · Circuit Breaker]
        NX[Nginx :8080\\nReverse Proxy · TLS Termination]
    end

    subgraph FrontendLayer["Frontend Layer"]
        ANG[Angular 21 + Tailwind\\nAdmin Dashboard :4200]
        REACT[Next.js 14 + React 18\\nExample App :3000]
        REACT_BE[Go Chi Backend\\nExample App :3005]
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
```"""


def generate_port_map():
    """Static port map table."""
    return """| Component | Internal Port | External Port | Protocol |
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
| Loki | 3100 | 3100 | HTTP |"""


def generate_communication_patterns():
    """Static communication patterns table."""
    return """| Pattern | Protocol | Examples | Characteristics |
|---------|----------|---------|----------------|
| **Sync REST** | HTTP/mTLS | Gateway → Policy, Gateway → IAM | Request/response, circuit-broken, correlation IDs |
| **Async Events** | Kafka (Transactional Outbox) | IAM → `auth.events`, Policy → `policy.changes` | Exactly-once delivery, at-least-once consumption |
| **SSE (Server-Sent Events)** | HTTP/Streaming | Audit SSE → Dashboard | Real-time audit log streaming to frontend |
| **SDK Inline** | gRPC/HTTP | Example App → Policy (SDK) | Synchronous policy evaluation with 60s TTL caching |
| **SIEM Outbound** | HTTPS POST (HMAC-SHA256) | Alerting → Splunk/Datadog/Sentinel | Signed payloads with replay protection |"""


def generate_architectural_patterns():
    """Static key architectural patterns table."""
    return """| Pattern | Where | Description |
|---------|-------|-------------|
| **Transactional Outbox** | IAM, Policy | Write to `outbox_records` in same PG transaction → Relay polls/publishes to Kafka |
| **RLS (Row-Level Security)** | PostgreSQL (all services) | `app.current_org_id` session variable enforces multi-tenant isolation |
| **CQRS** | Audit (MongoDB) | Write-optimized primary + read-optimized secondary for audit logs |
| **Circuit Breaker** | Control-Plane → Policy/IAM | gobreaker-based, 30s open duration, 5-failure threshold |
| **Bulkhead** | Compliance, Alerting | Max 10 concurrent report generations, 50 concurrent alert processing |
| **Fail-Closed SDK** | Example App | 60s TTL cache; deny access if control plane unreachable |
| **mTLS** | All service-to-service | Mutual TLS for encrypted + authenticated communication |
| **Dead-Letter Queue** | Outbox, Webhook, DLP | Failed messages routed to DLQ topics after retry exhaustion |"""


def generate_detector_table():
    """Static threat detector reference."""
    return """| Detector | Consumes From | Trigger | Action |
|----------|---------------|---------|--------|
| BruteForceDetector | `auth.events` | >11 failed logins in window | Publish `threat.alerts` |
| ImpossibleTravelDetector | `auth.events` | Geo-velocity > threshold (GeoLite2) | Publish `threat.alerts` |
| OffHoursDetector | `auth.events` | Access outside 22:00–06:00 | Publish `threat.alerts` |
| DataExfiltrationDetector | `data.access` | Anomalous data volume/patterns | Publish `threat.alerts` |
| AccountTakeoverDetector | `auth.events` | Behavioral anomaly (device/IP/geo) | Publish `threat.alerts` |
| PrivilegeEscalationDetector | `auth.events` + `policy.changes` | Suspicious role/privilege changes | Publish `threat.alerts` |"""


def generate_full_system_map(data):
    """Assemble the complete SYSTEM_MAP.md."""
    sections = [
        "# System Topology Map\n",
        "## 1. High-Level Architecture\n",
        generate_architecture_diagram(data),
        "",
        "---\n",
        "## 2. Kafka Event Flow — Topic-Level Trace\n",
        "",
        "All event-driven communication follows the **Transactional Outbox** pattern: "
        "services write to a local PostgreSQL `outbox_records` table within the same "
        "transaction as business logic, then an Outbox Relay polls and publishes to "
        "Kafka. Consumers read asynchronously.\n",
        generate_kafka_flow_diagram(data),
        "",
        generate_kafka_table(data),
        "",
        "---\n",
        "## 3. Layered System Stack\n",
        "",
        generate_layered_stack(),
        "",
        "---\n",
        "## 4. Service Registry\n",
        "",
        "| # | Service | Port | Data Stores | Produces (Kafka) | Consumes (Kafka) | Router |",
        "|:-:|---------|:----:|-------------|------------------|------------------|--------|",
        generate_service_table(data),
        "",
        "---\n",
        "## 5. Port Map\n",
        "",
        generate_port_map(),
        "",
        "---\n",
        "## 6. Communication Patterns\n",
        "",
        generate_communication_patterns(),
        "",
        "---\n",
        "## 7. Key Architectural Patterns\n",
        "",
        generate_architectural_patterns(),
        "",
        "---\n",
        "## 8. Threat Detectors (Internal Detail)\n",
        "",
        generate_detector_table(),
        "",
        "---\n",
        "> **Legend:** ═══ thick/arrow line = transactional (outbox) write / Kafka publish, "
        "─── thin line = direct connection / REST, "
        "- - - dashed = indirect/integration / SDK, "
        "(()) = data store",
    ]
    return "\n".join(sections)


def main():
    data = load_data()
    output = generate_full_system_map(data)
    with open(OUTPUT_PATH, "w") as f:
        f.write(output)
    print(f"System Map generated at {OUTPUT_PATH} ({len(output.splitlines())} lines)")


if __name__ == "__main__":
    main()
