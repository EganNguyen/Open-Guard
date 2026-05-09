# Policy Engine — Workflow

## Level 1: High-Level Architecture

```
  ┌────────────────────────────────────────────────────────────────────────────────────┐
  │                            EXTERNAL CLIENTS                                        │
  │                                                                                      │
  │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐                 │
  │  │  Angular Admin   │  │  SDK/Apps        │  │  Control Plane   │                 │
  │  │  (Dashboard)     │  │  (policy eval)   │  │  (proxy /v1/*)   │                 │
  │  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘                 │
  │           │                     │                     │                             │
  │           │     HTTPS (mTLS optional)                 │                             │
  │           ▼                     ▼                     ▼                             │
  │  ┌───────────────────────────────────────────────────────────────────────────┐     │
  │  │                     POLICY SERVICE (port 8083)                              │     │
  │  │                                                                              │     │
  │  │  ┌──────────────────────────────────────────────────────────────────────┐ │     │
  │  │  │                    MIDDLEWARE STACK                                     │ │     │
  │  │  │  RequestID → RealIP → Logger → Recoverer                               │ │     │
  │  │  │  → RequestSize(512KB) → Timeout(5s) → SecurityHeaders                  │ │     │
  │  │  │  → RateLimiter(1000/s, burst 2000) → AuthJWT + Blocklist               │ │     │
  │  │  │  → Idempotency (POST/PUT) → Handler                                    │ │     │
  │  │  └──────────────────────────────────────────────────────────────────────┘ │     │
  │  │                                                                              │     │
  │  │  ┌──────────────────────────────────────────────────────────────────────┐ │     │
  │  │  │                      HANDLER LAYER (handler.go)                       │ │     │
  │  │  │                                                                        │ │     │
  │  │  │  POST   /v1/policies          → CreatePolicy    (idempotent)          │ │     │
  │  │  │  GET    /v1/policies          → ListPolicies                           │ │     │
  │  │  │  GET    /v1/policies/{id}     → GetPolicy                             │ │     │
  │  │  │  PUT    /v1/policies/{id}     → UpdatePolicy   (idempotent)           │ │     │
  │  │  │  DELETE /v1/policies/{id}     → DeletePolicy                           │ │     │
  │  │  │  GET    /v1/assignments       → ListAssignments                        │ │     │
  │  │  │  POST   /v1/assignments       → CreateAssignment (idempotent)         │ │     │
  │  │  │  DELETE /v1/assignments/{id}  → DeleteAssignment                       │ │     │
  │  │  │  POST   /v1/policy/evaluate   → Evaluate        (core engine)          │ │     │
  │  │  │  GET    /v1/policy/eval-logs  → ListEvalLogs                           │ │     │
  │  │  └──────────────────────────────────────────────────────────────────────┘ │     │
  │  │                                                                              │     │
  │  │  ┌──────────────────────────────────────────────────────────────────────┐ │     │
  │  │  │                     SERVICE LAYER (service.go)                        │ │     │
  │  │  │                                                                        │ │     │
  │  │  │  ┌────────────────────────────────────────────────────────────────┐  │ │     │
  │  │  │  │  EVALUATE (Policy Decision)                                     │  │ │     │
  │  │  │  │                                                                  │  │ │     │
  │  │  │  │  1. Build cache key: SHA256(org:subject:action:resource)        │  │ │     │
  │  │  │  │  2. Try Redis cache (stale-while-revalidate):                    │  │ │     │
  │  │  │  │     ├── Fresh (≤55s) → return cached, cache_hit="redis"        │  │ │     │
  │  │  │  │     ├── Stale (55-60s) → return cached + background refresh   │  │ │     │
  │  │  │  │     └── Miss → fall through                                     │  │ │     │
  │  │  │  │  3. Singleflight + DB query (circuit-breaker wrapped)           │  │ │     │
  │  │  │  │  4. Evaluate rules in order (CEL / RBAC / allow_all / deny_all)│  │ │     │
  │  │  │  │  5. Cache result in Redis (TTL 60s)                            │  │ │     │
  │  │  │  │  6. Async eval log (buffered channel, capacity 1000)            │  │ │     │
  │  │  │  └────────────────────────────────────────────────────────────────┘  │ │     │
  │  │  │                                                                        │ │     │
  │  │  │  ┌────────────────────────────────────────────────────────────────┐  │ │     │
  │  │  │  │  CRUD (Policy Management)                                      │  │ │     │
  │  │  │  │                                                                  │  │ │     │
  │  │  │  │  1. Begin DB transaction                                        │  │ │     │
  │  │  │  │  2. Validate CEL logic (compile check)                         │  │ │     │
  │  │  │  │  3. repo.CreatePolicyTx / UpdatePolicyTx / DeletePolicyTx       │  │ │     │
  │  │  │  │  4. outbox.WriteTx (policy.changes topic)                      │  │ │     │
  │  │  │  │  5. Commit transaction                                          │  │ │     │
  │  │  │  │  6. Invalidate org Redis cache (background goroutine)           │  │ │     │
  │  │  │  └────────────────────────────────────────────────────────────────┘  │ │     │
  │  │  └──────────────────────────────────────────────────────────────────────┘ │     │
  │  │                                                                              │     │
  │  │  ┌──────────────────────────────────────────────────────────────────────┐ │     │
  │  │  │                 REPOSITORY LAYER (repository.go)                      │ │     │
  │  │  │                                                                        │ │     │
  │  │  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │ │     │
  │  │  │  │ PolicyStore  │  │ EvalLogStore │  │AssignmentSt  │              │ │     │
  │  │  │  │ - CRUD       │  │ - Write      │  │ ore          │              │ │     │
  │  │  │  │ - GetMatchin │  │ - List       │  │ - CRUD       │              │ │     │
  │  │  │  │   gPolicies  │  │              │  │              │              │ │     │
  │  │  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘              │ │     │
  │  │  │         │                 │                 │                         │ │     │
  │  │  │         │   All queries use RLS: set_config('app.org_id', ...)       │ │     │
  │  │  │         │   Transactional ops use rls.TxSetSessionVar                │ │     │
  │  │  └──────────────────────────────────────────────────────────────────────┘ │     │
  │  └──────────────────────────────────────────────────────────────────────────┘     │
  │                                                                                      │
  │  ┌──────────────────────┐  ┌──────────────────────┐  ┌──────────────────────┐     │
  │  │      PostgreSQL       │  │       Redis           │  │       Kafka          │     │
  │  │                       │  │                        │  │                       │     │
  │  │  policies             │  │  policy:eval:<hash>   │  │  policy.changes      │     │
  │  │    - UUID PK          │  │    (TTL 60s)           │  │  → Threat Service    │     │
  │  │    - logic JSONB      │  │  policy:index:<org>   │  │  → Audit Service     │     │
  │  │    - version INT      │  │    (TTL 24h)           │  │  → SDK Cache Inval   │     │
  │  │                       │  │  idem:<org>:<sha>     │  │                       │     │
  │  │  policy_assignments   │  │    (TTL 24h)           │  │  outbox_records      │     │
  │  │    - subject_id UUID  │  │  blocklist:<jti>      │  │    (status: pending  │     │
  │  │    - subject_type     │  │    (varies)            │  │     → published      │     │
  │  │                       │  │                        │  │     → dead DLQ)      │     │
  │  │  policy_eval_log      │  │                        │  │                       │     │
  │  │  outbox_records       │  │                        │  │                       │     │
  │  │                       │  │                        │  │                       │     │
  │  │  RLS on all tables    │  │                        │  │                       │     │
  │  └──────────────────────┘  └──────────────────────┘  └──────────────────────┘     │
```

---

## Level 2A: Policy Evaluation Flow

```
  SDK / App                    Policy Service                         Redis           PostgreSQL
    │                              │                                    │                 │
    │  POST /v1/policy/evaluate    │                                    │                 │
    │  { subject_id, action,       │                                    │                 │
    │    resource, user_groups }   │                                    │                 │
    │────────────────────────────>│                                    │                 │
    │                              │                                    │                 │
    │                              │  Build cache key:                   │                 │
    │                              │  SHA256(org:subj:act:res:groups)   │                 │
    │                              │                                    │                 │
    │                              │  GET policy:eval:<hash>            │                 │
    │                              │───────────────────────────────────>│                 │
    │                              │                                    │                 │
    │         ┌─ Cache HIT        │                                    │                 │
    │         │  Check ExpiresAt:  │                                    │                 │
    │         │  ├─ FRESH (≤55s)  │                                    │                 │
    │  <──────│──│──── cached decision, cache_hit="redis" ───────────│                 │
    │         │  │                                    │                 │                 │
    │         │  ├─ STALE (55-60s)                                    │                 │
    │  <──────│──│──── cached decision, cache_hit="stale" ───────────│                 │
    │         │  │    spawn background refresh (goroutine, semaphore)  │                 │
    │         │  │    → re-query DB, re-cache                          │                 │
    │         │  │                                    │                 │                 │
    │         └─ Cache MISS (or expired)                               │                 │
    │                              │                                    │                 │
    │                              │  Singleflight.Do(key):             │                 │
    │                              │  (deduplicates concurrent misses)  │                 │
    │                              │                                    │                 │
    │                              │  GetMatchingPolicies (CB wrapped,  │                 │
    │                              │  50ms timeout)                    │                 │
    │                              │──────────────────────────────────────────────────>│
    │                              │                                    │                 │
    │                              │  SELECT DISTINCT p.*              │                 │
    │                              │  FROM policies p                   │                 │
    │                              │  LEFT JOIN policy_assignments pa   │                 │
    │                              │  WHERE p.org_id = $1               │                 │
    │                              │  AND (pa.id IS NULL                │                 │
    │                              │    OR pa.subject_id = $2           │                 │
    │                              │    OR pa.subject_id = ANY($3))     │                 │
    │                              │  ORDER BY p.created_at ASC         │                 │
    │                              │                                    │                 │
    │  <────── policies[] ────────│────────────────────────────────────│                 │
    │                              │                                    │                 │
    │                              │  evaluate(req, policies):          │                 │
    │                              │    default effect = DENY           │                 │
    │                              │    for each policy (in order):     │                 │
    │                              │      switch logic.type:            │                 │
    │                              │        "deny_all" → DENY (explicit)│                 │
    │                              │        "allow_all" → ALLOW        │                 │
    │                              │        "rbac" → match via glob    │                 │
    │                              │        "cel" → compile + eval     │                 │
    │                              │          subject / action /        │                 │
    │                              │          resource / user_groups    │                 │
    │                              │    DENY wins over ALLOW            │                 │
    │                              │                                    │                 │
    │                              │  Cache result (SET + EXPIRE +     │                 │
    │                              │   SADD org index)                  │                 │
    │                              │───────────────────────────────────>│                 │
    │                              │                                    │                 │
    │                              │  Async eval log (buffered channel) │                 │
    │                              │                                    │                 │
    │  <── 200 { effect: "allow"  │                                    │                 │
    │         | "deny",           │                                    │                 │
    │         matched_by: [...],  │                                    │                 │
    │         cache_hit: "redis"  │                                    │                 │
    │       } ───────────────────│                                    │                 │
    │                              │                                    │                 │
    │         ┌─ Circuit breaker OPEN or DB error:                      │                 │
    │  <──────│── 200 { effect: "deny", matched_by: [] } ─────────────│                 │
```

### Rule Evaluation Details

```
  evaluate(req, policies):
    │
    ├── sort policies by created_at ASC
    │
    ├── effect = "deny" (default)
    ├── matchedIDs = []
    │
    ├── for each policy in policies:
    │     │
    │     ├── logic.type == "deny_all":
    │     │     effect = "deny"
    │     │     matchedIDs = [policy.ID]   (deny overrides all)
    │     │     break                       (immediate deny)
    │     │
    │     ├── logic.type == "allow_all":
    │     │     if effect != "deny":
    │     │       effect = "allow"
    │     │       matchedIDs.append(policy.ID)
    │     │
    │     ├── logic.type == "rbac":
    │     │     match := matchRBAC(req.subject, req.action, req.resource,
    │     │                       logic.subjects, logic.actions, logic.resources)
    │     │     if match:
    │     │       if effect != "deny":
    │     │         effect = "allow"
    │     │         matchedIDs.append(policy.ID)
    │     │
    │     └── logic.type == "cel":
    │           ast, err := celEnv.Compile(logic.expression)
    │           if err: skip (logged WARN)
    │           prog, err := celEnv.Program(ast)
    │           if err: skip
    │           result, err := prog.Eval(map{
    │             "subject": req.subject_id,
    │             "action":  req.action,
    │             "resource": req.resource,
    │             "user_groups": req.user_groups,
    │           })
    │           if result == true:
    │             if effect != "deny":
    │               effect = "allow"
    │               matchedIDs.append(policy.ID)
    │
    └── return { effect, matchedIDs, maxVersion }

    RBAC glob matching:
      "*"             → matches anything
      "user:*"        → matches "user:admin", "user:123", etc.
      "read:*"        → matches "read:docs", "read:files", etc.
      "read:docs"     → exact match only
      "prefix*"       → prefix match
```

### CEL Expression Examples (from tests)

```
  subject == 'user:admin'
  subject.startsWith('user:')
  'admin' in user_groups
  subject == 'user1' && resource.startsWith('prod:') && action == 'delete'
  resource in ['prod:db', 'prod:cache']
  action != 'delete' || (subject == 'superadmin')
```

---

## Level 2B: Policy CRUD Flow (Transactional Outbox)

```
  Admin / Dashboard            Policy Service                    PostgreSQL           Kafka
    │                              │                                │                   │
    │  POST /v1/policies           │                                │                   │
    │  { name, description,       │                                │                   │
    │    logic: { type: "cel",    │                                │                   │
    │             expression } }   │                                │                   │
    │────────────────────────────>│                                │                   │
    │                              │                                │                   │
    │                              │  Validate CEL compile          │                   │
    │                              │                                │                   │
    │                              │  BEGIN TX                      │                   │
    │                              │───────────────────────────────>│                   │
    │                              │  rls.TxSetSessionVar(org_id)   │                   │
    │                              │───────────────────────────────>│                   │
    │                              │                                │                   │
    │                              │  INSERT INTO policies          │                   │
    │                              │  (version=1)                   │                   │
    │                              │───────────────────────────────>│                   │
    │                              │                                │                   │
    │                              │  outboxWriter.WriteTx(         │                   │
    │                              │    topic="policy.changes",     │                   │
    │                              │    key=policyID,               │                   │
    │                              │    payload={event_type,          │                   │
    │                              │             org_id, policy_id}) │                   │
    │                              │───────────────────────────────>│                   │
    │                              │                                │                   │
    │                              │  COMMIT                        │                   │
    │                              │───────────────────────────────>│                   │
    │                              │                                │                   │
    │                              │  InvalidateOrgCache(orgID)     │                   │
    │                              │  (background goroutine)        │                   │
    │                              │                                │                   │
    │  <── 201 { policy } ───────│                                │                   │
    │                              │                                │                   │
    │                              │  (async) Outbox relay polls    │                   │
    │                              │  outbox_records WHERE           │                   │
    │                              │  status='pending' FOR UPDATE   │                   │
    │                              │  SKIP LOCKED                   │                   │
    │                              │───────────────────────────────────────────────>│
    │                              │  Mark published                                │
    │                              │<───────────────────────────────────────────────│
    │                              │                                │                   │
    │                              │                                │    policy.changes │
    │                              │                                │    → Threat Svc   │
    │                              │                                │    → Audit Svc    │
    │                              │                                │    → SDK Cache    │
```

---

## Level 3: State Transitions

### Policy Record State

```
                        ┌──────────┐
                        │  ACTIVE  │  (version=1, created_at=now)
                        └────┬─────┘
                             │
                    ┌────────┴────────┐
                    │                 │
               ┌────▼────┐     ┌────▼────┐
               │ UPDATED │     │ DELETED │
               │ (v+n)   │     │ (soft)  │
               └────┬─────┘     └─────────┘
                    │
               ┌────▼────┐
               │  ACTIVE │  (each update increments version)
               │ (v+1)   │
               └─────────┘

  Note: DELETE is a hard-delete (no archived state).
```

### Outbox Record State

```
                        ┌──────────┐
                        │  PENDING │  (written in DB tx)
                        └────┬─────┘
                             │
                    ┌────────┴────────┐
                    │                 │
               ┌────▼────┐     ┌────▼────┐
               │PUBLISHED│     │ FAILED  │
               │ (Kafka  │     │ (retry) │
               │  acked) │     └────┬─────┘
               └─────────┘          │
                              ┌─────┴─────┐
                              │           │
                              ▼           ▼
                        ┌─────────┐ ┌─────────┐
                        │  RETRY  │ │  DEAD   │
                        │ (×5)    │ │ (DLQ)   │
                        └─────────┘ └─────────┘
```

### Redis Cache State (Stale-While-Revalidate)

```
  Time 0:    decision written with ExpiresAt = now + 55s
  Time 0-55:  FRESH     → return cached, cache_hit = "redis"
  Time 55-60: STALE     → return cached + background refresh
  Time 60+:   EVICTED   → next request misses, hits DB

  Background refresh:
    - Bounded by semaphore (capacity 100)
    - Uses context.Background() + RLS context
    - 5s timeout with retry
    - On success: writes new decision, resets TTL
```

### Cache Invalidation

```
  On CreateAssignment / DeleteAssignment:
    → InvalidateOrgCache(orgID)
      → SMEMBERS policy:index:<orgID>
      → DEL each cache key
      → DEL policy:index:<orgID>

  On policy.created / policy.updated / policy.deleted:
    → Same invalidation (via outbox event consumer, not yet wired)
```

---

## Consumer Group / Event Mapping

| Event Type | Topic | Produced By | Consumed By |
|------------|-------|-------------|-------------|
| `policy.created` | `policy.changes` | Policy Service (outbox) | Threat (escalation), Audit |
| `policy.updated` | `policy.changes` | Policy Service (outbox) | Threat (escalation), Audit |
| `policy.deleted` | `policy.changes` | Policy Service (outbox) | Threat (escalation), Audit |

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Angular Dashboard** | Policy CRUD UI, assignment management, evaluation testing |
| **SDK/Apps** | Policy evaluation requests with subject, action, resource context |
| **Control Plane** | Proxies `/v1/policy/*`, `/v1/policies/*`, `/v1/assignments/*` to Policy Service |
| **Policy Service** | Policy CRUD, CEL/RBAC evaluation engine, caching, outbox event production |
| **PostgreSQL** | RLS-enforced policies, assignments, eval logs, outbox records |
| **Redis** | Evaluation cache (stale-while-revalidate), idempotency keys, JWT blocklist |
| **Kafka** | Async policy change events for downstream consumers |
| **Threat Service** | Consumes `policy.changes` for privilege escalation detection |
| **Audit Service** | Consumes `policy.changes` for immutable audit trail |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **DB unreachable** | Policy | Circuit breaker (50ms timeout, 10 failures → 30s open) → fail-closed DENY |
| **Redis unreachable** | Policy | Cache miss (graceful degradation); singleflight still works |
| **JWT blocklist Redis down** | Policy | Blocklist check fail-open (request proceeds with warning) |
| **Migration fails** | Policy | Process exits (os.Exit(1)) — fail-early |
| **CEL compilation error at runtime** | Policy | Policy skipped with WARN log; other policies still evaluated |
| **CEL env not initialized** | Policy | All CEL policies skipped; only RBAC/deny_all/allow_all work |
| **Kafka broker down** | Policy | Outbox relay retries (max 5); events stay PENDING then go to DLQ |
| **Background refresh semaphore full** | Policy | Refresh skipped; cache expires naturally |
| **Eval log channel full (1000)** | Policy | Log entry dropped; no audit trail for that eval |
| **Missing org_id in context** | Policy | Handler returns 500 (middleware misconfiguration) |
| **mTLS certs missing** | Policy | Falls back to plain HTTP (dev mode) |
| **Idempotency collision across tenants** | Policy | Namespaced by org_id — safe from cross-tenant replay |
| **Policy assignment changed during eval** | Policy | May serve stale cache for up to 60s until refresh |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_policy_evaluations_total` | Counter | `effect`, `cache_hit` | Policy Service |
| `openguard_policy_eval_latency_seconds` | Histogram | `effect` | Policy Service |
| `openguard_policy_cache_operations_total` | Counter | `operation` (hit/miss/stale) | Policy Service |
| `openguard_policy_db_operations_total` | Counter | `operation` | Repository |
| `openguard_policy_cel_evaluations_total` | Counter | `status` (success/error) | Policy Service |

### Key Traces (Jaeger)

- `policy.evaluate` — full evaluation from HTTP to response
- `policy.evaluate.db` — DB query phase (GetMatchingPolicies)
- `policy.evaluate.cache` — Redis cache lookup + write
- `policy.evaluate.rules` — rule engine execution (per-policy spans)
- `policy.crud.create` / `update` / `delete` — mutation with outbox

### Audit Log Events

| Event | When | Payload |
|-------|------|---------|
| `policy.created` | Policy persisted | org_id, policy_id, logic_type |
| `policy.updated` | Policy modified | org_id, policy_id, version_bump |
| `policy.deleted` | Policy removed | org_id, policy_id |
| `policy.evaluated` | Policy decision made | org_id, effect, cache_hit, matched_ids, latency |
| `assignment.created` | Policy linked to subject | org_id, policy_id, subject_id, subject_type |
| `assignment.deleted` | Policy unlinked | org_id, policy_id, assignment_id |

---

## Request Flow Summary

```
  Evaluate:                    CRUD (Create/Update/Delete):
  ─────────                    ───────────────────────────
  SDK → POST /evaluate         Admin → POST/PUT/DELETE /policies
    │                              │
    ├── Middleware chain            ├── Middleware chain
    ├── Cache lookup (Redis)        ├── Auth + Idempotency
    ├── Singleflight.Do             ├── Validate CEL
    ├── DB query (CB wrapped)      ├── BEGIN TX
    ├── Rule engine (CEL/RBAC)      │   ├── DB mutation
    ├── Cache write                 │   └── Outbox write
    └── Async eval log             ├── COMMIT
                                   ├── Invalidate cache
                                   └── Async → Kafka relay
```
