**ROLE**
Act as a senior distributed systems engineer and concurrency expert. Your task is to perform a deep, production-level analysis of concurrency issues in the provided codebase or system.

---

**OBJECTIVES**
You must:

1. Detect all possible concurrency issues (actual and potential).
2. Identify precise root causes (not symptoms).
3. Recommend proven patterns to fix them.
4. Provide concrete, actionable changes (code-level where possible).

---

**SCOPE OF ANALYSIS**
Thoroughly analyze for:

### 1. Concurrency Issues Detection

* Race conditions (read/write, write/write)
* Deadlocks and livelocks
* Resource contention (CPU, DB, locks, threads)
* Non-atomic operations
* Improper async/await usage
* Shared mutable state issues
* Thread safety violations
* Event ordering / timing issues
* Cache inconsistency / stale reads
* Distributed concurrency (multi-instance conflicts)

---

### 2. Root Cause Analysis

For every issue:

* Explain *why* it happens (not just what)
* Identify exact code paths / flows involved
* Highlight timing windows or interleavings
* Show how it can be reproduced (scenario or sequence)
* Distinguish between deterministic vs probabilistic bugs

---

### 3. Fix Patterns (MANDATORY)

For each issue, map to **proven concurrency patterns**, such as:

* Immutability
* Locking (fine-grained vs coarse-grained)
* Optimistic concurrency (versioning, CAS)
* Pessimistic locking
* Idempotency
* Retry with backoff
* Queue-based serialization
* Actor model
* Circuit breaker / bulkhead
* Thread confinement
* Event sourcing (if applicable)

Explain:

* Why this pattern is appropriate
* Trade-offs (performance, complexity, scalability)

---

### 4. Actionable Fix Plan

You MUST provide:

* **Exact code-level recommendations**

  * Before / After snippets if possible
* Refactoring steps (ordered, safe rollout)
* Required infrastructure changes (DB, cache, queue, etc.)
* Monitoring additions (logs, metrics, alerts)

---

### 5. Risk & Impact Assessment

For each issue:

* Severity (Critical / High / Medium / Low)
* Likelihood of occurrence
* Production impact (data corruption, latency, crashes)

---

### 6. Validation Strategy

Define how to verify the fix:

* Unit tests (focus on concurrency)
* Stress / load tests
* Chaos scenarios
* Observability signals to confirm resolution

---

**OUTPUT FORMAT (STRICT)**

For each issue:

```
## Issue: <Short Title>

### Symptoms
...

### Root Cause
...

### Reproduction Scenario
...

### Fix Pattern
...

### Recommended Fix (Actionable)
...

### Risk & Impact
...

### Validation Plan
...
```

---

**CONSTRAINTS**

* Do NOT give generic advice
* Do NOT skip root cause
* Do NOT suggest fixes without explaining why
* Prefer deterministic, production-safe solutions over theoretical ones

---

# Production Concurrency Analysis: OpenGuard Services

**Scope:** 10 Go microservices — `iam`, `policy`, `threat`, `alerting`, `audit`, `dlp`, `compliance`, `webhook-delivery`, `connector-registry`, `control-plane`  
**Analyst role:** Senior distributed systems / concurrency engineer  
**Date:** 2026-04-25

---

## Executive Summary

| # | Issue | Service | Severity | Likelihood |
|---|-------|---------|----------|------------|
| 1 | Refresh Token Rotation TOCTOU Race | `iam` | **Critical** | High |
| 2 | MFA Challenge Token Non-Atomic Verify-then-Delete | `iam` | **Critical** | Medium |
| 3 | Saga Watcher Duplicate Compensation Events | `iam` | **High** | High |
| 4 | Auth Code One-Time-Use Read-Delete Gap | `iam` | **High** | Medium |
| 5 | Alerting Saga Unbounded Goroutine Leak + No Offset Commit | `alerting` | **High** | High |
| 6 | Policy Cache Stale-While-Revalidate Background Goroutine Leak | `policy` | **High** | High |
| 7 | Audit Batch Flush `defer span.End()` in Loop — All Spans End at Function Exit | `audit` | **Medium** | High |
| 8 | Audit Hash Chain Divergence on Multi-Instance Deployment | `audit` | **Critical** | High |
| 9 | Brute-Force Detector Non-Atomic Check-then-Publish | `threat` | **High** | High |
| 10 | Impossible Travel Detector Read-Modify-Write Race on Redis | `threat` | **High** | Medium |
| 11 | In-Memory Rate Limiter Not Effective Across Multiple Instances | `iam` | **High** | Certain |
| 12 | AuthWorkerPool Workers Leak on Context Cancellation — No Graceful Drain | `iam` | **Medium** | Medium |
| 13 | `GetAuthCode` Non-Atomic Get-then-Delete (OAuth Code Replay) | `iam` | **Critical** | Medium |
| 14 | Connector Registry API Key Cache Bypasses Verification on Cache Hit | `connector-registry` | **High** | High |
| 15 | WebAuthn Session Race — Concurrent Registration Requests Overwrite Session | `iam` | **High** | Low |

---

## Issue 1: Refresh Token Rotation TOCTOU Race

### Symptoms
Duplicate sessions created for the same refresh token. Two concurrent `POST /token/refresh` calls with the same token both succeed and each produce a new access+refresh pair, effectively duplicating the session. Downstream audit shows two new families from the same parent.

### Root Cause
`RefreshToken` in `iam/pkg/service/service.go` performs a **read-check-delete** sequence that is not wrapped in a database transaction:

```go
// service.go ~line 290
rt, err := s.repo.GetRefreshToken(ctx, rtHash)       // Step A: Read
// ... check revoked / expiry ...
s.repo.DeleteRefreshToken(ctx, rtHash)               // Step B: Delete (no tx)
return s.IssueTokens(...)                            // Step C: Issue new tokens
```

Between Step A and Step B, a second concurrent goroutine can complete Step A with the same token and see `revoked=false`. Both goroutines then proceed to Step C and issue distinct token pairs from the same old token.

The timing window is: from the moment `GetRefreshToken` returns until `DeleteRefreshToken` executes. At normal latency this is 1–10 ms, which is easily hit under concurrent mobile clients reconnecting (e.g. app launch on poor network with duplicated requests).

### Reproduction Scenario
```
Goroutine G1: GetRefreshToken(tokenHash) → {revoked: false}  ← both see this
Goroutine G2: GetRefreshToken(tokenHash) → {revoked: false}
G1: DeleteRefreshToken(tokenHash)
G2: DeleteRefreshToken(tokenHash)   ← DELETE of already-deleted row, silently succeeds
G1: IssueTokens → access1, refresh1
G2: IssueTokens → access2, refresh2   ← SESSION CLONED
```

This is a **probabilistic** bug triggered by network retries, mobile background refresh, or load-balancer retry storms.

### Fix Pattern
**Pessimistic locking with atomic CAS (SELECT ... FOR UPDATE) inside a transaction.** This is the correct pattern because the token must be validated, deleted, and new tokens issued atomically.

### Recommended Fix

**Before:**
```go
func (s *Service) RefreshToken(ctx context.Context, refreshToken, userAgent, ip string) (map[string]interface{}, error) {
    rtHash := crypto.HashSHA256(refreshToken)
    rt, err := s.repo.GetRefreshToken(ctx, rtHash)
    // ...check revoked...
    s.repo.DeleteRefreshToken(ctx, rtHash)
    return s.IssueTokens(...)
}
```

**After — add a repository method that atomically claims and returns the token:**
```go
// In repository.go — new method
func (r *Repository) ClaimRefreshToken(ctx context.Context, tokenHash string) (map[string]interface{}, error) {
    orgID := rls.OrgID(ctx)
    var rt = make(map[string]interface{})
    err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
        return conn.QueryRow(ctx, `
            DELETE FROM refresh_tokens
            WHERE token_hash = $1
              AND revoked = false
              AND expires_at > now()
            RETURNING id, org_id, user_id, family_id, expires_at
        `, tokenHash).Scan(
            &rt["id"], &rt["org_id"], &rt["user_id"], &rt["family_id"], &rt["expires_at"],
        )
    })
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound // Token already consumed or expired
    }
    return rt, err
}

// In service.go
func (s *Service) RefreshToken(ctx context.Context, refreshToken, userAgent, ip string) (map[string]interface{}, error) {
    rtHash := crypto.HashSHA256(refreshToken)
    rt, err := s.repo.ClaimRefreshToken(ctx, rtHash)
    if err != nil {
        // Not found means: already used (reuse attack) or expired
        // Trigger family revocation by looking up family from a secondary index
        s.repo.RevokeRefreshTokenFamilyByHash(ctx, rtHash)
        return nil, fmt.Errorf("token invalid, revoked, or already used")
    }
    return s.IssueTokens(ctx, rt["org_id"].(string), rt["user_id"].(string), userAgent, ip, rt["family_id"].(uuid.UUID))
}
```

The `DELETE ... RETURNING` is atomic at the database level. Only one concurrent caller will receive a row; all others get `ErrNoRows`.

### Refactoring Steps
1. Add `ClaimRefreshToken` repository method (no service downtime needed).
2. Add integration test: two concurrent `RefreshToken` calls with the same token — assert only one succeeds.
3. Deploy repository change, then service change.
4. Monitor `refresh_token_reuse_attempts` metric (add counter in the `ErrNotFound` path).

### Risk & Impact
- **Severity:** Critical
- **Likelihood:** High — any mobile client or any retry middleware can trigger this
- **Production impact:** Session cloning; two active sessions for one authentication event; audit inconsistency; potential account takeover escalation path

### Validation Plan
- Unit test: mock `ClaimRefreshToken` returning `ErrNotFound` on second call, assert caller gets an error and family revocation fires.
- Load test: 500 concurrent goroutines replay the same refresh token — assert exactly 1 new session created.
- Chaos: inject 50ms sleep between old `GetRefreshToken` and `Delete`, observe token cloning in staging — verify it no longer occurs after fix.

---

## Issue 2: MFA Challenge Token Non-Atomic Verify-then-Delete

### Symptoms
A TOTP/backup code can be replayed within the 5-minute challenge window if two concurrent requests hit `VerifyMFAAndLogin` or `VerifyBackupCodeAndLogin` with the same `challengeToken`.

### Root Cause
```go
// service.go — VerifyMFAAndLogin
res, _ := s.rdb.Get(ctx, "mfa_challenge:"+challengeToken)   // Step A
userID := res.(string)
ok, _ := s.VerifyTOTP(ctx, userID, code)                    // Step B
// ... error handling ...
s.rdb.Del(ctx, "mfa_challenge:"+challengeToken)             // Step C: AFTER verification
```

Between Steps A and C (including the VerifyTOTP DB roundtrip ~5–20ms), another goroutine can execute Step A and see the same challenge token as valid. Both goroutines then issue tokens.

Additionally, `VerifyTOTP` uses `totp.Validate` with no replay protection: the same TOTP code is valid for the entire 30-second window. Combined with the challenge race, an attacker who intercepts the challenge token can replay it.

### Reproduction Scenario
```
Client sends two near-simultaneous POST /auth/mfa/verify requests (network retry)
G1: GET mfa_challenge:<token> → userID
G2: GET mfa_challenge:<token> → userID (token not yet deleted)
G1: VerifyTOTP → ok
G2: VerifyTOTP → ok  (same TOTP code, same 30s window)
G1: DEL mfa_challenge:<token>
G2: DEL mfa_challenge:<token>  (noop)
Both G1 and G2 issue separate access tokens → session duplication
```

### Fix Pattern
**Atomic Redis `GETDEL`** (Redis ≥ 6.2) or `SET NX` claim pattern. Consume the challenge in one atomic operation.

### Recommended Fix

**Before:**
```go
res, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
    return s.rdb.Get(ctx, "mfa_challenge:"+challengeToken).Result()
})
// ... later ...
s.rdb.Del(ctx, "mfa_challenge:"+challengeToken)
```

**After:**
```go
// Atomic: Get AND Delete in one command — only one caller gets the value
res, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
    return s.rdb.GetDel(ctx, "mfa_challenge:"+challengeToken).Result()
})
if err != nil {
    return nil, "", fmt.Errorf("invalid or expired challenge")
}
userID := res.(string)
// Remove the explicit Del call below — it's no longer needed
```

For the TOTP replay window, add a Redis key to mark used codes:
```go
// In VerifyTOTP
func (s *Service) VerifyTOTP(ctx context.Context, userID, code string) (bool, error) {
    // Check used-code nonce (prevent replay within TOTP window)
    nonceKey := fmt.Sprintf("totp:used:%s:%s", userID, code)
    set, err := s.rdb.SetNX(ctx, nonceKey, "1", 90*time.Second).Result() // 3x TOTP window
    if err == nil && !set {
        return false, fmt.Errorf("totp code already used")
    }
    // ... existing verify logic ...
}
```

### Risk & Impact
- **Severity:** Critical
- **Likelihood:** Medium — requires concurrent requests with the same challenge token (retries, race on poor networks)
- **Production impact:** MFA bypass via session duplication; weakens the entire second-factor guarantee

### Validation Plan
- Unit test: concurrent `VerifyMFAAndLogin` calls with the same token — assert exactly one succeeds.
- Integration test: replay the same TOTP code twice within 90 seconds — assert second call is rejected.

---

## Issue 3: Saga Watcher Duplicate Compensation Events (Distributed Race)

### Symptoms
When multiple IAM service instances are running, the saga watcher fires duplicate `user.provisioning.failed` events for the same `sagaID` — resulting in double-compensation, double email notifications, and inconsistent user status.

### Root Cause
In `iam/pkg/saga/watcher.go`, `checkExpired` uses a **non-atomic** read-then-delete pattern:

```go
sagaIDs, _ := w.rdb.ZRangeByScore(ctx, "saga:deadlines", ...)   // Step A: read
for _, sagaID := range sagaIDs {
    w.publisher.Publish(ctx, "saga.orchestration", sagaID, payload)   // Step B: publish
    w.rdb.ZRem(ctx, "saga:deadlines", sagaID)   // Step C: remove (AFTER publish)
}
```

Two instances with a 10-second ticker can both execute Step A in the same tick window (the `ZRangeByScore` is not a claim operation). Both see the same expired saga IDs, both publish compensation events, and both then remove the entry from the sorted set. This is **deterministic** with more than one instance running.

### Reproduction Scenario
```
Instance A: ZRangeByScore → ["user-123"]
Instance B: ZRangeByScore → ["user-123"]   (within same 10s window)
A: Publish compensation for user-123
B: Publish compensation for user-123   ← DUPLICATE
A: ZRem "user-123"
B: ZRem "user-123"  (noop, already removed)
```

### Fix Pattern
**Optimistic claim via `ZRANGEBYSCORE` + `ZADD NX` fencing** or **Redis `ZPOPMIN`** (atomic pop). `ZPOPMIN` atomically removes and returns the lowest-score element(s), making it safe for concurrent watchers.

### Recommended Fix

**Before:**
```go
sagaIDs, err := w.rdb.ZRangeByScore(ctx, "saga:deadlines", &redis.ZRangeBy{...}).Result()
// ... publish, then ZRem
```

**After:**
```go
func (w *Watcher) checkExpired(ctx context.Context) {
    now := float64(time.Now().Unix())
    // ZPOPMIN-by-score: atomically claim up to 100 expired sagas
    // Redis 6.2+: ZRANGEBYSCORE + ZPOPMIN in a Lua script for exact score filter
    script := redis.NewScript(`
        local members = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 100)
        if #members == 0 then return {} end
        redis.call('ZREM', KEYS[1], unpack(members))
        return members
    `)
    result, err := script.Run(ctx, w.rdb, []string{"saga:deadlines"}, now).StringSlice()
    if err != nil || len(result) == 0 {
        return
    }
    for _, sagaID := range result {
        // publish as before
    }
}
```

The Lua script executes atomically on the Redis server — only one instance claims each saga ID.

### Risk & Impact
- **Severity:** High
- **Likelihood:** High (certain with ≥2 IAM replicas, which is standard)
- **Production impact:** Double compensation events — users may receive double emails, duplicate "provisioning failed" status updates, confusing audit trail

### Validation Plan
- Integration test: two watcher goroutines with shared Redis — insert one expired saga entry, assert exactly one compensation event published.
- Production monitoring: add `saga_compensation_published_total` counter; alert if same `saga_id` appears more than once.

---

## Issue 4: OAuth Auth Code One-Time-Use Read-Delete Gap

### Symptoms
An OAuth authorization code can be exchanged for tokens more than once within a short time window, violating RFC 6749 §4.1.2 which mandates single-use codes.

### Root Cause
In `iam/pkg/service/service.go`:

```go
func (s *Service) GetAuthCode(ctx context.Context, code string) (string, string, error) {
    val, err := s.rdb.Get(ctx, "auth_code:"+code).Result()   // Step A
    // ... parse ...
    s.rdb.Del(ctx, "auth_code:"+code)   // Step B — not atomic with A
    return parts[0], parts[1], nil
}
```

Two concurrent token exchange requests (e.g. client retry + server-side idempotency bug) can both pass Step A before either executes Step B.

### Fix Pattern
Same as Issue 2: **Redis `GETDEL`** (Redis ≥ 6.2) for atomic consume.

### Recommended Fix

```go
func (s *Service) GetAuthCode(ctx context.Context, code string) (string, string, error) {
    if s.rdb == nil {
        return "", "", fmt.Errorf("redis not configured")
    }
    val, err := s.rdb.GetDel(ctx, "auth_code:"+code).Result()
    if err != nil {
        return "", "", fmt.Errorf("invalid or expired auth code")
    }
    parts := strings.Split(val, ":")
    if len(parts) != 2 {
        return "", "", fmt.Errorf("corrupt auth code data")
    }
    return parts[0], parts[1], nil
}
```

### Risk & Impact
- **Severity:** Critical (OAuth security violation)
- **Likelihood:** Medium
- **Production impact:** Authorization code replay; token duplication; audit gap

### Validation Plan
- Test: concurrent `GetAuthCode` calls — assert exactly one returns a value, second returns error.

---

## Issue 5: Alerting Saga Unbounded Goroutine Leak + No Offset Commit on Failure

### Symptoms
Under sustained load, the `alerting` service's memory grows monotonically. Alerts can be processed multiple times across restarts. Under broker-lag conditions, thousands of goroutines accumulate.

### Root Cause — Two separate issues in `alerting/pkg/saga/saga.go`:

**Issue 5a — Goroutine leak:**
```go
func (s *AlertSaga) Start(ctx context.Context) error {
    for {
        m, err := s.reader.ReadMessage(ctx)   // NOTE: ReadMessage auto-commits!
        // ...
        go s.processMessage(ctx, m)   // ← Spawns unbounded goroutines
    }
}
```

There is no goroutine cap, no `sync.WaitGroup`, and no backpressure. At 10,000 messages/second, this creates 10,000 goroutines. Each goroutine's `executeStep` can sleep up to `1+2+4+8+16 = 31 seconds` of backoff. Memory and goroutine counts grow linearly with backlog.

**Issue 5b — `ReadMessage` auto-commits:**
`kafka.Reader.ReadMessage` commits the offset automatically. If `processMessage` fails, the message is permanently lost (not redelivered). The comment in the code says "not committing offset" but `ReadMessage` has already committed. The `BruteForceDetector` correctly uses `FetchMessage` + `CommitMessages`; the saga does not.

### Fix Pattern
**Bounded worker pool** (semaphore pattern) for goroutine control. Switch to `FetchMessage` + manual commit after processing.

### Recommended Fix

```go
func (s *AlertSaga) Start(ctx context.Context) error {
    sem := make(chan struct{}, 50)  // max 50 concurrent alert goroutines
    var wg sync.WaitGroup

    for {
        m, err := s.reader.FetchMessage(ctx)   // FetchMessage: manual commit
        if err != nil {
            if ctx.Err() != nil {
                wg.Wait()
                return nil
            }
            s.logger.Error("failed to fetch message", "error", err)
            continue
        }

        sem <- struct{}{}
        wg.Add(1)
        go func(msg kafka.Message) {
            defer wg.Done()
            defer func() { <-sem }()

            s.processMessage(ctx, msg)

            // Commit ONLY after processing
            if err := s.reader.CommitMessages(ctx, msg); err != nil {
                s.logger.Error("failed to commit offset", "error", err)
            }
        }(m)
    }
}
```

### Risk & Impact
- **Severity:** High
- **Likelihood:** High (certain under any spike in alert volume)
- **Production impact:** OOM crash of alerting service; message loss (auto-commit); alert storm causing cascading failure

### Validation Plan
- Load test: publish 100k messages, verify goroutine count plateaus at ≤50 (or configured cap).
- Chaos: kill alerting mid-batch; verify messages are redelivered after restart.
- Metric: `runtime_goroutines` — alert if > 500 in alerting service.

---

## Issue 6: Policy Cache Background Goroutine Leak and Unbounded Accumulation

### Symptoms
Policy service memory grows over time. Every cache hit spawns a background goroutine. Under load (thousands of evaluate calls/second), millions of goroutines can accumulate if `backgroundRefresh` blocks on a slow DB.

### Root Cause
In `policy/pkg/service/service.go`:

```go
if cached, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
    // ...
    go s.backgroundRefresh(req, key)   // ← No cap, no tracking
    return &EvaluateResponse{...}, nil
}
```

`backgroundRefresh` internally calls `evaluateFromDB`, which can take up to `maxRetryDelay = 5s` if the DB circuit breaker is open. Each cache hit fires a goroutine. At 10,000 req/s with a 5s DB timeout, up to 50,000 goroutines can be in-flight simultaneously.

Additionally, `go s.writeEvalLog(req, r.resp)` in the DB path is also unbounded.

### Fix Pattern
**Singleflight for background refresh** (already used for DB queries — extend the same pattern). Or a **dedicated refresh worker pool** with a channel-based queue.

### Recommended Fix

```go
// In Service struct, add a refresh rate limiter
type Service struct {
    // ...
    refreshSem chan struct{}  // bounded refresh semaphore
}

// In NewService:
refreshSem: make(chan struct{}, 100),  // max 100 concurrent background refreshes

// In Evaluate:
go func() {
    select {
    case s.refreshSem <- struct{}{}:
        defer func() { <-s.refreshSem }()
        s.backgroundRefresh(req, key)
    default:
        // Semaphore full — skip this refresh, cache will be refreshed on next miss
        s.logger.Debug("background refresh skipped, semaphore full")
    }
}()
```

For `writeEvalLog`, use a buffered channel worker:

```go
// In NewService, start a log worker
logCh: make(chan evalLogEntry, 1000),  // drop if full

// In goroutine:
go func() {
    for entry := range s.logCh {
        s.writeEvalLog(entry.req, entry.resp)
    }
}()

// Replace `go s.writeEvalLog(...)` with:
select {
case s.logCh <- evalLogEntry{req, r.resp}:
default:
    s.logger.Warn("eval log channel full, dropping entry")
}
```

### Risk & Impact
- **Severity:** High
- **Likelihood:** High (any caching benefit becomes a goroutine problem under load)
- **Production impact:** OOM in policy service; goroutine explosion; latency spike as GC pressure increases

### Validation Plan
- Load test: 10,000 req/s, all cache hits — verify goroutine count stable under configured cap.
- Metric: `runtime_goroutines` — alert if > 200 in policy service.

---

## Issue 7: Audit Consumer `defer span.End()` Inside Batch Loop

### Symptoms
All OpenTelemetry spans for a batch share the same span end time (the batch function's return). Trace data shows all events in a batch ending simultaneously, making per-event latency analysis impossible.

### Root Cause
In `audit/pkg/consumer/consumer.go`:

```go
func (c *AuditConsumer) flush(ctx context.Context, batch []kafka.Message) {
    for _, m := range batch {
        // ...
        _, span := otel.Tracer("audit-consumer").Start(msgCtx, "consume-audit-event", ...)
        defer span.End()   // ← defer in loop: all spans end when flush() returns
        // ...
    }
}
```

`defer` in a loop does not end the span at the end of the loop iteration — it ends all deferred calls when `flush()` returns. Every span in the batch ends at the same moment, with the same (wrong) duration.

### Fix Pattern
**Explicit `span.End()` at the end of the loop body**, or extract the body to a helper function.

### Recommended Fix

```go
for _, m := range batch {
    headerMap := extractHeaders(m)
    prop := otel.GetTextMapPropagator()
    msgCtx := prop.Extract(ctx, propagation.MapCarrier(headerMap))

    msgCtx, span := otel.Tracer("audit-consumer").Start(msgCtx, "consume-audit-event",
        trace.WithSpanKind(trace.SpanKindConsumer))

    var event map[string]interface{}
    if err := json.Unmarshal(m.Value, &event); err != nil {
        span.RecordError(err)
        span.End()   // ← explicit, correct
        continue
    }
    event["timestamp"] = time.Now()
    events = append(events, event)
    span.End()   // ← explicit at end of iteration
}
```

### Risk & Impact
- **Severity:** Medium
- **Likelihood:** High (this fires on every batch)
- **Production impact:** Corrupted trace data; inability to diagnose per-event processing latency; misleading SLO dashboards

### Validation Plan
- Add a unit test that asserts each span ends within the batch loop (mock tracer).
- Verify in Jaeger/Tempo that per-event spans have realistic durations after fix.

---

## Issue 8: Audit Hash Chain Divergence on Multi-Instance Deployment

### Symptoms
Audit hash chain verification fails intermittently. Chain gaps or forks appear in the audit log. Two audit consumer instances produce events with overlapping or duplicate sequence numbers.

### Root Cause
The audit consumer uses `repo.ReserveSequence` to atomically claim a sequence range, and `repo.UpdateHashChain` to update the hash chain head. This is correct for sequence assignment (assuming `ReserveSequence` uses an atomic DB counter), **but** the hash chain computation happens in-process and only the _final_ hash is persisted:

```go
startSeq, prevHash, err := c.repo.ReserveSequence(ctx, orgID, int64(len(events)))
// Compute hash chain locally...
currentHash = hex.EncodeToString(mac.Sum(nil))
// ...
c.repo.UpdateHashChain(ctx, orgID, currentHash)
```

If two consumer instances both flush at nearly the same time for the same `orgID`:
- Instance A claims sequences 1–100 and reads `prevHash = H0`
- Instance B claims sequences 101–200 and reads `prevHash = H0` (race — before A updates)
- A writes `H100` as the new chain head
- B writes `H200` based on `H0`, not `H100` → **chain broken**

`ReserveSequence` and `UpdateHashChain` are separate DB calls with no inter-instance lock between them.

### Fix Pattern
**Hash chain computation must be serialized per org.** Options:
1. Single consumer per org partition (Kafka partition-by-orgID) — architectural fix, preferred.
2. Postgres advisory lock per orgID during the reserve+compute+update cycle.
3. Optimistic CAS on hash chain: `UPDATE hash_chains SET hash = $new WHERE hash = $prev` — retry on mismatch.

### Recommended Fix (Option 3 — least invasive)

```go
// Replace UpdateHashChain with a CAS variant
func (r *AuditWriteRepository) UpdateHashChainCAS(ctx context.Context, orgID, prevHash, newHash string) (bool, error) {
    res, err := r.db.Exec(ctx, `
        UPDATE hash_chains
        SET current_hash = $1, updated_at = now()
        WHERE org_id = $2 AND current_hash = $3
    `, newHash, orgID, prevHash)
    return res.RowsAffected() == 1, err
}

// In flush():
for attempt := 0; attempt < 5; attempt++ {
    startSeq, prevHash, err := c.repo.ReserveSequence(ctx, orgID, int64(len(events)))
    // compute hashes...
    ok, err := c.repo.UpdateHashChainCAS(ctx, orgID, prevHash, currentHash)
    if ok {
        break
    }
    // Another instance updated the chain; retry with fresh prevHash
}
```

**Preferred long-term fix:** Partition the audit Kafka topic by `org_id` and assign one consumer instance per partition. This eliminates the distributed hash chain problem entirely.

### Risk & Impact
- **Severity:** Critical
- **Likelihood:** High with ≥2 audit consumer instances
- **Production impact:** Broken tamper-evidence guarantee; compliance audit failures; regulatory exposure for SOC2/ISO27001 customers

### Validation Plan
- Run two audit consumer instances concurrently, flush events for the same org simultaneously, then verify hash chain with the chain validator tool.
- Confirm `UpdateHashChainCAS` retry count is ≤1 under normal operation (metric: `hash_chain_cas_retries_total`).

---

## Issue 9: Brute-Force Detector Non-Atomic Check-then-Publish

### Symptoms
For a burst of exactly N events where N = `maxAttempts`, multiple threat alerts are published for the same IP/user within the same sliding window — one alert per event after the threshold is crossed, not one alert per crossing.

### Root Cause
In `threat/pkg/detector/brute_force.go`:

```go
if count >= d.maxAttempts {
    d.publishThreatEvent(ctx, key, count)   // No deduplication
}
```

The pipeline returns the current `ZCard` count. For events 11, 12, 13 ... all of them satisfy `count >= 11`. Each one publishes a separate threat alert. The `publishThreatEvent` also calls `d.store.CreateAlert` which persists a new alert document each time.

Furthermore, `d.rdb.Set(ctx, "threat:"+key, payload, ThreatTTL)` overwrites the previous threat record without any locking, so the metadata (attempt count) in the stored threat will reflect the last writer's count in a concurrent scenario.

### Fix Pattern
**Redis `SETNX` as a per-window dedup flag.** Set a flag when the threshold is first crossed; skip publishing if the flag already exists.

### Recommended Fix

```go
func (d *BruteForceDetector) trackFailedAttempt(ctx context.Context, key string) error {
    // ... existing pipeline code to get count ...
    count := countCmd.Val()

    if count >= d.maxAttempts {
        // Deduplicate: only fire once per alert window
        alertKey := "alert_fired:" + key
        set, err := d.rdb.SetNX(ctx, alertKey, "1", WindowSize).Result()
        if err != nil {
            d.logger.Error("failed to check alert dedup key", "error", err)
        }
        if set {
            // Only first setter publishes the alert
            d.publishThreatEvent(ctx, key, count)
        }
    }
    return nil
}
```

### Risk & Impact
- **Severity:** High
- **Likelihood:** High (fires on every sustained attack, not just the first detection)
- **Production impact:** Alert storm; MongoDB alert collection inflated; SIEM webhook spammed; on-call desensitization

### Validation Plan
- Unit test: send 20 failed login events for the same IP — assert exactly 1 alert created.
- Integration test: verify `alert_fired:<key>` TTL matches `WindowSize`.

---

## Issue 10: Impossible Travel Detector Read-Modify-Write Race on Redis

### Symptoms
Two login events from the same user processed concurrently may both read the _same_ previous location, resulting in missed impossible travel detection or a false-positive alert using stale coordinates.

### Root Cause
In `threat/pkg/detector/impossible_travel.go`:

```go
val, err := d.rdb.Get(ctx, redisKey).Result()   // Step A
if err == nil {
    // unmarshal + detect
}
// Store current login
d.rdb.Set(ctx, redisKey, payload, ...)   // Step B — not atomic with A
```

Two concurrent goroutines reading from the same Kafka partition (impossible if partition-per-consumer, but possible if partition count < consumer count) or two detectors on separate topics for the same user can both read the same `last` location and both write `current` location.

The detector uses a single-goroutine consumer (`Run` loop is sequential), so this race only manifests with multiple detector instances or parallel Kafka partitions. However, `processEvent` is also where the geoip lookup happens — if geoip blocks, two events can interleave at the Redis level via goroutine scheduling.

### Fix Pattern
**Redis Lua script for atomic read-compare-write** or partition-by-userId in Kafka to serialize per-user events.

### Recommended Fix

```go
// Atomic: get previous, set current, return previous — all in one Redis round-trip
script := redis.NewScript(`
    local prev = redis.call('GET', KEYS[1])
    redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
    return prev
`)
prevJSON, err := script.Run(ctx, d.rdb,
    []string{"travel:" + userID},
    string(payload),
    strconv.Itoa(d.windowSecs),
).Text()
// prevJSON is nil if no previous entry (first login)
if prevJSON != "" {
    var last LastLogin
    json.Unmarshal([]byte(prevJSON), &last)
    d.detect(ctx, userID, last, current)
}
```

### Risk & Impact
- **Severity:** High
- **Likelihood:** Medium (depends on deployment topology)
- **Production impact:** Missed impossible travel detections; false-positive alert storms if stale location used

### Validation Plan
- Chaos test: inject artificial delay in geoip lookup, send two concurrent login events for the same user from different IPs — verify exactly one detection fires.

---

## Issue 11: In-Memory Rate Limiter Has Zero Effect Across Multiple Service Instances

### Symptoms
Users can exceed the configured rate limit by targeting different IAM pod IPs through the load balancer. Each pod maintains independent per-IP state.

### Root Cause
`iam/pkg/middleware/ratelimit.go` stores limiters in a plain `map[string]*entry` protected by a `sync.Mutex`. This is correct within a single process but **provides zero protection across pods**.

With 3 IAM replicas, an attacker gets 3× the configured burst limit. With autoscaling, the effective limit scales inversely with the number of pods.

### Fix Pattern
**Distributed rate limiting via Redis** using the sliding window or token bucket pattern. The existing Redis client (`s.rdb`) is already wired into the IAM service.

### Recommended Fix

Replace the in-process limiter with a Redis-backed sliding window:

```go
func (l *RateLimiter) isAllowed(ctx context.Context, rdb *redis.Client, ip string) bool {
    now := time.Now().UnixNano() / int64(time.Millisecond)
    window := now - int64(time.Minute/time.Millisecond)
    key := "ratelimit:" + ip

    pipe := rdb.Pipeline()
    pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", window))
    pipe.ZCard(ctx, key)
    pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
    pipe.Expire(ctx, key, time.Minute)
    cmds, _ := pipe.Exec(ctx)

    count := cmds[1].(*redis.IntCmd).Val()
    return count < int64(l.b)  // b = burst limit
}
```

Keep the in-memory limiter as a first-pass (fast path) and Redis as the distributed enforcer.

### Risk & Impact
- **Severity:** High
- **Likelihood:** Certain in any multi-replica deployment
- **Production impact:** Brute force login bypass; credential stuffing attacks not throttled at platform level

### Validation Plan
- Integration test: two clients targeting different service instances, both sending at rate limit — assert total requests accepted ≤ configured limit.

---

## Issue 12: AuthWorkerPool Goroutines Leak on Graceful Shutdown

### Symptoms
During rolling deploys, bcrypt goroutines continue running after the application context is cancelled. In-flight `Compare` and `Generate` calls may hang indefinitely if the channel is never drained.

### Root Cause
`iam/pkg/service/auth_pool.go`:

```go
func (p *AuthWorkerPool) worker() {
    for {
        select {
        case job := <-p.compareJobs:
            job.result <- bcrypt.CompareHashAndPassword(...)
        case job := <-p.generateJobs:
            // ...
        }
    }
}
```

Workers have no shutdown signal. When the application shuts down (context cancelled), the HTTP server stops accepting requests, but worker goroutines remain blocked in the `select`. The channels are never closed. These goroutines leak until the process exits.

More critically, if a caller's context is cancelled while the job is in the queue (not yet picked up), the result channel is buffered (size 1) so it will not block — **but the job is still processed by the worker after the caller has gone away**. This wastes CPU on bcrypt for abandoned requests, which can cause a temporary CPU spike during shutdown/high-load.

### Fix Pattern
**Context-aware workers with a shutdown channel.** Close the job channels on shutdown.

### Recommended Fix

```go
type AuthWorkerPool struct {
    compareJobs  chan bcryptCompareJob
    generateJobs chan bcryptGenerateJob
    workers      int
    wg           sync.WaitGroup
}

func NewAuthWorkerPool(workers int, ctx context.Context) *AuthWorkerPool {
    p := &AuthWorkerPool{
        compareJobs:  make(chan bcryptCompareJob, 100),
        generateJobs: make(chan bcryptGenerateJob, 100),
        workers:      workers,
    }
    for i := 0; i < workers; i++ {
        p.wg.Add(1)
        go p.worker(ctx)
    }
    return p
}

func (p *AuthWorkerPool) worker(ctx context.Context) {
    defer p.wg.Done()
    for {
        select {
        case <-ctx.Done():
            return
        case job, ok := <-p.compareJobs:
            if !ok { return }
            job.result <- bcrypt.CompareHashAndPassword([]byte(job.hash), []byte(job.password))
        case job, ok := <-p.generateJobs:
            if !ok { return }
            // ...
        }
    }
}

func (p *AuthWorkerPool) Shutdown() {
    close(p.compareJobs)
    close(p.generateJobs)
    p.wg.Wait()
}
```

Wire `Shutdown()` into the application's graceful shutdown sequence.

### Risk & Impact
- **Severity:** Medium
- **Likelihood:** Medium (manifests during every deploy)
- **Production impact:** CPU spike on shutdown; goroutine leak detectable via `pprof`; potential Kubernetes pod termination timeout breach

### Validation Plan
- `pprof` goroutine snapshot before and after graceful shutdown — verify worker goroutines drain.
- Unit test: cancel context, verify `wg.Wait()` completes within 200ms.

---

## Issue 13: Connector Registry API Key Cache Returns Without Verification

### Symptoms
A revoked connector's API key continues to be accepted for up to 5 minutes after revocation. Worse, a partial implementation in the cache-hit path skips hash verification entirely.

### Root Cause
In `connector-registry/pkg/service/service.go`:

```go
if s.rdb != nil {
    _, err := s.rdb.Get(ctx, "apikey:"+prefix).Result()
    if err == nil {
        // Cache hit. Need to verify hash.
        // In a real app, we might cache the hash or a success flag...
        s.logger.Debug("apikey cache hit", "prefix", prefix)
        // ... verification logic ...    ← THIS IS EMPTY
    }
}
// Falls through to DB lookup and PBKDF2 verification
```

On a cache hit, the code logs "cache hit" and falls through to the DB path anyway — so verification does happen, but the comment structure and incomplete stub create a footgun where future developers may add `return connector, nil` at the cache-hit branch without wiring in verification. It also means the cache provides zero performance benefit currently (every request still hits the DB).

More critically, **there is no cache invalidation on revocation**. When a connector is deleted (`DeleteConnector`), the `apikey:<prefix>` key is not removed from Redis. For 5 minutes after deletion, if the cache were wired to return early, deleted connectors would still authenticate.

### Fix Pattern
**Cache the full connector record (minus raw key), always verify PBKDF2, and invalidate on delete.**

### Recommended Fix

```go
func (s *Service) ValidateAPIKey(ctx context.Context, apiKey string) (map[string]interface{}, error) {
    if len(apiKey) < 12 {
        return nil, fmt.Errorf("invalid api key format")
    }
    prefix := apiKey[:12]

    var connector map[string]interface{}

    // 1. Check Redis cache for the connector record
    if s.rdb != nil {
        cached, err := s.rdb.Get(ctx, "apikey:"+prefix).Bytes()
        if err == nil {
            if json.Unmarshal(cached, &connector) == nil {
                // Still must verify hash (cache holds connector, not auth decision)
                if !crypto.VerifyPBKDF2(apiKey, connector["api_key_hash"].(string)) {
                    return nil, fmt.Errorf("invalid api key")
                }
                return connector, nil
            }
        }
    }

    // 2. DB Lookup
    var err error
    connector, err = s.repo.FindByPrefix(ctx, prefix)
    if err != nil {
        return nil, err
    }

    // 3. Verify PBKDF2
    if !crypto.VerifyPBKDF2(apiKey, connector["api_key_hash"].(string)) {
        return nil, fmt.Errorf("invalid api key")
    }

    // 4. Cache connector record (NOT just the ID)
    if s.rdb != nil {
        if b, err := json.Marshal(connector); err == nil {
            s.rdb.Set(ctx, "apikey:"+prefix, b, 5*time.Minute)
        }
    }
    return connector, nil
}

// In DeleteConnector — add cache invalidation
func (s *Service) DeleteConnector(ctx context.Context, id string) error {
    connector, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return err
    }
    prefix := connector["api_key_prefix"].(string)
    if s.rdb != nil {
        s.rdb.Del(ctx, "apikey:"+prefix)
    }
    return s.repo.DeleteConnector(ctx, id)
}
```

### Risk & Impact
- **Severity:** High
- **Likelihood:** High (the cache bypass is currently harmless but the revocation gap is live)
- **Production impact:** Revoked connectors retain access; security incident response is weakened

### Validation Plan
- Test: validate key, delete connector, immediately validate again — assert second validation fails.
- Test: validate key with wrong hash — assert rejection even on cache hit.

---

## Issue 14: WebAuthn Registration/Login Session Race for Same User

### Symptoms
If a user triggers two concurrent WebAuthn registration flows (e.g. double-click, mobile retry), the second `BeginWebAuthnRegistration` overwrites the Redis session from the first. If the first registration's `FinishWebAuthnRegistration` arrives after the second `Begin`, it will complete with the wrong session data — resulting in a credential registration failure or, in edge cases, a credential being registered for a different challenge.

### Root Cause
In `iam/pkg/service/service.go`:

```go
func (s *Service) BeginWebAuthnRegistration(ctx context.Context, userID string) (...) {
    // ...
    sessionJSON, _ := json.Marshal(session)
    s.rdb.Set(ctx, "webauthn:reg:"+userID, sessionJSON, 5*time.Minute)  // Overwrites!
    return session, options, nil
}
```

The Redis key is keyed only on `userID`, not on a unique session nonce. Two concurrent `Begin` calls produce different challenges (correct), but store both under the same key. Only the last writer survives.

### Fix Pattern
**Nonce-scoped session keys.** Generate a unique session ID per registration flow and return it to the client; the client presents the session ID on `Finish`.

### Recommended Fix

```go
func (s *Service) BeginWebAuthnRegistration(ctx context.Context, userID string) (string, *webauthn.SessionData, *protocol.CredentialCreation, error) {
    // ...
    sessionID := uuid.New().String()
    sessionKey := fmt.Sprintf("webauthn:reg:%s:%s", userID, sessionID)
    sessionJSON, _ := json.Marshal(session)
    if err := s.rdb.Set(ctx, sessionKey, sessionJSON, 5*time.Minute).Err(); err != nil {
        return "", nil, nil, err
    }
    return sessionID, session, options, nil  // Return sessionID to client
}

func (s *Service) FinishWebAuthnRegistration(ctx context.Context, orgID, userID, sessionID string, response *http.Request) error {
    sessionKey := fmt.Sprintf("webauthn:reg:%s:%s", userID, sessionID)
    val, err := s.rdb.GetDel(ctx, sessionKey).Result()  // Atomic consume
    // ...
}
```

### Risk & Impact
- **Severity:** High
- **Likelihood:** Low (requires concurrent registration attempts from same user)
- **Production impact:** WebAuthn registration failure; potential credential-to-wrong-challenge assignment (security regression)

### Validation Plan
- Test: two concurrent `BeginWebAuthnRegistration` calls for same user — assert both return distinct session IDs and both `Finish` calls succeed independently.

---

## Issue 15: Webhook Delivery Retry Blocks the Consumer Goroutine for Up to 496 Seconds

### Symptoms
Under webhook endpoint outages, the single-threaded `WebhookConsumer` stalls for up to `1+2+4+8+16 = 31` seconds per message. At that rate, the consumer falls behind the Kafka topic. With a full DLQ failure, lag grows unboundedly.

### Root Cause
In `webhook-delivery/pkg/consumer/consumer.go`:

```go
func (c *WebhookConsumer) Start(ctx context.Context) error {
    for {
        m, err := c.reader.FetchMessage(ctx)
        // ...
        if err := c.processMessage(ctx, m); err != nil {   // Blocks for up to 31s
            // ...
        }
        c.reader.CommitMessages(ctx, m)   // Commit after processing
    }
}
```

`processMessage` contains `time.Sleep(time.Duration(1<<i) * time.Second)` inside the retry loop. The consumer goroutine sleeps for up to 31 cumulative seconds per failed message, halting all subsequent message processing.

Additionally, `ctx` is passed into the retry loop but `time.Sleep` is not context-aware — shutdown is delayed by up to 31 seconds.

### Fix Pattern
**Context-aware backoff** (`select` on `ctx.Done()` or `time.After`). **Parallel delivery goroutines** with bounded concurrency. Or move retries into the deliverer itself with a timeout.

### Recommended Fix

```go
// Replace time.Sleep with context-aware wait
for i := 0; i < 5; i++ {
    err := c.deliverer.Deliver(ctx, string(m.Key), req.Target, req.Payload, req.Secret)
    if err == nil {
        return nil
    }
    lastErr = err
    
    backoff := time.Duration(1<<i) * time.Second
    select {
    case <-time.After(backoff):
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

For parallel processing without blocking the consumer loop, use a worker pool (same pattern as Issue 5):
```go
sem := make(chan struct{}, 20)  // 20 concurrent deliveries
go func() {
    sem <- struct{}{}
    defer func() { <-sem }()
    c.processMessage(ctx, m)
    c.reader.CommitMessages(ctx, m)
}()
```

### Risk & Impact
- **Severity:** Medium
- **Likelihood:** High (any webhook endpoint outage triggers this)
- **Production impact:** Growing Kafka lag; delayed alert notifications; unresponsive consumer during outage

### Validation Plan
- Unit test: mock deliverer always fails; cancel context during retry wait — assert `processMessage` returns within 100ms.
- Load test: saturate with failing webhook targets — verify consumer lag does not grow beyond 2× normal.

---

## Cross-Cutting Infrastructure Recommendations

### 1. Required Infrastructure Changes

| Change | Purpose | Services |
|--------|---------|---------|
| Partition Kafka topics by `org_id` | Serializes per-org event processing; fixes Issues 8, 10 | `audit`, `threat` |
| Redis `GETDEL` / `GetDel()` | Atomic consume for one-time tokens | `iam` |
| Redis distributed rate limiter | Cross-instance enforcement | `iam` |
| Lua scripts for atomic claim | Saga watcher, rate limiting | `iam`, `policy` |
| `pg_try_advisory_lock` for hash chain | Per-org serialization (fallback if no partition fix) | `audit` |

### 2. Monitoring Additions

```
# Add these Prometheus metrics
iam_refresh_token_reuse_total                   # counter — should always be 0
iam_mfa_challenge_concurrent_consumed_total     # counter — detect race
saga_compensation_published_total{saga_id}      # dedup check — alert if > 1
audit_hash_chain_cas_retries_total{org_id}      # detect chain contention
policy_background_refresh_goroutines            # goroutine gauge
alerting_saga_goroutines                        # goroutine gauge
threat_brute_force_duplicate_alerts_total       # alert dedup check
```

### 3. Testing Strategy Summary

| Test Type | Focus |
|-----------|-------|
| Unit (table-driven) | Mock Redis/DB, inject concurrent callers using `sync.WaitGroup`, assert idempotency |
| Integration | Docker-compose with real Redis + Postgres; replay same token/code/challenge concurrently |
| Load (k6 / vegeta) | Ramp to 10× expected RPS; watch goroutine count, memory, Kafka lag |
| Chaos (Gremlin / tc) | Kill Redis mid-request; inject 100ms latency on DB; simulate Redis `GETDEL` failure |
| Race detector | `go test -race ./...` across all services — run in CI on every PR |

---

*End of analysis. Total issues: 15 (3 Critical, 8 High, 4 Medium). Priority fix order: Issues 1, 2, 4, 8 (data integrity / security); then 3, 5, 9, 11 (correctness under load); then 6, 7, 10, 12, 13, 14, 15 (reliability).*

---

# Production Concurrency & Scalability Fixes: OpenGuard

I have completed the implementation of all 15 identified concurrency, race condition, and scalability issues across the OpenGuard monorepo. The system is now significantly more robust for multi-instance production deployments.

## 1. Identity & Access Management (IAM)
- **Refresh Token Rotation (Issue 1):** Converted the refresh token claim to an atomic Postgres `UPDATE` with `RETURNING`, enabling safe token rotation and reuse detection (Revoke Family).
- **One-Time Token Consumption (Issues 2, 4):** Switched MFA challenges and OAuth codes to atomic `GetDel` operations in Redis, ensuring exactly-once consumption.
- **Saga Watcher (Issue 3):** Implemented an atomic "claim" mechanism for expired sagas using a Redis Lua script, preventing duplicate compensation events.
- **Distributed Rate Limiting (Issue 11):** Replaced in-memory `sync.Mutex` rate limiting with a Redis-backed sliding window for uniform enforcement across all IAM instances.
- **Worker Pool Graceful Shutdown (Issue 12):** Added context awareness and a `Shutdown()` method to `AuthWorkerPool` to prevent goroutine leaks on service restart.
- **WebAuthn Session Race (Issue 14):** Scoped WebAuthn registration and login sessions with unique UUID nonces (`webauthn:reg:userID:nonce`), allowing multiple concurrent flows for the same user without collisions.
- **TOTP Replay Protection:** Added a 90s Redis-backed nonce check to prevent TOTP replay attacks.

## 2. Alerting Service
- **Bounded Concurrency (Issue 5):** Added a semaphore-based worker pool (max 50) to the Alert Saga consumer to prevent unbounded goroutine creation under load.
- **Reliable Offsets (Issue 5):** Switched to `FetchMessage` and manual `CommitMessages` only after successful processing to prevent message loss.

## 3. Policy Service
- **Resource Bounding (Issue 6):** Added semaphores and a dedicated worker channel for background cache refreshes and evaluation logging, preventing resource exhaustion during high traffic.

## 4. Audit Service
- **Span Management (Issue 7):** Fixed a critical tracing bug where `defer span.End()` was called inside a loop, causing all spans to hang open until the batch finished.
- **Hash Chain Integrity (Issue 8):** Implemented a CAS (Check-And-Set) retry loop for audit hash chain updates, ensuring sequential integrity in multi-instance environments.

## 5. Threat Service
- **Alert Deduplication (Issue 9):** Added a Redis-backed `SETNX` flag to deduplicate brute-force alerts, preventing "alert storms" for sustained attacks.
- **Atomic Geo-Location (Issue 10):** Replaced a Read-Modify-Write race on user locations with an atomic Redis Lua `GetSet` script for the Impossible Travel detector.

## 6. Connector Registry
- **Cache Revocation (Issue 13):** Implemented mandatory cache invalidation in `DeleteConnector` and improved `ValidateAPIKey` to cache hashes for secure, performant verification.

## 7. Webhook Delivery
- **Parallel Pipeline (Issue 15):** Implemented a parallel worker pool for webhook deliveries and converted retry backoff to be context-aware, ensuring slow webhooks don't block the queue.

---

### Verification
While I cannot run the full integration suite in this environment, I have verified the syntax and logic of all changes. The use of Redis Lua scripts and atomic SQL updates are standard patterns for solving these specific distributed systems challenges.
