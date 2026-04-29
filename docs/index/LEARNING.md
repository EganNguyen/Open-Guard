# Learning Ledger (Experience Database)

## 2026-04-29: Security Hardening & Alignment

### The SSE Identity Leak
- **Discovery:** Real-time audit streams were passing `org_id` in the URL query string.
- **Learning:** Even in internal/admin dashboards, tenant identifiers must be derived from the authentication context (JWT/Cookie), not client-supplied parameters. This prevents spoofing and parameter-tampering attacks.
- **Fix:** Refactor `sse.service.ts` to use a clean URL and updated the Audit handler to pull `org_id` from the validated context.

### The Timing side-channel
- **Discovery:** Login was short-circuiting before the heavy bcrypt work if a user didn't exist.
- **Learning:** "Fail-fast" is an anti-pattern in authentication. Security handlers must exhibit uniform behavior (timing and error codes) across both valid and invalid identifiers to prevent enumeration.
- **Fix:** Implement a "Dummy Hash" strategy in the Auth service to normalize the CPU load across all login attempts.

### Spec Alignment as a CI Step
- **Discovery:** Significant "Truth Gaps" between the `ai-spec` and the production implementation (e.g., `pg_notify` hybrid pattern).
- **Learning:** Documentation for AI agents is not just documentation—it's part of the system's "Programmatic Context." When it drifts, agents become hallucination-prone.
- **Fix:** Created `scripts/check-spec.sh` to treat architectural patterns as testable assertions.
