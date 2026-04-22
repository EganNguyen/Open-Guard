# OpenGuard — Requirements & Design Approach

> Self-hostable centralized security control plane. Fortune-500 scale: 100k+ users, 10k+ orgs, millions of audit events/day, sub-100ms policy evaluation at p99.

---

## Table of contents

1. [Planning the approach](#1-planning-the-approach)
2. [Core entities](#2-core-entities)
3. [Functional requirements](#3-functional-requirements)
4. [Non-functional requirements](#4-non-functional-requirements)
5. [High-level design](#5-high-level-design)
6. [Potential deep dives](#6-potential-deep-dives)
7. [Depth of expertise checklist](#7-depth-of-expertise-checklist)

---

## 1. Planning the approach

Work through requirements **sequentially** — satisfy each functional requirement one at a time before moving to deep dives. This prevents scope creep and ensures nothing is missed.

### Recommended build order

```
Phase 1  →  Infra, CI/CD, Observability
Phase 2  →  Foundation & Authentication
Phase 3  →  Policy Engine
Phase 4  →  Event Bus & Audit
Phase 5  →  Threat Detection & Alerting
Phase 6  →  Compliance & Analytics
Phase 7  →  Security Hardening
Phase 8  →  Load Testing & SLO Verification
Phase 9  →  Documentation
Phase 10 →  DLP
```

> **Rule:** A phase is not complete until its SLOs pass k6 verification. SLOs are gates, not guidelines.

### Key architectural decisions to make early

| Decision | Chosen approach |
|---|---|
| Audit atomicity | Transactional Outbox (PostgreSQL → Kafka relay) |
| Service coordination | Choreography-based sagas via Kafka |
| Tenant isolation | PostgreSQL RLS (shared), schema/shard as extension points |
| Read/write split | CQRS — MongoDB primary write, secondary read |
| Fail mode | Closed (deny) on all unavailable dependencies |
| Service auth | mTLS on all internal calls |

---

## 2. Core entities

### Primary data entities

| Entity | Store | Notes |
|---|---|---|
| `orgs` | PostgreSQL | Root tenant. All tenant tables carry `org_id UUID NOT NULL`. |
| `users` | PostgreSQL | Status enum: `initializing → active → suspended → deprovisioned`. Also `provisioning_failed`. |
| `sessions` | PostgreSQL | Tracks active JWTs by `jti`. Used for bulk revocation on user deletion. |
| `api_tokens` | PostgreSQL | Long-lived tokens stored as PBKDF2 hash. |
| `connectors` | PostgreSQL | Registered connected apps. `status: active | suspended`. |
| `policies` | PostgreSQL | RBAC policy definitions scoped per org. |
| `outbox_records` | PostgreSQL | Transactional outbox — written atomically with business rows. |
| `audit_events` | MongoDB | Append-only. HMAC hash-chained per org. `event_id` unique index for deduplication. |
| `webhook_deliveries` | PostgreSQL | Delivery state per outbound webhook attempt. |
| `dlp_findings` | PostgreSQL | DLP scan results, scoped per org. |

### Key Redis structures

| Key pattern | Purpose | TTL |
|---|---|---|
| `connector:fasthash:{sha256_prefix}` | Connector credential cache | 30s |
| `jti:{jti}` | Revoked token blocklist | `exp − now()` (dynamic) |
| `saga:deadlines` (sorted set) | Saga timeout tracking, score = unix deadline | — |

### User status state machine

```
(new) ──────────────────────────────► initializing
                                            │
                          saga.completed ◄──┤──► saga.timed_out / compensation
                                            │                    │
                                            ▼                    ▼
                                          active         provisioning_failed
                                         ╱    ╲                  │
                              suspend   ╱      ╲  deprovision    │ reprovision (admin)
                                       ▼        ▼                │
                                   suspended  deprovisioned ◄────┘
                                       │
                               activate│
                                       ▼
                                     active
```

> `provisioning_failed` is **not terminal**. Org admins retry via `POST /users/:id/reprovision`.  
> IAM **must reject logins** for users in `initializing` status.

---

## 3. Functional requirements

### FR-IAM — Identity & access management

#### FR-IAM-01 · OIDC / SAML IdP
- Act as identity provider for all connected applications
- Issue ID tokens, access tokens (JWT with `jti`), and refresh tokens per RFC 6749 / 7519
- Support SSO via SAML 2.0 and OIDC authorization code + PKCE flows
- Access token max TTL: 1 hour. Refresh tokens rotated on use.

#### FR-IAM-02 · Multi-factor authentication
- Enforce TOTP (RFC 6238) and WebAuthn/FIDO2 as second factors
- Org admins can mandate MFA org-wide via policy
- Users in `initializing` status **must be denied login** at the IAM layer

#### FR-IAM-03 · Session & token lifecycle
- Every JWT carries a `jti` claim
- Validation order: (1) verify signature → (2) check `exp`, reject `ErrTokenExpired` → (3) check Redis blocklist
- Blocklist entry TTL = `exp − now()` (never a fixed TTL)
- Clock skew tolerance: max 5 minutes forward, zero backward (`ErrClockSkew` on early `iat`)

#### FR-IAM-04 · API token management
- Org admins create, rotate, and revoke long-lived API tokens
- Plaintext shown once at creation; never stored — only PBKDF2-HMAC-SHA512 hash stored

#### FR-IAM-05 · SCIM 2.0 provisioning
- Implement SCIM 2.0 `Users` and `Groups` endpoints
- `POST /scim/v2/Users` is idempotent on `scim_external_id`: if a non-`deprovisioned` user with the same external ID exists, return the existing resource (200), not 409
- `org_id` derived from SCIM bearer token — **never from client-supplied headers**
- New users start in `initializing` status and transition to `active` only after the provisioning saga completes

---

### FR-POL — Policy engine

#### FR-POL-01 · Real-time RBAC evaluation
- Evaluate `POST /v1/policy/evaluate` synchronously
- Return allow/deny decision with a reason code
- **Fail closed:** deny by default when the control plane is unreachable

#### FR-POL-02 · SDK local caching
- SDK caches policy decisions locally for up to 60 seconds
- After 60s without a control plane heartbeat, SDK must deny **all** requests
- Policy change invalidation must propagate to SDK within 60s

#### FR-POL-03 · Policy assignment (saga step)
- On user creation, default org policies assigned via choreography saga
- Emits `policy.assigned` on success or `policy.assignment.failed` (compensation) on failure

---

### FR-CON — Connector registry

#### FR-CON-01 · Connector registration
- Org admins register apps via `POST /v1/admin/connectors` (JWT auth)
- System issues a one-time plaintext API key: `prefix (8 chars) + secret`
- Prefix stored as `SHA-256(prefix)` fast-hash in Redis; secret stored as PBKDF2 hash in PostgreSQL
- Plaintext key **never stored** — shown once at creation

#### FR-CON-02 · Credential authentication
```
1. Parse key: prefix = key[0:8], secret = key[8:]
2. fastHash = SHA-256(prefix)
3. Redis GET "connector:fasthash:{fastHash}"
   → Cache hit:  deserialize ConnectedApp; verify PBKDF2 only if last_verified_at > 5min ago
   → Cache miss: full PBKDF2 verify → DB lookup → SET Redis (TTL 30s)
4. Check status == "active"
5. Set org RLS context and connector scopes
```
- Suspended connectors: Redis key updated to "suspended" sentinel — **not deleted**
- Reactivated connectors: Redis key overwritten immediately with full connector record (not just deleted)

#### FR-CON-03 · Outbound webhook delivery
- Deliver signed webhooks (HMAC-SHA256) to connector-registered URLs
- Every delivery includes `X-OpenGuard-Delivery` UUID header
- Delivery state persisted in `webhook_deliveries` (PostgreSQL)
- After `WEBHOOK_MAX_ATTEMPTS` failures → route to `webhook.dlq` topic

---

### FR-AUD — Audit trail & event ingestion

#### FR-AUD-01 · Transactional outbox (dual-write prevention)
```go
// CORRECT — both rows committed atomically
tx.Exec("INSERT INTO users ...")
tx.Exec("INSERT INTO outbox_records ...")
tx.Commit()
// Relay publishes to Kafka asynchronously — Kafka is never in the sync write path
```
- Outbox relay reads committed records and publishes to Kafka
- Kafka consumer commits offsets **only after** successful downstream write (MongoDB/ClickHouse)
- On crash before offset commit: messages reprocessed; `event_id` unique index makes this safe

#### FR-AUD-02 · Append-only HMAC chain
- Audit events stored append-only in MongoDB
- Each event includes `hmac(prev_event)` scoped per org — forms a verifiable chain
- Chain sequence assigned via batched atomic reservation for throughput
- MongoDB write path: bulk insert up to 500 documents or 1-second flush, whichever comes first
- MongoDB read path: `secondaryPreferred` (acceptable staleness: up to replica lag)

#### FR-AUD-03 · Connector event ingestion
- Connected apps push events to `POST /v1/events/ingest`
- Events normalized into the same Kafka pipeline as internal events
- Deduplication via `event_id` unique index on `audit_events`, scoped to retention window

#### FR-AUD-04 · Integrity verification
- `GET /audit/integrity` verifies the per-org HMAC chain
- **Must use MongoDB `readPreference: primary`** — secondary lag causes false-positive integrity failures

---

### FR-THR — Threat detection

#### FR-THR-01 · Streaming anomaly scoring
- Score events in real time for: brute force, impossible travel, off-hours access, account takeover, privilege escalation
- User baseline profile initialized via `threat.baseline.init` saga step on user creation

#### FR-THR-02 · Rule-based and ML alerts
- Emit alerts via rule-based thresholds and ML-scored anomalies
- Route alerts to SIEM export and signed outbound webhooks
- Alert preferences initialized per user during SCIM provisioning saga

---

### FR-COM — Compliance & DLP

#### FR-COM-01 · Compliance report generation
- Generate GDPR, SOC 2, HIPAA reports as PDF output
- Compliance queries: `readPreference: secondary` (acceptable staleness: 5s)
- Max 10 concurrent report generation jobs

#### FR-COM-02 · DLP content scanning
- Real-time detection of PII, credentials, and financial data
- Per-org opt-in sync-block mode: if DLP unavailable in sync mode, **reject the event** (fail closed)
- Async scan latency target: p99 ≤ 500ms

#### FR-COM-03 · GDPR erasure & offboarding
- On org offboarding: queue GDPR erasure export if requested
- Hard-delete after `ORG_DATA_RETENTION_DAYS` in FK-safe order:  
  `outbox_records → dlp_findings → dlp_policies → … → orgs`
- Audit chain must be finalized with `org.offboarded` terminal event **before** deletion

---

### FR-TEN — Multi-tenancy

#### FR-TEN-01 · Row-level security isolation
- PostgreSQL RLS enforced on all tenant tables
- RLS policy: `NULLIF(current_setting('app.org_id', true), '')::UUID` — handles NULL and empty string uniformly
- Zero cross-tenant data access possible at the DB layer
- Each service has its own DB user with table-level grants; migration runs as `openguard_migrate` (DDL only)

#### FR-TEN-02 · Three-tier tenancy model

| Tier | Mechanism | Target plan |
|---|---|---|
| Shared | PostgreSQL RLS on shared tables | Free / SMB |
| Schema | Dedicated PostgreSQL schema per org | Mid-market |
| Shard | Dedicated PostgreSQL instance per org | Enterprise / regulated |

All code written RLS-first. Schema and Shard tiers are extension points.

#### FR-TEN-03 · Tenant offboarding saga
On `org.offboard` event, services coordinate via Kafka:
1. IAM: revoke all sessions and API tokens; set users to `deprovisioned`
2. Control Plane: suspend all connectors; invalidate Redis cache entries
3. Policy: archive all org policies
4. Webhook Delivery: drain in-flight queue
5. Audit: finalize HMAC chain; write `org.offboarded` terminal event
6. Compliance: queue GDPR erasure export if requested
7. Scheduler: hard-delete after `ORG_DATA_RETENTION_DAYS`

---

### FR-ADM — Admin & API

#### FR-ADM-01 · Admin web console
- Next.js 14 dashboard for managing users, connectors, policies, alerts, and compliance reports
- Scoped to org admins; system admins have a super-admin view

#### FR-ADM-02 · API versioning policy
- All routes prefixed `/v1/`
- Breaking changes: implement `/v2/` alongside `/v1/`; add `Deprecation: true` and `Sunset: <date>` headers to `/v1/` responses
- `/v1/` maintained for **minimum 6 months** after `/v2/` GA

---

## 4. Non-functional requirements

### NFR-PERF · Performance SLOs

| Operation | p50 | p99 | p999 | Throughput |
|---|---|---|---|---|
| `POST /oauth/token` | 40ms | **150ms** | 400ms | 2,000 req/s |
| `POST /v1/policy/evaluate` (uncached) | 5ms | **30ms** | 80ms | 10,000 req/s |
| `POST /v1/policy/evaluate` (Redis cached) | 1ms | **5ms** | 15ms | 10,000 req/s |
| SDK local cache hit | <1ms | <1ms | <1ms | unlimited |
| `GET /audit/events` | 20ms | **100ms** | 250ms | 1,000 req/s |
| Kafka event → audit DB insert | — | 2s | 5s | 50,000 evt/s |
| Compliance report generation | — | 30s | 120s | 10 concurrent |
| `POST /v1/events/ingest` | 10ms | **50ms** | 150ms | 20,000 req/s |
| `GET /v1/scim/v2/Users` | 30ms | 500ms | 1,500ms | 500 req/s |
| Connector registry lookup (Redis cached) | 1ms | 5ms | 15ms | — |
| DLP async scan | — | 500ms | 2s | — |

> Bold p99 values are the primary SLO gates for k6 load tests.

### NFR-SCALE · Capacity model

**Bcrypt capacity (token issuance):**
- At cost 12, one CPU core processes ~2.86 logins/s (1 ÷ 350ms)
- Pool of `2N` workers on N-core node sustains ~`5.7N` logins/s
- Meeting 2,000 req/s SLO requires ~**35 CPU cores** (e.g., 6 pods × 6 cores = 36 cores)
- Single-pod deployments will 429 at ~150 req/s — this is correct behavior
- IAM HPA: `targetCPUUtilizationPercentage: 60`

**Event ingest at 20,000 req/s:**
- Requires backpressure shedding before PostgreSQL outbox writes become the bottleneck
- Connector auth: Redis fast-hash path must sustain ~20,000 lookups/s

### NFR-SEC · Security requirements

| Requirement | Implementation |
|---|---|
| Service-to-service auth | mTLS on all internal calls |
| Cross-tenant isolation | PostgreSQL RLS (DB layer, not app layer) |
| Secret storage | PBKDF2-HMAC-SHA512 (600,000 iterations) |
| JWT key rotation | `kid`-based; multiple valid keys coexist during rotation |
| MFA key rotation | Same `kid` pattern as JWT signing keys |
| Token revocation | `jti` blocklist in Redis; TTL = `exp − now()` |
| Webhook signing | HMAC-SHA256 with `X-OpenGuard-Delivery` UUID |
| Cert rotation | Zero-downtime rolling deploy; CA unchanged; dual-CA trust period for CA rotation |
| Connector suspension | Redis sentinel update (not delete) |
| Audit append-only | MongoDB append-only + HMAC chain — no update or delete paths |

### NFR-REL · Reliability requirements

| Requirement | Constraint |
|---|---|
| Fail mode | Closed (deny) — never open on unavailability |
| SDK degraded cache | 60s TTL; deny all after TTL expires |
| Audit delivery | Exactly-once (outbox + `event_id` dedup) |
| Kafka offset commit | Manual, post-write only |
| Outbox relay max lag | 10s |
| Relay pool conn lifetime | 60s (recycles after PostgreSQL primary failover) |
| Provisioning saga timeout | 40s (30s step timeout + 10s outbox max lag) |
| Webhook DLQ trigger | After `WEBHOOK_MAX_ATTEMPTS` exhaustion |

### NFR-DATA · Data & consistency requirements

| Requirement | Implementation |
|---|---|
| CQRS read/write split | MongoDB primary for writes; `secondaryPreferred` for reads |
| Integrity check read pref | **Primary only** (no secondary for `GET /audit/integrity`) |
| Compliance report staleness | `secondary`, up to 5s lag acceptable |
| Bulk write batch size | 500 documents or 1s flush interval, whichever first |
| Exactly-once Kafka delivery | Idempotent producer + manual offset commit |
| Connector credential caching | Redis 30s TTL; full PBKDF2 verify only if `last_verified_at > 5min` |

### NFR-OPS · Operability requirements

| Requirement | Target |
|---|---|
| Load test gate | All SLOs verified with k6 before phase completion |
| Certificate rotation | Zero-downtime (rolling deploy, CA unchanged) |
| CA rotation | Dual-CA trust period (see `docs/runbooks/ca-rotation.md`) |
| Migration role | `openguard_migrate` (DDL only, no `BYPASSRLS` on data tables) |
| Per-service DB users | Yes — table-level grants, least privilege |
| API deprecation notice | ≥6 months after `/v2/` GA |

### Connection pool targets

| Service | DB | Pool min | Pool max | Max conn lifetime |
|---|---|---|---|---|
| IAM | PostgreSQL | 5 | 25 | 1800s |
| Control Plane | PostgreSQL (outbox) | 2 | 15 | 300s |
| Outbox Relay | PostgreSQL | 2 | 10 | **60s** |
| Connector Registry | PostgreSQL | 2 | 10 | 1800s |
| Policy | PostgreSQL | 2 | 15 | 1800s |
| Audit (write) | MongoDB | 2 | 10 | — |
| Audit (read) | MongoDB | 5 | 30 | — |
| Compliance | ClickHouse | 2 | 8 | — |
| All services | Redis | 5 | 20 | — |

> Relay pool lifetime is 60s so stale connections are recycled within 60s of a PostgreSQL primary failover.

---

## 5. High-level design

### Services

```
┌─────────────────────────────────────────────────────────┐
│                    Admin Dashboard (Next.js 14)          │
└──────────────────────────┬──────────────────────────────┘
                           │ JWT (OIDC)
          ┌────────────────┼──────────────────────────────────┐
          ▼                ▼                 ▼                ▼
   ┌─────────────┐  ┌───────────┐  ┌───────────────┐ ┌───────────────┐
   │  IAM Service│  │  Policy   │  │ Control Plane │ │  Connector    │
   │  (OIDC/SAML)│  │  Engine   │  │               │ │  Registry     │
   └──────┬──────┘  └─────┬─────┘  └───────┬───────┘ └───────┬───────┘
          │               │                │                 │
          └───────────────┼────────────────┼─────────────────┘
                          │ Transactional Outbox
                          ▼
                   ┌─────────────┐
                   │  PostgreSQL │ ← RLS enforced, per-service users
                   │  (primary)  │
                   └──────┬──────┘
                          │ Outbox Relay
                          ▼
                   ┌─────────────┐
                   │    Kafka    │ ← Idempotent producer
                   └──────┬──────┘
  ┌───────┼───────────────┼──────────────┬──────┐
  ▼       ▼               ▼              ▼      ▼
┌─────┐ ┌──────┐ ┌─────────────┐ ┌─────────┐ ┌────────────────┐
│ DLP │ │Threat│ │Audit Service│ │Alerting │ │Webhook Delivery│
│     │ │      │ │  (MongoDB)  │ │         │ │                │
└─────┘ └──────┘ └──────┬──────┘ └─────────┘ └────────────────┘
                        │
                  ┌─────┴─────┐      ┌────────────┐
                  │ClickHouse │ ◄─── │ Compliance │
                  └───────────┘      └────────────┘
```

### Full system overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       OpenGuard — security control plane                    │
│                 (User traffic never flows through OpenGuard)                │
│                                                                             │
│  ┌──────────────┐          ┌──────────────┐          ┌──────────────┐       │
│  │ IAM service  │          │ Policy engine│          │  Connector   │       │
│  │ OIDC · SAML  │          │  RBAC eval   │          │   registry   │       │
│  │ SCIM · MFA   │          │  SDK cache   │          │  App creds   │       │
│  └──────┬───────┘          └──────┬───────┘          └──────┬───────┘       │
│         │                         │                         │               │
│         ▼                         ▼                         ▼               │
│  ┌──────────────┐          ┌──────────────┐          ┌──────────────┐       │
│  │    Redis     │          │  PostgreSQL  │          │    Kafka     │       │
│  │  Cache · JTI ├─────────▶│ RLS · outbox ├─────────▶│ Outbox relay │       │
│  └──────────────┘          └──────┬───────┘          │ audit trail  │       │
│                                   │                  └──────┬───────┘       │
│         ┌─────────────────────────┴─────────────────────────┤               │
│         ▼                         ▼                         ▼               │
│  ┌──────────────┐          ┌──────────────┐          ┌──────────────┐       │
│  │Audit consumer│          │    Threat    │          │   Alerting   │       │
│  │ MongoDB ·    │          │   detection  │          │     SIEM     │       │
│  │ HMAC chain   │          │Anomaly scoring│          │   webhooks   │       │
│  └──────┬───────┘          └──────┬───────┘          └──────┬───────┘       │
│         │                         │                         │               │
└─────────┼─────────────────────────┼─────────────────────────┼───────────────┘
          │                         │                         │
  ┌───────▼───────┐          ┌──────▼───────┐          ┌──────▼───────┐
  │   End users   │          │Admin dashboard│          │Connected SaaS│
  │ SSO · MFA ·   │          │  Next.js 14  │          │     apps     │
  │     SCIM      │          └──────────────┘          │ SDK · ingest │
  └───────────────┘                                    └──────────────┘

  (Also interfaces with: Compliance [ClickHouse/PDF] and SIEM [Alert export])
```

### The write path

```
  ┌────────────────┐         ┌────────────────────────┐
  │  HTTP handler  │         │     PostgreSQL tx      │
  │  Any service   │────────▶│ ┌────────────────────┐ │
  └────────────────┘         │ │ business_row INSERT│ │
          │                  │ └────────────────────┘ │
          │                  │ ┌────────────────────┐ │
          │                  │ │ outbox_record INSERT│ │
          │                  │ └────────────────────┘ │
          │                  └───────────┬────────────┘
          │                              │
    tx.Commit()                   atomic commit
                                         │
          ┌──────────────────────────────┴──────────────────────────────┐
          │                                                             │
          │                     async (not in write path)               │
          ▼                                                             │
  ┌────────────────┐         ┌────────────────────────┐                 │
  │  Outbox relay  │────────▶│         Kafka          │                 │
  │Polls committed │         │      audit.trail       │                 │
  │      rows      │         └────────────────────────┘                 │
  └────────────────┘                                                    │
          │                                                             │
          └────────────────── offset committed post-write ───────────────┘
```
### The policy evaluation path

```
       ┌──────────┐          ┌───────────────────────┐
       │ SaaS app │─────────▶│          SDK          │
       │user action│          │  Local cache 60s TTL  │
       └──────────┘          └──────────┬────────────┘
                                        │
             ┌──────────────────────────┴───────────────────────────┐
             │                          │                           │
        (cache hit)               (cache miss)                (unreachable)
             │                          │                           │
             ▼                          ▼                           ▼
      ┌──────────────┐          ┌────────────────┐          ┌──────────────┐
      │ allow / deny │◀─────────┤ Policy engine  │          │  Fail closed │
      └──────────────┘          │ POST /evaluate │          │ Deny after   │
             ▲                  └───────┬────────┘          │ 60s TTL      │
             │                          │                   └──────────────┘
             │                    (Redis lookup)
             │                          │
             │                  ┌───────▼────────┐          ┌──────────────┐
             │                  │     Redis      │  (miss)  │  PostgreSQL  │
             └──────────────────┤ Decision cache ├─────────▶│ Policies ·   │
                                └────────────────┘          │ roles · orgs │
                                                            └──────────────┘

  ───────────────────────────────────────────────────────────────────────────
  LATENCY METRICS:
  • <1ms hit (Local SDK cache)
  • 5ms cached (Policy engine + Redis)
  • 30ms uncached (Policy engine + PostgreSQL)
```

### Data flows

**Write path (any state change):**
`HTTP handler → PostgreSQL tx (business row + outbox_record) → Relay → Kafka → Audit Consumer → MongoDB`

**Policy evaluation path:**
`SDK → POST /v1/policy/evaluate → Redis cache → PostgreSQL (on miss) → decision`

**Event ingest path:**
`Connected app → POST /v1/events/ingest → connector auth (Redis fast-hash) → PostgreSQL outbox → Kafka → Audit Consumer`

---

## 6. Potential deep dives

### DD-1 · Transactional outbox & exactly-once audit
- Outbox relay design (poll vs. logical replication / Debezium)
- Kafka idempotent producer configuration
- Consumer offset commit strategy and DLQ routing
- `event_id` deduplication index design and retention window scoping

### DD-2 · HMAC hash chain integrity
- Batch chain sequence reservation (atomic counter in PostgreSQL)
- Chain verification algorithm and `GET /audit/integrity` implementation
- Why primary read preference is non-negotiable for integrity checks
- Handling chain gaps on bulk insert failure

### DD-3 · Bcrypt worker pool & token issuance SLO
- Bounded goroutine pool sizing: `2 × NumCPU`
- 429 behavior at pool saturation (correct, not a bug)
- HPA configuration (`targetCPUUtilizationPercentage: 60`)
- Horizontal scaling math: 35 CPU cores for 2,000 req/s

### DD-4 · Connector credential auth at 20,000 req/s
- Fast-hash prefix scheme: `SHA-256(prefix)` → Redis → PBKDF2 on miss
- `last_verified_at` optimization: skip PBKDF2 if verified within 5 min
- Suspension sentinel pattern (update, never delete)
- Cache invalidation on reactivation (immediate overwrite)

### DD-5 · Choreography-based SCIM provisioning saga
- Full saga state machine (8 steps, 4 services)
- Compensation events and `provisioning_failed` handling
- Saga timeout: Redis sorted set `saga:deadlines` + background watcher
- Idempotency of `POST /scim/v2/Users` on `scim_external_id`

### DD-6 · JWT revocation & session management
- `jti` blocklist in Redis: dynamic TTL = `exp − now()`
- Bulk revocation on user deletion (pipeline SETEX per active `jti`)
- Validation order: signature → expiry → blocklist
- Key rotation with `kid` — multiple valid keys during rotation window

### DD-7 · PostgreSQL RLS & zero cross-tenant leakage
- `NULLIF(current_setting('app.org_id', true), '')::UUID` — why this exact form
- Migration role separation (`openguard_migrate` vs. service roles)
- RLS on `outbox_records` — relay role must bypass RLS to drain all orgs
- Schema and Shard tier extension points

### DD-8 · MongoDB CQRS and read preferences
- Write path: primary, bulk 500 / 1s flush
- Read path: `secondaryPreferred` — audit queries and compliance
- Exception: `GET /audit/integrity` → `primary` only
- Acceptable staleness for compliance reports (5s)

### DD-9 · DLP content scanning
- Sync-block vs. async mode (per-org opt-in)
- Fail-closed behavior in sync-block mode
- PII, credential, and financial data detection pipeline
- `dlp_findings` schema and retention

### DD-10 · mTLS and certificate rotation
- Zero-downtime cert rotation procedure (rolling deploy, same CA)
- CA rotation: dual-CA trust period
- Service mesh vs. sidecar vs. library-level mTLS

---

## 7. Depth of expertise checklist

Use this to self-assess coverage before considering a phase complete.

### Identity & access
- [ ] OIDC authorization code + PKCE flow implemented end to end
- [ ] SAML 2.0 SP-initiated and IdP-initiated SSO
- [ ] TOTP and WebAuthn/FIDO2 enrollment and verification
- [ ] `jti` blocklist with dynamic TTL (`exp − now()`)
- [ ] Token validation order: signature → expiry → blocklist
- [ ] Clock skew: 5-min forward tolerance, zero backward
- [ ] `initializing` status blocks login in IAM middleware
- [ ] SCIM `POST /Users` idempotent on `scim_external_id`
- [ ] SCIM `org_id` from token, never from client headers

### Policy engine
- [ ] `POST /v1/policy/evaluate` returns allow/deny + reason
- [ ] SDK local cache: 60s TTL, deny-all on expiry
- [ ] Fail-closed behavior verified with control plane down

### Connectors & webhooks
- [ ] Fast-hash prefix scheme: SHA-256(prefix) → Redis → PBKDF2
- [ ] Suspension sentinel (update, not delete) in Redis
- [ ] Reactivation overwrites Redis immediately
- [ ] Webhook delivery signed with HMAC-SHA256
- [ ] `X-OpenGuard-Delivery` UUID on every delivery
- [ ] DLQ after `WEBHOOK_MAX_ATTEMPTS` exhaustion

### Audit trail
- [ ] Transactional outbox: business row + outbox record in one `tx.Commit()`
- [ ] No Kafka call in synchronous write path
- [ ] Kafka offset committed only after downstream write confirmed
- [ ] `event_id` unique index on `audit_events` (MongoDB)
- [ ] HMAC chain: each event references `hmac(prev_event)`
- [ ] `GET /audit/integrity` uses `readPreference: primary`
- [ ] Bulk write: 500 docs or 1s flush, offset committed after

### Multi-tenancy
- [ ] RLS policy uses `NULLIF(current_setting(...), '')::UUID`
- [ ] Every tenant table has `org_id UUID NOT NULL`
- [ ] Service DB users have table-level grants only
- [ ] `openguard_migrate` role: DDL only, no `BYPASSRLS`
- [ ] Relay role can drain outbox across all orgs (RLS bypass for relay only)

### Sagas & state machines
- [ ] User saga: 8 steps across 4 services
- [ ] `saga:deadlines` sorted set in Redis; watcher polls every 10s
- [ ] Saga timeout = `SAGA_STEP_TIMEOUT_SECONDS + OUTBOX_MAX_LAG_SECONDS` (40s default)
- [ ] Compensation events are idempotent
- [ ] `user.deleted` saga: bulk `jti` revocation before status update

### Performance & operability
- [ ] k6 load test for every SLO target — all must pass before phase complete
- [ ] Bcrypt worker pool sized to `2 × NumCPU`; HPA at 60% CPU
- [ ] Redis pool: min 5, max 20 per service
- [ ] Outbox relay pool lifetime: 60s
- [ ] Cert rotation: zero-downtime rolling procedure documented
- [ ] CA rotation: dual-CA trust period runbook at `docs/runbooks/ca-rotation.md`
- [ ] API deprecation: `Deprecation: true` + `Sunset` headers; ≥6 months notice

---

## 8. Concurrency Strategies and Implementations

```markdown
## 8.1. Write path: Transactional outbox
**Atomic dual-write**

### Problem
A process crash between the DB write and the Kafka publish creates a permanent audit gap. The two operations cannot be made atomic across different systems.

### Solution
Write the business row and the outbox_record in a single PostgreSQL transaction. The relay reads committed outbox rows and publishes to Kafka asynchronously. Kafka is never in the synchronous write path.

### Implementation
```go
// WRONG — gap if crash between these lines
db.Exec("INSERT INTO users ...")
kafka.Publish("audit.trail", event)

// CORRECT — atomic
tx.Exec("INSERT INTO users ...")
tx.Exec("INSERT INTO outbox_records ...")
tx.Commit()
// relay publishes async, offset committed post-write
```

### Risks & Invariants
* 🔴 Without outbox: crash between writes = silent audit gap forever
* 🔴 Without outbox: Kafka down = user writes blocked
* 🟢 With outbox: relay reprocesses on restart — event_id unique index makes this idempotent
* 🟢 Outbox relay pool lifetime = 60s to recycle stale conns after PG failover

### SLO Targets
| Metric | Value |
| :--- | :--- |
| Kafka in write path | Never |
| Outbox max lag | 10s |
| Relay pool lifetime | 60s |
| Delivery guarantee | Exactly-once |

---

## 8.2. Token issuance: Bcrypt worker pool
**Bounded goroutines**

### Problem
bcrypt at cost 12 takes 250–400ms per operation. At 2,000 req/s without pooling, ~800 goroutines would each block on bcrypt simultaneously, starving the CPU and causing cascading latency.

### Solution
A bounded goroutine pool sized to 2 × NumCPU serialises bcrypt work. Requests beyond pool capacity receive 429. HPA scales IAM pods until total CPU capacity meets the 2,000 req/s target.

### Implementation
```go
// Pool sized to 2 × runtime.NumCPU()
pool := make(chan struct{}, 2*runtime.NumCPU())

func hashPassword(pw string) (string, error) {
    pool <- struct{}{}        // acquire slot
    defer func() { <-pool }() // release on return
    return bcrypt.GenerateFromPassword(
        []byte(pw), bcrypt.DefaultCost,
    )
}
// 429 when pool is full — correct, not a bug
```

### Risks & Invariants
* 🔴 Unbounded goroutines: CPU starvation at ~150 req/s on a single pod
* 🔴 Fixed pool, single pod: 429s at ~5.7 × NumCPU logins/s
* 🟢 6 pods × 6 cores = 36 CPU cores → sustains 2,000 req/s with HPA at 60%
* 🟢 Pool blocks callers cleanly — back-pressure is intentional and observable

### SLO Targets
| Metric | Value |
| :--- | :--- |
| POST /oauth/token p99 | 150ms |
| Throughput target | 2,000 req/s |
| Pool size | 2 × NumCPU |
| HPA CPU target | 60% |
| Cores needed | ~35 total |

---

## 8.3. Connector auth: Cache-first credential lookup
**Fast-hash + PBKDF2**

### Problem
Connector auth runs on the hot path of every inbound event. Full PBKDF2 verification (~400ms) for every request would make 20,000 req/s impossible.

### Solution
Split the API key into a prefix (first 8 chars) and secret. SHA-256(prefix) serves as a fast Redis lookup key. Full PBKDF2 runs only on cache miss, or if last_verified_at > 5 min. Suspension writes a sentinel — never deletes — so cached lookups correctly reflect suspension.

### Implementation
```go
// 1. Parse key
prefix, secret := key[:8], key[8:]
fastHash := sha256.Sum256([]byte(prefix))

// 2. Redis hit path (~1ms)
cached := redis.Get("connector:fasthash:" + fastHash)
if cached != nil {
    if cached.Status == "suspended" { return 401 }
    if time.Since(cached.LastVerifiedAt) < 5*time.Minute {
        return authorise(cached) // skip PBKDF2
    }
    // fall through to PBKDF2 re-verify
}

// 3. Cache miss — full PBKDF2 (~400ms, rare)
conn := db.LookupByPBKDF2Hash(key)
redis.Set("connector:fasthash:" + fastHash, conn, 30*time.Second)
```

### Risks & Invariants
* 🔴 Delete on suspension: suspended connectors pass auth until TTL expires (30s gap)
* 🔴 Reactivation delete-only: next request takes the slow PBKDF2 path unnecessarily
* 🟢 Sentinel update on suspension: cached lookups immediately reflect suspended state
* 🟢 Overwrite on reactivation: Redis immediately has the full active connector record

### SLO Targets
| Metric | Value |
| :--- | :--- |
| POST /v1/events/ingest p99 | 50ms |
| Throughput | 20,000 req/s |
| Redis hit latency | ~1ms |
| PBKDF2 (cache miss) | ~400ms |
| Redis TTL | 30s |

---

## 8.4. Kafka consumers: Manual offset commit
**At-least-once + dedup**

### Problem
Auto-commit offsets before the downstream write means a crash after commit but before the MongoDB insert = lost event. The inverse — committing after — means reprocessing on restart, but that is safe.

### Solution
Every consumer uses manual offset commit mode. Offsets are committed only after the downstream write (MongoDB BulkWrite, ClickHouse insert, etc.) is confirmed. Crash before commit = reprocess on restart. The event_id unique index makes reprocessing idempotent.

### Implementation
```go
// Manual commit — never auto-commit
consumer.Config.AutoCommit = false

for msg := range consumer.Messages() {
    // 1. Process — write to MongoDB
    err := mongo.BulkWrite(ctx, docs)
    if err != nil {
        // do NOT commit — retry or DLQ
        consumer.MarkOffset(msg, "failed")
        continue
    }
    // 2. Only commit after confirmed write
    consumer.MarkOffset(msg, "")
    consumer.CommitOffsets()
}
// On restart: reprocess uncommitted msgs
// event_id unique index = idempotent
```

### Risks & Invariants
* 🔴 Auto-commit: offset committed before write = lost events on crash, no recovery
* 🔴 Commit inside write tx: Kafka and MongoDB cannot share a transaction
* 🟢 Manual post-write commit: reprocessing on restart is safe — event_id deduplicates
* 🟢 Bulk 500 / 1s flush: if crash before offset commit, all 500 are safely reprocessed

### SLO Targets
| Metric | Value |
| :--- | :--- |
| Kafka → audit DB p99 | 2s |
| Throughput | 50,000 evt/s |
| Bulk batch size | 500 docs |
| Flush interval | 1s |
| Delivery guarantee | At-least-once + dedup |

---

## 8.5. Token revocation: JTI blocklist with dynamic TTL
**Dynamic expiry**

### Problem
JWTs are stateless — once issued, a valid token stays valid until expiry. A fixed-TTL blocklist entry either outlives the token (wasted memory) or expires before the token (security gap).

### Solution
On revocation, set the Redis blocklist entry TTL to exactly exp − now(). The entry expires the instant the token would have expired anyway. Token validation checks signature → expiry → blocklist in that order. Bulk revocation on user deletion pipelines all active JTIs in one Redis operation.

### Implementation
```go
// Revoke single token
func revokeToken(jti string, exp time.Time) {
    ttl := time.Until(exp)      // dynamic TTL
    if ttl <= 0 { return }      // already expired
    redis.SetEx(
        "jti:"+jti,
        ttl,                    // NOT a fixed value
        "revoked",
    )
}

// Bulk revoke on user deletion
pipe := redis.Pipeline()
for _, session := range activeSessions {
    ttl := time.Until(session.Exp)
    pipe.SetEx("jti:"+session.JTI, ttl, "revoked")
}
pipe.Exec(ctx)

// Validation order (all three must pass)
// 1. signature valid?
// 2. exp not past?  → ErrTokenExpired
// 3. jti in blocklist? → ErrRevoked
```

### Risks & Invariants
* 🔴 Fixed TTL too long: Redis fills with expired-token entries = memory leak
* 🔴 Fixed TTL too short: blocklist entry expires before token → security gap
* 🟢 Dynamic TTL = exp − now(): zero waste, zero gap, self-cleaning
* 🟢 Pipeline bulk revocation: O(1) round trips even for users with many sessions

### SLO Targets
| Metric | Value |
| :--- | :--- |
| Validation order | sig → exp → blocklist |
| Blocklist TTL | exp − now() |
| Forward clock skew | 5 min max |
| Backward skew | Zero tolerated |
| Bulk revocation | Redis pipeline |

---

## 8.6. Provisioning: Choreography saga + deadline
**Deadline + compensation**

### Problem
SCIM user provisioning spans 4 services and 8 events. Any step can fail or stall. Without a timeout mechanism, a user stays in initializing indefinitely and can never log in.

### Solution
Each saga step publishes the next step's trigger. A deadline record is written to a Redis sorted set at saga start (score = unix deadline). A background watcher polls every 10s for expired entries and publishes compensation events. Any step failure also triggers compensation.

### Implementation
```go
// On saga start: write deadline to Redis
deadline := time.Now().Add(
    SAGA_STEP_TIMEOUT + OUTBOX_MAX_LAG, // 40s
)
redis.ZAdd("saga:deadlines",
    float64(deadline.Unix()), sagaID,
)

// Watcher polls every 10s
func watchDeadlines() {
    for range time.Tick(10 * time.Second) {
        expired := redis.ZRangeByScore(
            "saga:deadlines", 0, time.Now().Unix(),
        )
        for _, id := range expired {
            kafka.Publish("saga.timed_out", id)
            redis.ZRem("saga:deadlines", id)
        }
    }
}

// Compensation: any service failure publishes
kafka.Publish("policy.assignment.failed", {
    compensation: true,
    caused_by: eventID,
})
// IAM consumes → sets user status=provisioning_failed
```

### Risks & Invariants
* 🔴 No timeout: user stuck in initializing forever, login permanently denied
* 🔴 Non-idempotent steps: retry on redelivery = duplicate side-effects
* 🟢 Redis sorted set + watcher: O(log n) deadline tracking, 10s detection granularity
* 🟢 provisioning_failed is not terminal — admin can retry via POST /users/:id/reprovision

### SLO Targets
| Metric | Value |
| :--- | :--- |
| Saga timeout | 40s (30+10) |
| Watcher poll interval | 10s |
| Step idempotency | Required |
| Failure recovery | reprovision endpoint |
| Services in saga | 4 (IAM, Policy, Threat, Alert) |
```

---

*Generated from OpenGuard spec §1–2 · Last updated: 2026-04-06*
