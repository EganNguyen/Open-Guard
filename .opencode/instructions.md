# OpenGuard AI Session Context

## Critical Non-Negotiable Rules (BE)
- **Transactional Outbox:** Every Kafka publish MUST go through the Outbox relay (`shared/kafka/outbox`). NEVER call Kafka producer directly in service code.
- **RLS Multi-Tenancy:** Every org-scoped table MUST have RLS enabled. Use `rls.OrgPool` or `rls.WithOrgID` to ensure `app.org_id` is set before ANY query.
- **RLS Safety:** `pgxpool.AfterRelease` must reset `app.org_id` to empty string. Never remove this safety hook.
- **Fail Closed:** Security decisions (Policy/Auth/DLP) MUST fail closed. If the service or Redis is unreachable, DENY access.
- **Bcrypt Pool:** All bcrypt operations must run in the `AuthWorkerPool` to prevent CPU exhaustion. Return `429` on pool saturation.
- **Context Handling:** `ctx context.Context` is ALWAYS the first parameter. NEVER use `context.TODO()`.

## Critical Non-Negotiable Rules (FE)
- **Angular 19 Standards:** Use Standalone Components and Signals-first state management. Avoid `BehaviorSubject` for component state.
- **Strict Typing:** No `any`. Every API response and component prop must be fully typed.
- **Secure Identity:** `org_id` must NEVER be passed as a query parameter (e.g., in SSE). Derive it from the JWT on the server side.
- **Destructive Actions:** Always use `UiService.confirm()` before executing deletes or suspensions.

## Before Starting Any Task
1. Read `docs/index/ARCHITECTURE.md` to understand the event-driven flow.
2. Read `docs/index/HOTSPOTS.md` to avoid common pitfalls (timing oracles, RLS leaks).
3. Check the relevant phase YAML in `.opencode/`.
4. Run `make build` and `make lint` before submitting any change.

## Verification
- Run `make test` for unit tests with race detection.
- Run `make test-acceptance` for the end-to-end 45-step scenario. This is the gold standard for PR approval.
