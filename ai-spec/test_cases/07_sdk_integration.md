# SDK & Connected App Integration

## 1. Overview
This module validates the behavior of the OpenGuard SDKs (Go/TypeScript) within "Connected Apps". It emphasizes the "Beside-not-in-front" architecture and the "Fail-Closed" security posture.

## 2. Core SDK Lifecycle

### TC-SDK-001: SDK Initialization and API Key Validation `[E2E] [INTEGRATION]`
- **Flow:** App starts up and initializes the OpenGuard client with an API Key and Org ID.
- **Verification:**
  - `[INTEGRATION]`: SDK successfully performs an initial handshake with the Control Plane.
  - `[INTEGRATION]`: SDK retrieves the initial set of relevant policies for the Org.
  - `[E2E]`: Heartbeat metrics are visible in the Control Plane dashboard.

### TC-SDK-002: SDK Local Cache Hit (Low Latency) `[UNIT] [E2E]`
- **Flow:** App performs multiple authorization checks for the same subject/resource.
- **Steps:**
  1. `client.allow(user, 'read', 'doc')` -> Remote call (Latency: ~50ms).
  2. `client.allow(user, 'read', 'doc')` -> Local cache hit (Latency: <1ms).
- **Verification:** 
  - `[UNIT]`: Cache logic correctly handles TTL and key collision.
  - `[E2E]`: Second call returns identical result without triggering a network request.

## 3. Resilience & Security (Fail-Closed)

### TC-SDK-003: Stale-While-Unavailable (Grace Period) `[E2E]`
- **Flow:** The OpenGuard Control Plane becomes unreachable, but the SDK has cached decisions.
- **Steps:**
  1. Perform successful `allow` check (Populates cache).
  2. Simulate Control Plane outage (e.g. cut network).
  3. Perform `allow` check within the 60s Grace Period.
- **Expected Results:** SDK returns the **cached result** (Stale) despite the outage.
- **System Verifications:** Log shows warning: `Serving STALE cached decision during outage`.

### TC-SDK-004: Fail-Closed Behavior
- **Flow:** The Control Plane is down and the cache is expired or empty.
- **Steps:**
  1. Simulate Control Plane outage.
  2. Perform `allow` check for a new resource (not in cache) OR after Grace Period expires.
- **Expected Results:** SDK returns `false` (Access Denied).
- **Verification:** Ensure the app does not "Fail-Open" and grant unauthorized access.

## 4. Advanced Protection Scenarios

### TC-SDK-006: Contextual Policy Enforcement
- **Flow:** The app passes dynamic context (e.g., user's department, project sensitivity, or source IP reputation) to OpenGuard.
- **Steps:**
  1. App calls `client.allow(user, 'read', 'secret_doc', { "department": "legal", "clearance": 5 })`.
- **Expected Results:** OpenGuard evaluates the CEL policy using the provided context and returns a specific decision.
- **Verification:** Confirm that the `context` JSON reaches the Policy Service and is correctly bound to the CEL `request.context` object.

### TC-SDK-007: Outbound DLP Protection
- **Flow:** The app scans data for PII/Secrets before sending it to an external third-party or another service.
- **Steps:**
  1. App receives a request to export data.
  2. App calls `v1/dlp/scan` via the SDK/API.
  3. DLP findings indicate high-risk PII.
- **Expected Results:** App blocks the export based on the DLP finding.
- **Verification:** `dlp_findings` log in OpenGuard shows the blocked attempt.

### TC-SDK-008: SDK mTLS Handshake & Verification
- **Flow:** The SDK is configured with client certificates (`WithMTLS`) to authenticate itself to the Control Plane.
- **Verification:**
  - Control Plane logs show a successful mTLS handshake with the SDK's common name.
  - SDK fails to connect if a self-signed or expired certificate is provided (unless `WithInsecureSkipVerify` is explicitly set).

### TC-SDK-009: SDK Resilience (Retry & Backoff)
- **Flow:** The SDK handles transient network errors or rate limits (429) from OpenGuard.
- **Steps:**
  1. Simulate a 429 response from the Control Plane.
  2. SDK attempts retries with exponential backoff and jitter.
- **Verification:** App remains stable; logs show retry attempts before eventually returning a decision (or failing closed).

## 5. Observability

### TC-SDK-010: Distributed Tracing Propagation
- **Flow:** Trace context from the Connected App is propagated to OpenGuard services.
- **Steps:** 
  1. Connected App starts a span and calls the SDK.
  2. SDK includes `traceparent` header in its request to OpenGuard.
- **Verification:** A single trace is visible in the telemetry dashboard covering the journey from the Connected App -> Control Plane -> Policy/Audit Services.
