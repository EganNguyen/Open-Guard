---
name: openguard-nextjs-frontend
description: >
  Use this skill whenever writing, reviewing, or extending any Next.js frontend
  code in the OpenGuard Admin Dashboard (web/). Covers all mandatory patterns:
  App Router RSC-first architecture, typed API client layer, TanStack Query for
  server state, Zustand for UI state, NextAuth.js v5 OIDC + MFA gate, SSE
  real-time streams, React Hook Form + Zod, and security hardening (CSP,
  httpOnly cookies, Redactable component). All rules below are CI-enforced —
  violation = PR blocked.
license: Internal — OpenGuard Engineering
---

# OpenGuard — Next.js Frontend Skill

> Read files 00–03 of the FE spec before any feature work.
> Every pattern here is canonical and CI-enforced.

---

## 0. Absolute Rules (CI-enforced, no exceptions)

```
✗ No raw fetch in components — all API calls through lib/api/* typed client
✗ No tokens or org_id in localStorage — httpOnly cookies via NextAuth only
✗ No org-scoped page without withOrgContext HOC / layout wrapper
✗ No org_id interpolated from URL params — always from authenticated session
✗ No uncontrolled inputs — all forms use React Hook Form + Zod
✗ No raw WebSocket connections from client — SSE via /api/stream/* only
✗ No single-click destructive actions — ConfirmDialog with resource name typed
✗ No page without error boundary — unhandled errors must show recoverable UI
✗ No sensitive data (email, ip_address, token_prefix) outside <Redactable>
✗ No inline scripts or inline styles outside CSS Modules / Tailwind
✗ No any type — defeats TypeScript — CI lint failure
✗ No console.log in committed code — leaks sensitive data to browser DevTools
✗ No useEffect for data fetching — use TanStack Query useQuery
✗ No polling with setInterval — use useQuery with refetchInterval
✗ No hard-coded org_id strings — use useOrg() hook
```

---

## 1. Project Structure

```
web/
├── app/
│   ├── (auth)/                     # No sidebar layout
│   │   ├── login/page.tsx
│   │   ├── mfa/
│   │   │   ├── totp/page.tsx
│   │   │   └── webauthn/page.tsx
│   │   └── layout.tsx              # centered card shell
│   ├── (dashboard)/                # App shell layout — all authenticated routes
│   │   ├── layout.tsx              # AppShell: sidebar + topbar + org context
│   │   ├── connectors/
│   │   ├── policies/
│   │   ├── audit/
│   │   ├── threats/
│   │   ├── compliance/
│   │   ├── dlp/
│   │   ├── users/
│   │   ├── org/settings/
│   │   └── admin/system/
│   ├── api/
│   │   ├── auth/[...nextauth]/route.ts
│   │   └── stream/
│   │       ├── audit/route.ts      # SSE proxy → audit service
│   │       └── threats/route.ts    # SSE proxy → threat service
│   ├── error.tsx                   # Global error boundary
│   └── layout.tsx                  # Root: html, body, providers
├── components/
│   ├── ui/                         # Design system primitives (Button, Input, Badge…)
│   ├── layout/                     # AppShell, Sidebar, Topbar, Breadcrumbs
│   ├── data/                       # DataTable, Pagination, FilterPanel, Redactable
│   ├── feedback/                   # Toast, Alert, ConfirmDialog, LoadingSpinner
│   ├── charts/                     # LineChart, BarChart, GaugeChart (Recharts wrappers)
│   └── domain/                     # Feature components (ConnectorCard, PolicyRuleBuilder…)
├── lib/
│   ├── api/                        # Typed API client
│   │   ├── client.ts               # apiFetch<T>, OpenGuardAPIError
│   │   ├── connectors.ts
│   │   ├── policies.ts
│   │   ├── audit.ts
│   │   ├── threats.ts
│   │   ├── compliance.ts
│   │   ├── dlp.ts
│   │   ├── users.ts
│   │   └── admin.ts
│   ├── hooks/
│   │   ├── use-sse.ts              # SSE EventSource hook
│   │   ├── use-org.ts              # current org from session
│   │   ├── use-confirm.ts          # imperative ConfirmDialog
│   │   └── use-clipboard.ts
│   ├── auth/                       # NextAuth config, session helpers
│   ├── query/                      # TanStack Query client, key factories
│   ├── store/                      # Zustand stores (ui.ts, notification.ts)
│   ├── utils/                      # cn(), formatDate(), truncate(), encodeCursor()
│   └── validators/                 # Zod schemas (mirror BE models)
└── types/
    ├── api.ts                      # API response types
    ├── models.ts                   # Domain model types (mirrors BE shared/models)
    └── events.ts                   # SSE event envelope types
```

---

## 2. Next.js App Router — Core Rules

### 2.1 Server Components by default

```tsx
// ✓ correct — Server Component (no directive)
// app/(dashboard)/connectors/page.tsx
import { getConnectors } from '@/lib/api/connectors'

export default async function ConnectorsPage() {
    const connectors = await getConnectors()   // direct server-side fetch
    return <ConnectorList initialData={connectors} />
}
```

### 2.2 When to add "use client"

Add `"use client"` only when the component:
- Uses React state (`useState`, `useReducer`)
- Uses browser APIs (`window`, `document`, `EventSource`)
- Uses event handlers directly (`onClick`, `onChange`)
- Uses TanStack Query hooks (client-side refetch)
- Uses animation libraries (Framer Motion)

```tsx
// ✓ correct — named *.client.tsx when colocated with server component
// app/(dashboard)/audit/audit-stream.client.tsx
"use client"

export function AuditStream() { ... }
```

### 2.3 Middleware (auth + MFA gate + CSP nonce)

```ts
// middleware.ts
export default async function middleware(req: NextRequest) {
    const session = await auth()

    // 1. Auth gate
    if (!session && !isPublicRoute(req.nextUrl.pathname)) {
        return NextResponse.redirect(new URL('/login', req.url))
    }

    // 2. MFA gate — redirect before ANY dashboard page renders
    if (session?.mfaRequired && !session?.mfaVerified &&
        !req.nextUrl.pathname.startsWith('/mfa')) {
        return NextResponse.redirect(new URL('/mfa/totp', req.url))
    }

    // 3. Inject CSP nonce for legitimate inline scripts
    const nonce = Buffer.from(crypto.randomUUID()).toString('base64')
    const res = NextResponse.next()
    res.headers.set('x-nonce', nonce)
    return res
}

export const config = {
    matcher: ['/((?!_next/static|_next/image|favicon.ico).*)'],
}
```

### 2.4 SSE proxy route handlers

```ts
// app/api/stream/audit/route.ts
// Proxies SSE from backend — client never connects directly to backend
export async function GET(req: Request) {
    const session = await auth()
    if (!session) return new Response('Unauthorized', { status: 401 })

    const upstream = await fetch(`${process.env.INTERNAL_API_URL}/v1/audit/stream`, {
        headers: { Authorization: `Bearer ${session.accessToken}` },
    })

    return new Response(upstream.body, {
        headers: {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            'Connection': 'keep-alive',
        },
    })
}
```

---

## 3. TypeScript — strict: true

```ts
// tsconfig.json
{
  "compilerOptions": {
    "strict": true,          // required — no exceptions
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true
  }
}
```

### 3.1 Props typing — always explicit interface, never React.FC

```tsx
// ✓ correct
interface ConnectorCardProps {
    connector: Connector
    onSuspend: (id: string) => Promise<void>
    className?: string
}

export function ConnectorCard({ connector, onSuspend, className }: ConnectorCardProps) { ... }

// ✗ wrong — no React.FC, no implicit children: any
export const ConnectorCard: React.FC<{}> = () => { ... }
```

### 3.2 Type guard pattern for SSE events

```ts
// types/events.ts
export function isAuditEvent(e: unknown): e is AuditEvent {
    return typeof e === 'object' && e !== null && (e as any).event_type?.startsWith('audit.')
}

export function isAlertEvent(e: unknown): e is AlertEvent {
    return typeof e === 'object' && e !== null && (e as any).event_type?.startsWith('alert.')
}
```

### 3.3 Query key factories (no inline string arrays)

```ts
// lib/query/keys.ts — ONLY place query keys are defined
export const queryKeys = {
    connectors: {
        all:        (orgId: string) => ['connectors', orgId] as const,
        detail:     (orgId: string, id: string) => ['connectors', orgId, id] as const,
        deliveries: (orgId: string, id: string) => ['connectors', orgId, id, 'deliveries'] as const,
    },
    policies: {
        all:      (orgId: string) => ['policies', orgId] as const,
        detail:   (orgId: string, id: string) => ['policies', orgId, id] as const,
        evalLogs: (orgId: string) => ['policies', orgId, 'eval-logs'] as const,
    },
    audit: {
        events:    (orgId: string, filters: AuditFilters) => ['audit', orgId, 'events', filters] as const,
        integrity: (orgId: string) => ['audit', orgId, 'integrity'] as const,
    },
    users: {
        all:      (orgId: string) => ['users', orgId] as const,
        detail:   (orgId: string, id: string) => ['users', orgId, id] as const,
        sessions: (orgId: string, userId: string) => ['users', orgId, userId, 'sessions'] as const,
    },
}
```

---

## 4. API Client Layer

### 4.1 Base client (the only way to talk to the backend)

```ts
// lib/api/client.ts
export interface APIError {
    code: string
    message: string
    request_id: string
    trace_id: string
    retryable: boolean
    fields?: { field: string; message: string }[]   // VALIDATION_ERROR only
}

export class OpenGuardAPIError extends Error {
    code: string
    requestId: string
    traceId: string
    retryable: boolean
    fields?: APIError['fields']

    constructor(body: APIError) {
        super(ERROR_MESSAGES[body.code] ?? body.message)
        this.code = body.code
        this.requestId = body.request_id
        this.traceId = body.trace_id
        this.retryable = body.retryable
        this.fields = body.fields
    }
}

async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
    const base = typeof window === 'undefined'
        ? process.env.INTERNAL_API_URL      // server: direct to control plane
        : process.env.NEXT_PUBLIC_API_URL

    const session = await getSession()
    if (!session?.accessToken) throw new OpenGuardAPIError({ code: 'UNAUTHORIZED', ... })

    const headers: HeadersInit = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${session.accessToken}`,
        ...(options.idempotencyKey && { 'Idempotency-Key': options.idempotencyKey }),
    }

    const res = await fetch(`${base}${path}`, { ...options, headers })
    if (!res.ok) {
        const body = await res.json().catch(() => ({ code: 'UNKNOWN_ERROR', ... }))
        throw new OpenGuardAPIError(body.error ?? body)
    }
    if (res.status === 204) return undefined as T
    return res.json() as Promise<T>
}

export const api = {
    get:   <T>(path: string, opts?: FetchOptions) => apiFetch<T>(path, { ...opts, method: 'GET' }),
    post:  <T>(path: string, body: unknown, opts?: FetchOptions) =>
        apiFetch<T>(path, { ...opts, method: 'POST', body: JSON.stringify(body) }),
    patch: <T>(path: string, body: unknown, opts?: FetchOptions) =>
        apiFetch<T>(path, { ...opts, method: 'PATCH', body: JSON.stringify(body) }),
    del:   <T>(path: string, opts?: FetchOptions) => apiFetch<T>(path, { ...opts, method: 'DELETE' }),
}
```

### 4.2 Error message mapping

```ts
// lib/utils/error-messages.ts
export const ERROR_MESSAGES: Record<string, string> = {
    RESOURCE_NOT_FOUND:    'This resource no longer exists.',
    RESOURCE_CONFLICT:     'A resource with these details already exists.',
    FORBIDDEN:             'You do not have permission to perform this action.',
    UPSTREAM_UNAVAILABLE:  'A dependent service is temporarily unavailable. Please try again.',
    CAPACITY_EXCEEDED:     'The system is under high load. Please retry in a moment.',
    VALIDATION_ERROR:      'Please check the form for errors.',
    CONNECTOR_SUSPENDED:   'This connector is suspended.',
    INSUFFICIENT_SCOPE:    'This connector lacks the required permissions.',
    DLP_POLICY_VIOLATION:  'Event blocked: DLP policy violation detected.',
    SESSION_REVOKED_RISK:  'Your session was revoked due to suspicious activity. Please log in again.',
    SESSION_COMPROMISED:   'Session compromised. Please log in again.',
    TOTP_REPLAY_DETECTED:  'This MFA code has already been used. Please wait for the next code.',
}
```

### 4.3 Cursor pagination helpers

```ts
// lib/api/pagination.ts
export function encodeCursor(timestamp: number, id: string): string {
    return Buffer.from(JSON.stringify({ t: timestamp, id })).toString('base64url')
}

export function decodeCursor(cursor: string): { t: number; id: string } | null {
    try {
        return JSON.parse(Buffer.from(cursor, 'base64url').toString())
    } catch {
        return null    // never throw — return null for malformed input
    }
}
```

### 4.4 ETag-aware mutations

```ts
// GET stores ETag in ref. PATCH sends If-Match.
const etag = res.headers.get('ETag')

const update = useMutation({
    mutationFn: (data: UpdateUserInput) =>
        api.patch(`/v1/users/${userId}`, data, {
            headers: { 'If-Match': etagRef.current ?? '' }
        }),
    onError: (err: OpenGuardAPIError) => {
        if (err.code === 'PRECONDITION_FAILED') {
            // prompt user to refresh and retry
            toast.error('This record was updated by someone else. Please refresh.')
            queryClient.invalidateQueries({ queryKey: queryKeys.users.detail(orgId, userId) })
        }
    }
})
```

---

## 5. State Management

### 5.1 Decision table — where data lives

| Data type                           | Tool                   | Location                  |
|-------------------------------------|------------------------|---------------------------|
| Server data (lists, details)        | TanStack Query         | `useQuery` / `useMutation`|
| Form state                          | React Hook Form        | `useForm`                 |
| Global UI (sidebar, modals, drawers)| Zustand `ui` store     | `useUIStore`              |
| Notifications / toasts              | Zustand `notification` | `useNotificationStore`    |
| Auth session                        | NextAuth.js            | `useSession` / `auth()`   |
| Org context                         | NextAuth → `useOrg`    | Derived from session      |
| URL filters / pagination cursors    | nuqs                   | `useSearchParams`         |
| Real-time SSE data                  | Local `useState`       | Inside `useAuditStream`   |

**Rule**: TanStack Query is the single source of truth for all server data. Never duplicate server data into Zustand.

### 5.2 TanStack Query — mutation pattern

```tsx
// ✓ correct mutation pattern
const suspendConnector = useMutation({
    mutationFn: (id: string) => api.connectors.suspend(id),
    onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: queryKeys.connectors.all(orgId) })
        toast.success('Connector suspended')
    },
    onError: (error: OpenGuardAPIError) => {
        toast.error(error.message ?? 'Failed to suspend connector')
    },
})
```

### 5.3 Optimistic status toggle

```tsx
const toggleStatus = useMutation({
    mutationFn: (status: 'active' | 'suspended') =>
        api.patch(`/v1/admin/connectors/${id}`, { status }),

    onMutate: async (newStatus) => {
        await queryClient.cancelQueries({ queryKey: queryKeys.connectors.detail(orgId, id) })
        const previous = queryClient.getQueryData(queryKeys.connectors.detail(orgId, id))
        queryClient.setQueryData(queryKeys.connectors.detail(orgId, id),
            (old: Connector) => ({ ...old, status: newStatus }))
        return { previous }
    },

    onError: (_err, _vars, context) => {
        queryClient.setQueryData(queryKeys.connectors.detail(orgId, id), context?.previous)
        toast.error('Failed to update connector status')
    },

    onSettled: () => {
        queryClient.invalidateQueries({ queryKey: queryKeys.connectors.detail(orgId, id) })
    },
})
```

### 5.4 Zustand UI store

```ts
// lib/store/ui.ts — persist ONLY sidebarCollapsed across sessions
export const useUIStore = create<UIState>()(
    persist(
        (set) => ({
            sidebarCollapsed: false,
            toggleSidebar: () => set(s => ({ sidebarCollapsed: !s.sidebarCollapsed })),

            activeDrawer: { type: null, id: null },
            openDrawer: (type, id) => set({ activeDrawer: { type, id } }),
            closeDrawer: () => set({ activeDrawer: { type: null, id: null } }),

            auditStreamPaused: false,
            setAuditStreamPaused: (paused) => set({ auditStreamPaused: paused }),

            // imperative confirm dialog
            confirmDialog: null,
            openConfirm: (state) => set({ confirmDialog: state }),
            closeConfirm: () => set({ confirmDialog: null, resolveConfirm: null }),
            resolveConfirm: null,
            setResolveConfirm: (fn) => set({ resolveConfirm: fn }),
        }),
        {
            name: 'og:ui',
            partialize: (state) => ({ sidebarCollapsed: state.sidebarCollapsed }),
            // auth state is NEVER persisted — always from NextAuth session
        }
    )
)
```

---

## 6. Authentication & Session

### 6.1 NextAuth.js v5 config

```ts
// lib/auth/config.ts
export const config: NextAuthConfig = {
    providers: [{
        id: 'openguard-iam',
        type: 'oidc',
        issuer: process.env.IAM_OIDC_ISSUER,
        clientId: process.env.IAM_OIDC_CLIENT_ID,
        clientSecret: process.env.IAM_OIDC_CLIENT_SECRET,
        authorization: {
            params: {
                scope: 'openid email profile',
                code_challenge_method: 'S256',   // PKCE required per BE spec
            },
        },
    }],

    callbacks: {
        async jwt({ token, account, profile }) {
            if (account?.access_token) {
                token.accessToken = account.access_token
                token.refreshToken = account.refresh_token
                token.accessTokenExpires = account.expires_at
                token.orgId = (profile as any)?.org_id
                token.mfaRequired = (profile as any)?.mfa_required ?? false
                token.mfaVerified = false
            }
            // Proactive refresh — 60s before expiry
            if (Date.now() < (token.accessTokenExpires as number) * 1000 - 60_000) {
                return token
            }
            return refreshAccessToken(token)
        },

        async session({ session, token }) {
            session.accessToken = token.accessToken as string
            session.orgId       = token.orgId as string
            session.mfaRequired = token.mfaRequired as boolean
            session.mfaVerified = token.mfaVerified as boolean
            session.error       = token.error as string | undefined
            return session
        },
    },

    pages: { signIn: '/login', error: '/login' },
    session: { strategy: 'jwt' },
}
```

### 6.2 Session revocation handling

When API returns `SESSION_REVOKED_RISK` or `SESSION_COMPROMISED`:

```ts
// In apiFetch error handler
if (err.code === 'SESSION_REVOKED_RISK' || err.code === 'SESSION_COMPROMISED') {
    await signOut({ callbackUrl: '/login' })    // immediate sign-out, no confirmation needed
}
```

### 6.3 useOrg hook (org_id always from session)

```ts
// lib/hooks/use-org.ts
export function useOrg(): { orgId: string } {
    const { data: session } = useSession()
    const orgId = session?.orgId
    if (!orgId) throw new Error('useOrg called outside authenticated context')
    return { orgId }
}

// ✗ never — org_id from URL param
const orgId = params.orgId

// ✓ always
const { orgId } = useOrg()
```

---

## 7. Forms & Validation

```tsx
// ✓ correct — React Hook Form + Zod, always
const schema = z.object({
    name: z.string().min(2).max(100),
    url:  z.string().url(),
    scopes: z.array(z.enum(['events:write', 'users:read', 'policies:read'])).min(1),
})

type FormValues = z.infer<typeof schema>

function ConnectorForm() {
    const form = useForm<FormValues>({ resolver: zodResolver(schema) })

    const create = useMutation({
        mutationFn: (data: FormValues) =>
            api.post('/v1/admin/connectors', data, {
                idempotencyKey: crypto.randomUUID(),   // required on POST
            }),
        onSuccess: () => { /* invalidate + toast */ },
        onError: (err: OpenGuardAPIError) => {
            // Map field errors from VALIDATION_ERROR response
            err.fields?.forEach(f => form.setError(f.field as keyof FormValues,
                { message: f.message }))
        },
    })

    return (
        <form onSubmit={form.handleSubmit(data => create.mutate(data))}>
            {/* fields */}
        </form>
    )
}
```

---

## 8. Real-Time SSE

### 8.1 SSE hook

```ts
// lib/hooks/use-sse.ts
export function useSSE<T>(url: string, options?: UseSSEOptions<T>) {
    const [events, setEvents] = useState<T[]>([])
    const [status, setStatus] = useState<'connecting' | 'open' | 'error'>('connecting')

    useEffect(() => {
        const source = new EventSource(url)    // connects to /api/stream/* route handler

        source.onopen  = () => setStatus('open')
        source.onerror = () => {
            setStatus('error')
            // exponential backoff reconnect is handled by browser EventSource automatically
        }

        source.onmessage = (e) => {
            try {
                const parsed = JSON.parse(e.data) as T
                setEvents(prev => [parsed, ...prev].slice(0, options?.maxBuffer ?? 500))
                options?.onEvent?.(parsed)
            } catch { /* malformed SSE data — ignore */ }
        }

        return () => source.close()    // cleanup on unmount
    }, [url])

    return { events, status }
}
```

### 8.2 Audit stream with pause/resume

```tsx
// components/domain/audit-stream.client.tsx
"use client"

export function AuditStream() {
    const { auditStreamPaused, setAuditStreamPaused } = useUIStore()
    const bufferRef = useRef<AuditEvent[]>([])
    const [displayed, setDisplayed] = useState<AuditEvent[]>([])

    const { events } = useSSE<AuditEvent>('/api/stream/audit', {
        onEvent: (e) => {
            if (auditStreamPaused) {
                bufferRef.current.push(e)   // buffer while paused
            } else {
                setDisplayed(prev => [e, ...prev].slice(0, 200))
            }
        }
    })

    const resume = () => {
        setDisplayed(prev => [...bufferRef.current, ...prev].slice(0, 200))
        bufferRef.current = []
        setAuditStreamPaused(false)
    }

    // ...
}
```

---

## 9. Canonical Component Patterns

### 9.1 Cursor-paginated table

```tsx
// Uses useInfiniteQuery — NOT manual offset arithmetic
const { data, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: queryKeys.audit.events(orgId, filters),
    queryFn: ({ pageParam }) =>
        api.audit.listEvents(orgId, { cursor: pageParam, limit: 50, ...filters }),
    getNextPageParam: (lastPage) => lastPage.meta.next_cursor ?? undefined,
    initialPageParam: undefined,
})
```

### 9.2 Job status polling (compliance report)

```tsx
// Uses refetchInterval — NOT setInterval
const { data: job } = useQuery({
    queryKey: ['report-job', jobId],
    queryFn: () => api.compliance.getJobStatus(orgId, jobId),
    refetchInterval: (query) =>
        query.state.data?.status === 'completed' ? false : 2000,
    enabled: !!jobId,
})
```

### 9.3 API key one-time reveal

```tsx
// Key shown exactly once after connector creation — never fetchable again
function ApiKeyReveal({ keyPlaintext }: { keyPlaintext: string }) {
    const [revealed, setRevealed] = useState(false)
    const [copied, setCopied] = useState(false)

    if (!revealed) {
        return <Button onClick={() => setRevealed(true)}>Reveal API Key</Button>
    }

    return (
        <div>
            <code className="font-mono">{keyPlaintext}</code>
            <Button onClick={() => { navigator.clipboard.writeText(keyPlaintext); setCopied(true) }}>
                {copied ? 'Copied!' : 'Copy'}
            </Button>
            <p className="text-destructive text-sm">
                This key will not be shown again. Store it securely.
            </p>
        </div>
    )
}
```

### 9.4 Destructive action — always ConfirmDialog

```tsx
// useConfirm hook — imperative pattern via Zustand
const confirm = useConfirm()

async function handleSuspend(id: string, name: string) {
    const confirmed = await confirm({
        title: 'Suspend Connector',
        description: `This will immediately revoke ${name}'s access.`,
        variant: 'destructive',
        confirmLabel: 'Suspend',
        requireTyped: name,          // user must type the connector name to confirm
    })
    if (!confirmed) return
    suspendConnector.mutate(id)
}
```

### 9.5 Redactable sensitive data

```tsx
// All email, ip_address, token_prefix fields must use <Redactable>
<Redactable field="email" value={user.email} />
<Redactable field="ip_address" value={session.ip_address} />
<Redactable field="token_prefix" value={`${token.prefix}...`} />

// Redactable reads org's data visibility settings from session context
// Never render these fields as plain text anywhere
```

---

## 10. Security — Frontend

### 10.1 Content Security Policy (next.config.js)

```js
// next.config.js — CSP set server-side, not in components
const securityHeaders = [
    { key: 'Content-Security-Policy', value: cspHeader },
    { key: 'Strict-Transport-Security', value: 'max-age=63072000; includeSubDomains' },
    { key: 'X-Frame-Options', value: 'DENY' },
    { key: 'X-Content-Type-Options', value: 'nosniff' },
    { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
]

// No inline scripts outside CSS Modules / Tailwind
// Legitimate inline scripts MUST use nonce injected by middleware
```

### 10.2 Auth storage rules

```
✓ accessToken    → httpOnly cookie (NextAuth manages this)
✓ refreshToken   → httpOnly cookie (NextAuth manages this)
✓ orgId          → session (derived from JWT claim, never stored separately)
✗ NEVER          → localStorage, sessionStorage, client-accessible cookies
✗ NEVER          → window.__token or any global variable
```

### 10.3 Forbidden patterns summary

| Pattern                                  | Correct alternative                            |
|------------------------------------------|------------------------------------------------|
| `fetch('/api/...')` in component         | `api.get(...)` from `lib/api/*`               |
| `localStorage.setItem('token', ...)`     | NextAuth httpOnly cookies                      |
| `params.orgId` for data fetching         | `useOrg()` hook → session-derived             |
| `<input onChange={e => setVal(e.target.value)}>` | React Hook Form `register()`          |
| `useEffect(() => { fetchData() }, [])`   | `useQuery` with query key                      |
| `setInterval(refetch, 2000)`             | `useQuery({ refetchInterval: 2000 })`         |
| `<button onClick={deleteResource}>Delete</button>` | `useConfirm()` + ConfirmDialog      |
| `<td>{user.email}</td>`                  | `<Redactable field="email" value={user.email} />`|
| `style={{ color: 'red' }}`              | Tailwind class or CSS Module                   |
| `console.log(user)`                      | Remove before commit                           |

---

## 11. Testing Standards

### 11.1 Coverage thresholds

| Layer              | Tool                       | Threshold               |
|--------------------|----------------------------|-------------------------|
| Unit (utils, validators, hooks) | Vitest          | 80% coverage            |
| Component tests    | Vitest + Testing Library   | 70% coverage            |
| API client layer   | Vitest + MSW               | 100% of lib/api/* modules|
| E2E critical paths | Playwright                 | All flows in §13.5      |
| Accessibility      | axe-playwright             | 0 WCAG AA violations    |
| Performance        | Lighthouse CI              | LCP < 2.5s, TBT < 300ms, CLS < 0.1 |

### 11.2 MSW setup (API mocking — not jest.fn())

```ts
// test/mocks/handlers.ts
import { http, HttpResponse } from 'msw'

export const handlers = [
    http.get(`${NEXT_PUBLIC_API_URL}/v1/connectors`, () =>
        HttpResponse.json({ data: mockConnectors, meta: mockMeta })),

    http.patch(`${NEXT_PUBLIC_API_URL}/v1/admin/connectors/:id`, async ({ request }) => {
        const body = await request.json() as any
        return HttpResponse.json({ ...mockConnectors[0], status: body.status })
    }),
]
```

### 11.3 Testing principles

- Use `screen.getByRole`, `getByLabelText`, `getByText` — avoid `getByTestId`
- Test user-visible behavior, not implementation details
- Every destructive action test must verify ConfirmDialog appears
- SSE tests use `ReadableStream` mock — not actual `EventSource`
- Error boundary tests: verify recoverable UI renders on unhandled rejection

---

## 12. Environment Variables

```bash
# Public (available to browser)
NEXT_PUBLIC_API_URL=https://api.openguard.example.com

# Server-only (never exposed to browser)
INTERNAL_API_URL=http://control-plane:8080      # direct internal URL
INTERNAL_IAM_URL=http://iam:8081                # Used by NextAuth for server-side OIDC token exchange. Do not conflate with INTERNAL_API_URL.
IAM_OIDC_ISSUER=https://iam.openguard.example.com
IAM_OIDC_CLIENT_ID=dashboard
IAM_OIDC_CLIENT_SECRET=<secret>
NEXTAUTH_SECRET=<secret>
NEXTAUTH_URL=https://dashboard.openguard.example.com
```

Never access `process.env.IAM_OIDC_CLIENT_SECRET` in client-side code. If an env var must reach the browser, it **must** start with `NEXT_PUBLIC_`.

---

## 13. Tech Stack Reference

| Concern          | Choice                           | Notes                              |
|------------------|----------------------------------|------------------------------------|
| Framework        | Next.js 14 App Router            | RSC-first, `app/` directory        |
| Language         | TypeScript 5.x, `strict: true`   | No `any`                           |
| Styling          | Tailwind CSS + CSS Modules       | Utilities + component overrides    |
| Component library| Radix UI (headless) + custom     | Own the styles (shadcn pattern)    |
| Forms            | React Hook Form + Zod            | No exceptions                      |
| Server state     | TanStack Query v5                | `useQuery`, `useMutation`          |
| Client state     | Zustand v4                       | UI-only state                      |
| Auth             | NextAuth.js v5 (Auth.js)         | OIDC → IAM service                 |
| Real-time        | Native `EventSource` (SSE)       | No socket.io, no WebSockets        |
| Charts           | Recharts v2                      | Wrapped in typed components        |
| Tables           | TanStack Table v8                | All data tables                    |
| Testing          | Vitest + Testing Library + Playwright | See §11                      |
| Icons            | Lucide React                     | Only icon library permitted        |
| Animation        | Framer Motion v11                | Page transitions + complex UI only |
| URL state        | nuqs                             | Filters, cursors, sort order       |
