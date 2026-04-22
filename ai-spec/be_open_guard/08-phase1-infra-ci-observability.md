# §9 — Phase 1: Infra, CI/CD & Observability

---

## 9.1 Docker Compose

See `infra/docker/docker-compose.yml`. Services: postgres, mongo-primary, mongo-secondary-1, mongo-secondary-2, mongo-init, redis, zookeeper, kafka, clickhouse, jaeger, prometheus, grafana, and all application services (control-plane, connector-registry, iam, policy, threat, audit, alerting, webhook-delivery, compliance, dlp, web).

Key requirements:
- All services use `healthcheck` with `condition: service_healthy` in `depends_on`.
- MongoDB init service retries until primary is ready.
- All services load `env_file: [../../.env]`.

---

## 9.2 GitHub Actions CI

```yaml
# .github/workflows/ci.yml
jobs:
  go-test:        # go test ./... -race -coverprofile=coverage.out -timeout 5m; coverage gate 70% per package
  go-lint:        # golangci-lint
  sql-lint:       # go-sqllint — fails on string concatenation in SQL queries
  next-build:     # npm ci && npm run build && npm run lint
  contract-tests: # go test ./shared/... -run TestContract -v
  security-scan:  # govulncheck ./..., trivy --severity CRITICAL,HIGH, go mod verify
```

*Note: Phase 1 uses raw CLI commands explicitly (e.g. `go test`, `golangci-lint`); the uniform `Makefile` wrapper targets are an output deliverable of Phase 2.*

---

## 9.3 Prometheus Metrics

| Metric | Type | Labels |
|---|---|---|
| `openguard_outbox_pending_records` | Gauge | `service` |
| `openguard_outbox_relay_duration_seconds` | Histogram | `service`, `result` |
| `openguard_circuit_breaker_state` | Gauge | `name`, `state` (0=closed, 1=half-open, 2=open) |
| `openguard_rls_session_set_duration_seconds` | Histogram | `service` |
| `openguard_kafka_bulk_insert_size` | Histogram | `service` |
| `openguard_kafka_consumer_lag` | Gauge | `topic`, `group` |
| `openguard_kafka_offset_commit_duration_seconds` | Histogram | `topic`, `group` |
| `openguard_audit_chain_integrity_failures_total` | Counter | `org_id` |
| `openguard_policy_cache_hits_total` | Counter | `layer` (`sdk`\|`redis`) |
| `openguard_policy_cache_misses_total` | Counter | `layer` |
| `openguard_threat_detections_total` | Counter | `detector`, `severity` |
| `openguard_report_generation_duration_seconds` | Histogram | `type`, `format` |
| `openguard_report_bulkhead_rejected_total` | Counter | — |
| `openguard_connector_auth_total` | Counter | `result` |
| `openguard_events_ingested_total` | Counter | `connector_id` |
| `openguard_webhook_delivery_duration_seconds` | Histogram | `result` |
| `openguard_webhook_delivery_attempts_total` | Counter | `result` |
| `openguard_webhook_dlq_total` | Counter | — |
| `openguard_dlp_scan_duration_seconds` | Histogram | `mode` |
| `openguard_dlp_findings_total` | Counter | `type` |

---

## 9.4 Alertmanager Rules

```yaml
groups:
- name: openguard
  rules:
  - alert: OutboxLagHigh
    expr: openguard_outbox_pending_records > 1000
    for: 2m
    labels: { severity: warning }
    annotations:
      runbook: "docs/runbooks/outbox-dlq.md"

  - alert: CircuitBreakerOpen
    expr: openguard_circuit_breaker_state{state="2"} == 1
    for: 30s
    labels: { severity: critical }
    annotations:
      runbook: "docs/runbooks/circuit-breaker-open.md"

  - alert: KafkaConsumerLagHigh
    expr: openguard_kafka_consumer_lag > 50000
    for: 5m
    labels: { severity: warning }
    annotations:
      runbook: "docs/runbooks/kafka-consumer-lag.md"

  - alert: AuditChainIntegrityFailure
    expr: increase(openguard_audit_chain_integrity_failures_total[5m]) > 0
    labels: { severity: critical }
    annotations:
      runbook: "docs/runbooks/audit-hash-mismatch.md"

  - alert: PolicyServiceDown
    expr: up{job="policy"} == 0
    for: 30s
    labels: { severity: critical }

  - alert: KafkaOffsetCommitLag
    expr: histogram_quantile(0.99, openguard_kafka_offset_commit_duration_seconds_bucket) > 5
    for: 2m
    labels: { severity: warning }
```

---

## 9.5 Helm Chart

`infra/k8s/helm/openguard/` requirements:
- `Deployment` per service with `minReadySeconds: 30` and `RollingUpdate` strategy.
- `PodDisruptionBudget` per service: `minAvailable: 1`.
- `HorizontalPodAutoscaler` for `control-plane`, `iam`, `policy`, `audit`: CPU 70% and `openguard_kafka_consumer_lag`.
- `HorizontalPodAutoscaler` for `dlp`: minimum 3 replicas when any org has `dlp_mode=block`. CPU 80%.
- `NetworkPolicy`: internal services accept inbound only from `control-plane` (mTLS). IAM OIDC endpoints have a separate public `Ingress`.
- `terminationGracePeriodSeconds: 45` (see §19.3).
- `topologySpreadConstraints`: spread pods across 3 AZs.

---

## 9.6 Connected Apps Admin UI (`/connectors`)

**List view:** Table with name, status badge, scopes, created date, last event timestamp, event volume (30d). "Register app" button.

**Registration modal:** App name, Webhook URL, Scopes (multi-select). On success: API key displayed in one-time reveal panel with copy button. Warning: "This key will not be shown again."

**Detail page (`/connectors/:id`):** Metadata, edit webhook/scopes, webhook delivery log (last 100), event volume chart, "Send test webhook" button, danger zone.

---

## 9.7 Phase 1 Acceptance Criteria

- [ ] `docker compose up` starts all services healthy with MongoDB replica set initialized.
- [ ] MongoDB init service retries until primary is healthy.
- [ ] `go test ./... -race` passes in CI. Coverage gate enforced per package.
- [ ] `govulncheck ./...` reports no CRITICAL vulnerabilities.
- [ ] SQL lint catches string concatenation in a test file.
- [ ] Contract test: IAM `EventEnvelope` is parseable by audit consumer.
- [ ] All 11 services scraped by Prometheus. All `openguard_*` metrics appear in Grafana.
- [ ] `OutboxLagHigh` alert fires when relay is stopped for 2+ minutes.
- [ ] `CircuitBreakerOpen` alert fires when policy service is killed.
- [ ] `helm lint` and `helm template` pass without warnings.
- [ ] Connected app registration UI flow end-to-end.

---

## 9.8 Capacity Planning

- **PostgreSQL:** PgBouncer in transaction pooling mode. Min `max_connections=5000` via PgBouncer; Postgres at `max_connections=200`.
- **MongoDB:** 3-node Replica Set with NVMe SSD storage.
- **Backups:**
  - PostgreSQL: WAL-G or pgBackRest for continuous WAL archiving to S3 (PITR).
  - MongoDB: Percona Backup for MongoDB (PBM) for non-blocking snapshots + oplog archiving.
  - ClickHouse: `ALTER TABLE ... FREEZE` partitions daily for S3 export.
