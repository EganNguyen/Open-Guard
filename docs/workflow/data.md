# Data Stores — Complete Reference

All persistent and transient data stores across every Open-Guard microservice: PostgreSQL, Redis, MongoDB, ClickHouse, S3, and Kafka topics.

## Service Data Store Matrix

| Service | PostgreSQL | MongoDB | Redis | ClickHouse | S3 | Kafka Producer | Kafka Consumer |
|---------|-----------|---------|-------|------------|-----|----------------|----------------|
| **IAM** | 10 tables | — | 7 key patterns | — | — | `saga.orchestration` | `saga.orchestration` |
| **Policy** | 4 tables | — | 3 key patterns | — | — | `policy.changes` | — |
| **Audit** | — | 2 collections | 2 key patterns | — | — | `audit.trail` | 7 topics |
| **Threat** | — | 1 collection | 16 key patterns | — | — | `threat.alerts` | `auth.events`, `policy.changes`, `data.access` |
| **Alerting** | — | 1 collection | 1 key pattern | — | — | `notifications.outbound`, `audit.trail` | `threat.alerts` |
| **Compliance** | 1 table | — | 1 key pattern | 2 tables + 1 MV | 1 bucket | — | `audit.trail` |
| **DLP** | 2 tables | — | 1 key pattern | — | — | `dlp.dlq` | `control.plane.events` |
| **Webhook** | 1 table | — | — | — | — | `webhook.dlq` | `webhook.delivery` |
| **Control Plane** | — | — | — | — | — | `control.plane.events` | — |

---

## Topology Overview

```
                      ┌─────────────────────────────────────────────────────────────────────────────┐
                      │                              EVENT SOURCES                                  │
                      │                                                                             │
                      │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐ │
                      │  │   IAM    │  │  Policy  │  │  Audit   │  │  Threat  │  │ Control Plane│ │
                      │  │ Service  │  │ Service  │  │ Service  │  │ Service  │  │              │ │
                      │  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬───────┘ │
                      │       │              │             │             │                │         │
                      │       │  Outbox      │  Outbox     │             │                │         │
                      │       ▼              ▼             ▼             ▼                ▼         │
                      │  ┌──────────────────────────────────────────────────────────────────────┐ │
                      │  │                           KAFKA EVENT BUS                           │ │
                      │  │  10+ topics, up to 24 partitions each                              │ │
                      │  └────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬──────────┘ │
                      │       │     │     │     │     │     │     │     │     │     │            │
                      └───────┼─────┼─────┼─────┼─────┼─────┼─────┼─────┼─────┼─────┼────────────┘
                              │     │     │     │     │     │     │     │     │     │
          ┌───────────────────┼─────┼─────┼─────┼─────┼─────┼─────┼─────┼─────┼─────┼──────────────┐
          │                   ▼     ▼     ▼     ▼     ▼     ▼     ▼     ▼     ▼     ▼              │
          │         ┌────────────────────────────────────────────────────────────────────────────┐ │
          │         │                         SERVICE CONSUMERS                                  │ │
          │         │                                                                             │ │
          │         │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐         │ │
          │         │  │  Audit   │ │  Threat  │ │ Alerting │ │Compliance│ │  DLP/    │         │ │
          │         │  │  (7topics)│ │ (3topics)│ │ (1topic) │ │(1topic)  │ │ Webhook  │         │ │
          │         │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘         │ │
          │         └────────────────────┼────────────┼────────────┼────────────┼─────────────────┘ │
          │                              │            │            │            │                   │
          │                   ┌──────────┘            │            │            └──────────┐        │
          │                   ▼                       ▼            ▼                       ▼        │
          │         ┌────────────────┐     ┌────────────────┐ ┌──────────┐     ┌──────────────────┐│
          │         │    MONGODB     │     │    REDIS       │ │CLICKHOUSE│     │   POSTGRESQL     ││
          │         │                │     │                │ │          │     │                  ││
          │         │ openguard_audit│     │  Detector: 17  │ │ events   │     │ openguard (shared)││
          │         │   audit_events │     │  keys          │ │ (2yr TTL)│     │  10 IAM tables   ││
          │         │   hash_chains  │     │  Blocklist: 5  │ │ event_   │     │  4 Policy tables ││
          │         │                │     │  keys          │ │ counts_  │     │  1 Compliance    ││
          │         │ threats        │     │  Rate-limit: 1 │ │ daily MV │     │  2 DLP tables    ││
          │         │   alerts       │     │  keys          │ │ alert_   │     │  1 Webhook table ││
          │         │                │     │                │ │ stats    │     │                  ││
          │         │ alerting       │     │                │ │          │     │ openguard_dlp    ││
          │         │   alerts       │     │                │ │          │     │  2 DLP tables    ││
          │         └────────────────┘     └────────────────┘ └──────────┘     └──────────────────┘│
          └────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## PostgreSQL

**Database:** `openguard` (shared across IAM, Policy, Compliance, Webhook), `openguard_dlp` (DLP service)

All services use separate migration directories under `services/<name>/migrations/`.

### IAM Service — 10 Tables

Database: `openguard` | RLS: enabled on all tenant-scoped tables

| # | Table | Key Columns | Purpose |
|---|-------|-------------|---------|
| 1 | `orgs` | `id UUID PK`, `name`, `slug UNIQUE`, `status`, `tier_isolation`, `created_at`, `updated_at` | Organization tenants |
| 2 | `users` | `id UUID PK`, `org_id FK→orgs`, `email UNIQUE(org)`, `password_hash`, `role`, `status`, `mfa_enabled`, `failed_login_count`, `locked_until`, `last_login_ip`, `version`, `deleted_at` | User accounts & auth state |
| 3 | `sessions` | `id UUID PK`, `org_id`, `user_id`, `jti UNIQUE`, `user_agent`, `ip_address`, `revoked`, `expires_at` | Active JWT sessions |
| 4 | `api_tokens` | `id UUID PK`, `org_id`, `user_id`, `name`, `token_hash`, `token_prefix UNIQUE`, `scopes[]`, `revoked`, `expires_at`, `last_used_at` | API keys (prefix for lookup, hash for validation) |
| 5 | `mfa_configs` | `id UUID PK`, `org_id`, `user_id`, `mfa_type` (totp/webauthn), `secret_encrypted` (AES-GCM), `backup_code_hashes[]` (HMAC-SHA256), `webauthn_id`, `webauthn_public_key`, `sign_count` | MFA device registrations |
| 6 | `outbox_records` | `id UUID PK`, `org_id`, `topic TEXT` (e.g.`auth.events`), `key`, `payload BYTEA`, `status` (pending/published/dead), `attempts`, `last_error`, `dead_at`, `published_at`, `created_at` | Transactional outbox → Kafka relay |
| 7 | `refresh_tokens` | `id UUID PK`, `org_id`, `user_id`, `token_hash UNIQUE`, `family_id`, `expires_at`, `revoked` | Refresh token rotation (family = rotation chain) |
| 8 | `connectors` | `id TEXT PK`, `org_id`, `name`, `client_secret`, `redirect_uris[]`, `api_key_prefix UNIQUE`, `api_key_hash UNIQUE` (PBKDF2) | OAuth2 connector credentials |
| 9 | `webauthn_credentials` | `id UUID PK`, `org_id`, `user_id`, `credential_id`, `public_key`, `attestation_type`, `sign_count` | WebAuthn passkey devices |
| 10 | `saml_providers` | `id UUID PK`, `org_id UNIQUE`, `entity_id`, `sso_url`, `slo_url`, `metadata_xml`, `sp_cert_pem`, `sp_key_pem`, `attribute_map JSONB`, `enabled` | SAML SSO provider config (1 per org) |

**Key Indexes:**
- `users`: `idx_users_org_id`, `idx_users_email` (UNIQUE org+email), `idx_users_scim_id` (partial), `idx_users_status` (partial)
- `sessions`: `idx_sessions_jti` (UNIQUE), `idx_sessions_user_id`
- `api_tokens`: `idx_api_tokens_prefix` (UNIQUE)
- `mfa_configs`: UNIQUE(user_id, mfa_type)
- `outbox_records`: `idx_outbox_pending` (partial WHERE status='pending')
- `refresh_tokens`: `idx_refresh_tokens_token_hash` (UNIQUE), `idx_refresh_tokens_family_id`
- `webauthn_credentials`: UNIQUE(user_id, credential_id)

**Outbox Trigger:** `AFTER INSERT ON outbox_records → pg_notify('outbox_new', ...)`

**DB Roles:**
- `openguard_app` — CRUD on user tables (RLS-scoped to own org)
- `openguard_login` — SELECT on users, orgs (login flow)
- `openguard_outbox` — SELECT, UPDATE, DELETE on outbox_records (bypasses RLS)

---

### Policy Service — 4 Tables

Database: `openguard` | RLS: enabled on tenant tables

| # | Table | Key Columns | Purpose |
|---|-------|-------------|---------|
| 1 | `policies` | `id UUID PK`, `org_id`, `name`, `description`, `logic JSONB` (CEL/RBAC), `version`, `created_at`, `updated_at` | Policy definitions |
| 2 | `policy_assignments` | `id UUID PK`, `org_id`, `policy_id FK→policies CASCADE`, `subject_id UUID`, `subject_type` (user/group) | Subject-to-policy bindings |
| 3 | `policy_eval_log` | `id UUID PK`, `org_id`, `subject_id`, `action`, `resource`, `effect` (allow/deny), `matched_policy_ids[]`, `cache_hit` (none/redis/sdk), `latency_ms`, `evaluated_at` | Evaluation audit trail |
| 4 | `outbox_records` | `id UUID PK`, `org_id`, `topic TEXT` (e.g.`policy.changes`), `key`, `payload JSONB` (vs BYTEA in IAM), `status`, `attempts`, `last_error`, `dead_at`, `published_at`, `created_at` | Transactional outbox → Kafka relay |

**Key Indexes:**
- `policies`: `idx_policies_org_id`
- `policy_assignments`: `idx_assignments_subject` (subject_id, subject_type), `idx_assignments_org_id`
- `policy_eval_log`: `idx_policy_eval_log_org_id`, `idx_policy_eval_log_evaluated_at DESC`
- `outbox_records`: `idx_outbox_status` (partial WHERE status='pending'), `idx_outbox_org_id`

**Triggers:** `outbox_insert_notify` (AFTER INSERT → `pg_notify('outbox_new', ...)`)

---

### Compliance Service — 1 Table

Database: `openguard`

| # | Table | Key Columns | Purpose |
|---|-------|-------------|---------|
| 1 | `reports` | `id UUID PK`, `org_id`, `framework` CHECK(gdpr,soc2,hipaa), `status` CHECK(pending,generating,ready,failed), `s3_key`, `s3_sig_key`, `error_msg`, `created_at`, `updated_at` | Compliance report metadata |

**Index:** `idx_reports_org_id` (org_id, created_at DESC)

---

### DLP Service — 2 Tables

Database: `openguard_dlp` (separate DB)

| # | Table | Key Columns | Purpose |
|---|-------|-------------|---------|
| 1 | `dlp_policies` | `id UUID PK`, `org_id TEXT`, `name`, `rules TEXT[]` (email, ssn, credit_card, etc.), `action` (audit/block/mask), `enabled` | DLP scanning policies |
| 2 | `dlp_findings` | `id UUID PK`, `org_id TEXT`, `event_id`, `policy_id FK→dlp_policies`, `finding_type`, `action`, `confidence FLOAT8`, `matched_field`, `redacted_value` (always `REDACTED`), `created_at` | DLP scan results |

---

### Webhook Delivery Service — 1 Table

Database: `openguard`

| # | Table | Key Columns | Purpose |
|---|-------|-------------|---------|
| 1 | `webhook_deliveries` | `id UUID PK`, `org_id`, `connector_id`, `event_id`, `target_url`, `payload JSONB`, `attempts`, `status` (pending/delivered/failed/dlq), `last_error`, `next_retry_at`, `created_at`, `updated_at` | Webhook delivery tracking |

---

## MongoDB

Three separate databases across services.

### Audit Service — `openguard_audit`

#### Collection: `audit_events`

CQRS split: Primary (majority concern) for writes, SecondaryPreferred for reads.

| Field | Type | Notes |
|-------|------|-------|
| `event_id` | string | Unique, sparse index |
| `org_id` | string | Tenant filter |
| `type` | string | Event classifier |
| `actor_id` | string | Who performed the action |
| `actor_type` | string | user / system / connector |
| `source` | string | Origin service |
| `timestamp` | datetime | Sorted DESC |
| `sequence` | int64 | Monotonically increasing per org |
| `integrity_hash` | string | HMAC-SHA256 chain link |
| `payload` | arbitrary BSON | Free-form event data |

**Watched by:** Change stream → SSE endpoint (real-time event push)

#### Collection: `hash_chains`

One document per org. Stores the latest hash chain head for integrity verification.

| Field | Type | Notes |
|-------|------|-------|
| `org_id` | string | Unique |
| `hash` | string | Latest chain head (HMAC-SHA256) |
| `sequence` | int64 | Monotonically increasing counter |
| `created_at` | datetime | — |
| `updated_at` | datetime | CAS-updated on each append |

---

### Threat Service — `threats`

#### Collection: `alerts`

| Field | Type | Notes |
|-------|------|-------|
| `_id` | ObjectID | Auto-generated, cursor pagination |
| `org_id` | string | Required |
| `user_id` | string | Required |
| `detector` | string | `brute_force`, `impossible_travel`, `off_hours_access`, `data_exfiltration`, `account_takeover`, `privilege_escalation` |
| `score` | float64 | 0.0–1.0 |
| `severity` | string | `MEDIUM` / `HIGH` / `CRITICAL` |
| `status` | string | `open` → `acknowledged` → `resolved` |
| `created_at` | datetime | Auto-set |
| `resolved_at` | *datetime | Nullable |
| `mttr_seconds` | *int64 | Nullable, computed on resolve |
| `metadata` | object | Detector-specific geo, IP, device, etc. |

**Query patterns:** `{org_id, status?, severity?, _id: {$lt: cursor}}` sorted `{_id: -1}`

---

### Alerting Service — `alerting`

#### Collection: `alerts`

| Field | Type | Notes |
|-------|------|-------|
| `_id` | string | Cross-reference: hex of threat service's ObjectID |
| `org_id` | string | Required |
| `type` | string | Alert type |
| `severity` | string | `LOW` / `MEDIUM` / `HIGH` / `CRITICAL` |
| `status` | string | `open` / `acknowledged` / `resolved` |
| `risk_score` | float64 | 0.0–1.0 |
| `detector_id` | string | Detector name |
| `raw_event` | bson.M | Original threat alert payload |
| `saga_steps` | []SagaStep | Execution trace array |
| `created_at` | datetime | — |
| `ack_at` | *datetime | Nullable |
| `resolved_at` | *datetime | Nullable |
| `mttr_seconds` | float64 | Computed on resolve |

#### SagaStep sub-document

| Field | Type | Notes |
|-------|------|-------|
| `step` | string | `persist` / `notify` / `siem` / `audit` |
| `status` | string | `completed` / `failed` |
| `error` | string | Error detail on failure |
| `at` | datetime | Execution timestamp |
| `retries` | int | Retry attempt count |

---

## Redis

### Shared Keys (All Services)

| Key Pattern | Type | TTL | Writer | Reader | Services |
|------------|------|-----|--------|--------|----------|
| `blocklist:{jti}` | STRING | Remaining token lifetime | IAM (logout) | JWT middleware circuit-breaker | IAM, Policy, Audit, Threat, Alerting, Compliance, DLP |

### IAM Service — 7 Key Patterns

| Key Pattern | Type | TTL | Purpose | Writer | Reader |
|------------|------|-----|---------|--------|--------|
| `mfa_challenge:{challengeToken}` | STRING (JSON) | 5 min | MFA challenge → user mapping | Login handler | VerifyMFA handler |
| `blocklist:{jti}` | STRING | Until token expiry | JWT revocation | Logout handler | Auth middleware |
| `auth_code:{code}` | STRING (JSON) | 10 min | OAuth2 authorization code (PKCE) | StoreAuthCode | GetAuthCode |
| `webauthn:reg:{userID}:{sessionID}` | STRING (JSON) | 5 min | WebAuthn registration session | BeginWebAuthnRegistration | FinishWebAuthnRegistration |
| `webauthn:login:{userID}:{sessionID}` | STRING (JSON) | 5 min | WebAuthn login session | BeginWebAuthnLogin | FinishWebAuthnLogin |
| `totp:used:{userID}:{code}` | STRING (NX) | 90 sec | TOTP nonce deduplication | VerifyTOTP | VerifyTOTP |
| `migrate:lock:iam` | STRING | Acquired during migration | Distributed migration lock | main.go | main.go |

### Policy Service — 3 Key Patterns

| Key Pattern | Type | TTL | Purpose | Writer | Reader |
|------------|------|-----|---------|--------|--------|
| `policy:eval:{orgID}:{sha256(request)}` | STRING (JSON) | 60 sec | Evaluation result cache (stale-while-revalidate) | evaluateFromDB | Evaluate |
| `policy:index:{orgID}` | SET | 24 h | Org-level cache index (bulk invalidation) | evaluateFromDB | Cache invalidation |
| `migrate:lock:policy` | STRING | Acquired | Distributed migration lock | main.go | main.go |

### Audit Service — 2 Key Patterns

| Key Pattern | Type | TTL | Purpose | Writer | Reader |
|------------|------|-----|---------|--------|--------|
| `blocklist:{jti}` | STRING | Token lifetime | JWT revocation | Logout | Auth middleware |
| `ratelimit:{ip}` | ZSET | 1 min | Sliding window rate limiter | Rate limit middleware | Rate limit middleware |

### Threat Service — 16 Key Patterns

#### Brute Force Detector

| Key Pattern | Type | TTL | Writer | Reader |
|------------|------|-----|--------|--------|
| `bruteforce:ip:{ip}` | ZSET (epoch ms) | 5 min | trackFailedAttempt | trackFailedAttempt (ZCard) |
| `bruteforce:user:{email}` | ZSET (epoch ms) | 5 min | trackFailedAttempt | trackFailedAttempt (ZCard) |
| `alert_fired:bruteforce:ip:{ip}` | STRING (NX) | 5 min | trackFailedAttempt | trackFailedAttempt (Exists) |
| `alert_fired:bruteforce:user:{email}` | STRING (NX) | 5 min | trackFailedAttempt | trackFailedAttempt (Exists) |
| `threat:bruteforce:ip:{ip}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |
| `threat:bruteforce:user:{email}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |

#### Impossible Travel Detector

| Key Pattern | Type | TTL | Writer | Reader |
|------------|------|-----|--------|--------|
| `travel:{userID}` | STRING (JSON) | 1 h | Lua GETSET | Lua GETSET (returns old value) |
| `threat:travel:{userID}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |

#### Off-Hours Detector

| Key Pattern | Type | TTL | Writer | Reader |
|------------|------|-----|--------|--------|
| `offhours:{orgID}:{userID}:{YYYY-MM-DD}` | STRING (`"1"`) | 7 d | processEvent (in-hours) | processEvent (last 3 days) |
| `threat:offhours:{orgID}:{userID}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |

#### Data Exfiltration Detector

| Key Pattern | Type | TTL | Writer | Reader |
|------------|------|-----|--------|--------|
| `access:{orgID}:{userID}` | ZSET (epoch ms) | 1 h | processEvent pipeline | processEvent (ZCard) |
| `baseline:{orgID}:access_mean` | STRING (float) | None | External/computed | processEvent (3-sigma) |
| `baseline:{orgID}:access_stddev` | STRING (float) | None | External/computed | processEvent (3-sigma) |
| `threat:exfiltration:{orgID}:{userID}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |

#### Account Takeover Detector

| Key Pattern | Type | TTL | Writer | Reader |
|------------|------|-----|--------|--------|
| `ato:pwchange:{userID}` | STRING (`"1"`) | 24 h | processEvent | processEvent (Exists) |
| `ato:devices:{userID}` | SET (SHA-256) | 30 d | processEvent (SAdd) | processEvent (SIsMember) |
| `threat:ato:{userID}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |

#### Privilege Escalation Detector

| Key Pattern | Type | TTL | Writer | Reader |
|------------|------|-----|--------|--------|
| `privsec:login:{userID}` | STRING (`"1"`) | 1 h | consumeAuth (login success) | consumePolicy (role grant) |
| `threat:privesc:{actorID}` | STRING (JSON) | 24 h | publishThreatEvent | Legacy cache |

### Saga Watcher (IAM) — Shared Redis Keys

| Key Pattern | Type | TTL | Purpose |
|------------|------|-----|---------|
| `saga:deadlines` | ZSET | None (managed) | Saga timeout deadlines (polled every 10s) |

---

## ClickHouse

**Database:** `default` (configurable via `CLICKHOUSE_DB`)

### Table: `events`

**Engine:** `ReplacingMergeTree(occurred_at)` | **Partition:** `toYYYYMMDD(occurred_at)` | **Order:** `(org_id, type, occurred_at, event_id)` | **TTL:** `occurred_at + INTERVAL 2 YEAR`

| Column | Type | Codec | Notes |
|--------|------|-------|-------|
| `event_id` | String | ZSTD(3) | — |
| `type` | LowCardinality(String) | — | e.g. `auth.login.success`, `threat.alert.created` |
| `org_id` | String | ZSTD(3) | Partition key |
| `actor_id` | String | ZSTD(3) | — |
| `actor_type` | LowCardinality(String) | — | user / system / detector |
| `occurred_at` | DateTime64(3, 'UTC') | — | Event timestamp |
| `source` | LowCardinality(String) | — | Origin service |
| `payload` | String | ZSTD(3) | Full event JSON |

**Written by:** Compliance ClickHouseWriter (consumer of `audit.trail` Kafka topic)
**Read by:** Compliance `GetPosture()`, `GetStats()` APIs

### Materialized View: `event_counts_daily`

**Engine:** `SummingMergeTree()` | **Partition:** `toYYYYMM(day)` | **Order:** `(org_id, type, day)`

```sql
SELECT org_id, type, toDate(occurred_at) AS day, count() AS cnt
FROM events GROUP BY org_id, type, day
```

### Table: `alert_stats`

**Engine:** `SummingMergeTree()` | **Order:** `(org_id, day, severity)`
**Note:** Schema exists; no write code implemented yet.

| Column | Type |
|--------|------|
| `org_id` | String |
| `day` | Date |
| `severity` | LowCardinality(String) |
| `count` | UInt64 |
| `mttr_seconds` | UInt64 |

---

## S3 / MinIO Object Storage

**Bucket:** `compliance-reports` (configurable via `S3_BUCKET`)

| Object Pattern | Purpose | Uploaded By | Retrieved By |
|---------------|---------|-------------|--------------|
| `reports/{reportID}.pdf` | Compliance report (GDPR/SOC2/HIPAA) | Compliance worker | Presigned URL via API |
| `reports/{reportID}.sig` | Report signature for integrity verification | Compliance worker | Presigned URL via API |

**Access:** Pre-signed URLs only (no public access)

---

## Kafka — Topic Registry

### Configuration

| Parameter | Value |
|-----------|-------|
| `RequiredAcks` | `RequireAll` (all ISR replicas) |
| `Async` | false (synchronous publish) |
| `BatchSize` | 1 (no batching) |
| `BatchTimeout` | 0 (no delay) |
| `AllowAutoTopicCreation` | false |

### Topics

```
                     ┌──────────────────────────────────────────────────────────────┐
                     │                        PRODUCERS                              │
                     │                                                               │
                     │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐        │
                     │  │   IAM    │ │  Policy  │ │  Threat  │ │ Alerting │        │
                     │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘        │
                     │       │            │            │            │              │
                     │       │ saga.      │ policy.    │ threat.    │ notifications│
                     │       │ orchest.   │ changes    │ alerts     │ .outbound    │
                     │       │            │            │            │ +audit.trail │
                     ▼       ▼            ▼            ▼            ▼              │
               ┌──────────────────────────────────────────────────────────────────┐ │
               │                      KAFKA EVENT BUS                            │ │
               │                                                                  │ │
               │  auth.events(12p)  │  policy.changes(6p)  │  data.access(24p)   │ │
               │  threat.alerts(12p)│  audit.trail(24p)    │  saga.orchestration │ │
               │  notifications     │  webhook.delivery    │  control.plane.events│ │
               │  .outbound         │                      │                      │ │
               └──────────────────────────────────────────────────────────────────┘ │
                     │           │            │            │            │          │
                     │           │            │            │            │          │
                     ▼           ▼            ▼            ▼            ▼          │
                     ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐         │
                     │  Audit   │ │  Threat  │ │Alerting  │ │Compliance│         │
                     │ (7 topics)│ │ (3 topics)│ │(1 topic) │ │(1 topic) │         │
                     └──────────┘ └──────────┘ └──────────┘ └──────────┘         │
                     ┌──────────┐ ┌──────────┐ ┌──────────┐                       │
                     │   DLP    │ │ Webhook  │ │   IAM    │                       │
                     │ (1 topic)│ │ (1 topic)│ │ (1 topic)│                       │
                     └──────────┘ └──────────┘ └──────────┘                       │
                     ┌───────────────────────────────────────────────────────────┐│
                     │                       CONSUMERS                           ││
                     └───────────────────────────────────────────────────────────┘│
                     ┌────────────────────────────────────────────────────────────┘
                     │
                     ▼
```

| # | Topic | Partitions | Producers | Consumers | Event Types |
|---|-------|------------|-----------|-----------|-------------|
| 1 | `auth.events` | 12 | IAM (outbox relay) | Threat (5 detectors), Audit | `auth.login.success`, `auth.login.failed`, `password.changed` |
| 2 | `policy.changes` | 6 | Policy (outbox relay) | Threat (PrivilegeEscalation), Audit | `role.grant`, `policy.changed` |
| 3 | `data.access` | 24 | External SDK / example app | Threat (DataExfiltration), Audit | `resource.read`, `resource.write` |
| 4 | `threat.alerts` | 12 | Threat Service (all 6 detectors) | Alerting (AlertSaga), Audit | Threat alert JSON |
| 5 | `audit.trail` | 24 | Audit (ingest), Alerting (saga step 4) | Audit (self), Compliance (ClickHouseWriter) | Audit envelope `{event_id, type, org_id, payload}` |
| 6 | `saga.orchestration` | — | IAM (outbox), Saga Watcher | IAM (SagaConsumer), Audit | User provisioning, org offboarding events |
| 7 | `connector.events` | — | Connector Registry | Audit | Connector lifecycle events |
| 8 | `notifications.outbound` | 6 | Alerting (saga step 2) | Notification service | Alert notification payloads |
| 9 | `webhook.delivery` | — | Produced by other services | Webhook Delivery (WebhookConsumer) | Webhook trigger events |
| 10 | `webhook.dlq` | — | Webhook Delivery | (manual recovery) | Failed webhook deliveries |
| 11 | `control.plane.events` | — | Control Plane | DLP (DLPConsumer) | Control plane operations |
| 12 | `dlp.dlq` | — | DLP Service | (manual recovery) | Failed DLP scans |

---

## Data Flow by Business Process

### Authentication Flow (IAM → Redis + PostgreSQL + Kafka)

```
User                IAM Service              PostgreSQL              Redis              Kafka
 │                    │                         │                    │                    │
 │  POST /auth/login  │                         │                    │                    │
 │───────────────────>│                         │                    │                    │
 │                    │  Verify password        │                    │                    │
 │                    │────────────────────────>│  SELECT users      │                    │
 │                    │<────────────────────────│  WHERE email       │                    │
 │                    │                         │                    │                    │
 │                    │  Check MFA required     │                    │                    │
 │                    │────────────────────────>│  SELECT mfa_configs│                    │
 │                    │<────────────────────────│                    │                    │
 │                    │                         │                    │                    │
 │                    │  Store MFA challenge    │                    │  SET mfa_challenge: │
 │                    │──────────────────────────────────────────────────>{token}         │
 │                    │                         │                    │  (5 min TTL)       │
 │                    │  Write outbox event     │                    │                    │
 │                    │────────────────────────>│  INSERT outbox     │                    │
 │                    │                         │  (pending)         │                    │
 │                    │<── pg_notify ───────────│                    │                    │
 │                    │                         │                    │                    │
 │                    │  Create session          │                    │                    │
 │                    │────────────────────────>│  INSERT sessions   │                    │
 │  JWT token         │                         │                    │                    │
 │<───────────────────│                         │                    │                    │
 │                    │                         │                    │                    │
 │                    │  (async) Outbox relay   │                    │                    │
 │                    │────────────────────────>│  UPDATE status     │───────────────────>│ auth.events
 │                    │                         │  = published       │                    │
 │                    │                         │                    │                    │
 │                    │                         │                    │  SET blocklist:{jti}│
 │  POST /auth/logout │                         │                    │  (on logout)       │
 │───────────────────>│                         │                    │──────────────────>│
```

### Threat Detection Flow (Kafka → Threat → Redis + MongoDB + Kafka)

```
Kafka                  Threat Service              Redis                 MongoDB               Kafka
 │                         │                        │                      │                    │
 │  auth.events            │                        │                      │                    │
 │  (login.failed)         │                        │                      │                    │
 │────────────────────────>│  BruteForceDetector    │                      │                    │
 │                         │  consume event         │                      │                    │
 │                         │                        │                      │                    │
 │                         │  Track failed attempt  │                      │                    │
 │                         │───────────────────────>│  ZADD bruteforce:ip:│                    │
 │                         │                        │  :{ip}              │                    │
 │                         │                        │  ZCARD → check > 10 │                    │
 │                         │<───────────────────────│                      │                    │
 │                         │                        │                      │                    │
 │                         │  If threshold exceeded:│                      │                    │
 │                         │  Create Alert          │                      │                    │
 │                         │──────────────────────────────────────────────>│ INSERT threats.   │
 │                         │                        │                      │ alerts            │
 │                         │                        │                      │                    │
 │                         │  Cache alert           │                      │                    │
 │                         │───────────────────────>│  SET threat:brute-  │                    │
 │                         │                        │  force:ip:{ip}      │                    │
 │                         │                        │  (24h TTL)          │                    │
 │                         │                        │                      │                    │
 │                         │  Publish alert          │                     │                    │
 │                         │───────────────────────────────────────────────────────────────>│ threat.
 │                         │                        │                      │                    │ alerts
 │                         │  Commit offset         │                      │                    │
 │  (offset committed)     │                        │                      │                    │
 │<────────────────────────│                        │                      │                    │
```

### Audit Event Pipeline (SDK → Audit → MongoDB → Kafka → ClickHouse)

```
Example App           Audit Service              MongoDB                 Kafka              ClickHouse
    │                       │                      │                      │                    │
    │  POST /v1/events/     │                      │                      │                    │
    │  ingest               │                      │                      │                    │
    │──────────────────────>│                      │                      │                    │
    │                       │  (Optional DLP scan) │                      │                    │
    │                       │──> DLP service ──>   │                      │                    │
    │                       │<── allow/block ────  │                      │                    │
    │                       │                      │                      │                    │
    │                       │  Publish to Kafka    │                      │                    │
    │                       │─────────────────────────────────────────────>│ audit.trail       │
    │ 202 Accepted          │                      │                      │                    │
    │<──────────────────────│                      │                      │                    │
    │                       │                      │                      │                    │
    │                       │  (async) Consumer    │                      │                    │
    │                       │──────────────────────>│ INSERT audit_events │                    │
    │                       │                      │ + update hash_chain  │                    │
    │                       │                      │                      │                    │
    │                       │                      │                      │  (Compliance       │
    │                       │                      │                      │   consumer)        │
    │                       │                      │                      │───────────────────>│ INSERT events
    │                       │                      │                      │                    │ (ReplacingMergeTree)
```

### Alerting Saga (Kafka → Alerting → MongoDB + Kafka)

```
Kafka                 Alerting Service            MongoDB                 Kafka
(threat.alerts)             │                        │                      │
    │                       │                        │                      │
    │  Alert event          │                        │                      │
    │──────────────────────>│  Step 1: Persist        │                      │
    │                       │────────────────────────>│ INSERT alerting.    │
    │                       │                        │ alerts + saga_step  │
    │                       │<────────────────────────│                     │
    │                       │                        │                      │
    │                       │  Step 2: Notify        │                      │
    │                       │───────────────────────────────────────────────>│ notifications.
    │                       │                        │                      │ outbound
    │                       │  Step 3: SIEM          │                      │
    │                       │────────────────────────>│ (external webhook)  │
    │                       │<────────────────────────│                     │
    │                       │                        │                      │
    │                       │  Step 4: Audit         │                      │
    │                       │───────────────────────────────────────────────>│ audit.trail
    │                       │                        │                      │
    │                       │  Update saga status    │                      │
    │                       │────────────────────────>│ UPDATE saga_steps[] │
    │                       │<────────────────────────│                     │
```

### Policy Evaluation Flow (SDK → Policy → PostgreSQL + Redis)

```
SDK/Example App         Policy Service              PostgreSQL              Redis
    │                       │                         │                      │
    │  POST /v1/policy/     │                         │                      │
    │  evaluate             │                         │                      │
    │──────────────────────>│                         │                      │
    │                       │  Check cache            │                      │
    │                       │───────────────────────────────────────────────>│ GET policy:eval:
    │                       │<───────────────────────────────────────────────│ {sha256(req)}→{effect}
    │                       │                         │                      │
    │                       │  Cache miss: evaluate   │                      │
    │                       │────────────────────────>│ SELECT policies      │
    │                       │                         │ + assignments       │
    │                       │<────────────────────────│                     │
    │                       │                         │                      │
    │                       │  Write evaluation log   │                      │
    │                       │────────────────────────>│ INSERT policy_eval_ │
    │                       │                         │ log                 │
    │                       │                         │                      │
    │                       │  Update cache           │                      │
    │                       │───────────────────────────────────────────────>│ SET policy:eval:
    │                       │                         │                      │ {sha256(req)}
    │  {allowed, reason}    │                         │                      │
    │<──────────────────────│                         │                      │
```

---

## Data Lifecycle & TTL Summary

| Data | Store | Retention | Notes |
|------|-------|-----------|-------|
| User credentials | PostgreSQL | Indefinite | Soft-deleted via `deleted_at` |
| Sessions | PostgreSQL | Until revoked | Cleanup via expired session purge |
| Refresh tokens | PostgreSQL | Until revoked | Rotation chain tracked by `family_id` |
| Outbox records | PostgreSQL | Until published + 7d | Cleaned up by relay after publish |
| Policy evaluation logs | PostgreSQL | Indefinite | Audit requirement |
| Webhook delivery logs | PostgreSQL | Indefinite | — |
| DLP findings | PostgreSQL DLP | Indefinite | — |
| Audit events | MongoDB | Indefinite | Updated via hash chain on each append |
| Threat alerts | MongoDB `threats.alerts` | Indefinite | Status transitions: open → acknowledged → resolved |
| Alerting saga state | MongoDB `alerting.alerts` | Indefinite | 4-step saga tracking |
| Auth MFA challenges | Redis | 5 min | TTL auto-expiry |
| OAuth2 auth codes | Redis | 10 min | PKCE flow window |
| WebAuthn sessions | Redis | 5 min | Registration/login ceremony window |
| TOTP nonces | Redis | 90 sec | Deduplication window |
| Policy eval cache | Redis | 60 sec + 5s stale | Stale-while-revalidate |
| Brute force windows | Redis | 5 min | Sliding window ZSET + 5m alert dedup |
| Travel state | Redis | 1 h | Last login geo-location |
| Off-hours tracking | Redis | 7 d | Per-day in-hours access records |
| Device fingerprints | Redis | 30 d | Known device set for ATO detection |
| Data access windows | Redis | 1 h | Sliding window ZSET |
| Baseline stats | Redis | None (persistent) | Org-level access mean/stddev |
| Privilege escalation flags | Redis | 1 h | Recent login markers |
| Threat alert cache | Redis | 24 h | All detectors cache alerts |
| JWT blocklist | Redis | Until token expiry | TTL = remaining token lifetime |
| Rate limiter state | Redis | 1 min | Sliding window ZSET |
| Audit events (long-term) | ClickHouse | 2 years | ReplacingMergeTree + TTL |
| Event daily counts (CH) | ClickHouse | 2 years | SummingMergeTree materialized view |
| Alert stats (CH) | ClickHouse | Schema only | Not yet populated |
| Compliance reports | S3 | Indefinite | Stored as PDF + .sig files |
| Kafka topics | — | Configurable | Broker retention (default 7d) |
