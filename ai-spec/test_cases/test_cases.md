# OpenGuard — Test Strategy: Phases 1–10

> **Claude Code directive:** Read this file before writing any test code for
> Phases 1–10. Every test case traces back to a numbered acceptance criterion
> in the corresponding spec file. Phase N does not start until every acceptance
> gate for Phase N–1 is green in CI. Integration and E2E tests run against real
> services — real PostgreSQL, real Kafka, real MongoDB, real Redis, real
> ClickHouse. No mocks, no stubs, no in-memory substitutes.
>
> The absolute rules from `test_cases.md §2` carry forward unchanged
> into every phase. They are not repeated here; treat them as always in effect.

---

## Contents

- [Phase 1 — Infra, CI/CD & Observability](#phase-1--infra--cicd--observability)
- [Phase 2 — Foundation & Authentication](#phase-2--foundation--authentication)
- [Phase 3 — Policy Engine](#phase-3--policy-engine)
- [Phase 4 — Event Bus & Audit Log](#phase-4--event-bus--audit-log)
- [Phase 5 — Threat Detection & Alerting](#phase-5--threat-detection--alerting)
- [Phase 6 — Compliance & Analytics](#phase-6--compliance--analytics)
- [Phase 7 — Security Hardening](#phase-7--security-hardening)
- [Phase 8 — Load Testing & SLO Verification](#phase-8--load-testing--slo-verification)
- [Phase 9 — Documentation Quality Gates](#phase-9--documentation-quality-gates)
- [Phase 10 — DLP](#phase-10--dlp)
- [Full-System Acceptance Scenario (45 Steps)](#full-system-acceptance-scenario-45-steps)

---

## Phase 1 — Infra, CI/CD & Observability
> Phase 2 does not start until every test here passes in CI with `go test -race`.
> All integration and E2E tests use **real services** via `testcontainers-go` or the full `docker-compose` stack. No mocks, no fakes.

---

## Test Pyramid

| Layer | Tooling | Gate |
|---|---|---|
| Unit | `go test -race -short` / Vitest | 70% coverage (BE), 80% (FE) |
| Integration | `go test -run Integration -race` | All named tests pass |
| E2E | `go test ./tests/e2e/...` + Playwright | All named scenarios pass |
| Static | `golangci-lint`, `go-sqllint`, `govulncheck`, ESLint, `tsc` | Zero violations |

---

## Hard Rules

- No `t.Skip()` in integration or E2E tests.
- No mock services — use real containers (`postgres:16`, `cp-kafka:7.6.0`, `redis:7-alpine`, `mongo:7`).
- No hard-coded ports — let `testcontainers-go` assign ephemeral ports.
- No `time.Sleep` — use `require.Eventually` with a real poll interval.
- No asserting on internal struct fields — test behaviour through public APIs.
- No Kafka offset committed before the downstream DB write is confirmed.

---

## Unit Tests

### UT-01 — EventEnvelope JSON round-trip
Marshals a fully-populated `EventEnvelope` to JSON and back, then asserts every field survives the round-trip. A second sub-test unmarshals the raw JSON map and asserts all required contract key names are present (e.g. `org_id`, `schema_ver`). Guards against silent consumer breakage from field renames.

### UT-02 — OutboxRecord DB tag contract
Uses `reflect` to walk every field of `OutboxRecord` and assert its `db:` struct tag matches the exact SQL column name. A second sub-test asserts the three valid status string values (`pending`, `published`, `dead`) round-trip through the struct unchanged.

### UT-03 — Sentinel errors distinct identity
Iterates all sentinel errors (`ErrNotFound`, `ErrUnauthorized`, `ErrCircuitOpen`, etc.) and asserts no two compare equal via `errors.Is`. A second sub-test wraps a sentinel one and two levels deep and asserts `errors.Is` still resolves correctly.

### UT-04 — Kafka topic constant values
Asserts each Go constant (`TopicAuthEvents`, `TopicAuditTrail`, etc.) equals the exact string declared in `kafka/topics.json`. Guards against producers and consumers drifting to different topic names with no compile-time error.

### UT-05 — Circuit breaker state transitions
Drives a breaker to the failure threshold and asserts the next call returns `ErrCircuitOpen`. A second sub-test waits for the open duration to elapse, makes a successful probe call, and asserts the breaker closes. A third sub-test confirms a call that exceeds the request timeout returns `context.DeadlineExceeded`.

### UT-06 — RLS context helpers
Stores an org ID with `WithOrgID`, retrieves it with `OrgID`, and asserts the round-trip. Sub-tests confirm: an unset context returns `""`, two independent contexts do not cross-contaminate, and a child context inherits the parent's org ID.

### UT-07 — SafeAttr log redaction
Calls `SafeAttr` with each sensitive key name (`password`, `token`, `authorization`, etc.) and asserts the value is not the raw secret. A second sub-test calls it with safe keys (`user_id`, `status`) and asserts the value passes through unchanged. A third sub-test confirms the match is case-insensitive (`Authorization` → redacted).

### UT-08 — HMAC-SHA256 webhook signatures
Asserts the same input produces the same signature (deterministic). Asserts a modified payload produces a different signature (tamper-evident). Asserts `VerifyHMAC` accepts the valid signature and rejects a tampered payload. Asserts a same-length forged signature is rejected, documenting the requirement for `subtle.ConstantTimeCompare`.

### UT-09 — Outbox writer marshals EventEnvelope
Uses a `fakeTx` (hand-written, no mock framework) to capture INSERT arguments without a real DB. Asserts the org ID column uses the explicit `orgID` parameter, not `envelope.OrgID`. Asserts the payload bytes unmarshal back into a valid `EventEnvelope` with all fields intact.

### UT-10 — bcrypt worker pool backpressure
Floods a 2-worker pool with 200 concurrent `Verify` calls. Asserts at least some calls are rejected with `ErrBulkheadFull` and at least some are accepted. A second sub-test cancels the context before enqueuing and asserts `context.Canceled` is returned immediately.

---

## Integration Tests

All integration tests start real containers via `testcontainers-go`. Build tag: `//go:build integration`.

### IT-01 — RLS cross-tenant isolation (PostgreSQL)
Seeds two orgs and one user per org. Sets `app.org_id` session variable to org 1, queries `users`, and asserts exactly 1 row is returned (the org 1 user, not the org 2 user). Repeats for org 2. A third sub-test queries with no session variable set and asserts zero rows are returned (fail-safe default).

### IT-02 — Outbox atomic write (PostgreSQL)
Opens a transaction, inserts a business row and an outbox record in the same transaction, commits, then asserts both rows exist in their respective tables. A second sub-test performs the same inserts but calls `Rollback`, then asserts neither row exists.

A third sub-test writes an outbox record under org 1's RLS context, then queries as org 2 and asserts zero rows are visible.

### IT-03 — Outbox relay publishes to real Kafka
Seeds a `pending` outbox record, starts the relay pointing at real Kafka and real PostgreSQL. Consumes from the target topic and asserts a message is received. Asserts the outbox record status is updated to `published` in the DB.

A second sub-test injects a fault to cancel the relay context immediately after the Kafka send (before the DB status update). Asserts the record status is still `pending`. Restarts the relay and asserts the record is eventually re-processed and marked `published`.

### IT-04 — Contract: IAM event parseable by audit consumer (Kafka)
Produces a fully-populated IAM `EventEnvelope` onto `audit.trail` using a real Kafka producer. A real consumer reads from the same topic and deserialises the message using the audit service's parse logic. Asserts `SchemaVer`, `ID`, `Type`, `OrgID`, `Source`, and `EventSource` all match. Asserts the `Payload` field is valid JSON containing `email` and `mfa_used`.

### IT-05 — SQL lint catches string concatenation
Writes a temporary Go file containing SQL built via string concatenation and `fmt.Sprintf`. Runs `go-sqllint` against it and asserts a non-zero exit code and a violation message. A second sub-test runs `go-sqllint` against a parameterised query and asserts exit code 0.

### IT-06 — Health endpoints
Starts the control-plane service with all deps. Calls `/health/live` and asserts `200 {"status":"ok"}`. Starts the service without PostgreSQL and polls `/health/ready` until it returns `503`. Starts with full deps, calls `/health/ready`, and asserts `200` with a `checks` map containing `postgres`, `mongo`, `redis`, and `kafka`, all reporting `ok`.

### IT-07 — All Phase 1 metrics on `/metrics`
Starts the control-plane service, fetches `/metrics`, and asserts all 20 `openguard_*` metric names are present in the Prometheus text output.

---

## End-to-End Tests

E2E tests run against the full `docker-compose` stack. `setup_test.go` calls `docker compose up -d` and waits for all health checks to pass before any test runs.

### E2E-01 — All services healthy on startup
Polls every service's `/health/ready` endpoint and asserts `200` within 120 seconds. A second test connects to MongoDB and runs `replSetGetStatus`, asserting the replica set has at least one member with `stateStr == "PRIMARY"`.

### E2E-02 — CI pipeline simulation
Runs `go test ./... -race -short` against the project root and asserts exit code 0 and no `DATA RACE` output. A second test runs with `-coverprofile` and parses per-file coverage, asserting no file is below 70%. A third test runs `govulncheck ./...` and asserts no `CRITICAL` vulnerabilities.

### E2E-03 — Prometheus scrapes all 11 services
Queries the real Prometheus API (`/api/v1/targets`) and asserts all 11 service scrape targets are in `state: up`. Queries the Grafana datasource API and asserts all `openguard_*` metrics are queryable.

### E2E-04 — Alerting rules fire correctly
Stops the outbox relay for over 2 minutes and polls Alertmanager until `OutboxLagHigh` appears in the active alerts. Kills the policy service and polls until `CircuitBreakerOpen` is active. Both alerts must resolve after the services are restored.

### E2E-05 — Helm chart correctness
Runs `helm lint --strict` on the chart directory and asserts no warnings. Runs `helm template` and asserts the rendered output contains Deployment, Service, ServiceMonitor, and HorizontalPodAutoscaler manifests. Asserts `terminationGracePeriodSeconds: 45` is present in the Deployment spec.

### E2E-06 — Connected App UI (Playwright)
Four sequential scenarios run against the real web app:
1. **Registration** — completes the connector registration form and asserts a one-time API key is displayed exactly once.
2. **Connector listing** — navigates to the connectors page and asserts the newly registered connector appears in the list.
3. **Suspend confirmation** — clicks Suspend, asserts a confirmation dialog appears requiring the connector name to be typed, types it, confirms, and asserts the status changes to `Suspended`.
4. **Webhook delivery log** — triggers a test webhook delivery and asserts a delivery log entry appears in the UI within 30 seconds.

---

## Observability Tests (Integration)

### OBS-01 — No secrets in service logs
Captures stdout from the control-plane service as structured JSON. Scans every log line and asserts no field value contains patterns like `password`, `secret`, `bearer`, or `token`. A second test asserts every log line includes the required fields: `service`, `env`, `level`, and `msg`.

### OBS-02 — Graceful shutdown within 30 seconds
Starts the control-plane as a real OS process, verifies `/health/live` returns `200`, sends `SIGTERM`, and asserts the process exits cleanly (exit code 0) within 35 seconds.

---

## Phase 1 Acceptance Gate

All items below must pass in CI before Phase 2 begins.

**Infrastructure:** E2E-01 (all services healthy, MongoDB replica set initialised)

**CI Pipeline:** E2E-02 (race detector clean, 70% coverage gate, no critical CVEs), CI-02 (SQL lint catches concatenation), CI-03 (Trivy no CRITICAL/HIGH unfixed CVEs)

**Shared Contracts:** UT-01 (EventEnvelope round-trip), UT-02 (OutboxRecord DB tags), IT-04 (IAM event parseable by audit consumer)

**Observability:** E2E-03 (all 11 services scraped, all metrics in Grafana), IT-07 (all metrics on `/metrics`)

**Alerting:** E2E-04 (`OutboxLagHigh` and `CircuitBreakerOpen` fire correctly)

**Helm:** E2E-05 (lint, template render, `terminationGracePeriodSeconds=45`)

**Connected App UI:** E2E-06 (registration, connector listing, suspend confirmation, webhook delivery log)

**Security & Correctness:** IT-01 (cross-tenant RLS isolation), IT-02 (atomic outbox + business row, rollback), IT-03 (relay offset-after-write), UT-05 (circuit breaker opens at threshold), UT-10 (bcrypt pool backpressure)

**Health Checks:** IT-06 (`/health/live` 200, `/health/ready` 503 with no PostgreSQL, all four dep checks)

**Observability Correctness:** OBS-01 (no secrets in logs, required log fields), OBS-02 (graceful shutdown ≤35s)

## Phase 2 — Foundation & Authentication

**Spec:** `../be_open_guard/09-phase2-foundation-and-auth.md`  
**Goal:** Enterprise-grade auth skeleton running. JWT, RLS, Outbox, circuit breakers, and connector registration all operational.

---

### Unit Tests

**UT2-01 — bcrypt cost enforcement**  
Verify that `NewAuthWorkerPool` produces hashes with cost exactly 12. Generate a hash through the pool and use `bcrypt.Cost()` to read back the cost from the resulting hash string. Fail if cost ≠ 12. This prevents an accidental cost reduction that would weaken password security at scale without any runtime error.

**UT2-02 — JWT claims completeness**  
Sign a test JWT through the keyring and decode the claims without validating the signature. Assert that `jti`, `iat`, `exp`, `org_id`, `user_id`, and `kid` header are all present and non-empty. A missing `jti` means revocation via the Redis blocklist is impossible; a missing `kid` means the keyring cannot select the correct verification key on decode.

**UT2-03 — JWT keyring: active key signs, verify-only key verifies but does not sign**  
Configure a keyring with one active key and one verify-only key. Assert that `Sign()` always uses the active key (check `kid` in the output header). Assert that `Verify()` accepts tokens signed with either key. Assert that `Verify()` rejects a token with a kid that does not exist in the keyring.

**UT2-04 — JWT blocklist: revoked jti is rejected**  
Sign a JWT and immediately insert its `jti` into the fake Redis blocklist. Call `Verify()` and assert it returns `ErrTokenRevoked`. Then call `Verify()` with a different `jti` not in the blocklist and assert it succeeds. This test uses a handwritten in-memory fake for the blocklist, not a real Redis. The integration test (IT2-04) verifies the real Redis path.

**UT2-05 — Risk-based session scoring: table-driven**  
Run the session risk scorer against a table of inputs covering every factor in the spec: user-agent family change (score 60), IP subnet change (score 40), IP host change within same /16 (score 15), UA version change within same family (score 20). Verify combined scores are additive. Assert that a score ≥ 80 returns `SessionActionRevoke` and a score < 80 returns `SessionActionAccept`. Test boundary values: score exactly 80 revokes; score 79 accepts.

**UT2-06 — Refresh token single-use: reuse returns SESSION_COMPROMISED**  
Use a fake token store that marks tokens as used. Call `ConsumeRefreshToken()` once and assert success. Call it a second time with the same token and assert it returns `ErrSessionCompromised` (not `ErrTokenExpired` or `ErrTokenInvalid`). The error code distinction matters: `ErrSessionCompromised` triggers full session revocation in the HTTP handler.

**UT2-07 — Account lockout: exponential backoff thresholds**  
Using a fake user store, simulate 10 consecutive failed logins. Assert that after the 10th failure the account status transitions to locked. Assert that the locked-until timestamp is approximately 15 minutes in the future. Assert that the response error code is `INVALID_CREDENTIALS`, not `ACCOUNT_LOCKED` (enumeration protection).

**UT2-08 — Account enumeration protection: timing is constant**  
Call the login handler with a valid email+wrong password. Call it with a non-existent email+any password. Assert that both code paths call bcrypt. The non-existent user path must not return faster than the valid-user path because it must always call `Verify()` with a dummy hash (§10.3.10). This is a code-path test: verify `pool.Verify()` is called in both branches, not a wall-clock timing test.

**UT2-09 — TOTP: code reuse detection**  
Use a fake Redis store that acts as the reuse set. Submit a valid TOTP code the first time: assert success and that the code was written to the set. Submit the same code a second time: assert `ErrTOTPReplay`. Submit a different valid code: assert success.

**UT2-10 — AES-256-GCM multi-key keyring: encrypt/decrypt round-trip and key rotation**  
Encrypt with key `mk1` (active). Decrypt: assert success and matching plaintext. Add key `mk2` as active, set `mk1` to verify-only. Encrypt a new value: assert it uses `mk2` (check the `kid:` prefix in the ciphertext). Decrypt the original `mk1`-encrypted value: assert success. Remove `mk1` from the keyring entirely: assert decryption of the `mk1` value now returns an error.

**UT2-11 — MFA backup codes: HMAC stored, single-use on verification**  
Generate 8 backup codes. Assert that the stored values are HMAC-SHA256 hex strings, not the plaintext codes. Verify one code: assert success and that the code is marked used. Verify the same code again: assert failure. Verify a code that was never generated: assert failure.

**UT2-12 — SCIM ETag: version increments atomically**  
Use a fake repository. Simulate three PATCH operations on the same user. Assert that the version field increments from 1 to 2 to 3. Assert that `PUT` with a stale `If-Match` header (ETag mismatch) returns `ErrVersionConflict`. The version increment must happen in the same transaction as the field update, never in a separate statement.

**UT2-13 — Migration: every .up.sql has a matching .down.sql**  
Walk the `migrations/` directory for a given service. For every file matching `*.up.sql`, assert that the corresponding `*.down.sql` exists and is non-empty. This is a filesystem test, not a database test. It catches the common mistake of writing the up migration and forgetting the down.

---

### Integration Tests

**IT2-01 — Full login flow: real PostgreSQL, real Redis**  
Register an org and user in a real PostgreSQL container. Call `POST /oauth/token` with correct credentials against a running IAM service (started in a container with real deps). Assert the response contains `access_token`, `refresh_token`, and that the JWT header contains a `kid`. Decode the JWT and assert `jti`, `exp`, `org_id`, and `user_id` are present. Query Redis directly and assert the `jti` is NOT in the blocklist (it hasn't been revoked).

**IT2-02 — Logout: jti written to Redis blocklist with correct TTL**  
Perform a login to obtain an access token. Call `POST /auth/logout` with that token. Query Redis directly for the `jti` key. Assert it exists. Assert its TTL is approximately `exp - now()` (within 5 seconds). Call any authenticated endpoint with the same token. Assert `401 TOKEN_REVOKED`. This verifies the dynamic TTL requirement from §1.3.

**IT2-03 — RLS enforcement: migration role vs app role**  
Connect to real PostgreSQL as `openguard_migrate`. Create a table with `CREATE TABLE ... ENABLE ROW LEVEL SECURITY`. Assert no error. Connect as `openguard_app`. Execute a query without setting `app.org_id`. Assert the query returns 0 rows, not an error. Connect as `openguard_app`. Set `app.org_id` to a real org UUID. Assert the query returns only rows matching that org_id. Connect as `openguard_app`. Attempt a DDL statement: assert it fails with a permissions error.

**IT2-04 — Migration up/down: real PostgreSQL with golang-migrate**  
Run `migrate up` on a fresh PostgreSQL container. Assert all tables exist and RLS is enabled on every org-scoped table. Run `migrate down` (all steps). Assert all tables are removed. Run `migrate up` again to verify idempotency.

**IT2-05 — Distributed migration lock: two processes race to migrate**  
Start two IAM service processes simultaneously pointing at the same PostgreSQL container. Assert that exactly one wins the Redis `SET NX` lock and runs migrations. Assert the other waits and then succeeds (either by observing the schema is already migrated or by acquiring the lock after the first finishes). Assert that the DB schema ends up in the correct state with no duplicate constraint errors.

**IT2-06 — Connector registration: PBKDF2 hash stored, prefix indexed**  
Call `POST /v1/admin/connectors` on the real control-plane. Assert `201` and a one-time API key in the response with the expected `og_` prefix format. Query the real PostgreSQL `connector_registry` table directly. Assert `api_key_hash` is not empty and does not equal the plaintext key. Assert `api_key_prefix` matches the first 8 characters of the returned key. Assert the plaintext secret is nowhere in the database.

**IT2-07 — Connector auth: Redis cache path vs PBKDF2 fallback**  
Register a connector. Make a first authenticated request. Observe that a Redis key exists for the prefix. Assert the request succeeds. Delete the Redis key manually (simulate cache miss). Make a second authenticated request. Assert it still succeeds (PBKDF2 fallback). Assert a new Redis key is written after the cache miss. This verifies the hot-path performance design without mocking Redis.

**IT2-08 — SCIM provisioning: org_id derived from token, not X-Org-ID header**  
Configure a SCIM token for org A. Send `POST /v1/scim/v2/Users` with an `X-Org-ID: <org-B-id>` header injected. Query PostgreSQL as openguard_app with org A's RLS context. Assert the user was created under org A, not org B. This directly tests the critical SCIM security requirement from §10.3.7.

**IT2-09 — User creation: exactly one outbox record in the same transaction**  
Call `POST /auth/register` on the real service. Before the outbox relay runs (disable relay during test), query PostgreSQL. Assert exactly one row in `users` and exactly one row in `outbox_records`, and that both rows have the same `org_id`. Assert the outbox record's payload is a valid `EventEnvelope` with type `user.created`.

**IT2-10 — Outbox relay: IAM events reach Kafka**  
With the outbox relay running, call `POST /auth/register`. Consume from the real Kafka `audit.trail` topic using a real consumer. Assert a message arrives within 10 seconds. Assert the message deserializes as a valid `EventEnvelope` with `type: "user.created"`, `source: "iam"`, and `event_source: "internal"`.

**IT2-11 — Session risk scoring: high-risk refresh is rejected**  
Perform a login from IP `203.0.113.1` and user-agent `Mozilla/5.0 Chrome/120`. Issue a refresh from a different IP subnet `198.51.100.1` and a completely different user-agent `curl/8.0`. Assert the refresh returns `401 SESSION_REVOKED_RISK`. Query PostgreSQL and assert the session is marked revoked.

**IT2-12 — WebAuthn challenge state: stored in Redis with 5-minute TTL**  
Initiate a WebAuthn registration ceremony. Query Redis for the challenge key `webauthn:challenge:{user_id}:{session_id}`. Assert it exists. Assert its TTL is approximately 300 seconds. Assert the challenge is bound to the server-generated session ID, not a client-supplied value.

**IT2-13 — SAML replay protection: same assertion ID rejected twice**  
Construct a valid SAML assertion. Process it once: assert success. Process the same assertion again (same assertion ID): assert `401 SAML_REPLAY_DETECTED`. Query Redis and assert the assertion ID key exists with a non-zero TTL. This verifies §10.3.9's replay protection without mocking the SAML IdP.

**IT2-14 — TOTP window: codes outside ±1 interval are rejected**  
Enroll TOTP for a real user. Generate a code for T-3 (three 30-second windows in the past). Assert rejection. Generate a code for T-1 (one window in the past): assert success. Generate a code for T+1 (one window in the future): assert success. Generate a code for T+2: assert rejection.

---

### End-to-End Tests

**E2E2-01 — Full auth journey: register → login → use JWT → logout → token invalid**  
Start the full docker-compose stack. Call `POST /auth/register` to create an org and admin user. Call `POST /oauth/token` and capture the access token. Make an authenticated request to `GET /v1/admin/connectors` (any org-scoped endpoint). Assert `200`. Call `POST /auth/logout`. Make the same authenticated request again. Assert `401 TOKEN_REVOKED`. This covers full-system acceptance steps 2–3 and verifies the blocklist works end-to-end.

**E2E2-02 — Cross-tenant isolation via HTTP**  
Register two separate orgs each with an admin user. Log in as org A's admin. Attempt `GET /users` which should return only org A's users. Log in as org B's admin. Attempt `GET /users` and assert only org B's users are returned and org A's users are absent. This verifies RLS works through the full HTTP stack, not just at the DB layer.

**E2E2-03 — Connector registration UI + API key works end-to-end (acceptance step 4)**  
Register a connector via the admin API. Use the returned API key to call `POST /v1/policy/evaluate`. Assert `200` with a valid response body. Verify the audit trail contains a `connector.auth.success` event within 5 seconds. This is the end-to-end validation of the connector auth hot path.

**E2E2-04 — SCIM provisioning saga: user transitions from initializing to active**  
Send `POST /v1/scim/v2/Users` with a new user. Assert the immediate response shows the user with `provisioning_status: initializing`. Attempt to log in as the new user: assert `401 USER_PROVISIONING_PENDING`. Poll `GET /v1/scim/v2/Users/:id` until `provisioning_status` becomes `active` (timeout: 30 seconds). Attempt login again: assert success. This maps to full-system acceptance steps 43–44.

**E2E2-05 — MFA enrollment and TOTP replay (acceptance steps 40–42)**  
As an authenticated admin, call `POST /auth/mfa/enroll`. Assert the response contains a `otpauth://` URI with the correct issuer. Generate a valid TOTP code from the secret. Call `POST /auth/mfa/verify`. Assert `200` and that `mfa_enabled` is now `true` on the user. Call `POST /auth/mfa/challenge` with the exact same TOTP code within 90 seconds. Assert `401 TOTP_REPLAY_DETECTED`.

---

### Phase 2 Acceptance Gate

All items below must pass before Phase 3 begins:

- bcrypt cost is 12 in all hashes (UT2-01)
- JWT contains jti, iat, exp, org_id, kid (UT2-02)
- JWT keyring: active key signs, verify-only key only verifies (UT2-03)
- Redis blocklist revokes jti with dynamic TTL equal to remaining token lifetime (IT2-02)
- Org A cannot see Org B's users via HTTP or SQL (E2E2-02, IT2-03)
- SELECT on `users` without set_config returns 0 rows (IT2-03)
- openguard_migrate role used for all DDL; openguard_app cannot execute DDL (IT2-03)
- User creation produces exactly one outbox record in same transaction (IT2-09)
- Outbox relay publishes IAM events to Kafka and marks records published (IT2-10)
- POST /oauth/token p99 < 150ms at 500 req/s with bcrypt worker pool (performance smoke test)
- SCIM auth derives org_id from token, ignores X-Org-ID header (IT2-08)
- version column increments on user PATCH; If-Match enforced (UT2-12)
- Login failures log with SafeAttr redaction, no raw password in logs (OBS-01 from Phase 1 applies)
- Every request carries X-Request-ID and OTel traceparent header (manual verification)

---

## Phase 3 — Policy Engine

**Spec:** `../be_open_guard/10-phase3-policy-engine.md`  
**Goal:** p99 < 30ms uncached, p99 < 5ms Redis cached. Fail closed. SDK LRU + Redis two-tier cache. Cache invalidation on policy change within 1 second.

---

### Unit Tests

**UT3-01 — Redis cache key construction is deterministic and order-independent**  
Call the cache key function with `{action: "read", resource: "/files/x", user_id: "u1", user_groups: ["admin","viewer"]}`. Call it again with groups in a different order: `["viewer","admin"]`. Assert both produce identical cache keys. Call it with a different action: assert a different key. This tests the `sha256(sorted_json(...))` requirement. If groups are not sorted before hashing, two identical logical requests produce different keys, causing unnecessary cache misses.

**UT3-02 — Cache invalidation: per-org key index is maintained on SET**  
Use a fake Redis client that records pipeline commands. Call the policy evaluator's cache-write path. Assert that the pipeline includes both a `SET` for the result key and a `SADD` to the per-org key index `policy:eval:org:{org_id}:keys`. Assert that both the result key and the index set have TTLs set. This verifies the key index design that makes bulk invalidation possible.

**UT3-03 — Cache invalidation: policy.changes event clears the org's key index**  
Pre-populate a fake Redis with a result key and the corresponding per-org index set. Trigger the `policy.changes` event handler for the same org. Assert that both the result key and the index set are deleted (UNLINK). Assert that no other org's keys are touched. This is the critical test for cache isolation across tenants.

**UT3-04 — Fail-closed: SDK returns DenyDecision after cache TTL expires with no policy service**  
Use the SDK's policy client with a fake HTTP server that always returns 503. Set the SDK cache TTL to a very short value (100ms for test purposes). Make a first evaluate call that populates the local LRU cache. Wait for the cache TTL to expire. Make a second evaluate call. Assert it returns `DenyDecision` (not an error, not a panic, and not the stale cached value). This is the most critical safety property of the entire policy engine.

**UT3-05 — SDK local cache: second identical call produces zero outbound HTTP requests**  
Configure the SDK with a real LRU cache and a fake HTTP server that counts calls. Make two identical evaluate calls in sequence. Assert the fake server received exactly one request. Assert both calls returned the same result. Assert the second call's response includes `cache_hit: "sdk"` or equivalent.

**UT3-06 — Policy evaluation: singleflight deduplicates concurrent cache-miss requests**  
Simulate 50 concurrent goroutines calling evaluate with the same cache key during a cache miss (fake Redis always returns empty). Assert the upstream policy service (fake) receives exactly 1 request, not 50. This tests the thundering-herd mitigation. Without singleflight, a cache invalidation on a hot key causes a spike that can overwhelm the policy database.

**UT3-07 — Policy eval log: cache_hit field is accurate**  
Use a fake repository. Call evaluate and record the first result. Assert the logged `cache_hit` value is `"none"`. Warm the Redis cache. Call evaluate again with identical inputs. Assert the logged `cache_hit` is `"redis"`. The `policy_eval_log` is how operators diagnose cache effectiveness; inaccurate values mislead debugging.

**UT3-08 — Policy webhook: published via outbox on policy change, not direct Kafka call**  
Use a fake outbox writer and a fake Kafka producer. Call the policy update handler. Assert the fake outbox writer received the webhook event. Assert the fake Kafka producer received zero calls. This enforces §4's absolute rule: no direct Kafka produce from business handlers.

---

### Integration Tests

**IT3-01 — End-to-end evaluation: real PostgreSQL, real Redis, real policy service**  
Seed a real policy in PostgreSQL. Call `POST /v1/policy/evaluate` against the running policy service. Assert `200` with `permitted: true/false` matching the policy logic. Query the real Redis and assert the cache key exists. Call evaluate again with identical inputs. Assert the response includes `cache_hit: "redis"`. Query the `policy_eval_log` table in PostgreSQL and assert two rows exist, one with `cache_hit: "none"` and one with `cache_hit: "redis"`.

**IT3-02 — Cache invalidation end-to-end: policy change clears Redis within 1 second**  
Warm the Redis cache with an evaluate call. Send `PUT /v1/policies/:id` to update the policy. Assert within 1 second (using `require.Eventually`) that the Redis key no longer exists. Make another evaluate call. Assert the result reflects the new policy (not the stale cached value). Assert `cache_hit: "none"` in the eval log.

**IT3-03 — Circuit breaker: policy service down causes 503, then recovers**  
Establish that the circuit breaker is closed (make a successful evaluate call). Stop the policy service container. Make additional evaluate calls until the circuit breaker opens. Assert subsequent calls return `503 POLICY_SERVICE_UNAVAILABLE` immediately (not after timeout). Restart the policy service container. Wait for the circuit breaker's `OpenDuration` to elapse. Make an evaluate call. Assert success (circuit breaker moved to half-open, probe succeeded, now closed).

**IT3-04 — Policy webhook delivery: connector receives signed webhook within 5 seconds**  
Set up a real HTTP listener (not a mock — a real `net/http` server). Register a connector with scope `policy:read` and the listener's URL as the webhook URL. Update a policy. Assert within 5 seconds that the listener received a POST request. Verify the HMAC signature on the received payload using the known webhook secret. This is a real network call from the webhook-delivery service to the test listener.

**IT3-05 — version field and ETag: concurrent updates maintain consistency**  
Create a policy. Capture the `ETag` from the response. Send two concurrent `PUT` requests with the same `If-Match` header. Assert exactly one returns `200` and one returns `412 Precondition Failed`. Query PostgreSQL and assert the `version` is exactly 2 (not 3). This verifies optimistic concurrency control works correctly under real concurrent load.

---

### End-to-End Tests

**E2E3-01 — Full-system acceptance steps 6–9: policy creation, evaluation, scope enforcement**  
Create a policy via the admin API. Evaluate against a blocked IP with a connector that has `policy:evaluate` scope: assert `permitted: false, cache_hit: none`. Evaluate again with identical inputs: assert `cache_hit: redis`. Evaluate using a connector that only has `scim:read` scope: assert `403 INSUFFICIENT_SCOPE`. This directly maps to and validates steps 6, 7, 8, and 9 of the full-system acceptance scenario.

**E2E3-02 — SDK fail-closed: circuit breaker open → local cache → TTL expiry → deny**  
Run this test against the full stack. Make an evaluate call to populate the SDK local cache. Kill the policy service. Make evaluate calls until the circuit breaker opens. Assert the SDK returns the cached decision (not a deny yet). Wait for the SDK cache TTL to expire. Make another evaluate call. Assert `DenyDecision` is returned. Restart the policy service. Assert the next evaluate call succeeds. This validates full-system acceptance steps 28–30.

---

### Phase 3 Acceptance Gate

- POST /v1/policy/evaluate p99 < 30ms uncached under 500 concurrent requests (smoke test with hey or wrk)
- POST /v1/policy/evaluate p99 < 5ms Redis cached under 500 concurrent requests
- SDK local cache: second call produces 0 outbound HTTP requests (UT3-05)
- Policy change invalidates Redis cache within 1 second (IT3-02)
- Policy change webhook delivered to connector within 5 seconds (IT3-04)
- Circuit breaker open → 503 → SDK local cache → TTL expiry → DenyDecision (E2E3-02)
- version increments on update; ETag returned; stale If-Match returns 412 (IT3-05)
- policy_eval_log records cache_hit accurately (UT3-07)
- No direct Kafka produce from policy handlers — outbox only (UT3-08)

---

## Phase 4 — Event Bus & Audit Log

**Spec:** `../be_open_guard/11-phase4-event-bus-and-audit.md`  
**Goal:** Kafka fully operational with manual-commit consumers. Bulk MongoDB writes. HMAC hash chain. CQRS read/write split. Exactly-once semantics via idempotent event_id index.

---

### Unit Tests

**UT4-01 — Hash chain: ChainHash is deterministic given same inputs**  
Call `ChainHash(secret, prevHash, event)` twice with identical arguments. Assert both outputs are identical hex strings. Change the `prevHash` field: assert a different output. Change the `event.EventID`: assert a different output. Change the `event.OrgID`: assert a different output. This verifies that tampering with any single field of the chain is detectable.

**UT4-02 — Hash chain: sequential events produce a verifiable chain**  
Compute the chain for events 1 through 5 in sequence, starting from `prevHash = "genesis"`. Assert that each event's hash is produced by feeding the previous event's hash as input. Manually replay the chain from genesis to event 5 and assert the final hash matches. This confirms the verifier algorithm used by `GET /audit/integrity` is correct.

**UT4-03 — Bulk writer: batch does not exceed AUDIT_BULK_INSERT_MAX_DOCS**  
Use a fake MongoDB client that records the batch sizes received. Feed 1,250 documents into the bulk writer with a limit of 500. Assert that exactly 3 `BulkWrite()` calls are made (two with 500 docs, one with 250). Assert no single call exceeds 500 documents. Oversized batches can exceed MongoDB's BSON document limit and cause partial failures.

**UT4-04 — Bulk writer: flush triggered by time interval, not only by document count**  
Feed 3 documents into a bulk writer with max batch size 500 and flush interval 100ms. Assert that `BulkWrite()` is called within 200ms even though the batch is not full. This verifies the time-based flush path exists and prevents unbounded buffering when event volume is low.

**UT4-05 — Offset commit contract: offset is NOT committed when BulkWrite fails**  
Use a fake Kafka consumer and fake MongoDB client where `BulkWrite()` returns an error. Process a batch through the consumer loop. Assert that the fake consumer's `CommitOffsets()` was never called. This is the single most important test in Phase 4: offset-before-write means message loss on crash; write-before-offset means duplicates on restart. Both wrong; this test enforces the correct order.

**UT4-06 — Atomic chain sequence reservation: concurrent reservations produce non-overlapping ranges**  
Use a fake MongoDB that simulates `findOneAndUpdate($inc: {seq: batchSize})` atomically. Launch 10 goroutines each requesting a batch of 50 sequences. Assert that each goroutine receives a non-overlapping range. Assert the final counter value is exactly 500. This validates the batched atomic reservation design that avoids per-event round trips to MongoDB.

**UT4-07 — Duplicate event_id: BulkWrite with ordered=false skips duplicates and succeeds**  
Construct a batch where one document has an event_id that already exists in the fake MongoDB (simulated unique index violation, error code 11000). Submit the batch with `ordered: false`. Assert the BulkWrite call does not return an error. Assert that only the new documents (not the duplicate) were inserted. Assert that offsets ARE committed after a successful ordered=false bulk write with duplicate key errors. This tests the at-least-once replay safety guarantee from §12.2.2.

**UT4-08 — Cooperative-sticky Kafka consumer assignment: rebalance does not stop unaffected partitions**  
This is a configuration test. Read the Kafka consumer configuration object. Assert `partition.assignment.strategy` is set to `cooperative-sticky`. Assert `session.timeout.ms` is 45000, `heartbeat.interval.ms` is 3000, `max.poll.interval.ms` is 300000. These settings are not testable via behaviour without a multi-broker cluster, but confirming the config is correct prevents the default eager-rebalance behaviour.

---

### Integration Tests

**IT4-01 — Full pipeline: IAM login event appears in MongoDB within p99 2 seconds**  
Trigger a login event by calling `POST /oauth/token` on the real IAM service. Start a timer. Poll MongoDB using a real MongoDB client on the secondary read preference (not primary — match the production read path) for the `auth.login.success` event with the known `event_id`. Assert it appears within 2 seconds. Failing this criterion means the Kafka → audit pipeline is too slow for real-time security monitoring.

**IT4-02 — Offset commit contract: real Kafka crash before commit causes reprocessing, no data loss**  
Seed 20 events into the Kafka `audit.trail` topic. Pause the audit consumer after its `BulkWrite()` succeeds but before its `CommitOffsets()` call (use a fault-injection hook similar to IT-03 in Phase 1). Restart the consumer. Assert all 20 events exist in MongoDB exactly once (deduplicated by `event_id` unique index). Assert the Kafka consumer lag returns to 0 after reprocessing.

**IT4-03 — CQRS read/write split: GET /audit/events uses MongoDB secondary**  
Enable MongoDB profiling on both primary and secondary nodes. Call `GET /audit/events`. Check the profiling output: assert the query appears in the secondary's profile, not the primary's. This directly verifies the `readPreference: secondaryPreferred` requirement. Violating this sends all read traffic to the primary, defeating the CQRS split.

**IT4-04 — GET /audit/integrity uses MongoDB primary, not secondary**  
Enable MongoDB profiling. Call `GET /audit/integrity`. Assert the query appears in the primary's profile. This is the EXCEPTION to the CQRS split documented in §2.4: reading integrity data from a lagging secondary would generate false-positive integrity failures.

**IT4-05 — Hash chain integrity: manually delete a document → integrity returns a gap**  
Ingest 10 events for an org, allowing the chain to build. Verify `GET /audit/integrity` returns `ok: true`. Using a direct MongoDB connection (bypassing the audit service), delete the document with `chain_seq: 5`. Call `GET /audit/integrity` again. Assert the response returns `ok: false` and identifies a gap at `chain_seq: 5`. This directly validates the deletion-detection requirement from §12.3.

**IT4-06 — Chain sequence uniqueness under concurrency: 100 events same org**  
Send 100 concurrent `POST /v1/events/ingest` requests for the same org through the full stack. Wait for all 100 to be processed into MongoDB. Query all 100 audit documents for that org. Assert all 100 have unique `chain_seq` values. Assert the chain_seq values are contiguous (no gaps). Duplicates or gaps would indicate a concurrency bug in the atomic sequence reservation.

**IT4-07 — Topic configuration: retention, partitions, and replication match topics.json**  
Connect to the real Kafka broker using an admin client. For each topic in `infra/kafka/topics.json`, query its metadata. Assert partition count, replication factor, and retention.ms match the spec. `audit.trail` must have retention.ms = -1 (infinite retention). `data.access` must have 24 partitions. These are load-related and compliance-related — wrong values silently degrade the system.

---

### End-to-End Tests

**E2E4-01 — Full-system acceptance step 10: 50 events ingested, all in audit within 5 seconds**  
Using a registered connector with `events:write` scope, send `POST /v1/events/ingest` with 50 events in a single batch. Assert `200`. Start a timer. Poll `GET /audit/events` with the connector's org context until all 50 events appear. Assert this takes less than 5 seconds. Assert each event has `event_source: "connector:<connector_id>"`. Assert the outbox had exactly 50 records created in a single transaction.

**E2E4-02 — Service crash before offset commit: no data loss, no duplicates (full stack)**  
This is the most operationally important correctness test. Kill the audit service container while it is mid-batch (after BulkWrite succeeds, before CommitOffsets). Restart the container. Wait for Kafka consumer lag to return to zero. Assert that all events that were being processed exist in MongoDB exactly once. Assert no duplicate `event_id` values exist in the collection. Maps to full-system acceptance steps 33–34.

---

### Phase 4 Acceptance Gate

- Kafka consumer processes 50,000 events/s sustained (smoke test via kafka-throughput.js)
- Bulk writer: each batch ≤ 500 docs, flush interval ≤ 1000ms (UT4-03, UT4-04)
- Kafka offsets committed only after successful MongoDB BulkWrite (UT4-05, IT4-02)
- Event from IAM login appears in MongoDB within p99 2s (IT4-01)
- Duplicate event_id: batch succeeds with ordered=false, offsets committed (UT4-07)
- Service crash before offset commit: events reprocessed, no duplicates (E2E4-02)
- GET /audit/events uses MongoDB secondary (IT4-03)
- GET /audit/integrity uses MongoDB primary (IT4-04)
- Manually deleted document → integrity check reports gap (IT4-05)
- 100 concurrent events for same org → all unique, contiguous chain_seq values (IT4-06)

---

## Phase 5 — Threat Detection & Alerting

**Spec:** `../be_open_guard/12-phase5-threat-and-alerting.md`  
**Goal:** Real-time detection with Redis counters. Six threat detectors. Composite risk scoring. Saga-based alert lifecycle. SIEM webhooks HMAC-signed and replay-protected.

---

### Unit Tests

**UT5-01 — Brute force detector: threshold triggers at exactly N failures**  
Feed 9 `auth.login.failure` events for the same email in a time window to a detector using a fake Redis counter. Assert no alert is produced. Feed the 10th event. Assert an alert with `severity: HIGH` (score ≥ 0.8) and `detector: brute_force` is produced. Feed the 10th event for a different email: assert it does not trigger for the original email's counter (no cross-user contamination).

**UT5-02 — Impossible travel detector: distance calculation is correct**  
Provide two logins: IP1 resolving to New York, IP2 resolving to Tokyo, 30 minutes apart. Assert the detector computes the distance as > 10,000 km and flags as impossible. Provide two logins from cities 200 km apart, 2 hours apart: assert no flag (physically possible). Use a fake IP geolocation lookup, not a real network call. The formula must use the Haversine calculation; test a known city pair with a known expected distance.

**UT5-03 — Privilege escalation detector: login then immediate role grant triggers alert**  
Send a `auth.login.success` event for user X at T=0 to the detector. Send a `policy.changes` event with `role.grant` for user X at T+30 minutes. Assert an alert with score ≥ 0.9 is produced. Send a role grant for user X at T+90 minutes (outside the 60-minute window): assert no alert. This tests the temporal correlation logic.

**UT5-04 — Composite scoring: max of individual scores, not sum**  
Simulate events that simultaneously trigger brute-force (score 0.8) and off-hours (score 0.5) detectors. Assert the composite score is 0.8, not 1.3. Assert the severity is HIGH (0.8 ≤ score < 0.95). A sum-based implementation would exceed 1.0 and would incorrectly escalate to CRITICAL.

**UT5-05 — SIEM webhook HMAC: computed over timestamp.payload_bytes**  
Given a known timestamp, known payload, and known secret, compute the expected HMAC manually (reference implementation). Call the webhook signing function. Assert the `X-OpenGuard-Signature` header value matches the expected value exactly. Then change one byte of the payload and assert the signature changes. This verifies the signing format from §13.3.

**UT5-06 — Replay protection: timestamp outside 300-second window is rejected**  
Call the replay-protection validator with a timestamp that is `now - 301 seconds`. Assert it returns `ErrReplayDetected`. Call with `now - 299 seconds`: assert acceptance. Call with `now + 301 seconds` (future): assert rejection. Call with `now`: assert acceptance. The boundary cases are critical — a one-second error here causes legitimate webhooks to be rejected or replayed ones to be accepted.

**UT5-07 — SSRF: webhook URLs resolving to private IP ranges are blocked**  
Call the URL validator with each of the following and assert all are rejected: `http://localhost/internal` (loopback), `http://192.168.1.1/endpoint` (RFC 1918), `http://169.254.169.254/latest/meta-data/` (link-local / AWS metadata), `http://10.0.0.1/hook` (RFC 1918), and any URL with scheme `http://` rather than `https://`. Use a fake DNS resolver that returns controlled IPs so the test does not make real DNS calls.

**UT5-08 — Alert saga steps: each step produces an audit event**  
Use a fake outbox writer. Trigger the full alert lifecycle: create → acknowledge → resolve. Assert the outbox writer received exactly 4 calls corresponding to the 4 saga steps from §13.2. Assert each call carries the correct `saga_id` (same across all steps), incrementing `saga_step`, and the correct event type for each step.

**UT5-09 — MTTR calculation: resolved alert has correct duration**  
Create an alert with `created_at: T`. Resolve it at `T + 47 minutes`. Assert the computed MTTR is approximately 2,820 seconds (47 minutes). Run this calculation in a table-driven test covering zero-minute resolution, sub-minute resolution, multi-hour resolution, and multi-day resolution.

**UT5-10 — Account takeover detector: new device within 24h of password change**  
Send a `user.password_changed` event at T=0. Send a `auth.login.success` event from device fingerprint `device-new-xyz` at T+23 hours. Assert the ATO detector fires with score ≥ 0.7. Send the same login from a device fingerprint that was seen before the password change: assert no alert. The device fingerprint history must be keyed per user in the fake Redis.

---

### Integration Tests

**IT5-01 — Brute-force: 11 failed logins → HIGH alert in MongoDB within 5 seconds**  
Using the real event ingest pipeline, send 11 `auth.login.failure` events for the same email via `POST /v1/events/ingest` on the real control plane. Start a timer. Poll `GET /v1/threats/alerts` on the real threat service. Assert a HIGH alert appears for the correct org within 5 seconds. Assert the alert document in MongoDB has the correct `detector`, `severity`, and `org_id` fields. This is full-system acceptance step 11.

**IT5-02 — SIEM webhook: delivered to real endpoint with valid HMAC**  
Set up a real `net/http` listener that records requests. Configure the org's SIEM webhook URL to point to the listener. Trigger a HIGH threat alert. Assert within 10 seconds that the listener received a POST request. Extract the `X-OpenGuard-Signature` and `X-OpenGuard-Timestamp` headers. Recompute the HMAC using the known secret. Assert the signatures match. Assert the timestamp is within 30 seconds of now. This is full-system acceptance step 13.

**IT5-03 — SIEM webhook replay protection: stale timestamp rejected**  
Intercept a delivered SIEM webhook payload. Re-send the same payload to the SIEM endpoint 6 minutes later (with the original timestamp unchanged). Assert the SIEM endpoint returns `401` or `400` with a replay rejection error. This requires the SIEM endpoint to be a real service under test (the OpenGuard SIEM receiver), not a third-party system.

**IT5-04 — SSRF: SIEM URL pointing to instance metadata endpoint rejected at configuration time**  
Attempt to configure a SIEM webhook URL of `https://169.254.169.254/latest/meta-data/`. Assert the API returns `400` with error code `SSRF_BLOCKED`. Assert no outbound HTTP request was made to that URL. The validation must happen at registration, not only at delivery.

**IT5-05 — Alert saga: all 4 steps produce audit events on real Kafka**  
Trigger a threat alert. Consume from the real `audit.trail` Kafka topic. Assert 4 events arrive with types matching the saga steps from §13.2. Assert all 4 share the same `saga_id`. Assert `saga_step` values are 1, 2, 3, 4 in order.

**IT5-06 — ATO detector fires on new device within 24h of real password change**  
Perform an actual password change via `PUT /users/:id/password`. Send a login event from a new device fingerprint via `POST /v1/events/ingest` within 1 hour. Assert a threat alert with detector `account_takeover` appears in MongoDB within 10 seconds.

---

### End-to-End Tests

**E2E5-01 — Full-system steps 11–15: threat detection, audit integrity end-to-end**  
Using the registered AcmeApp connector, send 11 failed login events. Assert a HIGH alert appears in `GET /v1/threats/alerts` within 5 seconds. Assert the SIEM mock listener received a signed webhook payload. Assert `GET /audit/events` contains all events from registration through the threat detection. Assert `GET /audit/integrity` returns `ok: true` with no chain gaps. This validates a contiguous chain of security events through the entire stack.

---

### Phase 5 Acceptance Gate

- 11 failed logins → HIGH alert in MongoDB within 5 seconds (IT5-01)
- Privilege escalation detector fires within 5 seconds of role grant (IT5-01 variant)
- SIEM webhook includes valid HMAC; receiver can verify (IT5-02)
- Stale timestamp (> 5 minutes) rejected as replay (IT5-03)
- Alert saga: all 4 steps produce audit events in audit.trail (IT5-05)
- MTTR computed correctly on resolution (UT5-09)
- ATO detector fires when login from new device follows password change within 24h (IT5-06)
- SSRF: SIEM URL pointing to 169.254.169.254 rejected at configuration time (IT5-04)

---

## Phase 6 — Compliance & Analytics

**Spec:** `../be_open_guard/13-phase6-compliance-and-analytics.md`  
**Goal:** ClickHouse receives events. Bulkhead-limited report generation. PDF output complete with all required sections. Analytics queries meet p99 < 100ms.

---

### Unit Tests

**UT6-01 — Bulkhead: injected via constructor, not package-level**  
Inspect the `Generator` struct. Assert it has a `bulkhead` field of type `*resilience.Bulkhead` (or the interface over it). Instantiate `Generator` without providing a bulkhead and assert the constructor panics with a meaningful error. This test enforces the DI requirement from §14.3 and prevents the forbidden pattern of package-level mutable state.

**UT6-02 — Bulkhead: 11 concurrent report requests, 10 succeed, 11th returns ErrBulkheadFull**  
Create a Bulkhead with concurrency limit 10. Launch 11 goroutines each calling `bulkhead.Execute()` with a slow function (fake 500ms delay). Assert exactly 10 succeed. Assert exactly 1 returns `ErrBulkheadFull`. This is tested with the real `Bulkhead` implementation, not a fake — the bulkhead itself must be correct.

**UT6-03 — ClickHouse batch: offset not committed when batch.Send() fails**  
Use a fake ClickHouse client where `batch.Send()` returns an error. Run the ClickHouse writer's flush cycle. Assert the fake Kafka consumer's `CommitOffsets()` is never called. This is the same offset-commit contract as Phase 4 but applied to the ClickHouse writer — same correctness guarantee, different sink.

**UT6-04 — FINAL modifier: compliance queries must include FINAL keyword**  
Parse the SQL strings used in the compliance report generator. Assert that every `SELECT ... FROM events` statement contains the `FINAL` keyword. This can be a string search test over the query constants file. The FINAL modifier is required for `ReplacingMergeTree` to deduplicate at query time — without it, duplicate events are counted in compliance reports.

**UT6-05 — PDF report: all 5 GDPR sections are present in generated output**  
Run the GDPR report generator against a fake data set. Assert the generated PDF bytes are non-empty and parseable as a valid PDF. Assert the text content includes all 5 required GDPR section headings. Assert a table of contents is present. Assert page numbers are present. Use a PDF parsing library, not a byte-level check, so the test is resilient to minor formatting changes.

**UT6-06 — Partition key: ClickHouse schema must not partition by org_id**  
Parse the `CREATE TABLE events` DDL string from the schema file. Assert the `PARTITION BY` clause contains `toYYYYMMDD(occurred_at)`. Assert it does NOT contain `org_id`. Partitioning by org_id with 10,000+ orgs creates too many parts and degrades ClickHouse. This is a schema correctness test.

**UT6-07 — RSA-PSS PDF signing: signature verifies against the public key**  
Generate a test RSA-4096 key pair. Sign a sample PDF using the `Signer`. Use the public key to verify the signature. Assert verification succeeds. Modify one byte of the PDF and assert verification fails. This ensures the signing implementation matches the verification path described in §14.3.

---

### Integration Tests

**IT6-01 — ClickHouse bulk insertion: 10,000 events arrive in ≤ 3 batches**  
Send 10,000 events through the real Kafka pipeline with the ClickHouse writer consuming. Instrument the writer with a counter. Assert the `PrepareBatch`/`Send` cycle runs at most 3 times (batches of ≤ 5,000 rows). After processing, query ClickHouse and assert exactly 10,000 rows exist in the `events` table (no duplicates, no drops).

**IT6-02 — Materialized view: event_counts_daily populated automatically**  
Insert a batch of events for org X on date D. Wait for ClickHouse background materialization (poll `SELECT count() FROM event_counts_daily WHERE org_id=X AND day=D` until > 0, timeout 30 seconds). Assert the count in the materialized view matches the raw event count.

**IT6-03 — Compliance stats: p99 < 100ms under load**  
Run 200 concurrent `GET /v1/compliance/stats` requests against the real compliance service backed by real ClickHouse. Capture response times. Assert the p99 is less than 100ms. This uses the materialized view — the latency guarantee is only achievable with the view, not with raw table scans.

**IT6-04 — Report generation: GDPR report completes within 60 seconds**  
Trigger a GDPR report via `POST /v1/compliance/reports`. Poll `GET /v1/compliance/reports/:id` until `status: completed` (timeout: 90 seconds). Assert completion within 60 seconds. Download the report. Assert the downloaded PDF is a valid PDF with all 5 GDPR sections. Verify the detached RSA-PSS signature against the public key.

**IT6-05 — Bulkhead: 11th concurrent report returns 429 with Retry-After header**  
Submit 11 concurrent report generation requests to the real compliance service. Assert exactly 10 return `202 Accepted`. Assert the 11th returns `429 Too Many Requests`. Assert the response includes a `Retry-After: 30` header. Assert `openguard_report_bulkhead_rejected_total` increments by 1 in Prometheus.

**IT6-06 — ClickHouse Kafka offset committed only after batch.Send() succeeds**  
Stop the ClickHouse container mid-batch (after the consumer has polled messages but before batch.Send() completes). Restart ClickHouse. Assert Kafka consumer lag returns to 0 after the writer retries. Assert all events exist in ClickHouse exactly once. This verifies the at-least-once + idempotent pattern for the ClickHouse sink.

---

### End-to-End Tests

**E2E6-01 — Full-system steps 16–18: GDPR report creation, polling, download**  
Using the full docker-compose stack and the existing audit data from earlier steps, trigger a GDPR report. Poll until completed within 60 seconds. Download and verify the PDF. Assert all 5 GDPR sections are present. Assert the RSA-PSS signature is valid. This validates steps 16, 17, and 18 of the full-system acceptance scenario.

---

### Phase 6 Acceptance Gate

- ClickHouse receives 10,000 events in ≤ 3 batches of ≤ 5,000 rows (IT6-01)
- event_counts_daily materialized view populated automatically (IT6-02)
- GET /compliance/stats p99 < 100ms under load (IT6-03)
- GDPR report: 5 sections, valid PDF with ToC and page numbers (IT6-04)
- 11 concurrent report requests: 10 succeed, 11th returns 429 with Retry-After (IT6-05)
- Bulkhead injected via constructor; package-level bulkhead causes constructor panic (UT6-01)
- Kafka offsets committed only after successful ClickHouse batch.Send() (UT6-03, IT6-06)
- ClickHouse partition by day only; no org_id partition (UT6-06)

---

## Phase 7 — Security Hardening

**Spec:** `../be_open_guard/14-phase7-security-hardening.md`  
**Goal:** HTTP security headers on every response. SSRF protection at registration and delivery time. JWT rotation zero-downtime. MFA re-encryption zero-downtime. Idempotency keys with 24-hour replay cache.

---

### Unit Tests

**UT7-01 — Security headers: all required headers present on every response**  
Using a test HTTP handler wrapped with `SecurityHeaders` middleware, make a GET request. Assert the response contains all 6 headers from §15.1: `Strict-Transport-Security` with `max-age=63072000; includeSubDomains; preload`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'none'`, `Referrer-Policy: no-referrer`, and `X-Request-ID` with a non-empty value. Make a POST request: assert the same headers are present. Make a request to an error-returning handler (returning 404): assert the same headers are still present.

**UT7-02 — SSRF: re-validation occurs at delivery time, not only at registration**  
Register a webhook URL with a valid public IP at registration time. Change the fake DNS resolver to now return `127.0.0.1` for that same hostname (simulating a DNS rebind attack). Call the delivery function. Assert delivery is rejected with `SSRF_BLOCKED`. Assert no outbound HTTP connection was made. This verifies the "re-validate at delivery time" requirement from §15.2.

**UT7-03 — SSRF: resolved IP is not cached across deliveries**  
Register a webhook URL. Deliver once with the valid IP. Change the DNS resolver to return a private IP. Deliver again. Assert the second delivery is rejected. If the first delivery's resolved IP were cached, the second delivery would incorrectly succeed. Each delivery must call the DNS resolver fresh.

**UT7-04 — Idempotency: second request with same key returns cached response**  
Use a fake Redis for the idempotency cache. Make a POST request with `Idempotency-Key: key-abc`. Assert `200` and that the response was written to Redis. Make a second POST request with the same key but a different request body. Assert the response body is identical to the first response. Assert the response includes `Idempotency-Replayed: true`. Assert the underlying handler is called only once (fake handler counter = 1).

**UT7-05 — Idempotency: cache entries > 64KB are not cached**  
Configure a fake handler that returns a 65KB response body. Make a POST with `Idempotency-Key: big-key`. Assert the response is returned normally. Assert that Redis has no entry for `big-key`. Make a second POST with the same key. Assert the handler is called again (counter = 2). This verifies the size limit from §15.5.

**UT7-06 — Idempotency: TTL on cached entries is 24 hours**  
Use a fake Redis that records TTLs on SET operations. Make an idempotent POST. Assert the Redis entry was written with a TTL of exactly 86,400 seconds. A shorter TTL allows replays; a longer one wastes Redis memory.

**UT7-07 — JWT rotation: tokens signed with verify-only key are still accepted**  
Configure the keyring with two keys: `k1` active, `k2` verify-only. Sign a token with the `k2` key directly (simulating a token issued before rotation). Call `Verify()`. Assert success. This simulates the rotation window where users have tokens signed by the old key that are still valid.

**UT7-08 — JWT rotation: tokens signed with removed key are rejected**  
Sign a token with key `k1`. Remove `k1` from the keyring entirely. Call `Verify()`. Assert `ErrTokenInvalid` (not a panic, not a nil pointer dereference). The error must be graceful — a missing kid must be a predictable failure mode.

---

### Integration Tests

**IT7-01 — Security headers present on every service's responses**  
For each service in the `allServices` map (from Phase 1 E2E tests), make a real HTTP GET to `/health/live`. Assert all 6 security headers are present. This is an integration test because it requires the real running service, not a test handler. A service that forgets to wire the `SecurityHeaders` middleware will fail here.

**IT7-02 — JWT rotation end-to-end: old tokens valid during rotation window**  
Issue a JWT with key `k1` (active). Perform a zero-downtime rotation: add key `k2` as active, set `k1` to verify-only. Deploy (restart) the IAM service. Make an authenticated request with the `k1`-signed token. Assert `200`. This simulates the real rotation procedure from §15.4.

**IT7-03 — JWT rotation: old tokens rejected after key removal**  
Continue from IT7-02. Remove key `k1` from the keyring. Restart the IAM service. Make an authenticated request with the original `k1`-signed token. Assert `401 TOKEN_INVALID`. Make an authenticated request with a fresh token (signed by `k2`). Assert `200`. This is full-system acceptance step 27.

**IT7-04 — MFA re-encryption: TOTP codes valid before and after re-encryption script**  
Enroll a TOTP user with encryption key `mk1`. Generate a valid TOTP code. Verify it succeeds. Run the `re-encrypt-mfa.sh` script (add `mk2` as active, `mk1` as verify-only, run script, remove `mk1`). Generate a fresh valid TOTP code. Verify it succeeds. The code must work because the underlying TOTP secret (after decryption with the correct key) is unchanged.

**IT7-05 — govulncheck and npm audit pass on real dependency tree**  
Run `govulncheck ./...` against the real project source. Assert exit code 0 (no vulnerabilities). Run `npm audit --audit-level=high` in the `web/` directory. Assert no HIGH or CRITICAL vulnerabilities. If either fails, the phase is blocked — dependencies must be updated before proceeding to Phase 8.

**IT7-06 — Idempotency with real Redis: replay returns identical response, handler called once**  
With a real Redis container, make two `POST /v1/compliance/reports` requests with the same `Idempotency-Key` header. Assert the report job ID in both responses is identical. Assert only one report job row exists in PostgreSQL (handler executed once). Assert the second response includes the `Idempotency-Replayed: true` header.

---

### End-to-End Tests

**E2E7-01 — Full-system steps 26–27: JWT rotation zero-downtime**  
Issue tokens from the running system. Add a new JWT key. Assert old tokens still work. Remove the old key. Assert old tokens are rejected. Assert new tokens (signed with the new key) work. This runs against the full docker-compose stack and validates production rotation procedure.

**E2E7-02 — Connector suspension: Redis cache invalidated immediately (acceptance step 45)**  
Register a connector. Warm the connector auth cache (make a successful API call). Suspend the connector via `PATCH /v1/admin/connectors/:id {status: "suspended"}`. Immediately (within 1 second) make another API call with the connector's key. Assert `403 CONNECTOR_SUSPENDED`. Assert the Redis sentinel key was written during suspension.

---

### Phase 7 Acceptance Gate

- Security headers on every response from every service (IT7-01)
- SSRF: connector webhook URL `http://localhost/internal` rejected at registration (UT7-02, IT from phase 5)
- SSRF: SIEM URL `http://169.254.169.254/...` rejected at startup (Phase 5 IT5-04)
- SafeAttr: log entry containing password=secret123 shows [REDACTED] (Phase 1 OBS-01)
- JWT rotation: old tokens accepted during rotation window, rejected after key removal (IT7-02, IT7-03)
- MFA re-encryption: TOTP codes valid before and after re-encryption (IT7-04)
- go mod verify and govulncheck pass (IT7-05)
- npm audit --audit-level=high reports zero issues (IT7-05)
- Idempotency: same key twice → second response identical, handler called once (IT7-06)
- Idempotency replay cache entry > 64KB is not cached; next request re-executes (UT7-05)

---

## Phase 8 — Load Testing & SLO Verification

**Spec:** `../be_open_guard/15-phase8-load-testing.md`  
**Goal:** Every SLO from §1.2 verified under production-equivalent load. k6 reports committed. Grafana screenshots committed.

---

### Test Philosophy for Phase 8

Phase 8 tests are **performance tests**, not correctness tests. The system correctness was proven in Phases 1–7. Phase 8 proves the system can meet its performance contract at production load. Every k6 script produces a pass/fail based on the thresholds defined in the k6 `options.thresholds` object. A threshold violation blocks Phase 9.

Phase 8 tests require a pre-seeded dataset: 10,000 users across 100 orgs, with policies, connectors, and audit history. The seed script is `scripts/seed-loadtest.sh` and must be idempotent.

---

### Load Test Cases

**LT8-01 — `auth.js`: OIDC token endpoint throughput and latency**  
Ramp from 0 to 500 VUs over 1 minute. Hold at 2,000 VUs for 3 minutes. Ramp down over 1 minute. Each VU calls `POST /oauth/token` with a unique pre-seeded username/password pair. Threshold: p99 < 150ms. Error rate < 1%. This requires the bcrypt worker pool to be sized to `2 × NumCPU`. If the IAM service runs with too few replicas, it will fail the p99 threshold due to bcrypt queuing.

**LT8-02 — `policy-evaluate.js` scenario A: Redis cache hits at 10,000 req/s**  
All VUs send identical `(action, resource, user_id, user_groups)` tuples, guaranteeing cache hits. Threshold: p99 < 5ms. This tests Redis performance and network latency only — the database and evaluation logic are bypassed.

**LT8-03 — `policy-evaluate.js` scenario B: cache misses at 10,000 req/s**  
Each VU uses a unique `resource` value, guaranteeing cache misses on every request. Threshold: p99 < 30ms. This tests the full evaluation path: Redis lookup → PostgreSQL query (with indexes) → RBAC evaluation → Redis write → response.

**LT8-04 — SDK local cache verification via distributed tracing**  
Each VU calls evaluate twice with identical inputs, 1 second apart (within the SDK LRU TTL). After the test, query Jaeger for traces from the SDK to the control plane. Assert that the second call from each VU produced no span to the control plane (only the first call's span exists). SDK local cache hits must be invisible to the server — this is the only way to verify the SDK cache without modifying the SDK client under test.

**LT8-05 — `event-ingest.js`: connector event push throughput**  
Each VU sends `POST /v1/events/ingest` with a batch of 10 events. Total target: 20,000 events/s (2,000 req/s × 10 events/req). Threshold: p99 < 50ms. After the test, wait 10 seconds and assert all ingested events appear in the MongoDB audit log. Missing events indicate the outbox or Kafka pipeline dropped events under load.

**LT8-06 — `audit-query.js`: read path performance**  
1,000 VUs each call `GET /audit/events` with varying filter combinations (event type, actor, date range). Threshold: p99 < 100ms. This test directly verifies that MongoDB secondary reads with the compound indexes are fast enough. A miss indicates a missing or unused index.

**LT8-07 — `kafka-throughput.js`: event bus capacity**  
Using the `xk6-kafka` extension, produce 50,000 events/s directly to `audit.trail` for 5 minutes. Monitor `openguard_kafka_consumer_lag` via Prometheus. Assert consumer lag remains below 10,000 events throughout. Assert lag returns to 0 within 60 seconds of the producer stopping. A growing lag that does not recover indicates the bulk writer cannot keep up.

**LT8-08 — Connector auth Redis cached: p99 < 5ms at 20,000 req/s**  
Send 20,000 connector-authenticated requests per second, each hitting the Redis cache (same connector key, cache already warm). Threshold: p99 < 5ms. This is the hot path for all event ingestion; violating this threshold means the connector auth cache is the bottleneck.

---

### Tuning Verification Tests (Run After Each Failed Load Test)

When a load test threshold is missed, the tuning table from §16.2 specifies the root cause and action. After each tuning action, re-run the specific load test. Document the before/after result in `loadtest/results/`. The following tuning verifications are required:

**TV8-01** — If login p99 > 150ms: add IAM replicas and re-run LT8-01. Assert improvement proportional to added CPU capacity.

**TV8-02** — If policy p99 > 30ms (uncached): run `explain()` on the policy evaluation query. Assert an index exists on `(org_id, resource, action)`. If absent, add the index and re-run LT8-03.

**TV8-03** — If audit query p99 > 100ms: run `explain()` on the MongoDB query. Assert the query uses the compound index `{org_id: 1, occurred_at: -1}`. If absent, add and re-run LT8-06.

---

### Deliverables

Every load test must produce and commit the following artifacts:

- `loadtest/results/<test-name>-<date>.html` — k6 HTML summary report
- `docs/screenshots/<test-name>-grafana-<date>.png` — Grafana dashboard screenshot showing the SLO metrics under load
- `loadtest/results/<test-name>-<date>-pass.txt` or `fail.txt` — one-line summary of pass/fail against threshold

---

### Phase 8 Acceptance Gate

- auth.js: p99 < 150ms at 2,000 req/s, error rate < 1% (LT8-01)
- policy-evaluate.js: p99 < 5ms (cached), p99 < 30ms (uncached) at 10,000 req/s (LT8-02, LT8-03)
- SDK local cache: second call produces 0 spans to policy service, verified via Jaeger (LT8-04)
- event-ingest.js: p99 < 50ms at 20,000 req/s; all events in audit within 5s after test (LT8-05)
- audit-query.js: p99 < 100ms at 1,000 req/s (LT8-06)
- Kafka consumer lag < 10,000 during 50,000 events/s burst (LT8-07)
- Connector auth p99 < 5ms cached at 20,000 req/s (LT8-08)
- All k6 HTML reports committed to loadtest/results/ (deliverable)
- Grafana screenshots showing all SLOs met under load committed to docs/ (deliverable)

---

## Phase 9 — Documentation Quality Gates

**Spec:** `../be_open_guard/16-phase9-documentation.md`  
**Goal:** README, architecture docs, runbooks, and OpenAPI specs are complete, accurate, and verifiable. A new contributor can run the system from scratch using only `README.md`.

---

### Test Philosophy for Phase 9

Phase 9 tests are **documentation correctness tests**. They verify that documentation is accurate (not just present), runnable (not just written), and machine-verifiable where possible. Documentation that exists but is wrong is worse than no documentation.

---

### Documentation Tests

**DT9-01 — make dev on a clean machine: verified by CI ephemeral environment**  
Run the full `make dev` sequence in a GitHub Actions runner with a fresh OS image (no cached Docker layers, no pre-installed tools). Assert it completes without error. Assert all services are healthy afterwards. Assert the time to reach a healthy state is under 10 minutes. This is the literal verification of the "< 5 minutes" quick-start claim in `README.md` (CI environments are slower; 10 minutes is the CI-adjusted threshold).

**DT9-02 — OpenAPI specs: all services pass redocly lint**  
For each service, run `redocly lint docs/api/<service>.openapi.json`. Assert exit code 0. Assert no warnings (use `--fail-on-warn` flag). A service with an invalid OpenAPI spec cannot be used by SDK generator tools or API documentation portals.

**DT9-03 — OpenAPI specs: every HTTP endpoint is documented**  
For each service, parse the OpenAPI spec and extract all documented paths. Parse the service's router to extract all registered routes. Assert every registered route appears in the OpenAPI spec. Assert no extra paths exist in the OpenAPI spec that do not exist in the router (stale documentation). This test catches the common drift between code and docs.

**DT9-04 — Mermaid diagrams render in GitHub Markdown**  
For each `.md` file containing a ```` ```mermaid ```` block, extract the diagram content and run it through the Mermaid CLI (`mmdc`). Assert exit code 0 (valid Mermaid syntax, no rendering error). A broken diagram is useless to the on-call engineer.

**DT9-05 — Architecture diagram accuracy: all 10 services appear in C4 diagram**  
Parse `docs/architecture.md`. Assert all 10 microservice names appear in the C4 component diagram text. Assert the diagram shows connected apps calling OpenGuard (not traffic flowing through OpenGuard — the control plane model must be accurate per the project description).

**DT9-06 — Runbooks: all 10 runbooks exist and are non-trivial**  
For each runbook filename listed in §17.2, assert the file exists in `docs/runbooks/`. Assert each file has more than 200 words (non-trivial content). Assert each runbook contains the headings: "Symptoms", "Diagnosis", and "Resolution" (standard operational runbook format). A runbook with only a title and one sentence is not useful at 3 AM.

**DT9-07 — Contributing guide: adding a new detector produces a passing test**  
Follow the steps in `docs/contributing.md` for "Adding a new threat detector". Implement a minimal stub detector as described. Run `go test ./services/threat/...`. Assert the tests pass. If the contributing guide's steps produce a non-compilable or test-failing result, the guide is wrong. This test runs the guide mechanically in CI.

**DT9-08 — Contributing guide: adding a new control plane route produces correct scope enforcement**  
Follow the steps in `docs/contributing.md` for "Adding a new control plane route". Add a stub route with a test scope. Run the integration tests for the control plane. Assert that requests without the required scope receive `403 INSUFFICIENT_SCOPE`. Assert that requests with the correct scope receive `200`. If following the guide does not produce correct scope enforcement, the guide is incorrect.

**DT9-09 — SLO table accuracy: README SLO values match Phase 8 results**  
Parse the SLO table from `README.md`. For each SLO, compare the documented p99 value against the committed k6 HTML report from Phase 8. Assert that the README values are not more optimistic than the measured results. A README that claims p99 < 30ms when the Phase 8 result was p99 = 28ms is fine; one claiming p99 < 5ms when the result was 28ms is misleading and blocked.

---

### Phase 9 Acceptance Gate

- `make dev` works on a clean machine following only README.md (DT9-01)
- All OpenAPI specs pass `redocly lint` with no warnings (DT9-02)
- Every HTTP endpoint is documented in the OpenAPI spec (DT9-03)
- All Mermaid diagrams render without error via mmdc (DT9-04)
- All 10 runbooks present and non-trivial (DT9-06)
- Following contributing guide: new detector produces passing test (DT9-07)
- Following contributing guide: new route produces correct scope enforcement (DT9-08)

---

## Phase 10 — DLP

**Spec:** `../be_open_guard/17-phase10-dlp.md`  
**Goal:** PII, credential, and financial data detection. Sync block mode (dlp_mode=block) fails closed when DLP service is unavailable. Async monitor mode masks fields in MongoDB within 5 seconds. Entropy-based credential detection.

---

### Unit Tests

**UT10-01 — Regex scanner: email detection in JSON payloads**  
Run the scanner against a JSON payload containing a valid email in a nested field. Assert `finding_type: "pii"`, `finding_kind: "email"`, and the `json_path` pointing to the correct field. Run against a payload with no email. Assert no findings. Run against a payload with a malformed email address (missing domain): assert no finding (RFC 5322 simplified pattern, not a false positive generator).

**UT10-02 — Regex scanner: SSN detection with correct pattern**  
Provide the string `"123-45-6789"` in a payload field. Assert the scanner returns a finding with `finding_kind: "ssn"`. Provide `"1234-56-7890"` (too many digits): assert no finding. Provide `"123-456-7890"` (phone number pattern): assert no finding for SSN. The pattern must be `\b\d{3}-\d{2}-\d{4}\b`, not a looser regex.

**UT10-03 — Credit card scanner: Luhn validation filters false positives**  
Provide a valid Visa card number that passes the Luhn check. Assert a finding with `finding_kind: "credit_card"`. Provide a random 16-digit string that fails the Luhn check: assert no finding. Provide a valid Luhn-passing number that is 15 digits (not Visa or MC format): assert behavior matches the spec (AmEx 15-digit support must be explicitly tested). Luhn is necessary to avoid flagging random 16-digit strings in payloads (product IDs, order numbers, etc.).

**UT10-04 — Entropy scanner: detects AWS access key format**  
Pass the string `"AKIAIOSFODNN7EXAMPLE"` to the entropy scanner. Assert a finding with `finding_kind: "high_entropy"`. The known prefix `AKIA` should trigger an immediate flag regardless of entropy score. Pass a UUID string (`"550e8400-e29b-41d4-a716-446655440000"`): assert no finding (UUIDs are in the false-positive exclusion list). Pass a random 24-character base64 string with high entropy: assert a finding. Pass a 23-character string (below `DLP_MIN_CREDENTIAL_LENGTH`): assert no finding even if entropy is high.

**UT10-05 — Known credential prefixes: immediate flag without entropy calculation**  
For each known prefix (`sk_live_`, `sk_test_`, `AIza`, `AKIA`, `ghp_`, `github_pat_`, `xoxb-`, `xoxp-`), construct a plausible token with that prefix and sufficient length. Assert a finding for each. Assert the entropy calculation is not the deciding factor — a token with prefix `sk_live_` and low-entropy suffix should still be flagged. This ensures that even poorly generated API keys from these providers are detected.

**UT10-06 — Sync block mode: finding causes event rejection, no outbox write**  
Configure a fake policy resolver that returns `dlp_mode: block` for the org. Configure the DLP scanner to find a credit card. Call the ingest handler. Assert the response is `422 DLP_POLICY_VIOLATION`. Assert the fake outbox writer received zero calls. The event must not be written to the outbox if DLP blocks it. Writing to the outbox and then masking would allow a brief window where the unmasked event exists in Kafka — unacceptable in block mode.

**UT10-07 — Monitor mode: event accepted, finding produced asynchronously**  
Configure `dlp_mode: monitor` for the org. Feed an event with an SSN through the ingest handler. Assert the response is `200`. Assert the outbox writer received the event (it was accepted). The DLP scan happens asynchronously; this unit test only verifies the synchronous accept path. The async mask is tested in the integration test.

**UT10-08 — Fail closed in block mode: DLP service unavailable → reject event**  
Configure `dlp_mode: block`. Configure the DLP service client to always return a connection error. Call the ingest handler. Assert `503 DLP_UNAVAILABLE`. Assert the outbox writer received zero calls. This is the fail-closed requirement. Accepting an event when DLP is unreachable in block mode defeats the entire purpose of block mode.

**UT10-09 — DLP finding triggers HIGH alert for credential finding type**  
Use a fake alert publisher. Submit a finding with `finding_type: "credential"`. Assert the alert publisher was called with `severity: HIGH`. Submit a finding with `finding_type: "pii"`: assert no automatic alert (PII findings go to the audit log, not automatically to alerts). The distinction matters: credentials in event payloads indicate active compromise; PII indicates a data governance issue.

**UT10-10 — json_path accuracy: masking targets the correct field**  
Provide a deeply nested JSON payload. Run the scanner. Assert the `json_path` in the finding points exactly to the leaf field containing the sensitive value (e.g., `$.payload.form_data.social_security`). Assert the path resolves correctly when used in a MongoDB update command to replace the field value with `"[REDACTED:ssn]"`. A wrong `json_path` masks the wrong field or fails to mask anything.

---

### Integration Tests

**IT10-01 — Sync block: real DLP service, credit card in payload → 422**  
Send `POST /v1/events/ingest` with a payload containing a valid Visa credit card number to a connector in an org with `dlp_mode: block`. Assert `422 DLP_POLICY_VIOLATION`. Query PostgreSQL: assert zero outbox records were written. Query MongoDB: assert zero audit events for this request. The event must have been completely rejected before any persistence.

**IT10-02 — Sync block with DLP service down → 503, not 200**  
Stop the DLP service container. Send `POST /v1/events/ingest` with a credit card payload to an org with `dlp_mode: block`. Assert `503 DLP_UNAVAILABLE`. Assert no outbox record exists. This test verifies the circuit breaker around the sync DLP call is fail-closed, not fail-open.

**IT10-03 — Monitor mode: SSN detected, field masked in MongoDB within 5 seconds**  
Send `POST /v1/events/ingest` with a payload containing an SSN (`"123-45-6789"`) to an org with `dlp_mode: monitor`. Assert `200` (event accepted). Start a timer. Poll the audit event in MongoDB. Assert within 5 seconds that the SSN field has been replaced with `"[REDACTED:ssn]"`. Assert the `dlp_findings` PostgreSQL table has a row for this event with the correct `json_path`, `finding_kind: "ssn"`, and `action_taken: "mask"`. This is full-system acceptance step 19.

**IT10-04 — DLP finding metric: openguard_dlp_findings_total increments per finding**  
Send events containing different finding types through the monitor pipeline. Query Prometheus for `openguard_dlp_findings_total{type="pii"}` and `openguard_dlp_findings_total{type="credential"}`. Assert each counter has incremented by the expected number of findings. Metrics that are declared but not incremented provide false confidence in observability coverage.

**IT10-05 — DLP HPA: minimum 3 replicas when any org has dlp_mode=block**  
Set one org's DLP mode to `block`. Query the Kubernetes API for the DLP `HorizontalPodAutoscaler`. Assert `minReplicas` is at least 3. Set all orgs back to `monitor`. Assert the HPA's minimum decreases. This test requires the system to be deployed to a real Kubernetes cluster (or `kind`/`minikube` in CI) and verifies the Helm chart configuration is applied correctly.

**IT10-06 — RLS: dlp_findings are org-scoped**  
Create DLP findings for org A and org B. Query `dlp_findings` as `openguard_app` with org A's RLS context. Assert only org A's findings are returned. Query with org B's context. Assert only org B's findings. Unset `app.org_id`: assert zero findings returned. This applies the same RLS correctness verification from Phase 1/2 to the DLP tables.

---

### End-to-End Tests

**E2E10-01 — Full-system step 19: SSN in monitor mode → accepted → masked within 5 seconds**  
Using the full docker-compose stack, send an event containing an SSN through the AcmeApp connector (org configured for monitor mode). Assert `200`. Poll the audit log. Assert the SSN field is masked within 5 seconds. Assert a DLP finding record exists in PostgreSQL with the correct json_path. Assert a `dlp.finding.created` event appears in the `audit.trail` Kafka topic.

**E2E10-02 — Block mode end-to-end: credit card rejected before any persistence**  
Configure an org for `dlp_mode: block`. Send a credit card number via event ingest. Assert `422`. Assert the event does not appear anywhere in the audit log (MongoDB), outbox (PostgreSQL), or Kafka. The event must be rejected before touching any persistent store.

**E2E10-03 — DLP → threat alert pipeline: credential finding raises HIGH alert**  
Send a payload containing a GitHub PAT (`ghp_xxxx...`) to a monitor-mode org. Wait for the DLP scan to complete (5 seconds). Assert a `dlp.finding.created` event with `finding_type: "credential"` appears in the audit trail. Assert a HIGH threat alert appears in `GET /v1/threats/alerts` within 10 seconds. The alert pipeline from DLP findings must be fully wired.

---

### Phase 10 Acceptance Gate

- Regex scanner identifies email and SSN in JSON payloads (UT10-01, UT10-02)
- Luhn scanner identifies valid Visa credit card numbers; ignores random digit strings (UT10-03)
- Entropy scanner detects AKIAIOSFODNN7EXAMPLE (AWS access key) (UT10-04)
- Known prefixes immediately flagged without relying on entropy threshold (UT10-05)
- Sync block: POST /v1/events/ingest with credit card → 422 DLP_POLICY_VIOLATION (IT10-01)
- Sync block with DLP service down → 503, not 200 (IT10-02)
- Monitor mode: event accepted → SSN detected → audit log field masked within 5s (IT10-03, E2E10-01)
- DLP finding auto-creates HIGH threat alert for credential finding type (UT10-09, E2E10-03)
- openguard_dlp_findings_total metric incremented per finding (IT10-04)
- dlp_findings table is RLS-scoped; org A cannot see org B's findings (IT10-06)

---

## Full-System Acceptance Scenario (45 Steps)

**Spec:** `../be_open_guard/20-full-system-acceptance-criteria.md`  
**Run:** On every release candidate, as a single CI job. All 45 steps must pass without manual intervention for the release pipeline to publish.

---

### Scenario Flow Description

The full-system scenario runs as a single sequential Go test (or a bash script calling real API endpoints) against the full docker-compose stack. It is not a collection of independent tests — it is a single narrative that builds state from step 1 to step 45. A failure at step 15 blocks all remaining steps.

**Steps 1–5: Environment bootstrap and identity**  
Bring up all services. Verify health. Register org "Acme" with admin user in a single transaction. Obtain a JWT with the expected `kid` header field. Register two connectors with different scopes. The key invariant: the one-time API key returned for each connector must be stored securely by the test harness, as it cannot be retrieved again.

**Steps 6–9: Policy engine core path**  
Create an IP allowlist policy. Evaluate against a blocked IP and verify `permitted: false, cache_hit: none`. Evaluate again with identical inputs and verify `cache_hit: redis`. Evaluate using the wrong-scope connector and verify `403 INSUFFICIENT_SCOPE`. These four assertions together verify the evaluation correctness, caching behavior, and scope enforcement in a single continuous narrative.

**Steps 10–13: Event ingestion and threat detection**  
Ingest 50 events. Verify all 50 appear in the audit log within 5 seconds with correct `event_source` labels. Simulate 11 failed logins via event ingest. Assert a HIGH alert appears within 5 seconds. Verify the SIEM mock listener received the webhook and validate its HMAC signature. These steps verify the end-to-end pipeline from connector push to threat response.

**Steps 14–15: Audit trail completeness and integrity**  
Retrieve all audit events and assert they include every event from steps 2–11. Verify the hash chain integrity check returns `ok: true`. These steps act as a global correctness assertion: every state change in the system must have produced an audit event, and the chain must be unbroken.

**Steps 16–18: Compliance reporting**  
Trigger a GDPR report. Poll until completed within 60 seconds. Download and assert the PDF has all 5 GDPR sections. This validates the full async job pipeline and PDF output.

**Step 19: DLP monitor mode**  
Send an event with a plaintext SSN. Assert acceptance. Assert the SSN is masked in the audit log within 5 seconds. This validates the async DLP pipeline.

**Steps 20–23: Connector lifecycle**  
Suspend the second connector. Assert the connector cache is invalidated immediately (next request with its key returns `403`). Send a test webhook. Verify the delivery log shows both the test webhook and the earlier policy-change webhook. These steps validate the connector management lifecycle end-to-end.

**Steps 24–25: Session security**  
Perform a valid token refresh. Assert a new token is issued and the old refresh token is invalidated. Perform a high-risk refresh (UA family change from same device). Assert `401 SESSION_REVOKED_RISK`. These steps validate the risk-based session protection.

**Steps 26–27: JWT key rotation zero-downtime**  
Add a new JWT key and deploy (restart IAM). Assert old tokens still work. Remove the old key and redeploy. Assert old tokens now fail with `401`. Assert new tokens work. These steps validate the rotation runbook in production conditions.

**Steps 28–30: Policy engine fail-closed**  
Kill the policy service. Verify the SDK falls back to its local cache and continues returning decisions. Wait for the SDK cache TTL to expire. Assert the SDK returns `DenyDecision`. Restart the policy service. Assert the circuit breaker recovers and evaluate succeeds. This is the most critical safety verification in the entire scenario.

**Steps 31–34: Kafka failure and recovery**  
Kill Kafka. Ingest an event — assert it succeeds (outbox absorbs the write). Restart Kafka. Assert the outbox relay publishes the pending record within 30 seconds. Kill the audit consumer before it commits offsets. Restart it. Assert events are reprocessed without duplicates. These steps validate the durability of the event pipeline under infrastructure failure.

**Step 35: Race detector**  
Run `go test ./... -race` against the full test suite while the docker-compose stack is running. Assert all tests pass and zero data races are detected. Running the race detector in the context of a live system catches races that only manifest under real concurrent load.

**Steps 36–38: SLO spot-check under load**  
Run the k6 auth and policy-evaluate scripts. Assert p99 thresholds are met. Verify via Jaeger that SDK second calls produce zero spans to the policy service. These are the Phase 8 SLOs re-verified in the full system context.

**Step 39: Clean shutdown**  
Call `docker compose down`. Assert all containers exit cleanly (exit code 0). Assert no data corruption in PostgreSQL or MongoDB (run integrity checks post-shutdown). Assert all in-flight events that were in the outbox at shutdown are present and in `pending` or `published` state (nothing was lost).

**Steps 40–42: MFA enrollment and replay protection**  
Enroll TOTP for the admin user. Verify a valid code. Attempt to verify the same code a second time within 90 seconds. Assert `401 TOTP_REPLAY_DETECTED`. These steps validate the TOTP implementation including the anti-replay Redis set.

**Steps 43–44: SCIM provisioning saga**  
SCIM-provision a new user. Assert `provisioning_status: initializing`. Assert login is rejected while provisioning is in progress. Wait for saga completion. Assert `provisioning_status: active`. Assert login now succeeds.

**Step 45: Connector suspension and cache sentinel key**  
Suspend the first connector. Assert the Redis sentinel key is written immediately. Assert cached auth hits return `CONNECTOR_SUSPENDED` without going to the database.

---

### Full-System Acceptance Gate

The release pipeline must not publish until all 45 steps pass in a single CI run. If any step fails, the run is marked failed and the step number is reported in the CI output. The gates from all previous phases remain in effect — the full-system scenario is additive, not a replacement.

---

## Task Management App Integration Tests

**Goal:** Verify end-to-end integration constraints between the Simple Task Management App and the OpenGuard centralized security control plane.

### End-to-End Tests

**E2E-TM-01 — Task Management end-to-end auth flow**  
Launch the OpenGuard stack and the Task Management frontend/backend. Navigate to the Task Management frontend and initiate login. Assert redirection to OpenGuard OIDC. Authenticate using pre-seeded test credentials. Assert redirection back to the Task Application containing a valid JWT. Call the Task creation API with the JWT. Assert successful task creation corresponding to the authenticated user.

**E2E-TM-02 — Task Management policy integration fail-closed**  
With the user logged in, call the Task deletion API endpoint. Assert the Go backend evaluates the deletion policy via the OpenGuard SDK. Shut down the OpenGuard policy service. Wait for the local SDK cache to expire. Call the Task deletion API again. Assert the Go backend returns `403 Forbidden` demonstrating fail-closed behavior when policy evaluation fails.

**E2E-TM-03 — Task Management audit ingestion**  
Create, update, and delete different tasks via the Task Management application. Poll OpenGuard's `GET /audit/events` using an administrative API Key. Assert the presence of `task.created`, `task.updated`, and `task.deleted` audit events mapped correctly to the logged-in user's identity and showing the Task Management Backend as the source.

---

> **Claude Code reminder:** After implementing all tests through Phase 10, run the
> full-system acceptance scenario end-to-end. All 45 steps must pass in a single
> uninterrupted CI run before the system is considered production-ready. Record
> the run's output in `docs/acceptance-run-<date>.txt` and commit it with the
> release tag.
