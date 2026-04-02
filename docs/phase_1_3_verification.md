# OpenGuard Phase 1–3 Verification Report

## Phase 1 — Foundation (IAM + Control Plane API)

### ✅ Implemented

| Component | Status |
|---|---|
| IAM DB Migrations (orgs, users, sessions, mfa_configs, api_tokens, outbox) | ✅ All 7 migrations present with RLS |
| Outbox relay (PostgreSQL → Kafka via `LISTEN/NOTIFY`) | ✅ In [shared/outbox/relay.go](file:///Users/egannguyen/Documents/GitHub/Open-Guard/shared/outbox/relay.go) |
| Control Plane router (API key auth + JWT admin auth routes) | ✅ In [services/controlplane/pkg/router/router.go](file:///Users/egannguyen/Documents/GitHub/Open-Guard/services/controlplane/pkg/router/router.go) |
| Connector registration + API key issuance | ✅ `POST /api/v1/admin/connectors` |
| API key auth middleware (`APIKeyAuth`) | ✅ In `shared/middleware` |
| JWT multi-key rotation support (`JWTKeyring`) | ✅ In `shared/crypto/jwt.go` |
| RLS on all org-scoped tables | ✅ All migrations have `ALTER TABLE ... ENABLE ROW LEVEL SECURITY` |
| mTLS inter-service (`NewMTLSServer`, `NewMTLSClient`) | ✅ In `shared/middleware/mtls.go` |
| Circuit breakers on all upstream upstream calls | ✅ In `controlplane/router.go` via `resilience.NewBreaker` |

### ⚠️ Gaps / Issues

| Issue | Severity |
|---|---|
| **Outbox relay missing dead-letter-queue logic**: Spec requires marking records `dead` after 5 failures; relay only increments `attempts` and never marks `dead` or publishes to `outbox.dlq` | **High** |
| IAM integration test (`services/iam/integration_test.go`) expects `auth/refresh` and MFA endpoints to return `501` — but these are `NOT` explicitly routed as stubs in the router file reviewed. Actual behavior needs verification | Medium |
| `CORS AllowedOrigins` is hardcoded to `http://localhost:3000` in control-plane router — needs env-var override for staging/prod | Low |

### 🧪 Test Coverage

| Test Type | Status |
|---|---|
| IAM unit tests (`pkg/service/auth_test.go`, `pkg/handlers/`) | ✅ Present |
| IAM integration test (`integration_test.go -tags=integration`) | ✅ Present — covers register, login, full user CRUD, token CRUD |
| IAM router test | ✅ Present |

---

## Phase 2 — Policy Engine

### ✅ Implemented

| Component | Status |
|---|---|
| Policy DB migrations (policies, assignments, eval_log, outbox) | ✅ All 5 migrations with RLS |
| Redis caching with `SCAN`-based invalidation (not `KEYS`) | ✅ In `evaluator.go` + `cache_invalidator.go` |
| Fail-closed evaluation: deny on DB error | ✅ Line 102 of `evaluator.go` |
| Policy type support: `ip_allowlist`, `data_export`, `anon_access`, `session_limit` | ✅ In `applyPolicy()` |
| Kafka-based cache invalidation consumer | ✅ `CacheInvalidator` subscribes to `policy.changes` |
| Outbox relay for policy changes | ✅ Running in `main.go` |
| Policy CRUD API (`/v1/policies`) | ✅ Routed through control-plane |
| Policy evaluation API (`/v1/policy/evaluate`) | ✅ Routed through control-plane |

### ⚠️ Gaps / Issues

| Issue | Severity |
|---|---|
| **RBAC policy type not implemented**: `applyPolicy()` has no case for `rbac` / role-based rules — the spec defines RBAC as a core type | **High** |
| Eval log is written asynchronously with `go func()` using `context.WithoutCancel` — if the service crashes, the log goroutine is lost. Not a data integrity risk but creates metrics gaps. | Low |
| Policy integration test sends evaluation requests to `/policies/evaluate`, but the control-plane router registered the path as `/v1/policy/*` (strip prefix → `/evaluate`). Path mismatch may cause test failures against real server | Medium |

### 🧪 Test Coverage

| Test Type | Status |
|---|---|
| Evaluator unit tests (`evaluator_test.go`, `evaluator_ext_test.go`) | ✅ Present |
| Cache invalidator unit test | ✅ Present |
| Policy CRUD unit tests | ✅ Present |
| Policy integration test (`integration_test.go -tags=integration`) | ✅ Present — covers create, list (with poll), evaluate permit, evaluate deny, delete |

---

## Phase 3 — Event Bus, Outbox Relay & Audit Log

### ✅ Implemented

| Component | Status |
|---|---|
| Kafka topic config in spec (`topics.json`) | Per spec — 11 topics defined |
| Outbox relay: PostgreSQL `LISTEN/NOTIFY` + 100ms polling fallback | ✅ In `shared/outbox/relay.go` |
| Audit bulk writer (max docs or flush interval) | ✅ In `services/audit/pkg/consumer/bulk_writer.go` |
| Kafka consumer for audit events | ✅ In `services/audit/pkg/consumer/kafka.go` |
| CQRS read/write split (primary write, secondary read) | ✅ Per service split repository layer |
| MongoDB bulk insert with `BulkWrite` (unordered) | ✅ Correctly uses `SetOrdered(false)` for duplicate tolerance |

### ⚠️ Gaps / Issues

| Issue | Severity |
|---|---|
| **Outbox dead-letter logic missing** (same as Phase 1 gap) — records are never moved to `outbox.dlq` topic | **High** |
| Hash chaining in audit log: `hash_chain.go` file listed in spec directory layout but not yet verified as fully implemented | Medium |
| Audit service integration test uses `testcontainers-go` with real MongoDB — does not test the Kafka consumer path end-to-end | Medium |

### 🧪 Test Coverage

| Test Type | Status |
|---|---|
| Bulk writer unit tests | ✅ Present |
| Bulk writer integration test (MongoDB real container) | ✅ Present (`-tags=integration`) |
| Kafka consumer unit tests (`kafka_test.go`) | ✅ Present |

---

## Frontend (Next.js) — API Usage

### ✅ Covered in `lib/api.ts`

| API | Used |
|---|---|
| `POST /api/v1/auth/login` | ✅ |
| `POST /api/v1/auth/register` | ✅ |
| `POST /api/v1/auth/refresh` | ✅ |
| `POST /api/v1/auth/logout` | ✅ |
| `GET /api/v1/users` | ✅ |
| `GET /api/v1/policies` | ✅ |
| `GET /api/v1/threats` | ✅ |
| `GET /api/v1/audit` | ✅ |
| `GET /api/v1/alerts` | ✅ |
| `GET /api/v1/admin/connectors` | ✅ |
| `POST /api/v1/admin/connectors` | ✅ |

### ❌ Missing Frontend API Calls

| API | Status |
|---|---|
| `POST /api/v1/policies` (create policy) | ❌ Not in `api.ts` |
| `PUT/DELETE /api/v1/policies/:id` | ❌ Not in `api.ts` |
| `GET /api/v1/compliance` | ❌ Not in `api.ts` |
| `PATCH /api/v1/admin/connectors/:id` (suspend/activate) | ❌ Not in `api.ts` |
| `POST /api/v1/admin/connectors/:id/suspend` | ❌ Not in `api.ts` |
| `GET /api/v1/policy/eval-logs` | ❌ Not in `api.ts` |

---

## E2E Tests (Playwright — `web/e2e/`)

| Test File | Coverage |
|---|---|
| `login.spec.ts` | Login flow |
| `register.spec.ts` | Registration flow |
| `dashboard.spec.ts` | Dashboard navigation |
| `policy.spec.ts` | Policy list + create (mocked API) |
| `audit.spec.ts` | Audit event listing (mocked) |
| `iam.spec.ts` | User management (mocked) |
| `controlplane.spec.ts` | Connector registration (mocked) |
| `real.spec.ts` | Partial real API tests (register, login, dashboard) |

> [!WARNING]
> All Playwright E2E tests use **mocked API routes** (`page.route(...)`). There are no true end-to-end tests that hit real running services. `real.spec.ts` is the only partial exception but it uses `page.route` for some paths.

---

## Integration Tests (Go — `go test -tags=integration`)

| Service | File | Coverage |
|---|---|---|
| IAM | `services/iam/integration_test.go` | Register → Login → User CRUD → Token CRUD → Logout |
| Policy | `services/policy/integration_test.go` | Register → Create Policy → List Poll → Evaluate (permit + deny) → Delete |
| Audit | `services/audit/pkg/consumer/integration_test.go` | BulkWriter flush by maxDocs, by timeout, on cancel |

> [!IMPORTANT]
> Integration tests require a **running local stack** (`make dev`). They are real HTTP tests — not testcontainers. Run with: `make test-integration`

---

## Critical Gaps Summary

| # | Gap | Phase | Priority |
|---|---|---|---|
| 1 | Outbox relay does not mark records `dead` after 5 failures or publish to `outbox.dlq` | 1 & 3 | 🔴 High |
| 2 | RBAC policy type not implemented in evaluator | 2 | 🔴 High |
| 3 | Policy integration test evaluates against wrong path (`/policies/evaluate` vs `/v1/policy/*`) | 2 | 🟡 Medium |
| 4 | No full E2E test touching real backend (Playwright tests are all mocked) | All | 🟡 Medium |
| 5 | Audit hash chain verifier not fully verified | 3 | 🟡 Medium |
| 6 | Frontend missing API calls for policy CRUD, compliance, connector suspend | 1 & 2 | 🟡 Medium |
