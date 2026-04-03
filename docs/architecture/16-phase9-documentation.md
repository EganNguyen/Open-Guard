# §17 — Phase 9: Documentation & Runbooks

---

## 17.1 Required Documents

**`README.md`** must contain:
- One-sentence description.
- Architecture diagram (Mermaid) showing control plane model: connected apps calling OpenGuard (not traffic flowing through it).
- SLO table from §1.2.
- Quick start: `git clone`, `cp .env.example .env`, `make dev` — working in < 5 minutes.
- License and contributing links.

**`docs/architecture.md`** must contain:
- C4 Level 2 component diagram (Mermaid).
- Connector registration and API key authentication flow (including Redis cache path).
- Event ingest flow (internal outbox path and connected app push path).
- Transactional Outbox flow.
- Outbound webhook delivery flow.
- RLS enforcement flow (including OrgPool wrapper).
- Circuit breaker state machine.
- SDK cache layering (local LRU → Redis → DB).
- Saga choreography (user provisioning + compensation).
- MongoDB hash chain integrity model.
- Database ER diagram for each service (Mermaid erDiagram).

**`docs/contributing.md`** must contain:
- Local dev setup.
- Adding a new Kafka consumer (manual commit requirements).
- Adding a new threat detector (template).
- Adding a new compliance report type.
- Adding a new RLS-protected table (checklist).
- Adding a new control plane route (scope, middleware chain, circuit breaker, OpenAPI update).
- PR requirements: tests, lint, contract test if schema changes.

**OpenAPI specs** (`docs/api/<service>.openapi.json`) for all services, valid OpenAPI 3.1, passing `redocly lint`.

---

## 17.2 Operational Runbooks

| File | Scenario |
|---|---|
| `kafka-consumer-lag.md` | Consumer lag > 50k. Check bulk writer, scale consumers, check MongoDB write saturation. |
| `circuit-breaker-open.md` | Breaker fired. Identify failing service, check health endpoints, manual reset procedure. |
| `audit-hash-mismatch.md` | Integrity check fails. Identify affected org, time range, gap analysis, escalation. |
| `secret-rotation.md` | Full rotation for: JWT keys, MFA keys, connector API keys, webhook secrets, Kafka SASL, mTLS certs. |
| `outbox-dlq.md` | Messages in `outbox.dlq`. Inspect, replay, investigate root cause. |
| `postgres-rls-bypass.md` | Cross-tenant data returned. Incident response. Verify RLS policies. |
| `load-shedding.md` | Extreme load. Increase rate limits, scale services, shed non-critical consumers. |
| `connector-suspension.md` | Suspend misbehaving connector. `PATCH /v1/admin/connectors/:id`, verify 401, investigate event log. |
| `webhook-delivery-failure.md` | Connector not receiving webhooks. Check delivery log, DLQ, verify URL reachable. |
| `ca-rotation.md` | Rotate the mTLS CA. Dual-CA trust period. Rehearse in staging first. |

---

## 17.3 Phase 9 Acceptance Criteria

- [ ] `make dev` works on a clean machine following only `README.md`.
- [ ] All OpenAPI specs pass `redocly lint`.
- [ ] Architecture Mermaid diagrams render in GitHub Markdown.
- [ ] All 10 runbooks present.
- [ ] Following `docs/contributing.md`: adding a new detector produces a passing test.
- [ ] Following `docs/contributing.md`: adding a new control plane route produces correct scope enforcement.
