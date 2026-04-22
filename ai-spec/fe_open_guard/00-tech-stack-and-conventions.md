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

- **Page components:** `page.tsx` (Next.js convention).
- **Server Components:** Default. No `"use client"` unless the component uses hooks, browser APIs, or event handlers.
- **Client Components:** Named `*.client.tsx` when colocated with a server component of the same name, e.g., `audit-stream.client.tsx`.
- **Hooks:** `use-kebab-case.ts`.
- **Utilities:** `kebab-case.ts`.
- **Types:** `PascalCase` interfaces and types.

### Component naming

```tsx
// ✅ — PascalCase, descriptive, no "Component" suffix
export function ConnectorCard({ connector }: ConnectorCardProps) { ... }

// ❌ — redundant suffix
export function ConnectorCardComponent() { ... }

// ❌ — too generic
export function Card() { ... }  // use ui/card.tsx for the primitive
```

### Query key factories

All query keys are defined in `lib/query/keys.ts`. No inline string arrays.

```ts
// lib/query/keys.ts
export const queryKeys = {
  connectors: {
    all: (orgId: string) => ['connectors', orgId] as const,
    detail: (orgId: string, id: string) => ['connectors', orgId, id] as const,
    deliveries: (orgId: string, id: string) => ['connectors', orgId, id, 'deliveries'] as const,
  },
  policies: {
    all: (orgId: string) => ['policies', orgId] as const,
    detail: (orgId: string, id: string) => ['policies', orgId, id] as const,
    evalLogs: (orgId: string) => ['policies', orgId, 'eval-logs'] as const,
  },
  audit: {
    events: (orgId: string, filters: AuditFilters) => ['audit', orgId, 'events', filters] as const,
    integrity: (orgId: string) => ['audit', orgId, 'integrity'] as const,
  },
  threats: {
    alerts: (orgId: string, filters: AlertFilters) => ['threats', orgId, 'alerts', filters] as const,
    detail: (orgId: string, id: string) => ['threats', orgId, id] as const,
  },
  users: {
    all: (orgId: string) => ['users', orgId] as const,
    detail: (orgId: string, id: string) => ['users', orgId, id] as const,
    sessions: (orgId: string, userId: string) => ['users', orgId, userId, 'sessions'] as const,
  },
  // ... etc
}
```

---

## 0.4 Component Rules

### Server Components by default

```tsx
// ✅ — Server Component (no directive needed)
// app/(dashboard)/connectors/page.tsx
import { getConnectors } from '@/lib/api/connectors'

export default async function ConnectorsPage() {
  const connectors = await getConnectors()  // direct server-side fetch
  return <ConnectorList initialData={connectors} />
}
```

### When to use `"use client"`

- Component uses React state (`useState`, `useReducer`).
- Component uses browser APIs (`window`, `document`, `EventSource`).
- Component uses event handlers directly (`onClick`, `onChange`).
- Component uses animation libraries (Framer Motion).
- Component uses TanStack Query hooks (client-side refetch).

### Props typing

Every component has an explicit Props interface. No `React.FC<{}>`. No implicit `children: any`.

```tsx
interface ConnectorCardProps {
  connector: Connector
  onSuspend: (id: string) => Promise<void>
  className?: string
}

export function ConnectorCard({ connector, onSuspend, className }: ConnectorCardProps) { ... }
```

### No prop drilling beyond two levels

If a prop would be passed through more than two layers, use Zustand or React Context (scoped to the feature subtree).

---

## 0.5 State Management Philosophy

| Data type | Where it lives | Tool |
|---|---|---|
| Server data (lists, details) | TanStack Query cache | `useQuery` / `useMutation` |
| Form state | React Hook Form | `useForm` |
| Global UI state (sidebar, modals) | Zustand `ui` store | `useUIStore` |
| Notifications / toasts | Zustand `notification` store | `useNotificationStore` |
| Auth session | NextAuth.js session | `useSession` / `auth()` |
| Org context | NextAuth session + `useOrg` hook | Derived from session |
| URL state (filters, pagination cursors) | `useSearchParams` + `nuqs` | Synced to URL |
| Real-time stream data | Local `useState` inside SSE hook | `useAuditStream` |

**Rule:** TanStack Query is the single source of truth for all server data. Never duplicate server data into Zustand. Zustand is for UI-only state that has no server representation.

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
| `localStorage` for tokens or org_id | XSS-accessible; security boundary | `httpOnly` cookies via NextAuth |
| Raw `fetch` in components | No auth injection, no error normalization | `lib/api/*` client functions |
| `any` type | Defeats TypeScript | Define proper types in `types/` |
| Inline `style={{}}` for visual styling | Bypasses CSP, hard to maintain | Tailwind classes or CSS Modules |
| `useEffect` for data fetching | Race conditions, no caching | TanStack Query `useQuery` |
| Single-click destructive actions | Too easy to trigger accidentally | `ConfirmDialog` with resource name |
| Hard-coded org_id strings | Breaks multi-tenancy | `useOrg()` hook |
| `console.log` left in committed code | Leaks sensitive data to browser console | Remove before commit; use structured logging patterns |
| Cursor pagination with manual offset arithmetic | Error-prone, breaks on delete | Use `next_cursor` from API response meta |
| Polling with `setInterval` | Not cleanup-safe | `useQuery` with `refetchInterval` |
