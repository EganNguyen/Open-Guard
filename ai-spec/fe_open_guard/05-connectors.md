# §05 — Connectors

Mirrors BE spec §2.6 (Connector Credential Flow), §9.6 (Admin UI spec), and §10.4 (Control Plane Foundation).

---

## 5.1 Connector List Page

```
Route: /connectors
```

**Header:** "Connectors" title + "Register app" button (→ `/connectors/new`).

**Filters bar:**
- Status filter: All | Active | Suspended | Pending
- Search by name (debounced, `?q=` param)

**Table columns:**

| Column | Data | Notes |
|---|---|---|
| Name | `connector.name` | Clickable → detail page |
| Status | `<Badge>` | active=success, suspended=danger, pending=warning |
| Scopes | Comma-separated scope chips | `events:write`, `policy:evaluate`, etc. |
| Events (30d) | `connector.event_volume_30d` | From ClickHouse `event_counts_daily` |
| Last event | `connector.last_event_at` | `<TimeAgo>` |
| Created | `connector.created_at` | `<TimeAgo>` |
| Actions | Dropdown menu | View, Suspend/Activate, Test webhook, Delete |

**Pagination:** Offset-based (page 1…N). `per_page=50`.

**Empty state:** "No connectors registered. Register your first app to start ingesting events." + CTA button.

**Suspend/Activate action:**
- Uses `ConfirmDialog` with `requireTyped: connector.name` for suspension.
- Activation is single-confirm (less destructive).
- Optimistic update per §2.5.
- After suspension: note in toast "Cache invalidated immediately. Requests with this key will fail within seconds."

---

## 5.2 Connector Registration Wizard

```
Route: /connectors/new
```

A 3-step wizard. Progress indicated by step dots at the top.

### Step 1: Basic info

```
Fields:
  App name *      text input, min 2 chars, max 64 chars
  Webhook URL     text input, URL format, must be HTTPS
                  → validated client-side (URL parse + HTTPS check)
                  → server-side SSRF check on submit (BE rejects non-HTTPS / RFC 1918 IPs)
  Description     textarea, optional, max 256 chars
```

### Step 2: Permissions (Scopes)

```
Multi-select checkbox group:
  ☐ events:write       Ingest audit events
  ☐ policy:evaluate    Evaluate RBAC policies
  ☐ audit:read         Read audit log
  ☐ scim:write         Provision users via SCIM
  ☐ dlp:scan           Submit content for DLP scanning

At least one scope must be selected.
Scope descriptions rendered from a static map. Dangerous scopes (scim:write) show a yellow warning icon.
```

### Step 3: Review & Create

Summary of name, webhook URL, scopes. "Create connector" button.

```typescript
// On submit:
// 1. Generate idempotencyKey (Signal, initialized once)
// 2. POST /v1/admin/connectors → { connector, api_key_plaintext }
// 3. Navigate to the API key reveal screen
```

### API Key Reveal Screen

This is the most security-critical UI in the entire dashboard:

```typescript
// src/app/features/connectors/key-reveal/key-reveal.component.ts
// Shown immediately after connector creation.
// The plaintext key is passed via router state (not URL, not localStorage).

// UI:
//   ┌─────────────────────────────────────────────────────┐
2: //   │  ⚠️  Save this API key — it won't be shown again    │
//   │                                                     │
//   │  sk_og_[MASKED — click to reveal]    [👁] [Copy]   │
//   │                                                     │
//   │  Prefix: abc12345 (use for debugging / logs)        │
//   │                                                     │
//   │  [ I've saved the key securely ]  ← must click     │
//   └─────────────────────────────────────────────────────┘
//
// The key is hidden by default. "Reveal" button shows it once.
// Navigating away without acknowledging shows a browser 'beforeunload' warning.
// The plaintext key is NEVER stored in Signal state beyond this screen.
```

---

## 5.3 Connector Detail Page

```
Route: /connectors/[id]
```

**Header:** Connector name + status badge + actions dropdown (Suspend/Activate, Test webhook, Delete).

**Tabs:**
1. **Overview** — metadata card (name, scopes, webhook URL, created date, last event)
2. **Webhook Deliveries** — delivery log (see §5.4)
3. **Event Volume** — chart (events/day, last 30 days)
4. **Settings** — edit form (webhook URL, scopes)

**Settings tab — edit form:**

```
Webhook URL  (editable, same HTTPS validation as registration)
Scopes       (multi-select, same as registration)
[Save changes]
```

On save: `PATCH /v1/admin/connectors/:id`. The BE immediately invalidates the Redis cache for this connector (spec §2.6). Toast: "Settings saved. Cache updated — changes are effective immediately."

**Danger zone (bottom of Settings tab):**
- "Rotate API key" button → ConfirmDialog → `DELETE /v1/admin/connectors/:id/api-key` then `POST /v1/admin/connectors/:id/api-key` → reveals new key.
- "Delete connector" → `ConfirmDialog` with `requireTyped: connector.name` → `DELETE /v1/admin/connectors/:id`.

---

## 5.4 Webhook Delivery Log

```
Route: /connectors/[id]/deliveries
Tab: "Webhook Deliveries"
```

**Table columns:**

| Column | Data |
|---|---|
| Delivered at | `<TimeAgo>` |
| Event type | e.g. `policy.changed` |
| HTTP status | Color-coded: 2xx=green, 4xx=amber, 5xx=red |
| Latency | e.g. `234ms` |
| Attempts | e.g. `1/5` |
| Status | `delivered` / `failed` / `retrying` / `dead` |

**Cursor-based pagination** (newest first). Load more button at the bottom.

**Row expand:** Click row → reveals full request/response headers and body (truncated at 1KB with "Show full" toggle).

**Delivery status details:**
- `dead` rows: yellow warning icon + "Moved to DLQ" note. Link to DLQ inspector in admin section.
- `retrying` rows: shows next retry timestamp via `<TimeAgo>`.

**"Send test webhook" button:** Calls `POST /v1/admin/connectors/:id/test`. Shows spinner during in-flight. On success: toast + new row appears at top of delivery log (optimistic insert).

---

## 5.5 Scope Enforcement UI

When a connector's API key is used against an endpoint it lacks scope for, the BE returns `403 INSUFFICIENT_SCOPE`. The delivery log shows this as a failed delivery with the error code displayed. The detail drawer suggests: "Add the `[scope]` permission to this connector's settings to resolve this."
