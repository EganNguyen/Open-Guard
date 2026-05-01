# Integration & Webhook Flows

## 1. Overview
This module covers external integrations via Connectors and asynchronous notification delivery.

## 2. Webhook Delivery

### TC-WHK-001: Successful Webhook Delivery
- **Steps:** Trigger system event (e.g. policy created).
- **Expected Results:** Target receives POST with `X-OpenGuard-Signature` (HMAC-SHA256).
- **System Verifications:** SSRF-safe client; `webhook_deliveries` status update.

### TC-WHK-002: Webhook Retry with Exponential Backoff
- **Steps:** Target returns 503.
- **Expected Results:** Automatic retries at increasing intervals; eventually marked `dlq`.

### TC-WHK-003: Webhook Signature Verification (Receiver Side)
- **Flow:** The connected app/connector verifies that the webhook actually came from OpenGuard.
- **Steps:** 
  1. App receives a webhook POST.
  2. App computes HMAC-SHA256 of the body using the shared `secret`.
  3. App compares result with `X-OpenGuard-Signature` header.
- **Expected Results:** App processes only if signatures match; returns 401 otherwise.
- **Verification:** Secure integration preventing unauthorized event triggers.

## 3. Connector Registry

### TC-CON-001: Register Connector and Validate API Key
- **Steps:** `POST /v1/connectors`. Use plaintext key in header.
- **Expected Results:** 201 Created; Key shown once.
- **System Verifications:** PBKDF2-SHA256 hashing of keys (never stored in plaintext).
