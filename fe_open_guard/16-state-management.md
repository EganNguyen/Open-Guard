# §16 — Global State (Zustand Stores)

Zustand is used exclusively for UI state with no server representation. Server data lives in TanStack Query. See §00 State Management Philosophy.

---

## 16.1 UI Store

```ts
// lib/store/ui.ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface UIState {
  // Sidebar
  sidebarCollapsed: boolean
  setSidebarCollapsed: (collapsed: boolean) => void
  toggleSidebar: () => void

  // Global search
  searchOpen: boolean
  setSearchOpen: (open: boolean) => void

  // Active drawer (audit event detail, alert detail)
  activeDrawer: { type: 'audit' | 'alert' | null; id: string | null }
  openDrawer: (type: 'audit' | 'alert', id: string) => void
  closeDrawer: () => void

  // Audit stream
  auditStreamPaused: boolean
  setAuditStreamPaused: (paused: boolean) => void

  // Confirm dialog (imperative)
  confirmDialog: ConfirmDialogState | null
  openConfirm: (state: ConfirmDialogState) => void
  closeConfirm: () => void
  resolveConfirm: ((confirmed: boolean) => void) | null
  setResolveConfirm: (fn: (confirmed: boolean) => void) => void
}

interface ConfirmDialogState {
  title:         string
  description:   string
  confirmLabel?: string
  cancelLabel?:  string
  variant?:      'default' | 'destructive'
  requireTyped?: string    // resource name user must type to confirm
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
      toggleSidebar: () => set(s => ({ sidebarCollapsed: !s.sidebarCollapsed })),

      searchOpen: false,
      setSearchOpen: (open) => set({ searchOpen: open }),

      activeDrawer: { type: null, id: null },
      openDrawer:  (type, id)  => set({ activeDrawer: { type, id } }),
      closeDrawer: ()          => set({ activeDrawer: { type: null, id: null } }),

      auditStreamPaused: false,
      setAuditStreamPaused: (paused) => set({ auditStreamPaused: paused }),

      confirmDialog:    null,
      openConfirm:      (state) => set({ confirmDialog: state }),
      closeConfirm:     ()      => set({ confirmDialog: null, resolveConfirm: null }),
      resolveConfirm:   null,
      setResolveConfirm: (fn)   => set({ resolveConfirm: fn }),
    }),
    {
      name: 'og:ui',
      // Only persist sidebar state across sessions — not dialog / drawer state
      partialize: (state) => ({ sidebarCollapsed: state.sidebarCollapsed }),
    }
  )
)
```

---

## 16.2 Notification Store

```ts
// lib/store/notification.ts
import { create } from 'zustand'
import { nanoid } from 'nanoid'

type ToastType = 'success' | 'error' | 'warning' | 'info' | 'loading'

interface Toast {
  id:         string
  type:       ToastType
  message:    string
  action?:    { label: string; onClick: () => void }
  autoDismissMs: number | null    // null = no auto-dismiss (loading toasts)
}

interface NotificationState {
  toasts: Toast[]
  add: (toast: Omit<Toast, 'id'>) => string
  dismiss: (id: string) => void
  dismissAll: () => void
}

// Auto-dismiss durations (ms) per type
const DURATIONS: Record<ToastType, number | null> = {
  success: 4000,
  error:   8000,
  warning: 6000,
  info:    4000,
  loading: null,
}

export const useNotificationStore = create<NotificationState>((set) => ({
  toasts: [],

  add: (toast) => {
    const id = nanoid()
    set(s => ({
      // Max 3 visible toasts — evict oldest (FIFO)
      toasts: [...s.toasts.slice(-2), { ...toast, id }],
    }))
    if (toast.autoDismissMs !== null) {
      setTimeout(() => {
        set(s => ({ toasts: s.toasts.filter(t => t.id !== id) }))
      }, toast.autoDismissMs)
    }
    return id
  },

  dismiss: (id) => set(s => ({ toasts: s.toasts.filter(t => t.id !== id) })),
  dismissAll: () => set({ toasts: [] }),
}))

// Convenience API (matches interface described in §01 Design System §1.8)
export const toast = {
  success: (message: string, opts?: { action?: Toast['action'] }) =>
    useNotificationStore.getState().add({
      type: 'success', message, autoDismissMs: DURATIONS.success, ...opts,
    }),
  error: (message: string, opts?: { action?: Toast['action'] }) =>
    useNotificationStore.getState().add({
      type: 'error', message, autoDismissMs: DURATIONS.error, ...opts,
    }),
  warning: (message: string) =>
    useNotificationStore.getState().add({
      type: 'warning', message, autoDismissMs: DURATIONS.warning,
    }),
  info: (message: string) =>
    useNotificationStore.getState().add({
      type: 'info', message, autoDismissMs: DURATIONS.info,
    }),
  loading: (message: string) =>
    useNotificationStore.getState().add({
      type: 'loading', message, autoDismissMs: null,
    }),
  dismiss: (id: string) => useNotificationStore.getState().dismiss(id),
}
```

---

## 16.3 `useConfirm` Hook (Imperative Confirm Dialog)

```ts
// lib/hooks/use-confirm.ts
'use client'

import { useUIStore } from '@/lib/store/ui'

interface ConfirmOptions {
  title:         string
  description:   string
  confirmLabel?: string
  cancelLabel?:  string
  variant?:      'default' | 'destructive'
  requireTyped?: string
}

export function useConfirm() {
  const { openConfirm, closeConfirm, setResolveConfirm } = useUIStore()

  return function confirm(options: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
      setResolveConfirm((confirmed: boolean) => {
        closeConfirm()
        resolve(confirmed)
      })
      openConfirm(options)
    })
  }
}

// Usage:
//   const confirm = useConfirm()
//
//   const handleSuspend = async () => {
//     const ok = await confirm({
//       title: 'Suspend connector?',
//       description: `${connector.name} will immediately lose API access.`,
//       confirmLabel: 'Suspend',
//       variant: 'destructive',
//       requireTyped: connector.name,
//     })
//     if (!ok) return
//     await suspendConnector.mutateAsync(connector.id)
//   }
```

---

## 16.4 TanStack Query Client

```ts
// lib/query/client.ts
import { QueryClient } from '@tanstack/react-query'
import { OpenGuardAPIError } from '@/lib/api/client'

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // Data is considered fresh for 60s — matches BE policy cache TTL
        staleTime: 60 * 1000,
        // On window focus, refetch if data is stale
        refetchOnWindowFocus: true,
        // Retry once on failure; skip retry for 4xx (client errors are not transient)
        retry: (failureCount, error) => {
          if (error instanceof OpenGuardAPIError && !error.retryable) return false
          return failureCount < 1
        },
        // Global error handler — surfaces errors to the notification store
        // Note: individual queries can override this
      },
      mutations: {
        // Mutations do not retry by default (idempotency is caller's responsibility)
        retry: false,
      },
    },
  })
}

// Singleton for client components
let browserQueryClient: QueryClient | undefined

export function getQueryClient() {
  if (typeof window === 'undefined') {
    // Server: always create a new client (no singleton leakage between requests)
    return makeQueryClient()
  }
  if (!browserQueryClient) {
    browserQueryClient = makeQueryClient()
  }
  return browserQueryClient
}
```

```tsx
// app/layout.tsx — Providers wrapper
'use client'

import { QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { getQueryClient } from '@/lib/query/client'

export function Providers({ children }: { children: React.ReactNode }) {
  const queryClient = getQueryClient()
  return (
    <QueryClientProvider client={queryClient}>
      {children}
      {process.env.NODE_ENV === 'development' && <ReactQueryDevtools />}
    </QueryClientProvider>
  )
}
```

---

## 16.5 URL State (Filters & Pagination)

Filter state is always synced to the URL — users can bookmark filtered views and share links. Uses `nuqs` for type-safe URL search parameter management.

```ts
// lib/hooks/use-audit-filters.ts
import { useQueryStates, parseAsString, parseAsArrayOf, parseAsIsoDateTime } from 'nuqs'

export function useAuditFilters() {
  return useQueryStates({
    type:       parseAsArrayOf(parseAsString).withDefault([]),
    actor_type: parseAsArrayOf(parseAsString).withDefault([]),
    source:     parseAsArrayOf(parseAsString).withDefault([]),
    actor_id:   parseAsString.withDefault(''),
    from:       parseAsIsoDateTime,
    to:         parseAsIsoDateTime,
    cursor:     parseAsString,
  })
}

// lib/hooks/use-alert-filters.ts
export function useAlertFilters() {
  return useQueryStates({
    status:   parseAsString.withDefault('open'),
    severity: parseAsArrayOf(parseAsString).withDefault([]),
    detector: parseAsString.withDefault(''),
    cursor:   parseAsString,
  })
}
```

**Rule:** When filters change, the cursor is always reset to `undefined` (start from the first page of the new filter result set). This is enforced in the filter panel's `onApply` handler:

```ts
const [filters, setFilters] = useAuditFilters()

function applyFilters(newFilters: Partial<typeof filters>) {
  setFilters({ ...newFilters, cursor: null })  // always reset cursor on filter change
}
```
