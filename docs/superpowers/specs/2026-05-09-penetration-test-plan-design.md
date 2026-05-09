# Open-Guard Penetration Test Plan

## Overview

Full-stack penetration test targeting all 10 microservices, external gateway, Angular dashboard, Kafka event bus, and 5 data stores (PostgreSQL, Redis, MongoDB, ClickHouse, S3). Structured as a 4-layer attack surface progression mirroring Open-Guard's defense-in-depth architecture.

**Approach:** Layered Attack Surface (Perimeter → Service Mesh → Data Layer → Business Logic)

**Estimated Duration:** ~12 days (excluding retest cycles)

---

## Phase 0: Scope & Pre-requisites

### In Scope
- Gateway (port 8080)
- All 10 microservices: Control Plane, IAM, Policy, Threat, Audit, Alerting, Compliance, DLP, Connector Registry, Webhook Delivery
- Angular dashboard (port 4200)
- Kafka event bus (11 topics, PLAINTEXT)
- Data stores: PostgreSQL (4 schemas), Redis (2 DBs), MongoDB, ClickHouse, S3
- Go SDK and JavaScript SDK
- Example task-management app (port 3005)

### Out of Scope
- Third-party dependencies (npm packages, Go modules, container base images)
- OS-level exploits
- Physical security
- Social engineering

### Test Accounts Required

| Role | Org | Privileges |
|------|-----|------------|
| Guest (unauthenticated) | N/A | Public endpoints only |
| Normal User | Org A | Own data, task CRUD, policy evaluation |
| Normal User | Org B | Different tenant (IDOR target) |
| Admin | Org A | Full mgmt access within org |
| Read-only User | Org A | Limited read access |
| SCIM-provisioned User | Org A | External identity provisioned via SCIM |

### Environment
- Staging/Dev instance with Docker Compose
- Test data pre-seeded: 3 orgs (Alpha, Beta, Gamma), 10+ users, 50+ policies, 1000+ audit events, 20+ connectors
- Production-like but isolated

### Allowed Techniques
SQLi, XSS, SSRF, SSTI, IDOR, privilege escalation, business logic abuse, race conditions, JWT manipulation, SAML assertion manipulation, WebAuthn replay, MFA bypass, outbox/Kafka injection, RLS bypass, mTLS spoofing, prototype pollution, timing attacks.

### Not Allowed
DDoS, destructive payloads to production data stores, certificate authority compromise.

### Rollback Procedure
```bash
docker compose down && docker compose up -d
```
Restore test data seed if needed.

---

## Layer 1: Perimeter Testing

External attack surface — Gateway, Angular SPA, and all unauthenticated endpoints.

### 1.1 Gateway & CORS

**Goals:** Identify CORS misconfigurations, auth bypass, header smuggling.

- Test CORS `Access-Control-Allow-Origin` reflection (control-plane has hardcoded origins: `localhost:4200`, `localstack.cloud`, `instatunnel.io`)
- Test `POST /v1/logs` (no auth) — log injection, XSS via log payload, injection into backend log aggregator
- Verify `SameSite=Strict`, `HttpOnly`, `Secure`, `__Host-` prefix on `openguard_session` cookie
- HTTP method override (`X-HTTP-Method-Override`, `X-Method-Override`)
- HTTP request smuggling (CL/TE, TE/CL)
- Gateway rate limiter bypass tests

### 1.2 Angular SPA

**Goals:** Client-side vulnerabilities, sensitive data exposure, auth guard bypass.

- DOM-based XSS via route params and query strings
- Sensitive data in client-side bundles (API endpoint URLs, internal service names, JWK set information)
- Prototype pollution via Angular merge utilities
- SSE endpoint hijacking — can a cross-origin page subscribe to `/audit/v1/events/stream`?
- Auth guard bypass — access protected routes (`/`, `/admin`, `/mgmt/*`) without valid JWT
- Dependency vulnerability scan (npm audit)

### 1.3 Unauthenticated Endpoints

**Goals:** Find weaknesses in the ~25 public endpoints.

#### JWKS (GET /auth/jwks)
- HS256 symmetric key exposed with KID — key confusion attack
- Algorithm confusion: modify JWT header `alg` from `HS256` to `none`, `HS256` to `RS256` (using public key as HMAC secret if symmetric key serves dual purpose)
- KID injection: path traversal or SQL injection via KID value in JWT header

#### SAML (POST /auth/saml/acs)
- XML signature wrapping (XSW)
- XXE injection in SAML `Response` XML
- Assertion replay — capture a valid assertion, resend it
- Issuer confusion — authenticate against attacker-controlled IdP
- Timing attack on assertion ID uniqueness check (Redis `SetNX`)

#### SAML Metadata (GET /auth/saml/metadata?org_id=)
- IDOR on `org_id` query param — enumerate org existence

#### OAuth2/OIDC
- Authorization code injection (PKCE downgrade from `S256` to `plain`)
- CSRF on `redirect_uri` — open redirector
- Token endpoint: `client_secret_basic` vs `client_secret_post` — is one path less validated?
- `password` grant type (legacy) — credential stuffing

#### WebAuthn (POST /auth/webauthn/login/begin|finish)
- Ceremony verification bypass — replay a captured `authenticatorData` and `signature`
- Origin validation — can a different RP origin complete the login?
- Challenge reuse — does the same challenge work twice?

#### Metrics (GET /metrics on every service)
- Information disclosure: route patterns, request volumes, goroutine count, heap usage, GC stats
- Service enumeration via distinct metric label values
- PII exposure in metric labels (email addresses, user IDs, org IDs as label values)

### 1.4 Authentication Brute Force & Session Management

**Goals:** Credential-based attacks, session weaknesses.

- Rate limiting on `/auth/login` — verify escalating lockout (10 failures → 15min, 30min, 60min, up to 24h)
- Timing analysis — confirm constant-time bcrypt on both valid and invalid users (hotspot: `services/iam/pkg/service/auth.go`)
- Password policy: minimum length, complexity, common password rejection
- Refresh token rotation:
  1. Capture refresh token
  2. Use it → verify old token invalidated
  3. Reuse old token → verify entire family revoked (detection of `family_id` reuse)

### 1.5 Burp Configuration

- Proxy listener: Gateway (localhost:8080)
- Scope: `*.openguard.local`, `localhost:4200`, `localhost:8080`, `localhost:3005`
- Session handling rules: extract JWT from `/auth/login` response → auto-inject into `Authorization: Bearer` header for subsequent requests
- Install extensions: Autorize, Param Miner, JWT Editor, Collaborator Everywhere, Active Scan++, Logger++

---

## Layer 2: Service Mesh & Inter-Service Communication

Internal mesh testing — lateral movement after perimeter breach.

### 2.1 mTLS Weaknesses

**Goals:** Bypass, spoof, or downgrade mTLS authentication.

- IAM port 8080 uses `VerifyClientCertIfGiven` (optional) — test calling internal endpoints (`/mgmt/users`, `/mgmt/connectors`) without client cert
- IAM port 8443 uses `RequireAndVerifyClientCert` but CA cert is at `certs/ca.crt` — can we forge a client cert signed by the dev CA?
- Certificate SAN enumeration — does one service's cert cover multiple services? (cross-service impersonation)
- mTLS renegotiation — is insecure renegotiation supported? (downgrade attack)
- Dev fallback: services fall back to HTTP if certs not found (hotspot documented in HOTSPOTS.md)

### 2.2 Internal API Key

**Goals:** Steal or bypass `X-Internal-Key`.

- Observe header in logs, error responses, debug pages
- Is the key compared with constant-time? (timing side-channel on `subtle.ConstantTimeCompare`)
- Does the key appear in any client-side or Kafka event payload?
- Rotation mechanism assessment (likely none in dev)

### 2.3 Kafka Event Bus (PLAINTEXT)

**Goals:** Inject, poison, or eavesdrop on event streams.

- Connect directly to Kafka broker (port 9092, no auth) — produce and consume arbitrary events
- Outbox poisoning: `POST /v1/events/ingest` allows callers to set `event["topic"]` in JSON body — write to `policy.changes`, `auth.events`, `threat.alerts`
- DLQ poisoning: send malformed events to `outbox.dlq` — crash consumers on deserialization
- Event replay: capture a valid event (e.g., `auth.events` login), re-inject it
- Topic enumeration via `MetadataRequest` API

### 2.4 Service-to-Service Proxy Abuse

**Goals:** SSRF via Control Plane proxy paths.

- `POST /v1/policy/evaluate` — forwarded to Policy service — test URL manipulation in request body
- `POST /v1/events/ingest` — forwarded to Audit service — test host header injection
- `GET/POST/PATCH /v1/scim/v2/Users*` — forwarded to IAM — test path traversal
- Does Control Plane strip or validate `X-Internal-Key` header from incoming proxy requests? (if not, we can inject it into proxied requests)

### 2.5 Burp Configuration

- Direct proxy on internal ports via docker network (e.g., `iam:8443`, `policy:8082`)
- If mTLS blocks direct access, test via Control Plane proxy paths (SSRF vectors)
- Craft custom mTLS client certs from `certs/` directory using Repeater

---

## Layer 3: Data Layer Testing

All data store surfaces — SQL, NoSQL, cache, and object storage.

### 3.1 PostgreSQL RLS Bypass

**Goals:** Break the multi-tenancy boundary. Hotspot: `shared/rls/context.go`.

- RLS session leakage: `pgxpool.AfterRelease` may not clear `app.current_org_id` under concurrent connection reuse — test rapid org switching across 100+ concurrent goroutines
- Can we `SET LOCAL app.current_org_id = 'other-org-id'` via SQL injection on any endpoint?
- RLS bypass via `pg_stat_statements`, `pg_prewarm`, or other admin functions
- Error message analysis: do errors reveal presence of data in other orgs?

### 3.2 SQL Injection

**High-risk endpoints** (parameterized queries expected, verify each):

| Service | Endpoint | Parameters |
|---------|----------|------------|
| IAM | `GET /mgmt/users?email=` | email filter |
| IAM | `GET /auth/scim/v2/Users?filter=` | SCIM filter expression |
| Policy | `GET /v1/policies/?name=` | name filter, pagination |
| Audit | `GET /v1/events?type=&resource=&user_id=` | multiple filters |
| DLP | `GET /v1/dlp/findings?status=` | status filter |
| Compliance | `GET /v1/compliance/stats?from=&to=` | date range |

PostgreSQL-specific payloads:
- `pg_sleep()` for time-based blind
- `ST_*` function abuse
- JSONB operators (`@>`, `?`, `||`)
- Array operators (`ANY`, `ALL`)
- Error-based via `CAST` and type coercion
- Second-order injection in policy rule evaluation

### 3.3 MongoDB Injection

- `$where`, `$ne`, `$regex`, `$gt`, `$nin` injection in query filters
- SSE change stream (`GET /v1/events/stream`) — can we subscribe to a different org's change stream?
- CQRS split consistency window — race between primary write and secondary read
- NoSQL boolean-based blind injection via `$regex` payloads

### 3.4 Redis Attacks

- **JWT blocklist fail-open** (hotspot): If Redis is unavailable (`SET` fails), validation falls open — can we cause Redis unavailability via large key insertion or network congestion?
- Key collision: craft JTI that collides with blocklist entries
- Rate limiter bypass: reset counters via `RESTORE` or `DEL` if Redis CLI exposed
- MFA challenge race: parallel `SetNX` for same challenge UUID
- SAML assertion replay: parallel `SetNX` for same assertion ID
- Auth code reuse: authorization codes stored 10min TTL — race window

### 3.5 ClickHouse Injection

- Compliance analytics queries — `UNION` injection, `GLOBAL IN` subquery abuse
- `toString()` type coercion exploits
- Time-based injection via `sleepEachRow()` or heavy aggregation

### 3.6 S3 / Storage

- Compliance reports at `reports/{org_id}/{job_id}.pdf` — enumerate `job_id`
- Path traversal if `org_id` or `job_id` influenced by user input
- Presigned URL (1h TTL) — clock skew abuse to extend validity
- Bucket listing via `ListObjects` if bucket policy allows

### 3.7 Burp Configuration

- Custom Intruder payload sets for PostgreSQL (`pg_sleep`, `CAST`, JSONB) and MongoDB (`$regex`, `$ne`)
- Param Miner on every API endpoint for hidden query and POST parameters
- Collaborator Everywhere for blind SSRF/SQLi callback detection

---

## Layer 4: Business Logic & Cross-Cutting Attacks

Senior tester layer — identity federation seams, policy engine abuse, async race conditions.

### 4.1 Identity Federation Abuse

**Goals:** Bridge across the 6 authentication mechanisms.

- SAML→JWT bridge: authenticate via SAML, then manipulate JWT claims (`org_id`, `role`, `sub`)
- OAuth2→SAML→WebAuthn chain: authenticate via one mechanism, escalate by chaining to another
- OAuth2 authorization code injection: intercept auth code (from URL fragment or referrer), redeem with crafted `code_verifier`
- PKCE downgrade: if server accepts both `S256` and `plain`, force `plain` to reuse observed code
- SCIM provisioning escalation: SCIM-created user may receive unexpected privileges based on mapping
- IdP confusion: register rogue connector → trick users into authenticating against attacker-controlled IdP with crafted `issuer` and `audience`

### 4.2 Multi-Tenancy Violations

- Every endpoint deriving `org_id` from JWT: test Org A user accessing Org B resources
- Client-side `org_id` manipulation: if any endpoint accepts `org_id` from request body (excluding mgmt endpoints), test
- Org enumeration via timing or error message differences
- Cross-org admin access: can Org A admin access Org B data through any admin endpoint?

### 4.3 Policy Engine Attacks

**Critical endpoint:** `POST /v1/policy/evaluate`

- Craft policy evaluation payload that returns `allow: true` for unauthorized actions
- ReDoS (Regular Expression Denial of Service) in policy rule patterns
- Redis policy cache poisoning — inject cached decision for unauthorized action
- Thundering herd on cache invalidation: concurrent requests during cache miss
- Stale-while-revalidate test: can we force use of stale cache during invalidation?

### 4.4 Role Matrix Verification

Complete authorization matrix test using Autorize extension:

| Endpoint | Guest | Normal (Org A) | Normal (Org B) | Admin | Read-Only |
|----------|-------|----------------|----------------|-------|-----------|
| `GET /auth/me` | ❌ | ✅ | ✅ | ✅ | ✅ |
| `GET /mgmt/users` | ❌ | ❌ | ❌ | ✅ | ? |
| `POST /mgmt/users` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `GET /mgmt/users/{id}` | ❌ | own only | own only | ✅ | own only |
| `POST /mgmt/orgs` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `GET /v1/policies/` | ❌ | own org | own org | all | own org |
| `POST /v1/policies/` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `DELETE /v1/policies/{id}` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `POST /v1/policy/evaluate` | ❌ | own scope | own scope | all | own scope |
| `GET /v1/threats/alerts` | ❌ | own org | own org | all | own org |
| `POST /v1/dlp/policies` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `GET /v1/compliance/reports` | ❌ | own org | own org | all | own org |
| `DELETE /mgmt/connectors/{id}` | ❌ | ❌ | ❌ | ✅ | ❌ |

Special focus: read-only user attempting writes, normal user accessing admin-only features, inter-org data access.

### 4.5 Race Conditions & Concurrency

- **MFA enable race**: parallel requests to enable/disable MFA — bypass verification step
- **Refresh token rotation race**: send same token in 10 parallel requests — use twice before family rotates
- **User creation idempotency race**: parallel requests with same idempotency key — create duplicate users
- **SSE subscription race**: subscribe to audit stream while permissions are being revoked — does connection persist?
- **Rate limiter reset race**: hit limit, then parallel requests as counter resets

### 4.6 Outbox & Async Reliability

- Outbox relay polls every 5s — during Kafka downtime, records accumulate. On recovery, can duplicates occur?
- Idempotency key replay for user creation, policy creation — detect valid actions by trying same key
- DLQ accumulation: craft events that always end up in DLQ — clog broker
- Outbox table bloat: generate events faster than relay can publish

### 4.7 Report Generation & File Handling

- Compliance report generation via bulkhead worker — trigger mass generation (resource exhaustion)
- S3 presigned URL capture — test validity after 1h TTL via clock skew
- Report path manipulation if `org_id` or `job_id` is user-influenced

### 4.8 Burp Configuration

- Autorize: baseline high-priv session → replay all requests with low-priv session
- Turbo Intruder: HTTP pipelining for race conditions on MFA, refresh token, outbox
- Sequencer: token randomness analysis for JWT, MFA challenges, auth codes
- Repeater: SAML assertion crafting (XML signature wrapping, XXE)

---

## Automation & Tooling

### Burp Extensions

| Extension | Purpose | Layer |
|-----------|---------|-------|
| Autorize | Automated role matrix replay | L1, L4 |
| Param Miner | Hidden parameters discovery | L1, L2, L3 |
| JWT Editor | JWT manipulation (alg, KID, claims) | L1, L4 |
| Collaborator Everywhere | Blind SSRF/XXE/SSTI detection | L1, L2, L3 |
| Active Scan++ | Enhanced API scanning | All |
| Turbo Intruder | Race condition testing | L4 |
| Sequencer | Token entropy analysis | L1, L4 |
| Logger++ | Centralized request logging | All |

### External Tools

| Tool | Purpose |
|------|---------|
| kcat (kafkacat) | Kafka topic inspection and injection |
| psql | Direct PostgreSQL queries |
| mongosh | MongoDB injection testing |
| openssl | mTLS certificate manipulation |
| curl | Scripted endpoint testing for role matrix |
| jq | JSON response processing |
| Custom Go client | SDK-aware testing (mTLS + JWT + policy flow) |

### Test Data Seed

Pre-load before starting:
- 3 organizations: Alpha, Beta, Gamma
- 10+ users per org across all roles
- 50+ policies with varying scope and conditions
- 1000+ audit events for pagination/SSE testing
- 20+ SAML/OAuth2 connector configs
- Known vulnerable states: expired tokens, revoked sessions, locked accounts, orphaned MFA configs

---

## Reporting

### Finding Template

```yaml
Title: <CWE-ID> - <Service/Endpoint> - <Vulnerability>
Severity: Critical / High / Medium / Low / Info
Layer: L1 / L2 / L3 / L4
Status: Open / Fixed / NFP

Description:
  <2-3 sentences on the vulnerability>

Impact:
  <What an attacker can achieve>
  CVSS: <vector>

Affected Endpoints:
  - <method> <path>

Reproduction:
  1. <step>
  2. <step>
  3. <step>

Evidence:
  [Request/response details]

Remediation:
  <Code-level fix>

Bypass Notes:
  <If previously fixed, how was it re-bypassed?>
```

### Executive Summary

- Risk Score (aggregate CVSS)
- Critical/High finding count with top 3
- Attack surface summary (layers tested, endpoints covered, total requests made)
- Root cause trends (e.g. "40% of findings are authorization bypasses")
- Remediation roadmap: Immediate (patch) → Short-term (config) → Long-term (architecture)

---

## Retest Phase

After each fix cycle:

1. Replay exact reproduction steps — original POC must fail
2. Bypass attempt — same attack class, different technique
3. Regression check — fix must not break adjacent functionality
4. Full role matrix recheck if authorization logic changed
5. Status: Fixed (verified) / Partial (bypass found) / Not Reproducible

---

## Timeline

| Phase | Duration |
|-------|----------|
| Setup: environment, accounts, Burp config, test data | 0.5 day |
| L1: Perimeter (Gateway, Angular, auth endpoints, CORS) | 2 days |
| L2: Service Mesh (mTLS, internal key, Kafka, SSRF) | 2 days |
| L3: Data Layer (SQLi, RLS, MongoDB, Redis, ClickHouse) | 3 days |
| L4: Business Logic (federation, policy, races, roles) | 3 days |
| Reporting (exec summary + detailed findings) | 1 day |
| Retest (per fix cycle) | 1 day |
| **Total** | **~12 days** |

---

## Risk Map Cross-Reference

| Hotspot (from HOTSPOTS.md) | Tested In | Attack Type |
|----------------------------|-----------|-------------|
| `services/iam/pkg/service/auth.go` — timing oracle | L1.4 | Timing attack |
| `services/iam/pkg/service/mfa.go` — TOTP/WebAuthn replay | L4.5 | Race condition |
| `shared/kafka/outbox/relay.go` — outbox bloat | L4.6 | DoS / async abuse |
| `services/policy/pkg/service/` — thundering herd | L4.3 | Cache invalidation |
| `shared/rls/context.go` — RLS session leakage | L3.1 | Connection reuse |
| `/metrics` endpoints — exposed on all services | L1.3 | Information disclosure |
