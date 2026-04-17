# §09 — Compliance Reports

Mirrors BE spec §14 (Phase 6: Compliance & Analytics). Report generation is async; PDFs are cryptographically signed.

---

## 9.1 Compliance Posture Dashboard

```
Route: /compliance
```

**Posture score card (top):**

```
Overall Compliance Score
        87%           [▓▓▓▓▓▓▓▓▓░]
Trend: +3% vs last month
```

**Control status grid:**

```
GDPR                SOC 2 Type II         HIPAA
  Access Controls ✅   CC6.1 ✅               Access Controls ✅
  Data Retention ✅    CC6.6 ⚠               PHI Encryption ✅
  Right to Erasure ⚠  CC7.2 ✅               Audit Logging ✅
  Data Mapping ❌      CC8.1 ✅               Incident Response ⚠
```

Legend: ✅ Compliant | ⚠ Needs attention | ❌ Non-compliant

Each control item is a clickable card → expands with: current status, evidence (linked audit events), remediation guidance.

**Data source:** `GET /v1/compliance/posture`. Refreshes every 10 minutes.

---

## 9.2 Report Generation Wizard

```
Route: /compliance/reports → "Generate report" button
```

### Step 1: Report type

```
○ GDPR Data Protection Report
○ SOC 2 Type II Readiness Report
○ HIPAA Security Assessment Report

Description of selected report shown below radio group.
```

### Step 2: Time period

```
Report period:
  ○ Last 30 days
  ○ Last quarter
  ○ Custom range  [DateRangePicker]

Include sections: (pre-selected defaults per report type, adjustable)
  ☑ Executive Summary
  ☑ Access Control Analysis
  ☑ Audit Event Statistics
  ☑ Policy Compliance
  ☑ Threat Detection Summary
  ☑ Data Retention Verification
```

### Step 3: Generate

```
[Cancel]  [Generate report]
```

On submit: `POST /v1/compliance/reports`. Backend acknowledges with `{ job_id, status: 'pending' }`.

**Bulkhead response:** If the backend returns `429 CAPACITY_EXCEEDED` (10 concurrent reports limit from BE spec §14.3): show banner "Report queue is full. Please try again in a few minutes." Do not show a spinner — the request did not start.

---

## 9.3 Report List & Status Polling

```
Route: /compliance/reports
```

**Report jobs table:**

| Column | Data |
|---|---|
| Type | GDPR / SOC 2 / HIPAA |
| Period | "Jan 1 – Jan 31, 2024" |
| Requested | `<TimeAgo>` |
| Status | `<Badge>` |
| Sections | Count |
| Actions | Preview / Download / Verify / Delete |

**Status polling:**

```typescript
// src/app/features/compliance/reports/report-list.component.ts
// The component subscribes to the report progress signal.
// Polling is handled within the ComplianceService using RxJS timer and switchMap.
//
// Status transitions: pending → processing → completed | failed
//
// On completion, a notification is dispatched to the GlobalNotificationService.
```

**Download flow:**

```typescript
// "Download" action calls ComplianceService.getReportDownloadUrl(id)
// Returns a pre-signed S3 URL.
// Browser navigation to URL triggers PDF download.
```

---

### PDF Preview Panel

An embedded PDF viewer using the browser's native `<iframe>` or a specialized PDF viewer component.

```typescript
// src/app/features/compliance/reports/preview/pdf-preview.component.ts
// The preview URL is managed as a signal and refreshed if expiration is detected.
```

---

## 9.5 Signature Verification Panel

Each completed report shows:

```
PDF Signature
  ✅ Signature valid
  Algorithm:  RSA-PSS SHA-256 (4096-bit)
  Signed at:  2024-01-15 14:30:00 UTC
  Key ID:     compliance-signing-key-2024

  [Download .sig file]  [View public key]

To verify manually:
  openssl dgst -sha256 -verify compliance-pub.pem
              -sigopt rsa_padding_mode:pss
              -signature report.sig report.pdf
```

The `.sig` file download calls `GET /v1/compliance/reports/:id` → extracts `s3_sig_key` → pre-signed S3 URL for the detached signature file. This matches the RSA-PSS signing in BE spec §14.3.

---

## 9.6 Compliance Stats Charts

Available as cards on the posture page:

- **Events by type (30d):** BarChart from `GET /v1/compliance/stats`
- **Policy coverage:** % of org resources covered by at least one policy — LineChart over time
- **Data retention adherence:** count of events approaching `AUDIT_RETENTION_DAYS` limit — GaugeChart
