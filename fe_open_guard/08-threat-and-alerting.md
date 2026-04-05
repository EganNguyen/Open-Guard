# §08 — Threat Detection & Alerting

Mirrors BE spec §13 (Phase 5: Threat Detection & Alerting). The threat UI surfaces anomalies and provides an alert lifecycle workflow.

---

## 8.1 Alert List Page

```
Route: /threats
```

**Header:** "Threats & Alerts" + alert count badges by severity.

**Severity summary bar:**
```
CRITICAL [N]   HIGH [N]   MEDIUM [N]   LOW [N]
```
Each badge is a filter shortcut.

**Filter panel:**
- Status: Open | Acknowledged | Resolved | All
- Severity: Critical | High | Medium | Low | All
- Detector type: Brute force | Impossible travel | Off-hours | Data exfil | ATO | Privilege escalation | All
- Time range

**Alert table:**

| Column | Data | Notes |
|---|---|---|
| Severity | `<Badge variant={severity}>` | CRITICAL has `animate-ping` pulse indicator |
| Alert type | Detector name | Human-readable |
| Actor | `actor_id` | `<Redactable>` |
| Description | Summary | Auto-generated from detector |
| Score | Risk score `0.0–1.0` | Small progress bar |
| Status | Open / Acknowledged / Resolved | Badge |
| Detected | `occurred_at` | `<TimeAgo>` |
| MTTR | Time to resolve (resolved only) | e.g. "14m" |
| Actions | Acknowledge / Resolve | Inline buttons for Open alerts |

**Cursor-based pagination** (newest first).

**Real-time new alerts:** SSE stream `/api/stream/threats` prepends new alerts with a subtle slide-in animation. Badge count in the sidebar increments.

---

## 8.2 Alert Detail Page

```
Route: /threats/[id]
```

### Alert header

```
[CRITICAL] Privilege Escalation Detected
Actor: user_01j...
Detected: 3 minutes ago
Status: OPEN

[Acknowledge]  [Resolve]
```

### Risk score gauge

```tsx
// Circular gauge showing risk score 0.0–1.0
// Color: ≥0.95=critical, ≥0.80=high, ≥0.50=medium, <0.50=low
// Score breakdown table:
//   Privilege escalation:  0.90  ████████████░░
//   Impossible travel:     0.00  ░░░░░░░░░░░░░░
//   Brute force:           0.00  ░░░░░░░░░░░░░░
//   Composite:             0.90
```

### Saga Timeline

Visualizes the alert lifecycle saga steps (BE spec §13.2):

```tsx
// components/domain/alert-saga-timeline.tsx
//
// Step 1: Alert created        ✅  14:23:07
// Step 2: Notification queued  ✅  14:23:08
// Step 3: SIEM webhook fired   ✅  14:23:09  HTTP 200, 142ms
// Step 4: Audit event written  ✅  14:23:09
//
// Status icons: ✅ completed | ⏳ in-progress (spinner) | ❌ failed | ○ pending
// Failed steps show error detail expandable.
```

### Contributing events

```
Related audit events that triggered this alert:

[1] auth.login.failure  actor: user_01j...  14:22:58  →  /audit/abc
[2] auth.login.failure  actor: user_01j...  14:23:01  →  /audit/def
...
[11] auth.login.failure actor: user_01j...  14:23:07  →  /audit/xyz
```

### Acknowledge / Resolve workflow

**Acknowledge:** Single-click (non-destructive). Status → Acknowledged. Writes `auth.login.failure` audit event.

**Resolve:**
```
Modal:
  Status will change to: Resolved
  MTTR: 14 minutes 32 seconds

  Resolution note (optional):
  [                                    ]

  [Cancel]  [Resolve alert]
```

MTTR is computed by the BE (`occurred_at` to `resolved_at`). Displayed in the alert list and stats.

---

## 8.3 Detector Cards

```
Route: /threats → "Detectors" tab
```

Card grid showing each detector's configuration and current state:

```
┌─────────────────────────────────────┐
│ 🔴 Brute Force                      │
│ Active                              │
│                                     │
│ Threshold: 10 failures / 60 min     │
│ Risk score: 0.80                    │
│                                     │
│ Alerts (7d): 4   Last: 2 hours ago  │
└─────────────────────────────────────┘
```

Detectors are read-only (configured server-side via env vars). The cards are informational.

---

## 8.4 SIEM Webhook Configuration

```
Route: /org/settings → "Integrations" tab → "SIEM Webhook"
```

```
SIEM webhook URL *    text input (HTTPS required)
HMAC secret *        text input (shown masked, copy button)
                     → auto-generated on first save if empty
Replay tolerance     number input, seconds (default: 300)

[Test webhook]  [Save]
```

**Test webhook:** Sends a test payload to the configured URL. Shows result inline: HTTP status, latency, signature verification reminder.

**SSRF validation:** The URL is validated client-side (HTTPS check). The BE performs full SSRF validation on save and rejects RFC 1918 / loopback addresses (spec §13.3).

**Signature documentation panel:**

```
Your receiver must verify:
  Header: X-OpenGuard-Signature: sha256=<hmac>
  Header: X-OpenGuard-Timestamp: <unix_seconds>
  Payload: HMAC-SHA256("<timestamp>.<body>", secret)
  Replay check: |now - timestamp| < 300s

[Copy verification code (Go / Python / Node)]
```

---

## 8.5 Threat Stats Page

```
Route: /threats → "Statistics" tab
```

**Charts:**
- Alert volume by severity (last 30 days) — StackedBarChart
- MTTR by severity (last 30 days) — BarChart
- Alert type distribution (last 30 days) — HorizontalBarChart
- Detection rate by hour of day — HeatMap (shows off-hours anomaly patterns)

**Summary table:**

| Severity | Open | Acknowledged | Resolved (7d) | Avg MTTR |
|---|---|---|---|---|
| Critical | 0 | 1 | 3 | 8m 12s |
| High | 3 | 2 | 14 | 22m 45s |
| Medium | 12 | 5 | 38 | 1h 14m |
| Low | 47 | 18 | 102 | — |
