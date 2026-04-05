# §18 — Component Implementation Patterns

Canonical patterns for building the most complex and reused component types in the dashboard.

---

## 18.1 Data Table with Server-Side Pagination

The pattern for all paginated list pages (connectors, users, policies, etc.):

```tsx
// components/data/paginated-table.tsx — generic wrapper
'use client'

import { useQuery } from '@tanstack/react-query'
import { useSearchParams, useRouter, usePathname } from 'next/navigation'
import { DataTable } from '@/components/data/data-table'
import { PaginationControls } from '@/components/data/pagination-controls'
import type { ColumnDef } from '@tanstack/react-table'
import type { OffsetPage } from '@/lib/api/pagination'

interface PaginatedTableProps<T> {
  queryKey:    unknown[]
  fetcher:     (page: number) => Promise<OffsetPage<T>>
  columns:     ColumnDef<T>[]
  emptyMessage?: string
}

export function PaginatedTable<T extends { id: string }>({
  queryKey, fetcher, columns, emptyMessage,
}: PaginatedTableProps<T>) {
  const searchParams = useSearchParams()
  const router = useRouter()
  const pathname = usePathname()
  const page = Number(searchParams.get('page') ?? '1')

  const { data, isLoading, isError } = useQuery({
    queryKey: [...queryKey, page],
    queryFn:  () => fetcher(page),
  })

  function setPage(p: number) {
    const params = new URLSearchParams(searchParams.toString())
    params.set('page', String(p))
    router.push(`${pathname}?${params.toString()}`)
  }

  if (isError) {
    return <div className="text-og-danger text-sm p-4">Failed to load data. Please refresh.</div>
  }

  return (
    <div className="space-y-4">
      <DataTable
        columns={columns}
        data={data?.data ?? []}
        isLoading={isLoading}
        emptyMessage={emptyMessage}
      />
      {data && (
        <PaginationControls
          page={data.meta.page}
          totalPages={data.meta.total_pages}
          total={data.meta.total}
          onPageChange={setPage}
        />
      )}
    </div>
  )
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

```tsx
// app/(dashboard)/audit/page.tsx (client section)
'use client'

import { useState, useCallback } from 'react'
import { useSSE } from '@/lib/hooks/use-sse'
import { useUIStore } from '@/lib/store/ui'
import type { SSEAuditEvent } from '@/types/events'
import { auditColumns } from './columns'
import { DataTable } from '@/components/data/data-table'

const MAX_LIVE_EVENTS = 200

export function AuditStreamTable() {
  const [events, setEvents] = useState<SSEAuditEvent[]>([])
  const paused = useUIStore(s => s.auditStreamPaused)

  const handleMessage = useCallback((event: SSEAuditEvent) => {
    setEvents(prev => {
      const next = [event, ...prev]
      // Cap buffer at 200 — newest N retained (FIFO eviction of oldest)
      return next.slice(0, MAX_LIVE_EVENTS)
    })
  }, [])

  const { connected } = useSSE<SSEAuditEvent>({
    url:       '/api/stream/audit',
    onMessage: handleMessage,
    enabled:   !paused,
  })

  return (
    <div>
      <StreamStatusBar connected={connected} paused={paused} eventCount={events.length} />
      <DataTable
        columns={auditColumns}
        data={events}
        isLoading={false}
        emptyMessage={paused ? 'Stream paused. Apply filters to view historical events.' : 'Waiting for events…'}
        onRowClick={(row) => useUIStore.getState().openDrawer('audit', row.id)}
      />
    </div>
  )
}
```

---

## 18.3 Optimistic Status Toggle

Used for connector suspend/activate, DLP policy enable/disable — all status toggles that need immediate UI feedback:

```tsx
// components/domain/status-toggle.tsx
'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useConfirm } from '@/lib/hooks/use-confirm'
import { toast } from '@/lib/store/notification'
import type { Connector } from '@/types/models'
import type { OffsetPage } from '@/lib/api/pagination'

interface ConnectorStatusToggleProps {
  connector: Connector
  queryKey:  unknown[]
}

export function ConnectorStatusToggle({ connector, queryKey }: ConnectorStatusToggleProps) {
  const qc = useQueryClient()
  const confirm = useConfirm()

  const toggle = useMutation({
    mutationFn: async () => {
      if (connector.status === 'active') {
        return connectorsApi.suspend(connector.org_id, connector.id)
      }
      return connectorsApi.activate(connector.org_id, connector.id)
    },

    onMutate: async () => {
      await qc.cancelQueries({ queryKey })
      const prev = qc.getQueryData<OffsetPage<Connector>>(queryKey)
      // Optimistic update — flip status immediately
      qc.setQueryData(queryKey, (old: OffsetPage<Connector>) => ({
        ...old,
        data: old.data.map(c =>
          c.id === connector.id
            ? { ...c, status: connector.status === 'active' ? 'suspended' : 'active' }
            : c
        ),
      }))
      return { prev }
    },

    onError: (_err, _vars, ctx) => {
      qc.setQueryData(queryKey, ctx?.prev)
      toast.error(`Failed to ${connector.status === 'active' ? 'suspend' : 'activate'} connector`)
    },

    onSettled: () => qc.invalidateQueries({ queryKey }),
  })

  const handleToggle = async () => {
    if (connector.status === 'active') {
      const ok = await confirm({
        title:        `Suspend ${connector.name}?`,
        description:  'This connector will immediately lose API access. Cached auth tokens will be invalidated within 30 seconds.',
        confirmLabel: 'Suspend',
        variant:      'destructive',
        requireTyped: connector.name,
      })
      if (!ok) return
    }
    toggle.mutate()
  }

  return (
    <button
      onClick={handleToggle}
      disabled={toggle.isPending}
      className={connector.status === 'active' ? 'text-og-danger' : 'text-og-success'}
    >
      {connector.status === 'active' ? 'Suspend' : 'Activate'}
    </button>
  )
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
