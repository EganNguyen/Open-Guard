# OpenGuard

**OpenGuard** is an open-source, self-hostable enterprise security control plane. It serves as a centralized governance hub connecting applications via SDK, SCIM 2.0, OIDC/SAML, and outbound webhooks. OpenGuard is an out-of-band governance system — user traffic never flows *through* it, minimizing operational risk and latency impact on your core applications.

## 🏗 System Architecture

OpenGuard is architected as a highly scalable set of Go microservices managed by a Next.js administrative frontend.

### Core Components

*   **Backend (`services/`)**: A suite of robust Go microservices responsible for IAM, policy evaluation, threat detection, alerting, compliance reporting, and DLP. The backend utilizes Kafka for asynchronous event propagation (via the Transactional Outbox pattern), taking advantage of PostgreSQL for relational state and strict Row-Level Security (RLS) for multi-tenancy.
*   **Frontend (`web/`)**: An Angular 21+ Admin Dashboard utilizing Standalone Components, Angular Signals, and Tailwind CSS. It serves as the visual command center for system administrators to configure access policies, review audit streams, and manage threat-alerting sagas in real time.
*   **Shared Contract (`shared/`)**: Immutable Go models and utilities (middleware, crypto) shared across backend services to ensure strongly typed communication and schema enforcement.
*   **SDK (`sdk/`)**: Client libraries designed to help downstream applications integrate seamlessly with OpenGuard's policy decisions and event ingestion.

## 🛡️ Key Principles

*   **Fail-Closed Policy Engine**: Security evaluations are explicitly designed to be fail-closed. If caching layers (Redis) or the policy engine becomes unresponsive after grace periods expire, access is explicitly denied.
*   **Zero Cross-Tenant Leakage**: Rigidly enforced via PostgreSQL Row-Level Security. Every organzation-scoped table ensures deep isolation at the database layer.
*   **Strong Resiliency**: Addresses dual-write vulnerabilities between PostgreSQL and Kafka by enforcing the Transactional Outbox pattern alongside meticulously configured circuit breakers and bulkheading (e.g., dedicated bcrypt worker pools).
*   **High Performance**: Built to execute under intense throughput requirements (e.g., executing cached policy evaluations at 10,000 req/s within <5ms p99 latency).

## 📚 Documentation & Specifications

This repository embraces a specification-driven development model. Documentation is rigorously structured to guide development, ensuring consistent CI-compliance.

### For AI Agents
**Start Here ➔** [`ai-spec/project.md`](ai-spec/project.md): The absolute master index for AI coding assistants. **Do not begin any task without reading it.** It acts as the core routing layer pointing to specific SKILL files and architectural documents to guarantee alignment with CI rules.

### For Human Engineers
- [**Backend Documentation Index**](ai-spec/be_open_guard/README.md) — Comprehensive breakdown of architecture choices, phase planning, data models, error handling, and strict backend coding rules.
- [**Frontend Documentation Index**](ai-spec/fe_open_guard/README.md) — Detailed mapping of the UI architecture, canonical component patterns, state management, and stringent frontend conventions.

## 🚀 Quick Start (Phase 1)

Follow these steps to spin up the environment and verify the Phase 1 implementation.

### 1. Prerequisites
- Docker & Docker Compose
- Go 1.22+
- Node.js 20+ & npm

### 2. Infrastructure Setup
Spin up the core infrastructure (PostgreSQL, MongoDB, Kafka, etc.):
```bash
cd infra/docker
docker compose up -d
```

```bash
docker compose up -d --build
```

Verify all services are healthy:
```bash
docker compose ps
```

To clean all docker services:
```bash
docker compose down --volumes --remove-orphans
```

### 3. Backend Verification
Run tests and security scans:
```bash
# Run all tests with race detection
go test ./... -race

# Check for vulnerabilities
govulncheck ./...
```

### 4. Frontend Dashboard
Initialize and start the Angular dashboard:
```bash
cd web
npm install
npm start
```
The dashboard will be available at `http://localhost:4200`.

### 5. Connected App
```bash
cd examples/task-management-app
npm install
npm run dev
```

### 6. Manual Verification Checklist
To confirm Phase 1 is complete, verify the following:
- [ ] **Infrastructure**: All containers in `docker compose ps` show as `healthy`.
- [ ] **Metrics**: Access Prometheus at `http://localhost:9090` and verify `openguard_*` metrics are present.
- [ ] **Logs**: Access Grafana at `http://localhost:3000` (default credentials: `admin/admin`).
- [ ] **Dashboard**: Navigate to `http://localhost:4200/connectors` and ensure you can view the Connected Apps registration flow.

---
*OpenGuard — Enterprise Grade Security, Open Source Freedom.*
