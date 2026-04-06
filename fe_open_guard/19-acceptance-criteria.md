# §19 — Full-System Acceptance Criteria

The frontend acceptance criteria mirror the 45-step BE scenario (BE spec §20) from the user's perspective. Every step must be verified as a Playwright E2E test before a release is published.

---

## 19.1 Auth & Session Flows

- [ ] **Login:** Navigating to any dashboard route while unauthenticated redirects to `/login`. Clicking "Sign in" initiates the OIDC redirect. After successful auth, user lands on `/overview`.
- [ ] **TOTP MFA:** After login, user with `mfa_required: true` is redirected to `/mfa/totp`. Entering a valid 6-digit code marks MFA as verified and redirects to `/overview`.
- [ ] **TOTP replay:** Entering the same valid TOTP code a second time within 90s displays: "This code was already used. Please wait for the next one." (`TOTP_REPLAY_DETECTED`)
- [ ] **WebAuthn MFA:** After login, user with `mfa_method: webauthn` is redirected to `/mfa/webauthn`. Mock credential resolves successfully and redirects to `/overview`.
- [ ] **Session expiry:** When the access token expires and refresh fails, the next API call triggers `signOut` and redirects to `/login?reason=session_expired`. A banner displays: "Your session has expired."
- [ ] **Session revocation:** Admin revokes a user's sessions via `/users/[id]`. On the user's next API call, the 401 triggers `signOut` with the `SESSION_REVOKED_RISK` reason banner.
- [ ] **High-risk session refresh:** Token refresh from a new device family (User-Agent change score ≥ 80) returns `SESSION_REVOKED_RISK`. User is signed out with an appropriate banner.
- [ ] **Provisioning states:** A user with `status: initializing` who attempts login sees the provisioning message at login. An admin can see the saga progress timeline at `/users/[id]`.
- [ ] **Provisioning failure + retry:** Admin sees "Retry provisioning" button for users with `provisioning_status: provisioning_failed`. Clicking it triggers `POST /users/:id/reprovision`.
- [ ] **MFA enrollment:** Admin user navigates to `/org/settings` → Security → enrolls TOTP → QR code displayed → 6-digit verification → backup codes revealed → "I've saved these codes" acknowledgment.
- [ ] **Backup code usage:** User enters a backup code during MFA challenge. Toast: "One backup code used. You have N remaining."

---

## 19.2 Connector Lifecycle

- [ ] **Connector list:** `/connectors` loads with offset pagination. All columns render correctly. Status badges are color-coded.
- [ ] **Register connector:** Completing the 3-step registration wizard (name + webhook + scopes → review) calls `POST /v1/admin/connectors`. On success, the API key reveal screen shows with the masked key, reveal toggle, copy button, and acknowledgment button. `beforeunload` warning fires if user navigates away without acknowledging.
- [ ] **Idempotent creation:** Submitting the registration form twice (double-click) does not create two connectors — the idempotency key prevents the duplicate.
- [ ] **Suspend with confirmation:** Clicking "Suspend" opens `ConfirmDialog` requiring the connector name to be typed. Submitting calls `PATCH /v1/admin/connectors/:id {status:"suspended"}`. The optimistic update immediately shows the badge as SUSPENDED before the server responds.
- [ ] **Suspend cache invalidation:** After suspension, the delivery log for that connector shows subsequent events as `403 CONNECTOR_SUSPENDED` within 30 seconds (cache TTL elapsed).
- [ ] **Activate:** Clicking "Activate" on a suspended connector does NOT require typing the name (non-destructive). Status badge immediately flips to ACTIVE.
- [ ] **Scope edit:** Changing scopes via the Settings tab calls `PATCH /v1/admin/connectors/:id`. Toast: "Settings saved. Cache updated."
- [ ] **Webhook test:** "Send test webhook" button calls `POST /v1/admin/connectors/:id/test`. Result appears as a new row at the top of the delivery log.
- [ ] **Delivery log:** Webhook delivery log shows cursor-paginated entries. Failed entries show HTTP status code in red. Dead entries show "Moved to DLQ" with a link to the DLQ inspector.
- [ ] **Scope enforcement UI:** A connector with only `events:write` scope attempting `POST /v1/policy/evaluate` results in `403 INSUFFICIENT_SCOPE`. The delivery log row shows this error code and explains which scope is missing.

---

## 19.3 Policy Engine

- [ ] **Policy list:** `/policies` loads with all org policies. Version column renders as `v{N}` in monospace.
- [ ] **Policy creation:** New policy with at least one ALLOW rule saves successfully. Toast confirms creation. Policy appears in list with `v1`.
- [ ] **Policy evaluation (playground):** Entering a blocked IP in the evaluate playground returns `permitted: false` with the matched policy displayed. Cache layer shows `none` (first evaluation).
- [ ] **Redis cache hit:** Submitting identical inputs a second time shows `cache_hit: redis`.
- [ ] **Policy update ETag:** Editing a policy that was concurrently modified by another session returns a 412 banner: "This policy was modified by someone else. Reload to see the latest version."
- [ ] **Policy version increment:** After a successful `PUT`, the version badge increments from `v1` to `v2`.
- [ ] **Policy circuit breaker banner:** Killing the policy service in the test environment causes a non-dismissible banner in the Policies section: "⚡ Policy service is degraded."
- [ ] **Policy cache invalidation:** After updating a policy, the next evaluate call returns a fresh result (`cache_hit: none`) confirming cache was purged.
- [ ] **Scope guard:** A connector without `policy:evaluate` scope attempting evaluation gets `403 INSUFFICIENT_SCOPE` — visible in the playground as an error state, not a deny decision.

---

## 19.4 Audit Log

- [ ] **Live stream:** `/audit` connects to SSE and new events appear at the top of the table in real time. The "● LIVE" indicator is green and pulsing.
- [ ] **Buffer cap:** With 200+ events accumulated, the table does not grow beyond 200 rows (oldest events are dropped from view).
- [ ] **Pause and filter:** Clicking "Pause" disconnects SSE. Filter panel becomes active. Applying a filter for `type=auth.login.failure` returns matching historical events via cursor pagination.
- [ ] **Cursor pagination:** "Load more" appends older events below the current list. The cursor in the URL updates.
- [ ] **Event detail drawer:** Clicking a row opens the detail drawer without navigating away. All fields render: event ID (copy), type, actor, payload (formatted JSON), chain hash, chain seq.
- [ ] **DLP masking indicator:** An event whose payload was masked by DLP shows the "⚑ Some fields were masked" banner in the detail drawer.
- [ ] **Hash chain integrity badge:** Badge shows "🔒 Chain integrity verified" for a clean chain. Deleting a MongoDB document (test environment) causes the badge to show "⚠ Chain integrity failure" within 5 minutes (next check cycle).
- [ ] **Export creation:** Submitting the export form creates a job. Status polling transitions from "pending" to "processing" to "completed". Download button appears. Clicking it triggers a browser file download.
- [ ] **Audit stats:** Stats widget shows correct event counts (today, week, most common type).

---

## 19.5 Threat Detection & Alerting

- [ ] **Alert list:** 11 simulated failed login events (via connector ingest) result in a HIGH alert appearing in the list within 5 seconds. The sidebar unread count badge increments.
- [ ] **CRITICAL alert pulse:** A CRITICAL-severity alert shows a pulsing red indicator in the list.
- [ ] **Alert detail:** Clicking an alert shows the risk score gauge, saga timeline (all 4 steps with timestamps), and contributing audit events with links.
- [ ] **Acknowledge:** Clicking "Acknowledge" changes the status badge to ACKNOWLEDGED. The alert remains visible in the list (not removed).
- [ ] **Resolve:** Clicking "Resolve" opens the resolution modal. Entering an optional note and confirming changes status to RESOLVED. MTTR is displayed: "Resolved in 14m 32s."
- [ ] **SIEM webhook config:** Saving a SIEM webhook URL in Org Settings triggers a test delivery. The delivery result shows HTTP status and HMAC validation note.
- [ ] **Replay protection UI:** If the SIEM webhook test payload is older than 300s (simulated in tests via a mocked timestamp), the receiver should reject it — confirmed by the delivery log showing HTTP 400.
- [ ] **Privilege escalation detection:** Granting an admin role to a user who logged in within 60 minutes triggers a HIGH alert visible in the list within 5 seconds.

---

## 19.6 Compliance Reports

- [ ] **Report generation:** Completing the GDPR report wizard calls `POST /v1/compliance/reports`. The job card appears in the report list with status "pending".
- [ ] **Status polling:** The status badge transitions from PENDING → PROCESSING → COMPLETED without a page refresh (every 3s poll).
- [ ] **Completion toast:** A toast notification appears: "Report ready" with a "Download" action button.
- [ ] **PDF download:** Clicking Download redirects through `/api/compliance/reports/[id]/download` to the pre-signed S3 URL and triggers a browser download.
- [ ] **Signature verification panel:** The completed report shows "✅ Signature valid" with algorithm and key ID details. The "Download .sig file" button works independently.
- [ ] **Bulkhead 429:** Attempting to generate an 11th concurrent report returns `429 CAPACITY_EXCEEDED`. The UI shows: "Report queue is full. Please try again in a few minutes." (not a spinner or pending state).
- [ ] **Posture dashboard:** The compliance posture page shows control statuses across GDPR, SOC 2, and HIPAA frameworks. Clicking a control expands guidance.

---

## 19.7 DLP

- [ ] **Monitor mode:** Ingesting an event containing a Social Security Number (via connector in monitor mode) is accepted (200). The DLP findings list shows a new `pii/ssn` finding within 5 seconds.
- [ ] **Audit masking:** The audit event corresponding to the SSN ingest shows the masked payload field (`[REDACTED:ssn]`) in the detail drawer within 5 seconds of finding creation.
- [ ] **Block mode toggle:** Enabling block mode on a DLP policy shows the confirmation dialog explaining latency impact. After confirmation, the policy badge shows "BLOCK".
- [ ] **Block mode ingest rejection:** Ingesting an event with a credit card number through a connector where the org has an active block-mode DLP policy returns `422 DLP_POLICY_VIOLATION`. The delivery log shows the rejection.
- [ ] **DLP service down (block mode):** Stopping the DLP service in test environment causes ingested events to be rejected with `503 DLP_UNAVAILABLE` for block-mode orgs. Delivery log reflects this.
- [ ] **Findings table filters:** Filtering by `type=credential` shows only credential findings. Cursor pagination works across pages.
- [ ] **Credential auto-alert:** A `credential` finding automatically creates a HIGH threat alert visible in `/threats` within 5 seconds.

---

## 19.8 User & Org Management

- [ ] **User list:** Users table loads with correct status badges. Locked users show "🔒" indicator.
- [ ] **User detail:** Clicking a user shows profile, MFA status, active sessions, and API tokens.
- [ ] **Session revoke:** Admin clicks "Revoke this session" for a specific session. The session is removed from the list. The revoked user's next API call returns 401 (verified in a separate browser tab test).
- [ ] **Revoke all sessions:** "Revoke all sessions" revokes all JTIs for the user. Toast confirms. Session count drops to 0.
- [ ] **Account unlock:** Locked user shows "Unlock account" button. Clicking it resets `failed_login_count` and `locked_until`. Toast: "Account unlocked."
- [ ] **MFA revoke:** Admin clicks "Revoke MFA" for a TOTP user. User must re-enroll MFA on next login (verified by login flow test with that user).
- [ ] **API token creation:** Creating an API token shows the one-time reveal screen with the same acknowledge flow as connector key reveal.
- [ ] **Org settings save:** Updating `max_sessions` from 5 to 3 saves successfully. Toast: "Settings saved."
- [ ] **SCIM token rotation:** Rotating the SCIM bearer token shows the new token in a one-time reveal. Subsequent SCIM calls with the old token return 401.
- [ ] **Org delete:** Typing the org slug in the confirm dialog and clicking "Delete" triggers the offboarding saga. Org status becomes "offboarding" and the user is signed out.

---

## 19.9 System Health & Admin

- [ ] **System health page:** All service cards show HEALTHY status in a fresh environment. Stopping the Policy service turns its card to DOWN within 30 seconds (next poll cycle).
- [ ] **Circuit breaker panel:** Opening the Policy circuit breaker (`cb-policy`) state shows OPEN after the service is stopped for long enough. The impact description is rendered.
- [ ] **Outbox lag gauge:** Stopping the outbox relay causes the IAM outbox pending count to rise. The gauge turns amber at 100, red at 500.
- [ ] **Kafka consumer lag chart:** Stopping the audit consumer and running a load burst causes the lag chart to spike. Alert link appears at the 50,000 threshold.
- [ ] **DLQ inspector:** A dead outbox record (after 5 failed relay attempts) appears in the `outbox.dlq` tab. "Replay" button is present with confirmation dialog.
- [ ] **Replay DLQ message:** Clicking Replay shows the confirmation ("Consumer must be idempotent") and on confirm, the message is removed from the DLQ table.
- [ ] **Integrity failure report:** Navigating to `/audit/integrity-report` when a chain gap exists shows the gap details, expected vs found seq, and the runbook link.

---

## 19.10 JWT Key Rotation (Frontend Perspective)

- [ ] **Old tokens still verify:** After adding a new JWT key (`active`) and setting the old to `verify_only`, existing sessions continue to work without re-login.
- [ ] **Old tokens rejected after rotation complete:** After removing the old key from the keyring and redeploying IAM, the next API call from an old-token session returns 401. The middleware detects `RefreshAccessTokenError` and redirects to `/login?reason=session_expired`.
- [ ] **New login uses new key:** After rotation, logging in again issues a token with the new `kid` in the JWT header — visible in browser dev tools.

---

## 19.11 Performance & Accessibility Acceptance

- [ ] `npm run build` completes with zero TypeScript errors and zero Next.js warnings.
- [ ] `npx tsc --noEmit` exits 0.
- [ ] `npm run lint` exits 0 (ESLint + Prettier).
- [ ] `npx vitest run --coverage` exits 0 with ≥ 80% coverage per package.
- [ ] `npx playwright test` (all 11 critical path specs) passes in CI.
- [ ] Lighthouse CI: Performance ≥ 85, Accessibility ≥ 95, on `/overview`, `/audit`, `/threats` pages.
- [ ] `axe-playwright` reports 0 WCAG AA violations on all E2E page loads.
- [ ] First load JS per route < 150KB gzipped (verified by `ANALYZE=true npm run build`).
- [ ] All interactive elements are keyboard-navigable (Tab order is logical, Enter/Space activate buttons).
- [ ] All form error messages are associated with their input via `aria-describedby`.
- [ ] `prefers-reduced-motion` disables all Framer Motion animations — verified by media query override in Playwright.
