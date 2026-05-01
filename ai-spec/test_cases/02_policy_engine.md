# Policy Engine Flows

## 1. Overview
This module covers the management and real-time evaluation of Attribute-Based Access Control (ABAC) policies using CEL (Common Expression Language).

## 2. Policy Management

### TC-POL-001: Create Policy with Valid CEL Expression
- **User Flow:** Admin creates a new ABAC policy.
- **Steps:** `POST /v1/policies` with logic like `subject.role == 'viewer'`.
- **Expected Results:** 201 Created with ETag.
- **System Verifications:** Outbox record created; Kafka `policy.changes` notification.

### TC-POL-002: Update Policy Increments Version
- **User Flow:** Admin updates policy logic.
- **System Verifications:** `version` incremented; ETag updated; Redis CEL cache invalidated.

### TC-POL-003: Delete Policy Cascades to Assignments
- **User Flow:** Admin deletes policy.
- **System Verifications:** `ON DELETE CASCADE` removes all subject assignments.

## 3. Policy Evaluation

### TC-EVAL-001: Policy Evaluation – Allow Decision (Cache Miss)
- **User Flow:** Connector evaluates access; result is computed and cached.
- **Steps:** `POST /v1/policy/evaluate`.
- **Expected Results:** `effect: "allow"`, `cache_hit: false`.
- **System Verifications:** DB query + CEL compile -> Redis L2 cache write (60s TTL).

### TC-EVAL-002: Policy Evaluation – Deny Decision and Audit Log
- **User Flow:** User attempts forbidden action.
- **Expected Results:** `effect: "deny"`; event logged to `policy_eval_log`.

### TC-EVAL-003: Evaluation with Stale Cache After Policy Update
- **User Flow:** Admin updates policy; next evaluation must reflect change immediately.
- **System Verifications:** `InvalidateOrgCache` triggers on `policy.updated` event.
