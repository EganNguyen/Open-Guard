# §18 — Component Implementation Patterns

Canonical patterns for building the most complex and reused component types in the dashboard.

---

## 18.1 Data Table with Server-Side Pagination

The pattern for all paginated list pages (connectors, users, policies, etc.):

```typescript
// src/app/shared/components/paginated-table/paginated-table.ts
import { Component, Input, signal, inject, effect } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';

@Component({
  selector: 'og-paginated-table',
  templateUrl: './paginated-table.html'
})
export class PaginatedTableComponent<T> {
  @Input() queryKey!: string;
  @Input() fetcher!: (page: number) => Observable<OffsetPage<T>>;
  @Input() columns!: ColumnDef<T>[];

  private route = inject(ActivatedRoute);
  private router = inject(Router);
  
  data = signal<OffsetPage<T> | null>(null);
  isLoading = signal(false);

  constructor() {
    effect(() => {
      const page = this.route.snapshot.queryParams['page'] ?? 1;
      this.loadData(page);
    });
  }

  loadData(page: number) {
    this.isLoading.set(true);
    this.fetcher(page).subscribe(res => {
      this.data.set(res);
      this.isLoading.set(false);
    });
  }

  setPage(page: number) {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { page },
      queryParamsHandling: 'merge'
    });
  }
}
```

### Cursor-paginated table (audit log, alerts, DLP findings)

```tsx
// components/data/cursor-table.tsx
'use client'
import { useInfiniteQuery } from '@tanstack/react-query'
import type { CursorPage } from '@/lib/api/pagination'

interface CursorTableProps<T> {
  queryKey:    unknown[]
  fetcher:     (cursor?: string) => Promise<CursorPage<T>>
  columns:     ColumnDef<T>[]
  emptyMessage?: string
}

export function CursorTable<T extends { id: string }>({
  queryKey, fetcher, columns, emptyMessage,
}: CursorTableProps<T>) {
  const {
    data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) => fetcher(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage: CursorPage<T>) => lastPage.meta.next_cursor ?? undefined,
  })

  const rows = data?.pages.flatMap(p => p.data) ?? []

  return (
    <div className="space-y-4">
      <DataTable
        columns={columns}
        data={rows}
        isLoading={isLoading}
        emptyMessage={emptyMessage}
      />
      {hasNextPage && (
        <button
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="w-full py-2 text-sm text-og-text-secondary hover:text-og-text-primary border border-og-border rounded-md transition-colors"
        >
          {isFetchingNextPage ? 'Loading...' : 'Load more'}
        </button>
      )}
    </div>
  )
}
```

---

## 18.2 Real-Time SSE Table (Audit Stream)

```typescript
// src/app/features/audit/audit-stream.ts
import { Component, signal, inject, OnInit, OnDestroy } from '@angular/core';
import { SseService } from '@/app/core/sse/sse.service';
import { UiService } from '@/app/core/state/ui.service';

@Component({
  selector: 'og-audit-stream',
  templateUrl: './audit-stream.html'
})
export class AuditStreamComponent implements OnInit, OnDestroy {
  private sse = inject(SseService);
  private ui = inject(UiService);
  
  events = signal<SSEAuditEvent[]>([]);
  private disconnect?: () => void;

  ngOnInit() {
    this.disconnect = this.sse.connect('/api/stream/audit', (event) => {
      this.events.update(prev => [event, ...prev].slice(0, 200));
    });
  }

  ngOnDestroy() {
    this.disconnect?.();
  }

  handleRowClick(id: string) {
    this.ui.openDrawer('audit', id);
  }
}
```

---

## 18.3 Optimistic Status Toggle

Used for connector suspend/activate, DLP policy enable/disable:

```typescript
// src/app/features/connectors/status-toggle.ts
@Component({
  selector: 'og-status-toggle',
  template: `
    <button (click)="handleToggle()" [disabled]="isPending()">
      {{ status() === 'active' ? 'Suspend' : 'Activate' }}
    </button>
  `
})
export class StatusToggleComponent {
  connector = input.required<Connector>();
  isPending = signal(false);

  async handleToggle() {
    if (this.connector().status === 'active') {
      const ok = await this.ui.confirm({ ... });
      if (!ok) return;
    }

    this.isPending.set(true);
    this.service.toggleStatus(this.connector().id).subscribe({
      next: () => this.isPending.set(false),
      error: () => {
        this.isPending.set(false);
        this.toast.error('Failed to toggle status');
      }
    });
  }
}
```

---

## 18.4 Polling Status (Report & Export Jobs)

```tsx
// components/domain/job-status-poller.tsx
'use client'

import { useQuery } from '@tanstack/react-query'
import { queryKeys } from '@/lib/query/keys'
import { complianceApi } from '@/lib/api/compliance'
import { toast } from '@/lib/store/notification'
import { useEffect, useRef } from 'react'
import type { ComplianceReport } from '@/types/models'

interface JobStatusPollerProps {
  jobId: string
  orgId: string
  onComplete: (report: ComplianceReport) => void
}

export function JobStatusPoller({ jobId, orgId, onComplete }: JobStatusPollerProps) {
  const notified = useRef(false)

  const { data } = useQuery({
    queryKey:       queryKeys.compliance.report(orgId, jobId),
    queryFn:        () => complianceApi.getReport(orgId, jobId),
    // Poll every 3s while pending/processing; stop on terminal state
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'pending' || status === 'processing' ? 3000 : false
    },
  })

  useEffect(() => {
    if (!data || notified.current) return
    if (data.status === 'completed') {
      notified.current = true
      toast.success('Report ready', {
        action: { label: 'Download', onClick: () => onComplete(data) },
      })
    }
    if (data.status === 'failed') {
      notified.current = true
      toast.error(`Report generation failed: ${data.error ?? 'Unknown error'}`)
    }
  }, [data, onComplete])

  return null  // headless — drives notifications and parent state only
}
```

---

## 18.5 ETag-Aware Form Submission

Used for policy and SCIM resource updates where the BE enforces optimistic concurrency via `If-Match`:

```tsx
// lib/hooks/use-etag-mutation.ts
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef } from 'react'
import { OpenGuardAPIError } from '@/lib/api/client'
import { toast } from '@/lib/store/notification'

interface ETagMutationOptions<TInput, TOutput> {
  queryKey:   unknown[]
  mutationFn: (input: TInput, etag: string) => Promise<TOutput>
  onSuccess?: (data: TOutput) => void
  onConflict?: () => void
}

export function useETagMutation<TInput, TOutput>({
  queryKey, mutationFn, onSuccess, onConflict,
}: ETagMutationOptions<TInput, TOutput>) {
  const qc = useQueryClient()
  // ETag is derived from the current version number stored in query cache
  const getETag = () => {
    const cached = qc.getQueryData<{ version: number }>(queryKey)
    return cached ? `"${cached.version}"` : '*'
  }

  return useMutation({
    mutationFn: (input: TInput) => mutationFn(input, getETag()),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey })
      onSuccess?.(data)
    },
    onError: (err: OpenGuardAPIError) => {
      if (err.code === 'PRECONDITION_FAILED') {
        // 412 — concurrent edit detected
        toast.warning('This resource was modified by someone else. Reload to see the latest version.')
        qc.invalidateQueries({ queryKey })
        onConflict?.()
      }
    },
  })
}
```

---

## 18.6 API Key One-Time Reveal

```tsx
// components/domain/api-key-reveal.tsx
'use client'

import { useState, useEffect } from 'react'
import { CopyButton } from '@/components/ui/copy-button'
import { Icon } from '@/components/ui/icon'
import { Eye, EyeOff, AlertTriangle } from 'lucide-react'

interface APIKeyRevealProps {
  plaintextKey: string   // passed via route state; never stored
  prefix:       string   // first 8 chars; safe to display always
  onAcknowledged: () => void
}

export function APIKeyReveal({ plaintextKey, prefix, onAcknowledged }: APIKeyRevealProps) {
  const [revealed, setRevealed] = useState(false)
  const [acknowledged, setAcknowledged] = useState(false)

  // Warn on page unload if user hasn't acknowledged
  useEffect(() => {
    if (acknowledged) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [acknowledged])

  const maskedKey = `${prefix}${'•'.repeat(24)}`
  const displayKey = revealed ? plaintextKey : maskedKey

  return (
    <div className="rounded-lg border border-og-warning bg-og-warning-dim p-5 space-y-4">
      <div className="flex items-start gap-3">
        <Icon icon={AlertTriangle} size="md" className="text-og-warning mt-0.5 shrink-0" />
        <div>
          <p className="text-og-text-primary text-sm font-body font-medium">
            Save this API key — it will not be shown again
          </p>
          <p className="text-og-text-secondary text-xs mt-1">
            Store it in a secrets manager (e.g. Vault, AWS Secrets Manager).
            Once you close this page, the key cannot be recovered.
          </p>
        </div>
      </div>

      <div className="flex items-center gap-2 bg-og-bg-base rounded-md px-4 py-3 border border-og-border">
        <code className="flex-1 font-mono text-sm text-og-text-primary tracking-wider select-all">
          {displayKey}
        </code>
        <button
          onClick={() => setRevealed(r => !r)}
          className="text-og-text-secondary hover:text-og-text-primary transition-colors"
          aria-label={revealed ? 'Hide key' : 'Reveal key'}
        >
          <Icon icon={revealed ? EyeOff : Eye} size="md" />
        </button>
        <CopyButton value={plaintextKey} />
      </div>

      <div className="text-xs text-og-text-secondary font-mono">
        Prefix: <span className="text-og-accent">{prefix}</span>
        <span className="ml-2 text-og-text-muted">(safe to include in logs)</span>
      </div>

      <button
        onClick={() => { setAcknowledged(true); onAcknowledged() }}
        className="w-full bg-og-accent text-og-bg-base font-body font-medium text-sm py-2 rounded-md hover:bg-og-accent-dim transition-colors"
      >
        I've saved the key securely
      </button>
    </div>
  )
}
```
