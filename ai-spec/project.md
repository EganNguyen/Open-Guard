# OpenGuard — AI Agent Master Index

> **Purpose:** Central router for AI Agent. Read this file first on every task.
> It tells you which spec document, rule, and rules apply to the work at hand.
> Never skip this file — it prevents contradictions between FE and BE and keeps
> all generated code CI-compliant from the first commit.

---

## 0. What Is OpenGuard?

OpenGuard is an open-source, self-hostable **enterprise security control plane**.
Connected applications integrate via SDK, SCIM 2.0, OIDC/SAML, and outbound
webhooks. User traffic never flows *through* OpenGuard — it is a governance hub,
not a proxy.

**Core services (Go microservices):**

| Service | Module path | Primary responsibility |
|---|---|---|
| `control-plane` | `services/control-plane` | Org lifecycle, tenant management |
| `iam` | `services/iam` | Auth, OIDC/SAML, SCIM, MFA, JWT |
| `connector-registry` | `services/connector-registry` | App registration, API credentials |
| `policy` | `services/policy` | RBAC evaluation, cache, fail-closed |
| `audit` | `services/audit` | Not Implemented (v1 Stub) |
| `threat` | `services/threat` | Not Implemented (v1 Stub) |
| `alerting` | `services/alerting` | Not Implemented (v1 Stub) |
| `webhook-delivery` | `services/webhook-delivery` | Not Implemented (v1 Stub) |
| `compliance` | `services/compliance` | Not Implemented (v1 Stub) |
| `dlp` | `services/dlp` | Not Implemented (v1 Stub) |

**Frontend:** Angular 19+ Admin Dashboard at `web/` — TypeScript, Standalone Components,
Angular Signals, HttpClient Services, SseService for real-time data.

---

## 1. Rule Router — Read the Rule Before Writing Any Code

| Task type | Rule file to read first |
|---|---|
| Any Go backend code (new service, handler, repository, outbox, migration) | `rules/openguard-golang-backend/rules.md` |
| Any Angular frontend code (component, service, guard, signal, interceptor) | `rules/openguard-angular-frontend/rules.md` |
| Both at once (e.g. a new feature end-to-end) | Read **both** rule files before writing anything |

> **Rule:** If AI Agent starts writing code without reading the relevant rule
> file first, stop and read it. The rule files encode CI-enforced patterns —
> deviating from them blocks the PR.

---

## 2. Spec Document Router — Which File Answers Your Question?

### 2.1 Backend Spec (`be_open_guard/`)

| Question / Task | Spec file |
|---|---|
| What is OpenGuard? SLOs? Design principles? | `01-overview-and-architecture.md` |
| Repository layout, directory tree, Go workspace | `02-repository-layout.md` |
| Kafka envelope, Outbox record, shared models, sentinel errors | `03-shared-contracts.md` |
| All environment variables, `.env.example`, config loading | `04-environment-and-config.md` |
| PostgreSQL RLS, DB roles, OrgPool, per-tenant quotas, ClickHouse | `05-multi-tenancy-and-rls.md` |
| Outbox table DDL, Writer, Relay, business handler pattern | `06-transactional-outbox.md` |
| Circuit breakers, bcrypt worker pool, retry, bulkhead | `07-circuit-breakers-and-resilience.md` |
| Docker Compose, CI pipeline, Prometheus metrics, Helm | `08-phase1-infra-ci-observability.md` |
| IAM DB schema, JWT keyring, MFA, WebAuthn, SCIM v2, OIDC | `09-phase2-foundation-and-auth.md` |
| Policy DB schema, Redis cache, policy service, webhook push | `10-phase3-policy-engine.md` |
| Kafka topics, audit CQRS, hash chain, MongoDB schema | `11-phase4-event-bus-and-audit.md` |
| Threat detectors, alert saga, SIEM webhook, MTTR | `12-phase5-threat-and-alerting.md` |
| ClickHouse schema, report generation, PDF signing | `13-phase6-compliance-and-analytics.md` |
| HTTP security headers, SSRF protection, secret rotation, idempotency | `14-phase7-security-hardening.md` |
| k6 load tests, SLO verification | `15-phase8-load-testing.md` |
| OpenAPI specs, operational runbooks | `16-phase9-documentation.md` |
| DLP DB schema, scanning engine, entropy scanner | `17-phase10-dlp.md` |
| Disaster recovery, RTO/RPO, multi-region | `18-disaster-recovery.md` |
| Structured logging, distributed tracing, graceful shutdown, health checks | `19-cross-cutting-concerns.md` |
| End-to-end acceptance criteria (45-step scenario) | `20-full-system-acceptance-criteria.md` |
| Trade-off decisions and rationale | `21-appendix-trade-offs.md` |
| Code quality standards, forbidden patterns, review checklist | `00-code-quality-standards.md` |

### 2.2 Frontend Spec (`fe_open_guard/`)

| Question / Task | Spec file |
|---|---|
| Tech stack, project structure, naming conventions | `00-tech-stack-and-conventions.md` |
| Design tokens, color palette, typography, dark/light mode | `01-design-system.md` |
| Typed API client, auth interceptors, SSE service, pagination | `02-api-client-layer.md` |
| OIDC AuthService, TOTP/WebAuthn MFA screens, session refresh | `03-auth-and-session.md` |
| App shell, sidebar, org switcher, breadcrumbs, global search | `04-dashboard-and-layout.md` |
| Connector list, registration wizard, API key reveal, webhook log | `05-connectors.md` |
| Policy list, RBAC rule builder, evaluate playground, eval log | `06-policy-engine-ui.md` |
| Audit stream, filter panel, cursor pagination, export jobs | `07-audit-log.md` |
| Alert list, detector cards, saga timeline, SIEM config, MTTR | `08-threat-and-alerting.md` |
| Report wizard, job polling, PDF preview, posture dashboard | `09-compliance-reports.md` |
| DLP policy editor, findings table, masking, entropy config | `10-dlp.md` |
| User list, user detail, MFA status, SCIM saga, org settings | `11-user-and-org-management.md` |
| System health, outbox lag, circuit breaker status, Kafka charts | `12-observability-and-admin.md` |
| Jasmine/Karma, Testing Library, Playwright, accessibility, perf | `13-testing-and-quality.md` |
| All env vars, `angular.json`, `tsconfig`, Tailwind | `14-environment-and-config.md` |
| TypeScript domain types, Zod validators, SSE event types | `15-validators-and-types.md` |
| Angular Signals, Reactive Services, Router state | `16-state-management.md` |
| Angular Router, CanActivate Guards, HttpInterceptors | `17-route-handlers-and-middleware.md` |
| Canonical patterns: paginated table, SSE table, optimistic toggle | `18-component-patterns.md` |
| Full-system E2E acceptance checklist | `19-acceptance-criteria.md` |
| Frontend trade-offs, out-of-scope features for v1 | `20-appendix-trade-offs.md` |
 
 ### 2.3 Test Strategy & Scenarios (`test_cases/`)
 
 | Question / Task | Spec file |
 |---|---|
 | Full test strategy, unit/integration/E2E cases for Phases 1–10 | `test_cases/test_cases.md` |

---

## 3. Development Phases — What to Build in Order

Each phase has strict acceptance criteria that must pass before the next phase starts.

```
Phase 1  →  Infra, CI/CD, Observability         be_open_guard/08-phase1-infra-ci-observability.md
Phase 2  →  Foundation & Authentication          be_open_guard/09-phase2-foundation-and-auth.md
Phase 3  →  Policy Engine                        be_open_guard/10-phase3-policy-engine.md
Phase 4  →  Event Bus & Audit                    be_open_guard/11-phase4-event-bus-and-audit.md
Phase 5  →  Threat Detection & Alerting          be_open_guard/12-phase5-threat-and-alerting.md
Phase 6  →  Compliance & Analytics               be_open_guard/13-phase6-compliance-and-analytics.md
Phase 7  →  Security Hardening                   be_open_guard/14-phase7-security-hardening.md
Phase 8  →  Load Testing & SLO Verification      be_open_guard/15-phase8-load-testing.md
Phase 9  →  Documentation                        be_open_guard/16-phase9-documentation.md
Phase 10 →  DLP                                  be_open_guard/17-phase10-dlp.md

Frontend phases run in parallel with BE Phase 2+, tracked in fe_open_guard/19-acceptance-criteria.md
```

> Before starting any phase, re-read its spec file and verify the previous
> phase's acceptance criteria are met.

---

## 4. Absolute Rules — Backend (Go)

These are CI-enforced. Violation = PR blocked. No exceptions.

```
✗  No direct Kafka producer calls from business handlers — use Outbox relay only
✗  No string concatenation in SQL — parameterized queries ($1, $2) always
✗  No time.Sleep in service code — use time.NewTicker inside select{}
✗  No interfaces defined in shared/ — define them in the consuming package
✗  No raw goroutines for bcrypt — use bounded AuthWorkerPool
✗  No cross-service pkg/ imports — services are isolated
✗  No shared/utils or shared/helpers — every package must have a domain name
✗  No mutable package-level variables (except pre-compiled regexp, sentinel errors)
✗  No Kafka offset commit before successful downstream write
✗  No org_id from client-supplied headers in SCIM endpoints — derive from token
✗  No _ = err — every error must be handled or logged
✗  No RLS-scoped table without explicit org_id UUID column
✗  No inter-service HTTP call without circuit breaker + timeout + fallback
✗  No policy failure mode that is not fail-closed
✗  No webhook delivery state held only in memory — persist to PostgreSQL
```

---

## 5. Absolute Rules — Frontend (Angular)

These are CI-enforced. Violation = PR blocked. No exceptions.

```
✗  No raw fetch in components — all API calls through src/app/core/services/*
✗  No tokens or org_id in localStorage — secure cookies via AuthService only
✗  No org-scoped route without AuthGuard + OrgGuard check
✗  No org_id interpolated from URL params — always from authenticated session
✗  No uncontrolled inputs — all forms use Angular Reactive Forms + Zod
✗  No raw WebSocket connections — use SseService for all real-time streams
✗  No single-click destructive actions — ConfirmDialog with resource name typed
✗  No page without an error boundary
✗  No sensitive data (email, ip_address, token_prefix) outside RedactableComponent
✗  No inline scripts or inline styles outside Scoped CSS / Tailwind
✗  No any TypeScript type — CI lint failure
✗  No console.log in committed code
✗  No manual subscriptions for data — use async pipe or toSignal()
✗  No polling with setInterval — use RxJS timer() or signal-based polling
✗  No hard-coded org_id strings — use AuthService.currentOrgId() signal
```

---

## 6. Canonical Patterns Quick Reference

### Backend

| Pattern | Where to look |
|---|---|
| Transactional Outbox (Writer + Relay) | `be_open_guard/06-transactional-outbox.md`, `shared/kafka/outbox/` |
| RLS context setup (`WithOrgID`, `SetSessionVar`) | `be_open_guard/05-multi-tenancy-and-rls.md`, `shared/rls/context.go` |
| Circuit breaker wrap | `be_open_guard/07-circuit-breakers-and-resilience.md`, `shared/resilience/breaker.go` |
| bcrypt worker pool | `be_open_guard/07-circuit-breakers-and-resilience.md` §8.2 |
| JWT multi-key keyring | `be_open_guard/09-phase2-foundation-and-auth.md`, `shared/crypto/jwt.go` |
| Kafka idempotent producer + manual commit consumer | `be_open_guard/03-shared-contracts.md`, `shared/kafka/` |
| API key fast-hash prefix → Redis → PBKDF2 fallback | `be_open_guard/05-multi-tenancy-and-rls.md` §6.4, `shared/middleware/apikey.go` |
| SCIM bearer auth (token-derived org_id only) | `be_open_guard/09-phase2-foundation-and-auth.md`, `shared/middleware/scim.go` |
| HMAC hash chain (audit) | `be_open_guard/11-phase4-event-bus-and-audit.md` §12.3 |
| Graceful shutdown (30s window) | `be_open_guard/19-cross-cutting-concerns.md` §19.3 |
| SafeAttr structured logging | `be_open_guard/19-cross-cutting-concerns.md` §19.1, `shared/telemetry/logger.go` |

### Frontend

| Pattern | Where to look |
|---|---|
| Typed API client + error handling | `fe_open_guard/02-api-client-layer.md`, `src/app/core/services/api.service.ts` |
| SSE real-time stream service | `fe_open_guard/02-api-client-layer.md` §2.5, `src/app/core/services/sse.service.ts` |
| Cursor-paginated table | `fe_open_guard/18-component-patterns.md` §18.2 |
| Offset-paginated table | `fe_open_guard/18-component-patterns.md` §18.1 |
| SSE real-time table | `fe_open_guard/18-component-patterns.md` §18.3 |
| Optimistic status toggle | `fe_open_guard/18-component-patterns.md` §18.4 |
| Job-status polling | `fe_open_guard/18-component-patterns.md` §18.5 |
| API key one-time reveal | `fe_open_guard/18-component-patterns.md` §18.7 |
| Confirmation modal (destructive actions) | `fe_open_guard/16-state-management.md`, `ConfirmService` |
| AuthGuard | `fe_open_guard/04-dashboard-and-layout.md` |
| Redactable component | `fe_open_guard/18-component-patterns.md` |

---

## 7. Shared Contracts (Immutable)

Defined in `github.com/openguard/shared/models` (BE) and mirrored in
`src/app/core/models/` (FE). **Rename = major version bump of shared module +
migration of all consumers.**

| Contract | File |
|---|---|
| `EventEnvelope` — Kafka wire format | `be_open_guard/03-shared-contracts.md` §4.1 |
| `OutboxRecord` | `be_open_guard/03-shared-contracts.md` §4.2 |
| `SagaEvent` | `be_open_guard/03-shared-contracts.md` §4.3 |
| Kafka topic registry | `be_open_guard/03-shared-contracts.md` §4.4 |
| `User`, `ConnectedApp` models | `be_open_guard/03-shared-contracts.md` §4.5–4.6 |
| HTTP error response shape | `be_open_guard/03-shared-contracts.md` §4.7 |
| Sentinel errors | `be_open_guard/03-shared-contracts.md` §4.8 |
| TypeScript domain types + Zod validators | `fe_open_guard/15-validators-and-types.md` |

---

## 8. Infrastructure & Tooling

| Concern | Source |
|---|---|
| Docker Compose (all services, healthchecks) | `be_open_guard/08-phase1-infra-ci-observability.md` §9.1 |
| GitHub Actions CI pipeline | `be_open_guard/08-phase1-infra-ci-observability.md` §9.2 |
| Prometheus metrics catalogue | `be_open_guard/08-phase1-infra-ci-observability.md` §9.3 |
| Alertmanager rules | `be_open_guard/08-phase1-infra-ci-observability.md` §9.4 |
| Helm chart structure | `be_open_guard/08-phase1-infra-ci-observability.md` §9.5 |
| All background env vars | `be_open_guard/04-environment-and-config.md` |
| All frontend env vars + angular.json | `fe_open_guard/14-environment-and-config.md` |
| mTLS cert generation script | `be_open_guard/02-repository-layout.md` (`scripts/gen-mtls-certs.sh`) |
| Makefile targets | `be_open_guard/09-phase2-foundation-and-auth.md` §10.1 |

---

## 9. Performance Targets (Hard SLOs — Verified by Phase 8 k6 Tests)

| Operation | p99 target | Throughput |
|---|---|---|
| `POST /oauth/token` (OIDC) | 150ms | 2,000 req/s |
| `POST /v1/policy/evaluate` (uncached) | 30ms | 10,000 req/s |
| `POST /v1/policy/evaluate` (Redis cached) | 5ms | 10,000 req/s |
| `GET /audit/events` (paginated) | 100ms | 1,000 req/s |
| Kafka event → audit DB insert | 2s | 50,000 events/s |
| `POST /v1/events/ingest` (connector) | 50ms | 20,000 req/s |
| Compliance report generation | 30s | 10 concurrent |
| DLP async scan | 500ms | — |

> Bcrypt at cost 12 takes 250–400ms/op. The AuthWorkerPool sized to `2×NumCPU`
> is **mandatory** — without it, 2,000 req/s IAM throughput requires ~35 CPU cores.
> See `be_open_guard/07-circuit-breakers-and-resilience.md` §8.2.

---

## 10. Security Non-Negotiables

- **Policy engine failure mode: fail closed.** SDK caches for 60s; after TTL expires, deny.
- **JWT revocation:** check `jti` in Redis blocklist after signature + exp validation. Blocklist TTL = `exp - now()`.
- **RLS:** every org-scoped table has RLS enabled with `NULLIF(current_setting('app.org_id', true), '')::UUID`.
- **API key hot path:** fast-hash prefix → Redis; PBKDF2-HMAC-SHA512 (600k iterations) only on cache miss.
- **Webhook HMAC:** HMAC-SHA256 via `shared/crypto/hmac.go`. Recipients must verify signature before processing.
- **mTLS:** all internal service-to-service calls use mTLS.
- **SSRF protection:** apply `shared/middleware/security.go` SSRF guard to all outbound HTTP (webhooks, SCIM).
- **Secret rotation:** JWT and MFA keys use multi-key keyrings (`kid`-based). Multiple valid keys coexist during rotation window.

---

## 11. How AI Agent Should Approach Any Task

```
1. Read this file (project.md) — you are here.
2. Identify task type: Backend Go? Frontend Angular 19? Both?
3. Read the relevant rules.md file(s) from ai-spec/rules/.
4. Identify which spec file(s) answer the specific question (§2 above).
5. Read those spec sections before writing any code.
6. Apply the absolute rules (§4, §5) — treat them as pre-flight checks.
7. Use canonical patterns from §6 — do not invent alternatives.
8. Never touch shared contracts (§7) without a major version bump plan.
9. After writing code, verify against the phase acceptance criteria.
10. If a task spans a phase boundary, confirm the earlier phase's criteria first.
```

---

## 12. File Layout Reference

```
openguard/
├── ai-spec/                            ← YOU ARE HERE
│   ├── project.md                      ← Master Index
│   ├── rules/                          ← CI-enforced coding patterns
│   │   ├── openguard-golang-backend/rules.md
│   │   └── openguard-angular-frontend/rules.md
│   ├── be_open_guard/                  ← Backend spec (22 files)
│   │   ├── README.md                   ← BE doc index
│   │   └── 00-*.md … 21-*.md
│   ├── fe_open_guard/                  ← Frontend spec (21 files)
│   │   ├── README.md                   ← FE doc index
│   │   └── 00-*.md … 20-*.md
│   └── test_cases/                     ← E2E test scenarios
├── services/                          ← Go microservices (one dir per service)
├── sdk/                               ← Go SDK (policy client + event publisher)
├── shared/                            ← Shared Go module (contracts, middleware, crypto)
├── web/                               ← Angular 19+ Admin Dashboard
├── infra/
│   ├── docker/docker-compose.yml
│   ├── kafka/topics.json
│   └── helm/
├── scripts/
│   ├── gen-mtls-certs.sh
│   └── create-topics.sh
└── Makefile                           ← dev, test, lint, build, migrate, seed, load-test, certs
```
