# OpenGuard — Production Architecture Review

> Reviewed: Full codebase + AI-spec (BE + FE), all 10 phases, example app, SDK  
> Date: 2026-04-24 | Reviewer: Senior System Architect

---

## Executive Summary

| Area | Completion | Status |
|---|---|---|
| **Phase 1** — Infra / CI / Observability | **90%** | ✅ Near-complete |
| **Phase 2** — Foundation & Auth | **78%** | ⚠️ Bugs + missing WebAuthn/SAML |
| **Phase 3** — Policy Engine | **85%** | ⚠️ Cache TTL mismatch, no idempotency key middleware |
| **Phase 4** — Event Bus & Audit | **80%** | ⚠️ Audit SSE uses raw EventSource, no Redis backup |
| **Phase 5** — Threat Detection & Alerting | **82%** | ⚠️ Detectors live but commit-before-write rule violated |
| **Phase 6** — Compliance & Analytics | **60%** | 🔴 ClickHouse writer stub, no scheduled report generation |
| **Phase 7** — Security Hardening | **75%** | ⚠️ DLP exposes raw PII, idempotency middleware missing |
| **Phase 8** — Load Testing / SLO | **55%** | 🔴 5/6 k6 scripts exist, no kafka-throughput script, no CI gate |
| **Phase 9** — Documentation | **70%** | ⚠️ OpenAPI partial, runbooks exist |
| **Phase 10** — DLP | **65%** | 🔴 Raw PII in scan response, async mode missing |
| **Frontend (Angular)** | **68%** | 🔴 0 test files, many `any` types, no SseService, no OrgGuard |
| **Example App / SDK** | **80%** | ⚠️ SDK cache not circuit-breaker-wrapped |
| **Overall Production Readiness** | **73%** | ⚠️ Not production-ready as-is |

---

## 1. Critical Bugs (Compile / Runtime Failures)

### BUG-01 — `RegisterUser` Wrong Return Arity (Compile Error)
**File:** `services/iam/pkg/service/service.go` lines 121, 128, 134

Function signature is `(string, bool, error)` but three early-exit returns emit only `("", err)` — **will not compile**.

```go
// WRONG (lines 121, 128, 134):
return "", err
// FIX:
return "", false, err
```

**Priority: P0 — blocks IAM service from building.**

### BUG-02 — Brute Force Detector Commits Offset Before Downstream Write
**File:** `services/threat/pkg/detector/brute_force.go` line 82

The spec (absolute rule §4) states: *"No Kafka offset commit before successful downstream write."*  
`CommitMessages` is called immediately after `processEvent`, without checking if Redis write or alert store succeeded.

```go
// WRONG:
d.processEvent(ctx, m)
d.reader.CommitMessages(ctx, m)

// FIX: CommitMessages must only be called after processEvent returns nil error
```

**Priority: P0 — threat events can be silently lost.**

### BUG-03 — DLP `/v1/dlp/scan` Returns Raw PII Values in HTTP Response
**File:** `services/dlp/pkg/scanner/regex.go` line 89, `services/dlp/pkg/handlers/handler.go`

The scanner stores the matched credit card number, SSN, etc. in `Finding.Value` and the handler `json.NewEncoder(w).Encode(allFindings)` streams them back to the caller unredacted. This is a **data breach vector** — any authenticated app can exfiltrate PII via the scan endpoint.

```go
// FIX: mask before returning
Finding{Value: mask(m, rule.Kind)}  // e.g., "****-****-****-1234"
// Or: omit Value field entirely from API response; keep only Kind + RiskScore
```

**Priority: P0 — security/compliance breach.**

---

## 2. Production Readiness

### 2.1 Code Quality

| Finding | Severity | File |
|---|---|---|
| `RegisterUser` compile error (BUG-01) | P0 | `services/iam/pkg/service/service.go` |
| Brute-force offset commit before write (BUG-02) | P0 | `services/threat/pkg/detector/brute_force.go` |
| DLP returns raw PII (BUG-03) | P0 | `services/dlp/pkg/handlers/handler.go` |
| `console.log` in committed SSR code (spec rule §5) | P1 | `web/src/server.ts` lines 42–70 |
| `localStorage` use for sidebar state (spec rule §5: "No tokens or org_id in localStorage") | P2 | `web/src/app/core/state/ui.service.ts` — sidebar preference is not sensitive; however the rule is absolute and CI will fail |
| `any` types in 15+ FE service files — CI lint failure | P1 | `connector.service.ts`, `audit.service.ts`, `auth.service.ts`, `threats.ts`, etc. |
| Migration runner uses "already exists" string match instead of `golang-migrate` | P2 | `shared/database/migrate.go` — not idempotent for all error types |

### 2.2 Scalability / Reliability

| Finding | Severity | Detail |
|---|---|---|
| Policy cache TTL is 30s in code vs. 60s in spec | P1 | `service.go:25` — SDK fail-closed window is based on TTL; 30s = half the expected grace period |
| Migration lock heartbeat TTL is 60s but Expire call is inside a goroutine that may miss renewal if Postgres migration takes >60s | P2 | `shared/database/migrate.go` — extend `lockTTL` or use a longer value |
| `audit.service.ts` opens raw `EventSource` directly in service layer — bypasses re-connect, auth header injection | P1 | No `SseService` class exists; spec requires it |
| `BruteForceDetector` creates its own `redis.NewClient` ignoring pool config | P2 | All detectors do this — each spawns an independent Redis connection instead of sharing pool |
| `threat/main.go`: `ImpossibleTravelDetector` silently skips startup if GeoDB missing — no health-check failure | P2 | Service appears healthy but 1 of 6 detectors is offline |

### 2.3 Security

| Finding | Severity | Detail |
|---|---|---|
| Raw PII in DLP response (BUG-03) | P0 | Credit card #, SSN, AWS keys returned to caller |
| `authInterceptor` comment says "in a real app" — token handling appears incomplete | P1 | Cookie-based auth relies on `withCredentials: true` which is correct for HttpOnly cookies but there's no CSRF protection layer |
| SSE endpoint `ALLOWED_ORIGIN: "*"` fallback is dangerous for authenticated streams | P1 | `services/audit/pkg/handlers/sse.go:37` |
| `auth.service.ts` stores no token but navigates to `/` after login — session validation only on init | P2 | Race condition: if `/auth/me` is slow, routes are accessible before auth confirmed |

### 2.4 Observability

| Finding | Severity | Detail |
|---|---|---|
| No structured metrics for DLP scan latency, finding counts by type | P2 | `services/dlp/pkg/telemetry/metrics.go` exists but is essentially empty |
| `compliance` service has no consumer lag metric | P2 | ClickHouse writer is a stub |
| `threat/main.go` does not export per-detector `active` gauge | P2 | Spec §13 requires detector health metrics |

---

## 3. AI-Spec Consistency

### 3.1 Implementation Deviates from Spec

| Spec Reference | Spec Says | Code Does | Impact |
|---|---|---|---|
| `project.md §9` | Policy cache TTL = 60s (SDK fail-closed) | `service.go:25` cacheTTL = 30s | SDK denies 30s earlier than expected |
| `be_open_guard/11-phase4` | Audit HMAC key via `shared/crypto` keyring | `audit/consumer.go` reads `AUDIT_SECRET_KEY` env directly | Not multi-key, not rotatable |
| `be_open_guard/09-phase2` | WebAuthn credential table `005_create_mfa_configs.up.sql` | Migration exists but `pkg/handlers/handler.go` has no WebAuthn registration/authentication endpoint | No FIDO2 support in runtime |
| `be_open_guard/08-phase1` | Kafka-throughput k6 test (`kafka-throughput.js`) | File does not exist; only 5 of 6 scripts present | Phase 8 acceptance criteria cannot pass |
| `be_open_guard/14-phase7` | Idempotency key middleware on mutating endpoints | No `Idempotency-Key` middleware in any router; `ApiService.post()` accepts idempotency key client-side but it's never enforced server-side | Duplicate mutations possible |
| `be_open_guard/09-phase2` | SAML 2.0 SP support | No SAML handler, no library import in IAM go.mod | Missing enterprise SSO |
| `fe_open_guard/02-api-client-layer §2.5` | `SseService` class wrapping `EventSource` with reconnect | `AuditService.streamEvents()` creates raw `EventSource` inline | No reconnect, no auth injection for SSE |
| `fe_open_guard/13-testing-quality` | Jasmine/Karma unit tests, Playwright E2E | 0 `.spec.ts` files in `web/src` | Zero frontend test coverage |

### 3.2 Implemented but Undocumented in Spec

| Implementation | Spec Gap |
|---|---|
| `shared/database/migrate.go` custom SQL runner | Spec assumes `golang-migrate` tool; custom runner lacks version tracking, dirty-state detection |
| `services/compliance/pkg/consumer/clickhouse_writer.go` | Spec §14 describes ClickHouse schema but stub has no schema; ClickHouse not in docker-compose |
| `packages/detectors/` (TypeScript detector package) | Not mentioned anywhere in BE or FE spec — parallel detector system of unclear purpose |
| `packages/dashboard/` (React/Vite dashboard) | Spec only describes Angular 19 dashboard; second React dashboard is undocumented |
| `apps/example/` and `examples/task-management-app/` | Two separate example apps; spec only mentions one; task-management-app has a compiled binary committed (`backend/backend`) |

---

## 4. Spec Gaps

### 4.1 Missing from AI-Spec (needs documentation)

| Gap | Recommended Spec Addition |
|---|---|
| Custom SQL migration runner vs `golang-migrate` — choice not explained | Add §10.2a: "Custom runner rationale and limitations" |
| `SseService` reconnect strategy + JWT injection for SSE streams | Add §2.5a to `fe_open_guard/02-api-client-layer.md` |
| `OrgGuard` (separate from `AuthGuard`) is required by spec §5 but never defined | Define `OrgGuard` in `fe_open_guard/17-route-handlers-and-middleware.md` |
| `RedactableComponent` is mandated but not implemented or documented with a concrete template | Add concrete implementation to `fe_open_guard/18-component-patterns.md §18.8` |
| Error boundary pattern for Angular 19 (no `ErrorBoundary` in Angular — requires custom `ErrorHandler`) | Add §18.9 to `fe_open_guard/18-component-patterns.md` |
| Dual dashboard situation (React `packages/dashboard` vs Angular `web/`) | Clarify in `project.md` which is canonical; deprecate or integrate |
| ClickHouse not present in `docker-compose.yml` despite Phase 6 depending on it | Add ClickHouse + schema to Phase 6 infra section |
| DLP async scan mode (spec §17 mentions "async scan 500ms") | Define async scan queue contract in `be_open_guard/17-phase10-dlp.md` |

---

## 5. Phase Completeness

### Phase 1 — Infra / CI / Observability (90%)
✅ Docker Compose, Helm, Prometheus, Grafana dashboards, CI pipeline, Alertmanager rules, mTLS scripts  
❌ Missing: ClickHouse in docker-compose; `kafka-throughput.js` k6 script; CI does not gate on load-test results

### Phase 2 — Foundation & Authentication (78%)
✅ IAM service, JWT keyring, MFA TOTP, SCIM v2, bcrypt worker pool, outbox watcher, RLS migrations  
❌ Missing: WebAuthn endpoints (migration exists, no handler); SAML 2.0 SP; BUG-01 compile error

### Phase 3 — Policy Engine (85%)
✅ Policy service, Redis two-tier cache, singleflight, circuit breaker, outbox writer, policy CRUD  
❌ Cache TTL 30s ≠ spec 60s; no server-side idempotency key enforcement; policy evaluation playground endpoint missing

### Phase 4 — Event Bus & Audit (80%)
✅ Kafka consumers, HMAC hash chain, MongoDB bulk write, SSE stream, cursor pagination  
❌ Audit HMAC is single-key (not multi-key keyring); raw `EventSource` in FE instead of `SseService`; no export-to-CSV/S3 job

### Phase 5 — Threat Detection & Alerting (82%)
✅ 6 detectors (BruteForce, ImpossibleTravel, OffHours, DataExfil, AccountTakeover, PrivilegeEscalation) wired in `threat/main.go`; AlertSaga + SIEM webhook  
❌ BUG-02: offset commit before write; ImpossibleTravel silently disabled when GeoDB missing; no MTTR metric implementation

### Phase 6 — Compliance & Analytics (60%)
✅ Report generation handler, PDF signing (RSA), S3 storage, compliance handler structure  
❌ ClickHouse writer is mostly a stub with no real schema; no scheduled report cron/worker; no FINAL modifier in queries; compliance not in docker-compose

### Phase 7 — Security Hardening (75%)
✅ SSRF guard, security headers, API key hot-path (Redis prefix → PBKDF2), mTLS scripts, secret rotation design  
❌ BUG-03 raw PII in DLP response; no server-side idempotency middleware; SSE CORS fallback to `*`; no CSRF token for state-changing API calls

### Phase 8 — Load Testing / SLO (55%)
✅ 5 k6 scripts with correct SLO thresholds; Makefile `load-test` target  
❌ `kafka-throughput.js` missing; no CI step runs k6; SLO not enforced in pipeline; pre-seeded 10k user dataset script absent

### Phase 9 — Documentation (70%)
✅ OpenAPI specs for control-plane, IAM, policy; 7 runbooks  
❌ OpenAPI for audit, threat, compliance, DLP, webhook-delivery missing; SDK `README.md` exists but no auto-generated API docs

### Phase 10 — DLP (65%)
✅ Regex scanner (6 rules), entropy scanner, Luhn check, policy CRUD, findings persistence  
❌ BUG-03 raw PII response; async scan mode not implemented; masking/redaction not applied; DLP findings not published to Kafka for audit trail; custom rule creation not wired

### Frontend (68%)
✅ Angular 19 standalone components, HttpClient services, AuthGuard, error interceptor, Signals, Reactive Forms in policies/users, Zod in policy model  
❌ 0 test files (Jasmine/Karma/Playwright); no `SseService`; no `OrgGuard`; no `RedactableComponent`; no error boundaries; 15+ `any` types; `console.log` in server.ts; `localStorage` for UI state; `audit.service.ts` raw `EventSource`; no org switcher; no global search

### Connected Example App (80%)
✅ Express server, OpenGuard SDK integration, attack simulator UI, WebSocket live feed, guard config  
❌ `examples/task-management-app/backend/backend` is a committed binary (security risk); `apps/example` and `examples/task-management-app` overlap with no clear separation; SDK cache not circuit-breaker-wrapped per spec

---

## 6. Prioritized Recommendations

### P0 — Fix Before Any Deployment

| # | Fix | File |
|---|---|---|
| 1 | Fix `RegisterUser` — change 3× `return "", err` → `return "", false, err` | `services/iam/pkg/service/service.go:121,128,134` |
| 2 | Fix brute-force Kafka commit — move `CommitMessages` after successful `processEvent` | `services/threat/pkg/detector/brute_force.go` |
| 3 | Mask PII in DLP scan response — never return raw matched values | `services/dlp/pkg/scanner/regex.go` + handler |

### P1 — Fix Before Production Release

| # | Fix |
|---|---|
| 4 | Implement `SseService` in Angular per spec §2.5 — wrap `EventSource` with reconnect + auth headers |
| 5 | Remove all `console.log` from `web/src/server.ts` (CI violation) |
| 6 | Eliminate all `any` types in FE services — replace with typed interfaces |
| 7 | Add `OrgGuard` and apply it to all org-scoped routes |
| 8 | Implement server-side idempotency key middleware for mutating endpoints |
| 9 | Fix policy cache TTL: change `cacheTTL = 30s` → `60s` |
| 10 | Add ClickHouse to docker-compose and implement ClickHouse writer properly |
| 11 | Add `kafka-throughput.js` k6 test (xk6-kafka) |
| 12 | Implement WebAuthn endpoints (registration challenge, finish, authentication) |

### P2 — Before GA

| # | Fix |
|---|---|
| 13 | Replace `localStorage` for sidebar state with in-memory signal (use `sessionStorage` or eliminate) |
| 14 | Fix SSE CORS wildcard fallback — require `ALLOWED_ORIGIN` at startup |
| 15 | Upgrade audit HMAC to multi-key keyring (matching AES keyring pattern in shared/crypto) |
| 16 | Add per-detector health metrics and active gauge |
| 17 | Implement `RedactableComponent` for PII display |
| 18 | Write minimum 80% FE unit test coverage (policies, connectors, auth flow) |
| 19 | Add Playwright E2E for critical paths (login → MFA → policy eval → audit view) |
| 20 | Remove committed binary `examples/task-management-app/backend/backend` from git |
| 21 | Add SAML 2.0 SP support to IAM or document explicitly as out-of-scope v1 |
| 22 | Implement DLP async scan queue + findings Kafka event |

---

## 7. Spec Accuracy Score

| Spec Doc | Accuracy vs. Implementation |
|---|---|
| `be_open_guard/08-phase1` | 92% — docker-compose, CI, helm all match |
| `be_open_guard/09-phase2` | 75% — WebAuthn/SAML missing, compile bug |
| `be_open_guard/10-phase3` | 88% — cache TTL mismatch only major gap |
| `be_open_guard/11-phase4` | 80% — audit chain single-key, no export job |
| `be_open_guard/12-phase5` | 83% — all detectors present, commit-before-write bug |
| `be_open_guard/13-phase6` | 55% — ClickHouse stub |
| `be_open_guard/14-phase7` | 72% — DLP PII exposure, idempotency missing |
| `be_open_guard/15-phase8` | 60% — k6 script missing, no CI gate |
| `be_open_guard/17-phase10` | 65% — scanner good, response masking missing |
| `fe_open_guard/*` | 62% — structure correct, tests/SSE/types incomplete |

# Jobs
name: fix-p0-compile-and-security-bugs
description: >
  Fix three P0 bugs that block compilation or are security-critical.
  Must be completed and passing CI before any other work proceeds.

jobs:

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 1 — RegisterUser compile error: wrong return arity
  # ─────────────────────────────────────────────────────────────────────────────
  - id: fix-register-user-return-arity
    priority: P0
    title: "Fix RegisterUser — 3 wrong return statements (compile error)"
    file: services/iam/pkg/service/service.go
    description: >
      `RegisterUser` has signature `(string, bool, error)` but three early-exit
      paths return only `("", err)` — this is a Go compile error.
    changes:
      - find: 'tx, err := s.repo.BeginTx(ctx)\n\tif err != nil {\n\t\treturn "", err\n\t}'
        replace: 'tx, err := s.repo.BeginTx(ctx)\n\tif err != nil {\n\t\treturn "", false, err\n\t}'
        line_range: [119, 123]

      - find: 'userID, err := s.repo.CreateUser(ctx, orgID, email, string(hash), displayName, role, "initializing")\n\tif err != nil {\n\t\treturn "", err\n\t}'
        replace: 'userID, err := s.repo.CreateUser(ctx, orgID, email, string(hash), displayName, role, "initializing")\n\tif err != nil {\n\t\treturn "", false, err\n\t}'
        line_range: [126, 130]

      - find: 'if err := s.repo.UpdateUserSCIM(ctx, userID, scimExternalID, "initializing"); err != nil {\n\t\t\treturn "", err\n\t\t}'
        replace: 'if err := s.repo.UpdateUserSCIM(ctx, userID, scimExternalID, "initializing"); err != nil {\n\t\t\treturn "", false, err\n\t\t}'
        line_range: [132, 136]

    acceptance_criteria:
      - "`go build ./services/iam/...` exits 0"
      - "All existing `service_test.go` tests pass"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 2 — BruteForce Kafka offset commit before downstream write
  # ─────────────────────────────────────────────────────────────────────────────
  - id: fix-brute-force-kafka-commit-order
    priority: P0
    title: "Fix BruteForceDetector — CommitMessages called before processEvent succeeds"
    file: services/threat/pkg/detector/brute_force.go
    description: >
      Spec absolute rule: "No Kafka offset commit before successful downstream write."
      Currently `CommitMessages` is called unconditionally after `processEvent`,
      meaning failed Redis writes or alert store failures are silently lost.
    changes:
      - find: |
          d.processEvent(ctx, m)
          			d.reader.CommitMessages(ctx, m)
        replace: |
          if err := d.processEvent(ctx, m); err != nil {
          				d.logger.Error("processEvent failed, not committing offset", "error", err)
          				continue
          			}
          			if err := d.reader.CommitMessages(ctx, m); err != nil {
          				d.logger.Error("failed to commit kafka offset", "error", err)
          			}
    additional_changes:
      - description: "Change `processEvent` signature from void to `error`"
        file: services/threat/pkg/detector/brute_force.go
        detail: >
          Update `func (d *BruteForceDetector) processEvent(ctx context.Context, m kafka.Message)`
          to return `error`. Return the error from `trackFailedAttempt` if Redis pipeline fails.
          Return `nil` on success or unrecognized event type.

      - description: "Apply same fix pattern to all other detectors"
        files:
          - services/threat/pkg/detector/account_takeover.go
          - services/threat/pkg/detector/data_exfiltration.go
          - services/threat/pkg/detector/impossible_travel.go
          - services/threat/pkg/detector/off_hours.go
          - services/threat/pkg/detector/privilege_escalation.go

    acceptance_criteria:
      - "`go test ./services/threat/...` passes"
      - "Kafka offset is NOT committed when Redis pipeline returns error (verify via test mock)"
      - "`go vet ./services/threat/...` exits 0"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 3 — DLP Scan endpoint exposes raw PII in HTTP response
  # ─────────────────────────────────────────────────────────────────────────────
  - id: fix-dlp-pii-exposure-in-scan-response
    priority: P0
    title: "Fix DLP /scan — mask PII values before returning to caller"
    description: >
      `scanner.ScanRegex` stores the full matched value (SSN, credit card, AWS key, etc.)
      in `Finding.Value`. The handler returns `allFindings` directly via JSON encode.
      This is a critical data breach vector: any authenticated connector can exfiltrate PII
      by calling the scan endpoint.

    changes:
      - file: services/dlp/pkg/scanner/regex.go
        description: "Add MaskValue helper and apply it in ScanRegex"
        implementation: |
          // MaskValue redacts a finding value for safe API exposure.
          // Credit cards: keep last 4 digits. SSNs: "***-**-XXXX". Others: "[REDACTED]".
          func MaskValue(kind, value string) string {
              switch kind {
              case "credit_card":
                  digits := strings.Map(func(r rune) rune {
                      if r >= '0' && r <= '9' { return r }
                      return -1
                  }, value)
                  if len(digits) >= 4 {
                      return "****-****-****-" + digits[len(digits)-4:]
                  }
                  return "[REDACTED]"
              case "ssn":
                  if len(value) >= 4 {
                      return "***-**-" + value[len(value)-4:]
                  }
                  return "[REDACTED]"
              default:
                  return "[REDACTED]"
              }
          }

          // In ScanRegex, replace Value: m  with  Value: MaskValue(rule.Kind, m)

      - file: services/dlp/pkg/handlers/handler.go
        description: >
          Do NOT include raw matched value in the HTTP response.
          The response should contain: Kind, RiskScore, Location — never the raw matched string.
          Introduce a `ScanResponse` DTO that only exposes safe fields.
        implementation: |
          type ScanResponseFinding struct {
              Kind      string  `json:"kind"`
              RiskScore float64 `json:"risk_score"`
              Location  string  `json:"location,omitempty"`
          }

          // Convert findings before encoding:
          var safe []ScanResponseFinding
          for _, f := range allFindings {
              safe = append(safe, ScanResponseFinding{
                  Kind:      f.Kind,
                  RiskScore: f.RiskScore,
                  Location:  f.Location,
              })
          }
          json.NewEncoder(w).Encode(safe)

    acceptance_criteria:
      - "HTTP response from POST /v1/dlp/scan contains NO raw SSN, credit card, or AWS key values"
      - "Unit test: `ScanRegex` result `Value` field is masked, not the original match"
      - "Integration test: POST /v1/dlp/scan with content containing '4532015112830366' returns '****-****-****-0366'"

name: fix-p1-frontend-spec-compliance
description: >
  Fix P1 Angular frontend violations. These are CI-enforced rules per ai-spec/project.md §5.
  The linter will fail on `any` types and `console.log`. The SSE raw EventSource and missing
  OrgGuard are functional gaps that block production readiness.

jobs:

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 1 — Implement SseService (required by spec fe_open_guard/02-api-client-layer §2.5)
  # ─────────────────────────────────────────────────────────────────────────────
  - id: implement-sse-service
    priority: P1
    title: "Create SseService — wraps EventSource with reconnect, auth, typed events"
    file: web/src/app/core/services/sse.service.ts
    description: >
      Spec §2.5 mandates a shared `SseService` that all components use for real-time streams.
      Currently `AuditService.streamEvents()` creates a raw `EventSource` inline — no reconnect,
      no auth injection, no shared retry logic.
    implementation: |
      import { Injectable } from '@angular/core';
      import { Observable } from 'rxjs';
      import { environment } from '../../../environments/environment';

      export interface SseEvent<T = unknown> {
        type: string;
        data: T;
      }

      @Injectable({ providedIn: 'root' })
      export class SseService {
        private readonly baseUrl = environment.apiUrl;

        /**
         * Opens a typed SSE stream to `path`.
         * Automatically reconnects on error with exponential backoff (max 30s).
         * The browser sends HttpOnly cookies automatically (withCredentials via URL).
         */
        stream<T>(path: string): Observable<SseEvent<T>> {
          return new Observable(observer => {
            let retryDelay = 1000;
            let es: EventSource;
            let closed = false;

            const connect = () => {
              if (closed) return;
              es = new EventSource(`${this.baseUrl}${path}`, { withCredentials: true });

              es.onmessage = (event: MessageEvent) => {
                try {
                  const data: T = JSON.parse(event.data);
                  observer.next({ type: event.type || 'message', data });
                  retryDelay = 1000; // reset on success
                } catch (e) {
                  observer.error(e);
                }
              };

              es.onerror = () => {
                es.close();
                if (!closed) {
                  setTimeout(connect, retryDelay);
                  retryDelay = Math.min(retryDelay * 2, 30_000);
                }
              };
            };

            connect();

            return () => {
              closed = true;
              es?.close();
            };
          });
        }
      }

    follow_up_changes:
      - file: web/src/app/core/services/audit.service.ts
        description: >
          Replace raw EventSource with SseService.
          Replace `any` on `data` field with typed `AuditEvent`.
        implementation: |
          // Remove: new EventSource(...)  
          // Add: inject SseService
          // streamEvents returns: this.sseService.stream<AuditEvent>(`/audit/v1/events/stream`)
          //   .pipe(map(e => e.data))

    acceptance_criteria:
      - "No `new EventSource` calls exist in `web/src/app/core/services/`"
      - "`SseService` exists at `web/src/app/core/services/sse.service.ts`"
      - "AuditService uses SseService"
      - "`ng build` exits 0"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 2 — Implement OrgGuard
  # ─────────────────────────────────────────────────────────────────────────────
  - id: implement-org-guard
    priority: P1
    title: "Create OrgGuard — ensures authenticated user has access to the active org"
    file: web/src/app/core/guards/org.guard.ts
    description: >
      Spec §5 rule: "No org-scoped route without AuthGuard + OrgGuard check."
      AuthGuard exists. OrgGuard does not. All routes under the layout shell
      (connectors, policies, audit-logs, threats, dlp, compliance, users, admin)
      must require OrgGuard in addition to AuthGuard.
    implementation: |
      import { inject } from '@angular/core';
      import { CanActivateFn, Router } from '@angular/router';
      import { AuthService } from '../services/auth.service';

      export const orgGuard: CanActivateFn = () => {
        const auth = inject(AuthService);
        const router = inject(Router);
        const orgId = auth.currentOrgId();
        if (!orgId) {
          router.navigate(['/login']);
          return false;
        }
        return true;
      };

    route_changes:
      - file: web/src/app/app.routes.ts
        description: >
          Add `canActivate: [authGuard, orgGuard]` to every child route that renders
          org-scoped data (home, connectors, policies, audit-logs, threats, compliance,
          dlp, users, admin).

    acceptance_criteria:
      - "`web/src/app/core/guards/org.guard.ts` exists"
      - "All org-scoped routes in `app.routes.ts` list `[authGuard, orgGuard]`"
      - "`ng build` exits 0"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 3 — Eliminate `any` types in FE service layer
  # ─────────────────────────────────────────────────────────────────────────────
  - id: fix-frontend-any-types
    priority: P1
    title: "Replace all `any` types in services — CI lint failure"
    description: >
      Spec rule §5: "No `any` TypeScript type — CI lint failure."
      15+ occurrences exist across service files.
    files_to_fix:
      - path: web/src/app/core/services/connector.service.ts
        changes:
          - "Replace `connector: any` with `CreateConnectorRequest` and `UpdateConnectorRequest` typed interfaces"
          - "Define interfaces in `web/src/app/core/models/connector.model.ts`"

      - path: web/src/app/core/services/audit.service.ts
        changes:
          - "Replace `data: any` in AuditEvent with `data: Record<string, unknown>`"

      - path: web/src/app/core/services/auth.service.ts
        changes:
          - "Replace `credentials: any` with `LoginCredentials { email: string; password: string }`"
          - "Replace `oauthParams?: any` with `OAuthParams { client_id: string; redirect_uri: string; state?: string } | undefined`"
          - "Replace `Observable<any>` with typed response interfaces"

      - path: web/src/app/core/services/overview.service.ts
        changes:
          - "Replace `stats: any[]` with `DashboardStat[]` interface"
          - "Replace `health: any[]` with `ServiceHealth[]` interface"

      - path: web/src/app/core/services/api.service.ts
        changes:
          - "Replace `body: any` with `body: unknown` in post/put/patch — TypeScript allows `unknown` and it is safe"

      - path: web/src/app/threats/threats.ts
        changes:
          - "Define `ThreatEvent` interface with typed `metadata: Record<string, unknown>`"
          - "Replace `res: any` with `{ threats: ThreatEvent[] }`"

      - path: web/src/app/connectors/connectors.ts
        changes:
          - "Replace `res: any` with `{ connectors: Connector[] }`"

      - path: web/src/app/users/users.ts
        changes:
          - "Replace `{ connector: any, users: any[] }` with typed `ConnectorUserGroup` interface"

      - path: web/src/app/features/login/login.ts
        changes:
          - "Replace `oauthParams: any = null` with `oauthParams: OAuthParams | null = null`"

    acceptance_criteria:
      - "`npx eslint web/src --rule '{\"@typescript-eslint/no-explicit-any\": \"error\"}'` exits 0"
      - "`ng build` exits 0"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 4 — Remove console.log from server.ts
  # ─────────────────────────────────────────────────────────────────────────────
  - id: remove-console-log-server-ts
    priority: P1
    title: "Remove console.log statements from web/src/server.ts"
    file: web/src/server.ts
    description: >
      Spec rule §5: "No console.log in committed code."
      Lines 42, 47, 50, 70 in server.ts contain console.log.
      Replace with structured logging or remove entirely (SSR request logs are noise in prod).
    changes:
      - description: "Delete lines with console.log or replace with no-op"
        lines: [42, 47, 50, 70]
    acceptance_criteria:
      - "`grep -rn 'console.log' web/src --include='*.ts'` returns 0 results"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 5 — Fix localStorage usage in ui.service.ts
  # ─────────────────────────────────────────────────────────────────────────────
  - id: fix-localstorage-ui-service
    priority: P1
    title: "Replace localStorage in ui.service.ts — CI rule violation"
    file: web/src/app/core/state/ui.service.ts
    description: >
      Spec rule §5: "No tokens or org_id in localStorage."
      The sidebar collapsed state is not sensitive, but the rule is absolute and CI enforced.
      Use an in-memory Signal with optional persistence via a non-blocked mechanism,
      or simply drop persistence (sidebar state is non-critical).
    implementation: |
      // Remove localStorage.getItem / localStorage.setItem
      // Keep sidebarCollapsed as a plain writable Signal initialized to false
      // Optionally: inject DOCUMENT token and use document.cookie for non-sensitive UI prefs
      //   (cookies are the only allowed browser storage per spec)
    acceptance_criteria:
      - "`grep -rn 'localStorage\\|sessionStorage' web/src --include='*.ts'` returns 0 results"
      - "Sidebar collapsed/expanded behavior still works after page reload (may just default to expanded — acceptable)"

name: fix-p1-backend-spec-gaps
description: >
  P1 backend gaps: cache TTL mismatch, missing WebAuthn endpoints, server-side
  idempotency key enforcement, SSE CORS hardening, and the missing k6 load test.

jobs:

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 1 — Fix policy cache TTL: 30s → 60s
  # ─────────────────────────────────────────────────────────────────────────────
  - id: fix-policy-cache-ttl
    priority: P1
    title: "Fix policy service cache TTL to 60s (spec §9 SLO)"
    file: services/policy/pkg/service/service.go
    description: >
      Spec project.md §9 and ai-spec/be_open_guard/10-phase3-policy-engine.md specify
      that the SDK local cache TTL is 60 seconds and that fail-closed behaviour triggers
      after TTL expiry. The server-side Redis cache TTL sets the effective invalidation
      window. 30s causes the SDK to fail-closed 30s earlier than designed.
    changes:
      - find: 'cacheTTL      = 30 * time.Second // stale-while-revalidate window'
        replace: 'cacheTTL      = 60 * time.Second // stale-while-revalidate window (spec §9: SDK TTL = 60s)'
    acceptance_criteria:
      - "Policy service Redis TTL is 60s — verify via `TTL policy:eval:*` in Redis after an evaluation"
      - "`go test ./services/policy/...` passes"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 2 — Implement server-side Idempotency-Key middleware
  # ─────────────────────────────────────────────────────────────────────────────
  - id: implement-idempotency-middleware
    priority: P1
    title: "Add Idempotency-Key middleware to mutating endpoints (Phase 7 §14)"
    description: >
      Spec phase7 §14 requires idempotency on mutating API endpoints.
      The FE `ApiService.post()` already sends `Idempotency-Key` headers,
      but no server-side middleware validates or deduplicates them.
    new_file: shared/middleware/idempotency.go
    implementation: |
      package middleware

      import (
          "crypto/sha256"
          "encoding/hex"
          "encoding/json"
          "net/http"
          "time"

          "github.com/redis/go-redis/v9"
      )

      const idempotencyTTL = 24 * time.Hour

      // IdempotencyMiddleware deduplicates POST/PUT/PATCH requests that carry
      // an `Idempotency-Key` header. Cached responses are stored in Redis for 24h.
      // On replay: returns the cached status code + body without re-executing the handler.
      func IdempotencyMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
          return func(next http.Handler) http.Handler {
              return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                  if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
                      next.ServeHTTP(w, r)
                      return
                  }

                  key := r.Header.Get("Idempotency-Key")
                  if key == "" {
                      next.ServeHTTP(w, r)
                      return
                  }

                  // Namespace by org_id to prevent cross-tenant replay
                  orgID := GetOrgID(r.Context())
                  cacheKey := "idem:" + orgID + ":" + hashKey(key)

                  ctx := r.Context()
                  if cached, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil {
                      var entry idempotencyEntry
                      if json.Unmarshal(cached, &entry) == nil {
                          w.Header().Set("Content-Type", "application/json")
                          w.Header().Set("X-Idempotency-Replayed", "true")
                          w.WriteHeader(entry.StatusCode)
                          w.Write(entry.Body)
                          return
                      }
                  }

                  // Capture response
                  rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
                  next.ServeHTTP(rec, r)

                  // Cache only 2xx responses
                  if rec.statusCode >= 200 && rec.statusCode < 300 {
                      entry := idempotencyEntry{StatusCode: rec.statusCode, Body: rec.body}
                      if b, err := json.Marshal(entry); err == nil {
                          rdb.Set(ctx, cacheKey, b, idempotencyTTL)
                      }
                  }
              })
          }
      }

      type idempotencyEntry struct {
          StatusCode int    `json:"status_code"`
          Body       []byte `json:"body"`
      }

      func hashKey(k string) string {
          h := sha256.Sum256([]byte(k))
          return hex.EncodeToString(h[:])
      }

      // responseRecorder captures status + body for caching.
      type responseRecorder struct {
          http.ResponseWriter
          statusCode int
          body       []byte
      }

      func (r *responseRecorder) WriteHeader(code int) { r.statusCode = code; r.ResponseWriter.WriteHeader(code) }
      func (r *responseRecorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return r.ResponseWriter.Write(b) }

    apply_to_routers:
      - services/iam/pkg/router/router.go: "Wrap POST /auth/register, POST /oauth/token, POST /scim/v2/Users"
      - services/policy/pkg/router/router.go: "Wrap POST /v1/policies, PUT /v1/policies/:id"
      - services/connector-registry/pkg/router/router.go: "Wrap POST /v1/connectors"

    acceptance_criteria:
      - "Second POST with same Idempotency-Key returns HTTP 200 with `X-Idempotency-Replayed: true` header"
      - "Cached response is org-scoped (different org, same key = not replayed)"
      - "Unit test in `shared/middleware/idempotency_test.go`"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 3 — Harden SSE CORS: remove wildcard fallback
  # ─────────────────────────────────────────────────────────────────────────────
  - id: harden-sse-cors
    priority: P1
    title: "Require ALLOWED_ORIGIN env var for SSE — no wildcard fallback on authenticated streams"
    file: services/audit/pkg/handlers/sse.go
    description: >
      Line 37: `if allowedOrigin == "" { allowedOrigin = "*" }` allows unauthenticated cross-origin
      reads of authenticated audit SSE streams if the env var is not set.
    changes:
      - find: |
          allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
          	if allowedOrigin == "" {
          		allowedOrigin = "*"
          	}
          	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
        replace: |
          allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
          	if allowedOrigin == "" {
          		// Authenticated SSE stream must not be served cross-origin without an explicit allow-list.
          		// Fail fast in production; allow localhost for development.
          		if os.Getenv("APP_ENV") == "production" {
          			http.Error(w, "ALLOWED_ORIGIN must be set in production", http.StatusInternalServerError)
          			return
          		}
          		allowedOrigin = "http://localhost:4200"
          	}
          	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)

    acceptance_criteria:
      - "With `APP_ENV=production` and no `ALLOWED_ORIGIN`, SSE endpoint returns 500 with descriptive error"
      - "With `ALLOWED_ORIGIN=https://app.myco.io`, SSE responds with that origin header"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 4 — Add missing kafka-throughput.js k6 test
  # ─────────────────────────────────────────────────────────────────────────────
  - id: add-kafka-throughput-k6
    priority: P1
    title: "Add tests/load/kafka-throughput.js (Phase 8 acceptance criteria)"
    file: tests/load/kafka-throughput.js
    description: >
      Spec §16.1 defines a `kafka-throughput.js` k6 test using xk6-kafka extension.
      This is the only missing k6 script; Phase 8 acceptance criteria cannot pass without it.
    implementation: |
      /**
       * kafka-throughput.js
       * Phase 8 acceptance criteria: 50,000 events/s to audit.trail
       * Consumer lag must stay < 10,000 during burst.
       *
       * Requires: k6 built with xk6-kafka (https://github.com/grafana/xk6-kafka)
       *   k6 build --with github.com/grafana/xk6-kafka
       */
      import { check, sleep } from 'k6';
      import { writer, createTopic, CODEC_SNAPPY } from 'k6/x/kafka';
      import { Counter, Gauge } from 'k6/metrics';

      const errorCount = new Counter('kafka_errors');

      export const options = {
        scenarios: {
          burst: {
            executor: 'constant-arrival-rate',
            rate: 50000,
            timeUnit: '1s',
            duration: '2m',
            preAllocatedVUs: 500,
            maxVUs: 1000,
          },
        },
        thresholds: {
          kafka_errors: ['count<100'],  // < 0.2% error rate at 50k/s
        },
      };

      const kafkaWriter = writer({
        brokers: (__ENV.KAFKA_BROKERS || 'localhost:9092').split(','),
        topic: 'audit.trail',
        compression: CODEC_SNAPPY,
      });

      export default function () {
        const orgId = `org-${Math.floor(Math.random() * 100)}`;
        const messages = Array.from({ length: 10 }, (_, i) => ({
          key: `${orgId}-${__VU}-${__ITER}-${i}`,
          value: JSON.stringify({
            event_id: `${__VU}-${__ITER}-${i}`,
            org_id: orgId,
            source: 'k6-load-test',
            action: 'resource.read',
            actor: `user-${__VU}`,
            target: `document-${i}`,
            ts: Date.now(),
          }),
        }));

        const err = kafkaWriter.produce({ messages });
        if (err) {
          errorCount.add(1);
        }
      }

      export function teardown() {
        kafkaWriter.close();
      }

    makefile_update:
      file: Makefile
      description: "Add kafka-throughput to load-test target"
      find: "@k6 run tests/load/policy-eval.js --env BASE_URL=http://localhost:8083"
      replace: |
        @k6 run tests/load/policy-eval.js --env BASE_URL=http://localhost:8083
        	@k6 run tests/load/kafka-throughput.js --env KAFKA_BROKERS=localhost:9092

    acceptance_criteria:
      - "File `tests/load/kafka-throughput.js` exists and passes k6 parse check"
      - "Kafka consumer lag stays below 10,000 during test run (verify via Prometheus/Grafana)"
      - "`make load-test` runs all 6 scripts"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 5 — Implement WebAuthn endpoints in IAM service
  # ─────────────────────────────────────────────────────────────────────────────
  - id: implement-webauthn-endpoints
    priority: P1
    title: "Add WebAuthn (FIDO2) registration and authentication endpoints to IAM"
    description: >
      Spec §10.3 and migration `005_create_mfa_configs.up.sql` include a `webauthn_credentials`
      table. The IAM handler has no WebAuthn routes. Use github.com/go-webauthn/webauthn.
    new_routes:
      - "POST /auth/webauthn/register/begin   → BeginRegistration → returns CredentialCreation JSON"
      - "POST /auth/webauthn/register/finish  → FinishRegistration → store credential in DB"
      - "POST /auth/webauthn/login/begin      → BeginDiscoverableLogin → returns CredentialAssertion JSON"
      - "POST /auth/webauthn/login/finish     → ValidateDiscoverableLogin → issue JWT on success"

    implementation_notes: |
      1. Add `github.com/go-webauthn/webauthn` to services/iam/go.mod
      2. Store pending challenge in Redis (TTL 5 minutes): key = "webauthn:challenge:{user_id}"
      3. On FinishRegistration: INSERT into webauthn_credentials (credential_id TEXT, public_key BYTEA,
         sign_count INT, transports TEXT[], created_at TIMESTAMPTZ)
      4. On successful login: same JWT issuance flow as TOTP verify — set jti, create session, return access + refresh tokens
      5. Add webauthn config to shared/secrets (rpid, rp_origin) — do NOT hardcode

    migration_needed:
      - description: "Ensure 005_create_mfa_configs.up.sql creates webauthn_credentials table"
        check: "Already exists in migration file — verify schema matches go-webauthn library expectations"

    acceptance_criteria:
      - "POST /auth/webauthn/register/begin returns valid `PublicKeyCredentialCreationOptions` JSON"
      - "Full registration → login flow works in integration test using go-webauthn/webauthn test authenticator"
      - "WebAuthn credential stored in `webauthn_credentials` table with correct org_id (RLS scoped)"

name: fix-p2-gaps-and-test-coverage
description: >
  P2 gaps: audit HMAC multi-key, ClickHouse integration, frontend test scaffolding,
  RedactableComponent, DLP async mode, committed binary removal, and observability.

jobs:

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 1 — Upgrade audit HMAC to multi-key keyring
  # ─────────────────────────────────────────────────────────────────────────────
  - id: audit-hmac-multi-key
    priority: P2
    title: "Upgrade audit hash-chain HMAC to multi-key keyring (shared/crypto pattern)"
    file: services/audit/pkg/consumer/consumer.go
    description: >
      Currently the audit consumer reads `AUDIT_SECRET_KEY` as a single env var.
      Spec §11 mandates multi-key keyring (matching JWT and AES keyrings) for key rotation.
      Keys should be loaded via `shared/secrets` provider, not raw env vars.
    changes:
      - description: "Replace os.Getenv('AUDIT_SECRET_KEY') with keyring loaded from secrets provider"
        new_structure: |
          type HMACKey struct {
              Kid    string `json:"kid"`
              Secret string `json:"secret"`
              Status string `json:"status"` // "active" | "verify_only"
          }
          // Load via secrets.GetProvider().GetSecret("AUDIT_HMAC_KEYS") → []HMACKey
          // Sign with first active key; prefix hash with "kid:<hash>" for rotation traceability

    acceptance_criteria:
      - "AUDIT_HMAC_KEYS secret contains JSON array of HMACKey"
      - "Key rotation: add a new active key → old key becomes verify_only → existing hashes still verify"
      - "`go test ./services/audit/...` passes"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 2 — Add ClickHouse to docker-compose and implement compliance writer
  # ─────────────────────────────────────────────────────────────────────────────
  - id: clickhouse-integration
    priority: P2
    title: "Add ClickHouse to docker-compose and implement Phase 6 compliance writer"
    description: >
      Phase 6 requires ClickHouse for compliance analytics (spec §13).
      ClickHouse is missing from docker-compose. The `clickhouse_writer.go` is a stub.
    changes:
      - file: infra/docker/docker-compose.yml
        description: "Add ClickHouse service"
        snippet: |
          clickhouse:
            image: clickhouse/clickhouse-server:24.3
            ports:
              - "8123:8123"
              - "9000:9000"
            volumes:
              - clickhouse_data:/var/lib/clickhouse
              - ./clickhouse-init:/docker-entrypoint-initdb.d
            environment:
              CLICKHOUSE_DB: openguard
              CLICKHOUSE_USER: default
              CLICKHOUSE_PASSWORD: ""
            healthcheck:
              test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]
              interval: 10s
              timeout: 5s
              retries: 5

      - file: infra/docker/clickhouse-init/01_schema.sql
        description: "Create events table per spec §13"
        snippet: |
          CREATE TABLE IF NOT EXISTS openguard.events (
              org_id          UUID,
              event_id        UUID,
              source          String,
              action          LowCardinality(String),
              actor           String,
              target          String,
              effect          LowCardinality(String),
              policy_ids      Array(UUID),
              ts              DateTime64(3, 'UTC'),
              date            Date MATERIALIZED toDate(ts)
          ) ENGINE = ReplacingMergeTree(ts)
          PARTITION BY toYYYYMM(date)
          ORDER BY (org_id, date, event_id)
          TTL date + INTERVAL 90 DAY;

      - file: services/compliance/pkg/consumer/clickhouse_writer.go
        description: "Implement real ClickHouse batch writer using clickhouse-go driver"
        implementation: |
          // 1. Add github.com/ClickHouse/clickhouse-go/v2 to go.mod
          // 2. Connect via DSN from CLICKHOUSE_DSN env var
          // 3. Consume from audit.trail Kafka topic
          // 4. Batch insert 1000 events per flush or every 5 seconds
          // 5. Use INSERT INTO openguard.events with all fields from EventEnvelope
          // 6. Add SELECT ... FINAL to all compliance query helpers

    acceptance_criteria:
      - "`docker compose up clickhouse` starts and healthcheck passes"
      - "Compliance service starts and connects to ClickHouse without error"
      - "10 audit events ingested → appear in `SELECT count() FROM openguard.events`"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 3 — Frontend test scaffolding (minimum viable coverage)
  # ─────────────────────────────────────────────────────────────────────────────
  - id: frontend-test-scaffolding
    priority: P2
    title: "Add Jasmine/Karma unit tests for critical FE components"
    description: >
      Currently 0 `.spec.ts` files exist in `web/src`. Spec §13 requires 80%+ coverage
      for services and components. This job creates the scaffolding for critical paths.
    files_to_create:
      - path: web/src/app/core/services/auth.service.spec.ts
        test_cases:
          - "login() with valid credentials sets currentUser signal and navigates to '/'"
          - "login() with mfa_required does NOT navigate (stays on login for MFA step)"
          - "logout() calls /auth/logout and clears currentUser signal"
          - "isAuthenticated computed returns false when user is null"
          - "currentOrgId computed returns user.org_id when set"

      - path: web/src/app/core/services/api.service.spec.ts
        test_cases:
          - "get() appends path to apiUrl and returns typed Observable"
          - "post() with idempotencyKey adds Idempotency-Key header"

      - path: web/src/app/core/guards/auth.guard.spec.ts
        test_cases:
          - "Authenticated user: canActivate returns true"
          - "Unauthenticated user: canActivate redirects to /login and returns false"

      - path: web/src/app/core/guards/org.guard.spec.ts
        test_cases:
          - "User with orgId: canActivate returns true"
          - "User without orgId: canActivate redirects to /login and returns false"

      - path: web/src/app/policies/policies.spec.ts
        test_cases:
          - "Component loads and displays policy list from PolicyService"
          - "Create policy form is invalid without name or action"
          - "Delete policy shows ConfirmDialog before calling service"

      - path: web/src/app/core/interceptors/error.interceptor.spec.ts
        test_cases:
          - "401 response triggers token refresh attempt"
          - "401 on refresh request triggers logout + redirect to /login"

    playwright_e2e:
      path: web/e2e/
      tests:
        - name: auth-flow.spec.ts
          steps:
            - "Navigate to /login"
            - "Submit valid credentials"
            - "Assert redirect to /"
            - "Assert sidebar visible"
            - "Click logout"
            - "Assert redirect to /login"

        - name: policy-crud.spec.ts
          steps:
            - "Login as admin"
            - "Navigate to /policies"
            - "Create a new policy"
            - "Assert policy appears in list"
            - "Delete policy → confirm dialog → assert removed"

    acceptance_criteria:
      - "At least 6 `.spec.ts` files exist covering auth service, guards, and policies component"
      - "`npm test -- --no-watch --code-coverage` exits 0"
      - "Service coverage >= 70%"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 4 — Implement RedactableComponent
  # ─────────────────────────────────────────────────────────────────────────────
  - id: implement-redactable-component
    priority: P2
    title: "Implement RedactableComponent for PII display (spec rule §5)"
    description: >
      Spec rule §5: "No sensitive data (email, ip_address, token_prefix) outside RedactableComponent."
      No such component exists. Must be created and applied in users list, audit logs, and threats.
    file: web/src/app/core/components/redactable.ts
    implementation: |
      import { Component, input, signal, computed } from '@angular/core';
      import { CommonModule } from '@angular/common';

      @Component({
        selector: 'app-redactable',
        standalone: true,
        imports: [CommonModule],
        template: `
          <span class="inline-flex items-center gap-1 font-mono text-sm">
            @if (revealed()) {
              <span class="text-gray-800">{{ value() }}</span>
            } @else {
              <span class="text-gray-400 tracking-widest">{{ masked() }}</span>
            }
            <button (click)="toggle()"
                    class="text-xs text-blue-500 hover:underline focus:outline-none"
                    [attr.aria-label]="revealed() ? 'Hide' : 'Reveal'">
              {{ revealed() ? 'Hide' : 'Show' }}
            </button>
          </span>
        `
      })
      export class RedactableComponent {
        value = input.required<string>();
        type = input<'email' | 'ip' | 'token' | 'generic'>('generic');

        revealed = signal(false);

        masked = computed(() => {
          const v = this.value();
          switch (this.type()) {
            case 'email': {
              const [local, domain] = v.split('@');
              return `${local.slice(0, 2)}***@${domain ?? '***'}`;
            }
            case 'ip': return '*.*.*.***';
            case 'token': return `${v.slice(0, 8)}...`;
            default: return '***';
          }
        });

        toggle() { this.revealed.update(v => !v); }
      }

    apply_to:
      - "web/src/app/users/users.html — wrap email display in <app-redactable type='email'>"
      - "web/src/app/audit-logs/audit-logs.html — wrap actor/ip fields"
      - "web/src/app/threats/threats.html — wrap user_id/ip in threat metadata"

    acceptance_criteria:
      - "`RedactableComponent` exists and masks values by default"
      - "Click 'Show' reveals full value; click 'Hide' masks again"
      - "Used in users, audit-logs, threats templates"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 5 — Remove committed binary from examples/
  # ─────────────────────────────────────────────────────────────────────────────
  - id: remove-committed-binary
    priority: P2
    title: "Remove committed binary examples/task-management-app/backend/backend"
    description: >
      A compiled Go binary `examples/task-management-app/backend/backend` is committed to git.
      Committed binaries are a security risk (supply chain) and inflate repo size.
    changes:
      - "Run: git rm examples/task-management-app/backend/backend"
      - "Add `examples/task-management-app/backend/backend` to .gitignore"
      - "Add build instruction to examples/task-management-app/README.md: `cd backend && go build -o backend .`"

    acceptance_criteria:
      - "`git ls-files examples/task-management-app/backend/backend` returns empty"
      - "`.gitignore` contains the binary path"

  # ─────────────────────────────────────────────────────────────────────────────
  # JOB 6 — DLP async scan mode
  # ─────────────────────────────────────────────────────────────────────────────
  - id: dlp-async-scan
    priority: P2
    title: "Implement DLP async scan mode (spec §17: 500ms SLO)"
    description: >
      Spec §17 defines an async scan flow: POST /v1/dlp/scan returns a job ID immediately,
      scan runs in background, result polled via GET /v1/dlp/scan/{job_id}.
      Currently only synchronous scan exists. Large payloads block the HTTP thread.
    changes:
      - file: services/dlp/pkg/handlers/handler.go
        description: "Add async scan endpoint"
        new_endpoints:
          - "POST /v1/dlp/scan/async → enqueue scan job → return { job_id: uuid, status: 'pending' }"
          - "GET  /v1/dlp/scan/{job_id} → return job status + findings when complete"

      - file: services/dlp/pkg/consumer/consumer.go
        description: "Add worker goroutine that processes scan jobs from a Redis list or Kafka topic"
        implementation_notes: |
          1. On async scan request: store { content, org_id, status: "pending" } in Redis with 5min TTL
          2. Push job_id to Redis list `dlp:scan:queue`
          3. Worker goroutine: BLPOP from queue, run ScanRegex + ScanEntropy, update job status to "complete"
          4. GET /v1/dlp/scan/{job_id}: return current job state from Redis

    acceptance_criteria:
      - "POST /v1/dlp/scan/async returns within 10ms with a job_id"
      - "GET /v1/dlp/scan/{job_id} returns findings within 500ms of submission (SLO)"
      - "Findings in async response are masked (no raw PII) — same as sync endpoint"
