<div align="center">

<img src="https://img.shields.io/badge/OpenGuard-Enterprise%20Security%20Control%20Plane-1a1a2e?style=for-the-badge&logo=shield&logoColor=white" alt="OpenGuard" />

<br/>

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](go.work)
[![Angular](https://img.shields.io/badge/Angular-19+-DD0031?logo=angular)](web/package.json)
[![CI](https://img.shields.io/github/actions/workflow/status/openguard/openguard/ci.yml?label=CI&logo=github-actions)](/.github/workflows/ci.yml)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-6BA539?logo=swagger)](docs/openapi/)
[![SCIM](https://img.shields.io/badge/SCIM-2.0-orange)](https://www.simplecloud.info/)

**An open-source, self-hostable enterprise security control plane.**
Policy enforcement, cryptographic audit trails, and real-time threat detection — without routing user traffic through a proxy.

[Quick Start](#-quick-start) · [Architecture](#-architecture) · [Services](#-services) · [SDK](#-sdk) · [Configuration](#-configuration) · [Deployment](#-deployment)

</div>

---

## What is OpenGuard?

OpenGuard is a **centralized governance hub** for connected applications. It sits beside your services — not in front of them. Applications register with OpenGuard and integrate via a lightweight SDK, SCIM 2.0, OIDC/SAML, and outbound webhooks. User traffic never flows *through* OpenGuard, which means zero added latency to your critical paths and no single point of failure.

**Built for Fortune-500 scale:** 100,000+ users, 10,000+ organizations, millions of audit events per day, sub-100ms policy evaluation at p99, and a cryptographically verifiable audit trail.

```
┌─────────────────────────────────────────────────────────────┐
│                     Your Application                        │
│                                                             │
│   HTTP Request ──► Your Server                              │
│                        │                                    │
│                        ▼                                    │
│              OpenGuard SDK (embedded)                       │
│              ┌───────────────────┐                          │
│              │ Local cache hit?  │──Yes──► Allow/Deny (<1ms)│
│              └────────┬──────────┘                          │
│                       │ No (cache miss)                     │
│                       ▼                                     │
│              POST /v1/policy/evaluate                       │
│                       │                                     │
└───────────────────────┼─────────────────────────────────────┘
                        │                    ┌────────────────┐
                        └──────────────────► │  OpenGuard     │
                                             │  Control Plane │
                                             └────────────────┘
```

---

## ✨ Capabilities

| Capability | Details |
|---|---|
| **Identity & Access Management** | OIDC/SAML IdP · SSO · SCIM 2.0 provisioning · TOTP/WebAuthn MFA · API token lifecycle · Session management with revocation |
| **Policy Engine** | Real-time RBAC evaluation via SDK · Fail-closed after 60s local cache TTL · Redis cache-aside · 10,000 req/s at p99 < 5ms (cached) |
| **Connector Registry** | Application registration · API key issuance (`ogk_` prefix) · PBKDF2 hash at rest · Fast-hash Redis prefix lookup for 20,000 req/s event ingest |
| **Event Ingestion** | Connected apps push audit events via `POST /v1/events/ingest` · Deduplicated by `event_id` · Normalized into the shared Kafka pipeline |
| **Threat Detection** | Streaming anomaly scoring: brute force · impossible travel · off-hours access · data exfiltration · account takeover · privilege escalation |
| **Audit Log** | Append-only, HMAC-SHA256 hash-chained, MongoDB-backed · Cryptographically verifiable integrity · 2-year default retention |
| **Alerting & Webhooks** | Rule-based and composite-scored alerts · SIEM export · HMAC-signed outbound webhook delivery with replay protection |
| **Compliance Reporting** | GDPR · SOC 2 · HIPAA report generation · PDF output with digital signing · ClickHouse-backed analytics |
| **DLP** | Real-time PII, credential, and financial data detection · Regex + entropy scoring · Per-org `monitor` / `block` mode |
| **Admin Dashboard** | Angular 19+ · Real-time SSE audit stream · Policy rule builder · SCIM saga timeline · Circuit breaker status |

---

## 📊 Performance SLOs

All targets verified by k6 load tests (Phase 8). A release does not ship unless every SLO is green.

| Operation | p50 | p99 | p999 | Throughput |
|---|---|---|---|---|
| `POST /oauth/token` | 40ms | 150ms | 400ms | 2,000 req/s |
| `POST /v1/policy/evaluate` (cache miss) | 5ms | 30ms | 80ms | 10,000 req/s |
| `POST /v1/policy/evaluate` (Redis cached) | 1ms | 5ms | 15ms | 10,000 req/s |
| SDK local cache hit | <1ms | <1ms | <1ms | unlimited |
| `GET /audit/events` (paginated) | 20ms | 100ms | 250ms | 1,000 req/s |
| Kafka event → audit DB insert | — | 2s | 5s | 50,000 events/s |
| `POST /v1/events/ingest` | 10ms | 50ms | 150ms | 20,000 req/s |
| `GET /v1/scim/v2/Users` | 30ms | 500ms | 1,500ms | 500 req/s |
| Connector registry lookup (Redis) | 1ms | 5ms | 15ms | — |
| DLP async scan | — | 500ms | 2s | — |
| Compliance report generation | — | 30s | 120s | 10 concurrent |

---

## 🏗 Architecture

### Design Principles

**Fail closed** — Policy unavailable → SDK denies after 60s TTL. IAM unavailable → reject all logins. DLP sync-block unavailable → reject events (per-org opt-in).

**Exactly-once audit** — Every state-changing operation produces exactly one audit event via the Transactional Outbox. The business row and the outbox record commit atomically in the same PostgreSQL transaction; a separate relay publishes to Kafka asynchronously.

**Zero cross-tenant leakage** — PostgreSQL Row-Level Security enforced at the DB layer. `NULLIF(current_setting('app.org_id', true), '')::UUID` handles NULL and empty string uniformly. No cross-tenant query is possible at the application layer.

**Immutable audit trail** — Append-only MongoDB with per-org HMAC-SHA256 hash chaining. Batch sequence reservation via atomic `findOneAndUpdate $inc` for throughput. Tampering is detectable by `GET /audit/integrity`.

**Secret rotation without downtime** — JWT signing uses `kid` header. Multiple valid keys coexist during rotation. Same pattern for MFA encryption keys and connector API keys.

**mTLS between services** — All internal service-to-service calls use mutual TLS. `scripts/gen-mtls-certs.sh` generates per-service certificates. No service accepts internal traffic without a valid client certificate.

### System Topology

```
                         ┌───────────────────────────────────────────┐
                         │              Connected Apps                │
                         │  (OpenGuard SDK embedded in each service)  │
                         └──────────────┬────────────────────────────┘
                                        │  HTTPS + mTLS (API key)
                                        ▼
                         ┌──────────────────────────────┐
                         │        Control Plane          │
                         │  (reverse proxy + rate limit  │
                         │   + circuit breakers)         │
                         └──┬────┬────┬────┬────┬───────┘
                            │    │    │    │    │    (mTLS)
               ┌────────────┘    │    │    │    └──────────────┐
               ▼                 ▼    ▼    ▼                   ▼
          ┌────────┐      ┌──────────┐ ┌────────┐      ┌──────────────┐
          │  IAM   │      │  Policy  │ │ Audit  │      │  Connector   │
          │        │      │  Engine  │ │  Log   │      │  Registry    │
          └───┬────┘      └────┬─────┘ └───┬────┘      └──────┬───────┘
              │                │           │                   │
              └────────────────┴───────────┴───────────────────┘
                                        │
                               ┌────────▼────────┐
                               │      Kafka       │
                               │   (11 topics,    │
                               │  3 brokers,      │
                               │  replication=3)  │
                               └────┬────────┬────┘
                                    │        │
                         ┌──────────▼──┐  ┌──▼──────────┐
                         │   Threat    │  │  Alerting   │
                         │  Detection  │  │  & Webhooks │
                         └─────────────┘  └─────────────┘
                                    │
                         ┌──────────▼──────────┐
                         │  Compliance /        │
                         │  DLP / Analytics     │
                         │  (ClickHouse)        │
                         └─────────────────────┘
```

### Kafka Topics

| Topic | Partitions | Retention | Purpose |
|---|---|---|---|
| `auth.events` | 12 | 7 days | IAM login/logout events |
| `policy.changes` | 6 | 7 days | Policy mutation events |
| `data.access` | 24 | 3 days | Connector-pushed access events |
| `threat.alerts` | 12 | 30 days | Threat detector output |
| `audit.trail` | 24 | ∞ | Immutable cryptographic audit log |
| `connector.events` | 24 | 3 days | Normalized connector events |
| `webhook.delivery` | 12 | 1 day | Outbound webhook queue |
| `webhook.dlq` | 3 | ∞ | Failed webhook dead-letter |
| `notifications.outbound` | 6 | 1 day | Alert notifications |
| `saga.orchestration` | 12 | 7 days | SCIM choreography sagas |
| `outbox.dlq` | 3 | ∞ | Outbox relay dead-letter |

### Multi-Tenancy

Three isolation tiers:

| Tier | Mechanism | Target |
|---|---|---|
| **Shared** (default) | PostgreSQL RLS on shared tables | Free / SMB |
| **Schema** | Dedicated PostgreSQL schema per org | Mid-market |
| **Shard** | Dedicated PostgreSQL instance per org | Enterprise / regulated |

---

## 📦 Services

| Service | Port | Module | Responsibility |
|---|---|---|---|
| `control-plane` | 8080 | `services/control-plane` | Org lifecycle, reverse proxy, rate limiting, circuit breakers |
| `iam` | 8081 | `services/iam` | Auth, OIDC/SAML, SCIM 2.0, MFA (TOTP/WebAuthn), JWT, sessions |
| `policy` | 8082 | `services/policy` | RBAC evaluation, Redis cache, fail-closed, eval logging |
| `threat` | 8083 | `services/threat` | Streaming anomaly scoring, Redis-backed detectors |
| `audit` | 8084 | `services/audit` | Kafka consumer, MongoDB bulk write, hash chain, CQRS read/write |
| `alerting` | 8085 | `services/alerting` | Alert lifecycle saga, SIEM webhooks, MTTR tracking |
| `connector-registry` | 8090 | `services/connector-registry` | App registration, API key lifecycle, PBKDF2 hash store |
| `webhook-delivery` | 8091 | `services/webhook-delivery` | HMAC-signed delivery, retry, DLQ, SSRF guard |
| `compliance` | 8092 | `services/compliance` | ClickHouse queries, PDF report generation, report signing |
| `dlp` | 8093 | `services/dlp` | PII/credential/financial scanning, entropy detection, findings store |

---

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose v2
- Go 1.22+
- Node.js 20+ & npm
- `make`

### 1. Clone & Bootstrap

```bash
git clone https://github.com/openguard/openguard.git
cd openguard

# Copy and configure environment
cp .env.example .env
# Edit .env — at minimum set all *_SECRET and *_KEY values

# Generate mTLS certificates for all services
make certs
```

### 2. Start Infrastructure

```bash
# Start all infrastructure (PostgreSQL, MongoDB replica set,
# Kafka 3-broker cluster, Redis, ClickHouse, Prometheus, Grafana)
cd infra/docker
docker compose up -d

# Verify all containers are healthy
docker compose ps

# Initialize Kafka topics
make create-topics

# Download MaxMind GeoLite2 database (required for Impossible Travel detector)
# Set MAXMIND_LICENSE_KEY in .env first
make geo-db
```

### 3. Run Migrations & Seed

```bash
# Run all service migrations (uses distributed Redis lock — safe to run concurrently)
make migrate

# Seed dev data (creates Acme Corp org + admin user + example connector)
make seed
```

Default seed credentials:
- **Admin:** `admin@acme.example` / `changeme123!`
- **Org:** `acme-corp`
- **Connector API Key:** printed to stdout once — store it securely

### 4. Start Backend Services

```bash
# Start all services (builds from source)
make dev

# Or start individually
go run ./services/iam/...
go run ./services/policy/...
# etc.
```

### 5. Start the Dashboard

```bash
cd web
npm install
npm start
```

Open [http://localhost:4200](http://localhost:4200) and log in with the seeded admin credentials.

### 6. Run the Example App

```bash
cd examples/task-management-app

# Backend (Go — uses OpenGuard SDK)
cd backend && go run . &

# Frontend (Next.js)
npm install && npm run dev
```

Open [http://localhost:3000](http://localhost:3000). The example app demonstrates full OAuth flow, SDK-based policy enforcement, and event ingestion.

### 7. Verify Everything Is Working

```bash
# Run the full acceptance test suite
make test-acceptance

# Check individual SLOs
curl http://localhost:8082/health       # Policy service
curl http://localhost:8084/health       # Audit service
curl http://localhost:9090              # Prometheus
open http://localhost:3000              # Grafana (admin/admin)
```

---

## 🔧 Configuration

All configuration is via environment variables. Copy `.env.example` to `.env` and fill in all required values. Services will refuse to start if required variables are missing.

### Core Variables (excerpt)

```dotenv
# Environment
APP_ENV=production                     # development | staging | production
LOG_LEVEL=info                         # debug | info | warn | error
LOG_FORMAT=json

# Control Plane
CONTROL_PLANE_PORT=8080
CONTROL_PLANE_API_KEY_SALT=<32-byte-hex>
CONTROL_PLANE_WEBHOOK_SIGNING_SECRET=<32-byte-hex>
CONTROL_PLANE_POLICY_CACHE_TTL_SECONDS=60

# IAM
IAM_PORT=8081
IAM_JWT_KEYS_JSON=[{"kid":"k1","secret":"<min-32-chars>","algorithm":"HS256","status":"active"}]
IAM_JWT_EXPIRY_SECONDS=900
IAM_REFRESH_TOKEN_EXPIRY_DAYS=30
IAM_MFA_ENCRYPTION_KEY_JSON=[{"kid":"mk1","key":"<base64-32-bytes>","status":"active"}]
IAM_WEBAUTHN_RPID=openguard.example.com
IAM_BCRYPT_WORKER_COUNT=8             # Default: 2 × NumCPU

# Policy
POLICY_PORT=8082
POLICY_CACHE_TTL_SECONDS=60           # Must match SDK_POLICY_CACHE_TTL_SECONDS

# Threat Detection
THREAT_GEO_CHANGE_THRESHOLD_KM=500
THREAT_MAX_FAILED_LOGINS=10
MAXMIND_LICENSE_KEY=<your-key>        # Free at maxmind.com

# Audit
AUDIT_HASH_CHAIN_SECRET=<32-byte-hex>
AUDIT_RETENTION_DAYS=730
AUDIT_BULK_INSERT_MAX_DOCS=500

# Storage
DATABASE_URL=postgres://openguard_app:password@localhost:5432/openguard?sslmode=require
REDIS_URL=redis://:password@localhost:6379/0
MONGO_URI_PRIMARY=mongodb://localhost:27017,localhost:27018,localhost:27019/openguard?replicaSet=rs0
CLICKHOUSE_DSN=clickhouse://localhost:9000/openguard

# Kafka
KAFKA_BROKERS=broker1:9092,broker2:9092,broker3:9092
```

See [`.env.example`](.env.example) for the complete list (60+ variables with descriptions).

---

## 🔌 SDK

The OpenGuard SDK embeds into your application and handles policy decisions, local caching, and event ingestion.

### Installation

```bash
go get github.com/openguard/sdk
```

### Basic Usage

```go
package main

import (
    "context"
    "log"

    "github.com/openguard/sdk"
)

func main() {
    client, err := sdk.NewClient(sdk.Config{
        BaseURL:        "https://api.openguard.example.com",
        APIKey:         os.Getenv("OPENGUARD_API_KEY"),
        PolicyCacheTTL: 60 * time.Second, // local cache; denies after TTL on outage
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Policy evaluation — cache-first, then remote, fail-closed
    allowed, err := client.Allow(ctx, sdk.EvaluateRequest{
        SubjectID: "user:abc123",
        Action:    "documents:read",
        Resource:  "document:reports/*",
    })
    if err != nil || !allowed {
        http.Error(w, "Forbidden", http.StatusForbidden)
        return
    }

    // Push an audit event
    client.PushEvent(ctx, sdk.Event{
        Type:   "document.read",
        Actor:  "user:abc123",
        Target: "document:reports/q3-summary",
    })
}
```

### SDK Middleware (HTTP)

```go
// Wrap any http.Handler with OpenGuard policy enforcement
router.Use(client.Middleware(sdk.MiddlewareConfig{
    Action:   "api:access",
    Resource: "endpoint:{method}:{path}",
    OnDeny:   sdk.RespondForbidden,
}))
```

### Fail-Closed Behavior

The SDK maintains a local in-memory cache (default TTL: 60 seconds). If the OpenGuard control plane is unreachable:
- Cached decisions continue to be served until TTL expires
- After TTL: **all evaluations return `deny`**
- When control plane recovers: cache is refreshed on next evaluation

---

## 🔐 Authentication & SCIM

### OAuth 2.0 / OIDC

OpenGuard acts as an OIDC identity provider for connected applications.

```
# Authorization Code Flow
GET /auth/authorize?client_id=my-app&redirect_uri=https://app.example.com/callback&scope=openid profile

# Token Exchange
POST /oauth/token
Content-Type: application/x-www-form-urlencoded
grant_type=authorization_code&code=<code>&client_id=my-app&client_secret=<secret>

# OIDC Discovery
GET /.well-known/openid-configuration
```

### SCIM 2.0 Provisioning

OpenGuard implements SCIM 2.0 for automated user provisioning from your Identity Provider.

```
Base URL: https://api.openguard.example.com/scim/v2/
Authentication: Bearer <scim-token>   # org-scoped, set in IAM_SCIM_TOKENS_JSON
```

Supported endpoints: `Users`, `Groups`, `/Me`, `ServiceProviderConfig`, `Schemas`

Provisioning triggers a choreography-based saga across services:

```
SCIM POST /Users
  → user.created (status=initializing) → Kafka
    → Policy: assign default policies → policy.assigned
      → Threat: initialize baseline profile → threat.baseline.init
        → Alerting: configure preferences → alert.prefs.init
          → IAM: UPDATE users SET status='active' → saga.completed
```

### Multi-Factor Authentication

TOTP and WebAuthn (passkeys) are both supported and manageable per-user via the admin dashboard or the management API.

```bash
# TOTP setup (returns QR code URL)
GET /mgmt/users/mfa/totp/setup   (Bearer JWT)

# Enable TOTP (requires valid OTP to confirm setup)
POST /mgmt/users/mfa/totp/enable  { "code": "123456" }

# WebAuthn registration challenge
POST /mgmt/users/mfa/webauthn/register/begin
POST /mgmt/users/mfa/webauthn/register/finish
```

---

## 📋 Policy Engine

Policies are stored as flexible JSONB logic expressions and evaluated in real time.

### Policy Schema

```json
{
  "name": "Engineering Read-Only",
  "description": "Allow engineers to read all documents",
  "logic": {
    "type": "rbac",
    "subjects": ["group:engineering"],
    "actions":  ["documents:read", "reports:read"],
    "resources": ["document:*", "report:*"]
  }
}
```

Supported logic types: `rbac`, `allow_all`, `deny_all`. Logic expressions are extensible — new types are added without schema migrations.

### Evaluate via SDK (recommended)

```go
allowed, _ := client.Allow(ctx, sdk.EvaluateRequest{
    SubjectID:  "user:abc123",
    UserGroups: []string{"group:engineering"},
    Action:     "documents:read",
    Resource:   "document:reports/q3",
})
```

### Evaluate via API

```bash
POST /v1/policy/evaluate
Authorization: Bearer <api-key>

{
  "org_id":     "11111111-...",
  "subject_id": "user:abc123",
  "user_groups": ["group:engineering"],
  "action":     "documents:read",
  "resource":   "document:reports/q3"
}

# Response
{
  "effect":              "allow",
  "matched_policy_ids":  ["pppp-..."],
  "cache_hit":           "redis",
  "latency_ms":          2
}
```

---

## 🔍 Audit Log

Every state-changing operation — in OpenGuard itself and in connected applications — produces exactly one audit event. The audit trail is cryptographically verifiable.

### Querying Events

```bash
# List events (cursor-paginated, reads from MongoDB secondary)
GET /audit/events?org_id=<id>&type=auth.login&cursor=<cursor>&limit=100

# Verify hash chain integrity
GET /audit/integrity?org_id=<id>
# → { "ok": true, "verified_count": 142857, "last_seq": 142857 }
# If a document was deleted or tampered: "ok": false, "gap_at_seq": 98765

# Trigger async export
POST /audit/export  { "format": "json", "from": "2025-01-01", "to": "2025-12-31" }
GET  /audit/export/:job_id
GET  /audit/export/:job_id/download
```

### Event Ingestion from Connected Apps

```bash
POST /v1/events/ingest
X-OpenGuard-Key: ogk_<your-connector-api-key>

{
  "event_id":    "uuid-unique-per-event",   # deduplicated by unique index
  "type":        "task.created",
  "actor_id":    "user:abc123",
  "actor_type":  "user",
  "target":      "task:xyz789",
  "occurred_at": "2025-06-01T14:32:00Z",
  "metadata":    { "task_name": "Q3 Review" }
}
```

---

## 🛡 Threat Detection

The threat service continuously monitors the Kafka event stream and scores anomalies in real time.

| Detector | Signal | Default Threshold | Risk Score |
|---|---|---|---|
| Brute Force | Failed logins for same email | 10 in 60 min | 0.8 |
| Impossible Travel | Login from 2 IPs > 500km apart within 1hr | Physical impossibility | 0.9 |
| Off-Hours Access | Login outside 06:00–22:00 org time | 3+ consecutive deviations | 0.5 |
| Data Exfiltration | Access count > 3σ above org baseline | Statistical anomaly | 0.7 |
| Account Takeover | New device fingerprint within 24hr of password change | New device + recent change | 0.7 |
| Privilege Escalation | Role grant within 60min of login | Login → immediate grant | 0.9 |

Composite score ≥ 0.5 → Alert. ≥ 0.8 → HIGH. ≥ 0.95 → CRITICAL.

---

## 📈 Observability

### Metrics (Prometheus)

All services expose `GET /metrics` in Prometheus format. Key metrics:

```
openguard_policy_evaluations_total{effect,cache_hit}
openguard_policy_evaluation_duration_seconds{quantile}
openguard_auth_login_attempts_total{outcome}
openguard_audit_events_ingested_total{source}
openguard_kafka_outbox_lag_seconds
openguard_circuit_breaker_state{breaker,state}
openguard_threat_alerts_total{severity,detector}
```

### Dashboards

Import the pre-built Grafana dashboards from `infra/monitoring/dashboards/`:
- **OpenGuard Overview** — SLO burn rates, request volumes, error rates
- **Policy Engine** — evaluation throughput, cache hit ratio, p99 latency
- **Audit Pipeline** — Kafka consumer lag, bulk write latency, hash chain progress
- **Threat Intelligence** — alert volume by detector and severity, MTTR trends
- **Infrastructure Health** — outbox lag, circuit breaker states, DB connections

Access Grafana at [http://localhost:3000](http://localhost:3000) (default: `admin` / `admin`).

### Distributed Tracing

All services export OpenTelemetry traces. Configure `OTEL_EXPORTER_OTLP_ENDPOINT` to point at your Jaeger or Tempo instance.

---

## 🚢 Deployment

### Docker Compose (development/staging)

```bash
cd infra/docker
docker compose up -d --build
```

### Kubernetes / Helm

```bash
helm repo add openguard https://charts.openguard.io
helm install openguard openguard/openguard \
  --namespace openguard \
  --create-namespace \
  --values infra/k8s/values.production.yaml
```

Key Helm values:

```yaml
iam:
  replicaCount: 6
  resources:
    requests: { cpu: "2", memory: "1Gi" }
  hpa:
    targetCPUUtilizationPercentage: 60  # Bcrypt CPU saturation target

policy:
  replicaCount: 4
  resources:
    requests: { cpu: "1", memory: "512Mi" }

kafka:
  brokers: 3
  replicationFactor: 3
  topics:
    auditTrail:
      retention: "-1"   # Infinite retention
```

### Generating mTLS Certificates

```bash
# Generates CA + per-service certs in ./certs/
make certs

# Rotate certs (services reload on SIGHUP without downtime)
make certs-rotate
```

---

## 🔄 Secret Rotation

All secrets support zero-downtime rotation.

### JWT Signing Keys

Add the new key to `IAM_JWT_KEYS_JSON` with `"status": "active"` alongside the old key (set old to `"status": "rotating"`). Tokens signed with either key will be accepted. Remove the old key after all existing tokens have expired.

```json
[
  {"kid": "k2", "secret": "<new-secret>", "algorithm": "HS256", "status": "active"},
  {"kid": "k1", "secret": "<old-secret>", "algorithm": "HS256", "status": "rotating"}
]
```

### MFA Encryption Keys

Same pattern via `IAM_MFA_ENCRYPTION_KEY_JSON`. Existing TOTP secrets remain readable during rotation.

### Connector API Keys

Regenerate via the dashboard or `POST /mgmt/connectors/:id/rotate-key`. The previous key remains valid for `CONNECTOR_KEY_ROTATION_GRACE_MINUTES` (default: 60) to allow zero-downtime rollout on the connected app side.

---

## 🧪 Testing

```bash
# Unit tests (race detector enabled)
go test ./... -race -count=1

# Integration tests (requires running infrastructure)
make test-integration

# Full acceptance criteria suite (45-step scenario)
make test-acceptance

# k6 load tests (verifies all SLOs)
make load-test

# Frontend (Jasmine + Playwright E2E)
cd web && npm test
cd web && npm run e2e

# Vulnerability scan
govulncheck ./...

# Lint
golangci-lint run ./...
npx prettier --check .
sqlfluff lint .
```

---

## 🆘 Disaster Recovery

| Component | RPO | RTO |
|---|---|---|
| PostgreSQL (IAM, Policy) | 5 minutes | 30 minutes |
| MongoDB (Audit Log) | 1 hour | 2 hours |
| ClickHouse (Compliance) | 24 hours | 4 hours |
| Redis (Cache, Blocklist) | 0 (ephemeral) | 5 minutes |
| Kafka (Event Bus) | 0 (replicated) | 15 minutes |

### Audit Chain Verification Post-Restore

```bash
# Verify hash chain integrity after any restore
scripts/verify-audit-chain.sh --org-id <org-id> --from 2025-01-01
# Output: verified 142,857 events, last_seq=142857, integrity=ok
```

### Chaos Drills

- **Quarterly:** Redis Sentinel failover — verify blocklist survives
- **Monthly:** PostgreSQL restore to staging — run full acceptance suite
- **Bi-annual:** Full region failover drill against the 30-minute RTO

---

## 📁 Repository Layout

```
openguard/
├── ai-spec/                    # Architecture specifications & AI agent rules
│   ├── be_open_guard/          # Backend spec (22 documents)
│   ├── fe_open_guard/          # Frontend spec (21 documents)
│   └── project.md              # Master index (start here)
├── services/                   # Go microservices
│   ├── control-plane/          # Reverse proxy + rate limiting
│   ├── iam/                    # Identity & Access Management
│   ├── policy/                 # Policy engine
│   ├── audit/                  # Audit log (Kafka → MongoDB)
│   ├── threat/                 # Threat detection
│   ├── alerting/               # Alerting & SIEM
│   ├── connector-registry/     # App registration & API keys
│   ├── webhook-delivery/       # Signed outbound webhooks
│   ├── compliance/             # Reports & analytics
│   └── dlp/                    # Content scanning
├── shared/                     # Shared Go libraries
│   ├── crypto/                 # JWT keyring, AES encryption
│   ├── kafka/                  # Publisher, Outbox Writer, Relay
│   ├── middleware/             # APIKeyAuth, SCIMAuth, SecurityHeaders, SSRFGuard
│   ├── resilience/             # Circuit breaker wrapper
│   ├── rls/                    # PostgreSQL RLS session helpers
│   └── telemetry/              # Structured logger
├── sdk/                        # Go SDK for connected applications
├── web/                        # Angular 19+ admin dashboard
│   └── src/app/
│       ├── core/               # Services, guards, interceptors, layout
│       ├── features/           # Auth screens
│       ├── policies/           # Policy CRUD + rule builder
│       ├── audit-logs/         # Real-time audit stream
│       ├── connectors/         # Connector registration
│       ├── users/              # User management + SCIM saga timeline
│       ├── threat/             # Alert list + detector cards
│       ├── compliance/         # Report wizard + PDF preview
│       └── dlp/                # DLP policy editor + findings
├── infra/
│   ├── docker/                 # Docker Compose (dev/staging)
│   ├── k8s/                    # Helm charts
│   ├── kafka/                  # Topic configuration
│   └── monitoring/             # Prometheus, Grafana dashboards, Promtail
├── scripts/
│   ├── gen-mtls-certs.sh       # mTLS certificate generation
│   ├── create-topics.sh        # Kafka topic bootstrap
│   ├── download-geolite2.sh    # MaxMind DB download
│   └── verify-audit-chain.sh  # Post-restore integrity check
├── examples/
│   └── task-management-app/    # Next.js + Go example using OpenGuard SDK
├── docs/
│   └── openapi/                # OpenAPI 3.1 specifications per service
├── go.work                     # Go workspace (all modules)
└── Makefile                    # dev, test, lint, build, migrate, seed, certs, load-test
```

---

## 🤝 Contributing

We welcome contributions. Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) and the [backend spec](ai-spec/be_open_guard/00-code-quality-standards.md) before opening a PR. All code must pass CI — lint, format, unit tests, and integration tests.

For AI-assisted development, start with [`ai-spec/project.md`](ai-spec/project.md).

---

## 📄 License

OpenGuard is open-source software licensed under the [MIT License](LICENSE).

---

<div align="center">

**OpenGuard — Enterprise Grade Security, Open Source Freedom.**

[Documentation](docs/) · [OpenAPI Specs](docs/openapi/) · [Changelog](CHANGELOG.md) · [Security Policy](SECURITY.md)

</div>