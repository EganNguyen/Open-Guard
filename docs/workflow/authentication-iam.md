# Authentication & IAM — Workflow

## Level 1: High-Level Architecture

```
                         ┌────────────────────────────────────────────────────────────────────────────┐
                         │                         CLIENT LAYER                                        │
                         │                                                                              │
                         │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                      │
                         │  │  Angular UI  │  │  SDK/Apps    │  │  SCIM Client │                      │
                         │  │  (Dashboard) │  │  (external)  │  │  (IdP)       │                      │
                         │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘                      │
                         │         │                  │                 │                              │
                         │         │    HTTPS (mTLS)   │   HTTPS (mTLS) │                              │
                         │         ▼                  ▼                 ▼                              │
                         │  ┌──────────────────────────────────────────────────────────────────────┐ │
                         │  │                    CONTROL PLANE (port 8081)                         │ │
                         │  │  Proxy: /v1/scim/v2/Users → IAM:8080                               │ │
                         │  │       /auth/*             → IAM:8080                               │ │
                         │  └──────────────────────────────┬───────────────────────────────────────┘ │
                         └─────────────────────────────────┼─────────────────────────────────────────┘
                                                           │
                                                           ▼
                         ┌────────────────────────────────────────────────────────────────────────────┐
                         │                         IAM SERVICE (port 8082)                           │
                         │                                                                              │
                         │  ┌────────────────────────────────────────────────────────────────────────┐ │
                         │  │                          MIDDLEWARE STACK                              │ │
                         │  │  CORS → Correlation → Metrics → Rate Limit → Auth JWT → Handler       │ │
                         │  └────────────────────────────────────────────────────────────────────────┘ │
                         │                                                                              │
                         │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐         │
                         │  │  Auth Handlers    │  │  MFA Handlers    │  │  SCIM Handlers   │         │
                         │  │  - Login          │  │  - TOTP Verify   │  │  - User CRUD     │         │
                         │  │  - Refresh        │  │  - WebAuthn      │  │  - Group Mgmt    │         │
                         │  │  - Logout         │  │  - Backup Codes  │  │  - JIT Provision │         │
                         │  │  - OAuth2/OIDC    │  │                  │  │                  │         │
                         │  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘         │
                         │           │                      │                     │                     │
                         │           └──────────┬───────────┴─────────────────────┘                     │
                         │                      ▼                                                       │
                         │  ┌──────────────────────────────────────────────────────────────────────┐  │
                         │  │                      SERVICE LAYER                                   │  │
                         │  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │  │
                         │  │  │ auth.go      │  │ mfa.go       │  │ users.go     │               │  │
                         │  │  │ (Login,      │  │ (TOTP/WebA-  │  │ (CRUD,       │               │  │
                         │  │  │  Tokens,     │  │  uthn/Backup │  │  Offboard)   │               │  │
                         │  │  │  Refresh,    │  │  Codes)      │  │              │               │  │
                         │  │  │  Logout)     │  │              │  │              │               │  │
                         │  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘               │  │
                         │  │         │                  │                 │                         │  │
                         │  │         │    AuthWorkerPool (bcrypt cost 12) │                         │  │
                         │  │         │    Bounded goroutine pool: 2×CPUs  │                         │  │
                         │  └──────────────────────────────────────────────────────────────────────┘  │
                         │                                                                              │
                         │  ┌──────────────────────────────────────────────────────────────────────┐  │
                         │  │                     REPOSITORY LAYER                                  │  │
                         │  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │  │
                         │  │  │ repo_user.go │  │ repo_session │  │ repo_token   │               │  │
                         │  │  │ - GetByEmail │  │  .go          │  │ .go          │               │  │
                         │  │  │ - Lock       │  │  - Create     │  │  - Claim     │               │  │
                         │  │  │ - Reset      │  │  - Revoke     │  │  - Family    │               │  │
                         │  │  └──────────────┘  └──────────────┘  └──────────────┘               │  │
                         │  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │  │
                         │  │  │ repo_mfa.go  │  │ repo_outbox  │  │ repo_org.go  │               │  │
                         │  │  │ - TOTP/WebA- │  │  .go          │  │ - Create     │               │  │
                         │  │  │  uthn/Backup │  │  - Insert     │  │              │               │  │
                         │  │  └──────────────┘  └──────────────┘  └──────────────┘               │  │
                         │  └──────────────────────────────────────────────────────────────────────┘  │
                         └────────────────┬───────────────────────────────────────────────────────────┘
                                          │
              ┌───────────────────────────┼───────────────────────────┐
              │                           │                           │
              ▼                           ▼                           ▼
     ┌────────────────┐       ┌────────────────┐          ┌──────────────────────┐
     │   PostgreSQL    │       │     Redis      │          │   Kafka              │
     │  - users        │       │  - blocklist:* │          │  - auth.events       │
     │  - sessions     │       │  - mfa_chal:*  │          │  - saga.orchestr.    │
     │  - refresh_tok  │       │  - totp:used:* │          │                      │
     │  - mfa_configs  │       │  - rate limit  │          │  → Threat Service    │
     │  - outbox       │       │  - saga:deadln │          │  → Audit Service     │
     │  - RLS enforced │       └────────────────┘          └──────────────────────┘
     └────────────────┘
```

---

## Level 2A: Login Flow (Password + MFA)

```
                           PASSWORD LOGIN SEQUENCE

  Client                    IAM Service                          PostgreSQL           Redis
    │                          │                                    │                   │
    │  POST /auth/login        │                                    │                   │
    │  {email, password}       │                                    │                   │
    │─────────────────────────>│                                    │                   │
    │                          │                                    │                   │
    │                          │  Rate limit check (1 req/s, burst 5)                  │
    │                          │──────────────────────────────────────────────────────>│
    │                          │<──────────────────────────────────────────────────────│
    │                          │                                    │                   │
    │                          │  GetUserByEmail                    │                   │
    │                          │  (SET ROLE openguard_login)        │                   │
    │                          │────────────────────────────────────>│                   │
    │                          │<──── user{hash, status, locked} ───│                   │
    │                          │                                    │                   │
    │                          │  Anti-enumeration:                 │                   │
    │                          │  If user not found → use dummy hash│                   │
    │                          │  (always cost-12, always ~350ms)   │                   │
    │                          │                                    │                   │
    │                          │  AuthWorkerPool.Compare(pw, hash)  │                   │
    │                          │  (goroutine pool, bcrypt cost 12)  │                   │
    │                          │        ... wait ~350ms ...         │                   │
    │                          │                                    │                   │
    │                          │  ┌─ On MISMATCH:                   │                   │
    │                          │  │  Increment failed_login_count    │                   │
    │                          │  │────────────────────────────────>│                   │
    │                          │  │  If count >= 10:                 │                   │
    │                          │  │    LockAccount (escalating)      │                   │
    │                          │  │    → 15min / 30min / 1hr / ...  │                   │
    │                          │  │────────────────────────────────>│                   │
    │                          │  │<────────────────────────────────│                   │
    │                          │  │  Return 401 INVALID_CREDENTIALS  │                   │
    │                          │  │  (generic, no state leakage)     │                   │
    │                          │                                    │                   │
    │                          │  ┌─ On MATCH:                      │                   │
    │                          │  │  Reset failed_login_count=0      │                   │
    │                          │  │────────────────────────────────>│                   │
    │                          │  │  Check lockout:                  │                   │
    │                          │  │  If locked_until > now → 401     │                   │
    │                          │  │                                    │                   │
    │                          │  │  Check MFA configs               │                   │
    │                          │  │────────────────────────────────>│                   │
    │                          │  │<── mfa_configs ──────────────────│                   │
    │                          │  │                                    │                   │
    │                          │  │  ┌─ MFA ENABLED:                  │                   │
    │                          │  │  │  Generate challenge token      │                   │
    │                          │  │  │  → Redis SET mfa_challenge:{uuid}                  │
    │                          │  │  │─────────────────────────────────────────>│          │
    │                          │  │  │  202 {mfa_required: true,      │                   │
    │                          │  │  │        mfa_challenge: "..."}   │                   │
    │                          │  │  │<──────────────────────────────│                   │
    │                          │  │                                    │                   │
    │                          │  │  ┌─ NO MFA:                      │                   │
    │                          │  │  │  IssueTokens → JWT + refresh  │                   │
    │  <── 200 {token, user} ──│──│──│──│──│──│──│──│──│──│           │                   │
    │                          │  │  │                              │                   │
    │  (Sets HttpOnly cookie   │  │  │                              │                   │
    │   openguard_session)     │  │  │                              │                   │
```

### Token Issuance Detail (IssueTokens)

```
  IssueTokens(userID, orgID, userAgent, ip)
    │
    ├── 1. Generate JTI (UUID v4)
    ├── 2. Sign JWT access token (HS256, active key from keyring → see §Level 2E)
    │      Claims: { org_id, user_id, jti, iat, exp(1hr) }
    │
    ├── 3. Create DB session (sessions table)
    │      INSERT (org_id, user_id, jti, user_agent, ip_address, expires_at)
    │
    ├── 4. Generate refresh token (64-char random)
    │      SHA-256 hash → token_hash
    │      Family ID → UUID (for rotation lineage tracking)
    │      INSERT INTO refresh_tokens (org_id, user_id, token_hash, family_id, expires_at)
    │      TTL: 7 days
    │
    └── 5. Return: { access_token, refresh_token, expires_in, user }
```

---

## Level 2B: MFA Verification Flow

```
  Client                    IAM Service                          PostgreSQL           Redis
    │                          │                                    │                   │
    │  POST /auth/mfa/verify   │                                    │                   │
    │  {mfa_challenge, code}   │                                    │                   │
    │─────────────────────────>│                                    │                   │
    │                          │                                    │                   │
    │                          │  GETDEL mfa_challenge:{uuid}                          │
    │                          │──────────────────────────────────────────────────────>│
    │                          │<──────────────────── userID ──────────────────────────│
    │                          │                                    │                   │
    │                          │  TOTP nonce replay check:                              │
    │                          │  SETNX totp:used:{userID}:{code}  (TTL 90s)           │
    │                          │──────────────────────────────────────────────────────>│
    │                          │<──────────────────────────────────────────────────────│
    │                          │                                    │                   │
    │                          │  Decrypt TOTP secret from DB       │                   │
    │                          │────────────────────────────────────>│                   │
    │                          │<──────── encrypted_secret ─────────│                   │
    │                          │                                    │                   │
    │                          │  Validate: totp.Validate(code,     │                   │
    │                          │            decrypt(secret))        │                   │
    │                          │                                    │                   │
    │                          │  ┌─ Valid → IssueTokens (same as   │                   │
    │                          │  │           password login)       │                   │
    │  <── 200 {token, user} ──│──│──│──│──│──│──│──│──│──│          │                   │
    │                          │  │                                  │                   │
    │                          │  ┌─ Invalid → 401 MFA_FAILED       │                   │
    │  <── 401 ────────────────│──│                                  │                   │
```

### TOTP Enrollment Flow

```
  POST /auth/mfa/totp/setup
    │
    ├── 1. Generate TOTP secret (crypto/rand)
    ├── 2. Encrypt with AES keyring (IAM_AES_KEYS)
    ├── 3. Store in mfa_configs (org_id, user_id, type='totp', secret_encrypted)
    ├── 4. Generate provisioning URI (otpauth://totp/...)
    └── 5. Return URI + QR code data

  POST /auth/mfa/totp/enable  {code}
    ├── 1. Verify code against stored secret
    ├── 2. Mark MFA as verified (mfa_enabled = true)
    └── 3. Generate backup codes (10 codes, HMAC-SHA256 hashed)
```

---

## Level 2C: Token Refresh Flow

```
  Client                    IAM Service                          PostgreSQL           Redis
    │                          │                                    │                   │
    │  POST /auth/refresh      │                                    │                   │
    │  {refresh_token}         │                                    │                   │
    │─────────────────────────>│                                    │                   │
    │                          │                                    │                   │
    │                          │  SHA-256 hash incoming token       │                   │
    │                          │                                    │                   │
    │                          │  ClaimRefreshToken (atomic UPDATE)                     │
    │                          │  UPDATE refresh_tokens SET revoked │                   │
    │                          │  WHERE token_hash=$1 AND NOT revoked                   │
    │                          │  AND expires_at > NOW()            │                   │
    │                          │  RETURNING id,family_id,...        │                   │
    │                          │────────────────────────────────────>│                   │
    │                          │                                    │                   │
    │                          │  ┌─ SUCCESS (token claimed):       │                   │
    │                          │  │  Heuristic risk scoring:        │                   │
    │                          │  │    UA family change:     +60    │                   │
    │                          │  │    Subnet (/16) change:  +40    │                   │
    │                          │  │    IP host change:       +15    │                   │
    │                          │  │    UA version change:    +20    │                   │
    │                          │  │  If score >= 80:                │                   │
    │                          │  │    → Revoke entire family       │                   │
    │                          │  │    → Log security event         │                   │
    │                          │  │    → 401 SESSION_COMPROMISED    │                   │
    │                          │  │                                    │                   │
    │                          │  │  Score < 80: IssueTokens         │                   │
    │  <── 200 {token, user} ──│──│  (preserves family_id lineage)  │                   │
    │                          │  │                                    │                   │
    │                          │  ┌─ FAIL (token already used):     │                   │
    │                          │  │  → Revoke ALL tokens in family  │                   │
    │                          │  │  → 401 SESSION_COMPROMISED      │                   │
    │  <── 401 ────────────────│──│                                  │                   │
```

---

## Level 2D: Logout Flow

```
  Client                    IAM Service                                   Redis
    │                          │                                           │
    │  POST /auth/logout       │                                           │
    │  (Bearer token or cookie)│                                           │
    │─────────────────────────>│                                           │
    │                          │                                           │
    │                          │  Extract JTI + exp from JWT claims        │
    │                          │  (already verified by auth middleware)    │
    │                          │                                           │
    │                          │  SET blocklist:{jti} = "revoked"          │
    │                          │  (TTL = remaining token lifetime)         │
    │                          │──────────────────────────────────────────>│
    │                          │                                           │
    │                          │  Clear openguard_session cookie           │
    │                          │  (MaxAge=-1)                              │
    │                          │                                           │
    │  <── 200 OK ─────────────│                                           │
```

## Level 2E: JWT Key Rotation Flow

The IAM service uses a multi-key keyring (`kid`-based) for zero-downtime JWT signing key rotation. Multiple keys coexist during the rotation window: one `active` key signs new tokens, while `verify_only` keys still validate existing tokens.

```
                          ZERO-DOWNTIME KEY ROTATION

  Operator                  IAM Services              Secret Store
     │                         │                        │
     │  PHASE 1 — Introduce                              │
     │  Generate new key                                 │
     │  (unique kid)          │                        │
     │<───────────────────────── kid, secret             │
     │                         │                        │
     │  Update IAM_JWT_KEYS:   │                        │
     │  [{kid:"k-2025",       │                        │
     │    status:"verify_only"},│                        │
     │   {kid:"k-2026",       │                        │
     │    status:"active"}]   │                        │
     │─────────────────────────────────────────────────>│
     │                         │                        │
     │  Rolling deploy         │                        │
     │─────────────────────────> Load keyring           │
     │                         │───────────────────────>│
     │                         │<── keyring JSON ───────│
     │                         │                        │
     │                         │  Sign  → picks "k-2026"│
     │                         │  Verify → matches by   │
     │                         │    kid (old or new)    │
     │                         │                        │
     │  PHASE 2 — Finalize                              │
     │  Wait ≥ max JWT TTL    │                        │
     │  (default 1 hour)       │                        │
     │                         │                        │
     │  Remove old key from    │                        │
     │  IAM_JWT_KEYS           │                        │
     │─────────────────────────────────────────────────>│
     │                         │                        │
     │  Rolling deploy         │                        │
     │─────────────────────────> Reload keyring         │
     │                         │ (only active key)     │
     │                         │                        │
     │                         │  Old tokens now        │
     │                         │  rejected (kid gone)   │
```

### Key Lifecycle States

```
                 ┌──────────┐
                 │  ACTIVE   │  Signs new tokens + verifies all
                 │           │  Sign() selects first active key
                 └────┬──────┘
                      │
                 ┌────▼──────┐
                 │VERIFY_ONLY│  Only verifies existing tokens
                 │           │  Covers the rotation overlap
                 └────┬──────┘
                      │
                 ┌────▼──────┐
                 │  REMOVED   │  Fully rotated out
                 │           │  Tokens with this kid → 401
                 └───────────┘
```

### Keyring Routing

| Operation | Logic | Key Status Matched |
|-----------|-------|-------------------|
| `Sign(claims, keyring)` | Iterates keyring, returns first key where `Status == "active"` | `active` only |
| `Verify(token, keyring, claims)` | Extracts `kid` from JWT header, finds key by `Kid` (any status) | `active` or `verify_only` |

### Impact on Token Flows

- **Token Issuance:** New JWTs signed with the `active` key. The `kid` header is embedded in the token.
- **Token Refresh:** Refresh tokens are opaque (64-char random, SHA-256 hashed) — not JWT-signed. Rotation does not affect them. The re-issued access token uses the new `active` key.
- **Token Verification:** All services verify by extracting `kid` from the JWT header and looking up the matching key in the keyring. Both `active` and `verify_only` keys are accepted.

### Operational Procedure

| Phase | Step | Action | Impact |
|-------|------|--------|--------|
| 1 | 1 | Generate new key with unique `kid` | None |
| 1 | 2 | Add new key as `active`, demote old to `verify_only` in `IAM_JWT_KEYS` | None (not loaded) |
| 1 | 3 | Rolling deploy IAM + all services loading the keyring | Brief restart |
| 1 | 4 | New JWTs use new key; old tokens still verify | Zero-downtime |
| 2 | 5 | Wait ≥ `IAM_JWT_EXPIRY_SECONDS` (default 3600s) | All old-key tokens expired |
| 2 | 6 | Remove old key from `IAM_JWT_KEYS` | None (not loaded) |
| 2 | 7 | Rolling deploy all services | Old tokens rejected (401) |

### Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **No active key in keyring** | IAM | `Sign()` returns `ErrKeyNotFound` → token issuance fails (500) |
| **kid in JWT not found in keyring** | All | `Verify()` returns `ErrKeyNotFound` → request rejected (401) |
| **Keyring JSON malformed** | IAM | `LoadKeyring()` fails → service fails to start |
| **Old key removed before token expiry** | All | Valid tokens rejected (401); user must re-authenticate |
| **Duplicate kid in keyring** | IAM | `Verify()` matches first occurrence — startup validation recommended |

---

## Level 2F: JWT Revocation Flow

JWTs are revoked by writing the JTI to a Redis blocklist key. Every service checks this blocklist on each request before accepting a token. Revocation is not cryptographic — the token's signature remains valid — but the blocklist entry prevents its use for the token's remaining lifetime.

> **Immediate revocation via per-request blocklist check.** The blocklist write (SET) and the next API request's blocklist check (EXISTS) execute in the same Redis round-trip window — typically sub-millisecond. There is no polling, TTL sweep, or background job. **Trade-off:** every API request across all services pays one Redis EXISTS call (~0.5–2ms) to verify the token is not revoked. At Open-Guard's scale (1000s of requests/s), this is acceptable because blocklist keys are TTL-bound (max 1h) and Redis EXISTS is O(1) with negligible memory overhead (~40 bytes per active revocation).

```
                         JWT REVOCATION — TRIGGERS

  Client              IAM Service                        Redis              Downstream Svc
    │                     │                               │                     │
    │  ── Revocation Triggers ──                          │                     │
    │                     │                               │                     │
    │  POST /auth/logout  │                               │                     │
    │────────────────────>│                               │                     │
    │                     │  Logout():                     │                     │
    │                     │  SET blocklist:{jti}           │                     │
    │                     │  (TTL = remaining lifetime)    │                     │
    │                     │──────────────────────────────>│                     │
    │                     │                               │                     │
    │  Admin deletes user │                               │                     │
    │────────────────────>│  DeleteUser():                 │                     │
    │                     │  pipeline SET blocklist:{jti}  │                     │
    │                     │  for all active JTIs           │                     │
    │                     │──────────────────────────────>│                     │
    │                     │                               │                     │
    │  Refresh token reuse│                               │                     │
    │  (compromise)       │  RevokeFamily():               │                     │
    │                     │  pipeline SET blocklist:{jti}  │                     │
    │                     │  for all JTIs in family        │                     │
    │                     │──────────────────────────────>│                     │
    │                     │                               │                     │
    │  ── Verification ── │                               │                     │
    │                     │                               │                     │
    │  API request        │                               │                     │
    │  (Bearer token)     │                               │                     │
    │─────────────────────────────────────────────────────────────────────────>│
    │                     │                               │                     │
    │                     │          Auth middleware:       │                     │
    │                     │          EXISTS blocklist:{jti}│                     │
    │                     │──────────────────────────────>│                     │
    │                     │          <── 0 or 1 ──────────│                     │
    │                     │                               │                     │
    │                     │  ┌─ Found → 401 UNAUTHORIZED  │                     │
    │                     │  └─ Not found → allow request │                     │
```

### Blocklist Mechanics

| Property | Detail |
|----------|--------|
| **Key format** | `blocklist:{jti}` where `jti` is UUID v4 from JWT claims |
| **Value** | `"revoked"` (opaque, existence check only) |
| **TTL** | `time.Until(expiresAt)` — auto-expires when the token would have expired |
| **Cleanup** | None needed — Redis TTL handles expiry |
| **Memory** | ~40 bytes per entry × active revocations within 1h window |

### Verification Behavior by Service

| Service | Middleware | Fail Mode | Behavior on Redis Down |
|---------|-----------|-----------|----------------------|
| **IAM** | `iam/middleware/auth.go` | **Fail-closed** | Returns 401 — cannot verify revocation status |
| **Policy** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |
| **Threat** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |
| **Audit** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |
| **Alerting** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |
| **Compliance** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |
| **DLP** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |
| **Connector Registry** | `shared/middleware/jwt_auth.go` | **Fail-open** | Logs warning, allows request through |

### Revocation Triggers

| Trigger | Scope | Mechanism | Source |
|---------|-------|-----------|--------|
| **User logout** | Single JTI | `Logout()` → `SET blocklist:{jti}` | `auth.go:149` |
| **Password change** | All user sessions | Pipeline `SET blocklist:{jti}` for all active JTIs | `users.go` |
| **User deletion** | All user sessions | Pipeline `SET blocklist:{jti}` + DB session revoke | `users.go:115` |
| **Org offboarding** | All users in org | Pipeline `SET blocklist:{jti}` per user | `users.go:250` |
| **Refresh token reuse** | Entire token family | Pipeline `SET blocklist:{jti}` for all JTIs in family | Token refresh handler |
| **High-risk refresh** | Single token family | Same mechanism, triggered by risk score ≥80 | Token refresh handler |
| **Token expiry** | Single JTI | Passive — blocklist entry auto-expires or JWT `exp` claim | N/A (built-in) |

### Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Redis down (IAM middleware)** | IAM | Fail-closed — all requests return 401 |
| **Redis down (shared middleware)** | All other services | Fail-open — revoked tokens temporarily accepted |
| **Circuit breaker open (Redis)** | All | Shared middleware skips blocklist check with warning |
| **Blocklist key TTL too short** | IAM | Token usable again before natural expiry |
| **JTI collision** | IAM | Extremely unlikely (UUID v4), but would block a valid token |

---

## Level 3: State Transitions

### Account Lockout State

```
                         ┌──────────┐
                         │  ACTIVE  │  (failed_login_count = 0)
                         └────┬─────┘
                              │
                      ┌───────┴────────┐
                      │  Failed login  │  (count increments 1..9)
                      │  (count: N)    │
                      └───────┬────────┘
                              │
                      ┌───────┴────────┐
                      │ count >= 10    │
                      ▼                ▼
               ┌────────────┐   ┌──────────┐
               │  LOCKED    │   │  ACTIVE  │
               │  (15 min)  │   │ (reset=0)│
               └──────┬─────┘   └──────────┘
                      │
           ┌──────────┴──────────┐
           │                     │
      ┌────▼────┐          ┌────▼────┐
      │  Wait   │          │  More   │
      │  expires│          │ fails   │
      └────┬────┘          └────┬────┘
           │                    │
      ┌────▼────┐         ┌────▼───────┐
      │  ACTIVE  │         │  LOCKED    │
      │ (retry)  │         │ (escalated)│
      └──────────┘         │ 30min→1hr  │
                           │ →...→24hr  │
                           └────────────┘

  Lockout formula: 15 min * 2^(failCount/10 - 1), capped at 24 hours
```

### JWT Session State

```
  ┌────────┐     ┌──────────┐     ┌──────────┐
  │ ISSUED │────>│  ACTIVE  │────>│ EXPIRED  │
  │ (JWT   │     │ (in use) │     │ (1hr TTL)│
  │ signed)│     └────┬─────┘     └──────────┘
  └────────┘          │
                      │
                 ┌────▼──────┐
                 │ REVOKED   │
                 │ (logout)  │
                 │ Redis blk │
                 └───────────┘
```

### Refresh Token State

```
  ┌──────────┐     ┌──────────┐     ┌───────────┐
  │  ISSUED  │────>│  ACTIVE  │────>│  EXPIRED  │
  │ (64-char │     │ (single  │     │ (7 day    │
  │  random) │     │  use)    │     │  TTL)     │
  └──────────┘     └────┬─────┘     └───────────┘
                        │
                   ┌────▼─────┐
                   │  CLAIMED │
                   │ (atomic  │
                   │  UPDATE) │
                   └────┬─────┘
                        │
              ┌─────────┴──────────┐
              │                    │
              ▼                    ▼
        ┌──────────┐        ┌──────────────┐
        │ RE-ISSUED│        │ FAMILY       │
        │ (new JWT │        │ REVOKED      │
        │ + token) │        │ (reuse/risk  │
        └──────────┘        │  detected)   │
                            └──────────────┘
```

---

## Consumer Group / Event Mapping

| Event Type | Topic | Produced By | Consumed By |
|------------|-------|-------------|-------------|
| `auth.login.success` | `auth.events` | IAM (outbox) | Threat, Audit |
| `auth.login.failed` | `auth.events` | IAM (outbox) | Threat, Audit |
| `auth.failed` | `auth.events` | IAM (outbox) | Threat, Audit |
| `password.changed` | `auth.events` | IAM (outbox) | Threat (takeover) |
| `user.created` | `saga.orchestration` | IAM (outbox) | IAM (saga), Audit |
| `user.deleted` | `saga.orchestration` | IAM (outbox) | IAM (saga), Audit |
| `user.updated` | `saga.orchestration` | IAM (outbox) | IAM (saga), Audit |
| `user.provisioning.failed` | `saga.orchestration` | IAM (saga) | IAM (consumer) |
| `org.iam.offboarded` | `saga.orchestration` | IAM (outbox) | IAM (saga), Audit |

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Client (UI/SDK)** | Collect credentials, store tokens securely, send HttpOnly cookie or Bearer header |
| **Control Plane** | Routes `/auth/*` and `/v1/scim/*` to IAM service |
| **IAM Service** | Login, JWT issuance, MFA, token refresh, logout, user CRUD, SCIM, OAuth2/OIDC, SAML |
| **AuthWorkerPool** | Bounded goroutine pool (2×CPUs) for bcrypt cost-12 password hashing |
| **PostgreSQL** | Users, sessions, refresh tokens, MFA configs, outbox — all with RLS |
| **Redis** | JWT blocklist, MFA challenges, TOTP nonces, OAuth2 codes, WebAuthn sessions, saga deadlines, rate limiting |
| **Kafka** | Auth events → Threat/Audit; saga events → self-consumption |
| **Threat Service** | Consumes auth events for brute force, impossible travel, off-hours, account takeover detection |
| **Audit Service** | Consumes auth events for immutable audit trail |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Bcrypt worker pool full** | IAM | Request waits on channel until slot available; no timeout = potential hang |
| **Redis blocklist down** | IAM | Auth middleware returns 401 (fail-closed: cannot verify revocation) |
| **Redis MFA challenge down** | IAM | MFA login fails with 500; user cannot complete MFA step |
| **PostgreSQL down** | IAM | All auth endpoints fail; service unable to start without DB |
| **Refresh token reuse** | IAM | Entire family revoked, user forced to re-login (compromise containment) |
| **Kafka broker down (outbox)** | IAM | Outbox relay retries; auth events delayed but login still works |
| **Rate limit exceeded** | IAM | 429 Too Many Requests; protects bcrypt from DoS |
| **Anti-enumeration race** | IAM | Dummy hash comparison ensures ~350ms response regardless of user existence |
| **Saga deadline exceeded** | IAM | 40s ZSET deadline → publishes provisioning.failed → user set to provisioning_failed |
| **Circuit breaker open (Redis)** | IAM | Blocklist check skipped with warning; logout may not block JWT immediately |
| **TOTP replay (duplicate nonce)** | IAM | 90s Redis TOTP nonce prevents code reuse within window |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_auth_logins_total` | Counter | `status`, `mfa` | IAM Service |
| `openguard_auth_bcrypt_duration_seconds` | Histogram | (none) | IAM Service |
| `openguard_auth_pool_queue_depth` | Gauge | (none) | IAM Service |
| `openguard_auth_refresh_total` | Counter | `status` | IAM Service |
| `openguard_mfa_verifications_total` | Counter | `type`, `status` | IAM Service |
| `openguard_account_lockouts_total` | Counter | (none) | IAM Service |
| `openguard_auth_keyring_keys_total` | Gauge | `status` | IAM Service |

### Key Traces (Jaeger)

- `iam.login` — from HTTP receive to token response (includes bcrypt + DB)
- `iam.mfa.verify` — from MFA code receive to token response
- `iam.refresh` — token claim → risk scoring → re-issuance
- `iam.user.create` — user creation with saga deadline

### Security Events (Audit Log)

| Event | When | Payload |
|-------|------|---------|
| `auth.login.success` | Successful password login | user_id, org_id, IP, UA, MFA status |
| `auth.login.failed` | Failed password attempt | user_id, org_id, IP, attempt_count |
| `auth.account_locked` | Lockout threshold reached | user_id, org_id, duration |
| `auth.token.refreshed` | Token refresh succeeded | user_id, family_id, risk_score |
| `auth.session_compromised` | Refresh token reuse detected | user_id, family_id, old_UA, new_UA |
| `auth.logout` | Explicit logout | user_id, jti |
| `user.created` | New user registered | user_id, org_id, source (SCIM/manual) |
| `user.deleted` | User removed | user_id, org_id |
