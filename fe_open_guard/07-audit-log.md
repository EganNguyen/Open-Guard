# §07 — Audit Log

Mirrors BE spec §12 (Phase 4: Event Bus & Audit Log). The audit log is append-only, cryptographically hash-chained, and supports real-time streaming.

---

## 7.1 Audit Event List Page

```
Route: /audit
```

**Live stream mode** (default): New events appear at the top in real-time via SSE. A "Live" indicator pulses in cyan.

**Historical mode**: User can pause the stream and use filters to query historical data.

### Stream / Pause toggle

```tsx
// components/domain/audit-stream-toggle.tsx
// "use client"
//
// [● LIVE]  ←→  [⏸ PAUSED | Showing results for: last 24h]
//
// When LIVE: useSSE('/api/stream/audit') prepends events to local state buffer.
//            Buffer capped at 200 events to prevent unbounded memory growth.
//            Older events beyond 200 are dropped (most recent retained).
//
// When PAUSED: useInfiniteQuery on /audit/events with cursor pagination.
//              Filter panel becomes active.
//              "Resume live" button re-connects SSE.
```

### Filter Panel (visible in PAUSED mode)

```
Time range     DateRangePicker (last 1h / 6h / 24h / 7d / 30d / custom)
Event type     Multi-select dropdown (populated from distinct types in last 30d)
Actor ID       Text input (user ID, service name, "system")
Actor type     Multi-select: user | service | system
Source         Multi-select: iam | policy | control-plane | connector:*
               → "connector:*" expands to list of registered connectors

[Apply filters]  [Clear]
```

Filters are synced to the URL via `nuqs` (URL-based state management):
- `?from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z&type=auth.login.failure&actor_type=user`

### Event Table

| Column | Data | Notes |
|---|---|---|
| Time | `occurred_at` | `<TimeAgo>` + ISO on hover |
| Type | `event.type` | Dot-separated, monospace |
| Actor | `actor_id` | `<Redactable type="user-id">` |
| Source | `event.source` | Badge |
| Event source | Internal / `connector:AcmeApp` | Differentiates internal vs connected app events |
| Chain seq | `chain_seq` | `font-mono`, small |

**Row click** → event detail drawer (see §7.2).

**Cursor pagination** (newest first). "Load more" button appends older events below. No page numbers for audit log.

---

## 7.2 Event Detail Drawer

Slides in from the right when a row is clicked. Does not navigate away from the list.

```
Event ID:      <uuid>                         [Copy]
Type:          auth.login.failure
Occurred at:   2024-01-15 14:23:07 UTC        [Copy ISO]
Chain seq:     4821
Prev hash:     a3f8...d912                    [Copy]
Chain hash:    9b2c...1a47                    [Copy]

Actor
  ID:          user_01j...                   [Copy] [Redactable]
  Type:        user

Source:        iam
Event source:  internal

Payload
  ┌──────────────────────────────────────────┐
  │ {                                        │
  │   "ip": "203.0.113.42",                  │
  │   "email": "u***@example.com",           │  ← masked by DLP if SSN/CC detected
  │   "failure_reason": "invalid_password"   │
  │ }                                        │
  └──────────────────────────────────────────┘
                                      [Copy JSON]

Related alerts
  └ [⚠ HIGH] Brute force detected — 11 failures  →  /threats/abc-123
```

**DLP masking indicator:** If any payload fields were masked by the DLP service, show a banner: "⚑ Some fields in this event were masked by your DLP policy. [View findings →]"

---

## 7.3 Hash Chain Integrity Badge

Displayed in the audit page header:

```tsx
// components/domain/integrity-badge.tsx
// useQuery(queryKeys.audit.integrity(orgId), { refetchInterval: 300_000 }) — every 5min
//
// Results:
//   ok: true   → [🔒 Chain integrity verified]  (green)
//   ok: false  → [⚠ Chain integrity failure]   (red, pulsing)
//               "Gap detected at chain_seq 4820. Contact your security team."
//               Link → /audit/integrity-report
//
// NOTE: This endpoint uses MongoDB primary (BE spec §2.4 CQRS exception).
// Slightly higher latency is expected and acceptable — show a loading state.
```

When integrity fails: automatically create a HIGH threat alert via `POST /v1/threats/alerts` (server-side, from the background verification job in the BE — not client-side). The frontend simply reflects the alert.

---

## 7.4 Export Jobs

```
Route: /audit/exports
```

**Create export:**

```
Format:    CSV | JSON
Time range: DateRangePicker
Event types: Multi-select (optional)
[Generate export]
```

Submit → `POST /audit/export` → returns `{ job_id, status: 'pending' }`.

**Job status polling:**
```tsx
// useQuery(queryKeys.audit.exportJob(jobId), {
//   refetchInterval: (query) => query.state.data?.status === 'completed' ? false : 3000,
// })
//
// Status flow: pending → processing → completed | failed
// Spinner during pending/processing, download button on completed.
```

**Jobs table (last 10 exports):**

| Column | Data |
|---|---|
| Created | `<TimeAgo>` |
| Format | CSV / JSON |
| Time range | "Jan 1 – Jan 7" |
| Status | Badge |
| Size | e.g. "4.2 MB" |
| Actions | Download / Delete |

**Download:** Clicking "Download" calls `GET /audit/export/:job_id/download` directly as a browser download (anchor `href` with `download` attribute). Not via `apiFetch` — the response is a binary stream.

---

## 7.5 Audit Stats Widget

Available on the Overview page and as a card on the audit page header:

```
Events today:      1,847,392
Events this week:  12,340,019
Most common type:  auth.login.success (34%)
Unique actors:     2,847
```

Data from `GET /audit/stats`. Refreshes every 5 minutes.
