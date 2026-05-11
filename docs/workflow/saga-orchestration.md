# Saga Orchestration — Workflow

## Level 1: High-Level Architecture

```
                          ┌────────────────────────────────────────────────┐
                          │              EVENT PRODUCERS                  │
                          │                                                │
                          │  Service Layer (users.go, orgs.go)            │
                          │    ┌──────────────────────────────────────┐   │
                          │    │ RegisterUser() → user.created        │   │
                          │    │   + Redis ZADD saga:deadlines 40s    │   │
                          │    │ ReprovisionUser() → user.reprovision │   │
                          │    │ DeleteUser() → user.deleted          │   │
                          │    │ OffboardOrg() → org.offboard         │   │
                          │    └──────────┬───────────────────────────┘   │
                          │               │                               │
                          │               │  Transactional Outbox         │
                          │               │  (outbox_records → pg_notify) │
                          └───────────────┼───────────────────────────────┘
                                          │
                                          ▼
                          ┌────────────────────────────────────────────────┐
                          │           KAFKA: saga.orchestration           │
                          │          12 partitions, lz4 compression       │
                          └────────────┬──────────────────────────────────┘
                                       │
                    ┌──────────────────┼──────────────────┐
                    │                  │                    │
                    ▼                  ▼                    ▼
          ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
          │  SAGA CONSUMER  │  │  SAGA WATCHER   │  │  AUDIT SERVICE  │
          │  openguard-     │  │  (Redis ticker)  │  │  (side effect)  │
          │  saga-v1        │  │                  │  │                 │
          │                 │  │  polls           │  │  references     │
          │  dispatches:    │  │  saga:deadlines  │  │  saga.          │
          │  • provisioned  │  │  every 10s via   │  │  orchestration  │
          │  • failed       │  │  Lua script      │  │  for audit      │
          │  • offboard     │  │                  │  │  enrichment     │
          └────────┬────────┘  └────────┬─────────┘  └─────────────────┘
                   │                    │
                   ▼                    │
          ┌─────────────────┐           │
          │  SERVICE LAYER  │           │
          │                 │           │
          │  UpdateUserStatus           │
          │  OffboardOrg()  │           │
          └─────────────────┘           │
                                        │  publishes compensation events
                                        ▼
                              ┌─────────────────────┐
                              │  user.provisioning  │
                              │  .failed            │
                              └──────────┬──────────┘
                                         │
                                         ▼
                                  saga.orchestration
                                  (back to consumer)
```

## Level 2: Detailed Flows

### 2.1 User Provisioning Saga (Happy Path)

```
  UserService                Outbox              saga.orchestration     SagaConsumer        DB
      │                        │                       │                    │                │
      │  RegisterUser()        │                       │                    │                │
      │───┐                    │                       │                    │                │
      │   │ INSERT outbox      │                       │                    │                │
      │   │ (user.created)     │                       │                    │                │
      │   │ ZADD saga:deadlines│                       │                    │                │
      │   │ <sagaID, now+40s>  │                       │                    │                │
      │<──┘                    │                       │                    │                │
      │                        │  pg_notify → relay    │                    │                │
      │                        │────┬──────────────────│──── user.created ──│                │
      │                        │    │  (poll if miss)  │                    │                │
      │                        │    ▼                  │                    │                │
      │                        │  relay publishes      │                    │                │
      │                        │──→ user.created ──────│───────────────────│                │
      │                        │                       │                    │                │
      │   (external: SCIM      │                       │                    │  Start saga    │
      │    provisioning)       │                       │                    │───┐            │
      │                        │                       │                    │   │ UPDATE     │
      │                        │                       │                    │   │ user       │
      │                        │                       │                    │   │ status =   │
      │                        │                       │                    │   │ 'active'   │
      │                        │                       │                    │<──┘            │
      │                        │                       │                    │                │
      │  ← user.scim.provisioned event ────────────────── user.scim.       │                │
      │                        │                       │   provisioned ────│                │
      │                        │                       │                    │───┐            │
      │                        │                       │                    │   │ UPDATE     │
      │                        │                       │                    │   │ status =   │
      │                        │                       │                    │   │ 'active'   │
      │                        │                       │                    │<──┘            │
      │                        │                       │                    │                │
```

### 2.2 User Provisioning Saga (Timeout / Failure Path)

```
  UserService          Redis(saga:deadlines)       SagaWatcher          saga.orchestration       SagaConsumer
      │                        │                       │                       │                     │
      │  RegisterUser()        │                       │                       │                     │
      │───┐                    │                       │                       │                     │
      │   │ ZADD <sagaID,      │                       │                       │                     │
      │   │      now+40s>      │                       │                       │                     │
      │<──┘                    │                       │                       │                     │
      │                        │                       │                       │                     │
      │                   (40 seconds pass)            │                       │                     │
      │                        │                       │                       │                     │
      │                        │  ┌─ every 10s ─┐     │                       │                     │
      │                        │  │ tick        │     │                       │                     │
      │                        │  │ ZRANGEBYSCORE│     │                       │                     │
      │                        │  │ saga:deadlines│    │                       │                     │
      │                        │  │ -inf {now}   │     │                       │                     │
      │                        │  │ LIMIT 0 100  │     │                       │                     │
      │                        │  │──────────────│─────│                       │                     │
      │                        │  │ ZREM members │     │                       │                     │
      │                        │  │──────────────│─────│                       │                     │
      │                        │  └──────────────┘     │                       │                     │
      │                        │                       │  Publish              │                     │
      │                        │                       │──┐                    │                     │
      │                        │                       │  │ user.provisioning  │                     │
      │                        │                       │  │ .failed            │                     │
      │                        │                       │<─┘                    │                     │
      │                        │                       │  ─── compensation ────│──── user.           │
      │                        │                       │    event (saga_id)    │   provisioning.     │
      │                        │                       │                       │   failed ──────────│
      │                        │                       │                       │                     │───┐
      │                        │                       │                       │                     │   │
      │                        │                       │                       │                     │   │ UPDATE
      │                        │                       │                       │                     │   │ status =
      │                        │                       │                       │                     │   │ 'provisioning_failed'
      │                        │                       │                       │                     │<──┘
```

### 2.3 Org Offboarding Saga

```
  OrgService              Outbox              saga.orchestration         SagaConsumer              DB
      │                     │                       │                       │                      │
      │  OffboardOrg()      │                       │                       │                      │
      │───┐                 │                       │                       │                      │
      │   │ INSERT outbox   │                       │                       │                      │
      │   │ (org.offboard)  │                       │                       │                      │
      │<──┘                 │                       │                       │                      │
      │                     │  relay → publish      │                       │                      │
      │                     │── org.offboard ────────│──────────────────────│                      │
      │                     │                       │                       │───┐                  │
      │                     │                       │                       │   │ OffboardOrg()    │
      │                     │                       │                       │───│──────────────────│
      │                     │                       │                       │   │ (delete users,   │
      │                     │                       │                       │   │  revoke sessions,│
      │                     │                       │                       │   │  cleanup org)    │
      │                     │                       │                       │<──┘                  │
      │                     │                       │                       │                      │
      │                     │  publish              │                       │                      │
      │                     │  org.iam.offboarded   │                       │                      │
```

### 2.4 State Machine

```
                    ┌─────────────────────────────────────────────────────────┐
                    │              SAGA LIFECYCLE (per saga_id)               │
                    │                                                         │
                    │  ┌──────────┐    40s timeout      ┌────────────────┐   │
                    │  │ PENDING  │ ──────────────────── │ TIMED_OUT      │   │
                    │  │          │     (Watcher fires)  │                │   │
                    │  └────┬─────┘                      │ compensation: │   │
                    │       │                            │ provisioning  │   │
                    │       │                            │ _failed       │   │
                    │       │ SCIM completes             └────────────────┘   │
                    │       ▼                                                 │
                    │  ┌──────────┐                                           │
                    │  │ ACTIVE   │                                           │
                    │  │          │                                           │
                    │  └──────────┘                                           │
                    └─────────────────────────────────────────────────────────┘
```

## Level 3: Deep Dive

### 3.1 Event Catalog

All events on the `saga.orchestration` topic (12 partitions, lz4):

| Event | Producer | Consumer Handler | Effect |
|---|---|---|---|
| `user.created` | `service/users.go:RegisterUser` | (watcher monitors deadline) | Redis ZADD `saga:deadlines` with 40s TTL |
| `user.scim.provisioned` | External SCIM or reprovision flow | `UpdateUserStatus(userID, "active")` | Completes provisioning saga |
| `user.provisioning.failed` | Watcher (timeout) or manual | `UpdateUserStatus(userID, "provisioning_failed")` | Compensation action |
| `user.reprovision` | `service/users.go:ReprovisionUser` | (triggers re-provisioning) | Restart provisioning flow |
| `user.deleted` | `service/users.go:DeleteUser` | (triggers cleanup) | User deletion orchestration |
| `user.updated` | `service/users.go:PatchUser` | (triggers update) | User attribute sync |
| `org.offboard` | `service/users.go:OffboardOrg` | `OffboardOrg(orgID)` | Org deactivation flow |
| `org.iam.offboarded` | Saga consumer | (outcome event) | Confirms org offboarded |

### 3.2 Redis Key Structure

| Key | Type | Purpose | TTL |
|---|---|---|---|
| `saga:deadlines` | ZSET | Sorted set of `(sagaID, deadline_timestamp)` | N/A (managed by Lua ZREM) |

The deadline timestamp is `time.Now().Unix() + 40` (40 seconds from creation).

### 3.3 Lua Script (Atomic Claim)

```lua
-- Watcher.checkExpired(): claimed expired sagas atomically
local members = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 100)
if #members == 0 then return {} end
redis.call('ZREM', KEYS[1], unpack(members))
return members
```

**Why Lua:** ZRANGEBYSCORE + ZREM race condition — without Lua, two watcher instances could both read the same expired sagas and both publish compensation events. The script atomically reads and removes.

### 3.4 Code Paths

#### Saga Consumer (`services/iam/pkg/saga/consumer.go`)

```
Start(ctx):
  loop:
    m = reader.ReadMessage(ctx)
    event = json.Unmarshal(m.Value)
    switch event.Event:
      "user.provisioning.failed":
        userID = event.SagaID  # fallback if UserID empty
        UpdateUserStatus(userID, "provisioning_failed")
      "user.scim.provisioned":
        UpdateUserStatus(userID, "active")
      "org.offboard":
        OffboardOrg(orgID)
```

**Edge case:** When `user.provisioning.failed` comes from the Watcher, the `saga_id` field contains the user ID (the SagaID *is* the UserID in timeout scenarios). The consumer checks `event.UserID` first, falls back to `event.SagaID`.

#### Saga Watcher (`services/iam/pkg/saga/watcher.go`)

```
Run(ctx):
  ticker = 10s
  select:
    case <-ticker.C:
      checkExpired(ctx)
    case <-ctx.Done():
      return

checkExpired(ctx):
  now = time.Now().Unix()
  sagaIDs = Lua(ZRANGEBYSCORE + ZREM, "saga:deadlines", -inf, now, LIMIT 0, 100)
  for sagaID in sagaIDs:
    payload = {event: "user.provisioning.failed", saga_id: sagaID, compensation: true, reason: "saga_timeout", ts: now}
    publisher.Publish("saga.orchestration", sagaID, payload)
```

**Failure mode:** If `Publish` fails, the saga is already removed from Redis deadlines. No retry. The `consumer.go` comment acknowledges this gap.

### 3.5 Consumer Group Configuration

| Param | Value |
|---|---|
| Group ID | `openguard-saga-v1` |
| Topic | `saga.orchestration` |
| Brokers | Comma-separated from env (e.g. `localhost:9092`) |
| Min Bytes | 1 |
| Max Bytes | 10 MB |

### 3.6 Retry & Recovery

| Component | Failure | Behavior |
|---|---|---|
| Saga Consumer | Kafka connection error | Retries `Start()` every 5s |
| Saga Consumer | Unmarshal error | Log + skip message (no retry) |
| Saga Watcher | Publish failure | Log only — saga deadline already removed from Redis |
| Saga Watcher | Redis connection error | Returns silently, retries on next 10s tick |

### 3.7 Alerting Saga (separate system)

The alerting service has its own saga orchestrator at `services/alerting/pkg/saga/saga.go` with a different lifecycle:

```
Alert Saga (4 steps):
  1. Persist alert to MongoDB
  2. Notify webhook subscribers
  3. Deliver SIEM webhook (disabled: siemURL = "")
  4. Audit log (publish to audit.trail)
```

This is documented in detail in [alerting-siem.md](alerting-siem.md). Key difference: the alerting saga is a **single-service, single-process saga** with retry logic per step, while the IAM saga is a **distributed saga** spanning services via Kafka.

### 3.8 Design Decisions & Rationale

| Decision | Rationale | Alternative Considered |
|---|---|---|
| Redis ZSET for deadlines | Lightweight, no extra infra | Temporal (heavy for simple timeout) |
| Lua script for atomic claim | Prevents duplicate compensation | Distributed lock (complex, fragile) |
| 40s provisioning TTL | Balances SCIM response time vs. user experience | Configurable per-org (future) |
| 10s watcher tick | Granular enough for 40s window, low Redis load | 1s tick (wasteful), 30s tick (delayed recovery) |
| Consumer group `openguard-saga-v1` | Allows multiple consumers for parallelism | Single consumer (bottleneck) |
| Event field name inconsistency | Uses `"event"` field (not `"event_type"` or `"type"`) | (historical — matches IAM outbox convention) |
