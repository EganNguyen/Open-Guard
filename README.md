# OpenGuard

**OpenGuard** is an open-source, self-hostable enterprise security control plane. It serves as a centralized governance hub connecting applications via SDK, SCIM 2.0, OIDC/SAML, and outbound webhooks. OpenGuard is an out-of-band governance system — user traffic never flows *through* it, minimizing operational risk and latency impact on your core applications.

## 🏗 System Architecture

OpenGuard is architected as a highly scalable set of Go microservices managed by a Next.js administrative frontend.

### Core Components

*   **Backend (`services/`)**: A suite of robust Go microservices responsible for IAM, policy evaluation, threat detection, alerting, compliance reporting, and DLP. The backend utilizes Kafka for asynchronous event propagation (via the Transactional Outbox pattern), taking advantage of PostgreSQL for relational state and strict Row-Level Security (RLS) for multi-tenancy.
*   **Frontend (`web/`)**: A Next.js 14 Admin Dashboard utilizing the App Router, TypeScript, TanStack Query, and Zustand. It serves as the visual command center for system administrators to configure access policies, review audit streams, and manage threat-alerting sagas in real time.
*   **Shared Contract (`shared/`)**: Immutable Go models and utilities (middleware, crypto) shared across backend services to ensure strongly typed communication and schema enforcement.
*   **SDK (`sdk/`)**: Client libraries designed to help downstream applications integrate seamlessly with OpenGuard's policy decisions and event ingestion.

## 🛡️ Key Principles

*   **Fail-Closed Policy Engine**: Security evaluations are explicitly designed to be fail-closed. If caching layers (Redis) or the policy engine becomes unresponsive after grace periods expire, access is explicitly denied.
*   **Zero Cross-Tenant Leakage**: Rigidly enforced via PostgreSQL Row-Level Security. Every organzation-scoped table ensures deep isolation at the database layer.
*   **Strong Resiliency**: Addresses dual-write vulnerabilities between PostgreSQL and Kafka by enforcing the Transactional Outbox pattern alongside meticulously configured circuit breakers and bulkheading (e.g., dedicated bcrypt worker pools).
*   **High Performance**: Built to execute under intense throughput requirements (e.g., executing cached policy evaluations at 10,000 req/s within <5ms p99 latency).

## 📚 Documentation & Specifications

This repository embraces a specification-driven development model. Documentation is rigorously structured to guide development, ensuring consistent CI-compliance.

### For AI/Claude Agents
**Start Here ➔** [`claude.md`](claude.md): The absolute master index for AI coding assistants. **Do not begin any task without reading it.** It acts as the core routing layer pointing to specific SKILL files and architectural documents to guarantee alignment with CI rules.

### For Human Engineers
- [**Backend Documentation Index**](be_open_guard/README.md) — Comprehensive breakdown of architecture choices, phase planning, data models, error handling, and strict backend coding rules.
- [**Frontend Documentation Index**](fe_open_guard/README.md) — Detailed mapping of the UI architecture, canonical component patterns, state management, and stringent frontend conventions.

## 🚀 Getting Started

The platform's progression is tracked across tightly governed iterative **Phases**. 

1. **Bootstrap the Ecosystem**: Review the [Infrastructure & CI constraints](be_open_guard/08-phase1-infra-ci-observability.md) to instantiate the local development stack via Docker Compose (`infra/docker/`).
2. **Launch the Dashboard**: Delve into the [Frontend Tech Stack](fe_open_guard/00-tech-stack-and-conventions.md) before initializing the Next.js `web/` application.
3. **Core Development**: Utilize the `Makefile` as the entry point for all major operations (e.g., testing, linting, code generation, database migrations, and load-testing).

---
*OpenGuard — Enterprise Grade Security, Open Source Freedom.*
