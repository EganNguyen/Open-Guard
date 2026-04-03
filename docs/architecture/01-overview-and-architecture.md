# §1–2 — Project Overview & Architecture Principles

---

## 1. Project Overview

### 1.1 What is OpenGuard?

OpenGuard is an open-source, self-hostable **centralized security control plane**. Connected applications register with OpenGuard and integrate via a lightweight SDK, SCIM 2.0, OIDC/SAML, and outbound webhooks. User traffic never flows *through* OpenGuard — it is a governance hub, not a proxy.

It operates at Fortune-500 scale: 100,000+ users, 10,000+ organizations, millions of audit events per day, cryptographic audit trail integrity, zero cross-tenant data leakage, and sub-100ms policy evaluation at p99.

**Core capabilities:**
- **Identity & Access Management:** OIDC/SAML IdP. SSO, SCIM 2.0, TOTP/WebAuthn MFA, API token lifecycle, session management.
- **Policy Engine:** Real-time RBAC evaluation via SDK. Fails closed. SDK caches decisions locally for up to 60 seconds during control plane unavailability.
- **Connector Registry:** Connected applications register and receive org-scoped API credentials. Credentials stored with PBKDF2 hash at rest; hot-path lookup uses a fast-hash prefix scheme against Redis.
- **Event Ingestion:** Connected apps push audit events to `POST /v1/events/ingest`. Events are normalized into the same Kafka-backed audit pipeline as internal events.
- **Threat Detection:** Streaming anomaly scoring — brute force, impossible travel, off-hours access, account takeover, privilege escalation.
- **Audit Log:** Append-only, HMAC hash-chained, cryptographically verifiable event trail with configurable retention.
- **Alerting & Webhooks:** Rule-based and ML-scored alerts with SIEM export and signed outbound webhook delivery.
- **Compliance Reporting:** GDPR, SOC 2, HIPAA report generation with PDF output.
- **Content Scanning / DLP:** Real-time PII, credential, and financial data detection.
- **Admin Dashboard:** Next.js 14 web console.

### 1.2 Performance Targets (Canonical SLOs)

These are hard targets. Phase 8 must verify each one with k6 load tests. A phase is not complete until its SLOs are met.

| Operation | p50 | p99 | p999 | Throughput |
|-----------|-----|-----|------|------------|
| `POST /oauth/token` (IAM OIDC) | 40ms | 150ms | 400ms | 2,000 req/s |
| `POST /v1/policy/evaluate` (uncached) | 5ms | 30ms | 80ms | 10,000 req/s |
| `POST /v1/policy/evaluate` (Redis cached) | 1ms | 5ms | 15ms | 10,000 req/s |
| SDK local cache hit (no network) | <1ms | <1ms | <1ms | unlimited |
| `GET /audit/events` (paginated) | 20ms | 100ms | 250ms | 1,000 req/s |
| Kafka event → audit DB insert | — | 2s | 5s | 50,000 events/s |
| Compliance report generation | — | 30s | 120s | 10 concurrent |
| `POST /v1/events/ingest` (connector push) | 10ms | 50ms | 150ms | 20,000 req/s |
| `GET /v1/scim/v2/Users` | 30ms | 500ms | 1,500ms | 500 req/s |
| Connector registry lookup (Redis cached) | 1ms | 5ms | 15ms | — |
| DLP async scan latency | — | 500ms | 2s | — |

**Notes on SLO feasibility:**
- `POST /oauth/token` at 150ms p99 requires bcrypt to run through a bounded worker pool (§8.2). At cost 12, bcrypt takes 250–400ms per operation. Without pooling, 2,000 req/s would require ~800 goroutines each blocking on bcrypt, causing CPU starvation. With a worker pool sized to `2 × NumCPU`, throughput is limited by CPU capacity; scale horizontally to meet the target.
- **Bcrypt capacity model:** At cost 12, one CPU core processes ~2.86 logins/s (1/350ms). A pool of `2N` workers on an N-core node sustains approximately `5.7N` logins/second. Meeting the **2,000 req/s** SLO therefore requires ~**35 CPU cores** of IAM capacity (e.g., 6 pods × 6 cores = 36 cores). Engineers running a single pod will see 429s at ~150 req/s — this is correct behavior, not a bug. Add IAM HPA scaling targets to the Helm chart with `targetCPUUtilizationPercentage: 60`.
- `POST /v1/events/ingest` at 50ms p99 at 20,000 req/s assumes backpressure shedding kicks in before PostgreSQL outbox writes become the bottleneck (§8.6).

### 1.3 Design Principles

| Principle | Implementation |
|-----------|---------------|
| **Fail closed** | Policy unavailable → SDK denies after 60s cache TTL. IAM unavailable → reject all logins. DLP sync-block unavailable → reject events (per-org opt-in). |
| **Exactly-once audit** | Every state-changing operation produces exactly one audit event via the Transactional Outbox. Connected-app events deduplicated by `event_id` (unique index, scoped to retention window). |
| **Zero cross-tenant leakage** | PostgreSQL RLS enforced at the DB layer. RLS policy uses `NULLIF(current_setting('app.org_id', true), '')::UUID` to handle NULL and empty string uniformly. |
| **Immutable audit trail** | Append-only MongoDB with per-org HMAC hash chaining. Batch chain assignment for throughput (§11.2.3). |
| **Least privilege (services)** | Each service has its own DB user with table-level grants. Migration runs as `openguard_migrate` (DDL only, no `BYPASSRLS` on data tables). |
| **Secret rotation without downtime** | JWT signing uses `kid`. Multiple valid keys coexist during rotation. Same pattern for MFA encryption keys. |
| **Access token revocation** | JWT `jti` claim. Validation order: (1) verify signature, (2) check `exp` and reject `ErrTokenExpired` if expired, (3) check Redis blocklist. Blocklist entries MUST have a dynamic TTL equal to the token's remaining lifetime (`exp - now()`). |
| **Clock skew tolerance** | JWT `iat` must not be more than 5 minutes in the future. Refresh tokens arriving before their issued-at time are rejected with `ErrClockSkew`. Maximum accepted backward skew is zero. |
| **mTLS between services** | All internal service-to-service calls use mTLS. |
| **Exactly-once Kafka delivery** | Idempotent Kafka producer. Consumer commits offsets only after successful downstream write. |
| **Cache-first connector auth** | Fast-hash prefix → Redis; PBKDF2 only on cache miss → DB. Sustains 20,000 req/s event ingest. |

---

## 2. Architecture Principles

### 2.1 The Dual-Write Problem

The root cause of most audit trail gaps in security systems:

```go
// WRONG — process crash between these two lines = permanent audit gap
db.Exec("INSERT INTO users ...")
kafka.Publish("audit.trail", event)
```

**The fix:** The Transactional Outbox Pattern (§6). The business row and the event record are committed atomically in the same PostgreSQL transaction. A separate relay process reads committed outbox records and publishes to Kafka.

```go
// CORRECT — atomic: both succeed or both fail
tx.Exec("INSERT INTO users ...")
tx.Exec("INSERT INTO outbox_records ...")
tx.Commit()
// Relay publishes asynchronously — no Kafka in the write path
```

### 2.2 Kafka Consumer Offset Commit Contract

This rule is non-negotiable. Every Kafka consumer uses **manual offset commit mode**. An offset is committed only after the downstream write (MongoDB, ClickHouse, Redis, or PostgreSQL) has been confirmed.

```
Consumer reads message
  → Process (write to MongoDB, ClickHouse, etc.)
    → On success: commit offset
    → On failure: do NOT commit, retry or route to DLQ
```

The consequence: during bulk writes, if a batch of 500 documents is submitted to MongoDB but the service crashes before committing offsets, those 500 messages are reprocessed on restart. The `event_id` unique index on MongoDB `audit_events` and the Kafka idempotent producer together make this safe.

### 2.3 Multi-Tenancy Isolation

Three isolation tiers:

| Tier | Mechanism | Plan |
|------|-----------|------|
| **Shared** | PostgreSQL RLS on shared tables | Free / SMB |
| **Schema** | Dedicated PostgreSQL schema per org | Mid-market |
| **Shard** | Dedicated PostgreSQL instance per org | Enterprise / regulated |

This spec fully implements **Shared** (RLS) and scaffolds Schema/Shard as extension points. All application code is written RLS-first. Every tenant table has an explicit `org_id UUID NOT NULL` column. The RLS policy always compares against this column — never against the Kafka partition key or any other proxy.

### 2.4 CQRS and Read/Write Split

**MongoDB write path** (Kafka consumer → primary): Bulk insert up to 500 documents or 1-second flush interval. Offsets committed after successful `BulkWrite()`. Chain sequence assigned via batched atomic reservation (§11.2.3).

**MongoDB read path** (HTTP handlers → secondary): `readPreference: secondaryPreferred`. Compliance report queries use `readPreference: secondary` (acceptable staleness: 5s).

**EXCEPTION — `GET /audit/integrity`:** MUST use `readPreference: primary`. Reading from a lagging secondary would generate false positive integrity failures.

### 2.5 Choreography-Based Sagas

User provisioning via SCIM touches multiple services. OpenGuard uses choreography-based sagas via Kafka compensating events. Each step is idempotent and publishes the next step's trigger.

**Saga State Machine**: A new SCIM user is created with `status: initializing`. IAM MUST reject logins for any user in this state.

**User Status Transition Table:**

| From | To | Trigger |
|---|---|---|
| (new) | `initializing` | SCIM `POST /Users` |
| `initializing` | `active` | `saga.completed` event consumed by IAM |
| `initializing` | `provisioning_failed` | `saga.timed_out` or compensation event |
| `provisioning_failed` | `initializing` | Admin `POST /users/:id/reprovision` |
| `active` | `suspended` | Admin `POST /users/:id/suspend` |
| `suspended` | `active` | Admin `POST /users/:id/activate` |
| any | `deprovisioned` | SCIM `DELETE /Users/:id` |

**`provisioning_failed` is not terminal.** Org admins can retry via `POST /users/:id/reprovision`. The SCIM `POST /Users` endpoint MUST be idempotent on `scim_external_id`: if a user with the same external ID exists in any non-`deprovisioned` status, return the existing resource (200) rather than 409.

**Saga timeout:** Set deadline to `SAGA_STEP_TIMEOUT_SECONDS + OUTBOX_MAX_LAG_SECONDS` (defaults: 30s + 10s = 40s). When `user.created` is published, IAM writes a deadline record to Redis: `ZADD saga:deadlines <unix_deadline> <saga_id>`. A background watcher daemon (in IAM, consumer group `openguard-saga-v1`) polls every 10 seconds for expired entries.

**SCIM `POST /scim/v2/Users` saga:**

```
IAM:        user.created (status=initializing)   → audit.trail + saga.orchestration
Policy:     [consumes user.created]              → assigns default org policies
            policy.assigned                      → audit.trail
Threat:     [consumes policy.assigned]           → initializes baseline profile
            threat.baseline.init                 → audit.trail
Alerting:   [consumes threat.baseline.init]      → configures notification preferences
            alert.prefs.init                     → audit.trail
IAM:        [consumes alert.prefs.init]          → UPDATE users SET status = 'active'
            saga.completed                       → audit.trail
```

**Compensation (any step failure or timeout):**

```
Policy:     policy.assignment.failed (compensation:true, caused_by: <event_id>)
IAM:        [consumes policy.assignment.failed] → sets user status=provisioning_failed
            user.provisioning.failed → audit.trail
Threat:     [consumes user.provisioning.failed] → removes baseline profile
Alerting:   [consumes user.provisioning.failed] → removes notification preferences
```

**`user.deleted` saga** (SCIM `DELETE /Users/:id`) — includes immediate session revocation:

```
IAM:        [receives DELETE /scim/v2/Users/:id]
            1. Fetch all active jti values for user_id from sessions table
            2. PIPELINE: SETEX jti:{jti} <remaining_ttl> "revoked" for each active jti
            3. UPDATE sessions SET status='revoked' WHERE user_id = $1
            4. UPDATE users SET status='deprovisioned'
            user.deleted (status=deprovisioned) → audit.trail
Policy:     [consumes user.deleted] → removes org policy assignments
Threat:     [consumes user.deleted] → archives baseline profile
Alerting:   [consumes user.deleted] → removes notification preferences
```

### 2.6 App Registration and Credential Flow

**Key scheme:**
- **Prefix** (first 8 chars, non-secret): `base62(random_bytes(8))`. Used as the Redis cache lookup key via `SHA-256(prefix)`. O(microseconds).
- **Secret** (remaining chars): verified against stored PBKDF2 hash only on cache miss. ~400ms, rare.

```
Admin       → POST /v1/admin/connectors           (JWT auth)
            ← { connector_id, api_key_plaintext }  (one-time; never stored)

ConnectedApp → POST /v1/events/ingest             (Bearer api_key_plaintext)

Control Plane auth flow:
  1. Parse key: prefix = key[0:8], secret = key[8:]
  2. fastHash = SHA-256(prefix)
  3. Lookup Redis: GET "connector:fasthash:{fastHash}"
     → Cache hit: deserialize ConnectedApp; verify secret matches stored PBKDF2 hash
       (full PBKDF2 verify only if last_verified_at > 5min ago; else trust cache)
     → Cache miss: PBKDF2-HMAC-SHA512(key, salt, 600000) → lookup DB by pbkdf2_hash
       → on hit: SET in Redis with 30s TTL
  4. Check status == "active"
  5. rls.WithOrgID(ctx, connector.OrgID)
  6. withConnectorScopes(ctx, connector.Scopes)
```

**Cache invalidation on suspension:** Update the Redis key to a "suspended" sentinel rather than deleting it, so cached lookups correctly reflect suspension without bypassing it.

**Connector reactivation:** `PATCH /v1/admin/connectors/:id {status:"active"}` MUST overwrite the Redis key with the full connector record immediately (not just delete).

**Outbound webhook delivery:** Reads from `TopicWebhookDelivery`, signs with HMAC-SHA256, POSTs to connector URL. All requests MUST include `X-OpenGuard-Delivery` UUID header. Delivery state persisted in `webhook_deliveries` (PostgreSQL). After `WEBHOOK_MAX_ATTEMPTS` exhaustion → moved to `webhook.dlq`.

### 2.8 SCIM Authentication

SCIM provisioning callers authenticate with a per-org SCIM bearer token. **The org_id is derived from the token configuration, not from any client-supplied header.**

```go
// shared/middleware/scim.go
type SCIMToken struct {
    Token string `json:"token"`
    OrgID string `json:"org_id"`
}

func SCIMAuthMiddleware(tokens []SCIMToken) func(http.Handler) http.Handler {
    tokenMap := make(map[string]string, len(tokens))
    for _, t := range tokens {
        tokenMap[t.Token] = t.OrgID
    }
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            orgID, ok := tokenMap[raw]
            if !ok {
                writeError(w, http.StatusUnauthorized, "INVALID_SCIM_TOKEN", "invalid SCIM bearer token", r)
                return
            }
            ctx := rls.WithOrgID(r.Context(), orgID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### 2.9 Certificate Rotation

**Procedure (zero-downtime):**
1. Generate new cert: `scripts/gen-mtls-certs.sh --service <name> --renew`.
2. Update the target service's cert/key mounts. CA cert does not change.
3. Rolling deploy target service. Old and new pods share the same CA → mTLS succeeds across both.
4. CA rotation uses a dual-CA trust period documented in `docs/runbooks/ca-rotation.md`.

### 2.10 Connection Pooling Targets

| Service | DB | Pool min | Pool max | Max conn lifetime |
|---------|----|----------|----------|-------------------|
| IAM | PostgreSQL | 5 | 25 | 1800s |
| Control Plane | PostgreSQL (outbox only) | 2 | 15 | 300s |
| Outbox Relay | PostgreSQL (`openguard_outbox` role) | 2 | 10 | **60s** |
| Connector Registry | PostgreSQL | 2 | 10 | 1800s |
| Policy | PostgreSQL | 2 | 15 | 1800s |
| Audit (write) | MongoDB | 2 | 10 | — |
| Audit (read) | MongoDB | 5 | 30 | — |
| Compliance | ClickHouse | 2 | 8 | — |
| All services | Redis | 5 | 20 | — |

> **Note on Outbox Relay pool lifetime:** Set to 60s so that after a PostgreSQL primary failover, stale connections are recycled within 60 seconds and the relay resumes draining promptly.

### 2.11 Tenant Offboarding

Triggered by `org.offboard` event:

1. IAM: Revoke all active sessions and API tokens. Set users to `status=deprovisioned`.
2. Control Plane: Suspend all connectors. Invalidate their Redis cache entries.
3. Policy: Archive all policies.
4. Webhook Delivery: Drain in-flight queue.
5. Audit: Finalize hash chain; write `org.offboarded` terminal event.
6. Compliance: Queue GDPR erasure export if requested.
7. Scheduler: After `ORG_DATA_RETENTION_DAYS`, hard-delete in this exact order (respects FK constraints):
   `outbox_records` → `dlp_findings` → `dlp_policies` → ... → `orgs`.

### 2.12 API Versioning Policy

All routes are prefixed `/v1/`. When a breaking change is required:
1. Implement `/v2/` route alongside `/v1/`.
2. Add `Deprecation: true` and `Sunset: <date>` headers to `/v1/` responses.
3. Maintain `/v1/` for a minimum of 6 months after `/v2/` GA.
