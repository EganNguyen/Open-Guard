# Security, Resilience & Audit Flows

## 1. Overview
This module covers cross-cutting security concerns, multi-tenancy enforcement, and tamper-evident auditing.

## 2. Session Security & Multi-Tenancy

### TC-SEC-001: Session Revocation on Risk Score Threshold
- **Steps:** Submit refresh token from different User-Agent and IP subnet.
- **Expected Results:** 401 Unauthorized (`SESSION_REVOKED_RISK`).
- **System Verifications:** Risk calculation (+60 for UA, +40 for IP) ≥ 80 threshold triggers family revocation.

### TC-SEC-002: Cross-Tenant Policy Isolation (RLS Enforcement)
- **Steps:** Org A user attempts `GET /v1/policies/{org_b_id}`.
- **Expected Results:** 404 Not Found (Data hidden by Postgres RLS).
- **System Verifications:** `SET LOCAL app.org_id` is correctly scoped per request context.

### TC-SEC-003: Rate Limiting
- **Steps:** 6 login attempts within 1 second.
- **Expected Results:** 6th request returns 429 Too Many Requests.

### TC-SEC-004: Control-Plane Circuit Breaker
- **Steps:** Simulate 5 consecutive failures in Policy Service.
- **Expected Results:** Next call returns 503 from Control Plane immediately.

## 3. Audit Trail Integrity

### TC-AUD-001: Audit Event Ingestion and Hash Chain
- **Steps:** Trigger series of mutations.
- **System Verifications:** Each event's `prev_hash` = SHA-256 of prior event.
- **Persistence:** MongoDB `writeconcern.Majority` and CAS on chain head document.

### TC-AUD-002: Audit Chain Gap Detection
- **Preconditions:** A sequence of 5 valid events exists.
- **Steps:** Manually delete event #3 from the database or tamper with its `prev_hash`.
- **Expected Results:** `GET /v1/audit/integrity` returns `ok: false` and identifies the break at sequence #3.
- **System Verifications:** Background worker or on-demand query validates the cryptographic chain integrity.

## 4. Negative Scenarios

### TC-NEG-001: Unauthenticated Access
- **Steps:** Access protected routes without `Authorization` header.
- **Expected Results:** 401 Unauthorized.

### TC-NEG-002: Expired JWT Rejected
- **Steps:** Submit expired token.
- **Expected Results:** 401 Unauthorized.

### TC-NEG-003: SSRF Attempt via Webhook Target
- **Steps:** Use AWS metadata IP (`169.254.169.254`) as webhook URL.
- **Expected Results:** Immediate block by `SafeHTTPClient`.
