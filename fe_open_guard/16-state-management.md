# §16 — Global State (Signals & Services)

In Angular, we use **Signals** within **Injectable Services** for UI-only state. Server data lives in Services that wrap `HttpClient`.

---

## 16.1 UI Service

```typescript
// src/app/core/state/ui.service.ts
import { Injectable, signal, computed, effect } from '@angular/core';

@Injectable({ providedIn: 'root' })
export class UiService {
  // Sidebar
  readonly sidebarCollapsed = signal(
    JSON.parse(localStorage.getItem('og:ui:sidebar') ?? 'false')
  );

  constructor() {
    // Persist sidebar state
    effect(() => {
      localStorage.setItem('og:ui:sidebar', JSON.stringify(this.sidebarCollapsed()));
    });
  }

  toggleSidebar() {
    this.sidebarCollapsed.update(v => !v);
  }

  // Active drawer (audit event detail, alert detail)
  readonly activeDrawer = signal<{ type: 'audit' | 'alert' | null; id: string | null }>({
    type: null,
    id: null
  });

  openDrawer(type: 'audit' | 'alert', id: string) {
    this.activeDrawer.set({ type, id });
  }

  closeDrawer() {
    this.activeDrawer.set({ type: null, id: null });
  }

  // Confirm dialog state
  readonly confirmDialog = signal<ConfirmDialogState | null>(null);
  
  private resolveConfirm?: (confirmed: boolean) => void;

  async confirm(options: ConfirmDialogState): Promise<boolean> {
    this.confirmDialog.set(options);
    return new Promise((resolve) => {
      this.resolveConfirm = (confirmed) => {
        this.confirmDialog.set(null);
        resolve(confirmed);
      };
    });
  }

  handleConfirm(confirmed: boolean) {
    this.resolveConfirm?.(confirmed);
  }
}
```

---

## 16.2 Notification Service

```typescript
// src/app/core/state/notification.service.ts
import { Injectable, signal } from '@angular/core';

export type ToastType = 'success' | 'error' | 'warning' | 'info' | 'loading';

@Injectable({ providedIn: 'root' })
export class NotificationService {
  readonly toasts = signal<Toast[]>([]);

  success(message: string) {
    this.add({ type: 'success', message, autoDismissMs: 4000 });
  }

  error(message: string) {
    this.add({ type: 'error', message, autoDismissMs: 8000 });
  }

  private add(toast: Omit<Toast, 'id'>) {
    const id = crypto.randomUUID();
    this.toasts.update(list => [...list.slice(-2), { ...toast, id }]);
    
    if (toast.autoDismissMs) {
      setTimeout(() => this.dismiss(id), toast.autoDismissMs);
    }
    return id;
  }

  dismiss(id: string) {
    this.toasts.update(list => list.filter(t => t.id !== id));
  }
}
```

---

## 16.3 `confirm` Usage

In Angular, you simply inject the `UiService` and call the async `confirm` method.

```typescript
// src/app/features/connectors/connector-list.ts
export class ConnectorListComponent {
  private ui = inject(UiService);

  async handleSuspend(connector: Connector) {
    const ok = await this.ui.confirm({
      title: 'Suspend connector?',
      description: `${connector.name} will immediately lose API access.`,
      confirmLabel: 'Suspend',
      variant: 'destructive'
    });
    
    if (ok) {
      this.suspendConnector(connector.id);
    }
  }
}
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

## 16.5 URL State

In Angular, we use the `Router` and `ActivatedRoute` to sync state to the URL.

```typescript
// src/app/features/audit/audit-list.ts
import { Router, ActivatedRoute } from '@angular/router';

export class AuditListComponent {
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  updateFilters(newFilters: AuditFilters) {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { ...newFilters, cursor: null },
      queryParamsHandling: 'merge'
    });
  }
}
```
