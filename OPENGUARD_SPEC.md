# OpenGuard — Enterprise Implementation Specification v2.0

> **For the implementing LLM:** This is a complete, phase-gated specification for a true enterprise-scale security platform. Read the **entire document** before writing a single line of code. This version supersedes v1.0 and incorporates fixes for: the dual-write / Transactional Outbox problem, PostgreSQL Row-Level Security, circuit breakers, the Saga pattern for distributed operations, multi-tenancy isolation, read/write split (CQRS), secret rotation, mTLS, ClickHouse bulk-insert batching, load performance targets, and structured migration guarantees.
>
> **Non-negotiable rules:**
> - Every Kafka publish goes through the Outbox relay — never a direct producer call from a business handler.
> - Every table that holds org data has RLS enabled and enforced.
> - Every inter-service HTTP call wraps a circuit breaker.
> - Failure mode for the policy engine is **fail closed**: deny all access when unavailable.
> - No string concatenation in SQL — parameterized queries only, enforced by linter in CI.
> - All canonical names (env vars, topic names, table names, error codes) are fixed — do not rename.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Enterprise Architecture Principles](#2-enterprise-architecture-principles)
3. [Repository Layout](#3-repository-layout)
4. [Shared Contracts](#4-shared-contracts)
5. [Environment & Configuration](#5-environment--configuration)
6. [Multi-Tenancy Model](#6-multi-tenancy-model)
7. [Transactional Outbox Pattern](#7-transactional-outbox-pattern)
8. [Circuit Breakers & Resilience](#8-circuit-breakers--resilience)
9. [Phase 1 — Foundation (IAM + Gateway)](#9-phase-1--foundation-iam--gateway)
10. [Phase 2 — Policy Engine](#10-phase-2--policy-engine)
11. [Phase 3 — Event Bus, Outbox Relay & Audit Log](#11-phase-3--event-bus-outbox-relay--audit-log)
12. [Phase 4 — Threat Detection & Alerting](#12-phase-4--threat-detection--alerting)
13. [Phase 5 — Compliance & Analytics](#13-phase-5--compliance--analytics)
14. [Phase 6 — Frontend (Next.js)](#14-phase-6--frontend-nextjs)
15. [Phase 7 — Infra, CI/CD & Observability](#15-phase-7--infra-cicd--observability)
16. [Phase 8 — Security Hardening & Secret Rotation](#16-phase-8--security-hardening--secret-rotation)
17. [Phase 9 — Load Testing & Performance Tuning](#17-phase-9--load-testing--performance-tuning)
18. [Phase 10 — Documentation & Runbooks](#18-phase-10--documentation--runbooks)
19. [Cross-Cutting Concerns](#19-cross-cutting-concerns)
20. [Acceptance Criteria (Full System)](#20-acceptance-criteria-full-system)

---

## 1. Project Overview

### 1.1 What is OpenGuard?

OpenGuard is an open-source, self-hostable **organization security platform** inspired by Atlassian Guard. It is designed to operate at Fortune-500 scale: 100,000+ users, 10,000+ organizations, millions of audit events per day, with cryptographic audit trail integrity, zero cross-tenant data leakage, and sub-100ms policy evaluation at the 99th percentile.

Core capabilities:
- **Identity & Access Management (IAM):** SSO (SAML 2.0 / OIDC), SCIM 2.0 provisioning, TOTP/WebAuthn MFA, API token lifecycle, session management.
- **Policy Engine:** Real-time RBAC evaluation, data security rules, IP allowlists, session limits. Fails closed.
- **Threat Detection:** Streaming anomaly scoring — brute force, impossible travel, off-hours access, data exfiltration.
- **Audit Log:** Append-only, hash-chained, cryptographically verifiable event trail with configurable retention.
- **Alerting:** Rule-based + ML-scored alerts, SIEM webhook export, Slack/email delivery.
- **Compliance Reporting:** GDPR, SOC 2, HIPAA report generation with PDF output.
- **Admin Dashboard:** Next.js 14 web console.

### 1.2 Performance Targets (Canonical SLOs)

These are hard targets. Phase 9 must verify each one with k6 load tests. No phase is complete until its SLOs are met.

| Operation | p50 | p99 | p999 | Throughput |
|-----------|-----|-----|------|------------|
| `POST /auth/login` | 40ms | 150ms | 400ms | 2,000 req/s |
| `POST /policies/evaluate` | 5ms | 30ms | 80ms | 10,000 req/s |
| `GET /audit/events` (paginated) | 20ms | 100ms | 250ms | 1,000 req/s |
| Kafka event → audit DB insert | — | 2s | 5s | 50,000 events/s |
| Compliance report generation | — | 30s | 120s | 10 concurrent |
| Gateway JWT validation | 1ms | 5ms | 15ms | 50,000 req/s |

### 1.3 Design Principles

| Principle | Implication |
|-----------|-------------|
| Fail closed | Policy engine unavailable → deny all. IAM unavailable → reject all logins. Never fail open on security decisions. |
| Exactly-once audit | Every state-changing operation produces exactly one audit event. The Transactional Outbox guarantees this. |
| Zero cross-tenant leakage | PostgreSQL RLS enforced at the DB layer. Bug in application code cannot expose another org's data. |
| Immutable audit trail | Append-only MongoDB collection with hash chaining. Tampering is detectable. |
| Least privilege (services) | Each service has its own DB user with table-level grants. No service can read another service's tables. |
| Least privilege (tenants) | Tenant quotas enforced at the gateway. A noisy tenant cannot starve others. |
| Secret rotation without downtime | JWT signing uses key IDs (kid). Multiple valid keys coexist during rotation. |
| mTLS between services | All internal service-to-service calls use mTLS. A compromised service cannot impersonate another. |
| Structured migrations | golang-migrate with checksums. Down migrations required. Blue/green compatible (additive only in prod). |
| Observable by default | Every service emits traces, metrics, and structured logs from day one. No retrofitting. |

---

## 2. Enterprise Architecture Principles

### 2.1 The Dual-Write Problem and Why It Matters

The v1 spec had this pattern in every service handler:

```go
// WRONG — DO NOT DO THIS
db.Exec("INSERT INTO users ...")     // step 1
kafka.Publish("audit.trail", event) // step 2 — process crashes here = silent data loss
```

If the process crashes, OOM-kills, or loses network between step 1 and step 2:
- The user is created in PostgreSQL.
- No audit event is ever published.
- The audit log has a permanent gap.
- For a security platform, this is a compliance violation.

**The fix is the Transactional Outbox Pattern.** See Section 7 for the complete implementation contract.

### 2.2 Multi-Tenancy Isolation Levels

OpenGuard supports three isolation tiers selectable per organization plan:

| Tier | Mechanism | Use case |
|------|-----------|----------|
| **Shared** | PostgreSQL RLS on shared tables | SMB, free tier |
| **Schema** | Dedicated PostgreSQL schema per org | Mid-market |
| **Shard** | Dedicated PostgreSQL instance per org | Enterprise, regulated |

The spec implements **Shared** (RLS) fully and scaffolds **Schema** and **Shard** as extension points. All application code must be written to support RLS from day one — the schema and shard tiers slot in without changing handler logic.

### 2.3 CQRS and Read/Write Split

The audit log has asymmetric load: writes are high-throughput streaming (Kafka consumer); reads are ad-hoc queries from the dashboard and compliance exports.

MongoDB write path (Kafka consumer → primary):
- Consumer writes to the MongoDB **primary** only.
- Uses bulk insert with a buffer of up to 500 documents or 1 second, whichever comes first.

MongoDB read path (HTTP handlers → secondary):
- All `GET /audit/events` queries use `readPreference: secondaryPreferred`.
- Compliance report queries use `readPreference: secondary` (acceptable staleness: 5s).

This is enforced in the repository layer. See Section 11.

### 2.4 Saga Pattern for Distributed Operations

User provisioning via SCIM touches multiple services and must be atomic from the caller's perspective. OpenGuard uses **choreography-based sagas** via Kafka compensating events.

Example: SCIM `POST /scim/v2/Users`

```
IAM: user.created (org_id, user_id, scim_external_id) → audit.trail
Policy: consumes user.created → assigns default org policies → policy.assigned → audit.trail
Threat: consumes user.created → initializes baseline profile
Alerting: consumes user.created → configures notification preferences
```

If policy assignment fails, Policy publishes `policy.assignment.failed` with a `compensation: true` flag. IAM consumes this and sets the user to `status: provisioning_failed`, publishes `user.provisioning.failed` for the SCIM caller to poll.

Each saga step is idempotent. Consumer groups use `auto.offset.reset: earliest` so replays are safe.

### 2.5 Connection Pooling Targets

| Service | DB | Pool min | Pool max | Rationale |
|---------|----|----------|----------|-----------|
| IAM | PostgreSQL | 5 | 25 | Login burst |
| Policy | PostgreSQL | 2 | 15 | Short-lived evaluate queries |
| Audit (write) | MongoDB | 2 | 10 | Bulk inserts, low concurrency |
| Audit (read) | MongoDB | 5 | 30 | Dashboard queries |
| Compliance | ClickHouse | 2 | 8 | Long-running aggregations |
| All services | Redis | 5 | 20 | Rate limit + session |

These are configured via env vars (see Section 5) and enforced in the `db` package of each service.

---

## 3. Repository Layout

```
openguard/
├── .github/
│   └── workflows/
│       ├── ci.yml
│       ├── security.yml
│       └── release.yml
├── services/
│   ├── gateway/
│   ├── iam/
│   ├── policy/
│   ├── threat/
│   ├── audit/
│   ├── alerting/
│   └── compliance/
├── shared/                        # go module: github.com/openguard/shared
│   ├── go.mod
│   ├── kafka/
│   │   ├── producer.go
│   │   ├── consumer.go
│   │   ├── topics.go
│   │   └── outbox/
│   │       ├── relay.go           # Outbox → Kafka relay
│   │       └── poller.go
│   ├── middleware/
│   │   ├── auth.go
│   │   ├── tenant.go              # Sets app.org_id for RLS
│   │   ├── ratelimit.go
│   │   ├── circuitbreaker.go
│   │   ├── logger.go
│   │   └── mtls.go
│   ├── models/
│   │   ├── event.go
│   │   ├── user.go
│   │   ├── policy.go
│   │   ├── errors.go
│   │   ├── outbox.go
│   │   └── saga.go
│   ├── rls/
│   │   └── context.go             # Sets + reads app.org_id from Go context
│   ├── resilience/
│   │   ├── breaker.go             # Circuit breaker wrapper
│   │   ├── retry.go               # Exponential backoff with jitter
│   │   └── bulkhead.go            # Concurrency limiter
│   ├── telemetry/
│   │   ├── otel.go
│   │   ├── metrics.go
│   │   └── logger.go
│   ├── crypto/
│   │   ├── aes.go                 # AES-256-GCM with key versioning
│   │   ├── jwt.go                 # Multi-key JWT signing/verification
│   │   └── hash.go
│   └── validator/
│       └── validator.go
├── web/
│   ├── app/
│   ├── components/
│   ├── lib/
│   └── public/
├── proto/
│   ├── iam/v1/
│   ├── policy/v1/
│   ├── audit/v1/
│   └── threat/v1/
├── infra/
│   ├── docker/
│   │   ├── docker-compose.yml
│   │   └── docker-compose.dev.yml
│   ├── k8s/
│   │   ├── helm/
│   │   │   └── openguard/
│   │   └── kustomize/
│   │       ├── base/
│   │       ├── staging/
│   │       └── production/
│   ├── kafka/
│   │   └── topics.json
│   ├── certs/                     # mTLS certificate generation scripts
│   │   └── gen-certs.sh
│   └── monitoring/
│       ├── prometheus.yml
│       ├── alerts/
│       │   └── openguard.yml      # Alertmanager rules
│       └── grafana/
│           └── dashboards/
├── migrations/                    # Cross-service migration runner
│   └── runner.go
├── loadtest/                      # k6 load test scripts
│   ├── auth.js
│   ├── policy-evaluate.js
│   └── audit-query.js
├── docs/
│   ├── architecture.md
│   ├── runbooks/
│   │   ├── kafka-consumer-lag.md
│   │   ├── circuit-breaker-open.md
│   │   ├── audit-hash-mismatch.md
│   │   └── secret-rotation.md
│   ├── contributing.md
│   └── api/
├── scripts/
│   ├── create-topics.sh
│   ├── migrate.sh
│   ├── seed.sh
│   ├── gen-mtls-certs.sh
│   └── rotate-jwt-keys.sh
├── go.work
├── .env.example
├── Makefile
└── README.md
```

### 3.1 Go Workspace

```
go 1.22

use (
    ./shared
    ./services/gateway
    ./services/iam
    ./services/policy
    ./services/threat
    ./services/audit
    ./services/alerting
    ./services/compliance
)
```

### 3.2 Service Module Layout (canonical — every service follows this)

```
services/<name>/
├── go.mod                          # module: github.com/openguard/<name>
├── main.go                         # wires everything, starts server + graceful shutdown
├── Dockerfile
├── migrations/
│   ├── 001_<name>.up.sql
│   └── 001_<name>.down.sql         # Required for every up migration
├── pkg/
│   ├── config/
│   │   └── config.go               # env-var loading using shared pattern
│   ├── db/
│   │   ├── postgres.go             # pgxpool setup, RLS session var injection
│   │   ├── mongo.go                # separate read + write clients
│   │   └── migrations.go           # golang-migrate runner
│   ├── outbox/
│   │   └── writer.go               # writes to local outbox table (same TX as business data)
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

---

## 4. Shared Contracts

All types in this section live in `github.com/openguard/shared/models`. They are **immutable across phases** — rename requires a major version bump of the shared module and migration of all consumers.

### 4.1 Kafka Event Envelope

```go
package models

import (
    "encoding/json"
    "time"
)

// EventEnvelope is the wire format for every Kafka message on every topic.
// Consumers MUST validate SchemaVer before processing.
type EventEnvelope struct {
    ID         string          `json:"id"`          // UUIDv4, globally unique
    Type       string          `json:"type"`        // dot-separated, e.g. "auth.login.success"
    OrgID      string          `json:"org_id"`      // tenant identifier
    ActorID    string          `json:"actor_id"`    // user ID, service name, or "system"
    ActorType  string          `json:"actor_type"`  // "user" | "service" | "system"
    OccurredAt time.Time       `json:"occurred_at"` // event time, not processing time
    Source     string          `json:"source"`      // originating service: "iam", "policy", etc.
    TraceID    string          `json:"trace_id"`    // OpenTelemetry W3C trace ID
    SpanID     string          `json:"span_id"`     // OpenTelemetry span ID
    SchemaVer  string          `json:"schema_ver"`  // "1.0" — increment on breaking changes
    Idempotent string          `json:"idempotent"`  // dedup key for consumers
    Payload    json.RawMessage `json:"payload"`     // event-specific struct, JSON encoded
}
```

### 4.2 Outbox Record

```go
package models

import "time"

// OutboxRecord is persisted in the same transaction as the business operation.
// The relay process reads pending records and publishes to Kafka.
type OutboxRecord struct {
    ID          string    `db:"id"`           // UUIDv4
    Topic       string    `db:"topic"`        // Kafka topic name
    Key         string    `db:"key"`          // Kafka partition key (usually org_id)
    Payload     []byte    `db:"payload"`      // JSON-encoded EventEnvelope
    Status      string    `db:"status"`       // "pending" | "published" | "dead"
    Attempts    int       `db:"attempts"`     // number of publish attempts
    LastError   string    `db:"last_error"`   // last error message
    CreatedAt   time.Time `db:"created_at"`
    PublishedAt *time.Time `db:"published_at"`
    DeadAt      *time.Time `db:"dead_at"`
}
```

### 4.3 Saga Event

```go
package models

// SagaEvent wraps an EventEnvelope with saga orchestration metadata.
type SagaEvent struct {
    EventEnvelope
    SagaID       string `json:"saga_id"`              // UUIDv4, same across all steps
    SagaType     string `json:"saga_type"`            // "user.provision", "user.deprovision"
    SagaStep     int    `json:"saga_step"`            // 1-based step number
    Compensation bool   `json:"compensation"`         // true = this is a rollback event
    CausedBy     string `json:"caused_by,omitempty"` // event ID that caused this step
}
```

### 4.4 Kafka Topic Registry

```go
// shared/kafka/topics.go — canonical topic names, never hardcode strings
package kafka

const (
    TopicAuthEvents        = "auth.events"
    TopicPolicyChanges     = "policy.changes"
    TopicDataAccess        = "data.access"
    TopicThreatAlerts      = "threat.alerts"
    TopicAuditTrail        = "audit.trail"
    TopicNotificationsOut  = "notifications.outbound"
    TopicSagaOrchestration = "saga.orchestration"
    TopicOutboxDLQ         = "outbox.dlq"           // dead-letter for relay failures
)

// ConsumerGroups — canonical consumer group IDs
const (
    GroupAudit      = "openguard-audit-v1"
    GroupThreat     = "openguard-threat-v1"
    GroupAlerting   = "openguard-alerting-v1"
    GroupCompliance = "openguard-compliance-v1"
    GroupPolicy     = "openguard-policy-v1"
    GroupSaga       = "openguard-saga-v1"
)
```

### 4.5 Canonical User Model

```go
package models

import "time"

type User struct {
    ID              string     `json:"id" db:"id"`
    OrgID           string     `json:"org_id" db:"org_id"`
    Email           string     `json:"email" db:"email"`
    DisplayName     string     `json:"display_name" db:"display_name"`
    Status          UserStatus `json:"status" db:"status"`
    MFAEnabled      bool       `json:"mfa_enabled" db:"mfa_enabled"`
    MFAMethod       string     `json:"mfa_method,omitempty" db:"mfa_method"` // "totp" | "webauthn"
    SCIMExternalID  string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
    ProvisioningStatus string  `json:"provisioning_status" db:"provisioning_status"` // "complete" | "pending" | "failed"
    TierIsolation   string     `json:"tier_isolation" db:"tier_isolation"` // "shared" | "schema" | "shard"
    CreatedAt       time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
    DeletedAt       *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type UserStatus string

const (
    UserStatusActive             UserStatus = "active"
    UserStatusSuspended          UserStatus = "suspended"
    UserStatusDeprovisioned      UserStatus = "deprovisioned"
    UserStatusProvisioningFailed UserStatus = "provisioning_failed"
)
```

### 4.6 Standard HTTP Contracts

**Error response (all services):**
```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "User with id 'abc' not found",
    "request_id": "req_01j...",
    "trace_id": "4bf92f3577b34da6...",
    "retryable": false
  }
}
```

```go
package models

type APIError struct {
    Error APIErrorBody `json:"error"`
}

type APIErrorBody struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    RequestID string `json:"request_id"`
    TraceID   string `json:"trace_id"`
    Retryable bool   `json:"retryable"` // clients use this to decide whether to retry
}
```

**Pagination envelope (all list endpoints):**
```json
{
  "data": [],
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 1024,
    "total_pages": 21,
    "next_cursor": "eyJpZCI6IjEyMyJ9"
  }
}
```

Cursor-based pagination is used for audit log and threat alert endpoints (high volume). Page-number pagination is acceptable for user and policy lists.

---

## 5. Environment & Configuration

### 5.1 `.env.example` (canonical — every variable required)

```dotenv
# ── App ──────────────────────────────────────────────────────────────
APP_ENV=development                   # development | staging | production
LOG_LEVEL=info                        # debug | info | warn | error
LOG_FORMAT=json                       # json | text (use json in non-dev)

# ── Gateway ──────────────────────────────────────────────────────────
GATEWAY_PORT=8080
GATEWAY_JWT_KEYS_JSON=[{"kid":"k1","secret":"change-me","algorithm":"HS256","status":"active"}]
# JWT_KEYS_JSON is an array — supports multiple keys for rotation.
# "status": "active" = sign + verify. "status": "verify_only" = verify only (rotation window).
GATEWAY_JWT_EXPIRY_SECONDS=3600
GATEWAY_REFRESH_TOKEN_EXPIRY_DAYS=30
GATEWAY_RATE_LIMIT_ANON=300           # req/min per IP (unauthenticated)
GATEWAY_RATE_LIMIT_AUTHED=1000        # req/min per user ID (authenticated)
GATEWAY_TENANT_QUOTA_RPM=5000         # req/min per org_id (all users combined)
GATEWAY_MTLS_CERT_FILE=/certs/gateway.crt
GATEWAY_MTLS_KEY_FILE=/certs/gateway.key
GATEWAY_MTLS_CA_FILE=/certs/ca.crt

# ── IAM Service ──────────────────────────────────────────────────────
IAM_PORT=8081
IAM_SAML_ENTITY_ID=https://openguard.example.com
IAM_SAML_IDP_METADATA_URL=https://idp.example.com/metadata
IAM_OIDC_ISSUER=https://accounts.example.com
IAM_OIDC_CLIENT_ID=openguard
IAM_OIDC_CLIENT_SECRET=change-me
IAM_SCIM_BEARER_TOKEN=change-me
IAM_MFA_TOTP_ISSUER=OpenGuard
IAM_MFA_ENCRYPTION_KEY_JSON=[{"kid":"mk1","key":"base64-encoded-32-bytes","status":"active"}]
# Encryption keys follow the same rotation pattern as JWT keys.
IAM_WEBAUTHN_RPID=openguard.example.com
IAM_WEBAUTHN_RPORIGIN=https://openguard.example.com
IAM_MTLS_CERT_FILE=/certs/iam.crt
IAM_MTLS_KEY_FILE=/certs/iam.key
IAM_MTLS_CA_FILE=/certs/ca.crt

# ── Policy Service ───────────────────────────────────────────────────
POLICY_PORT=8082
POLICY_CACHE_TTL_SECONDS=30           # Redis cache TTL for evaluated policies
POLICY_MTLS_CERT_FILE=/certs/policy.crt
POLICY_MTLS_KEY_FILE=/certs/policy.key
POLICY_MTLS_CA_FILE=/certs/ca.crt

# ── Threat Detection ─────────────────────────────────────────────────
THREAT_PORT=8083
THREAT_ANOMALY_WINDOW_MINUTES=60
THREAT_MAX_FAILED_LOGINS=10
THREAT_GEO_CHANGE_THRESHOLD_KM=500
THREAT_MAXMIND_DB_PATH=/data/GeoLite2-City.mmdb
THREAT_MTLS_CERT_FILE=/certs/threat.crt
THREAT_MTLS_KEY_FILE=/certs/threat.key
THREAT_MTLS_CA_FILE=/certs/ca.crt

# ── Audit Service ────────────────────────────────────────────────────
AUDIT_PORT=8084
AUDIT_RETENTION_DAYS=730
AUDIT_HASH_CHAIN_SECRET=change-me     # HMAC secret for audit chain integrity
AUDIT_BULK_INSERT_MAX_DOCS=500        # Max documents per bulk insert
AUDIT_BULK_INSERT_FLUSH_MS=1000       # Max ms before forced flush
AUDIT_MTLS_CERT_FILE=/certs/audit.crt
AUDIT_MTLS_KEY_FILE=/certs/audit.key
AUDIT_MTLS_CA_FILE=/certs/ca.crt

# ── Alerting Service ─────────────────────────────────────────────────
ALERTING_PORT=8085
ALERTING_SLACK_WEBHOOK_URL=
ALERTING_SMTP_HOST=smtp.example.com
ALERTING_SMTP_PORT=587
ALERTING_SMTP_USER=
ALERTING_SMTP_PASS=
ALERTING_SIEM_WEBHOOK_URL=
ALERTING_SIEM_WEBHOOK_HMAC_SECRET=change-me  # HMAC-SHA256 signature on SIEM payloads
ALERTING_MTLS_CERT_FILE=/certs/alerting.crt
ALERTING_MTLS_KEY_FILE=/certs/alerting.key
ALERTING_MTLS_CA_FILE=/certs/ca.crt

# ── Compliance Service ───────────────────────────────────────────────
COMPLIANCE_PORT=8086
COMPLIANCE_REPORT_MAX_CONCURRENT=10
COMPLIANCE_MTLS_CERT_FILE=/certs/compliance.crt
COMPLIANCE_MTLS_KEY_FILE=/certs/compliance.key
COMPLIANCE_MTLS_CA_FILE=/certs/ca.crt

# ── PostgreSQL ───────────────────────────────────────────────────────
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=openguard_app           # application user — limited grants
POSTGRES_PASSWORD=change-me
POSTGRES_DB=openguard
POSTGRES_SSLMODE=verify-full          # never "disable" in staging/production
POSTGRES_SSLROOTCERT=/certs/postgres-ca.crt
POSTGRES_POOL_MIN_CONNS=5
POSTGRES_POOL_MAX_CONNS=25
POSTGRES_POOL_MAX_CONN_IDLE_SECS=300
POSTGRES_POOL_MAX_CONN_LIFETIME_SECS=3600
# Outbox relay uses a dedicated superuser-equivalent for LISTEN/NOTIFY
POSTGRES_OUTBOX_USER=openguard_outbox
POSTGRES_OUTBOX_PASSWORD=change-me

# ── MongoDB ──────────────────────────────────────────────────────────
MONGO_URI_PRIMARY=mongodb://localhost:27017        # write path
MONGO_URI_SECONDARY=mongodb://localhost:27018      # read path
MONGO_DB=openguard
MONGO_AUTH_SOURCE=admin
MONGO_TLS_CA_FILE=/certs/mongo-ca.crt
MONGO_WRITE_POOL_MIN=2
MONGO_WRITE_POOL_MAX=10
MONGO_READ_POOL_MIN=5
MONGO_READ_POOL_MAX=30

# ── Redis ────────────────────────────────────────────────────────────
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=change-me
REDIS_DB=0
REDIS_TLS_CERT_FILE=/certs/redis.crt
REDIS_POOL_SIZE=20
REDIS_MIN_IDLE_CONNS=5

# ── Kafka ────────────────────────────────────────────────────────────
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=openguard
KAFKA_TLS_CA_FILE=/certs/kafka-ca.crt
KAFKA_SASL_MECHANISM=SCRAM-SHA-512
KAFKA_SASL_USER=openguard
KAFKA_SASL_PASSWORD=change-me
KAFKA_PRODUCER_MAX_MESSAGE_BYTES=1048576   # 1MB
KAFKA_CONSUMER_SESSION_TIMEOUT_MS=45000
KAFKA_CONSUMER_HEARTBEAT_MS=3000
KAFKA_CONSUMER_MAX_POLL_RECORDS=500

# ── ClickHouse ───────────────────────────────────────────────────────
CLICKHOUSE_ADDR=localhost:9000
CLICKHOUSE_USER=openguard
CLICKHOUSE_PASSWORD=change-me
CLICKHOUSE_DB=openguard
CLICKHOUSE_TLS_CA_FILE=/certs/clickhouse-ca.crt
CLICKHOUSE_BULK_FLUSH_ROWS=5000
CLICKHOUSE_BULK_FLUSH_MS=2000

# ── Circuit Breakers ─────────────────────────────────────────────────
CB_POLICY_TIMEOUT_MS=50             # policy evaluate request timeout
CB_POLICY_FAILURE_THRESHOLD=5       # failures before opening
CB_POLICY_OPEN_DURATION_MS=10000    # ms before moving to half-open
CB_IAM_TIMEOUT_MS=200
CB_IAM_FAILURE_THRESHOLD=5
CB_IAM_OPEN_DURATION_MS=15000

# ── OpenTelemetry ────────────────────────────────────────────────────
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SERVICE_NAMESPACE=openguard
OTEL_SAMPLING_RATE=0.1              # 10% in production, 1.0 in development

# ── Frontend (Next.js) ───────────────────────────────────────────────
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me
```

### 5.2 Config Loading Pattern (shared, implement once)

```go
// shared/config/config.go
package config

import (
    "encoding/json"
    "fmt"
    "os"
    "strconv"
    "time"
)

func Must(key string) string {
    v := os.Getenv(key)
    if v == "" {
        panic(fmt.Sprintf("required env var %q not set", key))
    }
    return v
}

func Default(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func MustInt(key string) int {
    v := Must(key)
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
    }
    return n
}

func DefaultInt(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
    }
    return n
}

func MustDuration(key string) time.Duration {
    return time.Duration(MustInt(key)) * time.Millisecond
}

// MustJSON parses a JSON env var into dest.
func MustJSON(key string, dest any) {
    v := Must(key)
    if err := json.Unmarshal([]byte(v), dest); err != nil {
        panic(fmt.Sprintf("env var %q is not valid JSON: %v", key, err))
    }
}
```

---

## 6. Multi-Tenancy Model

### 6.1 PostgreSQL Row-Level Security

Every table that stores tenant data **must** have RLS enabled. This is enforced by the migration runner — it refuses to apply any migration that creates a new table with an `org_id` column without also enabling RLS.

#### 6.1.1 Application DB User

The application uses a dedicated PostgreSQL user with no superuser access:

```sql
-- Run once, not in migrations
CREATE ROLE openguard_app LOGIN PASSWORD 'change-me';
GRANT CONNECT ON DATABASE openguard TO openguard_app;
-- Tables are granted individually per migration (see below)
```

#### 6.1.2 RLS Setup (apply to every org-scoped table)

```sql
-- Example: users table
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY; -- applies to table owner too

CREATE POLICY users_org_isolation ON users
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);

-- org_id can be empty for system-level queries (e.g. SCIM provisioning before org is set)
-- The 'true' flag makes current_setting return NULL instead of error when not set
-- A NULL org_id means no rows match — fail safe
```

Apply this pattern to: `users`, `api_tokens`, `sessions`, `mfa_configs`, `policies`, `policy_assignments`, `outbox_records` (IAM), `outbox_records` (Policy).

#### 6.1.3 Setting the Tenant Context

The `shared/rls` package manages the tenant context:

```go
// shared/rls/context.go
package rls

import (
    "context"
    "fmt"
    "github.com/jackc/pgx/v5/pgxpool"
)

type contextKey struct{}

// WithOrgID stores the org ID in the Go context.
func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

// OrgID retrieves the org ID from context. Returns "" if not set.
func OrgID(ctx context.Context) string {
    v, _ := ctx.Value(contextKey{}).(string)
    return v
}

// SetSessionVar sets the PostgreSQL session variable for RLS.
// Must be called before every query on a pooled connection.
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    if orgID == "" {
        // Unset the variable — this results in no rows for RLS-protected tables
        _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
        return err
    }
    _, err := conn.Exec(ctx, fmt.Sprintf(
        "SELECT set_config('app.org_id', '%s', false)", orgID,
    ))
    return err
}
```

Every repository method that executes a PostgreSQL query must:
1. Acquire a connection from the pool.
2. Call `rls.SetSessionVar(ctx, conn, rls.OrgID(ctx))`.
3. Execute the query.
4. Release the connection.

```go
// Example repository method pattern
func (r *UserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
    conn, err := r.pool.Acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquire connection: %w", err)
    }
    defer conn.Release()

    if err := rls.SetSessionVar(ctx, conn, rls.OrgID(ctx)); err != nil {
        return nil, fmt.Errorf("set rls context: %w", err)
    }

    var u models.User
    err = conn.QueryRow(ctx,
        `SELECT id, org_id, email, display_name, status, mfa_enabled,
                scim_external_id, provisioning_status, created_at, updated_at, deleted_at
         FROM users WHERE id = $1 AND deleted_at IS NULL`,
        id,
    ).Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Status,
           &u.MFAEnabled, &u.SCIMExternalID, &u.ProvisioningStatus,
           &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt)
    if err != nil {
        return nil, fmt.Errorf("query user: %w", err)
    }
    return &u, nil
}
```

#### 6.1.4 Tenant Middleware (HTTP)

The `shared/middleware/tenant.go` middleware reads `X-Org-ID` (injected by the gateway after JWT validation) and sets it in the Go context:

```go
func TenantMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        orgID := r.Header.Get("X-Org-ID")
        if orgID == "" {
            // This should never happen if gateway is doing its job
            // Fail closed: return 401
            writeError(w, http.StatusUnauthorized, "MISSING_TENANT", "org context required", r)
            return
        }
        ctx := rls.WithOrgID(r.Context(), orgID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### 6.2 Per-Tenant Quotas

The gateway enforces three rate limit tiers using Redis sliding window:

```go
// shared/middleware/ratelimit.go

// Three limit keys per request:
// 1. IP-based (anonymous): key = "rl:ip:{ip}"
// 2. User-based (authenticated): key = "rl:user:{user_id}"
// 3. Tenant-based (org aggregate): key = "rl:org:{org_id}"

// All three are checked. The most restrictive applies.
// Tenant quota prevents a single org's users from consuming all capacity.
```

Limits are configurable via `GATEWAY_RATE_LIMIT_ANON`, `GATEWAY_RATE_LIMIT_AUTHED`, `GATEWAY_TENANT_QUOTA_RPM`.

On quota exceeded: return `429` with body:
```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Request rate limit exceeded",
    "retryable": true,
    "request_id": "...",
    "trace_id": "..."
  }
}
```
And headers: `Retry-After: <seconds>`, `X-RateLimit-Limit: <limit>`, `X-RateLimit-Remaining: 0`.

---

## 7. Transactional Outbox Pattern

This section is the most critical in the document. Every service that publishes Kafka events must implement the Outbox pattern. No exceptions.

### 7.1 Outbox Table (add to every service that publishes events)

```sql
-- Add to each service's PostgreSQL migrations
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,          -- Kafka partition key
    payload      BYTEA NOT NULL,         -- JSON-encoded EventEnvelope
    status       TEXT NOT NULL DEFAULT 'pending', -- pending | published | dead
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

-- RLS on outbox
ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;
CREATE POLICY outbox_org_isolation ON outbox_records
    USING (key = current_setting('app.org_id', true));

CREATE INDEX idx_outbox_status_created ON outbox_records(status, created_at)
    WHERE status = 'pending';

-- NOTIFY trigger so relay wakes up immediately instead of polling
CREATE OR REPLACE FUNCTION notify_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('outbox_new', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
    AFTER INSERT ON outbox_records
    FOR EACH ROW EXECUTE FUNCTION notify_outbox();
```

### 7.2 Outbox Writer

```go
// shared/kafka/outbox/writer.go
package outbox

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/jackc/pgx/v5"
    "github.com/openguard/shared/models"
)

// Writer writes events to the outbox table within the caller's transaction.
type Writer struct{}

// Write inserts an EventEnvelope into the outbox within the provided transaction.
// The transaction must already have the RLS session variable set.
func (w *Writer) Write(ctx context.Context, tx pgx.Tx, topic, key string, envelope models.EventEnvelope) error {
    payload, err := json.Marshal(envelope)
    if err != nil {
        return fmt.Errorf("marshal envelope: %w", err)
    }
    _, err = tx.Exec(ctx,
        `INSERT INTO outbox_records (topic, key, payload)
         VALUES ($1, $2, $3)`,
        topic, key, payload,
    )
    return err
}
```

### 7.3 Outbox Relay

```go
// shared/kafka/outbox/relay.go
package outbox

// Relay reads pending outbox records and publishes them to Kafka.
// It uses PostgreSQL LISTEN/NOTIFY to wake up immediately on new records,
// and falls back to polling every 100ms to handle missed notifications.
//
// Guarantees:
// - At-least-once delivery to Kafka (Kafka's idempotent producer handles dedup)
// - Records are marked "published" only after Kafka ack
// - Records that fail 5 times are marked "dead" and sent to outbox.dlq
// - The relay is safe to run as multiple instances (row-level locking via FOR UPDATE SKIP LOCKED)

type Relay struct {
    pool     *pgxpool.Pool
    producer kafka.Producer
    logger   *slog.Logger
}

func (r *Relay) Run(ctx context.Context) error {
    // 1. Start LISTEN on "outbox_new" channel
    // 2. On notification OR 100ms tick, call processBatch
    // 3. processBatch: SELECT ... FOR UPDATE SKIP LOCKED LIMIT 100 WHERE status='pending'
    // 4. For each record: produce to Kafka with idempotent producer
    // 5. On success: UPDATE status='published', published_at=NOW()
    // 6. On failure: UPDATE attempts=attempts+1, last_error=...
    //    If attempts >= 5: UPDATE status='dead', dead_at=NOW(), publish to outbox.dlq
    // 7. All updates in a single transaction per batch
}

// processBatch implements the core relay loop.
// Must use FOR UPDATE SKIP LOCKED to prevent multiple relay instances from double-publishing.
func (r *Relay) processBatch(ctx context.Context) (int, error) {
    tx, err := r.pool.Begin(ctx)
    if err != nil {
        return 0, err
    }
    defer tx.Rollback(ctx)

    rows, err := tx.Query(ctx, `
        SELECT id, topic, key, payload, attempts
        FROM outbox_records
        WHERE status = 'pending'
        ORDER BY created_at
        LIMIT 100
        FOR UPDATE SKIP LOCKED
    `)
    // ... publish each, update status, commit
}
```

### 7.4 Business Handler Pattern (with Outbox)

This is the canonical pattern every handler must follow:

```go
// Canonical write handler pattern — do NOT deviate from this
func (s *UserService) CreateUser(ctx context.Context, input CreateUserInput) (*models.User, error) {
    // 1. Acquire connection
    conn, err := s.pool.Acquire(ctx)
    if err != nil {
        return nil, fmt.Errorf("acquire conn: %w", err)
    }
    defer conn.Release()

    // 2. Begin transaction
    tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return nil, fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx) // no-op if committed

    // 3. Set RLS context within the transaction
    if _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", rls.OrgID(ctx)); err != nil {
        return nil, fmt.Errorf("set rls: %w", err)
    }

    // 4. Business operation — write to users table
    user, err := s.repo.CreateUserTx(ctx, tx, input)
    if err != nil {
        return nil, fmt.Errorf("create user: %w", err)
    }

    // 5. Write to outbox IN THE SAME TRANSACTION
    envelope := buildUserCreatedEnvelope(ctx, user)
    if err := s.outboxWriter.Write(ctx, tx, kafka.TopicAuditTrail, user.OrgID, envelope); err != nil {
        return nil, fmt.Errorf("write outbox: %w", err)
    }

    // 6. Commit — both the user row and the outbox record are committed atomically
    if err := tx.Commit(ctx); err != nil {
        return nil, fmt.Errorf("commit: %w", err)
    }

    // 7. The relay publishes the outbox record to Kafka asynchronously
    // There is NO direct Kafka.Publish() call here
    return user, nil
}
```

---

## 8. Circuit Breakers & Resilience

### 8.1 Circuit Breaker Implementation

Use `github.com/sony/gobreaker` wrapped in `shared/resilience/breaker.go`:

```go
// shared/resilience/breaker.go
package resilience

import (
    "context"
    "fmt"
    "time"
    "github.com/sony/gobreaker"
    "github.com/openguard/shared/models"
)

type BreakerConfig struct {
    Name            string
    Timeout         time.Duration // request timeout
    MaxRequests     uint32        // max requests in half-open state
    Interval        time.Duration // stat collection window
    FailureThreshold uint32       // failures before opening
    OpenDuration    time.Duration // time before moving to half-open
}

// NewBreaker creates a circuit breaker with standard OpenGuard defaults.
func NewBreaker(cfg BreakerConfig) *gobreaker.CircuitBreaker {
    return gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        cfg.Name,
        MaxRequests: cfg.MaxRequests,
        Interval:    cfg.Interval,
        Timeout:     cfg.OpenDuration,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= cfg.FailureThreshold
        },
        OnStateChange: func(name string, from, to gobreaker.State) {
            // Emit metric: openguard_circuit_breaker_state_change{name, from, to}
            // Emit structured log: level=warn msg="circuit breaker state changed"
        },
    })
}

// Call executes fn through the circuit breaker with a context timeout.
// Returns models.ErrCircuitOpen if the breaker is open.
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    result, err := cb.Execute(func() (any, error) {
        return fn(ctx)
    })
    if err != nil {
        var zero T
        if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
            return zero, fmt.Errorf("%w: %s", models.ErrCircuitOpen, cb.Name())
        }
        return zero, err
    }
    return result.(T), nil
}
```

### 8.2 Failure Modes (Canonical)

These failure mode decisions are non-negotiable for a security platform:

| Scenario | Required behaviour | Rationale |
|----------|--------------------|-----------|
| Policy service unreachable | **Deny all** — return `403 SERVICE_UNAVAILABLE` | Never grant access when you cannot evaluate policy |
| IAM service unreachable | **Reject all logins** — return `503` | Cannot authenticate without IAM |
| Audit service unreachable | **Continue operation, buffer via Outbox** — events will publish when audit recovers | Audit is observability, not a gate |
| Threat detection unreachable | **Continue operation, log warning** | Threat is advisory, not a gate |
| Redis unreachable | **Rate limiting fails open** — allow requests, log error | Availability over rate limiting when Redis is down |
| Kafka unreachable | **Outbox buffers events in PostgreSQL** — writes succeed, events queue | Kafka is not in the write path |
| ClickHouse unreachable | **Compliance reports fail with 503** — no writes blocked | Analytics is read-only |

### 8.3 Retry Policy

```go
// shared/resilience/retry.go
package resilience

// RetryConfig defines exponential backoff with full jitter.
type RetryConfig struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    Retryable   func(error) bool // returns true if the error warrants a retry
}

// DefaultRetry is the standard retry config for idempotent operations.
var DefaultRetry = RetryConfig{
    MaxAttempts: 5,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    10 * time.Second,
    Retryable: func(err error) bool {
        // Retry on network errors, 429, 503
        // Do NOT retry on 400, 401, 403, 404, 409
        return errors.Is(err, models.ErrRetryable)
    },
}

// Do executes fn with retries according to cfg.
// Uses exponential backoff with full jitter: sleep = rand(0, min(MaxDelay, BaseDelay * 2^attempt))
func Do(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error
```

### 8.4 Bulkhead (Concurrency Limiter)

Compliance report generation and audit CSV exports are expensive. Limit concurrency to prevent these from consuming all goroutines:

```go
// shared/resilience/bulkhead.go
package resilience

// Bulkhead limits concurrent executions of a function.
type Bulkhead struct {
    sem chan struct{}
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
    return &Bulkhead{sem: make(chan struct{}, maxConcurrent)}
}

func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
    select {
    case b.sem <- struct{}{}:
        defer func() { <-b.sem }()
        return fn()
    case <-ctx.Done():
        return fmt.Errorf("%w: bulkhead full", models.ErrBulkheadFull)
    }
}
```

---

## 9. Phase 1 — Foundation (IAM + Gateway)

**Goal:** Running skeleton with enterprise-grade auth. JWT multi-key rotation, RLS enforced, Outbox in place, circuit breakers configured. At the end of Phase 1, a user can register, login, and receive a JWT. Every write publishes via the Outbox, not directly to Kafka.

### 9.1 Prerequisites (produce before any service code)

1. `infra/docker/docker-compose.yml` — PostgreSQL 16, MongoDB 7 (primary + secondary replica), Redis 7, Kafka 3.6 + Zookeeper, ClickHouse 24, Jaeger, Prometheus, Grafana.
2. `scripts/gen-mtls-certs.sh` — generates a CA and per-service client certificates using `openssl`. Outputs to `infra/certs/`.
3. `scripts/create-topics.sh` — idempotent topic creation from `infra/kafka/topics.json`.
4. `Makefile` with targets: `dev`, `test`, `lint`, `build`, `migrate`, `seed`, `load-test`, `certs`.
5. `.env.example` as defined in Section 5.1.

### 9.2 Migration Strategy

Use `golang-migrate/migrate` with these rules:

- Every `.up.sql` must have a corresponding `.down.sql`.
- Migrations are **additive only** in production: add columns (nullable), add indexes, add tables. Never drop or rename in the same migration as adding.
- Every migration that creates a table with `org_id` must include the RLS setup for that table.
- The migration runner verifies checksums — it will refuse to apply a modified historical migration.
- Migration runner at service startup (not as a separate job) — use `migrate.Up()` on startup with a distributed lock (Redis `SET NX` with 30s TTL) to prevent concurrent runs in multi-replica deployments.

```go
// pkg/db/migrations.go (in each service)
func RunMigrations(ctx context.Context, dsn string, redisClient *redis.Client) error {
    // 1. Acquire distributed lock: "migrate-lock:<service-name>"
    // 2. Run golang-migrate Up()
    // 3. Release lock
    // Lock timeout: 120s (long enough for large migrations)
}
```

### 9.3 API Gateway

#### 9.3.1 JWT Multi-Key Rotation

JWT keys are stored as a JSON array in `GATEWAY_JWT_KEYS_JSON`. Each key has:
- `kid` — key identifier, included in JWT header.
- `secret` — the signing secret.
- `algorithm` — `HS256` | `RS256`.
- `status` — `active` (sign + verify) | `verify_only` (verify old tokens during rotation window).

```go
// shared/crypto/jwt.go
package crypto

type JWTKey struct {
    Kid       string `json:"kid"`
    Secret    string `json:"secret"`
    Algorithm string `json:"algorithm"`
    Status    string `json:"status"` // "active" | "verify_only"
}

type JWTKeyring struct {
    keys []JWTKey
}

// Sign uses the first key with status="active".
func (k *JWTKeyring) Sign(claims jwt.Claims) (string, error)

// Verify tries all keys, matching on kid from the token header.
// Returns ErrTokenExpired, ErrTokenInvalid, or nil.
func (k *JWTKeyring) Verify(tokenString string) (jwt.MapClaims, error)
```

**Key rotation procedure** (documented in `docs/runbooks/secret-rotation.md`):
1. Generate new key, add to `GATEWAY_JWT_KEYS_JSON` with `status: "active"`. Set old key to `status: "verify_only"`.
2. Deploy — new tokens are signed with the new key. Old tokens still verify.
3. Wait for `JWT_EXPIRY_SECONDS` — all old tokens have expired.
4. Remove the old key from the JSON array.
5. Deploy again.

The script `scripts/rotate-jwt-keys.sh` automates steps 1 and 4.

#### 9.3.2 Routing Table

| Method | Path prefix | Upstream | Auth | Circuit breaker |
|--------|------------|----------|------|----------------|
| `*` | `/api/v1/auth/*` | `iam:8081` | No | `cb-iam` |
| `*` | `/api/v1/users/*` | `iam:8081` | JWT | `cb-iam` |
| `*` | `/api/v1/scim/*` | `iam:8081` | SCIM bearer | `cb-iam` |
| `*` | `/api/v1/policies/*` | `policy:8082` | JWT | `cb-policy` |
| `*` | `/api/v1/threats/*` | `threat:8083` | JWT | `cb-threat` |
| `*` | `/api/v1/audit/*` | `audit:8084` | JWT | `cb-audit` |
| `*` | `/api/v1/alerts/*` | `alerting:8085` | JWT | `cb-alerting` |
| `*` | `/api/v1/compliance/*` | `compliance:8086` | JWT | `cb-compliance` |
| `GET` | `/health/live` | gateway | No | — |
| `GET` | `/health/ready` | gateway | No | — |
| `GET` | `/metrics` | gateway | No | — |

When a circuit breaker is open, the gateway returns:
```json
{ "error": { "code": "UPSTREAM_UNAVAILABLE", "message": "Service temporarily unavailable", "retryable": true } }
```

With `Retry-After: 10` header.

**Special rule for policy circuit breaker:** When `cb-policy` is open and the request requires policy evaluation (any data-access or export endpoint), the gateway returns `403 POLICY_SERVICE_UNAVAILABLE`. It never grants access when it cannot evaluate policy.

#### 9.3.3 mTLS Between Services

The gateway communicates with upstream services using mTLS. Each service validates the client certificate against the shared CA.

```go
// shared/middleware/mtls.go
func NewMTLSServer(certFile, keyFile, caFile string) (*tls.Config, error) {
    // Load service cert + key
    // Load CA for client verification
    // Return tls.Config with ClientAuth: tls.RequireAndVerifyClientCert
}

func NewMTLSClient(certFile, keyFile, caFile string) (*http.Client, error) {
    // Load client cert + key
    // Load CA for server verification
    // Return http.Client with custom TLS transport
}
```

### 9.4 IAM Service

#### 9.4.1 Database Schema

**001_create_orgs.up.sql**
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE orgs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    slug              TEXT NOT NULL UNIQUE,
    plan              TEXT NOT NULL DEFAULT 'free',    -- free | pro | enterprise
    isolation_tier    TEXT NOT NULL DEFAULT 'shared',  -- shared | schema | shard
    mfa_required      BOOLEAN NOT NULL DEFAULT FALSE,
    sso_required      BOOLEAN NOT NULL DEFAULT FALSE,
    max_users         INT,                             -- NULL = unlimited
    max_sessions      INT NOT NULL DEFAULT 5,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Orgs table is NOT org-scoped — it's a cross-tenant table
-- Only system role can read all orgs; app user can only read its own org
CREATE POLICY orgs_self_read ON orgs FOR SELECT
    USING (id = current_setting('app.org_id', true)::UUID);
ALTER TABLE orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE orgs FORCE ROW LEVEL SECURITY;
```

**002_create_users.up.sql**
```sql
CREATE TABLE users (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id               UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email                TEXT NOT NULL,
    display_name         TEXT NOT NULL DEFAULT '',
    password_hash        TEXT,
    status               TEXT NOT NULL DEFAULT 'active',
    mfa_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method           TEXT,                          -- 'totp' | 'webauthn' | NULL
    scim_external_id     TEXT,
    provisioning_status  TEXT NOT NULL DEFAULT 'complete',
    tier_isolation       TEXT NOT NULL DEFAULT 'shared',
    last_login_at        TIMESTAMPTZ,
    last_login_ip        INET,
    failed_login_count   INT NOT NULL DEFAULT 0,
    locked_until         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_users_org_id    ON users(org_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_email     ON users(email)  WHERE deleted_at IS NULL;
CREATE INDEX idx_users_scim_ext  ON users(org_id, scim_external_id) WHERE scim_external_id IS NOT NULL;

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY users_org_isolation ON users
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**003_create_api_tokens.up.sql**
```sql
CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_tokens_org_id  ON api_tokens(org_id);
CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id);

ALTER TABLE api_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY api_tokens_org_isolation ON api_tokens
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**004_create_sessions.up.sql**
```sql
CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    refresh_hash TEXT NOT NULL UNIQUE,
    ip_address   INET,
    user_agent   TEXT,
    country_code TEXT,
    city         TEXT,
    lat          DECIMAL(9,6),
    lng          DECIMAL(9,6),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id) WHERE revoked = FALSE;
CREATE INDEX idx_sessions_org_id  ON sessions(org_id)  WHERE revoked = FALSE;

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY sessions_org_isolation ON sessions
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**005_create_mfa_configs.up.sql**
```sql
CREATE TABLE mfa_configs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    org_id            UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type              TEXT NOT NULL DEFAULT 'totp',
    encrypted_secret  TEXT NOT NULL,    -- AES-256-GCM, includes key ID prefix: "mk1:base64..."
    backup_codes_hash TEXT[] NOT NULL DEFAULT '{}',  -- bcrypt hashes of backup codes
    verified          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE mfa_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE mfa_configs FORCE ROW LEVEL SECURITY;
CREATE POLICY mfa_configs_org_isolation ON mfa_configs
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
```

**006_create_outbox.up.sql**
```sql
-- Outbox table for IAM events
CREATE TABLE outbox_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,
    payload      BYTEA NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON outbox_records(created_at) WHERE status = 'pending';

CREATE OR REPLACE FUNCTION notify_outbox() RETURNS trigger AS $$
BEGIN PERFORM pg_notify('outbox_new', NEW.id::text); RETURN NEW; END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
    AFTER INSERT ON outbox_records FOR EACH ROW EXECUTE FUNCTION notify_outbox();
```

#### 9.4.2 MFA Encryption (Key Versioning)

TOTP secrets are encrypted with AES-256-GCM. The ciphertext is stored with a key ID prefix so the correct decryption key can be selected during rotation:

```go
// shared/crypto/aes.go
package crypto

type EncryptionKey struct {
    Kid    string `json:"kid"`
    Key    string `json:"key"`    // base64-encoded 32-byte key
    Status string `json:"status"` // "active" | "verify_only"
}

type EncryptionKeyring struct{ keys []EncryptionKey }

// Encrypt encrypts plaintext using the active key.
// Output format: "<kid>:<base64(nonce+ciphertext)>"
func (k *EncryptionKeyring) Encrypt(plaintext []byte) (string, error)

// Decrypt parses the kid prefix, finds the matching key, and decrypts.
// Works for all valid keys (active or verify_only).
func (k *EncryptionKeyring) Decrypt(ciphertext string) ([]byte, error)
```

#### 9.4.3 HTTP API

All endpoints from v1 spec, plus:

| Method | Path | Description | New in v2 |
|--------|------|-------------|-----------|
| `POST` | `/auth/register` | Create org + admin user | — |
| `POST` | `/auth/login` | Password login → JWT + refresh | — |
| `POST` | `/auth/refresh` | Exchange refresh token for new JWT | — |
| `POST` | `/auth/logout` | Revoke session + blacklist refresh token | — |
| `POST` | `/auth/mfa/enroll` | Begin TOTP/WebAuthn enrollment | WebAuthn |
| `POST` | `/auth/mfa/verify` | Complete enrollment | — |
| `POST` | `/auth/mfa/challenge` | Verify TOTP at login | — |
| `POST` | `/auth/webauthn/register` | Begin WebAuthn registration | Yes |
| `POST` | `/auth/webauthn/register/finish` | Complete WebAuthn registration | Yes |
| `POST` | `/auth/webauthn/login` | Begin WebAuthn login | Yes |
| `POST` | `/auth/webauthn/login/finish` | Complete WebAuthn login | Yes |
| `GET` | `/users` | List users (cursor paginated) | Cursor |
| `POST` | `/users` | Create user | — |
| `GET` | `/users/:id` | Get user | — |
| `PATCH` | `/users/:id` | Update user | — |
| `DELETE` | `/users/:id` | Soft-delete | — |
| `POST` | `/users/:id/suspend` | Suspend | — |
| `POST` | `/users/:id/activate` | Activate | — |
| `GET` | `/users/:id/sessions` | List active sessions | — |
| `DELETE` | `/users/:id/sessions/:sid` | Revoke session | — |
| `DELETE` | `/users/:id/sessions` | Revoke all sessions | Yes |
| `GET` | `/users/:id/tokens` | List API tokens | — |
| `POST` | `/users/:id/tokens` | Create API token | — |
| `DELETE` | `/users/:id/tokens/:tid` | Revoke token | — |
| `POST` | `/users/bulk` | Bulk create/update (SCIM internal) | Yes |
| `GET` | `/orgs/me` | Get current org settings | Yes |
| `PATCH` | `/orgs/me` | Update org settings | Yes |

**SCIM v2:** Same as v1 spec. Add `ETag` header support for conditional updates.

#### 9.4.4 Kafka Events (via Outbox)

All events written to `outbox_records` table, relay publishes to Kafka. Payload is `EventEnvelope` with appropriate `Type`:

| Event type | Topic | Payload key fields |
|------------|-------|-------------------|
| `auth.login.success` | `auth.events` | `user_id`, `ip`, `country`, `mfa_used` |
| `auth.login.failure` | `auth.events` | `email`, `ip`, `reason` |
| `auth.login.locked` | `auth.events` | `user_id`, `locked_until` |
| `auth.logout` | `auth.events` | `user_id`, `session_id` |
| `auth.mfa.enrolled` | `auth.events` | `user_id`, `method` |
| `auth.mfa.failed` | `auth.events` | `user_id`, `ip` |
| `auth.token.created` | `auth.events` | `user_id`, `token_id`, `scopes` |
| `auth.token.revoked` | `auth.events` | `user_id`, `token_id` |
| `user.created` | `audit.trail` + `saga.orchestration` | Full user object |
| `user.updated` | `audit.trail` | Changed fields diff |
| `user.deleted` | `audit.trail` + `saga.orchestration` | `user_id`, `org_id` |
| `user.suspended` | `audit.trail` | `user_id`, `reason` |
| `user.scim.provisioned` | `audit.trail` + `saga.orchestration` | SCIM payload |
| `user.scim.deprovisioned` | `audit.trail` + `saga.orchestration` | `user_id`, `scim_id` |

### 9.5 Phase 1 Acceptance Criteria

- [ ] `POST /auth/register` creates org + admin user. Both writes are in one DB transaction with an outbox record.
- [ ] `POST /auth/login` returns a JWT signed with `kid` in header. Refresh token stored as `refresh_hash` (SHA-256).
- [ ] JWT verified by gateway using multi-key keyring. Token from removed key returns 401.
- [ ] New key added alongside old → old tokens still verify. Old key removed → old tokens return 401.
- [ ] RLS enforced: querying `users` with `app.org_id=''` returns zero rows.
- [ ] RLS enforced: two orgs' users are never visible to each other even with a broken `WHERE` clause omitted.
- [ ] Outbox relay publishes events to Kafka within 200ms of commit.
- [ ] Relay handles PostgreSQL restart: events buffered in `outbox_records` are published when relay reconnects.
- [ ] Relay marks records `dead` after 5 failures and publishes to `outbox.dlq`.
- [ ] mTLS: request from non-mTLS client to IAM service is rejected.
- [ ] Passwords hashed with bcrypt cost 12. Raw passwords never appear in logs.
- [ ] TOTP secret stored as `"mk1:base64..."` format, decryptable only with correct key.
- [ ] `go test ./... -race` passes.
- [ ] `docker compose up` starts all infra and services healthy.

---

## 10. Phase 2 — Policy Engine

**Goal:** Policy evaluation is the most latency-sensitive path in the system. Target: p99 < 30ms. Results must be cached in Redis. The service fails closed when unavailable.

### 10.1 Database Schema

Same as v1, plus outbox table (same pattern as IAM 006 migration), plus:

**003_create_policy_cache.up.sql**
```sql
-- Policy evaluation cache log for audit purposes
CREATE TABLE policy_eval_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    user_id      UUID NOT NULL,
    action       TEXT NOT NULL,
    resource     TEXT NOT NULL,
    result       BOOLEAN NOT NULL,
    policy_ids   UUID[] NOT NULL DEFAULT '{}',
    latency_ms   INT NOT NULL,
    cached       BOOLEAN NOT NULL DEFAULT FALSE,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_eval_org_user ON policy_eval_log(org_id, user_id, evaluated_at DESC);

ALTER TABLE policy_eval_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_eval_log FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_eval_org_isolation ON policy_eval_log
    USING (org_id = current_setting('app.org_id', true)::UUID);
```

### 10.2 Redis Caching for Evaluate

Policy evaluation results are cached in Redis:

```
Key:   "policy:eval:{org_id}:{sha256(action+resource+user_id+user_groups)}"
Value: JSON { "permitted": bool, "matched_policies": [...], "reason": "..." }
TTL:   POLICY_CACHE_TTL_SECONDS (default: 30)
```

Cache is invalidated on any `policy.changes` Kafka event for the org. The policy service subscribes to its own topic as a consumer and calls `DEL` on all keys matching `policy:eval:{org_id}:*` (use Redis `SCAN` not `KEYS`).

### 10.3 Evaluator Interface (unchanged from v1)

The evaluator must be called within the circuit breaker. Gateway calls the policy service via mTLS HTTP. Policy service does not expose gRPC in Phase 2 (scaffold it in Phase 7).

### 10.4 Phase 2 Acceptance Criteria

- [ ] `POST /policies/evaluate` p99 < 30ms under 500 concurrent requests (k6 test).
- [ ] Second evaluate call for same inputs returns `cached: true` in eval log.
- [ ] Policy change invalidates cache: evaluate returns fresh result within 1s.
- [ ] Policy service circuit breaker open → gateway returns `403 POLICY_SERVICE_UNAVAILABLE`.
- [ ] All policy writes go through outbox. Cache invalidation via Kafka consumer.

---

## 11. Phase 3 — Event Bus, Outbox Relay & Audit Log

**Goal:** Kafka is fully operational. The Outbox relay runs in every service. The Audit Log consumes all events with bulk inserts, hash chaining, and CQRS read/write split.

### 11.1 Kafka Topic Configuration

```json
[
  { "name": "auth.events",            "partitions": 12, "replication": 3, "retention_ms": 604800000,  "compression": "lz4" },
  { "name": "policy.changes",         "partitions": 6,  "replication": 3, "retention_ms": 604800000,  "compression": "lz4" },
  { "name": "data.access",            "partitions": 24, "replication": 3, "retention_ms": 259200000,  "compression": "lz4" },
  { "name": "threat.alerts",          "partitions": 12, "replication": 3, "retention_ms": 2592000000, "compression": "lz4" },
  { "name": "audit.trail",            "partitions": 24, "replication": 3, "retention_ms": -1,         "compression": "lz4" },
  { "name": "notifications.outbound", "partitions": 6,  "replication": 3, "retention_ms": 86400000,   "compression": "lz4" },
  { "name": "saga.orchestration",     "partitions": 12, "replication": 3, "retention_ms": 604800000,  "compression": "lz4" },
  { "name": "outbox.dlq",             "partitions": 3,  "replication": 3, "retention_ms": -1,         "compression": "lz4" }
]
```

Replication factor 3 requires 3 Kafka brokers in staging/production. Docker Compose uses single-broker (replication=1) for local dev. The `create-topics.sh` script detects broker count and adjusts replication factor accordingly.

### 11.2 Audit Log Service — CQRS Architecture

```
services/audit/
├── pkg/
│   ├── consumer/
│   │   ├── bulk_writer.go      # Buffers + bulk-inserts to MongoDB primary
│   │   └── hash_chain.go       # Computes and stores hash chain
│   ├── repository/
│   │   ├── write.go            # Uses MONGO_URI_PRIMARY
│   │   └── read.go             # Uses MONGO_URI_SECONDARY
│   ├── handlers/
│   │   ├── events.go           # GET /audit/events (reads from secondary)
│   │   └── export.go           # Export jobs (reads from secondary)
│   └── integrity/
│       └── verifier.go         # Hash chain verification
```

#### 11.2.1 Bulk Insert with Backpressure

```go
// pkg/consumer/bulk_writer.go
type BulkWriter struct {
    coll        *mongo.Collection    // primary
    buffer      []mongo.WriteModel
    mu          sync.Mutex
    maxDocs     int                  // AUDIT_BULK_INSERT_MAX_DOCS (default: 500)
    flushAfter  time.Duration        // AUDIT_BULK_INSERT_FLUSH_MS (default: 1000ms)
    metrics     *BulkWriterMetrics
}

// Add appends a document to the buffer. Flushes if maxDocs reached.
func (b *BulkWriter) Add(ctx context.Context, doc AuditEvent) error

// flush writes the buffer to MongoDB as a bulk write (ordered=false for throughput).
// Called by Add when buffer is full, or by the ticker on flushAfter interval.
func (b *BulkWriter) flush(ctx context.Context) error {
    // mongo.Collection.BulkWrite with options.BulkWrite().SetOrdered(false)
    // Ordered=false: inserts continue even if one document fails (e.g. duplicate event_id)
    // Log failed documents individually, don't fail the entire batch
}
```

#### 11.2.2 Hash Chain Integrity

Each audit event stores a chain hash linking it to the previous event for the same `org_id`. This makes tampering with or deleting an event detectable.

```go
// pkg/consumer/hash_chain.go

// ChainHash computes HMAC-SHA256 of: prev_hash + event_id + org_id + type + occurred_at
// Key: AUDIT_HASH_CHAIN_SECRET
func ChainHash(secret, prevHash string, event AuditEvent) string

// AuditEvent in MongoDB includes:
type AuditEvent struct {
    // ... standard fields ...
    ChainHash     string    `bson:"chain_hash"`      // hash of this event
    PrevChainHash string    `bson:"prev_chain_hash"` // hash of previous event for this org
    ChainSeq      int64     `bson:"chain_seq"`       // monotonically increasing per org
}

// The integrity verifier (GET /audit/integrity) recomputes the chain
// and reports any gaps or hash mismatches.
```

#### 11.2.3 MongoDB Schema

Collection: `audit_events`

```js
db.audit_events.createIndex({ org_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ org_id: 1, type: 1, occurred_at: -1 })
db.audit_events.createIndex({ actor_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ event_id: 1 }, { unique: true })
db.audit_events.createIndex({ org_id: 1, chain_seq: 1 })  // for integrity checks
db.audit_events.createIndex({ occurred_at: 1 }, { expireAfterSeconds: <retention> })
```

#### 11.2.4 HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/audit/events` | List events (cursor paginated, all filters) |
| `GET` | `/audit/events/:id` | Get single event |
| `POST` | `/audit/export` | Trigger async CSV/JSON export |
| `GET` | `/audit/export/:job_id` | Poll export job status |
| `GET` | `/audit/export/:job_id/download` | Stream download when complete |
| `GET` | `/audit/integrity` | Verify hash chain for org (returns gaps/mismatches) |
| `GET` | `/audit/stats` | Event counts by type and day (for dashboard) |

### 11.3 Phase 3 Acceptance Criteria

- [ ] Kafka consumer processes 50,000 events/s (k6 + Kafka producer load test).
- [ ] Bulk writer flushes ≤ 500 docs per batch, ≤ 1000ms flush interval.
- [ ] Event from IAM login appears in MongoDB audit within p99 2s.
- [ ] Duplicate `event_id` skipped, no error.
- [ ] Dead-letter documents in `audit_dead_letter` after 5 consumer failures.
- [ ] `GET /audit/events` reads from MongoDB secondary (verify with `explain()`).
- [ ] `GET /audit/integrity` returns `ok: true` on a clean chain.
- [ ] Manually deleting a document causes `GET /audit/integrity` to report a gap.
- [ ] Hash chain breaks are reported in Prometheus metric `audit_chain_integrity_failures_total`.

---

## 12. Phase 4 — Threat Detection & Alerting

**Goal:** Real-time detection via Redis-backed counters. Composite risk scoring. Saga-based alert lifecycle. SIEM payloads signed with HMAC.

### 12.1 Threat Detection Service

Identical detector interfaces to v1, with these additions:

**Account takeover detector** (`ato.go`):
- Detects: login success from a new device/browser fingerprint after a recent password change.
- Signal: `auth.login.success` events where `user_agent` doesn't match any session in the last 30 days AND `password_changed_within_24h = true` in the payload.
- Risk score: 0.7.

**Privilege escalation detector** (`priv_escalation.go`):
- Detects: user granted admin role within 60 minutes of a new login.
- Signal: `policy.changes` event with action `role.grant` where `target_user_id` logged in within 60 minutes.
- Risk score: 0.9.

All detector results feed the composite scorer (same formula as v1). Composite score ≥ 0.5 → alert. Composite ≥ 0.8 → HIGH severity. Composite ≥ 0.95 → CRITICAL.

### 12.2 Alert Lifecycle (Saga)

Alert creation, acknowledgement, and resolution form a saga published on `saga.orchestration`:

```
threat.alert.created   → saga step 1: alert persisted in MongoDB
                       → saga step 2: notification enqueued (notifications.outbound)
                       → saga step 3: SIEM webhook fired (if configured)
                       → saga step 4: audit event written (audit.trail)
threat.alert.acknowledged → updates alert status, publishes audit event
threat.alert.resolved     → updates alert status, computes MTTR, publishes audit event
```

MTTR (mean time to resolve) is tracked per org per severity for the compliance dashboard.

### 12.3 SIEM Webhook Signing

Every SIEM webhook POST includes:
```
X-OpenGuard-Signature: sha256=<hmac-sha256-hex>
X-OpenGuard-Delivery: <uuid>
X-OpenGuard-Timestamp: <unix seconds>
```

HMAC is computed over `timestamp.payload` using `ALERTING_SIEM_WEBHOOK_HMAC_SECRET`. Replay protection: reject requests where `abs(now - timestamp) > 300` seconds. Document in `docs/api/webhooks.md`.

### 12.4 Phase 4 Acceptance Criteria

- [ ] 11 failed logins within window produce HIGH alert in MongoDB within 3s.
- [ ] Privilege escalation detector fires within 5s of role grant event.
- [ ] SIEM webhook POST includes valid HMAC signature.
- [ ] Alert saga completes all 4 steps; all steps produce audit events.
- [ ] MTTR is computed and stored on alert resolution.
- [ ] `GET /threats/stats` returns correct open count by severity.

---

## 13. Phase 5 — Compliance & Analytics

**Goal:** ClickHouse receives a bulk-inserted event stream. Report generation is concurrency-limited. PDF output is complete and signed. Analytics queries meet p99 < 100ms.

### 13.1 ClickHouse Schema

```sql
CREATE TABLE IF NOT EXISTS events (
    event_id     String        CODEC(ZSTD(3)),
    type         LowCardinality(String),
    org_id       String        CODEC(ZSTD(3)),
    actor_id     String        CODEC(ZSTD(3)),
    actor_type   LowCardinality(String),
    occurred_at  DateTime64(3, 'UTC'),
    source       LowCardinality(String),
    payload      String        CODEC(ZSTD(3))
) ENGINE = MergeTree()
PARTITION BY (toYYYYMM(occurred_at), org_id)
ORDER BY (org_id, type, occurred_at)
TTL occurred_at + INTERVAL 2 YEAR
SETTINGS index_granularity = 8192;

-- Materialized view for fast dashboard queries
CREATE MATERIALIZED VIEW IF NOT EXISTS event_counts_daily
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (org_id, type, day)
AS SELECT
    org_id,
    type,
    toDate(occurred_at) AS day,
    count() AS cnt
FROM events
GROUP BY org_id, type, day;

CREATE TABLE IF NOT EXISTS alert_stats (
    org_id       String,
    day          Date,
    severity     LowCardinality(String),
    count        UInt64,
    mttr_seconds UInt64
) ENGINE = SummingMergeTree(count, mttr_seconds)
ORDER BY (org_id, day, severity);
```

### 13.2 ClickHouse Bulk Insertion

The compliance consumer **must not** insert one row per Kafka message. Use a buffered writer:

```go
// Bulk insert config:
// CLICKHOUSE_BULK_FLUSH_ROWS = 5000 (rows per batch)
// CLICKHOUSE_BULK_FLUSH_MS   = 2000 (max ms before forced flush)

// Use ClickHouse's native batch API (clickhouse-go v2):
batch, err := conn.PrepareBatch(ctx, "INSERT INTO events")
for _, event := range bufferedEvents {
    batch.Append(event.EventID, event.Type, ...)
}
batch.Send()
```

Throughput target: 100,000 rows/second. Verify in Phase 9.

### 13.3 Report Generation with Bulkhead

```go
// pkg/reporter/generator.go

var reportBulkhead = resilience.NewBulkhead(
    config.DefaultInt("COMPLIANCE_REPORT_MAX_CONCURRENT", 10),
)

func (g *Generator) Generate(ctx context.Context, report *Report) error {
    return reportBulkhead.Execute(ctx, func() error {
        return g.generate(ctx, report)
    })
}
```

When bulkhead is full: return `429` with `Retry-After: 30`.

### 13.4 Phase 5 Acceptance Criteria

- [ ] ClickHouse receives 10,000 events in ≤ 3 batches of ≤ 5,000 rows each.
- [ ] Materialized view `event_counts_daily` is populated automatically.
- [ ] `GET /compliance/stats?metric=logins&granularity=day` p99 < 100ms.
- [ ] GDPR report includes all 5 sections and is valid PDF with ToC and page numbers.
- [ ] 11 concurrent report requests: 10 succeed, 11th returns 429.
- [ ] Report includes a digital timestamp (generation time + org name + hash of report content).

---

## 14. Phase 6 — Frontend (Next.js)

Identical to v1 spec, with these additions:

### 14.1 Real-Time Threat Dashboard

Replace the 30-second polling on the threats page with Server-Sent Events (SSE):

```ts
// app/(dashboard)/threats/page.tsx
// Use native EventSource connected to GET /api/v1/threats/stream
// The alerting service exposes an SSE endpoint that pushes new alerts
// Reconnects automatically on disconnect
```

The alerting service adds `GET /alerts/stream` — an SSE endpoint that keeps the connection open and writes `data: <json>\n\n` for each new alert.

### 14.2 Audit Log — Virtual Scrolling

The audit log page must use virtual scrolling (TanStack Virtual) for the events table. Loading 1,000+ rows into the DOM is not acceptable. Fetch 100 rows at a time using cursor pagination.

### 14.3 Security Headers (Next.js)

In `next.config.ts`, add all security headers plus a strict Content Security Policy:

```ts
const securityHeaders = [
    { key: 'X-Content-Type-Options', value: 'nosniff' },
    { key: 'X-Frame-Options', value: 'DENY' },
    { key: 'Strict-Transport-Security', value: 'max-age=63072000; includeSubDomains; preload' },
    { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
    { key: 'Permissions-Policy', value: 'camera=(), microphone=(), geolocation=()' },
    {
        key: 'Content-Security-Policy',
        value: [
            "default-src 'self'",
            "script-src 'self' 'unsafe-inline'",  // Next.js requires unsafe-inline
            "style-src 'self' 'unsafe-inline'",
            "img-src 'self' data: blob:",
            "connect-src 'self' https://api.openguard.example.com",
            "frame-ancestors 'none'",
        ].join('; '),
    },
];
```

### 14.4 Phase 6 Acceptance Criteria

- [ ] Dashboard loads in < 2s on 3G throttled connection (Lighthouse).
- [ ] Threat alert SSE stream delivers new alert to browser within 1s of creation.
- [ ] Audit log table handles 10,000 rows without browser jank (virtual scroll).
- [ ] All security headers present on every response (verify with `curl -I`).
- [ ] Lighthouse accessibility score ≥ 90.

---

## 15. Phase 7 — Infra, CI/CD & Observability

### 15.1 Docker Compose

```yaml
# infra/docker/docker-compose.yml
services:
  postgres:
    image: postgres:16-alpine
    environment: [POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB]
    volumes: [postgres-data:/var/lib/postgresql/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $$POSTGRES_USER"]
      interval: 5s
      timeout: 5s
      retries: 10

  mongo-primary:
    image: mongo:7
    command: mongod --replSet rs0 --bind_ip_all
    volumes: [mongo-primary-data:/data/db]
    healthcheck:
      test: ["CMD", "mongosh", "--eval", "db.adminCommand('ping')"]

  mongo-secondary:
    image: mongo:7
    command: mongod --replSet rs0 --bind_ip_all
    volumes: [mongo-secondary-data:/data/db]
    depends_on: [mongo-primary]

  mongo-init:
    image: mongo:7
    depends_on: [mongo-primary, mongo-secondary]
    command: >
      mongosh --host mongo-primary --eval
      "rs.initiate({_id:'rs0', members:[{_id:0,host:'mongo-primary:27017'},{_id:1,host:'mongo-secondary:27017',priority:0}]})"

  redis:
    image: redis:7-alpine
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes: [redis-data:/data]

  zookeeper:
    image: bitnami/zookeeper:3.9
    environment: [ALLOW_ANONYMOUS_LOGIN=yes]
    volumes: [zookeeper-data:/bitnami/zookeeper]

  kafka:
    image: bitnami/kafka:3.6
    depends_on: [zookeeper]
    environment:
      - KAFKA_CFG_ZOOKEEPER_CONNECT=zookeeper:2181
      - KAFKA_CFG_NUM_PARTITIONS=12
      - KAFKA_CFG_DEFAULT_REPLICATION_FACTOR=1
      - ALLOW_PLAINTEXT_LISTENER=yes
    volumes: [kafka-data:/bitnami/kafka]

  clickhouse:
    image: clickhouse/clickhouse-server:24
    volumes: [clickhouse-data:/var/lib/clickhouse]
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]

  jaeger:
    image: jaegertracing/all-in-one:latest
    ports: ["16686:16686", "4317:4317"]

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus

  grafana:
    image: grafana/grafana:latest
    volumes:
      - grafana-data:/var/lib/grafana
      - ./monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards
    environment: [GF_SECURITY_ADMIN_PASSWORD=admin]
    ports: ["3001:3000"]

volumes:
  postgres-data: mongo-primary-data: mongo-secondary-data: redis-data:
  zookeeper-data: kafka-data: clickhouse-data: prometheus-data: grafana-data:
```

### 15.2 GitHub Actions CI

**`.github/workflows/ci.yml`:**

```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main]

jobs:
  go-test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env: { POSTGRES_PASSWORD: test, POSTGRES_DB: openguard_test }
        options: --health-cmd pg_isready --health-interval 5s --health-retries 10
      redis:
        image: redis:7-alpine
        options: --health-cmd "redis-cli ping" --health-interval 5s
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22', cache: true }
      - run: go work sync
      - run: go test ./... -race -coverprofile=coverage.out -covermode=atomic -timeout 5m
      - run: go vet ./...
      - name: Check coverage
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Coverage $COVERAGE% is below 70% threshold"
            exit 1
          fi

  go-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout 5m

  sql-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          go install github.com/ryanprior/go-sqllint@latest
          find services -name "*.go" | xargs go-sqllint
          # Fails on any string concatenation in SQL queries

  next-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: web/package-lock.json }
      - run: cd web && npm ci && npm run build && npm run lint

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Go dependency audit
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
      - name: Container scan
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: fs
          severity: CRITICAL,HIGH
          exit-code: 1

  contract-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - name: Run contract tests
        run: go test ./shared/... -run TestContract -v
        # Contract tests verify: EventEnvelope produced by IAM is parseable by Audit consumer
        # Contract tests verify: Policy evaluate request/response shape
```

**`.github/workflows/security.yml`** — runs weekly:
```yaml
name: Security Audit
on:
  schedule: [{ cron: '0 3 * * 1' }]  # Monday 3am UTC
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: govulncheck ./...
      - run: cd web && npm audit --audit-level=high
      - uses: aquasecurity/trivy-action@master
        with: { scan-type: fs, format: sarif, output: trivy.sarif }
      - uses: github/codeql-action/upload-sarif@v3
        with: { sarif_file: trivy.sarif }
```

### 15.3 Prometheus Metrics (Extended)

Every service exposes these metrics in addition to the standard HTTP metrics:

| Metric | Type | Labels | Service |
|--------|------|--------|---------|
| `openguard_outbox_pending_records` | Gauge | `service` | All |
| `openguard_outbox_relay_duration_seconds` | Histogram | `service`, `result` | All |
| `openguard_circuit_breaker_state` | Gauge | `name`, `state` | Gateway |
| `openguard_rls_session_set_duration_seconds` | Histogram | `service` | All (Postgres) |
| `openguard_kafka_bulk_insert_size` | Histogram | `service` | Audit, Compliance |
| `openguard_kafka_consumer_lag` | Gauge | `topic`, `group` | All consumers |
| `openguard_audit_chain_integrity_failures_total` | Counter | `org_id` | Audit |
| `openguard_policy_cache_hits_total` | Counter | — | Policy |
| `openguard_policy_cache_misses_total` | Counter | — | Policy |
| `openguard_threat_detections_total` | Counter | `detector`, `severity` | Threat |
| `openguard_report_generation_duration_seconds` | Histogram | `type`, `format` | Compliance |
| `openguard_report_bulkhead_rejected_total` | Counter | — | Compliance |

### 15.4 Alertmanager Rules

In `infra/monitoring/alerts/openguard.yml`:

```yaml
groups:
- name: openguard
  rules:
  - alert: OutboxLagHigh
    expr: openguard_outbox_pending_records > 1000
    for: 2m
    labels: { severity: warning }
    annotations:
      summary: "Outbox relay is lagging ({{ $value }} pending records)"

  - alert: CircuitBreakerOpen
    expr: openguard_circuit_breaker_state{state="open"} == 1
    for: 30s
    labels: { severity: critical }
    annotations:
      summary: "Circuit breaker {{ $labels.name }} is open"

  - alert: KafkaConsumerLagHigh
    expr: openguard_kafka_consumer_lag > 50000
    for: 5m
    labels: { severity: warning }

  - alert: AuditChainIntegrityFailure
    expr: increase(openguard_audit_chain_integrity_failures_total[5m]) > 0
    labels: { severity: critical }
    annotations:
      summary: "Audit chain integrity violation detected for org {{ $labels.org_id }}"

  - alert: PolicyServiceDown
    expr: up{job="policy"} == 0
    for: 30s
    labels: { severity: critical }
    annotations:
      summary: "Policy service is down — all policy evaluations are failing closed"
```

### 15.5 Helm Chart

`infra/k8s/helm/openguard/` with:
- `Deployment` per service with `minReadySeconds: 30` and `RollingUpdate` strategy.
- `PodDisruptionBudget` per service: `minAvailable: 1`.
- `HorizontalPodAutoscaler` for gateway, IAM, policy, threat: scale on CPU 70% AND custom metric `openguard_kafka_consumer_lag`.
- `NetworkPolicy`: each service only accepts traffic from the gateway (and from Kafka for consumers). No service can directly call another service's write endpoints.
- `ServiceAccount` per service with least-privilege RBAC.
- `Secret` references via `external-secrets.io` annotations for production. Plain secrets for dev.
- `topologySpreadConstraints`: spread pods across 3 AZs.

### 15.6 Phase 7 Acceptance Criteria

- [ ] `docker compose up` starts all infra healthy with MongoDB replica set initialized.
- [ ] `go test ./... -race` passes in CI with ≥ 70% coverage.
- [ ] `govulncheck ./...` reports no CRITICAL vulnerabilities.
- [ ] SQL lint catches a deliberately injected string concatenation in a test file.
- [ ] Contract test verifies IAM event is parseable by Audit consumer.
- [ ] Prometheus scrapes all 8 services. All `openguard_*` metrics appear in Grafana.
- [ ] `OutboxLagHigh` alert fires when relay is artificially stopped.
- [ ] `CircuitBreakerOpen` alert fires when policy service is killed.
- [ ] `helm lint` and `helm template` pass without warnings.

---

## 16. Phase 8 — Security Hardening & Secret Rotation

### 16.1 HTTP Security Middleware (all services)

```go
// shared/middleware/security.go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Content-Security-Policy", "default-src 'none'")
        w.Header().Set("Referrer-Policy", "no-referrer")
        w.Header().Set("X-Request-ID", generateRequestID())
        next.ServeHTTP(w, r)
    })
}
```

### 16.2 SSRF Protection

The SIEM webhook URL (`ALERTING_SIEM_WEBHOOK_URL`) is validated on startup and on update to prevent SSRF:

```go
// pkg/service/alerting.go
func validateWebhookURL(raw string) error {
    u, err := url.Parse(raw)
    if err != nil {
        return err
    }
    if u.Scheme != "https" {
        return errors.New("webhook URL must use HTTPS")
    }
    // Resolve to IP and block private ranges
    ips, err := net.LookupHost(u.Hostname())
    for _, ip := range ips {
        parsed := net.ParseIP(ip)
        if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
            return fmt.Errorf("webhook URL resolves to private IP %s — SSRF blocked", ip)
        }
    }
    return nil
}
```

### 16.3 Safe Logger (no secret leakage)

```go
// shared/telemetry/logger.go
var sensitiveKeys = []string{
    "password", "secret", "token", "key", "auth", "credential",
    "private", "bearer", "authorization", "cookie", "session",
}

// SafeAttr returns a slog.Attr with the value redacted if the key is sensitive.
func SafeAttr(key string, value any) slog.Attr {
    keyLower := strings.ToLower(key)
    for _, sensitive := range sensitiveKeys {
        if strings.Contains(keyLower, sensitive) {
            return slog.String(key, "[REDACTED]")
        }
    }
    return slog.Any(key, value)
}
```

### 16.4 Secret Rotation Runbook

Document in `docs/runbooks/secret-rotation.md`:

**JWT key rotation (zero-downtime):**
1. Generate new key: `scripts/rotate-jwt-keys.sh new`.
2. Update `GATEWAY_JWT_KEYS_JSON` to include both old (`verify_only`) and new (`active`) keys.
3. Rolling deploy gateway. New tokens signed with new key; old tokens still verify.
4. Wait `JWT_EXPIRY_SECONDS` seconds.
5. Update env to remove old key.
6. Rolling deploy gateway.

**MFA encryption key rotation:**
1. Add new key to `IAM_MFA_ENCRYPTION_KEY_JSON` as `active`, set old to `verify_only`.
2. Deploy IAM.
3. Run `scripts/re-encrypt-mfa.sh` — reads all `mfa_configs`, decrypts with old key, re-encrypts with new key. Runs in batches of 100, waits 50ms between batches to avoid DB overload.
4. Remove old key from JSON. Deploy IAM.

**Kafka SASL credential rotation:**
1. Add new credential to Kafka ACL without removing old.
2. Update `KAFKA_SASL_PASSWORD` in env. Rolling deploy all services.
3. Remove old credential from Kafka ACL.

### 16.5 Dependency Pinning

`go.sum` must be committed and CI must fail if `go.sum` is not up to date (`go mod verify`). Node dependencies pinned with `package-lock.json` (exact versions, not ranges). `dependabot.yml` configured for weekly auto-PRs for Go and Node.

### 16.6 Phase 8 Acceptance Criteria

- [ ] Security headers on every HTTP response from every service.
- [ ] SSRF: webhook URL `http://localhost/internal` is rejected at configuration time.
- [ ] Safe logger: `password=secret123` does not appear in structured log output.
- [ ] JWT rotation runbook executed end-to-end: old tokens rejected after rotation complete.
- [ ] MFA re-encryption script runs without data loss (verify before/after spot check).
- [ ] `go mod verify` passes in CI.
- [ ] `govulncheck` and `npm audit --audit-level=high` report zero issues.

---

## 17. Phase 9 — Load Testing & Performance Tuning

**Goal:** Verify every SLO from Section 1.2. No phase is "done" until SLOs are met.

### 17.1 k6 Test Scripts

Produce these scripts in `loadtest/`:

**`auth.js`** — login throughput:
```js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '1m', target: 500 },
        { duration: '3m', target: 2000 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        'http_req_duration{scenario:default}': ['p(99)<150'],
        'http_req_failed': ['rate<0.01'],
    },
};

export default function () {
    const res = http.post(`${__ENV.BASE_URL}/api/v1/auth/login`, JSON.stringify({
        email: `user-${__VU}@load-test.example.com`,
        password: 'Load-test-password-123!',
    }), { headers: { 'Content-Type': 'application/json' } });

    check(res, {
        'status is 200': (r) => r.status === 200,
        'has token': (r) => JSON.parse(r.body).token !== undefined,
    });
    sleep(0.5);
}
```

**`policy-evaluate.js`** — policy evaluation latency (most critical):
```js
// 10,000 req/s with p99 < 30ms
// Test both cache-hit and cache-miss paths
// Cache-hit: same action+resource+user, p99 target 5ms
// Cache-miss: unique resource per VU, p99 target 30ms
```

**`audit-query.js`** — read path under load:
```js
// 1,000 req/s GET /audit/events with various filters
// p99 < 100ms
// Verify MongoDB explains show secondaryPreferred
```

**`kafka-throughput.js`** — event bus capacity:
```js
// Direct Kafka producer (use k6 xk6-kafka extension)
// Produce 50,000 events/s to audit.trail
// Verify consumer lag stays below 10,000 messages
```

### 17.2 Tuning Targets and Actions

Run `make load-test` and address failures:

| SLO failing | Likely cause | Tuning action |
|-------------|-------------|---------------|
| Login p99 > 150ms | bcrypt too slow under load | Increase IAM replicas; bcrypt is CPU-bound |
| Policy evaluate p99 > 30ms | Cache miss on cold start | Pre-warm cache on deployment |
| Audit query p99 > 100ms | Missing MongoDB index | Add compound index, analyze with `explain()` |
| Kafka consumer lag growing | Bulk writer too slow | Increase `AUDIT_BULK_INSERT_MAX_DOCS`, tune MongoDB write concern |
| Memory OOM on IAM pod | Connection pool too large | Reduce `POSTGRES_POOL_MAX_CONNS` per replica, add replicas instead |

### 17.3 Phase 9 Acceptance Criteria

- [ ] `auth.js` k6 run: p99 login < 150ms at 2,000 req/s, error rate < 1%.
- [ ] `policy-evaluate.js`: p99 < 5ms (cached), p99 < 30ms (uncached) at 10,000 req/s.
- [ ] `audit-query.js`: p99 < 100ms at 1,000 req/s.
- [ ] Kafka consumer lag stays < 10,000 during 50,000 events/s burst.
- [ ] All k6 HTML reports committed to `loadtest/results/`.
- [ ] Grafana dashboards show all SLOs met under load (screenshot in docs).

---

## 18. Phase 10 — Documentation & Runbooks

### 18.1 Required Documents

**`README.md`** — must contain:
- One-sentence project description.
- Feature matrix (what OpenGuard does vs Atlassian Guard).
- Quick start: `git clone`, `cp .env.example .env`, `make dev` — working in < 5 minutes.
- Architecture diagram (Mermaid).
- SLO table (from Section 1.2).
- License and contributing links.

**`docs/architecture.md`** — must contain:
- Component diagram (Mermaid C4 level 2).
- Transactional Outbox flow diagram.
- RLS enforcement diagram.
- Circuit breaker state machine diagram.
- Saga choreography diagram (user provisioning).
- Database ER diagram (Mermaid erDiagram) for each service.

**`docs/contributing.md`** — must contain:
- Local dev setup (Docker Compose).
- Makefile targets explained.
- Adding a new Kafka consumer.
- Adding a new threat detector (with template).
- Adding a new compliance report type.
- Adding a new RLS-protected table (checklist).
- PR requirements: tests, lint, contract test if schema changes.
- Commit format: Conventional Commits.

**OpenAPI specs** — `docs/api/<service>.openapi.json` for all 7 services, valid OpenAPI 3.1, passing `redocly lint`.

### 18.2 Operational Runbooks

`docs/runbooks/` must contain:

| File | Scenario |
|------|----------|
| `kafka-consumer-lag.md` | Consumer lag > 50k. Steps: check bulk writer, scale consumers, check MongoDB write saturation. |
| `circuit-breaker-open.md` | Circuit breaker fired. Steps: identify failing service, check health endpoints, manual reset procedure. |
| `audit-hash-mismatch.md` | Integrity check fails. Steps: identify affected org, time range, gap analysis, escalation path. |
| `secret-rotation.md` | Full rotation procedures for all secret types. |
| `outbox-dlq.md` | Messages in `outbox.dlq`. Steps: inspect, replay, investigate root cause. |
| `postgres-rls-bypass.md` | If a query returns cross-tenant data (must never happen). Incident response. |
| `load-shedding.md` | Under extreme load. Steps: increase rate limits temporarily, scale services, shed non-critical consumers. |

### 18.3 Phase 10 Acceptance Criteria

- [ ] `make dev` runs to a working state on a clean machine following `README.md` only.
- [ ] All 7 OpenAPI specs pass `redocly lint`.
- [ ] Architecture Mermaid diagrams render in GitHub Markdown.
- [ ] All 7 runbooks are present and reviewed by a second engineer (or simulated review by a second LLM pass).
- [ ] `docs/contributing.md` — adding a new detector by following the guide produces a passing test.

---

## 19. Cross-Cutting Concerns

### 19.1 Structured Logging

All services use `log/slog` with JSON output in non-dev environments. Every log entry must include:

| Field | Source |
|-------|--------|
| `service` | Hardcoded service name |
| `env` | `APP_ENV` |
| `level` | Log level |
| `msg` | Human-readable message |
| `trace_id` | OpenTelemetry trace ID |
| `span_id` | OpenTelemetry span ID |
| `request_id` | `X-Request-ID` header |
| `org_id` | From RLS context (omit for system operations) |
| `user_id` | From JWT (omit for unauthenticated requests) |
| `duration_ms` | For request-scoped logs |

Use `SafeAttr` (Section 16.3) for all log attributes.

### 19.2 Distributed Tracing

Every service initializes OpenTelemetry on startup. Traces propagate via W3C `traceparent` header between services. The Outbox relay injects `trace_id` from the parent context into the `EventEnvelope`, so you can trace from an HTTP request all the way to the audit event in MongoDB.

Sampling rate: `OTEL_SAMPLING_RATE` (0.1 in production, 1.0 in development).

### 19.3 Graceful Shutdown (30-second window)

```go
// main.go pattern for every service
func main() {
    // ... setup ...

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Order matters:
    // 1. Stop accepting new HTTP requests (server.Shutdown)
    // 2. Stop Kafka consumer (no new messages)
    // 3. Flush Outbox relay (publish buffered records)
    // 4. Flush bulk writer (write buffered MongoDB/ClickHouse docs)
    // 5. Close DB connections
    server.Shutdown(ctx)
    kafkaConsumer.Close()
    outboxRelay.Flush(ctx)
    bulkWriter.Flush(ctx)
    dbPool.Close()
    mongoClient.Disconnect(ctx)
}
```

### 19.4 Health Checks

Every service:
- `GET /health/live` — returns `200 {"status":"ok"}` immediately. Used by Kubernetes liveness probe.
- `GET /health/ready` — checks PostgreSQL, MongoDB, Redis, Kafka connectivity. Returns `200` only if all pass. Returns `503 {"status":"not_ready","checks":{...}}` with per-dependency detail. Used by Kubernetes readiness probe.

Readiness probe failure should trigger circuit breaker state change if the service is a dependency.

### 19.5 Idempotency

All `POST` endpoints that create resources accept an `Idempotency-Key` header (UUID). Cached in Redis for 24 hours: key = `idempotent:{service}:{idempotency-key}`, value = response status + body. On duplicate key: return cached response with `Idempotency-Replayed: true` header.

### 19.6 Request Validation

Use `github.com/go-playground/validator/v10` for struct-level validation. Every handler binds the request body to a typed struct and calls `validate.Struct()` before passing to the service layer. Validation errors return `422 VALIDATION_ERROR` with per-field detail:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "fields": [
      { "field": "email", "message": "must be a valid email address" },
      { "field": "password", "message": "must be at least 12 characters" }
    ]
  }
}
```

### 19.7 Testing Standards

| Layer | Tool | Requirement |
|-------|------|-------------|
| Unit tests | `testing` + `testify` | ≥ 70% per package, deterministic, no `time.Sleep` |
| Integration tests | `testcontainers-go` | PostgreSQL + Redis + MongoDB containers, 1 per service |
| Contract tests | Custom (in `shared/`) | Verify producer → consumer schema compatibility |
| API tests | `net/http/httptest` | All happy paths + key error paths |
| Load tests | k6 | All SLOs from Section 1.2 |
| Chaos tests (Phase 9+) | `toxiproxy` | Verify circuit breaker and outbox behavior under network partition |

---

## 20. Acceptance Criteria (Full System)

When all 10 phases are complete, the following end-to-end scenario must execute without manual intervention. Run it as a CI job on every release.

```
1.  docker compose up -d                         → all services healthy
2.  POST /api/v1/auth/register                   → org "Acme" + admin user created
3.  POST /api/v1/auth/login                      → JWT issued, kid in header
4.  POST /api/v1/policies                        → IP allowlist policy created
5.  POST /api/v1/policies/evaluate               → blocked IP returns permitted:false
6.  Simulate 11 failed logins (k6 script)        → HIGH alert in MongoDB within 5s
7.  GET /api/v1/threats/alerts                   → alert visible, severity=high
8.  Verify Slack webhook mock received payload   → HMAC signature valid
9.  GET /api/v1/audit/events                     → all events from steps 2-8 present
10. GET /api/v1/audit/integrity                  → ok:true, no chain gaps
11. POST /api/v1/compliance/reports {type:gdpr}  → report job created
12. Poll GET /api/v1/compliance/reports/:id      → status=completed within 60s
13. GET /api/v1/compliance/reports/:id/download  → valid PDF, all 5 GDPR sections
14. Open http://localhost:3000                   → redirect to /login
15. Login with admin credentials                → dashboard loads, alert count = 1
16. Open /threats                               → SSE stream delivers alert within 1s
17. Open /audit                                 → virtual-scrolled table, all events
18. JWT key rotation: add new key, deploy       → old tokens still verify
19. JWT key rotation: remove old key, deploy    → old tokens return 401
20. Kill policy service                         → gateway returns 403 POLICY_SERVICE_UNAVAILABLE (not 500)
21. Restart policy service                      → circuit breaker resets, requests succeed
22. Kill Kafka                                  → IAM login succeeds, outbox records pending
23. Restart Kafka                               → outbox records published within 30s
24. go test ./... -race                         → all tests pass
25. k6 run loadtest/auth.js                     → p99 < 150ms at 2,000 req/s
26. k6 run loadtest/policy-evaluate.js          → p99 < 30ms at 10,000 req/s
27. docker compose down                         → clean shutdown, no data loss
```

Every step is a CI assertion. The release pipeline does not publish unless all 27 steps pass.

---

*End of OpenGuard Enterprise Specification v2.0. Begin with the prerequisites in Phase 1 Section 9.1. Do not write service code until the Outbox table migrations and the RLS policy pattern are implemented and tested.*