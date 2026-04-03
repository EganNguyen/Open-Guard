# OpenGuard — Enterprise Security Control Plane Specification

> **Document status:** Authoritative. Supersedes all prior versions.  
> **Audience:** Implementing engineers, technical reviewers, security architects.  
> **How to use:** Read Sections 0–4 in full before writing any code.

## Mandatory Rules (enforced by CI and code review)

- Every Kafka publish goes through the Outbox relay. No direct producer calls from business handlers.
- Every table storing org-scoped data has RLS enabled with an explicit `org_id UUID` column.
- Every inter-service HTTP call wraps a circuit breaker with a defined timeout and fallback.
- Policy engine failure mode: **fail closed**. Cache grace period: 60s. After expiry: deny.
- No string concatenation in SQL. Parameterized queries only.
- No `time.Sleep` in service code. Use `time.NewTicker` inside `select{}` for all polling.
- Interfaces are defined in the consuming package, never in `shared/`.
- All canonical names (env vars, topic names, table names, error codes) are fixed. Rename = major version bump.
- Kafka consumer offsets are committed only after successful downstream write (manual commit mode).
- The connector registry lookup result is cached in Redis. Every `org_id` derivation hits cache, not DB.
- PBKDF2 is used for DB storage only. The hot-path API key lookup uses a fast-hash prefix scheme.
- bcrypt verification runs inside a bounded worker pool. Never in raw goroutines.
- Webhook delivery state is persisted to PostgreSQL. Not held in memory.
- SCIM org_id is derived from the bearer token configuration, never from client-supplied headers.

---

## Document Index

| File | Contents |
|------|----------|
| [00-code-quality-standards.md](00-code-quality-standards.md) | §0 — Package design, naming, error handling, interfaces, concurrency, context, DI, config, testing, observability, HTTP rules, forbidden patterns, review checklist |
| [01-overview-and-architecture.md](01-overview-and-architecture.md) | §1–2 — Project overview, SLOs, design principles, dual-write problem, multi-tenancy, CQRS, sagas, connector flow, SCIM auth, cert rotation, connection pooling, tenant offboarding, API versioning |
| [02-repository-layout.md](02-repository-layout.md) | §3 — Full directory tree, SDK circuit breaker spec, scope middleware, Go workspace |
| [03-shared-contracts.md](03-shared-contracts.md) | §4 — Kafka envelope, outbox record, saga event, topic registry, user model, connected app model, HTTP contracts, sentinel errors |
| [04-environment-and-config.md](04-environment-and-config.md) | §5 — `.env.example` (all variables), config loading pattern |
| [05-multi-tenancy-and-rls.md](05-multi-tenancy-and-rls.md) | §6 — PostgreSQL RLS, DB roles, OrgPool wrapper, outbox RLS, API key middleware, per-tenant quotas, ClickHouse multi-tenancy wrapper |
| [06-transactional-outbox.md](06-transactional-outbox.md) | §7 — Outbox table DDL, Writer, Relay, business handler pattern |
| [07-circuit-breakers-and-resilience.md](07-circuit-breakers-and-resilience.md) | §8 — Circuit breaker impl, bcrypt worker pool, failure mode table, retry policy, bulkhead, outbox write latency breaker |
| [08-phase1-infra-ci-observability.md](08-phase1-infra-ci-observability.md) | §9 — Docker Compose, GitHub Actions CI, Prometheus metrics, Alertmanager rules, Helm chart, Admin UI spec, capacity planning |
| [09-phase2-foundation-and-auth.md](09-phase2-foundation-and-auth.md) | §10 — IAM DB schema, MFA, JWT keyring, risk-based sessions, WebAuthn, SCIM v2, IAM endpoints, SAML, OIDC security, bcrypt pool integration, Control Plane foundation, Phase 2 acceptance criteria |
| [10-phase3-policy-engine.md](10-phase3-policy-engine.md) | §11 — Policy DB schema, Redis caching, policy service architecture, webhook to connectors, policy API, Phase 3 acceptance criteria |
| [11-phase4-event-bus-and-audit.md](11-phase4-event-bus-and-audit.md) | §12 — Kafka topic config, consumer group tuning, audit CQRS architecture, bulk writer, hash chain, MongoDB schema, audit HTTP API, Phase 4 acceptance criteria |
| [12-phase5-threat-and-alerting.md](12-phase5-threat-and-alerting.md) | §13 — Threat detectors, alert lifecycle saga, SIEM webhook signing, threat & alerting API, Phase 5 acceptance criteria |
| [13-phase6-compliance-and-analytics.md](13-phase6-compliance-and-analytics.md) | §14 — ClickHouse schema, bulk insertion, report generation, PDF signing, compliance API, Phase 6 acceptance criteria |
| [14-phase7-security-hardening.md](14-phase7-security-hardening.md) | §15 — HTTP security headers, SSRF protection, safe logger, secret rotation runbooks, idempotency key constraints, Phase 7 acceptance criteria |
| [15-phase8-load-testing.md](15-phase8-load-testing.md) | §16 — k6 test scripts, tuning table, Phase 8 acceptance criteria |
| [16-phase9-documentation.md](16-phase9-documentation.md) | §17 — Required documents, OpenAPI specs, operational runbooks, Phase 9 acceptance criteria |
| [17-phase10-dlp.md](17-phase10-dlp.md) | §18 — DLP DB schema, scanning engine, integration flow, DLP API, Phase 10 acceptance criteria |
| [18-disaster-recovery.md](18-disaster-recovery.md) | §18.5–18.6 — RTO/RPO targets, PostgreSQL/MongoDB/Kafka/Redis recovery, multi-region topology, acceptance criteria |
| [19-cross-cutting-concerns.md](19-cross-cutting-concerns.md) | §19 — Structured logging, distributed tracing, graceful shutdown, health checks, idempotency, request validation, testing standards |
| [20-full-system-acceptance-criteria.md](20-full-system-acceptance-criteria.md) | §20 — 45-step end-to-end scenario |
| [21-appendix-trade-offs.md](21-appendix-trade-offs.md) | Appendix A — Known trade-offs table |
