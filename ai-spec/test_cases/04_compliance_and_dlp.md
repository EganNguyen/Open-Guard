# Compliance & DLP Flows

## 1. Overview
This module covers Data Loss Prevention (DLP) and automated compliance reporting.

## 2. Data Loss Prevention (DLP)

### TC-DLP-001: Content Scan Detects PII
- **Steps:** `POST /v1/dlp/scan` with credit card numbers.
- **Expected Results:** HTTP 200 with masked findings.
- **System Verifications:** Regex + Entropy scanners; Finding persisted in DB (masking raw content).

### TC-DLP-002: DLP Policy CRUD
- **Steps:** Create policy to block `ssn` and `api_key`.
- **System Verifications:** RLS-scoped policy persistence.

### TC-DLP-003: Sync Block Mode Ingestion
- **Config:** `dlp_mode=block`.
- **User Flow:** Connector ingests event containing credit card number.
- **Expected Results:** 422 Unprocessable Entity (`DLP_POLICY_VIOLATION`).
- **System Verifications:** Event NOT written to outbox or Postgres; DLP finding persisted.

### TC-DLP-004: Fail-Closed on DLP Service Outage
- **Config:** `dlp_mode=block`.
- **Preconditions:** DLP service is unreachable.
- **User Flow:** Connector ingests any event.
- **Expected Results:** 503 Service Unavailable or 422 (`DLP_UNAVAILABLE`).
- **System Verifications:** High-assurance "fail-closed" logic blocks all ingestion until DLP is restored.

## 3. Compliance Reporting

### TC-CMP-001: Generate and Download Compliance Report
- **Steps:** `POST /v1/compliance/reports` (SOC2). Poll until `ready`. Download PDF.
- **System Verifications:** ClickHouse audit query -> PDF generation -> RSA-PSS signing -> S3 upload.

### TC-CMP-002: Compliance Posture and Stats
- **Steps:** `GET /v1/compliance/posture`.
- **System Verifications:** Real-time aggregation of audit events in ClickHouse.
