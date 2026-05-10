# Redis Scaling Strategies

## Current Redis Footprint

Every service in Open-Guard connects to a single Redis instance. The following table catalogues all key patterns, their purpose, TTL, and per-request access cost.

### Key Patterns by Service

| Service | Key Pattern | Purpose | TTL | Access Per Request |
|---------|-------------|---------|-----|-------------------|
| **IAM** | `blocklist:{jti}` | JWT revocation (per-request check by all services) | Remaining token lifetime (max 1h) | 1 EXISTS |
| **All** (via `shared/middleware/jwt_auth.go`) | `blocklist:{jti}` | Same key, checked by 8 services | Same | 1 EXISTS per service |
| **IAM** | `mfa_challenge:{token}` | MFA challenge session | 5min | 1 SET, 1 GETDEL |
| **IAM** | `totp:used:{userID}:{code}` | TOTP nonce replay prevention | 90s | 1 SETNX |
| **IAM** | `webauthn:reg:{userID}` | WebAuthn registration session | 5min | 1 SET, 1 GETDEL |
| **IAM** | `webauthn:login:{userID}` | WebAuthn login session | 5min | 1 SET, 1 GETDEL |
| **IAM** | `auth_code:{code}` | OAuth2 authorization code | 10min | 1 SET, 1 GETDEL |
| **IAM** | `saml:replay:{messageID}` | SAML assertion replay prevention | Configurable | 1 SETNX |
| **IAM** | `saga:deadlines` | Sorted set for provisioning deadlines | 40s polling | Lua script per poll |
| **IAM** | `blocklist:{jti}` (via `users.go`) | Bulk blocklist on user delete/offboard | Remaining lifetime | Pipeline per user |
| **Rate Limiter** | Rate limit counts | Sliding window rate limiting per client | Per-window | Pipeline per request |
| **Idempotency** | Idempotency keys | Request deduplication for POST/PUT/PATCH | 24h | 1 GET, 1 SET |
| **Migration Lock** | `migration:lock` | Distributed migration lock | 60s | 1 SETNX, periodic EXISTS |
| **Threat detectors** | Threat-specific counters | Brute force, off-hours, data exfiltration tracking | 5min–24h | Per event |

### Current Bottleneck

The **blocklist EXISTS** is the highest-frequency access pattern. Every API request across all 8 services that authenticate via JWT executes:

```
rdb.Exists(ctx, "blocklist:"+jti)
```

At 1M req/s, this is **8M EXISTS/s** to a single Redis node. Redis single-threaded command execution caps at ~100-200K ops/s for EXISTS (simple O(1)). This is the primary scaling constraint.

Secondary patterns (MFA challenges, rate limiting, threat detection) add load but are typically an order of magnitude lower volume.

---

## Scale Tiers

### Tier 1: Single Instance (current) — up to ~100K req/s

A single Redis instance (c6g.large or equivalent) handles the full workload with headroom for failover.

- No changes needed.
- Tune `maxclients` and connection pooling per service.
- Enable RDB/AOF persistence with `appendfsync everysec`.

### Tier 2: Redis Cluster with Read Replicas — up to ~500K req/s

**When:** Blocklist EXISTS saturates the single node. Rate limiting and threat counters add pressure.

**Architecture:**

```
                   ┌──────────────┐
                   │  Redis Cluster │  (3 masters, 3 replicas per master)
                   │  Hash slot =   │
                   │  CRC16(JTI)    │
                   └──┬────┬────┬──┘
                      │    │    │
              ┌───────┘    │    └───────┐
              │            │            │
         ┌────▼───┐   ┌───▼────┐   ┌───▼────┐
         │ Master │   │ Master │   │ Master │
         │ A      │   │ B      │   │ C      │
         └───┬────┘   └───┬────┘   └───┬────┘
             │            │            │
        ┌────▼───┐   ┌───▼────┐   ┌───▼────┐
        │Replica │   │Replica │   │Replica │
        │ A-1    │   │ B-1    │   │ C-1    │
        └────────┘   └────────┘   └────────┘
```

**Changes required:**

1. **Blocklist EXISTS → choose consistency mode.** You must decide between two modes (see section below).
2. **Pipelines → cross-slot caution.** Bulk operations (user delete, org offboard) pipeline SETs for multiple JTIs. If JTIs hash to different slots, the pipeline must use `{hash}tag` or the client sends per-slot commands.
3. **Circuit breaker retuning.** Per-node breakers, not a single global breaker. A single replica failure does not disable the blocklist check entirely.
4. **GoRedis cluster client.** Replace `redis.NewClient` with `redis.NewClusterClient` across all services.

**Trade-off:** Cluster mode adds ~2-5ms latency per hop (client → proxy → shard). Client must handle MOVED/ASK redirects. GoRedis cluster client handles this transparently, but connection management overhead increases.

### Consistency vs Performance Mode

Redis master-replica replication is **asynchronous**. When you write `SET blocklist:abc` to the master, the replica does not have it yet. If the next API request reads `EXISTS` from the replica, it sees `0` — the revoked token is temporarily accepted.

You choose between two modes:

#### Mode A: Consistency over Performance

Route `EXISTS` to the **master**. The read sees the write immediately because it hits the same node.

```
Revocation:      Admin → IAM → SET master (immediate)
Verification:    User  → svc → EXISTS master → sees 1 → 401
                                 ↑ same node, consistent
```

**Configuration — GoRedis cluster client:**

```go
client := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs: []string{"master-a:6379", "master-b:6379", "master-c:6379"},
    // No RouteByLatency: EXISTS goes to master by default
})
```

| Pros | Cons |
|------|------|
| Zero revocation delay | Master handles ALL EXISTS traffic |
| Simple, no replication lag concern | ~33% of cluster capacity for reads (3 masters vs 6 total nodes) |
| No consistency edge cases | |

#### Mode B: Performance over Consistency

Route `EXISTS` to **replicas**, spreading read load. Accept that a revoked token may pass until the replica catches up (typical window: ~0.5ms same-AZ, potentially more cross-AZ).

```
Revocation:      Admin → IAM → SET master → async replication stream → replica
Verification:    User  → svc → EXISTS replica → may see 0 → ALLOW (window)
                                             ↑ replica lag (~0.5ms)
```

To **minimize the window**, use `WAIT` on revocation writes:

```go
// On revocation (Logout, DeleteUser):
pipe := s.rdb.Pipeline()
pipe.Set(ctx, "blocklist:"+jti, "revoked", ttl)
pipe.Wait(ctx, 1, 1000)  // Wait for ≥1 replica to acknowledge, max 1000ms
pipe.Exec(ctx)
```

`WAIT` blocks the write until the specified number of replicas confirm they received the command. This closes the window from "unknown replica lag" to the **network RTT to the replica** (~0.5ms).

**Configuration — GoRedis cluster client:**

```go
client := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs:          []string{"master-a:6379", "master-b:6379", "master-c:6379"},
    RouteByLatency: true, // Route reads to lowest-latency node (likely replica)
    ReadOnly:       true, // Allow reads from replicas
})
```

| Pros | Cons |
|------|------|
| Distributes read load across all 6 nodes | ~0.5ms revocation window (without WAIT) |
| Lower latency for geo-distributed replicas | WAIT adds +RTT to write path (~0.5ms) |
| Master free to focus on writes | Consistency edge case: user deleted, API request hits replica before sync |

**Recommendation:** Start with **Mode A** (consistency) until master EXISTS becomes the bottleneck. Then evaluate whether the sub-ms window in Mode B is acceptable for your security requirements. If it is not, jump to **Tier 3** (bloom filter) which eliminates the trade-off entirely.

### Consistency Strategies at Scale

Replicas are always **delayed copies** of the primary — there is no synchronous replication in standard Redis. The following three strategies manage this inconsistency by classifying each Redis access pattern by criticality and applying the appropriate routing rule.

#### Strategy 1: Reads → Replicas (Non-Critical Paths Only)

Route reads to replicas only for data where **stale reads are harmless**. Use this for patterns that tolerate seconds of delay without security or correctness impact.

| Redis Pattern | Tolerates Stale? | Route Reads To | Rationale |
|---------------|-----------------|----------------|-----------|
| Rate limit counters | Yes | Replica | A slightly stale counter may allow 1 extra request — acceptable for burst protection |
| Threat detection counters | Yes | Replica | Brute force / off-hours / data exfiltration counters are approximate by design |
| Idempotency cache | Yes | Replica | Missing a cached response means re-execution — safe, just wasteful |
| Saga deadlines | No (but low volume) | Master | ZSET poll is a single Lua script — no benefit from replica routing |

**Configuration:**

```go
// Service-level Redis client separation
rdbNonCritical := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs:          []string{"master-a:6379", "master-b:6379", "master-c:6379"},
    RouteByLatency: true,
    ReadOnly:       true,
})

rdbCritical := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs: []string{"master-a:6379", "master-b:6379", "master-c:6379"},
    // Default: reads go to master
})
```

Inject the appropriate client per use case — `rdbNonCritical` for rate limiter and threat detector, `rdbCritical` for blocklist and MFA challenges.

#### Strategy 2: Bypass Replicas for Critical Paths

For the **blocklist EXISTS** and **MFA challenge** patterns, bypass replicas entirely. These must see the latest write or a security failure occurs.

| Redis Pattern | Critical? | Read Route | Consequence of Stale Read |
|---------------|-----------|------------|--------------------------|
| `blocklist:{jti}` | Yes — security | Master only | Revoked token accepted |
| `mfa_challenge:{token}` | Yes — auth | Master only | MFA step skipped or replayed |
| `totp:used:{userID}:{code}` | Yes — replay prevention | Master only | TOTP code reused |
| `saml:replay:{messageID}` | Yes — replay prevention | Master only | SAML assertion replayed |
| `auth_code:{code}` | Yes — OAuth2 | Master only | OAuth2 code reused |

**Implementation — force master route per-call:**

```go
// Always read blocklist from master via ReadFrom(master)
val, err := rdb.Exists(ctx, "blocklist:"+jti).Result()
// No RouteByLatency or ReadOnly on this client

// Or with cluster client, pin to master node:
client.ReadOnly = false  // Global setting
// Per-call override for non-critical reads:
client.Process(ctx, redis.NewStringCmd(ctx, "GET", "rate_limit:user:123"))
```

If you must use replicas for read scaling on the blocklist (Mode B above), pair with `WAIT` on the write side:

```go
// Write path — block until replica confirms
pipe.Set(ctx, "blocklist:"+jti, "revoked", ttl)
pipe.Wait(ctx, 1, 1000)  // at least 1 replica must ACK
pipe.Exec(ctx)
```

This shrinks the inconsistency window to network RTT (~0.5ms) rather than unbounded replica lag.

#### Strategy 3: Session / Request Affinity

Route **all Redis operations for the same user** to the same hash slot. If the user was just revoked (SET on master A), their next API request's EXISTS hits master A — not a different shard or a lagging replica.

**How it works:**

Redis Cluster uses `CRC16(key) % 16384` to determine the hash slot. The key constraint: all keys for the same user must hash to the same slot.

```
blocklist:{user_jti}    → CRC16 determines slot
mfa_challenge:{user_token} → different key, may hash to different slot
```

**Fix — hash tags:**

Redis Cluster supports **hash tags**: anything inside `{...}` is used as the hash input instead of the full key. By embedding the user ID or org ID in a hash tag, all related keys land on the same node.

```go
// With hash tags:
key := "blocklist:{" + userID + "}:" + jti
mfaKey := "mfa_challenge:{" + userID + "}:" + token
rateKey := "ratelimit:{" + orgID + "}:" + clientIP

// These three keys are guaranteed to hash to the same slot
// because only the content inside {} is used for CRC16
```

**Now the flow is consistent:**

```
Time 0ms:  Admin deletes user
Time 0ms:  IAM writes SET blocklist:{user123}:jti-abc → Master A (slot 4721)
Time 0ms:  Deleted user's next request
Time 0ms:  EXISTS blocklist:{user123}:jti-abc → Master A (same slot via hash tag)
Time 0ms:  → sees 1 → 401
```

**Without hash tags**, the blocklist and the request could route to different masters (different hash slots). With hash tags, they are guaranteed co-located.

**Where hash tags help:**

| Pattern | Hash Tag Key | Benefit |
|---------|-------------|---------|
| Blocklist + MFA challenge | `{userID}` | Revocation and auth ops hit same node |
| Rate limit + session | `{orgID}` | Rate counters and session data co-located |
| Threat counters per user | `{userID}` | Brute force, off-hours, travel counters same node |

**Where hash tags hurt:**

Hash tags concentrate load on specific slots if not carefully chosen. `{userID}` is high-cardinality (millions of unique values) — good distribution. `{orgID}` with a large org can hotspot a single slot. Test for distribution:

```bash
redis-cli --cluster check 127.0.0.1:6379 | grep slot
```

If any slot has significantly more keys than others, reconsider the hash tag prefix.

#### Putting It Together

The three strategies compose into a single per-pattern routing table:

| Redis Pattern | Strategy 1 (Replicas) | Strategy 2 (Bypass) | Strategy 3 (Affinity) |
|---------------|----------------------|---------------------|----------------------|
| `blocklist:{jti}` | No — critical | Force master | Hash tag `{userID}` |
| `mfa_challenge:{token}` | No — critical | Force master | Hash tag `{userID}` |
| `totp:used:{userID}:{code}` | No — replay critical | Force master | Hash tag `{userID}` (already has it) |
| Rate limit counters | Yes — tolerate stale | Replica preferred | Hash tag `{orgID}` |
| Threat detection counters | Yes — tolerate stale | Replica preferred | Hash tag `{userID}` |
| Idempotency cache | Yes — tolerate stale | Replica preferred | None needed |
| Saga deadlines | Low volume | Master (single script) | None needed |
| Migration lock | Low volume | Master (single SETNX) | None needed |

**Decision Flow:**

```
For each Redis access:
  ├── Is this a critical path? (blocklist, MFA, TOTP, SAML, OAuth)
  │     └── Yes → Route to MASTER (Strategy 2)
  │     └── No  → Is eventual consistency acceptable?
  │             └── Yes → Route to REPLICA (Strategy 1)
  │             └── No  → Route to MASTER (Strategy 2)
  │
  └── Does this user have related Redis keys on other shards?
        └── Yes → Add hash tag {userID} or {orgID} (Strategy 3)
```

### Tier 3: Local Cache + Bloom Filter — up to ~5M req/s

**When:** Even with clustering, 8M EXISTS/s per shard is too expensive. The key insight: >99.9% of tokens are NOT revoked, so most EXISTS calls return 0.

**Architecture:**

```
  Per-service-instance in-memory layer:

  ┌─────────────────────────────────────┐
  │         Service Instance            │
  │  ┌──────────────────────────────┐   │
  │  │  Bloom Filter (revoked JTIs) │   │  ← 1MB per 1M entries, 0.1% FP rate
  │  │  Refresh: every 1s via SCAN  │   │
  │  └──────────┬───────────────────┘   │
  │             │                       │
  │  ┌──────────▼───────────────────┐   │
  │  │ Not in bloom filter? → ALLOW │   │  99.9%+ fast path (nanoseconds)
  │  │ Possibly in bloom filter?    │   │
  │  │   → Redis EXISTS to confirm  │   │  <0.1% slow path (milliseconds)
  │  └──────────────────────────────┘   │
  └─────────────────────────────────────┘
```

**Changes required:**

1. **Bloom filter library** — `github.com/bits-and-blooms/bloom/v3` or similar. Filter size: ~1MB per 1M revoked JTIs at 0.1% false positive rate.
2. **Refresh mechanism** — Poll `SCAN 0 MATCH blocklist:*` every 1s in a background goroutine. Keep a shadow filter, swap atomically on refresh to avoid locking.
3. **Dual-write channel (optional)** — Subscribe to a Redis pub/sub channel (`blocklist:updates`) that IAM publishes to on revocation. Eliminates the 1s polling window.
4. **Eviction on filter full** — Bloom filters cannot delete. When capacity is reached, rebuild from scratch. Threshold: when estimated fill rate > 60%, trigger a fresh `SCAN` + rebuild.
5. **Race on first sync** — On cold start, the bloom filter is empty. Until the first `SCAN` completes, all requests fall through to Redis (slow path). Solution: initialize from a snapshot or accept ~1s warmup.

**Bloom filter sizing:**

| Max Concurrent Revoked JTIs | Filter Size | FP Rate | Memory |
|----------------------------|-------------|---------|--------|
| 100K | 175KB | 0.1% | ~0.2MB |
| 1M | 1.75MB | 0.1% | ~1.8MB |
| 10M | 17.5MB | 0.1% | ~18MB |

At 1h TTL, worst-case concurrent revocations = `(req/s × fraction revoked) × 3600`. If 0.01% of tokens are revoked per second at 1M req/s: 100 revoked/s × 3600s = 360K concurrent entries. ~0.6MB filter.

**Trade-off:** Revocation propagation delay of up to 1s (SCAN interval). Acceptable for all use cases except emergency lockdown (add pub/sub path for instant propagation).

### Tier 4: Sidecar Proxy — up to ~10M req/s

**When:** Application-level bloom filter maintenance becomes deployment overhead (N service instances × N services = maintenance burden).

**Architecture:**

```
  ┌─────────┐     ┌──────────────┐     ┌─────────────┐
  │ Service │────>│  Envoy Proxy │────>│  Redis       │
  │         │     │  + Wasm      │     │  Cluster     │
  │ (zero   │     │  filter      │     │             │
  │  Redis  │     │              │     │             │
  │  config)│     │  Bloom       │     │             │
  │         │     │  cache       │     │             │
  └─────────┘     └──────────────┘     └─────────────┘
```

The sidecar (Envoy + Wasm filter or Cilium eBPF) intercepts the blocklist check:
1. Extracts JTI from the Authorization header (or the service embeds it in a dedicated header like `X-OpenGuard-JTI`).
2. Checks local bloom filter.
3. On miss → allow (return 200 to upstream without hitting the app).
4. On hit → forward to app, which confirms via Redis.

**Changes required:**
- Sidecar deployment per host (daemonset or sidecar container).
- Service emits JTI in a header or the sidecar parses the JWT (avoiding double-parse with the app).
- Bloom filter refresh: sidecar maintains its own sync from Redis.

**Trade-off:** High operational complexity. Justified only at >5M req/s.

---

## Per-Pattern Optimization Strategies

### Blocklist EXISTS (highest frequency)

| Strategy | Redis Impact | Latency Impact | Complexity |
|----------|-------------|----------------|------------|
| Bloom filter + local cache | 99.9% reduction in EXISTS | Fast path: ~0ns. Slow path: same as before | Medium |
| Redis Cluster replicas | Distributes load across shards | +2-5ms per hop | Medium |
| Read replicas (non-cluster) | Offloads reads from master | +0.5-2ms replica lag | Low |
| EXISTS to Lua script batching | (Not applicable — single key) | — | — |

**Recommendation:** Bloom filter is the highest ROI for the blocklist pattern. Implement at Tier 3.

### MFA Challenges (medium frequency)

Challenges are short-lived (5min) and low volume relative to blocklist checks. No scaling action needed.

### Rate Limiting (per-request for some services)

The rate limiter uses a per-client sliding window with a Redis pipeline. At scale:

1. **Local token bucket + periodic sync.** Each instance maintains an in-memory token bucket, syncs counters to Redis every N seconds. Trade-off: allows burst violations within the sync window.
2. **Lua script on replica.** Evaluate rate limits on a replica with `ALLOW-LATENCY` config. Acceptable for rate limiting where sub-millisecond precision is not critical.

### Saga Deadlines (low frequency)

The 40s ZSET poll is a single Lua call. No scaling concern.

### Threat Detectors (per-event)

Each threat detector (brute force, off-hours, data exfiltration) maintains its own Redis counters. These are shardable by `org_id` or `user_id`. With Redis Cluster, the hash slot on these keys naturally distributes load.

---

## Connection Management

### Current Pattern

Each service opens one Redis connection pool via `redis.NewClient`. At scale:

| Service | Pool Size (default) | Estimated Concurrent Conns |
|---------|--------------------|---------------------------|
| IAM | 10 per CPU | ~80 |
| Policy | 10 per CPU | ~40 |
| Threat | 10 per CPU | ~40 |
| Audit | 10 per CPU | ~40 |
| Alerting | 10 per CPU | ~40 |
| Compliance | 10 per CPU | ~40 |
| DLP | 10 per CPU | ~40 |
| Connector Registry | 10 per CPU | ~40 |
| **Total** | | **~360 connections** |

### At 1M req/s

- 360 connections is well within a single Redis instance's capacity (default `maxclients` = 10,000).
- **Bottleneck is not connections, it's command execution.** Each connection still serializes commands on the single Redis thread.
- Bloom filter reduces command volume by 99.9%, making the connection count irrelevant.

### Pool Tuning

```go
redis.NewClient(&redis.Options{
    Addr:         addr,
    PoolSize:     20 * runtime.GOMAXPROCS(0),  // Scale with CPU
    MinIdleConns: 5,
    MaxRetries:   2,
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,
})
```

---

## Recommendation Summary

| Scale | Throughput | Strategy | Investment |
|-------|-----------|----------|------------|
| Current | <100K req/s | Single instance, tuned pools | None |
| Tier 2 | 100K–500K req/s | Redis Cluster (3 shards) + read replicas for EXISTS | Operational setup |
| Tier 3 | 500K–5M req/s | + Per-instance bloom filter for blocklist | ~2 weeks dev |
| Tier 4 | 5M–10M req/s | + Envoy sidecar with Wasm bloom filter | ~4 weeks dev + ops |

**Start with Tier 3.** Redis Cluster is a prerequisite for horizontal scaling, but bloom filtering eliminates 99.9% of the EXISTS traffic that drives the need for clustering in the first place. Deploy in order:

1. Add bloom filter with 1s SCAN refresh (Tier 3 core).
2. Add optional pub/sub channel for instant revocation propagation.
3. If more throughput needed, add Redis Cluster (Tier 2 infra) to handle the remaining 0.1% EXISTS fallback and all other Redis operations.

---

## Current Code Gap & Cluster Migration Plan

### The Gap

The entire codebase uses **standalone single-node Redis** only. There is zero cluster support today.

| Area | Current | Cluster Requires |
|------|---------|-----------------|
| Client init | `redis.NewClient(rOptions)` in 8 `main.go` files | `redis.NewClusterClient()` with seed list |
| Config | `REDIS_URL` (single endpoint) | `REDIS_CLUSTER_SEEDS` (comma-separated node list) |
| Type | `*redis.Client` everywhere (22+ struct fields + function params) | `*redis.ClusterClient` — different Go type, not interchangeable |
| Threat detectors | 6 detectors create their own `redis.NewClient()` internally, bypassing main | Must accept cluster client from main |
| Circuit breaker | Single `gobreaker.CircuitBreaker` per service | Per-node breakers or cluster-aware wrapper |
| Key routing | No hash tags — keys `blocklist:{jti}` hash to random slots | Must use `blocklist:{userID}:{jti}` for co-location and CROSSSLOT safety |
| Pipelines | Bulk SET per JTI in `DeleteUser`/`OffboardOrg` — all keys may cross slots | Hash tags required for multi-key pipeline |
| Lua scripts | `saga/watcher.go` uses `redis.NewScript` on single node | Must use `LOAD` + `EVALSHA` with same slot keys |
| Tests | `miniredis` for unit tests — does not support cluster | Need abstraction layer or integration test cluster |

### File Inventory — 35 Files to Touch

**Service main.go (8 files)** — Change init + config:

| File | Current Pattern |
|------|----------------|
| `services/iam/main.go:91-96` | `redis.ParseURL` → `redis.NewClient` |
| `services/policy/main.go:83-88` | Same |
| `services/threat/main.go:48-53` | Same |
| `services/audit/main.go:55-60` | Same |
| `services/alerting/main.go:61-66` | Same |
| `services/compliance/main.go:60-65` | Same |
| `services/dlp/main.go:49-54` | Same |
| `services/connector-registry/main.go:59-64` | Same |

**Threat detectors (6 files)** — Self-initialize Redis, must accept from main:

| File | Line | Pattern |
|------|------|---------|
| `services/threat/pkg/detector/brute_force.go` | 35 | `redis.NewClient(&redis.Options{Addr: redisAddr})` |
| `services/threat/pkg/detector/off_hours.go` | 31 | Same |
| `services/threat/pkg/detector/impossible_travel.go` | 46 | Same |
| `services/threat/pkg/detector/data_exfiltration.go` | 28 | Same |
| `services/threat/pkg/detector/account_takeover.go` | 27 | Same |
| `services/threat/pkg/detector/privilege_escalation.go` | 26 | Same |

**Shared middleware + database (4 files)** — Accept `*redis.Client`, must accept `*redis.ClusterClient`:

| File | Signature |
|------|-----------|
| `shared/middleware/jwt_auth.go:20` | `func AuthJWTWithBlocklist(keyring, rdb *redis.Client, breaker)` |
| `shared/middleware/ratelimit.go:31` | `func NewRateLimiter(rdb *redis.Client, r rate.Limit, b int)` |
| `shared/middleware/idempotency.go:18` | `func IdempotencyMiddleware(rdb *redis.Client)` |
| `shared/database/migrate.go:18` | `func RunWithLock(ctx, rdb *redis.Client, lockKey, task)` |

**IAM service (4 files)** — Core Redis-dependent types:

| File | Fields/Functions |
|------|-----------------|
| `services/iam/pkg/service/service_core.go:74` | `rdb *redis.Client` field |
| `services/iam/pkg/service/service_core.go:93` | `NewService(..., rdb *redis.Client, ...)` |
| `services/iam/pkg/router/router.go:24` | `NewRouter(..., rdb *redis.Client, ...)` |
| `services/iam/pkg/middleware/auth.go:22` | `func Auth(keyring, rdb *redis.Client)` |
| `services/iam/pkg/saga/watcher.go:19,26` | `rdb *redis.Client` field + `NewWatcher(rdb *redis.Client, ...)` |

**Other services (4 files)** — Router constructors accept `*redis.Client`:

| File | Signature |
|------|-----------|
| `services/policy/pkg/router/router.go:20` | `NewRouter(..., rdb *redis.Client, ...)` |
| `services/connector-registry/pkg/router/router.go:16` | `NewRouter(..., rdb *redis.Client, ...)` |
| `services/compliance/pkg/router/router.go:17` | `NewRouter(..., rdb *redis.Client, ...)` |
| `services/dlp/pkg/router/router.go:17` | `NewRouter(..., rdb *redis.Client, ...)` |
| `services/alerting/pkg/router/router.go:17` | `NewRouter(..., rdb *redis.Client, ...)` |
| `services/threat/pkg/router/router.go:16` | `NewRouter(..., rdb *redis.Client, ...)` |

**Other service cores (2 files):**

| File | Field/Signature |
|------|----------------|
| `services/policy/pkg/service/service.go:98,109` | `rdb *redis.Client` field + constructor |
| `services/connector-registry/pkg/service/service.go:26,30` | `rdb *redis.Client` field + constructor |

**Test files (~10 files)** — Use `miniredis` which does not support cluster:

| File | Pattern |
|------|---------|
| `services/iam/pkg/service/service_test.go` | `miniredis.Run()` → `redis.NewClient` |
| `services/threat/pkg/detector/*_test.go` | Same |
| `services/connector-registry/pkg/service/service_test.go` | Same |
| `services/alerting/pkg/router/router_test.go` | Same |

### Migration Plan — 4 Phases

#### Phase 1: Interface Abstraction (no behavioral change)

Introduce a shared interface that both `*redis.Client` and `*redis.ClusterClient` satisfy, so the type change does not cascade through every file at once.

```
shared/redis/client.go:

  // Cmdable is the subset of redis.Commands used by OpenGuard.
  type Cmdable interface {
      Get(ctx, key) *StringCmd
      Set(ctx, key, value, ttl) *StatusCmd
      SetArgs(ctx, key, value, SetArgs) *StatusCmd
      SetNX(ctx, key, value, ttl) *BoolCmd
      Exists(ctx, keys...) *IntCmd
      Del(ctx, keys...) *IntCmd
      Expire(ctx, key, ttl) *BoolCmd
      Pipeline() Pipeliner
      ZAdd(ctx, key, members...) *IntCmd
      ZRem(ctx, key, members...) *IntCmd
      ZRangeByScore(ctx, key, opt) *StringSliceCmd
      GetDel(ctx, key) *StringCmd
      // cluster-specific:
      ReadOnly()  // no-op for single client
  }
```

**Files changed:** New file `shared/redis/client.go`. Zero other changes — the interface is introduced but not yet used.

#### Phase 2: Wire the Interface (type migration)

Change every `*redis.Client` to `shared.Cmdable` across all services. The `*redis.Client` already satisfies the interface, so this compiles immediately with zero behavior change.

**Pattern — before:**
```go
type Service struct {
    rdb *redis.Client
}
func NewService(rdb *redis.Client) *Service { ... }
```

**Pattern — after:**
```go
type Service struct {
    rdb Cmdable
}
func NewService(rdb Cmdable) *Service { ... }
```

**Files changed:** All 22+ struct fields and function signatures listed above (~25 files). Each change is a one-line type swap.

**Validation:** `go build ./...` passes. All existing tests pass. All behavior identical.

#### Phase 3: Cluster Client Factory

Add a cluster-aware factory function in `shared/redis/client.go`:

```go
// NewClient creates the appropriate Redis client based on config.
// Single node when REDIS_URL is set.
// Cluster when REDIS_CLUSTER_SEEDS is set.
func NewClient(ctx context.Context, cfg Config) (Cmdable, error) {
    if cfg.ClusterSeeds != "" {
        seeds := strings.Split(cfg.ClusterSeeds, ",")
        return redis.NewClusterClient(&redis.ClusterOptions{
            Addrs:         seeds,
            RouteByLatency: true,
            ReadOnly:       true,
        }), nil
    }
    opts, err := redis.ParseURL(cfg.URL)
    if err != nil {
        return nil, err
    }
    return redis.NewClient(opts), nil
}
```

Swap the 8 `main.go` files from direct `redis.NewClient` to `shared.NewClient`. Add `REDIS_CLUSTER_SEEDS` env var alongside `REDIS_URL`.

**Files changed:** `shared/redis/client.go` (update), 8 `main.go` files.

#### Phase 4: Hash Tags + Pipeline Safety

Add hash tags to blocklist keys for cluster slot affinity:

| Before | After |
|--------|-------|
| `blocklist:{jti}` | `blocklist:{userID}:{jti}` |
| `mfa_challenge:{token}` | `mfa_challenge:{userID}:{token}` |
| `totp:used:{userID}:{code}` | Already has `{userID}` — no change |
| `auth_code:{code}` | `auth_code:{clientID}:{code}` |

Update `DeleteUser()` and `OffboardOrg()` pipelines — they iterate JTIs by user, so all keys naturally share the same `{userID}` hash tag after the change. No cross-slot risk.

Update the saga watcher Lua script — ZSET keys `saga:deadlines` are per-org, add `saga:{orgID}:deadlines` hash tag.

**Files changed:** `services/iam/pkg/service/auth.go` (Logout), `services/iam/pkg/service/users.go` (DeleteUser, OffboardOrg), `services/iam/pkg/middleware/auth.go` (blocklist check), `shared/middleware/jwt_auth.go` (blocklist check), `services/iam/pkg/saga/watcher.go` (Lua script key).

### Migration Order

```
Phase 1 ──→ Phase 2 ──→ Phase 3 ──→ Phase 4
 (interface)  (type swap)  (cluster     (hash tags)
                            init)
                                  │
                                  ▼
                              Deploy cluster, flip
                              REDIS_CLUSTER_SEEDS
```

Each phase is independently deployable and reversible. No phase should be merged until the previous one is running in production.

### Rollback

If cluster migration causes issues:

1. Unset `REDIS_CLUSTER_SEEDS` (or set to empty).
2. Restart services — they fall back to single-node `redis.NewClient`.
3. No code revert needed — the interface abstraction handles both modes transparently.

The hash tag change (Phase 4) is forward-compatible: single-node Redis ignores `{...}` in key names, so keys like `blocklist:{userID}:{jti}` work identically on standalone Redis.
