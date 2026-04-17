# OpenGuard — Frontend Specification

> **Scope:** Angular 19 Admin Dashboard. This spec is a peer to the Backend spec and must be read alongside it. All API contracts, error codes, pagination formats, and data models come from binary-compatible shared contracts.
>
> **Audience:** Frontend engineers, design reviewers, QA.
>
> **How to use:** Read files 00–03 (foundation) before any feature work. Feature files (04–12) are self-contained but cross-reference each other.

---

## Mandatory Rules (enforced by CI and code review)

- All API calls go through the typed API client layer (`src/app/core/api/`). No raw `fetch` in components.
- All sensitive values (tokens, keys) are stored in secure session storage or `httpOnly` cookies from the backend.
- Every page that shows org-scoped data must be protected by the `authGuard`. No ad-hoc org ID reads from URL params.
- `org_id` is derived from the authenticated session (JWT claims).
- All forms use **Angular Reactive Forms** + validation. No manual `e.target.value` parsing.
- Real-time data (audit stream, threat alerts) uses **Server-Sent Events** via a dedicated `SseService`.
- Every destructive action (suspend, delete, revoke) requires a confirmation modal with the resource name typed (Pessimistic UI).
- Global Error Handling via `ErrorHandler` provider ensures no blank screens on unhandled errors.
- Sensitive data display (`email`, `ip_address`) pass through a redactable pipe or component.
- Security headers are set via web server configuration or SSR `server.ts`.

---

## Document Index

| File | Contents |
|------|----------|
| [00-tech-stack-and-conventions.md](00-tech-stack-and-conventions.md) | Tech stack, project structure, naming conventions, component rules, state management philosophy |
| [01-design-system.md](01-design-system.md) | Design tokens, color palette, typography, spacing scale, component library, motion system, dark/light mode |
| [02-api-client-layer.md](02-api-client-layer.md) | Typed API client, auth interceptors, error handling, pagination helpers, SSE client, optimistic updates |
| [03-auth-and-session.md](03-auth-and-session.md) | OIDC custom Service, TOTP/WebAuthn MFA screens, session signals, SCIM provisioning state, JWT revocation awareness |
| [04-dashboard-and-layout.md](04-dashboard-and-layout.md) | App shell, sidebar, org switcher, breadcrumbs, notification service, global search component |
| [05-connectors.md](05-connectors.md) | Connector list, registration wizard, API key one-time reveal, scope selector |
| [06-policy-engine-ui.md](06-policy-engine-ui.md) | Policy list, editor (CDK drag-drop rule builder), evaluate playground, cache hit viz |
| [07-audit-log.md](07-audit-log.md) | Real-time event stream, filter panel, cursor pagination, detail drawer, integrity badge |
| [08-threat-and-alerting.md](08-threat-and-alerting.md) | Alert list, detectors, alert detail + saga timeline, workflow, SIEM webhook config |
| [09-compliance-reports.md](09-compliance-reports.md) | Report wizard, status polling, PDF preview, download, posture dashboard |
| [10-dlp.md](10-dlp.md) | DLP editor, findings table, masking status, monitor/block mode toggle |
| [11-user-and-org-management.md](11-user-and-org-management.md) | User list, detail, MFA status, sessions, SCIM saga, org settings, lockout |
| [12-observability-and-admin.md](12-observability-and-admin.md) | System health, outbox lag, circuit breaker, consumer lag charts, DLQ inspector |
| [13-testing-and-quality.md](13-testing-and-quality.md) | Testing strategy, Karma/Jasmine + Angular Testing Library, E2E (Playwright), accessibility, performance |
| [14-environment-and-config.md](14-environment-and-config.md) | Env tokens, `angular.json`, `server.ts`, proxy, Tailwind, tsconfig |
| [15-validators-and-types.md](15-validators-and-types.md) | TypeScript domain types, API response types, SSE event types, type guards |
| [16-state-management.md](16-state-management.md) | Signal-based Services (Stores), notification service, `confirm` service, Router filter state |
| [17-route-handlers-and-middleware.md](17-route-handlers-and-middleware.md) | Angular Guards (Auth, MFA), HttpInterceptors, client-side aggregation |
| [18-component-patterns.md](18-component-patterns.md) | Canonical patterns: paginated tables, SSE real-time tables, status toggles, job status polling |
| [19-acceptance-criteria.md](19-acceptance-criteria.md) | Full-system E2E acceptance checklist (auth, connectors, policies, audit, threats, compliance, DLP, users, admin, JWT rotation, performance) — mirrors BE spec §20 |
| [20-appendix-trade-offs.md](20-appendix-trade-offs.md) | Decision log for all major frontend choices; out-of-scope features for v1; known limitations |
