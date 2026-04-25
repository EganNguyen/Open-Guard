# OpenGuard — Pre-Production Business Correctness Audit

**Auditor:** Senior Product Architect / Domain Expert  
**Scope:** Full codebase review (backend services, shared libraries, threat detectors, SCIM, saga orchestration, policy engine, webhook delivery)  
**Date:** 2026-04-25  
**Severity scale:** 🔴 Critical · 🟠 High · 🟡 Medium · 🔵 Low

---

## Executive Summary

OpenGuard is architecturally well-considered — transactional outbox, RLS, circuit breakers, and choreography sagas are all present. However, the audit reveals **4 Critical** and **7 High** severity issues that would cause security breaches, data loss, or operational failures in production. The most dangerous are a complete bypass of SCIM authentication (any caller can impersonate any tenant), suspended/deprovisioned users being allowed to log in, and a saga consumer that silently drops failures.

---

## Issue 1 — SCIM Endpoints Have No Authentication

- **Issue:** All five SCIM endpoints (`/scim/v2/Users`, `GET`, `POST`, `PATCH`, `DELETE`) are mounted without any authentication middleware. The spec (§2.8) mandates a per-org SCIM bearer token enforced by `SCIMAuthMiddleware`, and the `org_id` must be derived from the token — not from any client-supplied header.
- **Location:** `services/iam/pkg/router/router.go` lines 101–112; `services/iam/pkg/handlers/scim.go` lines 45, 69–74, 118, 131, 142
- **Why it is incorrect (business perspective):** The router only attaches `idemMiddleware` to the SCIM route group. There is no `SCIMAuthMiddleware`. The handlers then read `org_id` from the raw `X-Org-ID` HTTP header supplied by the caller. Any unauthenticated HTTP request can supply an arbitrary `org_id`, enumerate all users of any tenant, create users in any org, or delete them. The `SCIMToken` struct and `SCIMAuthMiddleware` function exist in `shared/middleware/scim.go` but are never wired into the router.
- **Real-world impact:** Complete tenant isolation bypass. An attacker with network access to the IAM service (e.g., a compromised connected app) can read, create, suspend, or deprovision users in any organisation with no credentials. This is a GDPR breach (unauthorized access to personal data) and a full account takeover vector. In a multi-tenant SaaS deployment this is catastrophic.
- **Severity:** 🔴 Critical
- **Recommended fix:**
  ```go
  // In router.go, load SCIM tokens from config/env:
  scimTokens := config.SCIMTokens // []middleware.SCIMToken loaded at startup
  r.Route("/scim/v2", func(r chi.Router) {
      r.Use(shared_middleware.SCIMAuthMiddleware(scimTokens))
      r.Use(idemMiddleware)
      r.Get("/Users", h.ListScimUsers)
      // ... remaining routes
  })
  ```
  Additionally, remove all `r.Header.Get("X-Org-ID")` calls from every SCIM handler — org_id must be read exclusively from `rls.OrgID(r.Context())` which is set by the middleware from the token map.

---

## Issue 2 — Suspended and Deprovisioned Users Can Log In

- **Issue:** The `Login` service method only rejects users whose status is `"initializing"`. It does not reject `"suspended"`, `"deprovisioned"`, or `"provisioning_failed"` users.
- **Location:** `services/iam/pkg/service/service.go` lines 514–517 (`Login` function)
- **Why it is incorrect (business perspective):** Suspension is an administrative action taken to deny access — e.g., when an employee is placed on leave, under investigation, or an account is detected as compromised. A deprovisioned user (SCIM `DELETE`) should have zero access. The current code allows both to authenticate, receive valid JWTs, and access connected applications.
- **Real-world impact:** A suspended employee can continue accessing all connected apps indefinitely. A deleted/deprovisioned user (e.g., one who has left the company) retains login capability until their token expires. This violates HRMS integration contracts, SOC 2 access controls, and creates direct legal liability.
- **Severity:** 🔴 Critical
- **Recommended fix:**
  ```go
  // In Login(), after getting the user, add:
  switch user["status"].(string) {
  case "suspended":
      return nil, "", fmt.Errorf("ACCOUNT_SUSPENDED")
  case "deprovisioned":
      return nil, "", fmt.Errorf("ACCOUNT_DEPROVISIONED")
  case "provisioning_failed":
      return nil, "", fmt.Errorf("ACCOUNT_PROVISIONING_FAILED")
  case "initializing":
      return nil, "", fmt.Errorf("USER_PROVISIONING_IN_PROGRESS")
  }
  ```
  The same check must be applied in `FinishWebAuthnLogin` and `VerifyMFAAndLogin`, which also bypass it today.

---

## Issue 3 — Saga Consumer Uses Auto-Commit (`ReadMessage`), Silently Drops Failures

- **Issue:** The IAM saga consumer (`services/iam/pkg/saga/consumer.go`) uses `reader.ReadMessage(ctx)` instead of `reader.FetchMessage(ctx)` + `reader.CommitMessages(ctx, m)`. `ReadMessage` auto-commits the offset before the message has been processed.
- **Location:** `services/iam/pkg/saga/consumer.go` line 41
- **Why it is incorrect (business perspective):** The spec (§2.2) explicitly states: "An offset is committed only after the downstream write has been confirmed." With auto-commit, if `UpdateUserStatus` fails (e.g., DB is temporarily unavailable, network error), the offset is already committed. The compensation event — which sets a failed user to `provisioning_failed` — is permanently lost. The user remains stuck in `initializing` indefinitely and can never log in.
- **Real-world impact:** Every transient DB error during saga compensation leaves a user in a zombie `initializing` state. Admins have no visibility; the saga timeout watcher will re-fire if the deadline is still in Redis, but if the process restarted after auto-commit, the deadline is consumed. Users cannot be logged in, cannot be reprovisioned, and cannot be deleted cleanly. This is an operational correctness violation that compounds under load.
- **Severity:** 🔴 Critical
- **Recommended fix:**
  ```go
  // Replace ReadMessage with FetchMessage + CommitMessages:
  m, err := c.reader.FetchMessage(ctx)
  // ... process event ...
  if processErr == nil {
      if err := c.reader.CommitMessages(ctx, m); err != nil {
          c.logger.Error("failed to commit offset", "error", err)
      }
  }
  // On processErr: do NOT commit — message will be redelivered
  ```

---

## Issue 4 — `DeleteUser` DB State and Redis Blocklist Are Not Atomic

- **Issue:** In `DeleteUser`, the Redis pipeline to blocklist active JTIs is executed **outside** the PostgreSQL transaction. The Postgres TX is opened, but the Redis ops happen before any DB writes, and failures in either can leave the system in a split state.
- **Location:** `services/iam/pkg/service/service.go` lines 400–453 (`DeleteUser`)
- **Why it is incorrect (business perspective):** The session revocation sequence is: (1) fetch JTIs from DB, (2) pipeline Redis blocklist, (3) revoke sessions in DB, (4) set user to deprovisioned, (5) write outbox event, (6) commit TX. If Redis fails at step 2, the function returns an error — but the user has not been deprovisioned in DB. This is recoverable. However, if Redis succeeds but the DB TX fails to commit at step 6, the user's tokens are blocklisted in Redis (sessions will be denied), but the user remains `active` in the DB and the `user.deleted` event is never published — downstream services (Policy, Threat, Alerting) never clean up their state, creating a permanently inconsistent ghost record.
- **Real-world impact:** Partial deprovisioning: from Redis's perspective the user is gone (tokens revoked), but from the DB and all downstream services the user is still active. Policy assignments remain. Baseline threat profiles remain. The audit trail has no `user.deleted` event, which is a compliance gap.
- **Severity:** 🔴 Critical
- **Recommended fix:** Separate "best-effort immediate revocation" from the transactional deprovisioning. Commit the DB transaction first (status=deprovisioned + outbox event), then blocklist Redis as a best-effort post-commit step. A background job can re-blocklist any active JTIs for deprovisioned users on startup to handle crashes between commit and Redis write. Alternatively, publish a `user.deprovisioned` Kafka event and have a dedicated revocation consumer handle Redis — keeping Redis state eventual but never ahead of the source of truth.

---

## Issue 5 — `ReprovisionUser` Does Not Guard the Current State Transition

- **Issue:** `ReprovisionUser` does not check whether the user is currently in `provisioning_failed` before allowing the transition. It will accept any user in any status.
- **Location:** `services/iam/pkg/service/service.go` lines 364–396
- **Why it is incorrect (business perspective):** The spec state machine (§2.5) specifies: `provisioning_failed → initializing` via `POST /users/:id/reprovision`. No other transition is permitted. As implemented, an admin can reprovision an `active` user, resetting them to `initializing` and blocking their logins while the saga re-runs unnecessarily. They can also reprovision a `deprovisioned` user, re-triggering provisioning for a deleted account.
- **Real-world impact:** Active users can be accidentally locked out. Deprovisioned accounts can be accidentally reactivated by running reprovision — a security violation. The state machine invariant is broken.
- **Severity:** 🟠 High
- **Recommended fix:**
  ```go
  func (s *Service) ReprovisionUser(ctx context.Context, orgID, userID string) error {
      user, err := s.repo.GetUserByID(ctx, userID)
      if err != nil { return err }
      if user["status"].(string) != "provisioning_failed" {
          return fmt.Errorf("INVALID_TRANSITION: can only reprovision users in provisioning_failed state, current: %s", user["status"])
      }
      // ... rest of logic
  }
  ```

---

## Issue 6 — Policy Cache Key Is Non-Deterministic (Map Serialization)

- **Issue:** `cacheKey()` in the policy service builds a `map[string]interface{}` and serializes it with `json.Marshal`. Go's `map` type has **non-deterministic iteration order**, meaning the same logical request can produce different JSON — and therefore different SHA-256 hashes — across goroutines or restarts.
- **Location:** `services/policy/pkg/service/service.go` lines 94–107
- **Why it is incorrect (business perspective):** Two identical policy evaluation requests — same org, subject, action, resource, and user groups — will frequently miss the Redis cache because they hash to different keys. The cache effectively becomes useless for a significant fraction of requests. This directly contradicts the core SLO: "POST /v1/policy/evaluate (Redis cached): p99 5ms."
- **Real-world impact:** At 10,000 req/s, cache miss rates approaching 50% would cause the DB circuit breaker to open under load, triggering fail-closed denials. Connected apps would see spurious authorization failures at scale — an operational and compliance disaster.
- **Severity:** 🟠 High
- **Recommended fix:** Build the cache key from sorted, concatenated fields, not a map:
  ```go
  func cacheKey(req EvaluateRequest) string {
      groups := make([]string, len(req.UserGroups))
      copy(groups, req.UserGroups)
      sort.Strings(groups)
      raw := req.OrgID + ":" + req.SubjectID + ":" + req.Action + ":" + req.Resource + ":" + strings.Join(groups, ",")
      sum := sha256.Sum256([]byte(raw))
      return fmt.Sprintf("%s%s:%x", cachePrefix, req.OrgID, sum)
  }
  ```

---

## Issue 7 — Webhook Delivery Treats 4xx Client Errors as Permanent Failures Without DLQ Routing

- **Issue:** The webhook deliverer returns an error for both 4xx and 5xx responses. The consumer retries on any error up to `WEBHOOK_MAX_ATTEMPTS`, then routes to `webhook.dlq`. For 4xx (e.g., 400 Bad Request, 404 Not Found, 410 Gone), retrying is pointless — the remote endpoint has permanently rejected the payload.
- **Location:** `services/webhook-delivery/pkg/deliverer/deliverer.go` lines 68–74
- **Why it is incorrect (business perspective):** Retrying 4xx responses exhausts all delivery attempts on a definitively undeliverable webhook, burning quota against other deliveries and filling the DLQ with events that will never succeed. A 410 Gone from the target URL means the endpoint has been removed — every retry is wasted work.
- **Real-world impact:** Connected apps that change webhook endpoints will see all their in-flight events exhaustively retried (burning potentially hours of queue time), and legitimate 5xx retryable failures get throttled by a saturated queue. SLA for webhook delivery degrades for all tenants.
- **Severity:** 🟠 High
- **Recommended fix:** Treat 4xx as a permanent failure — mark the delivery as `permanently_failed` immediately and do not retry. Only retry on 5xx, network errors, and timeouts. Optionally surface a `webhook.endpoint.rejected` alert to the tenant admin.

---

## Issue 8 — SCIM Handler `ListScimUsers` Accepts Unauthenticated Requests

- **Issue:** `ListScimUsers` does not check for a missing `X-Org-ID` header, unlike `PostScimUser` which at least checks for empty. It proceeds with an empty `orgID`, passing it directly to `ListUsers`. With an empty org_id context, the RLS session variable `app.org_id` is set to an empty string, which may bypass or break the RLS policy (spec §2.3 uses `NULLIF(current_setting('app.org_id', true), '')::UUID` — a NULL cast would match no rows for a strict RLS policy, but could return all rows if the policy is misconfigured).
- **Location:** `services/iam/pkg/handlers/scim.go` lines 44–52
- **Why it is incorrect (business perspective):** Even if RLS holds, a GET with no org_id should return 401, not an empty list or a DB error. The endpoint is effectively unauthenticated for read operations.
- **Severity:** 🟠 High (compounded by Issue 1 — if SCIM auth is added, this becomes a secondary validation gap only)
- **Recommended fix:** Add the same `X-Org-ID` empty-check as `PostScimUser`. Ultimately resolved when Issue 1 (SCIM authentication) is fixed, since org_id will come from the token and be guaranteed non-empty.

---

## Issue 9 — `GetUserByID` Does Not Return `version` Field, Causing Runtime Panic in `mapToScim`

- **Issue:** `GetUserByID` in the repository queries `SELECT org_id, email, display_name, role, status` — no `version` column. However, `mapToScim` calls `user["version"].(int)` which will panic with a nil interface assertion when `version` is absent from the map.
- **Location:** `services/iam/pkg/repository/repository.go` `GetUserByID` function; `services/iam/pkg/handlers/scim.go` `mapToScim` line `fmt.Sprintf("v%d", user["version"].(int))`
- **Why it is incorrect (business perspective):** SCIM `GET /Users/:id` and `POST /Users` (which calls `GetCurrentUser` → `GetUserByID`) will panic on every successful request, returning a 500 to the SCIM provisioner. This breaks every SCIM provisioning integration.
- **Real-world impact:** All SCIM provisioners (Okta, Azure AD, etc.) will fail to provision or retrieve users. Enterprise identity integrations are completely broken. This would be caught in the first integration test.
- **Severity:** 🟠 High
- **Recommended fix:** Add `version` to the `GetUserByID` SELECT and Scan. Use a safe type assertion: `v, _ := user["version"].(int)` before formatting.

---

## Issue 10 — Off-Hours Detector Uses UTC Only; No Per-Org Timezone

- **Issue:** The `OffHoursDetector` uses `time.Now().UTC().Hour()` to determine if access is "off hours." The off-hours window (`22–06 UTC`) is a global constant with no per-org timezone configuration.
- **Location:** `services/threat/pkg/detector/off_hours.go`
- **Why it is incorrect (business perspective):** A user in Tokyo accessing at 9 AM local time (00:00 UTC) would be flagged as an off-hours threat. Conversely, a US-based attacker accessing at 2 AM EST (07:00 UTC) would not be detected because UTC is within business hours. The detector produces both false positives (legitimate foreign users) and false negatives (attackers in certain time zones).
- **Real-world impact:** High volume of false-positive alerts for global organizations, eroding trust in the alerting system ("alert fatigue"). Security teams will disable or ignore alerts. Real threats in opposite time zones go undetected. This is a correctness failure of the core threat detection value proposition.
- **Severity:** 🟡 Medium
- **Recommended fix:** Store an `org_timezone` field on organizations. The threat event should carry the user's `org_id`; look up the org's configured timezone. If not set, fall back to per-org business hours configuration. At minimum, emit `org_timezone` in audit events so the detector can apply the correct offset.

---

## Issue 11 — Impossible Travel Detector Ignores VPN, CDN, and Private IPs

- **Issue:** The `ImpossibleTravelDetector` computes geo-distance from IP using MaxMind GeoLite2. There is no handling for: (a) private/RFC1918 IP addresses (which GeoLite2 cannot resolve and will return an error, currently silently ignored), (b) known VPN/CDN exit nodes that geo-locate to one country but represent users in another, and (c) IPv6 addresses in some edge cases.
- **Location:** `services/threat/pkg/detector/impossible_travel.go` — `processEvent` silently returns `nil` on geo-lookup errors
- **Why it is incorrect (business perspective):** Returning `nil` on a geo-lookup failure silently updates the last-known location to the current IP without actually computing distance. The next login from a legitimate location will then compute distance from a corrupt baseline, potentially triggering false alarms or masking real impossible travel. Additionally, corporate VPN exit nodes clustered in one city will produce a `last.IP == current.IP` match even for globally distributed users, completely suppressing detection for VPN users.
- **Real-world impact:** Attackers using corporate VPN or common cloud exit IPs (AWS us-east-1, GCP europe-west1) will never trigger impossible travel alerts. Private IP users (internal network logins) will corrupt the location baseline. Detection reliability is materially degraded.
- **Severity:** 🟡 Medium
- **Recommended fix:** On geo-lookup failure for private/unknown IPs, do NOT update the location baseline — skip location storage entirely. Maintain a separate configurable allowlist of known VPN/CDN CIDR ranges. Flag events from unknown IPs differently (e.g., `unknown_geo` alert, lower severity) rather than treating them as a valid location.

---

## Issue 12 — Account Takeover Detection: New Users Always Trigger Alert on First Login After Password Change

- **Issue:** The `AccountTakeoverDetector` flags a login as suspicious when `ato:pwchange:{userID}` exists AND the device fingerprint is not in the known devices set. For a newly registered user whose password was set during provisioning (which always sets `ato:pwchange`), **every first login from any device** will trigger a HIGH severity account takeover alert, because no known devices exist yet.
- **Location:** `services/threat/pkg/detector/account_takeover.go` — `processEvent`
- **Why it is incorrect (business perspective):** The business intent is to detect account takeover after a password reset by an attacker — not to alarm on the normal new-user login flow. The current logic cannot distinguish "new user logging in for the first time" from "attacker who changed password logging in from a new device."
- **Real-world impact:** Every new user provisioned via SCIM will generate a HIGH severity alert, flooding the security team's queue. For an org with 1,000 new hires per month, this is 1,000 false-positive critical alerts. Security team will tune out alerts, missing real incidents.
- **Severity:** 🟡 Medium
- **Recommended fix:** Introduce a distinction between "provisioning-time password set" and "user-initiated password change." Only set the `ato:pwchange` flag for user-initiated password resets. Alternatively, skip the ATO check when the user's device set is empty (first-ever login) — treat first login as device enrollment, not a threat signal.

---

## Issue 13 — Idempotency Key Namespace Allows Cross-Tenant Replay When org_id Is Empty

- **Issue:** The idempotency middleware namespaces cache keys by `orgID + ":" + hashKey(key)`. When `orgID` is empty (unauthenticated or pre-auth requests), the namespace collapses to `":" + hash`. Two different callers supplying the same `Idempotency-Key` header but from different (or no) orgs will share the same Redis cache entry.
- **Location:** `shared/middleware/idempotency.go` line — `cacheKey := "idem:" + orgID + ":" + hashKey(key)`
- **Why it is incorrect (business perspective):** An attacker can supply an `Idempotency-Key` that was recently used by another org on an unauthenticated route (e.g., `POST /token`), and receive the cached response from the other org's request — including tokens or auth codes.
- **Real-world impact:** Potential cross-tenant token/auth-code leakage on unauthenticated endpoints. Severity is bounded by the 24h TTL and the need to know a victim's recently-used idempotency key, but the attack surface is real.
- **Severity:** 🟡 Medium
- **Recommended fix:** For unauthenticated endpoints (where `orgID` is empty), include a stable per-endpoint prefix and the request IP or client identifier in the cache key to prevent cross-caller collisions. Or simply disable the idempotency middleware on unauthenticated routes and only apply it post-authentication.

---

## Issue 14 — `ReprovisionUser` Status Update Is Outside the Transaction

- **Issue:** In `ReprovisionUser`, `UpdateUserStatus(ctx, userID, "initializing")` is called before the transaction is committed, but it is called on the repository directly — not via the `tx` object. This is a separate DB operation outside the transaction boundary.
- **Location:** `services/iam/pkg/service/service.go` lines 377–383
- **Why it is incorrect (business perspective):** If the outbox event write succeeds but the TX rolls back for any reason, the status is already committed to `initializing` but no saga event is in the outbox. The user is stuck in `initializing` with no event to drive the saga forward, and the watcher's deadline has been cleared. The user cannot log in and will need manual DB intervention.
- **Real-world impact:** A DB write that partially fails during reprovision leaves the user permanently locked in `initializing`. This is unrecoverable without direct DB access.
- **Severity:** 🟠 High
- **Recommended fix:** The `UpdateUserStatus` call must be performed within the same transaction — pass the `tx` object and use it for the status update, or use a `WithTx` pattern on the repository. All mutations in a saga step must be atomic.

---

## Summary Table

| # | Issue | Location | Severity |
|---|-------|----------|----------|
| 1 | SCIM endpoints have no authentication; org_id is caller-supplied | `router.go`, `scim.go` | 🔴 Critical |
| 2 | Suspended/deprovisioned users can log in | `service.go:Login` | 🔴 Critical |
| 3 | Saga consumer uses auto-commit `ReadMessage` — silently drops failures | `saga/consumer.go` | 🔴 Critical |
| 4 | `DeleteUser` Redis blocklist and DB deprovisioning are not atomic | `service.go:DeleteUser` | 🔴 Critical |
| 5 | `ReprovisionUser` does not validate current state | `service.go:ReprovisionUser` | 🟠 High |
| 6 | Policy cache key is non-deterministic due to Go map iteration order | `policy/service.go:cacheKey` | 🟠 High |
| 7 | Webhook 4xx treated as retriable — exhausts attempts on permanent failures | `deliverer/deliverer.go` | 🟠 High |
| 8 | `ListScimUsers` accepts no auth and empty org_id | `scim.go:ListScimUsers` | 🟠 High |
| 9 | `GetUserByID` missing `version` field → runtime panic in `mapToScim` | `repository.go`, `scim.go` | 🟠 High |
| 10 | Off-hours detector uses UTC only; no per-org timezone | `detector/off_hours.go` | 🟡 Medium |
| 11 | Impossible travel silently corrupts baseline on private/unknown IPs | `detector/impossible_travel.go` | 🟡 Medium |
| 12 | ATO detector triggers on every new user's first login | `detector/account_takeover.go` | 🟡 Medium |
| 13 | Idempotency key namespace is empty for unauthenticated routes | `middleware/idempotency.go` | 🟡 Medium |
| 14 | `ReprovisionUser` status update is outside the transaction | `service.go:ReprovisionUser` | 🟠 High |

---

## Ambiguities for Product Owner Clarification

1. **SCIM deprovisioned idempotency:** The spec says `POST /Users` should return 200 if the external ID exists in any non-deprovisioned status, but return a CONFLICT error for deprovisioned. The current code returns a 409 for deprovisioned. Is the correct behavior to allow re-use of a deprovisioned external ID by creating a fresh user, or to force a reprovision workflow? This has GDPR implications (right to erasure vs. re-engagement).

2. **Connector status check in `APIKeyAuthComplex`:** The middleware verifies the PBKDF2 hash but does not check `connector.status == "active"`. The spec (§2.6 step 4) says "Check status == active." Is the status check supposed to happen in the middleware or in the downstream handler? Currently it is in neither.

3. **Backup code entropy:** Backup codes use `GenerateRandomString(8)` — the implementation of this function was not visible in the reviewed files. If it generates 8 ASCII characters from a limited alphabet (e.g., alphanumeric only), entropy may be insufficient. Industry standard is at least 10 random bytes (80 bits). Clarify the alphabet used.

4. **`user.deleted` outbox event with empty `org_id`:** The `DeleteUser` method publishes the outbox event with `org_id = ""`. If the outbox relay filters or routes by `org_id`, this event may never be published. Confirm whether the relay handles empty org_id events, and whether this is intentional (system-level event) or a bug.