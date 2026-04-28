# Codebase Index

## 1. Service Registry
| Service | Directory | Entry Point | Port | Data Store |
| :--- | :--- | :--- | :--- | :--- |
| **Control Plane** | `services/control-plane` | `main.go` | 8081 | PG, Redis |
| **IAM** | `services/iam` | `main.go` | 8082 | PG, Redis |
| **Policy** | `services/policy` | `main.go` | 8083 | PG, Redis |
| **Threat** | `services/threat` | `main.go` | 8084 | Mongo, Redis |
| **Audit** | `services/audit` | `main.go` | 8085 | Mongo, Kafka |
| **Alerting** | `services/alerting` | `main.go` | 8086 | Mongo, Redis |
| **Webhook Delivery**| `services/webhook-delivery`| `main.go` | 8087 | Kafka |
| **Compliance** | `services/compliance` | `main.go` | 8088 | ClickHouse, PG |
| **DLP** | `services/dlp` | `main.go` | 8089 | PG, Redis |
| **Connector Reg.** | `services/connector-registry`| `main.go` | 8090 | PG, Redis |

## 2. Shared Library (`shared/`)
Core utilities shared across all microservices:
- `shared/crypto`: JWT validation, mTLS helpers, and encryption.
- `shared/resilience`: Circuit breaker (gobreaker) and retry logic.
- `shared/middleware`: Auth, Logging, Correlation, and Security Headers.
- `shared/db`: Postgres/PGX helpers and RLS session management.
- `shared/telemetry`: Prometheus metrics and structured logging (SafeAttr).

## 3. Frontend Architecture (`web/`)
- **Tech Stack:** Angular 19, Tailwind CSS.
- **State Management:** Angular Signals (Modern) instead of BehaviorSubject (Legacy).
- **Key Services:**
    - `AuthService`: Identity and token management.
    - `ThreatService`: Real-time alert ingestion via SSE.
    - `PolicyService`: Management of security rules.

## 4. Key Infrastructure Files
- `infra/docker/docker-compose.yml`: Local dev stack.
- `go.work`: Multi-module workspace management.
- `Makefile`: Central dev/test/build automation.
- `.env`: Environment configuration (derived from `.env.example`).
