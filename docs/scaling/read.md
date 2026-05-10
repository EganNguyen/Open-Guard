# CQRS & Read Scaling at Open Guard

> **Target Audience:** Senior Backend Engineers & System Architects
> **Version:** 1.0 | **Classification:** Engineering Reference

## Table of Contents

1. [Read Architecture Overview](#1-read-architecture-overview)
2. [Policy Evaluation: 3-Tier Read Path](#2-policy-evaluation-3-tier-read-path)
3. [IAM Read Path](#3-iam-read-path)
4. [Audit Read Path](#4-audit-read-path)
5. [Threat Alert Reads](#5-threat-alert-reads)
6. [Compliance Read Path](#6-compliance-read-path)
7. [JWT Authentication: The Universal Read Gate](#7-jwt-authentication-the-universal-read-gate)
8. [Cache Hierarchy](#8-cache-hierarchy)
9. [Read Consistency Guarantees](#9-read-consistency-guarantees)
10. [Scaling Read Capacity](#10-scaling-read-capacity)
11. [CQRS Read Decision Matrix](#11-cqrs-read-decision-matrix)
12. [Operational Considerations](#12-operational-considerations)

---

## 1. Read Architecture Overview

Open Guard's read architecture follows a **tiered, service-specific CQRS model**. Unlike the write side — where the Transactional Outbox provides a uniform pattern — read paths diverge based on latency requirements, data volatility, and query complexity.

### Read Topology

```
                         ┌─────────────────────────────┐
                         │     HTTP Request (Ingress)   │
                         └─────────────┬───────────────┘
                                       │
                         ┌─────────────▼───────────────┐
                         │ AuthJWTWithBlocklist         │
                         │  • JWT verify (in-process)   │
                         │  • Redis blocklist EXISTS    │
                         │  • Context injection (RLS)   │
                         └─────────────┬───────────────┘
                                       │
          ┌────────────────────────────┼────────────────────────────┐
          │                            │                            │
          ▼                            ▼                            ▼
┌─────────────────────┐   ┌─────────────────────┐   ┌─────────────────────────┐
│  Policy Service     │   │  IAM Service        │   │  Audit / Threat /       │
│                     │   │                     │   │  Compliance             │
│  Tier 1: Redis GET  │   │  PostgreSQL (RLS)   │   │                         │
│    (cache key)      │   │  (no caching)       │   │  MongoDB (Secondary)    │
│  Tier 2: Singlefl.  │   │                     │   │  • audit_events         │
│    (dedup)          │   │  Users: direct SQL  │   │  • alerts               │
│  Tier 3: PostgreSQL │   │  Sessions: direct   │   │                         │
│    (RLS + breaker)  │   │  MFA: direct SQL    │   │  ClickHouse             │
│                     │   │  Tokens: direct SQL │   │  • event_counts_daily   │
│  Write-back:        │   │  Connectors: direct │   │  • events FINAL         │
│  Redis SET (cache)  │   │                     │   │                         │
│  + org index SADD   │   │                     │   │  PostgreSQL (metadata)  │
│                     │   │                     │   │  • reports              │
└─────────────────────┘   └─────────────────────┘   └─────────────────────────┘
```

### Read Path Spectrum

| Service | Cache | Primary Read Store | Query Pattern | p99 Latency Target |
|---------|-------|-------------------|---------------|-------------------|
| Policy Evaluation | Redis (60s TTL + stale-while-revalidate) | PostgreSQL (RLS) | Point lookup by hash key | < 5ms |
| Policy CRUD | None | PostgreSQL (RLS) | Direct SQL | < 20ms |
| IAM (users, sessions, tokens) | None | PostgreSQL (RLS) | Direct SQL | < 20ms |
| Audit events | None | MongoDB (SecondaryPreferred) | Paginated scan | < 50ms |
| Audit SSE stream | None | MongoDB Change Streams | Real-time watch | < 100ms |
| Threat alerts | None | MongoDB (Primary — gap) | Cursor-paginated + aggregation | < 100ms |
| Compliance posture | None | ClickHouse (ReplacingMergeTree) | Analytical aggregation | < 1s |
| Compliance stats | None | ClickHouse (SummingMergeTree MV) | Pre-aggregated read | < 200ms |
| Compliance reports | None | PostgreSQL (metadata) + S3 | Direct SQL + presigned URL | < 500ms |

---

## 2. Policy Evaluation: 3-Tier Read Path

**Location:** `services/policy/pkg/service/service.go`

Policy evaluation is the hottest read path — every authorization decision flows through it. It uses a **3-tier cache hierarchy** with stale-while-revalidate.

### Flow Diagram

```
Evaluate(ctx, {subject, action, resource})
    │
    ├── 1. Compute cache key
    │     key = SHA256(orgID + subjectID + groups + action + resource)
    │
    ├── 2. Redis GET (cache key)                                  ← Tier 1
    │     ├── Fresh hit (ExpiresAt > now) → return cached decision
    │     ├── Stale hit (within 5s grace) → return stale + background refresh
    │     └── Miss → continue
    │
    ├── 3. Singleflight Group (key)                                ← Tier 2
    │     ├── First caller → proceed to DB
    │     └── Concurrent callers → share result (dedup)
    │
    ├── 4. PostgreSQL (circuit-breaker protected)                  ← Tier 3
    │     ├── SELECT matching policies (RLS scoped by org_id)
    │     ├── Evaluate rules: deny_all → allow_all → rbac → cel
    │     ├── Default: "deny" (fail-closed)
    │     └── Write-back: Redis SET + SADD to org index
    │
    └── Return EvaluationResponse { decision, matched_policies, ttl }
```

### Tier 1: Redis Cache (Cache-Aside + Stale-While-Revalidate)

**Lines 175-208:**

```go
// Stale-while-revalidate pattern
if time.Now().Before(decision.ExpiresAt) {
    return decision, "redis", nil  // Fresh hit
}
if time.Now().Before(decision.StaleAt) {
    go s.backgroundRefresh(ctx, key, req)  // Stale: return + async refresh
    return decision, "redis(stale)", nil
}
// Miss: continue to singleflight + DB
```

Configuration:
- `cacheTTL`: 60s (line 28) — how long a decision is considered fresh
- `staleWindow`: 5s (line 29) — grace period where stale is served while refreshing
- `refreshSem`: buffered channel of 100 (line 133) — limits concurrent background refreshes
- Stale-while-revalidate ensures p99 latency stays under 5ms even during cache-miss storms

**Cache key design** (lines 151-163):

```
policy:eval:{orgID}:SHA256(orgID + subjectID + groups + action + resource + contextHash)
```

The hash prevents key-length issues and makes keys uniformly distributed across Redis hash slots.

### Tier 2: Singleflight (Deduplication)

**Lines 213-242:**

```go
ch := s.sfGroup.DoChan(key, func() (interface{}, error) {
    return s.evaluateFromDB(ctx, req)
})
```

`singleflight.Group` ensures that when N concurrent requests miss cache for the same key, only one goroutine hits PostgreSQL. The other N-1 goroutines share the result. This prevents thundering herds on cache expiration.

### Tier 3: PostgreSQL (Circuit-Breaker Protected)

**Lines 246-289** (`evaluateFromDB`):

```go
result, err := resilience.Call(ctx, s.dbBreaker, func() (interface{}, error) {
    ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
    defer cancel()
    return s.repo.GetMatchingPolicies(ctx, req.OrgID, ...)
})
```

- **Circuit breaker** (`dbBreaker`, lines 126-132): 10 consecutive failures → open for 30s
- **Timeout**: 50ms per call — fast failure prevents connection pool exhaustion
- **Fail-closed**: If DB is unreachable, returns `"deny"` with empty policy IDs (lines 252-257)

### Rule Evaluation Engine

**Lines 298-388** (`evaluate()` method):

```
Rules applied in order (first match wins):
  1. deny_all   → immediate deny (explicit blacklist)
  2. allow_all  → explicit allow
  3. rbac       → glob-style matching (subjects, actions, resources)
  4. cel        → Common Expression Language evaluation (compiled against cached cel.Env)

Default: "deny" (fail-closed — if no rule matches, access is denied)
Explicit deny always overrides allow (deny-first semantics)
```

### Cache Invalidation

**Lines 543-565** (`InvalidateOrgCache`):

```go
func (s *Service) InvalidateOrgCache(ctx context.Context, orgID string) {
    keys, _ := s.rdb.SMembers(ctx, "policy:index:"+orgID).Result()
    if len(keys) > 0 {
        s.rdb.Del(ctx, keys...)
        s.rdb.Del(ctx, "policy:index:"+orgID)
    }
}
```

Triggered by:
- Policy mutations (create, update, delete) — published via `policy.changes` Kafka topic
- Assignment mutations (create, delete) — background goroutine (`CreateAssignment`, `DeleteAssignment`)
- The `policy:index:{orgID}` Redis set tracks all cached decision keys per org, enabling bulk invalidation

### CRUD Reads (Non-Cached)

**`GetPolicy`, `ListPolicies`, `ListAssignments`, `ListEvalLogs`** — all delegate directly to PostgreSQL. No caching. The assumption is these are low-frequency management operations, not hot-path authorization checks.

---

## 3. IAM Read Path

**Location:** `services/iam/pkg/service/`

IAM reads are **uncached** — every query hits PostgreSQL directly. This is intentional: user/session/token data is write-frequently and consistency-sensitive.

### Read Methods

| Method | File | Query | RLS |
|--------|------|-------|-----|
| `GetCurrentUser` | `users.go:222` | `SELECT ... FROM users WHERE id = $1` | Yes (`withOrgContext`) |
| `ListUsers` | `users.go:226` | `SELECT ... FROM users WHERE org_id = $1` | Conditional |
| `ListUsersPaginated` | `users.go:230` | Same + `COUNT(*) OFFSET/LIMIT` | Conditional |
| `GetUserByEmail` | `repository_user.go:52` | `SELECT id, password_hash, ... FROM users WHERE email = $1` | No (`SET ROLE openguard_login`) |
| `GetSessionByUserID` | `repository_session.go:79` | `SELECT jti, ... FROM sessions WHERE user_id = $1 AND expires_at > NOW()` | Yes |
| `GetActiveJTIs` | `repository_session.go:25` | `SELECT jti FROM sessions WHERE user_id = $1 AND expires_at > NOW()` | Yes |
| `GetSessionTTL` | `repository_session.go:49` | `SELECT expires_at FROM sessions WHERE jti = $1` | Yes |
| `GetRefreshToken` | `repository_token.go:26` | `SELECT ... FROM refresh_tokens WHERE token_hash = $1` | Yes |
| `GetMFAConfig` | `repository_mfa.go:13` | `SELECT secret_encrypted FROM mfa_configs WHERE user_id = $1` | Yes |
| `ListMFAConfigs` | `repository_mfa.go:34` | `SELECT ... FROM mfa_configs WHERE user_id = $1` | Yes |
| `GetSAMLProvider` | `repository_saml.go:50` | `SELECT ... FROM saml_providers WHERE org_id = $1` | Yes |
| `ListSAMLProviders` | `repository_saml.go:69` | `SELECT ... FROM saml_providers WHERE org_id = $1 ORDER BY created_at DESC` | Yes |
| `ListConnectors` | `repository_connector.go:41` | `SELECT ... FROM connectors WHERE org_id = $1` | Conditional |
| `ListWebAuthnCredentials` | `repository_webauthn.go:25` | `SELECT ... FROM webauthn_credentials WHERE user_id = $1` | Yes |

### RLS Bypass Paths

Some read operations bypass RLS for system-level access:

| Operation | Bypass Mechanism | File & Line |
|-----------|-----------------|-------------|
| `GetUserByEmail` | `SET ROLE openguard_login` (login role) | `repository_user.go:65` |
| `ListUsers` (system org) | `SET ROLE openguard_login` | `repository_user.go:196` |
| `ListConnectors` (system admin) | `SET ROLE openguard_login` | `repository_connector.go:62` |

### Session Validation Read Path

On every authenticated request, the JWT middleware checks Redis (`blocklist:{jti}`). IAM services also validate session data via PostgreSQL `GetSessionByUserID` for sensitive operations (refresh token rotation, MFA verification).

---

## 4. Audit Read Path

**Location:** `services/audit/pkg/repository/repository.go`

Audit reads use MongoDB with **SecondaryPreferred** read preference — the most explicit CQRS split in the system.

### MongoDB CQRS Configuration

**Location:** `services/audit/main.go:71-105`

```go
// Primary (Writes): majority write concern
wc := writeconcern.Majority()
writeOpts := options.Client().ApplyURI(primaryURI).SetWriteConcern(wc)
writeClient, _ := mongo.Connect(ctx, writeOpts)

// Secondary (Reads): route to secondaries for read isolation
rp := readpref.SecondaryPreferred()
readOpts := options.Client().ApplyURI(secondaryURI).SetReadPreference(rp)
readClient, _ := mongo.Connect(ctx, readOpts)

writeRepo := repository.NewAuditWriteRepository(writeClient, "openguard_audit")
readRepo := repository.NewAuditReadRepository(readClient, "openguard_audit")
```

### Read Repository

**Location:** `services/audit/pkg/repository/repository.go:16-18,93-127`

```go
type AuditReadRepository struct {
    db *mongo.Database
}

func (r *AuditReadRepository) FindEvents(ctx context.Context, filter interface{}, limit int64, skip int64) ([]map[string]interface{}, error) {
    cur, err := r.db.Collection("audit_events").Find(ctx, filter,
        options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.M{"timestamp": -1}))
    // ...
}
```

- `FindEvents` — paginated queries with sort by `timestamp DESC`
- `GetLatestHash` — reads hash chain for integrity verification from `hash_chains` collection

### SSE Change Stream Read

**Location:** `services/audit/pkg/handlers/sse.go:56-97`

```go
pipeline := mongo.Pipeline{
    {{"$match", bson.M{"fullDocument.org_id": orgID}}},
}
cs, err := collection.Watch(ctx, pipeline, options.ChangeStream().SetFullDocument(options.UpdateLookup))
```

MongoDB Change Streams provide real-time event streaming. The pipeline filters by `org_id` and emits SSE `data:` lines with `event_id` as the `id:` field.

### Index Strategy

| Collection | Index | Purpose |
|-----------|-------|---------|
| `audit_events` | Unique sparse on `event_id` | Idempotent inserts (consumer dedup) |
| `hash_chains` | Primary on `org_id` (upsert) | Integrity chain lookup |

**Notable gap:** No query-pattern indexes on `org_id`, `actor_id`, `timestamp` for the `FindEvents` path. At scale, paginated scans of the `audit_events` collection will degrade — compound indexes are required for production.

---

## 5. Threat Alert Reads

**Location:** `services/threat/pkg/alert/alert.go`

Threat alerts are read from MongoDB. **Unlike the Audit service, the Threat service reads from Primary — no `SecondaryPreferred` is configured.** This is a gap in the current CQRS implementation.

### Read Operations

| Method | Lines | Query | Notes |
|--------|-------|-------|-------|
| `GetAlert` | 65-73 | `FindOne({"_id": oid})` | Point lookup by ObjectID |
| `ListAlerts` | 75-109 | `Find({org_id, [status], [severity], [_id: {$lt: cursor}]})` | Cursor-based pagination, sort by `_id DESC` |
| `GetStats` | 143-173 | `Aggregate([{$match: {org_id}}, {$group: {_id: "$severity", count, avg_mttr_sec}}])` | MongoDB aggregation pipeline |

### CQRS Gap

| Aspect | Audit | Threat |
|--------|-------|--------|
| Read preference | `SecondaryPreferred` | **Primary (default)** |
| Write connection | Majority write concern | Majority write concern |
| Read isolation | Reads isolated from write load | **Reads compete with writes** |

**Recommendation:** The Threat service should adopt the same CQRS pattern as Audit — connect with `SecondaryPreferred` for `GetAlert`/`ListAlerts`/`GetStats` to offload read traffic from the MongoDB primary.

---

## 6. Compliance Read Path

**Location:** `services/compliance/pkg/repository/repository.go`

Compliance reads are split across two engines: **ClickHouse** for analytical queries and **PostgreSQL** for report metadata.

### ClickHouse Analytical Reads

ClickHouse is optimized for analytical read patterns — columnar storage, pre-aggregated materialized views, and FINAL modifier for deduplication.

#### `GetStats` — Pre-Aggregated via SummingMergeTree MV

**Lines 131-152:**

```go
func (r *Repository) GetStats(ctx context.Context, orgID string, from, to time.Time) (map[string]interface{}, error) {
    query := `SELECT type, sum(cnt) as total
              FROM event_counts_daily
              WHERE org_id = ? AND day BETWEEN ? AND ?
              GROUP BY type ORDER BY total DESC`
    // SummingMergeTree materialized view — pre-aggregated by (org_id, type, day)
}
```

- Reads from `event_counts_daily` — a **SummingMergeTree** materialized view
- Pre-aggregated by `(org_id, type, day)` — queries avoid scanning raw events
- Response time: **p99 < 200ms** even over billions of events

#### `GetPosture` — Raw Event Scan with FINAL

**Lines 154-187:**

```go
func (r *Repository) GetPosture(ctx context.Context, orgID string) (map[string]interface{}, error) {
    query := `SELECT countIf(type LIKE 'auth.%'), countIf(type LIKE 'policy.%'), ...
              FROM events FINAL
              WHERE org_id = ? AND occurred_at > now() - INTERVAL 30 DAY`
    // ReplacingMergeTree — FINAL deduplicates at query time
}
```

- Reads from `events` (ReplacingMergeTree) with `FINAL` modifier — deduplicates at query time
- 30-day window — bounded scan
- `countIf` — ClickHouse aggregate function, avoids subqueries
- Response time: **p99 < 1s** for most tenants

#### ClickHouse Schema

```sql
CREATE TABLE events (
    event_id    String,
    org_id      String,
    type        String,
    occurred_at DateTime,
    ...
) ENGINE = ReplacingMergeTree(occurred_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (org_id, type, occurred_at, event_id)
TTL occurred_at + INTERVAL 2 YEAR

CREATE MATERIALIZED VIEW event_counts_daily
ENGINE = SummingMergeTree
ORDER BY (org_id, type, day)
AS SELECT org_id, type, toDate(occurred_at) as day, count(*) as cnt
   FROM events GROUP BY org_id, type, day
```

### PostgreSQL Report Metadata

| Method | Query | Purpose |
|--------|-------|---------|
| `GetReport` | `SELECT ... FROM reports WHERE id = $1 AND org_id = $2` | Report status check |
| `ListReports` | `SELECT ... FROM reports WHERE org_id = $1 ORDER BY created_at DESC` | Report listing |
| `GetPendingReports` | `SELECT ... FROM reports WHERE status = 'pending' ORDER BY created_at ASC` | Scheduler polling |

### Download Read Path (S3 Presigned URL)

**Location:** `services/compliance/pkg/storage/s3.go:55-63`

```go
func (s *S3Storage) GetPresignedURL(ctx context.Context, key string) (string, error) {
    req, _ := s.s3.GetObjectRequest(&s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
    return req.Presign(1 * time.Hour)
}
```

The download handler reads the report status from PostgreSQL, then issues a 302 redirect to a presigned S3 URL (1-hour TTL). This avoids proxying large files through the service.

---

## 7. JWT Authentication: The Universal Read Gate

**Location:** `shared/middleware/jwt_auth.go`

Every authenticated read request in every service passes through this middleware. It is the **first read operation** on every request path.

### Middleware Pipeline

```
Request ──► JWT Parse ──► Signature Verify ──► Redis EXISTS (blocklist) ──► Context Injection ──► Handler
                 │                │                       │                        │
            in-process      in-process                0.3ms (L2)           in-process
            (no I/O)        (crypto ops)           circuit-brokered
```

```go
func AuthJWTWithBlocklist(keyring *crypto.Keyring, rdb *redis.Client, breaker *gobreaker.CircuitBreaker) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. JWT parse + signature verify (in-process crypto)
            token, err := keyring.Verify(r.Header.Get("Authorization"))

            // 2. Redis blocklist check (circuit-brokered)
            blocked, err := resilience.Call(ctx, breaker, func() (interface{}, error) {
                return rdb.Exists(ctx, "blocklist:"+jti).Result()
            })

            // 3. Inject user context for downstream services
            ctx = rls.WithOrgID(ctx, orgID)
            ctx = context.WithValue(ctx, middleware.CtxKeyUserID, userID)
            // ...
        })
    }
}
```

### Circuit Breaker Behavior

| State | Behavior | Latency Impact |
|-------|----------|---------------|
| **Closed** (normal) | Redis EXISTS called normally | ~0.3ms |
| **Open** (Redis down) | Returns cached error, allows request | ~0ms (fail-open) |
| **Half-open** (probing) | Allows single request through | ~0.3ms (if succeeds, close; if fails, open) |

**Design rationale:** The blocklist check is fail-open — if Redis is unreachable, the middleware logs a warning and allows the request. This prevents a Redis outage from blocking all authenticated traffic. The security trade-off is that revoked tokens may be accepted until Redis recovers.

---

## 8. Cache Hierarchy

### Current State

| Layer | Technology | Services | Hit Rate | Latency |
|-------|-----------|----------|----------|---------|
| L1 (in-process) | **None** | — | 0% | N/A |
| L2 (distributed) | Redis | Policy (eval cache), All (blocklist) | Target: 95%+ | ~0.3ms |
| L3 (database) | PostgreSQL / MongoDB / ClickHouse | All | ~5% | 5-50ms |

**Notable:** There is **no in-process (L1) cache** anywhere in the codebase. The closest patterns are:
- `singleflight.Group` in Policy service — deduplicates concurrent in-flight requests, but does not cache across requests
- CEL environment singleton — compiled once, reused for all evaluations
- Circuit breakers — fail-fast, not caching

### Redis Cache Footprint

| Key Pattern | Service | TTL | Invalidation | Purpose |
|------------|---------|-----|-------------|---------|
| `policy:eval:{orgID}:{hash}` | Policy | 60s + 5s stale | On policy mutation (via `InvalidateOrgCache`) | Authorization decisions |
| `policy:index:{orgID}` | Policy | 24h | On policy mutation (tracked set) | Cache key registry for bulk invalidation |
| `blocklist:{jti}` | All | Per-token TTL | On logout/revocation | Token revocation |
| `mfa_challenge:{token}` | IAM | 5min | On challenge completion | MFA challenge state |
| `totp:used:{userID}:{code}` | IAM | 90s | Auto-expire | Replay prevention |
| `auth_code:{code}` | IAM | 10min | On code consumption | OAuth2 PKCE |
| `saml:replay:{messageID}` | IAM | Assertion TTL | Auto-expire | SAML replay prevention |
| Idempotency keys | All | 24h | Auto-expire | Request deduplication |
| Rate limit counters | All | Per-window | Auto-expire | Rate limiting |

### Future: L1 Cache Introduction

When the system outgrows Redis-based caching alone, introduce L1 caching:

| Tier | Technology | TTL | Hit Rate Target | Pattern |
|------|-----------|-----|----------------|---------|
| L1 | `ristretto` / `bigcache` | 5-10s | 70% | In-process, per pod |
| L2 | Redis Cluster | 60s | 25% | Distributed, shared across pods |
| L3 | Database | — | 5% | Source of truth |

L1 caching depends on the cache invalidation mechanism — the current design relies on Redis pub/sub (`policy.changes` Kafka topic → consumer → cache flush). An L1 layer would need a similar invalidation channel (e.g., Redis pub/sub or WebSocket broadcast).

---

## 9. Read Consistency Guarantees

### Consistency Matrix

| Read Path | Consistency Level | Replication Lag | Stale Read Window | Notes |
|-----------|------------------|----------------|-------------------|-------|
| Policy eval (Redis fresh hit) | Strong (single-node) | 0 | 0 | Same Redis node |
| Policy eval (Redis stale hit) | Weak | 0-5s | 5s (stale window) | Acceptable for authz |
| Policy eval (PostgreSQL) | Strong (read-your-writes) | 0 | 0 | Same TX |
| IAM user/session/token (PG) | Strong (read-your-writes) | 0 | 0 | Same connection |
| Audit events (MongoDB Secondary) | Eventual | 10-100ms | ~100ms | SecondaryPreferred |
| Threat alerts (MongoDB Primary) | Strong | 0 | 0 | Primary read — no lag |
| Compliance stats (ClickHouse MV) | Eventual | 1-10s | Up to 10s | SummingMergeTree MV refresh |
| Compliance posture (ClickHouse FINAL) | Eventual | 1-5s | Up to 5s | ReplacingMergeTree dedup |

### Read-Your-Writes Guarantee

The system guarantees **read-your-writes** for:

- **IAM and Policy**: Write to PostgreSQL → subsequent reads from the same connection see the write (strong consistency)
- **Audit**: Write to Kafka → subsequent read may not see the event until the consumer flushes to MongoDB (eventual consistency, typically < 1s)
- **Compliance**: Write to Kafka → ClickHouse consumer → subsequent read may see stale data until the MV refreshes (eventual, typically < 10s)

### Stale Read Tolerance

| Data Type | Stale Tolerated | Reason |
|-----------|----------------|--------|
| Authorization decision | Up to 5s (stale window) | Cache-aside with background refresh — acceptable for authz latency |
| User profile | 0 (no cache) | Identity data must be current |
| Session state | 0 (no cache) | Security-critical |
| Audit event | ~100ms | Async pipeline — eventual consistency accepted |
| Compliance report | Minutes | Long-running report generation — staleness inherent |
| Threat alert | ~100ms | Near-real-time detection — sub-second acceptable |

---

## 10. Scaling Read Capacity

### 10a. Redis Read Scaling

**Current:** Single Redis instance.

**At scale (Tier 2+):** Redis Cluster with read replicas.

See `docs/scaling/redis.md` for the full Redis scaling strategy, including:
- Bloom filter for blocklist EXISTS (99.9% reduction in Redis reads)
- Hash tags for slot affinity
- Read replica routing

### 10b. PostgreSQL Read Scaling

**Options:**

| Strategy | Complexity | Read Throughput Gain | Consistency Impact |
|----------|-----------|---------------------|-------------------|
| Connection pooling (PgBouncer) | Low | 5x (more concurrent reads) | None |
| Read replicas + statement routing | Medium | 10x | Eventual (replica lag) |
| Connection pool per replica + weight-based routing | Medium | 10x with HA | Eventual |
| Sharding (Citus / Vitess) | High | 100x | Strong within shard |

**Recommended approach** (from `docs/scaling/scale.md`):
1. Add PgBouncer sidecar per pod (connection pooling)
2. Add read replicas for IAM listing queries
3. Route heavy analytical queries (report generation) to replicas

### 10c. MongoDB Read Scaling

| Strategy | Read Throughput | Consistency |
|----------|----------------|-------------|
| SecondaryPreferred (current for Audit) | 3x (with 3-node replica set) | Eventual |
| Sharded cluster (shard key: org_id) | 100x | Eventual within shard |
| Read-only secondary nodes | 10x | Eventual |

**Priority for MongoDB reads:**
1. Ensure Threat service uses `SecondaryPreferred` (closing the CQRS gap)
2. Add compound indexes for the query patterns used by `FindEvents` (org_id + timestamp)
3. If read contention grows beyond replica set capacity, shard by `org_id`

### 10d. ClickHouse Read Scaling

ClickHouse scales reads natively:

- **Columnar storage**: Reads only the columns needed (`type`, `cnt`, etc.) — no full-row scan
- **Pre-aggregated MVs**: `event_counts_daily` serves `GetStats` without touching raw data
- **Partition pruning**: `PARTITION BY toYYYYMM(occurred_at)` — queries with date filters scan only relevant partitions
- **Distributed tables**: With multiple ClickHouse nodes, queries scatter-gather across shards transparently

### 10e. Read Path Throughput Targets

| Read Path | Current Capacity | Target Capacity | Scaling Mechanism |
|-----------|-----------------|----------------|-------------------|
| Policy evaluation | 50K/s per Redis node | 2M/s | Redis Cluster + L1 cache |
| IAM user queries | 10K/s per PG node | 100K/s | Read replicas + PgBouncer |
| Audit event queries | 5K/s per MongoDB replica | 50K/s | Compound indexes + sharding |
| Compliance stats | 1K/s per ClickHouse node | 50K/s | Distributed ClickHouse |
| JWT blocklist check | 100K/s per Redis node | 5M/s | Bloom filter (Tier 3) |

---

## 11. CQRS Read Decision Matrix

| Operation | Read Source | Cache | CQRS Separation | Why This Design |
|-----------|------------|-------|----------------|-----------------|
| Policy evaluation | Redis → PostgreSQL | L2 only (Redis) | Logical (same service) | Latency-critical: cache absorbs 95%+ of reads |
| Policy CRUD | PostgreSQL | None | None (same repo) | Low-frequency management |
| IAM user list/get | PostgreSQL | None | None (same repo) | Consistency-critical: no stale identity data |
| IAM login/auth | PostgreSQL | Redis (blocklist only) | None (same repo) | Auth path must read fresh data |
| IAM session/refresh | PostgreSQL | None | None (same repo) | Security-critical: rotation requires fresh reads |
| Audit event browse | MongoDB (Secondary) | None | **Physical** (separate repo + connection) | Write-isolated: secondary reads don't compete with primary writes |
| Audit SSE stream | MongoDB (Change Streams) | None | Logical (same cluster, separate connection) | Real-time: changes streamed from primary op log |
| Threat alert list | MongoDB (Primary — gap) | None | None (same connection) | Needs SecondaryPreferred fix |
| Compliance stats | ClickHouse MV | None | **Storage** (separate engine) | Pre-aggregated reads: optimized for analytical queries |
| Compliance posture | ClickHouse (FINAL) | None | **Storage** (separate engine) | Columnar scan: efficient for bounded-window aggregates |
| Report metadata | PostgreSQL | None | None (same repo) | Simple CRUD, low volume |
| Report download | S3 (presigned URL) | None | **Storage** (object store) | Large file offload: avoids proxying through service |

### When We Skip Read Caching

| Skip Reason | Examples | Risk if Unchanged |
|-------------|---------|-------------------|
| Consistency-sensitive | IAM users, sessions, tokens | Auth bypass if stale |
| Low-frequency (< 100/s) | Policy CRUD, report metadata | None |
| Already optimized at storage layer | ClickHouse MVs | None |
| Real-time requirement | Audit SSE, threat alerts | Stale alerts |

---

## 12. Operational Considerations

### Read-Side Monitoring

| Metric | What It Tells You | Target |
|--------|-------------------|--------|
| `redis_cache_hit_ratio` | Policy eval cache effectiveness | > 95% |
| `policy_eval_duration_ms{p99}` | End-to-end evaluation latency | < 5ms |
| `pg_query_duration_ms{p99}` | PostgreSQL query performance | < 20ms |
| `mongodb_read_latency_ms{p99}` | MongoDB read performance | < 50ms |
| `clickhouse_query_duration_ms{p99}` | Analytical query performance | < 1s |
| `jwt_blocklist_exists_latency` | Redis blocklist check latency | < 1ms |
| `circuit_breaker_state` | PostgreSQL circuit breaker | Closed |

### Common Read Path Failures

| Failure Mode | Symptom | Mitigation |
|-------------|---------|-----------|
| Redis down | Policy eval falls through to PG every time; blocklist check opens circuit | Circuit breaker (fail-open for blocklist, fail-closed for eval) |
| PostgreSQL replica lag | IAM reads return stale data | Route critical reads to primary (already done) |
| MongoDB replica lag | Audit reads miss recent events | SecondaryPreferred tolerates lag; primary read for hash chain verification |
| ClickHouse MV staleness | Compliance stats show outdated counts | SummingMergeTree processes parts in background — eventual consistency is by design |
| Cache stampede (policy eval) | N requests miss cache simultaneously, all hit PG | singleflight deduplication prevents thundering herd |

### Read Path Testing

| Test | What It Validates |
|------|------------------|
| Cache hit/miss behavior | Policy eval returns correct decision from Redis vs PG |
| Stale-while-revalidate | Background refresh completes without blocking the response |
| Singleflight dedup | N concurrent calls with the same key result in 1 PG query |
| Circuit breaker open/close | PG outage causes fail-closed deny, recovers when PG returns |
| MongoDB read preference | Audit queries route to secondary; verify no `notMaster` errors |
| RLS isolation | Cross-tenant reads return empty results |

### Production Readiness Checklist

- [ ] Redis eval cache hit ratio > 95% (if below, tune TTL or investigate cache key collisions)
- [ ] MongoDB read preference set to `SecondaryPreferred` for both Audit and Threat services
- [ ] Compound indexes on MongoDB matching query patterns (org_id + timestamp)
- [ ] Circuit breaker configured for all PostgreSQL read operations
- [ ] PgBouncer sidecar for PostgreSQL connection pooling
- [ ] ClickHouse MV processing lag < 30s
- [ ] Blocklist EXISTS p99 < 1ms

---

## References

| Document | What It Covers |
|----------|---------------|
| `docs/scaling/write.md` | Write-side CQRS, Transactional Outbox |
| `docs/scaling/scale.md` | Overall scaling strategy, sharding, multi-tenancy |
| `docs/scaling/eda.md` | Event-driven architecture, outbox relay details |
| `docs/scaling/redis.md` | Redis scaling, cluster migration, bloom filters |
| `docs/index/ARCHITECTURE.md` | Core design patterns |
| `services/policy/pkg/service/service.go` | Policy evaluation 3-tier cache |
| `services/audit/main.go` | MongoDB CQRS split configuration |
| `shared/middleware/jwt_auth.go` | JWT authentication middleware with Redis blocklist |
| `shared/middleware/context.go` | Context accessor functions |
| `shared/rls/context.go` | RLS session variable management |
