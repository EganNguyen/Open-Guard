# OpenGuard E2E Test Case: Phases 1-3 Final Verification

This test plan provides a comprehensive walkthrough of the OpenGuard core architecture (Phases 1, 2, and 3). It simulates a real-world user journey, exercising public-facing APIs, the Control Plane Gateway, and asynchronous background processes (Kafka, Outbox, Redis caching, and ClickHouse/MongoDB).

---

## Phase 1: Foundation (Identity, MFA, and Control Plane)

### **Step 1: Tenant Onboarding & Admin Provisioning**
*   **Action:** A new enterprise customer signs up via the UI.
*   **APIs Triggered:**
    *   `POST /auth/register` (Creates the Organization and the initial Admin User).
    *   `POST /auth/login` (Admin logs in with password, receives JWT).
*   **Validation:**
    *   An outbox record for `auth.login.success` is generated in `iam_outbox_records`.
    *   The browser receives a signed JWT with `org_id` and `sub` claims.
    *   **Automation:** Verified via `web/e2e/real.spec.ts`.

### **Step 2: Security Hardening (MFA Integration)**
*   **Action:** Verify the system's preparedness for MFA enrollment.
*   **APIs Triggered:**
    *   `POST /auth/mfa/enroll` (Requests TOTP secret — Returns `501 NOT_IMPLEMENTED` in stub phase).
    *   `POST /auth/mfa/verify` (Validates TOTP — Returns `501 NOT_IMPLEMENTED`).
*   **Validation:** Ensuring stubs explicitly return 501 prevents unexpected 404/500 errors.
    *   **Automation:** Verified via `services/iam/integration_test.go`.

### **Step 3: Internal API Protection (mTLS Bypass)**
*   **Action:** Administrative tools call management APIs (e.g., listing users).
*   **APIs Triggered:**
    *   `GET /api/v1/users` (Proxied via Control Plane to IAM).
*   **Validation:**
    *   In `APP_ENV=development`, the Gateway-level `RequireMTLS` check is bypassed.
    *   Identity headers (`X-Org-ID`, `X-User-ID`) are correctly injected by `JWTAuth`.

### **Step 4: Connector Registration**
*   **Action:** The Admin registers a third-party integrated app (Connector) via the Dashboard.
*   **APIs Triggered:**
    *   `POST /api/v1/admin/connectors` (Registers app and generates Bearer API Key).
*   **Validation:** The one-time plaintext API key is returned and can be used for subsequent Data Plane calls.

---

## Phase 2: Policy Engine (Real-time Evaluation & Caching)

### **Step 5: Policy Definition**
*   **Action:** The Admin defines an RBAC access control policy.
*   **APIs Triggered:**
    *   `POST /api/v1/policies` (Creates a policy: `type: rbac, allowed_roles: ["admin"]`).
*   **Validation:** Polling `GET /api/v1/policies` confirms the policy is active across the cluster.

### **Step 6: Gateway Policy Enforcement**
*   **Action:** An external Connector attempts to perform an action and is checked via the Gateway.
*   **APIs Triggered:**
    *   `GET /api/v1/threats` (Protected resource).
*   **Validation:**
    *   The Control Plane intercepts the request and calls `POST /policies/evaluate`.
    *   If the user/connector lacks the required role or IP, the Gateway returns `403 Forbidden` (`POLICY_DENIED`).
    *   **Automation:** Verified via `services/policy/integration_test.go`.

### **Step 7: Cache Hit & Invalidation**
*   **Action:** Verify sub-millisecond performance for repeated evaluations.
*   **Validation:**
    *   Evaluation #1: Cache Miss (Hits DB).
    *   Evaluation #2: Cache Hit (sub-1ms response via Redis).
    *   **Invalidation:** Updating/Deleting the policy triggers a `policy.changes` event via Kafka, which clears the Redis cache for that Organization.

---

## Phase 3: Event Bus & Audit Log (Reliability & Integrity)

### **Step 8: Transactional Outbox & DLQ**
*   **Action:** A high-volume event stream is published.
*   **Validation:**
    *   Records are safely buffered in the Postgres outbox first.
    *   **Retries:** If Kafka is unavailable, the relay retries 5 times.
    *   **DLQ:** After 5 failures, records are marked `dead` and moved to the `outbox.dlq` topic.
    *   **Automation:** Verified via `shared/outbox/outbox_test.go`.

### **Step 9: Audit Log Integrity Verification**
*   **Action:** The Compliance Admin verifies the tamper-evident audit trail.
*   **APIs Triggered:**
    *   `GET /api/v1/audit/integrity` (System validates the cryptographic chain).
*   **Validation:**
    *   The `HashChain` verifier checks `prev_hash` links between all events.
    *   The system detects: sequence gaps, tampered payloads, and broken links.
    *   **Automation:** Verified via `services/audit/pkg/integrity/verifier_test.go`.

---

## Verification Summary
| Test Type | Scope | Success Criteria |
| :--- | :--- | :--- |
| **Unit** | Core Logic | 100% Pass in Audit Verifier, Policy Evaluator, and Outbox DLQ. |
| **Integration** | Service-to-Service | Successful registration -> policy creation -> enforcement cycle. |
| **E2E (Real)** | UI/API Waterfall | Full login/auth flow hit against the real backend stack on port 8080. |

