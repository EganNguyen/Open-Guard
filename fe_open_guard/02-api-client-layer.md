# §02 — API Client Layer

All communication with the OpenGuard backend goes through `lib/api/`. Components and hooks never call `fetch` directly.

---

## 2.1 Base Client

```ts
// lib/api/client.ts
import { getSession } from 'next-auth/react'
import { ERROR_MESSAGES } from '@/lib/utils/error-messages'

export interface APIError {
  code: string
  message: string
  request_id: string
  trace_id: string
  retryable: boolean
  fields?: { field: string; message: string }[] // VALIDATION_ERROR only
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

interface FetchOptions extends RequestInit {
  orgId?: string         // injected from session; overrides default
  idempotencyKey?: string
}

async function apiFetch<T>(path: string, options: FetchOptions = {}): Promise<T> {
  // 1. Resolve base URL — server vs client
  const base = typeof window === 'undefined'
    ? process.env.INTERNAL_API_URL   // server: direct to control plane (no hop through browser)
    : process.env.NEXT_PUBLIC_API_URL

  // 2. Get auth token
  const session = await getSession()
  if (!session?.accessToken) {
    throw new OpenGuardAPIError({
      code: 'UNAUTHORIZED',
      message: 'Session expired. Please log in again.',
      request_id: '',
      trace_id: '',
      retryable: false,
    })
  }

  // 3. Build headers
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${session.accessToken}`,
    ...( options.orgId && { 'X-Org-ID': options.orgId }),  // Admin multi-org override only
    ...( options.idempotencyKey && { 'Idempotency-Key': options.idempotencyKey }),
    ...options.headers,
  }

  // 4. Fetch
  const res = await fetch(`${base}${path}`, { ...options, headers })

  // 5. Handle non-2xx
  if (!res.ok) {
    const body = await res.json().catch(() => ({
      code: 'UNKNOWN_ERROR',
      message: res.statusText,
      request_id: res.headers.get('X-Request-ID') ?? '',
      trace_id: '',
      retryable: res.status >= 500,
    }))
    throw new OpenGuardAPIError(body.error ?? body)
  }

  // 204 No Content
  if (res.status === 204) return undefined as T

  return res.json() as Promise<T>
}

export const api = {
  get:    <T>(path: string, opts?: FetchOptions) => apiFetch<T>(path, { ...opts, method: 'GET' }),
  post:   <T>(path: string, body: unknown, opts?: FetchOptions) =>
    apiFetch<T>(path, { ...opts, method: 'POST', body: JSON.stringify(body) }),
  patch:  <T>(path: string, body: unknown, opts?: FetchOptions) =>
    apiFetch<T>(path, { ...opts, method: 'PATCH', body: JSON.stringify(body) }),
  put:    <T>(path: string, body: unknown, opts?: FetchOptions) =>
    apiFetch<T>(path, { ...opts, method: 'PUT', body: JSON.stringify(body) }),
  delete: <T>(path: string, opts?: FetchOptions) => apiFetch<T>(path, { ...opts, method: 'DELETE' }),
}
```

---

## 2.2 Pagination Helpers

The BE uses two pagination patterns (from §4.7 of the BE spec):

```ts
// lib/api/pagination.ts

// Cursor-based (audit events, threat alerts, DLP findings)
export interface CursorPage<T> {
  data: T[]
  meta: {
    next_cursor: string | null
    total: null  // not provided for cursor endpoints
    per_page: number
  }
}

// Offset-based (users, policies)
export interface OffsetPage<T> {
  data: T[]
  meta: {
    page: number
    per_page: number
    total: number
    total_pages: number
    next_cursor: null
  }
}

// Encode/decode keyset cursors
export function encodeCursor(t: number, id: string): string {
  return btoa(JSON.stringify({ t, id }))
}

export function decodeCursor(cursor: string): { t: number; id: string } | null {
  try {
    return JSON.parse(atob(cursor))
  } catch {
    return null
  }
}

// TanStack Query infinite query helper for cursor-based endpoints
export function getNextPageParam<T>(lastPage: CursorPage<T>) {
  return lastPage.meta.next_cursor ?? undefined
}
```

---

## 2.3 Resource-Specific API Modules

### `lib/api/connectors.ts`

```ts
import { api } from './client'
import type { Connector, ConnectorCreateInput, WebhookDelivery } from '@/types/models'
import type { OffsetPage } from './pagination'

export const connectorsApi = {
  list: (orgId: string, page = 1) =>
    api.get<OffsetPage<Connector>>(`/v1/connectors?page=${page}&per_page=50`, { orgId }),

  get: (orgId: string, id: string) =>
    api.get<Connector>(`/v1/connectors/${id}`, { orgId }),

  create: (orgId: string, input: ConnectorCreateInput, idempotencyKey: string) =>
    api.post<{ connector: Connector; api_key_plaintext: string }>(
      `/v1/admin/connectors`,
      input,
      { orgId, idempotencyKey }
    ),

  update: (orgId: string, id: string, patch: Partial<ConnectorCreateInput>) =>
    api.patch<Connector>(`/v1/admin/connectors/${id}`, patch, { orgId }),

  suspend: (orgId: string, id: string) =>
    api.patch<Connector>(`/v1/admin/connectors/${id}`, { status: 'suspended' }, { orgId }),

  activate: (orgId: string, id: string) =>
    api.patch<Connector>(`/v1/admin/connectors/${id}`, { status: 'active' }, { orgId }),

  delete: (orgId: string, id: string) =>
    api.delete<void>(`/v1/admin/connectors/${id}`, { orgId }),

  sendTestWebhook: (orgId: string, id: string) =>
    api.post<void>(`/v1/admin/connectors/${id}/test`, {}, { orgId }),

  listDeliveries: (orgId: string, id: string, cursor?: string) =>
    api.get<CursorPage<WebhookDelivery>>(
      `/v1/admin/connectors/${id}/deliveries${cursor ? `?cursor=${cursor}` : ''}`,
      { orgId }
    ),
}
```

### `lib/api/policies.ts`

```ts
export const policiesApi = {
  list: (orgId: string, page = 1) =>
    api.get<OffsetPage<Policy>>(`/v1/policies?page=${page}&per_page=50`, { orgId }),

  get: (orgId: string, id: string) =>
    api.get<Policy>(`/v1/policies/${id}`, { orgId }),

  create: (orgId: string, input: PolicyCreateInput) =>
    api.post<Policy>(`/v1/policies`, input, { orgId }),

  update: (orgId: string, id: string, input: PolicyUpdateInput) =>
    api.put<Policy>(`/v1/policies/${id}`, input, { orgId }),

  delete: (orgId: string, id: string) =>
    api.delete<void>(`/v1/policies/${id}`, { orgId }),

  evaluate: (orgId: string, req: PolicyEvaluateRequest) =>
    api.post<PolicyEvaluateResponse>(`/v1/policy/evaluate`, req, { orgId }),

  listEvalLogs: (orgId: string, filters: EvalLogFilters) =>
    api.get<OffsetPage<EvalLogEntry>>(
      `/v1/policy/eval-logs?${new URLSearchParams(filters as any)}`,
      { orgId }
    ),
}
```

### `lib/api/audit.ts`

```ts
export const auditApi = {
  // Cursor-based (keyset on occurred_at + event_id)
  listEvents: (orgId: string, filters: AuditFilters, cursor?: string) =>
    api.get<CursorPage<AuditEvent>>(
      `/audit/events?${buildAuditParams(filters, cursor)}`,
      { orgId }
    ),

  getEvent: (orgId: string, id: string) =>
    api.get<AuditEvent>(`/audit/events/${id}`, { orgId }),

  checkIntegrity: (orgId: string) =>
    api.get<IntegrityResult>(`/audit/integrity`, { orgId }),
    // NOTE: This endpoint reads from MongoDB primary (see BE spec §2.4).
    // Expect slightly higher latency than regular audit queries.

  createExport: (orgId: string, input: ExportInput) =>
    api.post<ExportJob>(`/audit/export`, input, { orgId }),

  getExportJob: (orgId: string, jobId: string) =>
    api.get<ExportJob>(`/audit/export/${jobId}`, { orgId }),

  getExportDownloadUrl: (orgId: string, jobId: string) =>
    `/audit/export/${jobId}/download`, // streamed directly; not via apiFetch
}
```

### `lib/api/threats.ts`

```ts
export const threatsApi = {
  listAlerts: (orgId: string, filters: AlertFilters, cursor?: string) =>
    api.get<CursorPage<ThreatAlert>>(
      `/v1/threats/alerts?${buildAlertParams(filters, cursor)}`,
      { orgId }
    ),

  getAlert: (orgId: string, id: string) =>
    api.get<ThreatAlert>(`/v1/threats/alerts/${id}`, { orgId }),

  acknowledge: (orgId: string, id: string) =>
    api.post<ThreatAlert>(`/v1/threats/alerts/${id}/acknowledge`, {}, { orgId }),

  resolve: (orgId: string, id: string, note?: string) =>
    api.post<ThreatAlert>(`/v1/threats/alerts/${id}/resolve`, { note }, { orgId }),

  getStats: (orgId: string) =>
    api.get<ThreatStats>(`/v1/threats/stats`, { orgId }),
}
```

---

## 2.4 SSE Client Hook

Real-time data (audit stream, live threat alerts) uses Server-Sent Events routed through Next.js route handlers (which proxy to the backend and forward the auth token).

```ts
// lib/hooks/use-sse.ts
'use client'

import { useEffect, useRef, useState } from 'react'

interface UseSSEOptions<T> {
  url: string
  onMessage: (data: T) => void
  onError?: (err: Event) => void
  enabled?: boolean
}

export function useSSE<T>({ url, onMessage, onError, enabled = true }: UseSSEOptions<T>) {
  const [connected, setConnected] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!enabled) return

    const es = new EventSource(url)
    esRef.current = es

    es.onopen = () => setConnected(true)

    es.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data) as T
        onMessage(data)
      } catch {
        // malformed JSON — skip silently, log in production
      }
    }

    es.onerror = (err) => {
      setConnected(false)
      onError?.(err)
      // EventSource auto-reconnects on error; no manual retry needed
    }

    return () => {
      es.close()
      esRef.current = null
      setConnected(false)
    }
  }, [url, enabled])

  return { connected }
}
```

**Route handler (Next.js proxies to audit service):**

```ts
// app/api/stream/audit/route.ts
import { auth } from '@/lib/auth'
import { NextRequest } from 'next/server'

export async function GET(req: NextRequest) {
  const session = await auth()
  if (!session?.accessToken) {
    return new Response('Unauthorized', { status: 401 })
  }

  const upstream = await fetch(`${process.env.INTERNAL_API_URL}/audit/stream`, {
    headers: {
      Authorization: `Bearer ${session.accessToken}`,
      Accept: 'text/event-stream',
    },
  })

  // Forward the SSE stream as-is
  return new Response(upstream.body, {
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    },
  })
}
```

---

## 2.5 Optimistic Updates

For low-latency UI (connector status toggle, policy enable/disable), use TanStack Query optimistic updates:

```ts
// Example: optimistic connector suspend
const suspendConnector = useMutation({
  mutationFn: (id: string) => connectorsApi.suspend(orgId, id),

  onMutate: async (id) => {
    await queryClient.cancelQueries({ queryKey: queryKeys.connectors.all(orgId) })
    const previous = queryClient.getQueryData<OffsetPage<Connector>>(queryKeys.connectors.all(orgId))

    // Optimistically update
    queryClient.setQueryData(queryKeys.connectors.all(orgId), (old: OffsetPage<Connector>) => ({
      ...old,
      data: old.data.map(c => c.id === id ? { ...c, status: 'suspended' } : c),
    }))

    return { previous }
  },

  onError: (_err, _id, context) => {
    // Roll back on error
    queryClient.setQueryData(queryKeys.connectors.all(orgId), context?.previous)
    toast.error('Failed to suspend connector')
  },

  onSettled: () => {
    queryClient.invalidateQueries({ queryKey: queryKeys.connectors.all(orgId) })
  },
})
```

---

## 2.6 Idempotency Keys

For any mutation that creates a resource, generate and store an idempotency key to handle network retries safely:

```ts
// lib/utils/idempotency.ts
import { v4 as uuidv4 } from 'uuid'

// Generate a stable key per form submission attempt.
// Stored in component state (not persisted) — regenerated on fresh form open.
export function generateIdempotencyKey(): string {
  return uuidv4()
}
```

```tsx
// Usage in connector registration form
const [idempotencyKey] = useState(() => generateIdempotencyKey())

const createConnector = useMutation({
  mutationFn: (input: ConnectorCreateInput) =>
    connectorsApi.create(orgId, input, idempotencyKey),
  // ...
})
```

If the mutation returns `Idempotency-Replayed: true` header, show a notice: "This connector was already created — showing existing record."
