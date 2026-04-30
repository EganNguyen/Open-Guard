# OpenGuard – E2E Test Suite

---

## 1. Overview

### 1.1 Scope

This suite covers all critical user journeys across OpenGuard's full stack:
IAM (authentication, MFA, SAML, WebAuthn, SCIM, OAuth 2.0 / PKCE), Policy Management
(CRUD, evaluation, assignment), Threat Detection (brute force, impossible travel, privilege
escalation, off-hours, data exfiltration), Alerting, DLP, Compliance Reporting, Webhook
Delivery, and Connector Registry.

Every test case validates both the external user behaviour and the internal system behaviour
(API calls, service interactions, database state, Kafka events, and external integrations).

### 1.2 Architecture Reference

| Layer | Technology |
|---|---|
| Entry point | Control-plane reverse proxy (chi, circuit breakers) |
| Identity | IAM service → PostgreSQL (RLS), Redis (sessions/blocklist) |
| Policy engine | Policy service → PostgreSQL (RLS), Redis (L1/L2 cache), Kafka outbox |
| Threat detection | Threat service → Kafka consumers, Redis, MongoDB (alerts) |
| Audit | Audit service → MongoDB (hash-chained ledger) |
| Messaging | Kafka (topics: `auth.events`, `policy.changes`, `threat.alerts`, `saga.orchestration`, `webhook.delivery`) |
| Async delivery | Webhook-delivery consumer → external HTTP targets |
| Compliance | Compliance service → ClickHouse, S3 (PDF reports, RSA-signed) |
| DLP | DLP service → PostgreSQL, regex + entropy scanners |

### 1.3 Test ID Convention

`TC-[domain abbreviation]-[sequence]`

| Domain | Prefix |
|---|---|
| Authentication | AUTH |
| Multi-Factor Auth | MFA |
| OAuth / PKCE | OAUTH |
| SAML SSO | SAML |
| WebAuthn | WA |
| SCIM Provisioning | SCIM |
| User Management | USR |
| Policy Management | POL |
| Policy Evaluation | EVAL |
| Threat Detection | THR |
| Alerting | ALT |
| DLP | DLP |
| Compliance | CMP |
| Webhook Delivery | WHK |
| Connector Registry | CON |
| Session / Security | SEC |

### 1.4 Precondition Glossary

- **Valid org**: An org record exists in `orgs` table with `status = 'active'`.
- **Active user**: A user with `status = 'active'` and a valid bcrypt password hash.
- **Valid JWT**: An `access_token` signed with the active key in the IAM keyring, not on the Redis blocklist, and not expired.
- **Active policy**: A policy record in `policies` with valid JSONB `logic` containing a CEL expression.

---

## 2. Authentication Flows

---

### TC-AUTH-001: Successful Password Login

- **User Flow:** User submits email and password via the login form; receives JWT tokens.
- **Preconditions:**
  - Active org and active user exist.
  - User has no MFA configured.
  - Redis is reachable.

- **Steps:**
  1. Navigate to `/login`.
  2. Submit `POST /auth/login` with `{ "email": "user@example.com", "password": "correct_password" }`.
  3. Observe the response.

- **Expected Results:**
  - HTTP 200 with body `{ "access_token": "...", "refresh_token": "...", "token_type": "Bearer", "expires_in": ... }`.
  - `mfa_required` key absent or `false`.

- **System Verifications:**
  - **APIs:** `POST /auth/login` → IAM service.
  - **Services:** `AuthWorkerPool.Compare` executes bcrypt comparison off the main goroutine. `IncrementFailedLogin` is NOT called. `ResetFailedLogin` IS called. `CreateSession` writes a new session row. Outbox event `auth.login` is written and relayed to Kafka topic `auth.events`.
  - **Database:** `sessions` table: new row with correct `jti`, `org_id`, `user_id`, `user_agent`, `ip_address`, `expires_at`. `users.last_login_at` updated. `failed_login_attempts` reset to 0.
  - **Redis:** Refresh token hash stored at `rt:{hash}` with TTL. `jti` added to active session index.
  - **Events:** `auth.events` Kafka topic receives `{ "event": "auth.login", "user_id": "...", "org_id": "..." }`.

- **Edge Cases:**
  - Login with email in mixed case → normalised to lowercase before lookup.
  - Request with `Content-Type: text/plain` → 400 Bad Request.
  - Request body > 1 MB → 413 Request Entity Too Large (IAM has `RequestSize(1<<20)` middleware).

- **Failure Scenarios:**
  - Wrong password → 401; `failed_login_attempts` incremented; no session created.
  - User status `initializing` → 403 `ACCOUNT_SETUP_PENDING`.
  - User status `deprovisioned` → 401 `INVALID_CREDENTIALS` (indistinguishable from wrong password).
  - Redis down during login → session write fails → 500; failed attempt NOT counted (db write rolled back).

---

### TC-AUTH-002: Account Lockout After Repeated Failures

- **User Flow:** Attacker submits wrong password multiple times; account is locked.
- **Preconditions:** Active user with `failed_login_attempts = 4`.

- **Steps:**
  1. Submit `POST /auth/login` with wrong password (5th attempt).
  2. Observe the response.
  3. Submit `POST /auth/login` with **correct** password.
  4. Observe the second response.

- **Expected Results:**
  - Step 2: HTTP 401.
  - Step 3: HTTP 401 (account locked, correct credentials irrelevant).

- **System Verifications:**
  - **Services:** After step 1: `IncrementFailedLogin` returns `5`; `LockAccount` is called with `until = now() + lockout_duration`.
  - **Database:** `users.locked_until` set to future timestamp. `users.failed_login_attempts = 5`.
  - **Events:** `auth.events` → `{ "event": "auth.account_locked", "user_id": "..." }`.

- **Edge Cases:**
  - Lockout expires → user can log in again normally; `locked_until` = NULL, `failed_login_attempts` = 0 after success.
  - Admin manually unlocks user via reprovision endpoint → lockout cleared immediately.

- **Failure Scenarios:**
  - Clock skew: `locked_until` in the past due to NTP drift → treated as unlocked (server-side check is `NOW() > locked_until`).

---

### TC-AUTH-003: JWT Refresh Token Rotation

- **User Flow:** Client exchanges a refresh token for new tokens; old token is invalidated.
- **Preconditions:** Active user with a valid refresh token stored in Redis.

- **Steps:**
  1. Submit `POST /auth/refresh` with `{ "refresh_token": "<valid_token>" }`.
  2. Observe the response.
  3. Submit `POST /auth/refresh` with the **same** refresh token again.
  4. Observe the second response.

- **Expected Results:**
  - Step 1: HTTP 200 with new `access_token` and `refresh_token`.
  - Step 3: HTTP 401 (token already consumed; potential refresh token reuse detected).

- **System Verifications:**
  - **Services:** Step 1: `ClaimRefreshToken` atomically marks old token as claimed. `CreateRefreshToken` issues new token in same `family_id`. Step 3: `GetRefreshToken` returns claimed token → `RevokeRefreshTokenFamily` called → all tokens in family revoked (RTR pattern).
  - **Database:** Old refresh token row: `status = 'claimed'`. New token row: `status = 'active'`. On reuse: entire family set to `status = 'revoked'`.
  - **Redis:** Old `jti` added to blocklist. New `jti` stored.
  - **Events:** `auth.events` → `auth.token_refresh`; on reuse → `auth.token_reuse_detected`.

- **Edge Cases:**
  - Expired refresh token → 401; no new tokens issued; family not revoked.
  - User agent / IP mismatch on refresh → Risk score computed; if ≥ 80, session revoked (see `TC-SEC-001`).

- **Failure Scenarios:**
  - Refresh token family already revoked → 401 immediately; no further DB writes.

---

### TC-AUTH-004: Logout Blocklists Access Token

- **User Flow:** User logs out; the access token is immediately invalidated.
- **Preconditions:** User is authenticated with a valid JWT `access_token`.

- **Steps:**
  1. Submit `POST /auth/logout` with `Authorization: Bearer <access_token>`.
  2. Submit `GET /auth/me` with the same token.

- **Expected Results:**
  - Step 1: HTTP 200 or 204.
  - Step 2: HTTP 401 (token on blocklist).

- **System Verifications:**
  - **Services:** `jti` extracted from token and written to Redis blocklist with TTL equal to token's remaining lifetime.
  - **Database:** Session row for `jti`: `revoked_at` set to `now()`. Refresh token family revoked.
  - **Redis:** `blocklist:{jti}` key present with positive TTL.
  - **Events:** `auth.events` → `auth.logout`.

- **Edge Cases:**
  - `POST /auth/logout` called twice with same token → second call also returns 200/204 (idempotent); token already on blocklist.
  - Token already expired at logout time → JWT middleware rejects the token before the logout handler runs → 401.

- **Failure Scenarios:**
  - Redis unavailable during `GET /auth/me` after logout → circuit breaker policy: **fail open** → 200 returned despite blocklisted token (known design trade-off; alert threshold should fire on `blocklist_fail_open_total`).

---

## 3. Multi-Factor Authentication

---

### TC-MFA-001: TOTP Setup and Enable

- **User Flow:** Admin enables TOTP for a user; user scans QR code and verifies.
- **Preconditions:** Active authenticated admin user. Target user has no MFA configured.

- **Steps:**
  1. Admin submits `GET /mgmt/users/mfa/totp/setup` with admin JWT.
  2. User scans the returned QR code / enters the `secret` in an authenticator app.
  3. User submits `POST /mgmt/users/mfa/totp/enable` with `{ "totp_code": "<valid 6-digit code>" }`.
  4. Observe response.

- **Expected Results:**
  - Step 1: HTTP 200 with `{ "secret": "...", "qr_code_url": "..." }`.
  - Step 3: HTTP 200; MFA enabled.

- **System Verifications:**
  - **Services:** TOTP secret is AES-256-GCM encrypted (using IAM `aesKeyring`) before DB write.
  - **Database:** `mfa_configs` row: `type = 'totp'`, `secret_encrypted` = ciphertext, `enabled = false` after step 1; `enabled = true` after step 3. `users.mfa_enabled = true`, `users.mfa_method = 'totp'`.
  - **Events:** `auth.events` → `auth.mfa_enabled` with `method = 'totp'`.

- **Edge Cases:**
  - TOTP code is 30 seconds old (clock drift allowance) → still accepted if within ±1 window.
  - TOTP code submitted twice within the same window → second attempt rejected (replay prevention).

- **Failure Scenarios:**
  - Invalid TOTP code during enable → 401; MFA not enabled; `mfa_configs.enabled` remains false.

---

### TC-MFA-002: MFA-Required Login Flow

- **User Flow:** User with TOTP enabled logs in; must verify TOTP before receiving tokens.
- **Preconditions:** Active user with `mfa_enabled = true`, `mfa_method = 'totp'`.

- **Steps:**
  1. Submit `POST /auth/login` with correct credentials.
  2. Observe initial response.
  3. Submit `POST /auth/mfa/verify` with `{ "mfa_challenge": "<challenge_from_step_2>", "totp_code": "<valid_code>" }`.
  4. Observe the response.

- **Expected Results:**
  - Step 1: HTTP 403 with `{ "error": "mfa_required", "mfa_challenge": "<challenge_token>" }`.
  - Step 3: HTTP 200 with full `access_token` and `refresh_token`.

- **System Verifications:**
  - **Services:** Step 1: `Login` returns `mfa_required = true`; a short-lived MFA challenge token is stored in Redis at `mfa_challenge:{challenge}`. Step 3: challenge token consumed (deleted from Redis); `IssueTokens` called; `CreateSession` written.
  - **Redis:** MFA challenge key TTL ≤ 5 minutes. Key deleted after successful verification.
  - **Database:** Session and refresh token rows created only after step 3.

- **Edge Cases:**
  - MFA challenge token expired → 401; user must log in again from step 1.
  - Backup code used instead of TOTP → `POST /auth/mfa/backup-verify`; backup code record consumed and marked as used in DB.

- **Failure Scenarios:**
  - Wrong TOTP code → 401; challenge token remains valid for remaining TTL; attempt counter incremented.
  - All backup codes exhausted → 401; user must contact admin for reprovision.

---

## 4. OAuth 2.0 / PKCE

---

### TC-OAUTH-001: Authorization Code Flow with PKCE

- **User Flow:** An integrated connector initiates OAuth login; user authenticates; connector receives tokens.
- **Preconditions:** A registered connector exists in `connectors` table with at least one `redirect_uri`. Active user in the org.

- **Steps:**
  1. Connector constructs and submits `GET /auth/authorize?client_id=<id>&redirect_uri=<uri>&code_challenge=<S256_hash>&code_challenge_method=S256&state=<random>`.
  2. IAM redirects to the dashboard login page with the same parameters.
  3. User authenticates via the dashboard.
  4. Dashboard calls `POST /auth/oauth/login` with credentials and `code_challenge`.
  5. Dashboard receives an auth code and redirects the user agent to the connector's `redirect_uri?code=<code>&state=<state>`.
  6. Connector submits `POST /auth/token` with `grant_type=authorization_code`, `code`, `code_verifier`, `client_id`.
  7. Observe response.

- **Expected Results:**
  - Step 1: HTTP 302 redirect to dashboard login URL.
  - Step 6: HTTP 200 with `access_token`, `refresh_token`, `token_type: "Bearer"`, `expires_in`.

- **System Verifications:**
  - **Services:** Step 4: auth code (base64-encoded UUID) and `code_challenge` stored in Redis at `auth_code:{code}` with 5-minute TTL. Step 6: `GetAuthCode` retrieves the challenge; PKCE verification: `SHA256(code_verifier) == stored_challenge` (constant-time compare). Redis key deleted after use. `IssueTokens` called; tokens issued.
  - **Redis:** `auth_code:{code}` key: present after step 4, absent after step 6 (one-time use).
  - **Database:** Session and refresh token rows created for the authenticated user after step 6.
  - **Events:** `auth.events` → `auth.oauth_token_issued` with `client_id` and `org_id`.

- **Edge Cases:**
  - `code_challenge_method` ≠ `S256` → 400 immediately at the authorize step.
  - `redirect_uri` not in connector's allowlist → 401 at the authorize step; no redirect issued (open redirect prevention).
  - Auth code used twice → second `POST /auth/token` → 401; code already consumed.
  - `state` parameter round-tripped through redirect without modification → connector must validate state before calling `/auth/token`.

- **Failure Scenarios:**
  - `code_verifier` does not match the stored `code_challenge` → 401; tokens not issued; code consumed (to prevent enumeration).
  - Auth code TTL expired → 401; no tokens issued.

---

### TC-OAUTH-002: Token Endpoint Idempotency

- **User Flow:** Connector submits duplicate `POST /auth/token` requests (network retry).
- **Preconditions:** `Idempotency-Key` middleware enabled on `/auth/token`.

- **Steps:**
  1. Submit `POST /auth/token` with `Idempotency-Key: <uuid>` and valid auth code.
  2. Submit identical `POST /auth/token` with the same `Idempotency-Key`.
  3. Observe both responses.

- **Expected Results:**
  - Both requests return HTTP 200 with **identical** response bodies and identical tokens.

- **System Verifications:**
  - **Redis:** `idempotency:{key}` stores the first response; TTL = 24 hours. Second request served from cache; no DB writes occur on second call.
  - **Database:** Only one session row and one refresh token row created (from first call).

- **Failure Scenarios:**
  - Same idempotency key, different request body → 422 Unprocessable Entity (key collision, body mismatch).

---

## 5. SAML SSO

---

### TC-SAML-001: SAML Identity Provider Login

- **User Flow:** Enterprise user authenticates via an external SAML IdP.
- **Preconditions:** SAML provider configured for the org (`saml_providers` row with valid `metadata_xml`). User exists in IdP and in OpenGuard with matching email.

- **Steps:**
  1. User navigates to the dashboard and selects "Login with SSO".
  2. Dashboard fetches `GET /auth/saml/metadata` to learn the service provider metadata.
  3. User is redirected to the IdP login page.
  4. User authenticates at the IdP; IdP posts a SAML assertion to `POST /auth/saml/acs`.
  5. IAM validates the assertion and issues tokens.
  6. User is redirected to the dashboard in authenticated state.

- **Expected Results:**
  - Step 2: HTTP 200 with XML containing OpenGuard SP entity ID and ACS URL.
  - Step 4: HTTP 302 redirect to dashboard with auth code or tokens in URL fragment.
  - Dashboard renders authenticated state.

- **System Verifications:**
  - **Services:** ACS handler: XML signature validated against IdP certificate stored in `saml_providers`. `NameID` email extracted; `GetUserByEmail` called. Session created.
  - **Database:** `saml_providers.last_used_at` updated. Session row created.
  - **Events:** `auth.events` → `auth.saml_login`.

- **Edge Cases:**
  - SAML assertion `NotOnOrAfter` is in the past (replayed assertion) → 401; assertion rejected.
  - User exists in IdP but not in OpenGuard → 401 or auto-provisioned depending on org config.

- **Failure Scenarios:**
  - Assertion XML signature invalid → 401; detailed error suppressed in response to avoid information leakage.
  - `metadata_xml` malformed at provider creation → `POST /auth/saml/providers` returns 400 with `invalid metadata_xml: <err>`.

---

## 6. WebAuthn / Passkey

---

### TC-WA-001: WebAuthn Credential Registration

- **User Flow:** Authenticated user registers a hardware security key or passkey.
- **Preconditions:** User is authenticated with valid JWT. WebAuthn configured with correct `WEBAUTHN_RP_ID`.

- **Steps:**
  1. Submit `POST /auth/webauthn/register/begin` with `Authorization: Bearer <token>`.
  2. Receive the credential creation options (challenge, rp, user).
  3. User performs authenticator gesture (touch, FaceID, etc.) → browser returns `PublicKeyCredential`.
  4. Submit `POST /auth/webauthn/register/finish` with the attestation response.
  5. Observe response.

- **Expected Results:**
  - Step 1: HTTP 200 with `PublicKeyCredentialCreationOptions` JSON.
  - Step 4: HTTP 200; credential saved.

- **System Verifications:**
  - **Services:** Step 1: WebAuthn session challenge stored in Redis/session. Step 4: Attestation verified by `go-webauthn` library against the stored challenge. `SaveWebAuthnCredential` called.
  - **Database:** `webauthn_credentials` row: `org_id`, `user_id`, `credential_id` (bytes), `public_key`, `sign_count = 0`, `aaguid`, `created_at`.

- **Failure Scenarios:**
  - RP ID mismatch (credential registered on `example.com`, used on `evil.com`) → 400; credential rejected by WebAuthn library.
  - Challenge replay (same challenge submitted twice) → 400; challenge already consumed.

---

### TC-WA-002: WebAuthn Login

- **User Flow:** User authenticates using a registered passkey instead of password.
- **Preconditions:** User has at least one registered WebAuthn credential.

- **Steps:**
  1. Submit `POST /auth/webauthn/login/begin` with `{ "email": "user@example.com" }`.
  2. Receive authentication options (challenge, allowed credentials).
  3. User performs authenticator gesture → browser returns signed assertion.
  4. Submit `POST /auth/webauthn/login/finish` with the assertion.
  5. Observe response.

- **Expected Results:**
  - Step 1: HTTP 200 with `PublicKeyCredentialRequestOptions`.
  - Step 4: HTTP 200 with `access_token` and `refresh_token`.

- **System Verifications:**
  - **Services:** Step 4: Assertion signature verified. `sign_count` checked: must be > stored count (clone detection). `webauthn_credentials.sign_count` updated. Session and refresh token created.
  - **Database:** `webauthn_credentials.sign_count` incremented. Session row created.

- **Failure Scenarios:**
  - `sign_count` in assertion ≤ stored count → 401 (authenticator cloned); credential should be flagged.
  - Credential ID not found in user's credentials → 401.

---

## 7. SCIM 2.0 Provisioning

---

### TC-SCIM-001: SCIM User Provisioning (Create)

- **User Flow:** Identity Provider (Okta/Azure AD) provisions a new user via SCIM.
- **Preconditions:** SCIM bearer token configured via `SCIM_TOKENS` env var. Org ID associated with the token.

- **Steps:**
  1. IdP submits `POST /auth/scim/v2/Users` with `Authorization: Bearer <scim_token>` and SCIM user payload.
  2. Observe response.
  3. Verify the user status reaches `active` after the provisioning saga completes.

- **Expected Results:**
  - Step 1: HTTP 201 with SCIM user representation including `id` and `meta`.
  - Step 3: User `status = 'active'` in DB within 30 seconds.

- **System Verifications:**
  - **Services:** `RegisterUser` called with `scim_external_id`. Transactional outbox event `user.created` published to Kafka topic `saga.orchestration`. Saga deadline written to Redis `saga:deadlines` sorted set with 40-second TTL.
  - **Kafka:** `saga.orchestration` topic receives `{ "event": "user.created", "user_id": "...", "status": "initializing" }`. Downstream saga consumer processes event and publishes `user.scim.provisioned`. IAM saga consumer receives `user.scim.provisioned` → calls `UpdateUserStatus("active")`.
  - **Database:** `users` row: `status = 'initializing'` immediately, transitions to `'active'` after saga. `users.scim_external_id` set. `outbox` row created and transitioned to `published`.
  - **Redis:** `saga:deadlines` contains user ID with score = deadline Unix timestamp.

- **Edge Cases:**
  - Same SCIM request sent twice (duplicate `externalId`) → `PostScimUser` returns existing user's representation with HTTP 200 (idempotent).
  - Saga times out before `user.scim.provisioned` received → Saga watcher fires; `UpdateUserStatus("provisioning_failed")`.

- **Failure Scenarios:**
  - Invalid SCIM bearer token → 401 from SCIM middleware before handler runs.
  - User email already exists in org → conflict error; HTTP 409.
  - Previously deprovisioned user's `externalId` reused → 409 `CONFLICT: user was deprovisioned`.

---

### TC-SCIM-002: SCIM User Deprovisioning (Delete)

- **User Flow:** Admin removes a user in the IdP; SCIM deprovisions them in OpenGuard.
- **Preconditions:** Active user with `scim_external_id` set.

- **Steps:**
  1. IdP submits `DELETE /auth/scim/v2/Users/{id}` with valid SCIM token.
  2. Observe response.
  3. Attempt `POST /auth/login` as the deprovisioned user.

- **Expected Results:**
  - Step 1: HTTP 204 No Content.
  - Step 3: HTTP 401.

- **System Verifications:**
  - **Services:** `UpdateUserStatus("deprovisioned")` called. All active sessions for the user revoked. All refresh token families revoked.
  - **Kafka:** `saga.orchestration` → `{ "event": "user.deprovisioned", "user_id": "..." }`.
  - **Database:** `users.status = 'deprovisioned'`. `sessions` rows for user: `revoked_at` set. `refresh_tokens` for user: `status = 'revoked'`.
  - **Redis:** All `jti` values from user's sessions added to blocklist.

---

### TC-SCIM-003: SCIM PATCH – Disable User

- **User Flow:** Admin disables (but does not delete) a user in the IdP.
- **Preconditions:** Active user with SCIM external ID.

- **Steps:**
  1. IdP submits `PATCH /auth/scim/v2/Users/{id}` with `{ "Operations": [{ "op": "replace", "path": "active", "value": false }] }`.
  2. Observe response.
  3. Attempt to use an existing access token for the disabled user.

- **Expected Results:**
  - Step 1: HTTP 200 with updated SCIM user representation (`active: false`).
  - Step 3: HTTP 401.

- **System Verifications:**
  - **Services:** PATCH handler parses operations; `active = false` maps to `UpdateUserStatus("suspended")`. Sessions revoked.
  - **Database:** `users.status = 'suspended'`. Sessions revoked.
  - **Events:** `auth.events` → `auth.user_suspended`.

---

## 8. User Management

---

### TC-USR-001: Org and User Creation

- **User Flow:** Super-admin creates a new org and initial admin user.
- **Preconditions:** Authenticated super-admin with a valid JWT that has `mgmt` scope.

- **Steps:**
  1. Submit `POST /mgmt/orgs` with `{ "name": "Acme Corp" }`.
  2. Note the returned `org_id`.
  3. Submit `POST /mgmt/users` with `{ "email": "admin@acme.com", "password": "...", "role": "admin" }` and `Idempotency-Key: <uuid>`.
  4. Observe response.

- **Expected Results:**
  - Step 1: HTTP 201 with `{ "id": "<uuid>", "name": "Acme Corp", "slug": "acme-corp" }`.
  - Step 3: HTTP 201 with the new user's ID.

- **System Verifications:**
  - **Services:** `AuthWorkerPool.Generate` used for bcrypt hashing (non-blocking). Transactional outbox event written; Kafka `saga.orchestration` notified.
  - **Database:** `orgs` row created. `users` row created with `status = 'initializing'`. RLS `app.org_id` session variable ensures new user is scoped to the new org.
  - **Events:** `saga.orchestration` → `user.created`. User status eventually transitions to `active`.

- **Edge Cases:**
  - `Idempotency-Key` reused for user creation → second call returns cached 201 response; no duplicate user created.
  - Slug collision (two orgs with same name) → slug auto-incremented (e.g., `acme-corp-2`).

- **Failure Scenarios:**
  - Duplicate email within same org → 409 Conflict.
  - Password too short / fails policy → 400 Bad Request.

---

### TC-USR-002: User Reprovision

- **User Flow:** Admin re-activates a `provisioning_failed` or `suspended` user.
- **Preconditions:** User with `status = 'provisioning_failed'` or `'suspended'`.

- **Steps:**
  1. Submit `POST /mgmt/users/{id}/reprovision` with admin JWT.
  2. Observe response.
  3. Attempt login as the reprovisioned user.

- **Expected Results:**
  - Step 1: HTTP 200.
  - Step 3: HTTP 200 with tokens (if provisioning saga completes successfully).

- **System Verifications:**
  - **Services:** `UpdateUserStatus("initializing")` called. New outbox event `user.reprovisioned` published to `saga.orchestration`. Saga runs again.
  - **Database:** `users.status` transitions: `provisioning_failed` → `initializing` → `active`.

---

## 9. Policy Management

---

### TC-POL-001: Create Policy with Valid CEL Expression

- **User Flow:** Admin creates a new ABAC policy.
- **Preconditions:** Authenticated admin with valid JWT. Empty policy list for the org.

- **Steps:**
  1. Submit `POST /v1/policies` with:
     ```json
     {
       "name": "Allow Data Viewers",
       "description": "Viewers can read all reports",
       "logic": { "expression": "subject.role == 'viewer' && resource.startsWith('reports/')" }
     }
     ```
     and `Idempotency-Key: <uuid>`.
  2. Observe response.
  3. Submit `GET /v1/policies/{id}` with the returned ID.

- **Expected Results:**
  - Step 1: HTTP 201 with policy object including `id`, `version: 1`, `ETag: "<id>-v1"`.
  - Step 3: HTTP 200 with the same policy object.

- **System Verifications:**
  - **Services:** `CreatePolicy` called. Outbox record created in the same transaction. Org Redis cache invalidated asynchronously.
  - **Kafka:** `policy.changes` topic receives `{ "event": "policy.created", "policy_id": "...", "org_id": "..." }`.
  - **Database:** `policies` row: `org_id`, `name`, `description`, `logic` (JSONB), `version = 1`. `outbox_records` row: `status = 'pending'` → transitions to `'published'`. RLS enforced: policy only visible with the correct `app.org_id` session variable.
  - **Redis:** Org-scoped cache key for the policy list is invalidated.

- **Edge Cases:**
  - Policy name duplicate within same org → 409 Conflict (unique constraint on `(org_id, name)`).
  - `logic` field is valid JSON but contains invalid CEL syntax → Currently accepted (400 only on JSON parse failure). See known gap in code review.
  - Request body exactly 512 KB (policy service limit) → accepted. 512 KB + 1 byte → 413.

- **Failure Scenarios:**
  - `logic` field missing → 400 `name and logic are required`.
  - Malformed JSON body → 400 `invalid request body`.
  - Duplicate `Idempotency-Key` → second request returns cached 201; no second DB write.

---

### TC-POL-002: Update Policy Increments Version

- **User Flow:** Admin updates an existing policy; version is bumped for ETag-based caching.
- **Preconditions:** Policy with `version = 1` exists.

- **Steps:**
  1. Submit `PUT /v1/policies/{id}` with updated `name` and `logic`.
  2. Observe response headers and body.
  3. Submit `GET /v1/policies/{id}` and compare ETag.

- **Expected Results:**
  - Step 1: HTTP 200 with `version: 2` and `ETag: "<id>-v2"`.
  - Step 3: `version: 2`, ETag matches.

- **System Verifications:**
  - **Database:** `policies.version` incremented to 2. `policies.updated_at` updated.
  - **Redis:** Org cache keys evicted. CEL program cache entry for the policy invalidated (when CEL cache is implemented; currently re-compiled on next evaluation).
  - **Kafka:** `policy.changes` → `{ "event": "policy.updated", "policy_id": "...", "version": 2 }`.

- **Failure Scenarios:**
  - Policy `id` not found for org → 404 Not Found.
  - Policy `id` exists but belongs to a different org → RLS returns ErrNotFound → 404 (not 403; no cross-tenant leak).

---

### TC-POL-003: Delete Policy Cascades to Assignments

- **User Flow:** Admin deletes a policy; all assignments referencing it are removed.
- **Preconditions:** Policy with two assignments exists.

- **Steps:**
  1. Confirm assignments exist: `GET /v1/assignments`.
  2. Submit `DELETE /v1/policies/{id}`.
  3. Confirm assignments are removed: `GET /v1/assignments`.

- **Expected Results:**
  - Step 2: HTTP 200 `{ "status": "deleted" }`.
  - Step 3: Assignment list does not contain the deleted policy's assignments.

- **System Verifications:**
  - **Database:** `policies` row deleted. `policy_assignments` rows with `policy_id = <id>` deleted via `ON DELETE CASCADE` foreign key. `policy_eval_log` rows retained (audit trail; no cascade on eval logs).
  - **Kafka:** `policy.changes` → `policy.deleted`.
  - **Redis:** Org cache invalidated.

---

### TC-POL-004: Policy Assignment to User

- **User Flow:** Admin assigns a policy to a specific user.
- **Preconditions:** Active policy and active user in the same org.

- **Steps:**
  1. Submit `POST /v1/assignments` with `{ "policy_id": "<id>", "subject_id": "<user_id>", "subject_type": "user" }`.
  2. Observe response.
  3. Evaluate a request for that user to verify the policy applies.

- **Expected Results:**
  - Step 1: HTTP 201 with assignment object.
  - Step 3: Evaluation returns `effect: "allow"` (or `"deny"` per policy logic).

- **System Verifications:**
  - **Database:** `policy_assignments` row created. RLS enforced on insert.
  - **Redis:** Org assignment cache invalidated.

- **Failure Scenarios:**
  - `policy_id` does not exist in the org → 404 (FK constraint violation mapped to ErrNotFound).
  - Duplicate assignment (same policy + subject) → 409 Conflict.

---

## 10. Policy Evaluation

---

### TC-EVAL-001: Policy Evaluation – Allow Decision (Cache Miss)

- **User Flow:** A connector evaluates access; no cache exists; DB is queried and result cached.
- **Preconditions:** Active policy assigned to the requesting user. Redis cache empty for org.

- **Steps:**
  1. Submit `POST /v1/policy/evaluate` with:
     ```json
     { "subject_id": "<user_id>", "action": "read", "resource": "reports/q3-2025", "context": { "role": "viewer" } }
     ```
  2. Observe response and timing.
  3. Submit identical request immediately after.
  4. Observe second response and timing.

- **Expected Results:**
  - Step 1: HTTP 200 `{ "effect": "allow", "matched_policy_ids": ["<id>"], "cache_hit": false }` with `ETag` header.
  - Step 3: HTTP 200 with `cache_hit: true`; measurably lower latency.

- **System Verifications:**
  - **Services:** Step 1: `singleflight.DoChan` called; DB queried via `evaluateFromDB`; CEL expression compiled and evaluated; result written to Redis with 60-second TTL; result written to singleflight shared result. Step 3: Redis L2 cache hit; DB not queried.
  - **Database (Step 1):** `policy_eval_log` row created: `effect = 'allow'`, `cache_hit = false`, `matched_policy_ids`, `latency_ms`. `logCh` goroutine processes the write asynchronously.
  - **Database (Step 3):** `policy_eval_log` row created: `cache_hit = true`.
  - **Redis (Step 1):** `policy:{org_id}:{hash(subject,action,resource)}` key set with 60s TTL. Index key `policy_idx:{org_id}` updated with cache key.

- **Edge Cases:**
  - Request with `subject_id` having no assigned policies → `effect: "deny"` (default deny); evaluation logged.
  - Two concurrent identical requests arrive simultaneously → singleflight coalesces them; one DB query; both callers receive same result.

- **Failure Scenarios:**
  - 5-second policy service timeout (set at router level) → HTTP 504 returned by control-plane circuit breaker.
  - Redis unavailable → L2 cache miss; falls through to DB; evaluation succeeds; cache write skipped.

---

### TC-EVAL-002: Policy Evaluation – Deny Decision and Audit Log

- **User Flow:** User attempts a forbidden action; denial is recorded in the audit log.
- **Preconditions:** Active policy that explicitly denies the action for the user's role.

- **Steps:**
  1. Submit `POST /v1/policy/evaluate` with an action that should be denied.
  2. Observe response.
  3. Query eval logs: `GET /v1/policy/eval-logs`.

- **Expected Results:**
  - Step 1: HTTP 200 `{ "effect": "deny", "matched_policy_ids": ["<deny_policy_id>"], "cache_hit": false }`.
  - Step 3: Latest log entry shows `effect = 'deny'` for the evaluated subject/action/resource triple.

- **System Verifications:**
  - **Database:** `policy_eval_log` row: `effect = 'deny'`. `matched_policy_ids` contains the deny policy ID.
  - **Kafka:** `auth.events` (or a separate access control topic) receives the deny decision for downstream audit.

---

### TC-EVAL-003: Evaluation with Stale Cache After Policy Update

- **User Flow:** Admin updates a policy; next evaluation reflects the new logic without stale cache serving.
- **Preconditions:** Policy evaluated at least once (cache populated). Redis cache populated.

- **Steps:**
  1. Evaluate request → `effect: "allow"`, `cache_hit: false`.
  2. Evaluate identical request → `effect: "allow"`, `cache_hit: true`.
  3. Admin updates the policy to deny the same action (`PUT /v1/policies/{id}`).
  4. Evaluate the same request again.

- **Expected Results:**
  - Step 4: HTTP 200 `{ "effect": "deny", "cache_hit": false }`.

- **System Verifications:**
  - **Services:** Step 3 triggers `InvalidateOrgCache`; Redis keys `policy:{org_id}:*` deleted; index key deleted. Step 4 is a fresh DB query.
  - **Kafka:** `policy.changes` event received by policy service subscriber; in-memory subscriber triggers cache invalidation.

- **Failure Scenarios:**
  - Cache invalidation only partially succeeds (TOCTOU race between SMembers and Del; see known issue) → stale evaluation possible for up to 60 seconds (TTL). Monitor via `eval_cache_stale_total` metric.

---

## 11. Threat Detection

---

### TC-THR-001: Brute Force Detection

- **User Flow:** Attacker submits many failed login attempts; brute force alert is raised.
- **Preconditions:** Brute force detector running, consuming `auth.events`.

- **Steps:**
  1. Submit 10 `POST /auth/login` requests with wrong password for the same email within 60 seconds.
  2. Observe alert store after a few seconds.

- **Expected Results:**
  - A threat alert with `type = 'brute_force'` appears in MongoDB alert store.
  - Alert `severity` reflects the number of attempts.

- **System Verifications:**
  - **Services:** Each failed login publishes `auth.login_failed` to `auth.events`. `BruteForceDetector` consumes these events. Redis counter `brute:{email}` incremented. When threshold crossed: `publishThreatEvent` called.
  - **Kafka:** `threat.alerts` topic receives brute force event.
  - **MongoDB:** Alert document inserted with `org_id`, `user_id`, `type = 'brute_force'`, `status = 'open'`, `severity`, `created_at`.
  - **Redis:** `brute:{email}` counter with sliding TTL.

- **Edge Cases:**
  - Attempts spread across 61 seconds (outside window) → counter resets; threshold not reached; no alert.
  - Multiple source IPs for same email → counter still accumulates (keyed by email, not IP).

- **Failure Scenarios:**
  - Kafka consumer lag > threshold window → detector may miss alert window. Monitor consumer lag.

---

### TC-THR-002: Impossible Travel Detection

- **User Flow:** User logs in from New York, then 5 minutes later from Tokyo; impossible travel alert raised.
- **Preconditions:** `ImpossibleTravelDetector` running with GeoLite2 DB loaded. Threshold = 500 km.

- **Steps:**
  1. Simulate a successful login event from IP geolocating to New York.
  2. Simulate a successful login event from IP geolocating to Tokyo 5 minutes later.
  3. Observe alert store.

- **Expected Results:**
  - A threat alert `type = 'impossible_travel'` appears in MongoDB.
  - Alert includes distance (km) and time delta in metadata.

- **System Verifications:**
  - **Services:** Event 1: `LastLogin` stored in Redis at `geo_login:{user_id}`. Event 2: previous `LastLogin` retrieved; Haversine distance calculated; distance > threshold AND time delta < 1 hour → `detect` triggered; `publishThreatEvent` called.
  - **Kafka:** `threat.alerts` → impossible travel event with `dist_km`, `time_delta_sec`.
  - **MongoDB:** Alert document with `type = 'impossible_travel'`, `metadata.distance_km`, `metadata.time_delta_seconds`.

- **Edge Cases:**
  - GeoLite2 DB returns no record for IP (private IP range) → event skipped silently; no false alert.
  - Distance exactly at threshold (500 km) → alert raised (≥ threshold condition).

- **Failure Scenarios:**
  - GeoLite2 DB file missing at startup → `ImpossibleTravelDetector` returns error; `main.go` warns and continues without it; other detectors unaffected.

---

### TC-THR-003: Off-Hours Access Detection

- **User Flow:** User accesses the system at 3 AM (outside configured business hours); alert raised.
- **Preconditions:** `OffHoursDetector` running. Business hours configured as 09:00–18:00 UTC.

- **Steps:**
  1. Simulate a login event with timestamp 03:00 UTC.
  2. Observe alert store.

- **Expected Results:**
  - Alert `type = 'off_hours_access'` in MongoDB within seconds.

- **System Verifications:**
  - **Kafka:** `auth.events` consumed. Server-side time check; `publishThreatEvent` called.
  - **MongoDB:** Alert with `org_id`, `user_id`, `type = 'off_hours_access'`.

- **Edge Cases:**
  - Event timestamp exactly at 09:00:00 UTC → no alert (boundary inclusive for business hours).
  - Event timestamp at 08:59:59 → alert raised.

---

### TC-THR-004: Privilege Escalation Detection

- **User Flow:** A policy change significantly elevates a subject's permissions; alert raised.
- **Preconditions:** `PrivilegeEscalationDetector` consuming `policy.changes` and `auth.events`. Baseline permissions stored.

- **Steps:**
  1. Admin creates a policy granting broad `admin` access to a non-admin user.
  2. Observe alert store.

- **Expected Results:**
  - Alert `type = 'privilege_escalation'` with details of the policy change.

- **System Verifications:**
  - **Kafka:** `policy.changes` event consumed by detector. Permission delta computed. If delta exceeds threshold, `publishThreatEvent`.
  - **MongoDB:** Alert with `type = 'privilege_escalation'`, `policy_id`, `subject_id`.

---

### TC-THR-005: Data Exfiltration Detection

- **User Flow:** A user downloads unusually large volumes of data; alert raised.
- **Preconditions:** `DataExfiltrationDetector` consuming `data.access` events.

- **Steps:**
  1. Simulate 50 data access events for the same user within 10 minutes totalling > exfiltration threshold.
  2. Observe alert store.

- **Expected Results:**
  - Alert `type = 'data_exfiltration'` raised.

- **System Verifications:**
  - **Redis:** Sliding window counter for user's data volume.
  - **Kafka:** `threat.alerts` → exfiltration event.
  - **MongoDB:** Alert document with volume metadata.

---

## 12. Alerting

---

### TC-ALT-001: List and Filter Alerts

- **User Flow:** Security analyst views active high-severity alerts.
- **Preconditions:** At least 3 alerts in MongoDB: 1 `open/high`, 1 `acknowledged/medium`, 1 `resolved/low`.

- **Steps:**
  1. Submit `GET /v1/threats/alerts?status=open&severity=high&limit=10` with valid JWT.
  2. Observe response.

- **Expected Results:**
  - HTTP 200; response contains only the `open/high` alert. Pagination cursor present in `X-Next-Cursor` header.

- **System Verifications:**
  - **Services:** MongoDB query filters on `org_id` (from JWT), `status`, `severity`; sorted by `created_at DESC`; limited to `limit`. Cursor-based pagination uses `_id` as the cursor field.
  - **MongoDB:** Query uses indexes on `org_id + status + severity`.

- **Edge Cases:**
  - `limit` not provided → defaults to 50.
  - `cursor` provided from previous page → returns the next page without overlap.

---

### TC-ALT-002: Acknowledge Alert

- **User Flow:** Analyst acknowledges an alert to signal it is being investigated.
- **Preconditions:** Alert with `status = 'open'` exists.

- **Steps:**
  1. Submit `POST /v1/threats/alerts/{id}/acknowledge` with valid JWT.
  2. Observe response.
  3. Submit `GET /v1/threats/alerts/{id}` and verify status.

- **Expected Results:**
  - Step 1: HTTP 204 No Content.
  - Step 3: `status = 'acknowledged'`, `acknowledged_at` set.

- **System Verifications:**
  - **MongoDB:** Alert document updated: `status = 'acknowledged'`, `acknowledged_by = <user_id>`, `acknowledged_at = now()`.

- **Failure Scenarios:**
  - Alert already `resolved` → 409 Conflict (cannot acknowledge resolved alert).
  - Alert ID not found for org → 404.

---

### TC-ALT-003: Resolve Alert

- **User Flow:** Analyst closes an alert as resolved after investigation.
- **Preconditions:** Alert with `status = 'open'` or `'acknowledged'` exists.

- **Steps:**
  1. Submit `POST /v1/threats/alerts/{id}/resolve`.
  2. Verify alert `status = 'resolved'`.

- **Expected Results:**
  - HTTP 204; MongoDB document updated to `resolved`.

---

## 13. DLP

---

### TC-DLP-001: Content Scan Detects PII

- **User Flow:** Connector submits content for DLP scanning; PII is detected and a finding persisted.
- **Preconditions:** Active DLP policy with `rules: ["credit_card", "ssn"]` and `action: "block"`. User authenticated with valid JWT.

- **Steps:**
  1. Submit `POST /v1/dlp/scan` with `{ "content": "My card is 4532-0151-3456-7899 and SSN 123-45-6789" }`.
  2. Observe response.
  3. Submit `GET /v1/dlp/findings` and verify persistence.

- **Expected Results:**
  - Step 1: HTTP 200 with findings array: `[{ "kind": "credit_card", "value": "****-****-****-7899", "risk_score": 0.9 }, { "kind": "ssn", "value": "***-**-6789", "risk_score": 0.95 }]`. Values masked in response.
  - Step 3: Finding records present for org.

- **System Verifications:**
  - **Services:** `ScanRegex` and `ScanEntropy` run against content. Findings matched against active policies. `SaveFinding` called for each match.
  - **Database (DLP):** `dlp_findings` row: `org_id`, `policy_id`, `finding_type`, `action`. Raw content NOT stored (privacy).

- **Edge Cases:**
  - Content with high entropy string (API key) but no regex match → `ScanEntropy` detects it as `high_entropy`; persisted if a matching policy rule exists.
  - Empty `content` field → HTTP 400 Bad Request.
  - No active DLP policies for org → scan still runs; findings returned but none persisted.

- **Failure Scenarios:**
  - Content > 512 KB (DLP service inherits gorilla/mux, no explicit body limit set on that route) → request succeeds but scanner performance degrades. Should be capped.

---

### TC-DLP-002: DLP Policy CRUD

- **User Flow:** Admin creates a DLP policy.
- **Preconditions:** Authenticated admin.

- **Steps:**
  1. Submit `POST /v1/dlp/policies` with `{ "name": "PII Block", "rules": ["credit_card", "ssn"], "action": "block", "enabled": true }`.
  2. Submit `GET /v1/dlp/policies` and verify the policy appears.

- **Expected Results:**
  - Step 1: HTTP 201 with policy object.
  - Step 2: Policy present in list with `enabled: true`.

- **System Verifications:**
  - **Database:** DLP policies table row created for `org_id`. RLS enforced.

---

## 14. Compliance Reporting

---

### TC-CMP-001: Generate and Download Compliance Report

- **User Flow:** Compliance officer generates a PDF compliance report; downloads it.
- **Preconditions:** Authenticated compliance officer. ClickHouse populated with audit events. S3 storage configured. RSA signing key set via `COMPLIANCE_SIGNING_KEY_PATH`.

- **Steps:**
  1. Submit `POST /v1/compliance/reports` with `{ "type": "SOC2", "period": { "start": "2025-01-01", "end": "2025-03-31" } }`.
  2. Note the returned `report_id`.
  3. Poll `GET /v1/compliance/reports/{id}` until `status = 'ready'`.
  4. Submit `GET /v1/compliance/reports/{id}/download`.
  5. Receive the PDF and verify the RSA-PSS signature.

- **Expected Results:**
  - Step 1: HTTP 202 Accepted with `{ "report_id": "...", "status": "pending" }`.
  - Step 3: Eventually HTTP 200 with `status = 'ready'` and `download_url`.
  - Step 4: HTTP 200 with `Content-Type: application/pdf`; PDF contains audit events for the period.
  - Step 5: RSA-PSS signature (included in PDF metadata or as a sidecar) verifiable with the org's public key.

- **System Verifications:**
  - **Services:** Step 1: `CreateReport` writes report record with `status = 'pending'`; bulkhead-controlled background goroutine started. Background job queries ClickHouse for events in period; generates PDF via `gofpdf`; signs with RSA-PSS + SHA-256; uploads to S3; updates report record to `status = 'ready'`. Bulkhead limits concurrent report generation.
  - **ClickHouse:** Audit events queried for `org_id` and date range.
  - **S3:** PDF uploaded at key `reports/{org_id}/{report_id}.pdf`.
  - **Database:** `compliance_reports` row: `status` transitions `pending → generating → ready`. `s3_key` populated.

- **Edge Cases:**
  - Period with no audit events → PDF generated with "No events found" page; `status = 'ready'`.
  - Two concurrent report requests → bulkhead limits concurrency; second request may queue or return 429.

- **Failure Scenarios:**
  - S3 upload fails → report status set to `'failed'`; `error_message` populated. `GET /v1/compliance/reports/{id}` returns `status = 'failed'`.
  - Signing key not configured → PDF generated without signature; no error (graceful degradation; this is a security gap).

---

### TC-CMP-002: Compliance Posture and Stats

- **User Flow:** Compliance officer views the org's current security posture score.
- **Preconditions:** Audit events exist in ClickHouse. Compliance consumer has processed events.

- **Steps:**
  1. Submit `GET /v1/compliance/posture` with valid JWT.
  2. Submit `GET /v1/compliance/stats`.

- **Expected Results:**
  - Step 1: HTTP 200 with posture score and breakdown by control category.
  - Step 2: HTTP 200 with aggregated statistics.

- **System Verifications:**
  - **ClickHouse:** Queries use `org_id` filter. Results aggregated server-side.
  - **Services:** Compliance consumer must have written events to ClickHouse from `audit.events` Kafka topic.

---

## 15. Webhook Delivery

---

### TC-WHK-001: Successful Webhook Delivery

- **User Flow:** A system event triggers a webhook; it is delivered to the configured target URL.
- **Preconditions:** Connector registered with a webhook target URL. Kafka `webhook.delivery` topic contains a delivery request.

- **Steps:**
  1. Publish a `WebhookDeliveryRequest` to `webhook.delivery` Kafka topic: `{ "target": "https://example.com/hook", "payload": "...", "secret": "...", "org_id": "...", "event_id": "..." }`.
  2. Observe the target URL receives the HTTP POST within 5 seconds.
  3. Verify delivery record in `webhook_deliveries` table.

- **Expected Results:**
  - Target endpoint receives `POST` with correct payload and `X-OpenGuard-Signature` HMAC-SHA256 header.
  - `webhook_deliveries` row: `status = 'delivered'`, `attempts = 1`.

- **System Verifications:**
  - **Services:** `WebhookConsumer.Start` fetches message; semaphore (max 50 concurrent) acquired; `Deliverer.Deliver` called. HTTP POST made with SSRF-safe HTTP client (private IPs blocked). HMAC-SHA256 computed with `secret`; included in `X-OpenGuard-Signature`. Kafka offset committed only after successful DB status update.
  - **Database:** `webhook_deliveries` updated: `status = 'delivered'`, `attempts = 1`.
  - **Events:** Kafka offset committed; message not reprocessed.

- **Edge Cases:**
  - Target returns 2xx but with slow response (29 seconds) → delivery marked `delivered`.
  - Duplicate Kafka delivery (broker retry) → `event_id` deduplication check; second delivery attempt skipped (idempotent on `event_id`).

- **Failure Scenarios:**
  - Target returns 500 → retry with exponential backoff (1s, 2s, 4s, ...); after max retries → `status = 'dlq'`.
  - Target URL resolves to a private IP (SSRF attempt) → `SafeHTTPClient` blocks the request; delivery marked `failed` immediately; no retry.
  - Target URL uses self-signed TLS → connection fails; delivery marked `failed`.

---

### TC-WHK-002: Webhook Retry with Exponential Backoff

- **User Flow:** Webhook delivery fails repeatedly; retries occur with backoff; eventually marked DLQ.
- **Preconditions:** Target URL returns 503 for all attempts.

- **Steps:**
  1. Publish a delivery request with a target that always returns 503.
  2. Observe delivery attempts over time.

- **Expected Results:**
  - Attempt 1 at T+0 → 503.
  - Attempt 2 at T+1s → 503.
  - Attempt 3 at T+3s → 503.
  - After max retries: `status = 'dlq'`, `last_error` populated.

- **System Verifications:**
  - **Database:** `webhook_deliveries.attempts` incremented on each try. `next_retry_at` set using `DefaultBackoff(i) = 1<<i seconds`. `status = 'dlq'` on exhaustion.
  - **Services:** `DefaultBackoff` function governs delay between attempts.

---

## 16. Connector Registry

---

### TC-CON-001: Register Connector and Validate API Key

- **User Flow:** Admin registers a new connector integration; receives an API key; key is validated.
- **Preconditions:** Authenticated admin with valid JWT.

- **Steps:**
  1. Submit `POST /v1/connectors` with:
     ```json
     { "id": "connector-abc", "org_id": "<org_id>", "name": "Slack", "redirect_uris": ["https://slack.com/callback"] }
     ```
     and `Idempotency-Key: <uuid>`.
  2. Note the returned `api_key`.
  3. Submit `POST /v1/connectors/validate` with `X-API-Key: <api_key>`.
  4. Observe response.

- **Expected Results:**
  - Step 1: HTTP 201 with `{ "id": "connector-abc", "api_key": "<plaintext_key>" }`. Key shown **only once**.
  - Step 3: HTTP 200 with connector metadata.

- **System Verifications:**
  - **Services:** Step 1: `RegisterConnector` generates a random API key. Key hashed with PBKDF2-SHA256 (per `shared/crypto/apikey.go`). Only hash stored in DB. Plaintext returned once. Step 3: `ValidateAPIKey` iterates stored hashes using `ValidateAPIKey` (constant-time compare).
  - **Database:** `connectors` row: `api_key_hash` (PBKDF2 hash), NOT plaintext. `redirect_uris` stored as array.

- **Edge Cases:**
  - Same `Idempotency-Key` → second request returns same `api_key` from idempotency cache (only within 24-hour window).
  - API key lost → no recovery; admin must delete and re-register connector.

- **Failure Scenarios:**
  - `X-API-Key` not provided → 401 `missing x-api-key header`.
  - Invalid API key → 401 after constant-time comparison (timing-safe).

---

## 17. Session Security

---

### TC-SEC-001: Session Revocation on Risk Score Threshold

- **User Flow:** Refresh token is used from a significantly different device/network; session revoked.
- **Preconditions:** Active session established from Chrome on macOS from IP `1.2.3.4`.

- **Steps:**
  1. Submit `POST /auth/refresh` with the refresh token from an entirely different User-Agent (Firefox on Linux) and from subnet `10.0.0.0/8`.
  2. Observe response.

- **Expected Results:**
  - HTTP 401 `SESSION_REVOKED_RISK` (UA family changed: +60 points; IP subnet changed: +40 points; total = 100 ≥ 80 threshold).

- **System Verifications:**
  - **Services:** `CalculateRiskScore` computes UA family change (+60) + IP subnet change (+40) = 100. Threshold = 80. `RevokeRefreshTokenFamily` called.
  - **Database:** All refresh tokens in family revoked. Session revoked.
  - **Redis:** All `jti`s from user's sessions added to blocklist.
  - **Events:** `auth.events` → `auth.session_revoked_risk`.

---

### TC-SEC-002: Cross-Tenant Policy Isolation (RLS Enforcement)

- **User Flow:** A user from Org A attempts to access Org B's policies via a forged request.
- **Preconditions:** Two orgs (A and B) each with one policy. User authenticated as Org A user.

- **Steps:**
  1. Obtain a valid JWT for Org A user.
  2. Submit `GET /v1/policies` with Org A JWT.
  3. Submit `GET /v1/policies/{org_b_policy_id}` with Org A JWT.

- **Expected Results:**
  - Step 2: Only Org A policies returned (no Org B data).
  - Step 3: HTTP 404 (Org B policy not visible; RLS returns 0 rows → ErrNotFound).

- **System Verifications:**
  - **Database:** PostgreSQL RLS: `SET LOCAL app.org_id = '<org_a_id>'` set by `withOrgContext`. `WHERE org_id = <org_b_id>` never matches because RLS policy enforces `org_id = app.org_id`. No cross-tenant data leaked.
  - **Services:** `ErrNotFound` mapped to HTTP 404, never 403 (prevents org ID enumeration).

---

### TC-SEC-003: Rate Limiting on Auth Endpoints

- **User Flow:** Client exceeds rate limit on login endpoint; receives 429.
- **Preconditions:** Rate limiter configured at 1 req/sec, burst 5.

- **Steps:**
  1. Submit 6 `POST /auth/login` requests within 1 second from the same IP.
  2. Observe the 6th response.

- **Expected Results:**
  - Requests 1–5: HTTP 200 or 401 (depending on credentials).
  - Request 6: HTTP 429 Too Many Requests.

- **System Verifications:**
  - **Services:** `NewRateLimiter`: in-memory `rate.Limiter` checked first; Redis token bucket updated. If either exceeds limit → 429 with `Retry-After` header.
  - **Redis:** Rate limit key for IP updated with each request.

- **Edge Cases:**
  - After 1 second, 1 new token is available → 7th request succeeds.

- **Failure Scenarios:**
  - Client sets `X-Real-IP: 1.2.3.4` header to spoof IP → rate limiter uses spoofed IP (known vulnerability, TC-SEC-003 also validates this behaviour should be fixed per code review finding #4).

---

### TC-SEC-004: Control-Plane Circuit Breaker Opens on Policy Service Failure

- **User Flow:** Policy service goes down; control-plane opens the circuit breaker; clients receive 503.
- **Preconditions:** Policy service healthy initially.

- **Steps:**
  1. Simulate 5 consecutive failed requests to the policy service (connection refused or 500).
  2. Submit `POST /v1/policy/evaluate` immediately after.
  3. Wait 30 seconds (circuit open duration) and submit again.

- **Expected Results:**
  - Step 2: HTTP 503 from control-plane (circuit open; no forwarding to policy service).
  - Step 3: HTTP 200 (half-open probe succeeds; circuit closes).

- **System Verifications:**
  - **Services:** `CircuitBreakerTransport` in control-plane: after 5 failures, `cb-policy` circuit opens. Subsequent calls return `gobreaker.ErrOpenState`. After `OpenDuration = 30s`, circuit enters half-open with max 3 probe requests.

---

## 18. Audit Trail

---

### TC-AUD-001: Audit Event Ingestion and Hash Chain Integrity

- **User Flow:** Events ingested via API are stored in MongoDB with a tamper-evident hash chain.
- **Preconditions:** Audit service running, consuming from Kafka.

- **Steps:**
  1. Trigger a series of policy mutations (create, update, delete).
  2. Verify each produces an audit event in MongoDB.
  3. Verify the hash chain is unbroken: each event's `prev_hash` = SHA-256 of the previous event.

- **Expected Results:**
  - Events present in MongoDB in order. Each `prev_hash` matches SHA-256 of the prior event's `hash`.
  - Chain head matches `hash_chain` record in the org's chain state document.

- **System Verifications:**
  - **Services:** `AuditConsumer` processes bulk Kafka messages. `ReserveSequence` allocates monotonic sequence numbers per org. `UpdateHashChainCAS` uses compare-and-swap to safely update the chain head. `BulkWrite` uses MongoDB `writeconcern.Majority`.
  - **MongoDB:** Audit events collection: each document has `seq_num`, `hash`, `prev_hash`. CAS on chain state document prevents concurrent chain corruption.

- **Failure Scenarios:**
  - CAS failure (concurrent consumer conflict) → retry with fresh `prev_hash`; eventually consistent.
  - Tampered event detected by verifying hash chain offline → `prev_hash` mismatch on tampered document.

---

## 19. Negative and Security Scenarios

---

### TC-NEG-001: Unauthenticated Access to Protected Endpoints

- **User Flow:** Attacker accesses protected resources without a token.

- **Steps:**
  1. Submit `GET /v1/policies` with no `Authorization` header.
  2. Submit `POST /v1/policy/evaluate` with no header.
  3. Submit `GET /v1/threats/alerts` with no header.

- **Expected Results:**
  - All requests: HTTP 401.

- **System Verifications:**
  - **Services:** `AuthJWTWithBlocklist` middleware rejects requests before handlers are invoked. No DB query performed.

---

### TC-NEG-002: Expired JWT Rejected

- **Steps:**
  1. Obtain a valid JWT, then wait for it to expire (or use a pre-crafted expired token).
  2. Submit `GET /v1/policies` with the expired token.

- **Expected Results:**
  - HTTP 401.

- **System Verifications:**
  - **Services:** JWT middleware validates `exp` claim before blocklist check. No Redis call made for expired tokens.

---

### TC-NEG-003: JWT with Invalid Signature Rejected

- **Steps:**
  1. Take a valid JWT and alter one character in the signature segment.
  2. Submit any protected request with the tampered token.

- **Expected Results:**
  - HTTP 401.

- **System Verifications:**
  - **Services:** `crypto.ValidateToken` tries all keys in the keyring; all fail for tampered signature.

---

### TC-NEG-004: Oversized Request Body Rejected

- **Steps:**
  1. Submit `POST /v1/policies` with a request body of 600 KB (exceeds 512 KB policy service limit).
  2. Submit `POST /auth/login` with a 2 MB body (exceeds 1 MB IAM limit).

- **Expected Results:**
  - Both requests: HTTP 413 Request Entity Too Large.

---

### TC-NEG-005: SSRF Attempt via Webhook Target

- **Steps:**
  1. Register a connector with a webhook target URL of `http://169.254.169.254/latest/meta-data/` (AWS metadata).
  2. Trigger a webhook delivery for that connector.

- **Expected Results:**
  - Delivery fails immediately with an SSRF-blocked error. No HTTP request reaches the metadata endpoint.

- **System Verifications:**
  - **Services:** `NewSafeHTTPClient` resolves the target hostname; checks if resolved IP is in a private or link-local range; rejects before TCP connection.
  - **Database:** `webhook_deliveries.status = 'failed'`, `last_error` contains SSRF-blocked message.

---

### TC-NEG-006: Cross-Tenant Alert Isolation

- **Steps:**
  1. Authenticate as Org A user.
  2. Attempt `GET /v1/threats/alerts/{alert_id_from_org_b}`.

- **Expected Results:**
  - HTTP 404 (alert not visible; `org_id` from JWT used in MongoDB query).

- **System Verifications:**
  - **Services:** Alert query always includes `org_id` filter derived from JWT. Org B's alert not returned.

---

## 20. Test Dependency Map

```
TC-AUTH-001 (login)
    └── TC-AUTH-003 (token refresh)
    └── TC-AUTH-004 (logout)
    └── TC-MFA-002 (MFA login)
    └── TC-OAUTH-001 (OAuth code flow)
    └── TC-SEC-001 (risk-based revocation)

TC-USR-001 (org + user creation)
    └── TC-SCIM-001 (SCIM provisioning)
    └── TC-SCIM-002 (SCIM deprovisioning)
    └── TC-MFA-001 (TOTP setup)
    └── TC-WA-001 (WebAuthn registration)

TC-POL-001 (create policy)
    └── TC-POL-002 (update policy)
    └── TC-POL-003 (delete policy)
    └── TC-POL-004 (assign policy)
    └── TC-EVAL-001 (evaluate - allow)
    └── TC-EVAL-002 (evaluate - deny)
    └── TC-EVAL-003 (stale cache)

TC-THR-001..005 (threat detection)
    └── TC-ALT-001..003 (alerting)

TC-CON-001 (connector registration)
    └── TC-OAUTH-001 (OAuth)
    └── TC-WHK-001 (webhook delivery)

TC-CMP-001 (report generation)
    └── TC-AUD-001 (audit chain, prerequisite data)
```

---

## 21. Execution Priority

| Priority | Test Cases | Rationale |
|---|---|---|
| P0 – Blocking | TC-AUTH-001, TC-AUTH-002, TC-AUTH-004, TC-SEC-002, TC-NEG-001 | Auth and tenant isolation are system prerequisites |
| P1 – Critical | TC-MFA-002, TC-OAUTH-001, TC-POL-001, TC-EVAL-001, TC-SCIM-001, TC-WHK-001, TC-CON-001 | Core business flows |
| P2 – High | TC-AUTH-003, TC-POL-002, TC-POL-003, TC-EVAL-003, TC-THR-001, TC-THR-002, TC-DLP-001, TC-SEC-001, TC-SEC-004 | Correctness and security depth |
| P3 – Standard | TC-MFA-001, TC-SAML-001, TC-WA-001, TC-WA-002, TC-SCIM-002, TC-SCIM-003, TC-ALT-001..003, TC-CMP-001, TC-AUD-001 | Full coverage |
| P4 – Exploratory | TC-NEG-002..006, TC-THR-003..005, TC-OAUTH-002, TC-WHK-002 | Edge case and resilience validation |