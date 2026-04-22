# §10 — Data Loss Prevention (DLP)

Mirrors BE spec §18 (Phase 10: Content Scanning & DLP). The DLP UI exposes policy management, findings, and the monitor vs block mode toggle.

---

## 10.1 DLP Overview Page

```
Route: /dlp
```

**Finding summary cards (top row):**
- PII findings (last 24h) → `openguard_dlp_findings_total{type="pii"}`
- Credential findings (last 24h) → `{type="credential"}`
- Financial findings (last 24h) → `{type="financial"}`
- Blocked events (last 24h — sync-block mode only)

**Findings trend chart:** LineChart, 14 days, 3 series (PII / credential / financial).

**Recent findings table** (last 20):

| Column | Data |
|---|---|
| Time | `occurred_at`, `<TimeAgo>` |
| Type | `pii` / `credential` / `financial` — Badge |
| Kind | `email` / `ssn` / `credit_card` / `api_key` / `high_entropy` |
| Source | Connector name |
| Action | `monitor` / `mask` / `block` |
| Event | `event_id` → link to audit event detail |

---

## 10.2 DLP Findings Table

```
Route: /dlp → "Findings" tab
```

Full findings list with cursor-based pagination.

**Filter panel:**
- Finding type: PII | Credential | Financial | All
- Finding kind: email | ssn | credit_card | api_key | high_entropy | All
- Action taken: monitor | mask | block | All
- Connector: All | (connector list)
- Time range

**Finding detail drawer:** Click row →

```
Finding ID:   <uuid>
Type:         credential
Kind:         high_entropy
JSON path:    $.payload.auth_token
Action:       mask

Source event:    [event_id]   → link to /audit/[event_id]
DLP policy:      [policy_name] → link to /dlp/policies/[id]

Original value:  [REDACTED — masked by DLP]
Masked value:    "[REDACTED:credential]"

Rule matched:
  Entropy ≥ 4.5 AND length ≥ 24 characters
  OR prefix matches: AKIA, ghp_, sk_live_, ...
```

---

## 10.3 DLP Policy Editor

```
Route: /dlp/policies/new  and  /dlp/policies/[id]
```

### Policy form

```
Policy name *    text input

Mode             [○ Monitor]  [○ Block]
                 ↳ Monitor: events are accepted, findings logged, masked post-hoc.
                 ↳ Block: events with matching content are rejected at ingest (sync path).
                    ⚠ Block mode adds ~30ms to every event ingest (DLP_SYNC_BLOCK_TIMEOUT_MS).

Enabled          Toggle

Rules section:
  The DLP scanning engine has two tiers of built-in detection (PII regex + entropy).
  Custom rules can supplement the built-in detection:

  [+ Add custom rule]
    Rule type: Regex | Keyword list | JSON path
    Pattern: [text input]
    Finding type: pii | credential | financial
    Action: monitor | mask | block
    [Save rule]  [Remove]
```

**Mode toggle — block mode warning:**

```typescript
// When switching from Monitor → Block, show ConfirmDialog:
// [ConfirmDialogComponent properties]:
// Title: "Enable block mode?"
// Description: "Block mode will synchronously scan every incoming event..."
```

**Form submission:**
- New: `POST /v1/dlp/policies`
- Edit: `PUT /v1/dlp/policies/:id`
- On success: invalidate `queryKeys.dlp.policies(orgId)`.

---

## 10.4 DLP Policies List

```
Route: /dlp → "Policies" tab
```

| Column | Data |
|---|---|
| Name | `policy.name` |
| Mode | `monitor` / `block` — Badge |
| Status | Enabled / Disabled |
| Rules | Count |
| Findings (24h) | Count |
| Last modified | `<TimeAgo>` |
| Actions | Edit, Enable/Disable, Delete |

**Enable/Disable toggle:** Optimistic update. No confirmation required (non-destructive).

**Delete:** `ConfirmDialog` with `requireTyped: policy.name`. Deleting an active policy clears all cached DLP rules for the org from Redis (BE handles this internally).

---

## 10.5 Entropy Scanner Config Display

On each DLP policy's detail page, a read-only "Scanner Config" section shows the active detector thresholds:

```
Tier 1: Regex Detection
  Email addresses           ● Active
  US Social Security Numbers ● Active
  Credit card numbers        ● Active  (Luhn validation)
  US phone numbers           ● Active

Tier 2: Entropy Detection
  Minimum length:       24 characters   (DLP_MIN_CREDENTIAL_LENGTH)
  Entropy threshold:    4.5 bits/char   (DLP_ENTROPY_THRESHOLD)
  Known prefixes:       sk_live_, sk_test_, AIza, AKIA, ghp_, github_pat_, xoxb-, xoxp-

These thresholds are configured globally via environment variables.
Contact your administrator to adjust them.
```

---

## 10.6 DLP Stats

```
Route: /dlp → "Stats" tab
```

**Charts:**
- Findings by type and day (14d) — StackedBarChart
- Top finding kinds by volume — HorizontalBarChart
- Block rate (for block-mode policies): blocked events / total events — LineChart

**Data source:** `GET /v1/dlp/stats`. Refreshes every 5 minutes.
