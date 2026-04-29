# §00 — Tech Stack & Conventions

UI/UX design was inspired by **Atlassian Guard**: 

---

## 0.1 Core Stack

| Concern | Choice | Version | Notes |
|---|---|---|---|
| Framework | Angular (Standalone) | 19.x | Standalone Components; Signals-first |
| Language | TypeScript | 5.x | `strict: true`; no `any` |
| Styling | Tailwind CSS | 4.x | Utility-first styling |
| Component library | Custom (Angular CDK) | latest | No pre-styled component libraries |
| Forms | Angular Reactive Forms | — | All forms; no exceptions |
| State | Angular Signals | — | Primary state management mechanism |
| Auth | Custom OIDC Service | — | OIDC provider → IAM service |
| Real-time | native `EventSource` (SSE) wrapped in a service | — | No socket.io |
| Charts | Recharts (or Angular equivalent) | — | Wrapped in typed chart components |
| Tables | Angular Material Table / Custom | — | All data tables |
| Testing | Jasmine + Karma / Jest | latest | See §13 |
| Linting | ESLint (Angular config) + Prettier | — | CI-enforced |
| Icons | Lucide Angular | latest | Only icon library permitted |
| Animation | Angular Animations | — | Page transitions and UI animations |

---

## 0.2 Project Structure

```
web/
├── src/
│   ├── app/                        # Angular Application Core
│   │   ├── home/                   # Home feature
│   │   │   ├── home.ts
│   │   │   ├── home.html
│   │   │   └── home.css
│   │   ├── connectors/             # Connectors feature
│   │   │   ├── connectors.ts
│   │   │   ├── connectors.html
│   │   │   └── connectors.css
│   │   ├── app.ts                 # Root component
│   │   ├── app.html
│   │   ├── app.config.ts
│   │   └── app.routes.ts          # Routing configuration
│   ├── assets/                     # Static assets
│   ├── index.html
│   ├── main.ts
│   └── styles.css                  # Global styles (Tailwind)
├── angular.json
├── tailwind.config.js
├── tsconfig.json
└── package.json
```

---

## 0.3 Naming Conventions

### Files and directories

- **Components:** `kebab-case.ts` (Logic) / `kebab-case.html` (Template) / `kebab-case.css` (Style).
- **Services:** `name.service.ts`.
- **Guards:** `name.guard.ts`.
- **Interceptors:** `name.interceptor.ts`.
- **Utilities:** `kebab-case.ts`.
- **Types:** `PascalCase` interfaces and types.

### Component naming

Angular components use `@Component` decorator with a selector and a class name.

```typescript
// ✅ — PascalCase class name with Component suffix
@Component({ selector: 'og-connector-card', ... })
export class ConnectorCardComponent { ... }

// ✅ — Kebab-case selectors with 'og-' prefix
// og-policy-list, og-audit-detail
```

---

## 0.4 Component Rules

### Signals-First State
Angular 19+ uses Signals for change detection. Prefer `signal`, `computed`, and `effect` over `BehaviorSubject` where possible.

```typescript
export class ConnectorListComponent {
  connectors = signal<Connector[]>([]);
  activeCount = computed(() => this.connectors().filter(c => c.status === 'active').length);
}
```

### Typed Forms
Always use Angular Reactive Forms with strong typing.

### No implicit 'any'
Strict mode is enabled. All component inputs, outputs, and internal state must have explicit types.

---

## 0.5 State Management Philosophy

| Data type | Where it lives | Tool |
|---|---|---|
| Server data (lists, details) | Angular Services + Signals | `HttpClient` + `signal` |
| Form state | Reactive Forms | `FormGroup`, `FormControl` |
| Global UI state (sidebar, modals) | Singleton Services | `UiService` |
| Notifications / toasts | Singleton Services | `NotificationService` |
| Auth session | Auth Service | `AuthService` (OIDC) |
| Org context | Auth Service | `AuthService.currentOrg()` |
| URL state | Angular Router | `ActivatedRoute` queryParams |

**Rule:** Services are the single source of truth for data. Components should only hold UI-specific state.

---

## 0.6 Error Handling


Every async operation uses a consistent pattern:

```tsx
// In mutations (TanStack Query)
const suspendConnector = useMutation({
  mutationFn: (id: string) => api.connectors.suspend(id),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.connectors.all(orgId) })
    toast.success('Connector suspended')
  },
  onError: (error: APIError) => {
    toast.error(error.message ?? 'Failed to suspend connector')
  },
})
```

**Error codes from BE (`shared/models/errors.go`) map to UI messages:**

```ts
// lib/utils/error-messages.ts
export const ERROR_MESSAGES: Record<string, string> = {
  RESOURCE_NOT_FOUND: 'This resource no longer exists.',
  RESOURCE_CONFLICT: 'A resource with these details already exists.',
  FORBIDDEN: 'You do not have permission to perform this action.',
  UPSTREAM_UNAVAILABLE: 'A dependent service is temporarily unavailable. Please try again shortly.',
  CAPACITY_EXCEEDED: 'The system is under high load. Please retry in a moment.',
  VALIDATION_ERROR: 'Please check the form for errors.',
  CONNECTOR_SUSPENDED: 'This connector is suspended.',
  INSUFFICIENT_SCOPE: 'This connector lacks the required permissions.',
  DLP_POLICY_VIOLATION: 'Event blocked: DLP policy violation detected.',
  SESSION_REVOKED_RISK: 'Your session was revoked due to suspicious activity. Please log in again.',
  SESSION_COMPROMISED: 'Session compromised. Please log in again.',
  TOTP_REPLAY_DETECTED: 'This MFA code has already been used. Please wait for the next code.',
}
```

---

## 0.7 Forbidden Patterns

| Pattern | Why forbidden | Alternative |
|---|---|---|
| `localStorage` for sensitive tokens | XSS-accessible; security boundary | `httpOnly` cookies |
| Raw `fetch` in components | No auth injection, no error normalization | Angular `HttpClient` |
| `any` type | Defeats TypeScript | Define proper typed interfaces |
| Inline `style={{}}` | Bypasses CSP, hard to maintain | Tailwind classes |
| `BehaviorSubject` for simple state | Signals are more efficient in Angular 19 | Angular `signal()` |
| Single-click destructive actions | Too easy to trigger accidentally | `UiService.confirm()` |
| Hard-coded org_id strings | Breaks multi-tenancy | `AuthService` current org signal |
| `console.log` in production | Leaks sensitive data | Remove before commit |
