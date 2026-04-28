# Task Playbooks (AI Recipes)

This document provides step-by-step "recipes" for extending Open-Guard. Follow these strictly to maintain architectural consistency.

---

## 1. Adding a New Security Detector
Used when introducing a new CEL-based threat detection rule.

1.  **Shared Events:** Define a new alert type/struct in `shared/pkg/events/threat.go`.
2.  **Engine Logic:** Implement the detector in `services/threat/pkg/detector/`.
    - Use the `Detector` interface.
    - Register it in `detector/factory.go`.
3.  **Kafka Configuration:** Add the new topic (if needed) in `infra/docker/docker-compose.yml` under the `kafka-init` container.
4.  **UI Visualization:** 
    - Update `web/src/app/core/models/threat.model.ts`.
    - Add a specialized rendering block in `web/src/app/threats/threats.html` using the `@switch` block.

---

## 2. Adding a New Microservice
Used when introducing a new domain service (e.g., "Reporting" or "SIEM Integrator").

1.  **Scaffold Service:** Create `services/<name>/` with a `pkg/` and `main.go`.
2.  **Go Workspace:** Add `./services/<name>` to the `use` block in the root `go.work`.
3.  **Shared Utilities:** Import `shared/crypto`, `shared/resilience`, and `shared/middleware` in your router.
4.  **Infrastructure:** 
    - Create a `Dockerfile` in the service root.
    - Add the service to `infra/docker/docker-compose.yml` with proper healthchecks (HTTPS/8080).
    - Add a `make build` and `make test` entry in the root `Makefile`.
5.  **mTLS:** Ensure the service mounts `/certs` and calls `shared/crypto.LoadClientCerts`.

---

## 3. Extending the Admin Dashboard
Used when adding a new page or widget.

1.  **Convention:** Use **Angular Signals** for all state. No `BehaviorSubject`.
2.  **API Service:** Create a new service in `web/src/app/core/services/` (e.g., `audit.service.ts`).
3.  **Signal Integration:** Expose data as a `signal` or `computed` property.
4.  **Component Syntax:**
    - Use `@if`, `@for`, and `@switch` (Legacy `*ngIf` is forbidden).
    - Use `viewChild` and `viewChildren` for DOM access.
5.  **Routing:** Register the new route in `web/src/app/app.routes.ts`.
6.  **Navigation:** Add the menu item to `navItems` in `web/src/app/core/layout/layout.ts`.

---

## 4. Database Migration
Used when modifying Postgres schemas.

1.  **SQL Script:** Create a new `.sql` file in `infra/docker/postgres-init/`.
2.  **RLS Policy:** Every new table MUST have:
    - `ALTER TABLE <name> ENABLE ROW LEVEL SECURITY;`
    - `CREATE POLICY org_isolation ON <name> ...`
3.  **Go Integration:** Update the repository layer to use `db.WithOrgID(ctx, orgID)` from `shared/db`.
