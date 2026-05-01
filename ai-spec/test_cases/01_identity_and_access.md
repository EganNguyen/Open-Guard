# Identity & Access Management (IAM) Flows

## 1. Overview
This module covers all authentication and identity lifecycle journeys in OpenGuard, including Multi-Factor Authentication (MFA), SSO (SAML/OAuth), and automated provisioning (SCIM).

## 2. Authentication Flows

### TC-AUTH-001: Successful Password Login `[E2E] [INTEGRATION] [UNIT]`
- **User Flow:** User submits email and password via the login form; receives JWT tokens.
- **Preconditions:** Active org and active user exist; User has no MFA; Redis reachable.
- **Steps:** 
  1. `POST /auth/login` with credentials.
- **Expected Results:** HTTP 200 with `access_token` and `refresh_token`.
- **System Verifications:** 
  - `[UNIT]`: Bcrypt comparison logic.
  - `[INTEGRATION]`: Session creation in Postgres; Token storage in Redis.
  - `[INTEGRATION]`: `auth.login` Kafka event published via Outbox.

### TC-AUTH-002: Account Lockout After Repeated Failures `[E2E] [INTEGRATION]`
- **User Flow:** Attacker submits wrong password multiple times; account is locked.
- **Preconditions:** Active user with 4 failed attempts.
- **Steps:** Submit 5th wrong attempt, then 1 correct attempt.
- **Expected Results:** Both return HTTP 401.
- **System Verifications:** 
  - `[INTEGRATION]`: `users.locked_until` set in DB.
  - `[INTEGRATION]`: `auth.account_locked` event sent to Kafka.

### TC-AUTH-003: JWT Refresh Token Rotation
- **User Flow:** Client exchanges refresh token; old token invalidated.
- **Steps:** 1. `POST /auth/refresh` (Success). 2. Repeat same token (Failure).
- **Expected Results:** 1. 200 OK. 2. 401 Unauthorized (Reuse detection).
- **System Verifications:** Refresh token family revocation (RTR pattern).

### TC-AUTH-004: Logout Blocklists Access Token
- **User Flow:** User logs out; access token invalidated immediately.
- **Steps:** 1. `POST /auth/logout`. 2. `GET /auth/me` with same token.
- **Expected Results:** 2. 401 Unauthorized.
- **System Verifications:** `jti` added to Redis blocklist.

## 3. Multi-Factor Authentication (MFA)

### TC-MFA-001: TOTP Setup and Enable
- **User Flow:** User registers TOTP via QR code.
- **Steps:** 1. `GET /mgmt/users/mfa/totp/setup`. 2. `POST /mgmt/users/mfa/totp/enable`.
- **Expected Results:** MFA enabled for user.
- **System Verifications:** AES-256-GCM encrypted secret in DB.

### TC-MFA-002: MFA-Required Login Flow
- **User Flow:** User logs in; must verify TOTP.
- **Steps:** 1. `POST /auth/login`. 2. `POST /auth/mfa/verify` with challenge.
- **Expected Results:** 1. 403 Forbidden (`mfa_required`). 2. 200 OK with tokens.

## 4. OAuth 2.0 / PKCE

### TC-OAUTH-001: Authorization Code Flow with PKCE
- **User Flow:** Connector initiates OAuth login; user authenticates; connector receives tokens.
- **Steps:** Authorize -> Dashboard Login -> Auth Code -> Token Exchange (with code_verifier).
- **Expected Results:** Tokens issued only after successful PKCE verification.

## 5. SAML SSO

### TC-SAML-001: SAML Identity Provider Login
- **User Flow:** Enterprise user authenticates via external SAML IdP.
- **System Verifications:** XML signature validation, `NameID` mapping, session creation.

## 6. WebAuthn / Passkey

### TC-WA-001: WebAuthn Credential Registration
- **User Flow:** User registers a hardware security key.
- **System Verifications:** Challenge/Response verification, `webauthn_credentials` persistence.

### TC-WA-002: WebAuthn Login
- **User Flow:** User logs in using a passkey.
- **System Verifications:** Signature verification, `sign_count` clone detection.

## 7. SCIM 2.0 Provisioning

### TC-SCIM-001: SCIM User Provisioning (Create)
- **User Flow:** IdP provisions a user via SCIM.
- **System Verifications:** Transactional outbox event -> Saga orchestration -> Status transition to `active`.

### TC-SCIM-002: SCIM User Deprovisioning (Delete)
- **User Flow:** Admin deletes user in IdP.
- **System Verifications:** User status set to `deprovisioned`; all sessions revoked.

## 8. User Management

### TC-USR-001: Org and User Creation
- **User Flow:** Super-admin creates new org and initial admin.
- **System Verifications:** RLS session scoping, Bcrypt hashing, Saga initialization.
