# §11 — Phase 3: Policy Engine

**Goal:** p99 < 30ms for `POST /v1/policy/evaluate` (uncached); p99 < 5ms (Redis cached). Two-tier cache: SDK LRU (client-side) + Redis (server-side). Fail closed.

---

## 11.1 Database Schema

**001_create_policies.up.sql**
```sql
CREATE TABLE policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    version      INT NOT NULL DEFAULT 1,
    logic        JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- + policy_assignments table, + standard outbox table
```

**003_create_policy_eval_log.up.sql**
```sql
CREATE TABLE policy_eval_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    user_id      UUID NOT NULL,
    action       TEXT NOT NULL,
    resource     TEXT NOT NULL,
    result       BOOLEAN NOT NULL,
    policy_ids   UUID[] NOT NULL DEFAULT '{}',
    latency_ms   INT NOT NULL,
    cache_hit    TEXT NOT NULL DEFAULT 'none',  -- 'none' | 'redis' | 'sdk'
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- RLS policy + indexes
```

---

## 11.2 Redis Caching for Evaluate

**Cache key:**
```
"policy:eval:{org_id}:{sha256(sorted_json(action, resource, user_id, user_groups))}"
```

**Cache value:**
```json
{ "permitted": true, "matched_policies": ["uuid1"], "reason": "RBAC match", "evaluated_at": "..." }
```

**TTL:** `POLICY_CACHE_TTL_SECONDS` (default: 30).

**Cache invalidation on policy change:** Policy service subscribes to `TopicPolicyChanges` and uses a per-org key index:

```go
// On every cache SET, also add the key to a Redis Set (pipelined):
//   SADD "policy:eval:org:{org_id}:keys" "<full_cache_key>"
//   EXPIRE "policy:eval:org:{org_id}:keys" <TTL>
//
// On policy.changes event:
//   1. SMEMBERS "policy:eval:org:{org_id}:keys"
//   2. UNLINK <each key>   (non-blocking DEL)
//   3. UNLINK "policy:eval:org:{org_id}:keys"
//
// SCALABILITY BOUND: SMEMBERS on M > 10,000 entries is a blocking O(N) operation.
// Alert threshold: SCARD > 5,000 (warning), > 10,000 (critical, use UNLINK path).
//
// Thundering Herd Mitigation:
// Use singleflight.Group per (org_id, cache_key) to prevent stampede on cache invalidation.
// Additionally, use stale-while-revalidate: return stale value (grace +5s) while
// a background goroutine refreshes the cache.
```

---

## 11.3 Policy Service Architecture

**Evaluation flow:**
1. SDK sends `POST /v1/policy/evaluate` to control plane.
2. Control plane's `cb-policy` circuit breaker wraps the call to the policy service.
3. Policy service checks Redis cache first.
4. Cache miss: query PostgreSQL (RLS-scoped), evaluate RBAC rules, write to Redis, log via outbox.
5. Control plane returns result to SDK.
6. SDK stores result in local LRU cache with TTL = `SDK_POLICY_CACHE_TTL_SECONDS`.

**Circuit breaker open:**
- Control plane returns `503 POLICY_SERVICE_UNAVAILABLE`.
- SDK uses local cache if available.
- After SDK cache TTL expires with no successful re-fetch: SDK returns `DenyDecision`.

---

## 11.4 Policy Webhook to Connectors

When a policy changes, connected apps with scope `policy:read` receive a signed outbound webhook within 5 seconds.

---

## 11.5 Policy Management API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/policies` | List policies |
| `POST` | `/v1/policies` | Create policy |
| `GET` | `/v1/policies/:id` | Get policy |
| `PUT` | `/v1/policies/:id` | Update policy (publishes `policy.changes` via outbox) |
| `DELETE` | `/v1/policies/:id` | Delete policy |
| `POST` | `/v1/policy/evaluate` | Real-time evaluation (SDK entry point) |
| `GET` | `/v1/policy/eval-logs` | Evaluation history |

---

## 11.6 Phase 3 Acceptance Criteria

- [ ] `POST /v1/policy/evaluate` p99 < 30ms (uncached) under 500 concurrent requests.
- [ ] `POST /v1/policy/evaluate` p99 < 5ms (Redis cached) under 500 concurrent requests.
- [ ] SDK local cache hit: second identical call produces 0 outbound HTTP requests.
- [ ] Policy change → Redis cache invalidated → next evaluate returns fresh result within 1s.
- [ ] Policy change → webhook delivered to connector with `policy:write` scope within 5s.
- [ ] Policy service circuit breaker open → `503` → SDK falls back to local cache → after TTL: SDK denies.
- [ ] `version` increments on policy update; returns correct `ETag` header.
- [ ] `policy_eval_log` records `cache_hit: "redis"` for cache hits, `"none"` for misses.
