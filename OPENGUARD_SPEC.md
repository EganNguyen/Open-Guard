# OpenGuard — Implementation Specification

> **For the implementing LLM:** This document is a complete, phase-gated specification. Read the entire file before writing a single line of code. Each phase builds on the last. Do not skip ahead. Follow the contracts exactly — data models, API signatures, Kafka topic names, and environment variable names are canonical and must not be renamed without explicit justification. Where a section says "implement X", produce working, compilable code with tests. Where it says "scaffold X", produce the file and directory structure with stubs and TODO markers. Assume Go 1.22+, Next.js 14 (App Router), and Kafka 3.6+.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Repository Layout](#2-repository-layout)
3. [Shared Contracts](#3-shared-contracts)
4. [Environment & Configuration](#4-environment--configuration)
5. [Phase 1 — Foundation](#5-phase-1--foundation)
6. [Phase 2 — Policy Engine](#6-phase-2--policy-engine)
7. [Phase 3 — Event Bus & Audit Log](#7-phase-3--event-bus--audit-log)
8. [Phase 4 — Threat Detection & Alerting](#8-phase-4--threat-detection--alerting)
9. [Phase 5 — Compliance & Analytics](#9-phase-5--compliance--analytics)
10. [Phase 6 — Frontend (Next.js)](#10-phase-6--frontend-nextjs)
11. [Phase 7 — Infra, CI/CD & Observability](#11-phase-7--infra-cicd--observability)
12. [Phase 8 — Hardening & Documentation](#12-phase-8--hardening--documentation)
13. [Cross-Cutting Concerns](#13-cross-cutting-concerns)
14. [Acceptance Criteria (Full System)](#14-acceptance-criteria-full-system)

---

## 1. Project Overview

### 1.1 What is OpenGuard?

OpenGuard is an open-source, self-hostable **organization security platform** inspired by Atlassian Guard. It provides:

- **Identity & Access Management (IAM):** SSO (SAML 2.0 / OIDC), SCIM provisioning, MFA enforcement, API token lifecycle.
- **Policy Engine:** Data security rules — export restrictions, anonymous access controls, role-based access control (RBAC).
- **Threat Detection:** Real-time anomaly scoring on login and data-access event streams.
- **Audit Log:** Immutable, queryable record of all admin, user, and system actions.
- **Alerting:** Rule-based and ML-scored alerts with SIEM webhook export.
- **Compliance Reporting:** GDPR, SOC 2, HIPAA report generation.
- **Admin Dashboard:** Next.js web console for all of the above.

### 1.2 Design Principles

| Principle | Implication |
|-----------|-------------|
| Event-driven | All side effects flow through Kafka. Services never call each other's write paths synchronously. |
| Immutable audit | The audit log is append-only. No UPDATE or DELETE on `audit_events`. |
| Least privilege | Every service has its own DB user with only the permissions it needs. |
| Self-hostable | Docker Compose must be the only dependency to run locally. Kubernetes is optional. |
| Open API | Every service exposes an OpenAPI 3.1 spec at `/openapi.json`. |
| Testable | Every package ships with unit tests. Every service ships with at least one integration test. |

### 1.3 Technology Choices (Canonical)

| Concern | Technology | Version |
|---------|-----------|---------|
| Backend services | Go | 1.22+ |
| HTTP framework | `net/http` + `chi` router | chi v5 |
| gRPC / protobuf | `google.golang.org/grpc` | v1.64+ |
| Frontend | Next.js (App Router) | 14 |
| Frontend UI | shadcn/ui + Tailwind CSS | latest |
| Relational DB | PostgreSQL | 16 |
| Document store | MongoDB | 7 |
| Cache / sessions | Redis | 7 |
| Event bus | Apache Kafka | 3.6 |
| Analytics DB | ClickHouse | 24 |
| Container runtime | Docker + Docker Compose | Compose v2 |
| Orchestration (opt) | Kubernetes + Helm | k8s 1.29 |
| CI/CD | GitHub Actions | — |
| Metrics | Prometheus + Grafana | latest |
| Distributed tracing | OpenTelemetry → Jaeger | — |
| Secret management | environment variables (dev); Vault (prod) | — |

---

## 2. Repository Layout

Produce this exact directory tree. Every directory listed must exist (create a `.gitkeep` if empty).

```
openguard/
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── services/
│   ├── gateway/          # API Gateway (Go)
│   ├── iam/              # Identity & Access Management (Go)
│   ├── policy/           # Policy Engine (Go)
│   ├── threat/           # Threat Detection (Go)
│   ├── audit/            # Audit Log (Go)
│   ├── alerting/         # Alert Engine + SIEM export (Go)
│   └── compliance/       # Compliance Reporter (Go)
├── web/                  # Next.js 14 frontend
│   ├── app/
│   ├── components/
│   ├── lib/
│   └── public/
├── proto/                # Protobuf definitions (shared)
│   ├── iam/
│   ├── policy/
│   ├── audit/
│   └── threat/
├── infra/
│   ├── docker/
│   │   ├── docker-compose.yml
│   │   └── docker-compose.dev.yml
│   ├── k8s/
│   │   └── helm/
│   │       └── openguard/
│   ├── kafka/
│   │   └── topics.json
│   └── monitoring/
│       ├── prometheus.yml
│       └── grafana/
│           └── dashboards/
├── docs/
│   ├── architecture.md
│   ├── contributing.md
│   └── api/
├── scripts/
│   ├── seed.sh
│   └── migrate.sh
├── go.work                # Go workspace
├── .env.example
├── Makefile
└── README.md
```

### 2.1 Go Workspace

`go.work` must declare all service modules:

```
go 1.22

use (
    ./services/gateway
    ./services/iam
    ./services/policy
    ./services/threat
    ./services/audit
    ./services/alerting
    ./services/compliance
)
```

Each service directory is its own Go module named `github.com/openguard/<service>`.

### 2.2 Shared Internal Packages

Create a `pkg/` directory **inside each service** for service-specific packages. Create a **top-level** `shared/` Go module (`github.com/openguard/shared`) for packages used by ≥2 services:

```
shared/
├── go.mod
├── kafka/         # producer/consumer helpers
├── middleware/    # HTTP middleware (logging, tracing, auth)
├── models/        # Canonical event structs (used in Kafka payloads)
├── validator/     # Request validation helpers
└── telemetry/     # OpenTelemetry setup
```

Add `./shared` to `go.work`.

---

## 3. Shared Contracts

These data structures are **canonical**. Every service that touches these types must import from `github.com/openguard/shared/models`. Do not redefine them locally.

### 3.1 Kafka Event Envelope

```go
// shared/models/event.go
package models

import "time"

// EventEnvelope is the standard wrapper for all Kafka messages.
// Every message on every topic must be a JSON-serialized EventEnvelope.
type EventEnvelope struct {
    ID          string          `json:"id"`           // UUIDv4
    Type        string          `json:"type"`         // e.g. "auth.login.success"
    OrgID       string          `json:"org_id"`
    ActorID     string          `json:"actor_id"`     // user or system ID
    ActorType   string          `json:"actor_type"`   // "user" | "service" | "system"
    OccurredAt  time.Time       `json:"occurred_at"`
    Source      string          `json:"source"`       // originating service name
    TraceID     string          `json:"trace_id"`     // OpenTelemetry trace ID
    SchemaVer   string          `json:"schema_ver"`   // "1.0"
    Payload     json.RawMessage `json:"payload"`      // event-specific data
}
```

### 3.2 Kafka Topic Registry

These topic names are canonical. Do not hardcode strings in service code — import from `shared/kafka`.

```go
// shared/kafka/topics.go
package kafka

const (
    TopicAuthEvents       = "auth.events"
    TopicPolicyChanges    = "policy.changes"
    TopicDataAccess       = "data.access"
    TopicThreatAlerts     = "threat.alerts"
    TopicAuditTrail       = "audit.trail"
    TopicNotificationsOut = "notifications.outbound"
)
```

### 3.3 Canonical User Model

```go
// shared/models/user.go
package models

import "time"

type User struct {
    ID           string     `json:"id" db:"id"`             // UUIDv4
    OrgID        string     `json:"org_id" db:"org_id"`
    Email        string     `json:"email" db:"email"`
    DisplayName  string     `json:"display_name" db:"display_name"`
    Status       UserStatus `json:"status" db:"status"`
    MFAEnabled   bool       `json:"mfa_enabled" db:"mfa_enabled"`
    SCIMExternal string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
    CreatedAt    time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
    DeletedAt    *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type UserStatus string

const (
    UserStatusActive      UserStatus = "active"
    UserStatusSuspended   UserStatus = "suspended"
    UserStatusDeprovisioned UserStatus = "deprovisioned"
)
```

### 3.4 Canonical Policy Model

```go
// shared/models/policy.go
package models

import "time"

type Policy struct {
    ID          string          `json:"id" db:"id"`
    OrgID       string          `json:"org_id" db:"org_id"`
    Name        string          `json:"name" db:"name"`
    Description string          `json:"description" db:"description"`
    Type        PolicyType      `json:"type" db:"type"`
    Rules       json.RawMessage `json:"rules" db:"rules"`    // JSONB
    Enabled     bool            `json:"enabled" db:"enabled"`
    CreatedBy   string          `json:"created_by" db:"created_by"`
    CreatedAt   time.Time       `json:"created_at" db:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

type PolicyType string

const (
    PolicyTypeDataExport    PolicyType = "data_export"
    PolicyTypeAnonAccess    PolicyType = "anon_access"
    PolicyTypeIPAllowlist   PolicyType = "ip_allowlist"
    PolicyTypeSessionLimit  PolicyType = "session_limit"
)
```

### 3.5 Standard HTTP Error Response

All services return errors in this shape:

```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "User with id 'abc' not found",
    "request_id": "req_01j..."
  }
}
```

```go
// shared/models/errors.go
package models

type APIError struct {
    Error APIErrorBody `json:"error"`
}

type APIErrorBody struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    RequestID string `json:"request_id"`
}
```

---

## 4. Environment & Configuration

### 4.1 `.env.example` (canonical)

Every service reads its configuration from environment variables. No service may have a config file in production. All variables below must be present in `.env.example` with safe placeholder values.

```dotenv
# ── App ──────────────────────────────────────────────
APP_ENV=development          # development | staging | production
LOG_LEVEL=info               # debug | info | warn | error

# ── Gateway ──────────────────────────────────────────
GATEWAY_PORT=8080
GATEWAY_JWT_SECRET=change-me-in-production
GATEWAY_JWT_EXPIRY=3600      # seconds

# ── IAM Service ──────────────────────────────────────
IAM_PORT=8081
IAM_SAML_ENTITY_ID=https://openguard.example.com
IAM_SAML_IDP_METADATA_URL=https://idp.example.com/metadata
IAM_OIDC_ISSUER=https://accounts.example.com
IAM_OIDC_CLIENT_ID=openguard
IAM_OIDC_CLIENT_SECRET=change-me
IAM_SCIM_BEARER_TOKEN=change-me
IAM_MFA_TOTP_ISSUER=OpenGuard

# ── Policy Service ───────────────────────────────────
POLICY_PORT=8082

# ── Threat Detection ─────────────────────────────────
THREAT_PORT=8083
THREAT_ANOMALY_WINDOW_MINUTES=60
THREAT_MAX_FAILED_LOGINS=10
THREAT_GEO_CHANGE_THRESHOLD_KM=500

# ── Audit Service ────────────────────────────────────
AUDIT_PORT=8084
AUDIT_RETENTION_DAYS=730     # 2-year default

# ── Alerting Service ─────────────────────────────────
ALERTING_PORT=8085
ALERTING_SLACK_WEBHOOK_URL=
ALERTING_SMTP_HOST=smtp.example.com
ALERTING_SMTP_PORT=587
ALERTING_SMTP_USER=
ALERTING_SMTP_PASS=
ALERTING_SIEM_WEBHOOK_URL=

# ── Compliance Service ───────────────────────────────
COMPLIANCE_PORT=8086

# ── PostgreSQL ───────────────────────────────────────
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=openguard
POSTGRES_PASSWORD=change-me
POSTGRES_DB=openguard
POSTGRES_SSLMODE=disable

# ── MongoDB ──────────────────────────────────────────
MONGO_URI=mongodb://localhost:27017
MONGO_DB=openguard

# ── Redis ────────────────────────────────────────────
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# ── Kafka ────────────────────────────────────────────
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=openguard
KAFKA_GROUP_ID_AUDIT=audit-consumer-group
KAFKA_GROUP_ID_THREAT=threat-consumer-group
KAFKA_GROUP_ID_ALERTING=alerting-consumer-group
KAFKA_GROUP_ID_COMPLIANCE=compliance-consumer-group

# ── ClickHouse ───────────────────────────────────────
CLICKHOUSE_ADDR=localhost:9000
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=
CLICKHOUSE_DB=openguard

# ── OpenTelemetry ────────────────────────────────────
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SERVICE_NAME=openguard

# ── Frontend (Next.js) ───────────────────────────────
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me
```

### 4.2 Config Loading Pattern

Every Go service must use this pattern for config loading (implement in `pkg/config/config.go` within each service):

```go
package config

import (
    "fmt"
    "os"
    "strconv"
)

// Must returns the value of an environment variable or panics.
// Use for required variables that have no default.
func Must(key string) string {
    v := os.Getenv(key)
    if v == "" {
        panic(fmt.Sprintf("required environment variable %q is not set", key))
    }
    return v
}

// Default returns the value of an environment variable or a fallback.
func Default(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

// MustInt parses an integer env variable or panics.
func MustInt(key string) int {
    v := Must(key)
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env variable %q must be an integer, got %q", key, v))
    }
    return n
}
```

---

## 5. Phase 1 — Foundation

**Goal:** Running skeleton — API Gateway, IAM Service, PostgreSQL, Redis, and a working local dev environment. At the end of Phase 1, a user can register, log in with a password (TOTP later), and receive a JWT.

### 5.1 Prerequisites

Before writing any service code, produce:

1. `infra/docker/docker-compose.yml` — PostgreSQL, MongoDB, Redis, Kafka (via Bitnami), ClickHouse, Zookeeper.
2. `Makefile` with targets: `dev`, `test`, `lint`, `build`, `migrate`, `seed`.
3. `scripts/migrate.sh` — runs all `*.up.sql` migration files in `services/*/migrations/` in numeric order.
4. `.env.example` as defined in Section 4.1.

### 5.2 API Gateway (`services/gateway`)

**Purpose:** Single entry point. Validates JWTs, enforces rate limits, proxies to downstream services.

#### 5.2.1 Module setup

```
services/gateway/
├── go.mod              # module: github.com/openguard/gateway
├── main.go
├── pkg/
│   ├── config/
│   ├── proxy/          # reverse-proxy logic
│   ├── middleware/
│   │   ├── auth.go     # JWT validation
│   │   ├── ratelimit.go
│   │   └── logger.go
│   └── router/
│       └── router.go
└── Dockerfile
```

#### 5.2.2 Routing table

| Method | Path prefix | Upstream service | Auth required |
|--------|-------------|-----------------|---------------|
| `*` | `/api/v1/auth/*` | `iam:8081` | No |
| `*` | `/api/v1/users/*` | `iam:8081` | Yes |
| `*` | `/api/v1/scim/*` | `iam:8081` | SCIM bearer |
| `*` | `/api/v1/policies/*` | `policy:8082` | Yes |
| `*` | `/api/v1/threats/*` | `threat:8083` | Yes |
| `*` | `/api/v1/audit/*` | `audit:8084` | Yes |
| `*` | `/api/v1/alerts/*` | `alerting:8085` | Yes |
| `*` | `/api/v1/compliance/*` | `compliance:8086` | Yes |
| `GET` | `/health` | (gateway itself) | No |
| `GET` | `/metrics` | (gateway itself) | No |

#### 5.2.3 JWT middleware

- Validate `Authorization: Bearer <token>` on all routes marked "Yes".
- Secret from `GATEWAY_JWT_SECRET`.
- Inject `X-User-ID`, `X-Org-ID`, `X-User-Email` headers before proxying.
- On invalid token: return `401` with standard error body (code: `UNAUTHORIZED`).
- On missing token: return `401` (code: `MISSING_TOKEN`).

#### 5.2.4 Rate limiting

- Use Redis (`REDIS_ADDR`) with a sliding window.
- Default: 300 requests / minute per IP.
- Authenticated: 1000 requests / minute per `X-User-ID`.
- On limit exceeded: return `429` (code: `RATE_LIMIT_EXCEEDED`) with `Retry-After` header.

### 5.3 IAM Service (`services/iam`)

**Purpose:** User registration, login, SAML/OIDC SSO, SCIM provisioning, MFA (TOTP), API token management.

#### 5.3.1 Module setup

```
services/iam/
├── go.mod              # module: github.com/openguard/iam
├── main.go
├── migrations/
│   ├── 001_create_orgs.up.sql
│   ├── 002_create_users.up.sql
│   ├── 003_create_api_tokens.up.sql
│   ├── 004_create_sessions.up.sql
│   └── 005_create_mfa_configs.up.sql
├── pkg/
│   ├── config/
│   ├── db/             # PostgreSQL connection + query helpers
│   ├── handlers/
│   │   ├── auth.go
│   │   ├── users.go
│   │   ├── scim.go
│   │   ├── mfa.go
│   │   └── tokens.go
│   ├── service/
│   │   ├── auth.go
│   │   ├── user.go
│   │   ├── scim.go
│   │   └── mfa.go
│   ├── repository/
│   │   ├── user.go
│   │   ├── org.go
│   │   ├── session.go
│   │   └── apitoken.go
│   └── router/
└── Dockerfile
```

#### 5.3.2 Database schema (PostgreSQL)

Implement exactly these migrations:

**001 — orgs**
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE orgs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,
    plan         TEXT NOT NULL DEFAULT 'free',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**002 — users**
```sql
CREATE TABLE users (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email            TEXT NOT NULL,
    display_name     TEXT NOT NULL DEFAULT '',
    password_hash    TEXT,                          -- NULL for SSO-only users
    status           TEXT NOT NULL DEFAULT 'active',
    mfa_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    scim_external_id TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_users_org_id ON users(org_id);
CREATE INDEX idx_users_email  ON users(email);
```

**003 — api_tokens**
```sql
CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,   -- SHA-256 of the raw token
    prefix       TEXT NOT NULL,          -- first 8 chars of raw token for display
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**004 — sessions**
```sql
CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    ip_address   INET,
    user_agent   TEXT,
    country_code TEXT,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
```

**005 — mfa_configs**
```sql
CREATE TABLE mfa_configs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    type         TEXT NOT NULL DEFAULT 'totp',
    secret       TEXT NOT NULL,   -- encrypted TOTP secret
    backup_codes TEXT[] NOT NULL DEFAULT '{}',
    verified     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

#### 5.3.3 HTTP API

All paths are relative to the service root (gateway strips `/api/v1`).

**Auth endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/register` | Create org + admin user |
| `POST` | `/auth/login` | Password login → JWT + refresh token |
| `POST` | `/auth/logout` | Revoke session |
| `POST` | `/auth/refresh` | Refresh JWT |
| `POST` | `/auth/saml/callback` | SAML ACS endpoint |
| `GET`  | `/auth/oidc/login` | Initiate OIDC flow |
| `GET`  | `/auth/oidc/callback` | OIDC callback |
| `POST` | `/auth/mfa/enroll` | Begin TOTP enrollment |
| `POST` | `/auth/mfa/verify` | Complete TOTP enrollment |
| `POST` | `/auth/mfa/challenge` | Verify TOTP code at login |

**User endpoints (JWT required):**

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/users` | List org users (paginated) |
| `POST` | `/users` | Create user |
| `GET`  | `/users/:id` | Get user |
| `PATCH`| `/users/:id` | Update user |
| `DELETE`| `/users/:id` | Soft-delete (sets deleted_at) |
| `POST` | `/users/:id/suspend` | Set status=suspended |
| `POST` | `/users/:id/activate` | Set status=active |
| `GET`  | `/users/:id/sessions` | List active sessions |
| `DELETE`| `/users/:id/sessions/:sid` | Revoke session |
| `GET`  | `/users/:id/tokens` | List API tokens |
| `POST` | `/users/:id/tokens` | Create API token |
| `DELETE`| `/users/:id/tokens/:tid` | Revoke API token |

**SCIM v2 endpoints (SCIM bearer required):**

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/scim/v2/Users` | List users |
| `POST` | `/scim/v2/Users` | Provision user |
| `GET`  | `/scim/v2/Users/:id` | Get user |
| `PUT`  | `/scim/v2/Users/:id` | Replace user |
| `PATCH`| `/scim/v2/Users/:id` | Update user |
| `DELETE`| `/scim/v2/Users/:id` | Deprovision user |
| `GET`  | `/scim/v2/Groups` | List groups |

#### 5.3.4 Kafka events emitted by IAM

After every state-changing operation, publish an `EventEnvelope` to the appropriate topic:

| Event type | Topic | Trigger |
|------------|-------|---------|
| `auth.login.success` | `auth.events` | Successful login |
| `auth.login.failure` | `auth.events` | Failed login attempt |
| `auth.logout` | `auth.events` | Logout |
| `auth.mfa.enrolled` | `auth.events` | MFA enrollment completed |
| `auth.mfa.failed` | `auth.events` | MFA challenge failed |
| `auth.token.created` | `auth.events` | API token created |
| `auth.token.revoked` | `auth.events` | API token revoked |
| `user.created` | `audit.trail` | User created |
| `user.updated` | `audit.trail` | User fields changed |
| `user.deleted` | `audit.trail` | User soft-deleted |
| `user.suspended` | `audit.trail` | User suspended |
| `user.scim.provisioned` | `audit.trail` | SCIM provision |
| `user.scim.deprovisioned` | `audit.trail` | SCIM deprovision |

#### 5.3.5 Phase 1 acceptance criteria

- [ ] `POST /auth/register` creates an org and admin user, returns a JWT.
- [ ] `POST /auth/login` with valid credentials returns a JWT with `exp` = `GATEWAY_JWT_EXPIRY` seconds.
- [ ] `POST /auth/login` with invalid credentials returns `401`.
- [ ] Passwords are hashed with bcrypt cost ≥ 12.
- [ ] JWT is verified by the gateway middleware.
- [ ] `GET /users` proxied through gateway returns 200 with org users.
- [ ] `GET /users` without JWT returns 401 from gateway.
- [ ] Login event is published to `auth.events` Kafka topic.
- [ ] All unit tests pass: `go test ./...` from workspace root.
- [ ] `docker compose up` starts all services without errors.

---

## 6. Phase 2 — Policy Engine

**Goal:** Admins can define, publish, and evaluate data security policies. The Policy Engine can answer: "Is this action permitted for this user?"

### 6.1 Policy Service (`services/policy`)

```
services/policy/
├── go.mod              # module: github.com/openguard/policy
├── main.go
├── migrations/
│   ├── 001_create_policies.up.sql
│   └── 002_create_policy_assignments.up.sql
├── pkg/
│   ├── config/
│   ├── db/
│   ├── engine/
│   │   ├── evaluator.go   # core rule evaluation
│   │   └── rules/
│   │       ├── data_export.go
│   │       ├── anon_access.go
│   │       ├── ip_allowlist.go
│   │       └── session_limit.go
│   ├── handlers/
│   ├── service/
│   ├── repository/
│   └── router/
└── Dockerfile
```

### 6.2 Database schema

**001 — policies**

Uses the canonical `Policy` struct from Section 3.4. The `rules` column is JSONB.

```sql
CREATE TABLE policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL,
    rules        JSONB NOT NULL DEFAULT '{}',
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by   UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policies_org_id ON policies(org_id);
CREATE INDEX idx_policies_type   ON policies(type);
```

**002 — policy_assignments**

```sql
CREATE TABLE policy_assignments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id    UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL,
    target_type  TEXT NOT NULL,   -- "org" | "group" | "user"
    target_id    UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 6.3 Rule Schema per Policy Type

The `rules` JSONB field must conform to these schemas (validate on write):

**`data_export`**
```json
{
  "allow_external_export": false,
  "allowed_domains": ["example.com"],
  "block_public_links": true,
  "watermark_exports": false
}
```

**`anon_access`**
```json
{
  "allow_anonymous_read": false,
  "allow_anonymous_comment": false
}
```

**`ip_allowlist`**
```json
{
  "allowed_cidrs": ["10.0.0.0/8", "192.168.1.0/24"],
  "enforce_for_api_tokens": true
}
```

**`session_limit`**
```json
{
  "max_concurrent_sessions": 3,
  "session_timeout_minutes": 480,
  "idle_timeout_minutes": 60
}
```

### 6.4 Evaluator Interface

```go
// pkg/engine/evaluator.go

package engine

import "context"

// EvalRequest is the input to the policy evaluator.
type EvalRequest struct {
    OrgID      string
    UserID     string
    Action     string   // e.g. "data.export", "anon.read"
    Resource   string   // e.g. "confluence.page:123"
    IPAddress  string
    UserGroups []string
    Context    map[string]any
}

// EvalResult is the output.
type EvalResult struct {
    Permitted bool
    MatchedPolicies []string   // IDs of policies that matched
    Reason    string
}

// Evaluator evaluates policies for a given request.
type Evaluator interface {
    Evaluate(ctx context.Context, req EvalRequest) (EvalResult, error)
}
```

### 6.5 HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/policies` | List org policies (paginated, filterable by type/enabled) |
| `POST` | `/policies` | Create policy |
| `GET` | `/policies/:id` | Get policy |
| `PUT` | `/policies/:id` | Replace policy |
| `PATCH` | `/policies/:id` | Partial update |
| `DELETE` | `/policies/:id` | Delete policy |
| `POST` | `/policies/:id/enable` | Enable policy |
| `POST` | `/policies/:id/disable` | Disable policy |
| `POST` | `/policies/evaluate` | Synchronous policy evaluation |
| `GET` | `/policies/:id/assignments` | List assignments |
| `POST` | `/policies/:id/assignments` | Assign to org/group/user |
| `DELETE` | `/policies/:id/assignments/:aid` | Remove assignment |

### 6.6 Kafka events emitted

| Event type | Topic |
|------------|-------|
| `policy.created` | `policy.changes` |
| `policy.updated` | `policy.changes` |
| `policy.deleted` | `policy.changes` |
| `policy.enabled` | `policy.changes` |
| `policy.disabled` | `policy.changes` |
| `policy.evaluated` (result: denied only) | `audit.trail` |

### 6.7 Phase 2 acceptance criteria

- [ ] `POST /policies` creates a policy with valid JSONB rules.
- [ ] `POST /policies` with invalid rule schema returns `422`.
- [ ] `POST /policies/evaluate` with a matching IP allowlist policy returns `permitted: false` for a blocked IP.
- [ ] Policy change event is published to `policy.changes`.
- [ ] All existing Phase 1 tests still pass.

---

## 7. Phase 3 — Event Bus & Audit Log

**Goal:** Kafka is the backbone for all async flows. The Audit Log Service consumes all events and persists an immutable, queryable trail.

### 7.1 Kafka Infrastructure

In `infra/kafka/topics.json`, define all topics with their retention settings:

```json
[
  { "name": "auth.events",            "partitions": 6,  "retention_ms": 604800000 },
  { "name": "policy.changes",         "partitions": 3,  "retention_ms": 604800000 },
  { "name": "data.access",            "partitions": 12, "retention_ms": 259200000 },
  { "name": "threat.alerts",          "partitions": 6,  "retention_ms": 2592000000 },
  { "name": "audit.trail",            "partitions": 12, "retention_ms": -1 },
  { "name": "notifications.outbound", "partitions": 3,  "retention_ms": 86400000 }
]
```

Implement `scripts/create-topics.sh` that calls the Kafka CLI (or admin API) to create these topics idempotently.

### 7.2 Shared Kafka Helpers (`shared/kafka`)

```go
// Producer wraps the Kafka client for structured publishing.
type Producer interface {
    Publish(ctx context.Context, topic string, key string, envelope models.EventEnvelope) error
    Close() error
}

// Consumer wraps the Kafka consumer group.
type Consumer interface {
    Subscribe(topics []string, handler HandlerFunc) error
    Start(ctx context.Context) error
    Close() error
}

type HandlerFunc func(ctx context.Context, envelope models.EventEnvelope) error
```

Use `github.com/twmb/franz-go` as the Kafka client. Include retry logic with exponential backoff (max 5 attempts, base 100ms).

### 7.3 Audit Log Service (`services/audit`)

```
services/audit/
├── go.mod              # module: github.com/openguard/audit
├── main.go
├── pkg/
│   ├── consumer/       # Kafka consumer — subscribes to audit.trail
│   ├── repository/     # MongoDB write + query
│   ├── handlers/       # HTTP read endpoints
│   └── router/
└── Dockerfile
```

#### 7.3.1 MongoDB schema

Collection: `audit_events`

```go
// AuditEvent is the MongoDB document structure.
type AuditEvent struct {
    ID          primitive.ObjectID `bson:"_id,omitempty"`
    EventID     string             `bson:"event_id"`   // from EventEnvelope.ID
    Type        string             `bson:"type"`
    OrgID       string             `bson:"org_id"`
    ActorID     string             `bson:"actor_id"`
    ActorType   string             `bson:"actor_type"`
    OccurredAt  time.Time          `bson:"occurred_at"`
    Source      string             `bson:"source"`
    TraceID     string             `bson:"trace_id"`
    Payload     bson.Raw           `bson:"payload"`
    Archived    bool               `bson:"archived"`   // for retention processing
    InsertedAt  time.Time          `bson:"inserted_at"`
}
```

Required indexes:
```js
db.audit_events.createIndex({ org_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ org_id: 1, type: 1, occurred_at: -1 })
db.audit_events.createIndex({ actor_id: 1, occurred_at: -1 })
db.audit_events.createIndex({ event_id: 1 }, { unique: true })
db.audit_events.createIndex({ occurred_at: 1 }, { expireAfterSeconds: 63072000 })
```

The TTL index uses `AUDIT_RETENTION_DAYS × 86400` seconds. This is set at startup from the env variable.

#### 7.3.2 Consumer logic

- Subscribe to `audit.trail`.
- On each message: deserialize `EventEnvelope`, map to `AuditEvent`, insert with `InsertedAt = now()`.
- On duplicate `event_id` (unique index violation): log as warning, skip (idempotent).
- Dead letter: on 5 consecutive failures for the same message, write to a `audit_dead_letter` collection and continue.

#### 7.3.3 HTTP API (read-only)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/audit/events` | List events (paginated, filterable) |
| `GET` | `/audit/events/:id` | Get single event |
| `GET` | `/audit/events/export` | Export as CSV or JSON (async, returns job ID) |
| `GET` | `/audit/export-jobs/:id` | Check export job status |

**Query parameters for `GET /audit/events`:**

| Param | Type | Description |
|-------|------|-------------|
| `type` | string | Event type prefix filter (e.g. `auth.login`) |
| `actor_id` | UUID | Filter by actor |
| `from` | ISO 8601 | Start time |
| `to` | ISO 8601 | End time |
| `page` | int | Page number (default: 1) |
| `per_page` | int | Results per page (default: 50, max: 500) |

### 7.4 Phase 3 acceptance criteria

- [ ] Kafka topics are created by `scripts/create-topics.sh`.
- [ ] A login event published by IAM appears in MongoDB `audit_events` within 2 seconds.
- [ ] `GET /audit/events?type=auth.login` returns the event.
- [ ] Duplicate event IDs are skipped without error.
- [ ] Dead letter documents are written after 5 consumer failures.

---

## 8. Phase 4 — Threat Detection & Alerting

**Goal:** Real-time detection of suspicious activity (anomalous logins, brute force, impossible travel). Alerts delivered to Slack/email and optionally forwarded to a SIEM.

### 8.1 Threat Detection Service (`services/threat`)

```
services/threat/
├── go.mod              # module: github.com/openguard/threat
├── main.go
├── pkg/
│   ├── consumer/       # Kafka consumer — auth.events + data.access
│   ├── detector/
│   │   ├── brute_force.go
│   │   ├── impossible_travel.go
│   │   ├── off_hours.go
│   │   └── data_exfil.go
│   ├── scorer/         # Composite risk scoring
│   ├── repository/     # MongoDB for alert records
│   ├── handlers/
│   └── router/
└── Dockerfile
```

#### 8.1.1 Detectors

Each detector implements:

```go
type Detector interface {
    Name() string
    Detect(ctx context.Context, event models.EventEnvelope, history []models.EventEnvelope) (*Detection, error)
}

type Detection struct {
    DetectorName string
    RiskScore    float64  // 0.0 – 1.0
    Reason       string
    Metadata     map[string]any
}
```

**`brute_force.go`:**
- Window: `THREAT_ANOMALY_WINDOW_MINUTES`.
- Threshold: `THREAT_MAX_FAILED_LOGINS` failures for the same `actor_id` within the window.
- Uses Redis sorted sets for fast counting: key = `bf:{actor_id}`, score = Unix timestamp.
- Risk score: `min(1.0, failure_count / threshold)`.

**`impossible_travel.go`:**
- On `auth.login.success`, compare IP geolocation to the previous successful login's location.
- If distance > `THREAT_GEO_CHANGE_THRESHOLD_KM` AND time delta < 1 hour: raise detection.
- Use the `oschwald/geoip2-golang` library with the MaxMind GeoLite2-City database.
- Risk score: `min(1.0, distance_km / 5000)`.

**`off_hours.go`:**
- On any `auth.login.success`, check if the event time (in the org's configured timezone) is outside 07:00–22:00.
- Risk score: 0.3 (low, contextual).

**`data_exfil.go`:**
- On `data.access` events: if a single `actor_id` triggers > 100 data access events within 10 minutes, raise detection.
- Uses Redis for counting: key = `de:{actor_id}`, TTL 10 minutes.
- Risk score: `min(1.0, count / 100)`.

#### 8.1.2 Scorer

```go
// Composite score = max(individual scores) * 0.6 + average(individual scores) * 0.4
func CompositeScore(detections []Detection) float64
```

If composite score ≥ 0.5: produce an alert to `threat.alerts`.
If composite score ≥ 0.8: flag as HIGH severity.

#### 8.1.3 Alert MongoDB document

```go
type Alert struct {
    ID            primitive.ObjectID `bson:"_id,omitempty"`
    OrgID         string             `bson:"org_id"`
    ActorID       string             `bson:"actor_id"`
    Severity      string             `bson:"severity"`  // "low" | "medium" | "high" | "critical"
    CompositeScore float64           `bson:"composite_score"`
    Detections    []Detection        `bson:"detections"`
    Status        string             `bson:"status"`    // "open" | "acknowledged" | "resolved"
    TriggerEvent  models.EventEnvelope `bson:"trigger_event"`
    CreatedAt     time.Time          `bson:"created_at"`
    UpdatedAt     time.Time          `bson:"updated_at"`
    AcknowledgedBy string            `bson:"acknowledged_by,omitempty"`
    ResolvedBy     string            `bson:"resolved_by,omitempty"`
}
```

#### 8.1.4 HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/threats/alerts` | List alerts (filterable by severity, status) |
| `GET` | `/threats/alerts/:id` | Get alert detail |
| `POST` | `/threats/alerts/:id/acknowledge` | Acknowledge alert |
| `POST` | `/threats/alerts/:id/resolve` | Resolve alert |
| `GET` | `/threats/stats` | Summary stats (open alerts, by severity) |

### 8.2 Alerting Service (`services/alerting`)

#### 8.2.1 Consumers

Subscribe to `threat.alerts` and `notifications.outbound`.

For each `threat.alerts` message:
1. Evaluate notification rules (which channels are configured for this org and severity).
2. Enqueue to `notifications.outbound`.

#### 8.2.2 Notification channels

Each channel is a struct implementing:

```go
type Notifier interface {
    Name() string
    Send(ctx context.Context, notification Notification) error
}

type Notification struct {
    OrgID    string
    Subject  string
    Body     string
    Severity string
    AlertID  string
    ActorID  string
}
```

Implement:
- `SlackNotifier` — HTTP POST to `ALERTING_SLACK_WEBHOOK_URL`.
- `EmailNotifier` — SMTP via `ALERTING_SMTP_*` vars. Use Go `net/smtp`.
- `WebhookNotifier` — Generic HTTPS POST to `ALERTING_SIEM_WEBHOOK_URL` with JSON body.

#### 8.2.3 SIEM export format

```json
{
  "event_type": "openguard.threat.alert",
  "severity": "high",
  "timestamp": "2024-01-15T10:30:00Z",
  "org_id": "...",
  "actor_id": "...",
  "alert_id": "...",
  "composite_score": 0.87,
  "detectors_fired": ["brute_force", "impossible_travel"],
  "raw_alert": { ... }
}
```

### 8.3 Phase 4 acceptance criteria

- [ ] 11 failed logins within the window produce an `alert` document in MongoDB.
- [ ] Alert is published to `threat.alerts`.
- [ ] Slack notification is sent (verify webhook call in integration test with httptest mock).
- [ ] `GET /threats/alerts` returns the alert.
- [ ] `POST /threats/alerts/:id/acknowledge` updates status.
- [ ] `GET /threats/stats` returns correct open count.

---

## 9. Phase 5 — Compliance & Analytics

**Goal:** Generate audit-grade compliance reports. Provide a time-series analytics view over all activity.

### 9.1 ClickHouse Schema

```sql
CREATE TABLE IF NOT EXISTS events (
    event_id     String,
    type         String,
    org_id       String,
    actor_id     String,
    actor_type   String,
    occurred_at  DateTime64(3, 'UTC'),
    source       String,
    payload      String   -- JSON string
) ENGINE = MergeTree()
ORDER BY (org_id, occurred_at)
PARTITION BY toYYYYMM(occurred_at)
TTL occurred_at + INTERVAL 2 YEAR;

CREATE TABLE IF NOT EXISTS alert_stats (
    org_id       String,
    day          Date,
    severity     String,
    count        UInt64
) ENGINE = SummingMergeTree(count)
ORDER BY (org_id, day, severity);
```

### 9.2 Compliance Service (`services/compliance`)

```
services/compliance/
├── go.mod              # module: github.com/openguard/compliance
├── main.go
├── pkg/
│   ├── consumer/       # Kafka consumer → ClickHouse writer
│   ├── reporter/
│   │   ├── gdpr.go
│   │   ├── soc2.go
│   │   └── hipaa.go
│   ├── renderer/       # PDF + CSV rendering
│   ├── repository/
│   ├── handlers/
│   └── router/
└── Dockerfile
```

#### 9.2.1 Report definitions

Each report type is a struct:

```go
type Report struct {
    ID          string
    OrgID       string
    Type        ReportType       // "gdpr" | "soc2" | "hipaa" | "custom"
    Status      ReportStatus     // "pending" | "running" | "completed" | "failed"
    Format      string           // "pdf" | "csv" | "json"
    DateFrom    time.Time
    DateTo      time.Time
    GeneratedAt *time.Time
    DownloadURL string
    CreatedBy   string
    CreatedAt   time.Time
}
```

Reports are stored in MongoDB collection `compliance_reports`.

#### 9.2.2 Report sections (implement all)

**GDPR:**
- Total events by type in period.
- User data access log (who accessed what).
- Data export events.
- User creation/deletion events.
- Policy changes affecting data access.

**SOC 2 (Type II):**
- Authentication events (success/failure ratio).
- Privilege escalations.
- Session anomalies.
- Policy enforcement actions.
- Alert summary with resolution times.

**HIPAA:**
- Access log to sensitive resources.
- Failed access attempts.
- User provisioning/deprovisioning.
- Audit log integrity verification (hash chain summary).

#### 9.2.3 PDF rendering

Use `github.com/go-pdf/fpdf` for PDF generation. Reports must include:
- OpenGuard logo (SVG, embedded as base64).
- Organization name and report period.
- Section headers with a table of contents.
- Tables for each data section.
- Footer with page number and generation timestamp.

#### 9.2.4 HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/compliance/reports` | Create report job |
| `GET` | `/compliance/reports` | List org reports |
| `GET` | `/compliance/reports/:id` | Get report status |
| `GET` | `/compliance/reports/:id/download` | Stream file download |
| `DELETE` | `/compliance/reports/:id` | Delete report |
| `GET` | `/compliance/stats` | Analytics stats from ClickHouse |

**`GET /compliance/stats` query params:**

| Param | Description |
|-------|-------------|
| `metric` | `logins`, `failures`, `alerts`, `policy_changes`, `data_exports` |
| `from` | ISO 8601 |
| `to` | ISO 8601 |
| `granularity` | `hour`, `day`, `week`, `month` |

Response:
```json
{
  "metric": "logins",
  "granularity": "day",
  "data": [
    { "timestamp": "2024-01-01T00:00:00Z", "value": 142 },
    { "timestamp": "2024-01-02T00:00:00Z", "value": 198 }
  ]
}
```

### 9.3 Phase 5 acceptance criteria

- [ ] Events are written to ClickHouse from the compliance consumer.
- [ ] `POST /compliance/reports` with type `gdpr` creates a report job.
- [ ] Report job completes and produces a downloadable PDF with all sections.
- [ ] `GET /compliance/stats?metric=logins&granularity=day` returns time-series data.

---

## 10. Phase 6 — Frontend (Next.js)

**Goal:** A production-quality admin dashboard implementing all of the above. Server Components where possible. Client Components only for interactive UI.

### 10.1 Project setup

```
web/
├── app/
│   ├── (auth)/
│   │   ├── login/
│   │   │   └── page.tsx
│   │   └── sso/
│   │       └── page.tsx
│   ├── (dashboard)/
│   │   ├── layout.tsx
│   │   ├── page.tsx              # Overview / home
│   │   ├── users/
│   │   │   ├── page.tsx
│   │   │   └── [id]/
│   │   │       └── page.tsx
│   │   ├── policies/
│   │   │   ├── page.tsx
│   │   │   └── [id]/
│   │   │       └── page.tsx
│   │   ├── threats/
│   │   │   ├── page.tsx
│   │   │   └── [id]/
│   │   │       └── page.tsx
│   │   ├── audit/
│   │   │   └── page.tsx
│   │   └── compliance/
│   │       └── page.tsx
│   ├── api/
│   │   └── auth/
│   │       └── [...nextauth]/
│   │           └── route.ts
│   ├── layout.tsx
│   └── globals.css
├── components/
│   ├── ui/               # shadcn/ui re-exports
│   ├── layout/
│   │   ├── Sidebar.tsx
│   │   ├── Topbar.tsx
│   │   └── Breadcrumb.tsx
│   ├── users/
│   ├── policies/
│   ├── threats/
│   ├── audit/
│   └── compliance/
├── lib/
│   ├── api.ts            # typed fetch wrappers for each service
│   ├── auth.ts           # NextAuth config
│   └── utils.ts
└── next.config.ts
```

### 10.2 Authentication (NextAuth)

Configure `next-auth` with:
- **Credentials provider** — POST to Gateway `/api/v1/auth/login`, return JWT as session token.
- **SAML provider** — custom provider wrapping the IAM service's SAML callback.

Protect all `(dashboard)` routes via middleware (`middleware.ts`):

```ts
// middleware.ts
import { withAuth } from "next-auth/middleware";
export default withAuth({ pages: { signIn: "/login" } });
export const config = { matcher: ["/((?!login|api|_next).*)"] };
```

### 10.3 Pages specification

#### Dashboard Home (`/`)

- Stat cards: Total users, Active alerts, Policy violations (7d), Audit events (7d).
- Alert severity pie chart (recharts).
- Login activity time-series chart (recharts), last 30 days.
- Recent audit events table (last 10).

#### Users (`/users`)

- Paginated data table: name, email, status (badge), MFA (icon), last login.
- Filter by status.
- Actions: Suspend, Activate, View detail.
- "Invite user" button → modal → `POST /users`.

#### User Detail (`/users/[id]`)

- User info card.
- Active sessions table with "Revoke" per row.
- API tokens table with "Revoke" per row.
- Recent audit events for this user.

#### Policies (`/policies`)

- List of policies with type badge and enabled/disabled toggle.
- "Create policy" → multi-step form (type → rule fields → assign).
- Policy detail: rule display, assignments list, change history from audit log.

#### Threats (`/threats`)

- Alert table: severity badge, actor, detectors fired, timestamp, status.
- Filter by severity, status.
- Alert detail: detector breakdown, timeline, acknowledge/resolve actions.
- Real-time update via polling (`/threats/stats`) every 30 seconds.

#### Audit Log (`/audit`)

- Full-page event table with all filters from Section 7.3.3.
- "Export" button → triggers export job → polls and downloads.

#### Compliance (`/compliance`)

- List of generated reports.
- "Generate report" → form: type, date range, format.
- Progress indicator via polling job status.
- Download button when complete.
- Analytics tab: recharts dashboard for metrics from `GET /compliance/stats`.

### 10.4 API Client (`lib/api.ts`)

Generate a fully-typed API client. Each function must:
- Read `NEXT_PUBLIC_API_URL`.
- Include `Authorization: Bearer {token}` from NextAuth session.
- Return typed response or throw a typed `APIError`.

```ts
// Example signature shape:
export async function listUsers(params: ListUsersParams): Promise<PaginatedResponse<User>>
export async function getUser(id: string): Promise<User>
export async function createUser(data: CreateUserInput): Promise<User>
export async function listAlerts(params: ListAlertsParams): Promise<PaginatedResponse<Alert>>
// ... all endpoints
```

### 10.5 Phase 6 acceptance criteria

- [ ] `npm run dev` starts Next.js on port 3000 without errors.
- [ ] Unauthenticated visit to `/` redirects to `/login`.
- [ ] Login with credentials issued in Phase 1 lands on the dashboard.
- [ ] Users page lists users from the API.
- [ ] Alert table shows alerts from Phase 4.
- [ ] Compliance report can be triggered and downloaded.

---

## 11. Phase 7 — Infra, CI/CD & Observability

### 11.1 Docker Compose (local dev)

`infra/docker/docker-compose.yml` must define:

| Service | Image | Ports |
|---------|-------|-------|
| `postgres` | `postgres:16-alpine` | 5432 |
| `mongo` | `mongo:7` | 27017 |
| `redis` | `redis:7-alpine` | 6379 |
| `zookeeper` | `bitnami/zookeeper:3.9` | 2181 |
| `kafka` | `bitnami/kafka:3.6` | 9092 |
| `clickhouse` | `clickhouse/clickhouse-server:24` | 9000, 8123 |
| `jaeger` | `jaegertracing/all-in-one:latest` | 16686, 4317 |
| `prometheus` | `prom/prometheus:latest` | 9090 |
| `grafana` | `grafana/grafana:latest` | 3001 |

All services must have health checks. All services must use a named volume for persistence.

Each Go service and the Next.js frontend also has a Dockerfile. In `docker-compose.dev.yml`, override Go services to mount source and use `air` for hot reload.

### 11.2 GitHub Actions

**`.github/workflows/ci.yml`** — runs on every PR:

```yaml
jobs:
  go-test:
    runs-on: ubuntu-latest
    services:
      postgres: ...
      redis: ...
      kafka: ...
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go work sync
      - run: go test ./... -race -cover
      - run: go vet ./...

  go-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: golangci/golangci-lint-action@v4
        with: { version: latest }

  next-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: cd web && npm ci && npm run build

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: aquasecurity/trivy-action@master
        with: { scan-type: 'fs' }
```

**`.github/workflows/release.yml`** — runs on tag push `v*.*.*`:
- Build and push Docker images to `ghcr.io/openguard/*`.
- Tag images with the semver tag and `latest`.
- Create GitHub Release with auto-generated changelog.

### 11.3 Observability

#### OpenTelemetry

Every Go service must initialize OpenTelemetry tracing on startup using `shared/telemetry`:

```go
func InitTracer(serviceName string) (func(), error) {
    // OTLP gRPC exporter to OTEL_EXPORTER_OTLP_ENDPOINT
    // Returns shutdown function
}
```

Instrument:
- All HTTP handlers (middleware in `shared/middleware`).
- All Kafka produce/consume operations.
- All database operations (PostgreSQL, MongoDB, Redis, ClickHouse).

#### Prometheus metrics

Every Go service exposes `/metrics` (Prometheus format). Instrument:

| Metric name | Type | Labels |
|-------------|------|--------|
| `http_requests_total` | Counter | method, path, status |
| `http_request_duration_seconds` | Histogram | method, path |
| `kafka_messages_produced_total` | Counter | topic |
| `kafka_messages_consumed_total` | Counter | topic, consumer_group |
| `kafka_consumer_lag` | Gauge | topic, consumer_group |
| `db_query_duration_seconds` | Histogram | service, operation |

#### Grafana dashboards

Provide JSON for two dashboards in `infra/monitoring/grafana/dashboards/`:

1. `openguard-overview.json` — all services, request rates, error rates, Kafka lag.
2. `openguard-security.json` — alert rate, threat detections, failed logins, policy violations.

### 11.4 Helm chart

`infra/k8s/helm/openguard/` must be a valid Helm chart with:
- A `values.yaml` with all image tags, replica counts (default: 1), and resource requests.
- A `Deployment` + `Service` template for each Go service.
- A `ConfigMap` for non-secret environment variables.
- A `Secret` template (with instructions to use external-secrets in production).
- An `Ingress` template for the gateway.
- A `HorizontalPodAutoscaler` for gateway, iam, and threat services.

### 11.5 Phase 7 acceptance criteria

- [ ] `docker compose up` starts all infrastructure and services with no errors.
- [ ] `go test ./... -race` passes in CI.
- [ ] `npm run build` passes in CI.
- [ ] Trivy scan produces no CRITICAL vulnerabilities.
- [ ] Prometheus scrapes metrics from all services.
- [ ] Jaeger shows traces for a login → policy evaluate → audit log flow.
- [ ] `helm lint infra/k8s/helm/openguard` passes.

---

## 12. Phase 8 — Hardening & Documentation

### 12.1 Security hardening

Implement the following in every Go service:

- **HTTP security headers middleware** (apply to all routes):
  ```
  Strict-Transport-Security: max-age=31536000; includeSubDomains
  X-Content-Type-Options: nosniff
  X-Frame-Options: DENY
  Content-Security-Policy: default-src 'self'
  Referrer-Policy: no-referrer
  ```
- **CORS middleware:** Allow only `NEXT_PUBLIC_API_URL` origin.
- **Request size limit:** 10MB on all POST/PUT/PATCH endpoints.
- **SQL injection:** Use parameterized queries only. No string concatenation in SQL. Lint with `go-sqllint`.
- **Secrets in logs:** Implement a `SafeLogger` wrapper that redacts values for keys containing `password`, `secret`, `token`, `key`, `auth`.
- **Dependency audit:** `go list -m -json all | nancy` in CI.

### 12.2 Encryption

- TOTP secrets in `mfa_configs.secret` must be encrypted at rest using AES-256-GCM. Key from env `IAM_MFA_ENCRYPTION_KEY`.
- API token raw values are never stored. Only `token_hash` (SHA-256) is persisted.
- Passwords use bcrypt cost 12.
- All inter-service HTTP must use TLS in production (configure in Helm values).

### 12.3 Documentation

Produce the following:

**`README.md`** — must include:
- Project description and feature list.
- Quick start (Docker Compose) in < 5 steps.
- Architecture diagram (Mermaid).
- Links to all other docs.

**`docs/architecture.md`** — must include:
- Component diagram (Mermaid).
- Data flow diagrams for: login flow, policy evaluation, threat detection pipeline.
- Database schema ERD (Mermaid).

**`docs/contributing.md`** — must include:
- Local dev setup.
- PR guidelines.
- Commit message format (Conventional Commits).
- How to add a new detector.
- How to add a new report type.

**OpenAPI specs:** Each service must have a `/openapi.json` endpoint and a static copy at `docs/api/<service>.openapi.json`. Generate with `swaggo/swag` or hand-write — either is acceptable but must be valid OpenAPI 3.1.

### 12.4 Phase 8 acceptance criteria

- [ ] Security headers are present on all HTTP responses.
- [ ] TOTP secrets are unreadable without the encryption key.
- [ ] `nancy` finds no known CVEs in Go dependencies.
- [ ] `README.md` quick start works on a clean machine.
- [ ] All OpenAPI specs validate with `vacuum` or `redocly lint`.
- [ ] Architecture diagram renders correctly in GitHub Markdown.

---

## 13. Cross-Cutting Concerns

### 13.1 Structured logging

All services must use `log/slog` (stdlib, Go 1.21+) with JSON format in non-development environments:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
}))
```

Every log entry must include: `service`, `trace_id`, `request_id`, `level`, `msg`.

### 13.2 Graceful shutdown

Every service must handle `SIGTERM` and `SIGINT`:

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
// shutdown HTTP server, Kafka consumers, DB connections
```

### 13.3 Health check endpoints

Every service must implement:

- `GET /health/live` — returns `200 {"status":"ok"}` if the process is running.
- `GET /health/ready` — returns `200` only if all dependencies (DB, Kafka) are reachable; returns `503` otherwise.

### 13.4 Pagination

All list endpoints must return this envelope:

```json
{
  "data": [...],
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 1024,
    "total_pages": 21
  }
}
```

### 13.5 Idempotency

All `POST` endpoints that create resources must accept an `Idempotency-Key` header. If the same key is seen within 24 hours, return the original response (cached in Redis).

### 13.6 Testing standards

| Layer | Tool | Minimum coverage |
|-------|------|-----------------|
| Unit tests | `testing` stdlib | 70% per package |
| Integration tests | `testcontainers-go` | 1 per service |
| API tests | `net/http/httptest` | All happy paths |
| E2E (optional) | Playwright | Login + dashboard load |

All tests must be deterministic. No `time.Sleep`. Use `testify` for assertions.

---

## 14. Acceptance Criteria (Full System)

When all 8 phases are complete, the following end-to-end scenario must work without manual intervention:

1. `docker compose up -d` — all services start healthy.
2. `POST /api/v1/auth/register` — creates Org "Acme" and admin user.
3. `POST /api/v1/auth/login` — returns JWT.
4. `POST /api/v1/policies` — creates an IP allowlist policy.
5. `POST /api/v1/policies/evaluate` — returns `permitted: false` for a blocked IP.
6. Simulate 11 failed logins via the API — a `high` severity alert is created.
7. `GET /api/v1/threats/alerts` — alert is visible.
8. Slack webhook mock receives a notification.
9. `GET /api/v1/audit/events` — all events from steps 2–8 are present.
10. `POST /api/v1/compliance/reports` with type `gdpr` — report generates and is downloadable.
11. Next.js login at `http://localhost:3000` works with the admin credentials.
12. Dashboard displays the alert count and recent audit events.
13. `go test ./... -race` — all tests pass.
14. `docker compose down` — clean shutdown.

---

*End of specification. Begin with Phase 1. Do not proceed to Phase 2 until all Phase 1 acceptance criteria are met.*
