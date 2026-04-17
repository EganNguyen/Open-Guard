# §02 — API Client Layer

All communication with the OpenGuard backend goes through `lib/api/`. Components and hooks never call `fetch` directly.

---

## 2.1 Base Client & Interceptor

In Angular, we use the built-in `HttpClient` and `HttpInterceptor` to handle authentication, base URLs, and error normalization.

```typescript
// src/app/core/api/api.interceptor.ts
import { HttpInterceptorFn, HttpRequest, HttpHandlerFn, HttpErrorResponse } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, throwError } from 'rxjs';
import { AuthService } from '../auth/auth.service';
import { ERROR_MESSAGES } from '../utils/error-messages';

export const apiInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(AuthService);
  const baseUrl = environment.apiUrl;

  // 1. Clone request with Base URL and Headers
  let apiReq = req.clone({
    url: `${baseUrl}${req.url}`,
    setHeaders: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${auth.accessToken()}`,
    }
  });

  // 2. Handle request
  return next(apiReq).pipe(
    catchError((error: HttpErrorResponse) => {
      const apiError = error.error?.error ?? error.error;
      const normalizedError = new OpenGuardAPIError({
        code: apiError?.code ?? 'UNKNOWN_ERROR',
        message: ERROR_MESSAGES[apiError?.code] ?? apiError?.message ?? error.message,
        request_id: error.headers.get('X-Request-ID') ?? '',
        trace_id: '',
        retryable: error.status >= 500
      });
      return throwError(() => normalizedError);
    })
  );
};
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
```

---

### `src/app/core/api/connectors.service.ts`

```typescript
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import type { Connector, ConnectorCreateInput, WebhookDelivery } from '../models';
import type { OffsetPage, CursorPage } from './pagination';

@Injectable({ providedIn: 'root' })
export class ConnectorsService {
  private http = inject(HttpClient);

  list(orgId: string, page = 1): Observable<OffsetPage<Connector>> {
    return this.http.get<OffsetPage<Connector>>(`/v1/connectors`, {
      params: { page, per_page: 50 },
      headers: { 'X-Org-ID': orgId }
    });
  }

  get(orgId: string, id: string): Observable<Connector> {
    return this.http.get<Connector>(`/v1/connectors/${id}`, {
      headers: { 'X-Org-ID': orgId }
    });
  }

  suspend(orgId: string, id: string): Observable<Connector> {
    return this.http.patch<Connector>(`/v1/admin/connectors/${id}`, { status: 'suspended' }, {
      headers: { 'X-Org-ID': orgId }
    });
  }
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

## 2.4 SSE Service

Real-time data uses a dedicated Angular Service that wraps `EventSource`.

```typescript
// src/app/core/sse/sse.service.ts
import { Injectable, signal, NgZone } from '@angular/core';

@Injectable({ providedIn: 'root' })
export class SseService {
  connected = signal(false);

  connect<T>(url: string, onMessage: (data: T) => void) {
    const es = new EventSource(url);

    es.onopen = () => this.connected.set(true);

    es.onmessage = (e) => {
      const data = JSON.parse(e.data) as T;
      onMessage(data);
    };

    es.onerror = () => {
      this.connected.set(false);
    };

    return () => es.close();
  }
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

## 2.5 Reactive State & Optimistic Updates

In Angular, we use **Signals** to handle local state and optimistic updates.

```typescript
// src/app/features/connectors/connectors.component.ts
connectors = signal<Connector[]>([]);

suspendConnector(id: string) {
  const previous = this.connectors();

  // 1. Optimistic Update
  this.connectors.update(list =>
    list.map(c => c.id === id ? { ...c, status: 'suspended' } : c)
  );

  // 2. API Call
  this.connectorsService.suspend(orgId, id).subscribe({
    error: () => {
      // 3. Rollback on failure
      this.connectors.set(previous);
      this.toast.error('Failed to suspend connector');
    }
  });
}
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
