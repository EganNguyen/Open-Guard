# §04 — Dashboard Layout & Navigation

---

## 4.1 App Shell

In Angular, the App Shell is a standalone component that wraps the main `router-outlet`. Access control is handled via the `authGuard`.

```typescript
// src/app/layout/app-shell/app-shell.component.ts
import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { SidebarComponent } from '../sidebar/sidebar-component';
import { TopbarComponent } from '../topbar/topbar-component';

@Component({
  selector: 'og-app-shell',
  standalone: true,
  imports: [RouterOutlet, SidebarComponent, TopbarComponent],
  template: `
    <div class="flex h-screen overflow-hidden">
      <og-sidebar />
      <div class="relative flex flex-1 flex-col overflow-y-auto overflow-x-hidden">
        <og-topbar />
        <main class="mx-auto max-w-screen-2xl p-4 md:p-6 2xl:p-10">
          <router-outlet />
        </main>
      </div>
    </div>
  `
})
export class AppShellComponent {}
```

**AppShell structure:**
```
┌─────────────────────────────────────────────────────────┐
│ Topbar (64px, fixed)                                    │
│  [≡ OpenGuard]  [Breadcrumbs]        [Search] [Bell] [Avatar] │
├──────────┬──────────────────────────────────────────────┤
│ Sidebar  │ Main content area                            │
│ (240px)  │ (scrollable, max-w-screen-2xl mx-auto px-6) │
│          │                                              │
│          │                                              │
└──────────┴──────────────────────────────────────────────┘
```

---

## 4.2 Sidebar Navigation

```tsx
// components/layout/sidebar.tsx
// "use client" — manages collapse state
```

**Navigation structure:**

```
OpenGuard
  [org name badge]       ← OrgSwitcher component

OVERVIEW
  ○ Overview             → /overview

SECURITY
  ○ Connectors           → /connectors
  ○ Policies             → /policies
  ○ Audit Log            → /audit
  ○ Threats & Alerts     → /threats
    └ [N] badge if unacknowledged alerts > 0

COMPLIANCE
  ○ Compliance           → /compliance
  ○ DLP                  → /dlp

IDENTITY
  ○ Users                → /users
  ○ Org Settings         → /org/settings

ADMIN  ← only if user has admin role
  ○ System Health        → /admin/system
```

**Active state:** Current route gets `bg-og-bg-elevated text-og-text-primary` background. Inactive: `text-og-text-secondary hover:text-og-text-primary hover:bg-og-bg-elevated/50`.

**Unread badge on Threats:** Derived from `useQuery(queryKeys.threats.alerts(orgId, { status: 'open' }))` — count of unacknowledged alerts. Refreshes every 30s.

**Collapse behavior:**
- Desktop (lg+): sidebar always visible, collapsible to 56px icon-only mode via toggle button at bottom.
- Mobile (<lg): sidebar is an off-canvas drawer triggered by the hamburger in the topbar.
- Collapse state persisted to `localStorage` key `og:sidebar:collapsed`.

---

## 4.3 Topbar

```tsx
// components/layout/topbar.tsx
```

**Left:** Hamburger (mobile only) + "OpenGuard" wordmark in `font-display`.

**Center:** `<Breadcrumbs />` — auto-generated from the current route path with human-readable labels (e.g., `Connectors / AcmeApp / Delivery Log`).

**Right:**
1. `<GlobalSearch />` — keyboard shortcut `⌘K` / `Ctrl+K`.
2. `<NotificationBell />` — count of unread system notifications.
3. `<UserAvatar />` — dropdown with: Profile, Org Settings, Sign Out.

---

## 4.4 Global Search

A command palette component. Uses keyboard shortcut `⌘K` to open.

**Implementation:** A standalone Angular component that uses a debounced (300ms) input signal to trigger search across multiple services.

**Search scopes:**
- Users (by email, display name)
- Connectors (by name)
- Audit events (by event ID, actor ID)
- Policies (by name)
- Threat alerts (by alert ID, description)

**Implementation:** Debounced (300ms) calls to each resource's list endpoint with a `q=` query param. Results are grouped by scope with icons. Selecting a result navigates to the detail page.

**Keyboard shortcuts displayed in search:**
- `G then C` → Go to Connectors
- `G then P` → Go to Policies
- `G then A` → Go to Audit Log
- `G then T` → Go to Threats

---

## 4.5 Overview Page

```
Route: /overview
```

A summary dashboard showing the org's security posture at a glance.

**Metric cards (top row):**
- Total connectors (active / suspended)
- Unacknowledged threat alerts (breakdown by severity)
- Events ingested in last 24h (from `audit/stats`)
- Compliance score (from `compliance/posture`)
- Active users / locked users

**Charts (middle row):**
- Events/day (last 14 days) — BarChart from ClickHouse `event_counts_daily`
- Alert severity distribution (last 7 days) — BarChart
- Policy evaluation cache hit rate (last 24h) — LineChart

**Recent activity (bottom):**
- Last 10 audit events (live, refreshes every 15s)
- Last 5 unacknowledged alerts

**Data sources:**
- All charts use `useQuery` with `refetchInterval: 60_000` (1 minute).
- Recent activity uses `useQuery` with `refetchInterval: 15_000`.
- Overview page does NOT use SSE — polling is sufficient and avoids connection overhead for infrequent viewers.

---

## 4.6 Org Switcher

For users with admin access to multiple organizations (super-admins only):

```tsx
// components/layout/org-switcher.tsx
// Shows current org name in the sidebar header.
// If user has multiple orgs: renders a dropdown with org list.
// Switching org: calls signIn() to re-initiate OIDC with the target org context.
// The new access token will carry the new org_id claim.
//
// Note: org_id is always derived from the JWT claim (BE spec §2.8).
// The OrgSwitcher never writes org_id to localStorage or cookies directly.
```

---

## 4.7 Breadcrumbs

The `<Breadcrumbs />` component subscribes to `Router` events to extract segment labels from the route configuration.

```typescript
// src/app/layout/breadcrumbs/breadcrumbs.component.ts
export class BreadcrumbsComponent {
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  breadcrumbs = signal<string[]>([]);

  // Subscription logic to extract breadcrumbs from nested ActivatedRoute
}
```

---

## 4.8 Error Handling

Angular handles global errors via an `ErrorHandler` provider.

```typescript
// src/app/core/errors/global-error-handler.ts
import { ErrorHandler, Injectable } from '@angular/core';

@Injectable()
export class GlobalErrorHandler implements ErrorHandler {
  handleError(error: any): void {
    console.error('Global Error:', error);
    // Surface to UI via NotificationService or redirect to 500
  }
}
```
