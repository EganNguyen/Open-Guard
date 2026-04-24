# §10 — Phase 2: Foundation & Authentication

**Goal:** Running skeleton with enterprise-grade auth and working control plane. JWT multi-key rotation, RLS enforced, Outbox in place, circuit breakers configured, connector registration operational.

---

## 10.1 Prerequisites (produce before any service code)

1. `infra/docker/docker-compose.yml`
2. `scripts/gen-mtls-certs.sh` — generates CA and per-service certs
3. `scripts/create-topics.sh` — idempotent topic creation from `infra/kafka/topics.json`
4. `Makefile` with targets: `dev`, `test`, `lint`, `build`, `migrate`, `seed`, `load-test`, `certs`
5. `.env.example` as defined in §5.1
6. `.github/workflows/ci.yml` — must be operational from the first commit

---

## 10.2 Migration Strategy

Use `golang-migrate/migrate` with these invariants:

- Every `.up.sql` must have a corresponding `.down.sql`. Tested in CI by running `migrate up` then `migrate down`.
- Migrations are **additive only** in production: add nullable columns, add indexes, add tables. Never drop or rename in the same migration as adding.
- Every migration creating a table with an `org_id` column must include RLS setup.
- Migrations run at service startup with a **distributed Redis lock** (SET NX + heartbeat goroutine extending TTL every 10s).
- **No DML in migrations.** Data backfills run separately after migration completes.

**Rollback procedure:**
1. Pause all deployments via CI/CD feature flag.
2. If idempotent, fix and re-run. Otherwise run `migrate down N` with SRE supervision.
3. A `.down.sql` involving `DROP TABLE`/`DROP COLUMN` MUST include a `CREATE TABLE ... IF NOT EXISTS` guard.

---

## 10.3 IAM Service

### 10.3.1 Database Schema

**001_create_orgs.up.sql** — `orgs` table with RLS (self-read policy: `id = NULLIF(current_setting(...))::UUID`).

**002_create_users.up.sql**
```sql
CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email               TEXT NOT NULL,
    display_name        TEXT NOT NULL DEFAULT '',
    password_hash       TEXT,            -- bcrypt, cost 12
    status              TEXT NOT NULL DEFAULT 'active',
    mfa_enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method          TEXT,
    scim_external_id    TEXT,
    provisioning_status TEXT NOT NULL DEFAULT 'complete',
    tier_isolation      TEXT NOT NULL DEFAULT 'shared',
    version             INT NOT NULL DEFAULT 1,  -- Atomic increment for SCIM ETags
    last_login_at       TIMESTAMPTZ,
    last_login_ip       INET,
    failed_login_count  INT NOT NULL DEFAULT 0,
    locked_until        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    UNIQUE (org_id, email)
);
-- Indexes on org_id, email, scim_external_id; RLS policy; GRANT to openguard_app
```

**003_create_sessions.up.sql** — sessions with ip_address, user_agent, geo fields, revoked flag.

**004_create_api_tokens.up.sql** — api_tokens with token_hash, prefix, scopes, expires_at, revoked.

**005_create_mfa_configs.up.sql** — mfa_configs (encrypted TOTP secrets, backup_code_hashes via HMAC) and webauthn_credentials (credential_id, public_key, sign_count).

**006_create_outbox.up.sql** — standard outbox table (§7.1).

### 10.3.2 MFA Backup Code Storage

```go
// Backup code generation:
//   1. Generate 8 random 8-character codes
//   2. For each: HMAC-SHA256(code, IAM_MFA_BACKUP_CODE_HMAC_SECRET)
//   3. Store hex-encoded HMACs in mfa_configs.backup_code_hashes
//
// Verification: compute HMAC, check array (O(1)), remove on use (single-use)
```

### 10.3.3 MFA Encryption (AES-256-GCM Multi-Key)

```go
// shared/crypto/aes.go
type EncryptionKey struct {
    Kid    string `json:"kid"`
    Key    string `json:"key"`    // base64-encoded 32-byte key
    Status string `json:"status"` // "active" | "verify_only"
}

// Encrypt uses the first active key. Output: "<kid>:<base64(nonce+ciphertext)>"
// Decrypt parses kid from prefix, finds matching key (active OR verify_only), decrypts.
```

### 10.3.4 JWT Multi-Key Keyring

```go
// shared/crypto/jwt.go
type JWTKey struct {
    Kid       string `json:"kid"`
    Secret    string `json:"secret"`
    Algorithm string `json:"algorithm"` // "HS256" | "RS256"
    Status    string `json:"status"`    // "active" | "verify_only"
}

// Sign: uses first key with status="active". Includes kid in JWT header.
// Verify: extracts kid from header, finds matching key (active or verify_only),
//         verifies signature and expiry. Returns ErrTokenExpired or ErrTokenInvalid.
```

### 10.3.5 Risk-Based Session Protection

Applied at `/auth/refresh`. Scores are additive:

| Factor | Score |
|---|---|
| User agent family change | 60 |
| IP subnet change (/16) | 40 |
| IP host change (same /16) | 15 |
| UA version change (same family) | 20 |

- Score ≥ 80: Revoke session. Return `401 SESSION_REVOKED_RISK`.
- Score < 80: Accept. Rotate refresh token.

**Strict single-use refresh tokens:** If an already-used refresh token is presented, immediately revoke the entire session (token reuse = compromise indicator). Return `401 SESSION_COMPROMISED`. No grace window.

**Session Fixation Protection:** `POST /auth/mfa/challenge` and `/auth/webauthn/login/finish` MUST issue a new session ID and invalidate the pre-MFA session ID upon successful verification.

### 10.3.6 WebAuthn Implementation

Use `github.com/go-webauthn/webauthn`. WebAuthn challenge state stored in Redis (TTL: 5 minutes), keyed by `webauthn:challenge:{user_id}:{session_id}`. The `session_id` MUST be a server-generated opaque random token (min 128 bits) in an `HttpOnly; Secure; SameSite=Strict` cookie — NOT accessible to client-side JavaScript.

### 10.3.7 SCIM v2 Implementation

SCIM endpoints exposed at `/v1/scim/v2/*`, proxied to IAM via mTLS.

- **`ListResponse` envelope:** `schemas`, `totalResults`, `startIndex`, `itemsPerPage`, `Resources`.
- **`PATCH`:** Uses JSON Patch (RFC 6902) operations, not merge-patch.
- **`ETag` support:** Every SCIM resource response includes `ETag: "{version}"`. Conditional updates with `If-Match` enforced.
- **1-indexed offset pagination** (`startIndex` + `count` per RFC 7644) — overrides cursor pagination standard for SCIM endpoints.
- **Error format:** RFC 7644 §3.12 only (not `APIError` format).

### 10.3.8 IAM HTTP Endpoints

**OIDC/SAML IdP (public):**

| Method | Path | Description |
|---|---|---|
| `GET` | `/oauth/authorize` | OIDC authorization endpoint |
| `POST` | `/oauth/token` | OIDC token (password, auth_code, refresh_token grants) |
| `GET` | `/oauth/userinfo` | OIDC userinfo |
| `GET` | `/oauth/jwks` | JSON Web Key Set |
| `GET` | `/oauth/.well-known/openid-configuration` | OIDC discovery |
| `POST` | `/saml/acs` | SAML Assertion Consumer Service |
| `GET` | `/saml/metadata` | SAML SP metadata |

**OIDC Security Requirements:**
1. Strict redirect URI validation (exact match, no wildcards).
2. PKCE required (S256). Requests without `code_challenge` rejected with 400.
3. PKCE verified at `/oauth/token` — `code_verifier` validated via SHA-256.
4. `state` parameter echoed unmodified.
5. Authorization codes stored in Redis with 10-minute TTL, linked to `client_id`, user session, and `code_challenge`. Single-use.
6. Scope validation at token endpoint.
7. OIDC client secrets hashed with PBKDF2 at rest.

**Internal management API (mTLS):** `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `/auth/mfa/*`, `/auth/webauthn/*`, `/users/*`, `/orgs/*`.

**Account Lockout Policy:**
- 10 consecutive failures → lock. Exponential backoff: 15min → 24hr cap.
- Locked user receives `INVALID_CREDENTIALS` (not `ACCOUNT_LOCKED`).
- Admin unlock: `POST /users/:id/unlock`.

**TOTP Implementation:**
- Minimum 160-bit secret.
- ±1 window tolerance (90 seconds total).
- Code reuse prevention: successful TOTP code stored in Redis set (90s TTL); rejected if presented again within window.

**SCIM v2 endpoints:** `GET/POST/GET/:id/PUT/:id/PATCH/:id/DELETE/:id` on `/scim/v2/Users`.

### 10.3.9 SAML 2.0 Implementation

Use `github.com/crewjam/saml`.

Key requirements:
1. **Assertion replay protection:** Cache Assertion ID in Redis with TTL = `NotOnOrAfter - now()`. Key: `saml:assertion:{assertion_id}`.
2. **Clock skew:** ±5 minute tolerance (matching JWT/TOTP).
3. **NameID format:** Prefer `emailAddress`. Map to IAM user by org-scoped email lookup.
4. **IdP-initiated flows:** Accept only if org has explicitly enabled `IAM_SAML_ALLOW_IDP_INITIATED`.
5. **Signature validation:** Both Response and Assertion MUST be signed. Reject partial signatures.

Audit events: `auth.saml.login.success`, `auth.saml.login.failure`.

### 10.3.10 Account Enumeration Protection

Always run bcrypt comparison even for nonexistent users:
```go
if user == nil {
    _ = p.Verify(ctx, dummyHash, password)
    return ErrUnauthorized
}
```

### 10.3.11 IAM Kafka Events (via Outbox)

| Event type | Topic | Saga topic? |
|---|---|---|
| `auth.login.success` / `.failure` / `.locked` | `auth.events` | — |
| `auth.logout`, `auth.mfa.enrolled`, `auth.webauthn.registered` | `auth.events` | — |
| `auth.token.created` | `auth.events` | — |
| `user.created`, `user.deleted`, `user.scim.provisioned` | `audit.trail` | `saga.orchestration` |

### 10.3.12 SCIM Authentication

SCIM endpoints authenticate via Bearer token using a dedicated SCIM API key:
- The key is provisioned per-org via `POST /mgmt/connectors` with `scopes: ["scim:write"]`.
- The `shared/middleware/scim.go` middleware validates the key against the
  connector-registry `ValidateAPIKey` path.
- SCIM tokens are stored with type `scim` in the api_tokens table.
- IdP (Okta, Azure AD) provides the Bearer token in the `Authorization` header.
- SCIM requests without a valid token MUST return 401 with WWW-Authenticate header.

Route protection:
```go
r.Route("/auth/scim/v2", func(r chi.Router) {
    r.Use(shared_middleware.SCIMAuth(connectorRegistry))
    r.Get("/Users", h.ListScimUsers)
    r.Post("/Users", h.CreateScimUser)
    r.Get("/Users/{id}", h.GetScimUser)
    r.Patch("/Users/{id}", h.PatchScimUser)
})
```

#### Test Case: SCIM Authentication
1. **Unauthorized:** `GET /auth/scim/v2/Users` without `Authorization` header → `401 Unauthorized` with `WWW-Authenticate: Bearer` header.
2. **Authorized:** `GET /auth/scim/v2/Users` with `Authorization: Bearer <valid_scim_key>` → `200 OK` with SCIM ListResponse.
3. **Invalid Token:** `GET /auth/scim/v2/Users` with `Authorization: Bearer invalid` → `401 Unauthorized`.

---

## 10.4 Control Plane Foundation

### 10.4.1 Route Table

| Method | Path | Required Scope | Circuit Breaker |
|---|---|---|---|
| `POST` | `/v1/policy/evaluate` | `policy:evaluate` | `cb-policy` |
| `POST` | `/v1/events/ingest` | `events:write` | — |
| `GET` | `/v1/scim/v2/Users` | `scim:read` | `cb-iam` |
| `POST` | `/v1/scim/v2/Users` | `scim:write` | `cb-iam` |
| `GET` | `/v1/scim/v2/Users/:id` | `scim:read` | `cb-iam` |
| `PATCH` | `/v1/scim/v2/Users/:id` | `scim:write` | `cb-iam` |

**Admin API (mTLS + JWT):**
- `POST /v1/admin/connectors`: generates prefix (8 chars) + plaintext secret (24 chars), hashes with PBKDF2.
- `PATCH /v1/admin/connectors/:id`: invalidates Redis cache entry on status or scope change.

### 10.4.2 Connector Registry Schema

```sql
CREATE TABLE connector_registry (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    api_key_hash       TEXT NOT NULL,
    api_key_prefix     TEXT NOT NULL,
    webhook_url        TEXT,
    webhook_secret_hash TEXT,
    scopes             TEXT[] NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'active',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_prefix ON connector_registry(api_key_prefix, status);
ALTER TABLE connector_registry ENABLE ROW LEVEL SECURITY;
GRANT SELECT ON connector_registry TO openguard_app;
```

---

## 10.5 Phase 2 Acceptance Criteria

1. **IAM Security:**
   - [ ] `POST /oauth/token` passes with valid credentials; fails with 401 on invalid.
   - [ ] bcrypt cost is 12 (verified by unit test).
   - [ ] JWT contains `jti`, `iat`, `exp`, and `org_id`.
   - [ ] Redis blocklist correctly revokes `jti` on session logout.
2. **Multi-Tenancy:**
   - [ ] Org A cannot see Org B's users via API.
   - [ ] `SELECT` on `users` as `openguard_app` returns 0 rows without `set_config`.
   - [ ] `openguard_migrate` role used for all table creations.
3. **Audit Integrity:**
   - [ ] User creation produces exactly one `outbox_record` in the same transaction.
   - [ ] Outbox relay publishes to Kafka and marks as `published`.
   - [ ] `idempotent` key in Kafka message matches PostgreSQL `id`.
4. **Resilience & SLO:**
   - [ ] `POST /oauth/token` p99 < 150ms at 500 req/s with bcrypt worker pool enabled.
   - [ ] Outbox relay resumes draining within 60s of PostgreSQL primary failover.
5. **SCIM 2.0:**
   - [ ] `GET /v1/scim/v2/Users` returns correct resource counts and schema.
   - [ ] SCIM auth rejects `X-Org-ID` header; derives only from token.
   - [ ] `version` column increments on user patch.
6. **Observability:**
   - [ ] Login failures log with `SafeAttr` redaction.
   - [ ] Every request includes `X-Request-ID` and OTel trace propagation.
