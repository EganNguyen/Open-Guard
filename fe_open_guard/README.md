# OpenGuard — Frontend Specification

> **Scope:** Next.js 14 Admin Dashboard (`web/`). This spec is a peer to the Backend spec and must be read alongside it. All API contracts, error codes, pagination formats, and data models come from `shared/contracts` in the BE spec.
>
> **Audience:** Frontend engineers, design reviewers, QA.
>
> **How to use:** Read files 00–03 (foundation) before any feature work. Feature files (04–12) are self-contained but cross-reference each other.

---

## Mandatory Rules (enforced by CI and code review)

- All API calls go through the typed API client layer (`lib/api/`). No raw `fetch` in components.
- All sensitive values (tokens, keys) are stored in `httpOnly` cookies or server-side session only. Never in `localStorage` or client-accessible cookies.
- Every page that shows org-scoped data must use the `withOrgContext` HOC / layout wrapper. No ad-hoc org ID reads from URL params.
- `org_id` is never interpolated into URLs directly from user input — always sourced from the authenticated session.
- All forms use React Hook Form + Zod. No uncontrolled inputs. No manual `e.target.value` parsing.
- Real-time data (audit stream, threat alerts, Kafka metrics) uses Server-Sent Events via the `/api/stream/*` route handlers. No raw WebSocket connections from client code.
- Every destructive action (suspend, delete, revoke) requires a confirmation modal with the resource name typed or a two-step confirm. No single-click destructive actions.
- Error boundaries wrap every page. Unhandled async errors must surface a recoverable UI, not a blank screen.
- All table columns that display sensitive data (`email`, `ip_address`, `token_prefix`) must pass through the `<Redactable>` component, which respects the org's data visibility settings.
- `Content-Security-Policy` headers are set server-side in `next.config.js`. No inline scripts or inline styles outside of CSS Modules / Tailwind.

---

## Document Index

| File | Contents |
|------|----------|
| [00-tech-stack-and-conventions.md](00-tech-stack-and-conventions.md) | Tech stack, project structure, naming conventions, component rules, state management philosophy |
| [01-design-system.md](01-design-system.md) | Design tokens, color palette, typography, spacing scale, component library, motion system, dark/light mode |
| [02-api-client-layer.md](02-api-client-layer.md) | Typed API client, auth interceptors, error handling, pagination helpers, SSE client, optimistic updates |
| [03-auth-and-session.md](03-auth-and-session.md) | NextAuth.js setup, OIDC flow, TOTP/WebAuthn MFA screens, session refresh, SCIM provisioning state, JWT revocation awareness |
| [04-dashboard-and-layout.md](04-dashboard-and-layout.md) | App shell, sidebar navigation, org switcher, breadcrumbs, notification bell, global search, responsive layout |
| [05-connectors.md](05-connectors.md) | Connector list, registration wizard, API key reveal, scope selector, webhook delivery log, suspension flow |
| [06-policy-engine-ui.md](06-policy-engine-ui.md) | Policy list, policy editor (RBAC rule builder), evaluate playground, eval log table, cache hit visualization |
| [07-audit-log.md](07-audit-log.md) | Real-time event stream, filter panel, cursor pagination, event detail drawer, export jobs, hash chain integrity badge |
| [08-threat-and-alerting.md](08-threat-and-alerting.md) | Alert list, detector cards, alert detail + saga timeline, acknowledge/resolve workflow, SIEM webhook config, MTTR stats |
| [09-compliance-reports.md](09-compliance-reports.md) | Report generation wizard, job status polling, PDF preview, download + signature verification, posture dashboard |
| [10-dlp.md](10-dlp.md) | DLP policy editor, findings table, masking status, monitor vs block mode toggle, entropy scanner config |
| [11-user-and-org-management.md](11-user-and-org-management.md) | User list, user detail, MFA status, session list, SCIM provisioning saga state, org settings, lockout management |
| [12-observability-and-admin.md](12-observability-and-admin.md) | System health page, outbox lag gauge, circuit breaker status, Kafka consumer lag charts, DLQ inspector |
| [13-testing-and-quality.md](13-testing-and-quality.md) | Testing strategy, component tests (Vitest + Testing Library), E2E (Playwright), accessibility, performance budgets, CI integration |
| [14-environment-and-config.md](14-environment-and-config.md) | All env vars (public + server-only), `next.config.js` (CSP, rewrites, headers), Tailwind config, tsconfig |
| [15-validators-and-types.md](15-validators-and-types.md) | TypeScript domain types (mirrors BE shared/models), API response types, Zod form validators, SSE event types, type guards |
| [16-state-management.md](16-state-management.md) | Zustand UI store, notification/toast store, `useConfirm` hook, TanStack Query client setup, URL filter state with nuqs |
| [17-route-handlers-and-middleware.md](17-route-handlers-and-middleware.md) | Next.js middleware (auth + MFA guard + CSP nonce), SSE proxy route handlers, aggregation handlers, report download redirect, SCIM token rotation, MFA Server Action |
| [18-component-patterns.md](18-component-patterns.md) | Canonical patterns: offset-paginated table, cursor-paginated table, SSE real-time table, optimistic status toggle, job status polling, ETag-aware mutations, API key one-time reveal |
| [19-acceptance-criteria.md](19-acceptance-criteria.md) | Full-system E2E acceptance checklist (auth, connectors, policies, audit, threats, compliance, DLP, users, admin, JWT rotation, performance) — mirrors BE spec §20 |
| [20-appendix-trade-offs.md](20-appendix-trade-offs.md) | Decision log for all major frontend choices; out-of-scope features for v1; known limitations |
