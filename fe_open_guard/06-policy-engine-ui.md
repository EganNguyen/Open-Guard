# §06 — Policy Engine UI

Mirrors BE spec §11 (Phase 3: Policy Engine). The policy UI exposes RBAC rule creation and real-time evaluation.

---

## 6.1 Policy List Page

```
Route: /policies
```

**Header:** "Policies" title + "New policy" button.

**Table columns:**

| Column | Data | Notes |
|---|---|---|
| Name | `policy.name` | Clickable |
| Version | `v{policy.version}` | `font-mono` |
| Resources | Count of rules | e.g. "3 rules" |
| Last modified | `policy.updated_at` | `<TimeAgo>` |
| Status | Active / Archived | Badge |
| Actions | Edit, Delete | — |

**Empty state:** "No policies yet. Create your first policy to control access in connected apps."

---

## 6.2 Policy Editor

```
Route: /policies/new  and  /policies/[id]
```

The policy editor uses a **visual rule builder** — not raw JSON editing. The underlying `logic` field is a JSONB structure, but users interact with a structured form.

### Rule builder

Each policy contains one or more rules. A rule is:

```
IF  [Subject]  [Action]  [Resource]  →  [Effect: Allow | Deny]
```

```typescript
// src/app/features/policies/rule-builder/rule-builder.component.ts
//
// Subject options:  role, user_id, group, "*" (anyone)
// Action options:   read, write, delete, execute, "*"
// Resource options: free-text with autocomplete from recent resources
// Effect:           Allow (default) | Deny
//
// Multiple rules in a policy are evaluated top-to-bottom (explicit deny wins).
// UI shows rules using Angular CDK Drag and Drop for reordering.
```

**Add rule button** → inserts a new empty rule card at the bottom.

**Drag to reorder:** Uses `@angular/cdk/drag-drop` for accessible keyboard-navigable drag and drop.

### Policy form

```
Policy name *      text input
Description        textarea (optional)
Rules              rule builder (see above, minimum 1 rule)

[Cancel]  [Save policy]
```

On save: `PUT /v1/policies/:id` with `If-Match: "{version}"` ETag header. The BE increments `version` atomically (spec §11.5). If the response is `412 Precondition Failed` (concurrent edit), show banner: "This policy was modified by someone else. Reload to see the latest version."

**Optimistic ETag management:**
```ts
// The query cache stores the ETag alongside the policy data.
// On every successful policy load, store ETag from response header.
// On save, include the stored ETag in If-Match.
```

---

## 6.3 Evaluate Playground

```
Route: /policies/playground
```

An interactive tool for testing policy evaluation without deploying code. Mirrors `POST /v1/policy/evaluate`.

**Input panel (left):**
```
User ID *       text input
Action *        text input (e.g. "read")
Resource *      text input (e.g. "documents/finance/*")
User groups     tag input (comma-separated)
Context         expandable JSON editor (key-value pairs)
```

**Result panel (right):**
```typescript
// After "Evaluate" button press:
//
// Permitted: YES (green) or NO (red)
//
// Matched policies:
//   └ [Policy Name] (v2)  → Rule 1: Allow
//
// Cache layer: "DB" | "Redis" | "SDK" (from cache_hit field)
// Latency: 4ms
// Evaluated at: [ISO timestamp]
//
// Raw response: expandable JSON block (using a JSON highlighting component)
```

**Cache hit indicator colors:**
- `none` (DB hit) → cyan (full evaluation)
- `redis` → amber (cached, 30s TTL)
- `sdk` → gray (SDK-local, no server round-trip — note: playground always hits server)

**"Copy as cURL" button** — generates the equivalent `curl` command for integration testing.

---

## 6.4 Evaluation Log Table

```
Route: /policies → "Eval Logs" tab (or linked from policy detail)
```

**Table columns:**

| Column | Data |
|---|---|
| Time | `<TimeAgo>` |
| User | `actor_id` (`<Redactable>`) |
| Action | `action` |
| Resource | `resource` |
| Result | `✅ Permitted` / `❌ Denied` |
| Matched policies | Comma-separated policy names |
| Cache | `none` / `redis` / `sdk` badge |
| Latency | `latency_ms`ms |

**Filter bar:** Result (all/permitted/denied), policy name, user ID, time range.

**Pagination:** Offset-based.

---

## 6.5 Policy Cache Visualization

On the policy detail page, a **Cache Status** section shows:

```
Cache hit rate (last 1h):   Redis 84%  /  DB 16%
Cache invalidation events:  2 in last 24h
Last invalidated:           3 minutes ago

[Cache key pattern]
policy:eval:{org_id}:{sha256(action+resource+user_id+groups)}
```

This helps operators understand if cache TTL adjustments are needed.

---

## 6.6 Circuit Breaker Awareness

When the policy service circuit breaker is open (detected via system health or a `503 UPSTREAM_UNAVAILABLE` response):

```typescript
// Display a non-dismissible warning banner on the Policies section:
// "⚡ Policy service is degraded. The SDK is serving cached decisions
//  (up to 60s TTL). After TTL expiry, all evaluations will be denied.
//  Check System Health for details."
```

The banner is driven by the `SystemService` health signal.
