# §01 — Design System

OpenGuard's visual language is **industrial-minimal**: dense, information-rich, and purposeful — the aesthetic of a security operations center, not a marketing site. High contrast. Monospaced data. Status communicated through color with precision. Zero decorative elements that don't carry information.

---

## 1.1 Color Palette

```css
/* tailwind.config.ts — extend colors */
:root {
  /* Backgrounds */
  --og-bg-base:       #09090B;   /* zinc-950 — page background */
  --og-bg-surface:    #111113;   /* zinc-900 — card / panel background */
  --og-bg-elevated:   #18181B;   /* zinc-800 — hover state / nested panel */
  --og-bg-overlay:    #27272A;   /* zinc-700 — tooltip, popover */

  /* Borders */
  --og-border:        #27272A;   /* zinc-700 */
  --og-border-subtle: #1C1C1F;   /* between surface and base */

  /* Text */
  --og-text-primary:  #FAFAFA;   /* zinc-50 */
  --og-text-secondary:#A1A1AA;   /* zinc-400 */
  --og-text-muted:    #52525B;   /* zinc-600 */
  --og-text-disabled: #3F3F46;   /* zinc-700 */

  /* Brand accent — electric cyan */
  --og-accent:        #06B6D4;   /* cyan-500 */
  --og-accent-dim:    #0E7490;   /* cyan-700 */
  --og-accent-glow:   rgba(6,182,212,0.12);

  /* Semantic: status */
  --og-success:       #22C55E;   /* green-500 */
  --og-success-dim:   rgba(34,197,94,0.12);
  --og-warning:       #F59E0B;   /* amber-500 */
  --og-warning-dim:   rgba(245,158,11,0.12);
  --og-danger:        #EF4444;   /* red-500 */
  --og-danger-dim:    rgba(239,68,68,0.12);
  --og-critical:      #FF2056;   /* custom hot red — CRITICAL alerts only */
  --og-info:          #3B82F6;   /* blue-500 */

  /* Severity scale — threat levels */
  --og-sev-low:       #6B7280;   /* gray-500 */
  --og-sev-medium:    #F59E0B;
  --og-sev-high:      #EF4444;
  --og-sev-critical:  #FF2056;
}
```

**Light mode:** OpenGuard ships dark mode only in v1. Light mode is a Phase 2 stretch goal. The dashboard is used by security teams in dimly lit NOC environments.

---

## 1.2 Typography

```ts
// tailwind.config.ts
fontFamily: {
  // Display: used for headings, page titles, metric numbers
  display: ['"JetBrains Mono"', 'monospace'],
  // Body: used for all prose, labels, descriptions
  body: ['"IBM Plex Sans"', 'sans-serif'],
  // Mono: used for IDs, hashes, API keys, code, IP addresses, trace IDs
  mono: ['"JetBrains Mono"', 'monospace'],
}
```

**Load via `next/font/google`:**

```tsx
// app/layout.tsx
import { JetBrains_Mono, IBM_Plex_Sans } from 'next/font/google'

const jetbrainsMono = JetBrains_Mono({
  subsets: ['latin'],
  variable: '--font-display',
  weight: ['400', '500', '700'],
})
const ibmPlexSans = IBM_Plex_Sans({
  subsets: ['latin'],
  variable: '--font-body',
  weight: ['400', '500', '600'],
})
```

**Type scale (Tailwind classes only — no arbitrary values):**

| Usage | Class |
|---|---|
| Page title | `text-2xl font-display font-bold tracking-tight` |
| Section heading | `text-lg font-display font-medium` |
| Card title | `text-base font-display font-medium` |
| Body / paragraph | `text-sm font-body` |
| Label / meta | `text-xs font-body text-og-text-secondary` |
| Metric / big number | `text-3xl font-display font-bold tabular-nums` |
| Code / ID / hash | `text-xs font-mono text-og-text-secondary` |
| Badge / status | `text-xs font-body font-medium uppercase tracking-wide` |

---

## 1.3 Spacing Scale

Use Tailwind's default 4px base. All internal padding/margin uses the scale; no arbitrary values.

| Context | Value |
|---|---|
| Page container max-width | `max-w-screen-2xl mx-auto px-6` |
| Card padding | `p-5` |
| Section gap | `gap-6` |
| Inline element gap | `gap-2` |
| Form field gap | `gap-4` |
| Table cell padding | `px-4 py-3` |

---

## 1.4 Core UI Primitives (`components/ui/`)

These are Radix UI primitives styled to the OpenGuard design language. Each is a standalone file.

### Button

```tsx
// components/ui/button.tsx
// Variants: 'default' | 'destructive' | 'outline' | 'ghost' | 'link'
// Sizes: 'sm' | 'md' | 'lg' | 'icon'
//
// 'default'     → accent background, dark text
// 'destructive' → danger background
// 'outline'     → border only, no background fill
// 'ghost'       → no border, hover fill
//
// Loading state: accepts `loading` prop → shows spinner, disables interaction
// All buttons use a 150ms transition on background-color and box-shadow.
```

### Badge

```tsx
// components/ui/badge.tsx
// Variants: 'success' | 'warning' | 'danger' | 'critical' | 'info' | 'muted'
// Used for: connector status, user status, alert severity, policy version
//
// Renders: colored dot + label in monospace uppercase
// <Badge variant="danger">SUSPENDED</Badge>
// <Badge variant="critical">CRITICAL</Badge>
```

### StatusDot

```tsx
// components/ui/status-dot.tsx
// A pulsing dot indicator for live connection status.
// <StatusDot status="live" />    → cyan, pulse animation
// <StatusDot status="healthy" /> → green, no pulse
// <StatusDot status="degraded" /> → amber
// <StatusDot status="down" />    → red
```

### DataTable

```tsx
// components/data/data-table.tsx
// Built on TanStack Table v8.
//
// Required props:
//   columns: ColumnDef<T>[]
//   data: T[]
//   isLoading?: boolean
//   emptyMessage?: string
//
// Optional props:
//   onRowClick?: (row: T) => void   → entire row is clickable
//   pagination?: PaginationState    → controlled pagination
//   onPaginationChange?: ...
//
// Features:
//   - Skeleton rows during loading (8 rows, animated shimmer)
//   - Empty state with icon and message
//   - Sticky header
//   - Column sorting (client-side for small datasets, server-side flag for large)
//   - Row hover highlight
```

### ConfirmDialog

```tsx
// components/feedback/confirm-dialog.tsx
// Used for all destructive actions.
//
// Usage via imperative hook:
//   const confirm = useConfirm()
//   await confirm({
//     title: 'Suspend connector?',
//     description: 'AcmeApp will immediately lose access.',
//     confirmLabel: 'Suspend',
//     variant: 'destructive',
//     // requireTyped: 'AcmeApp'  → user must type the name to confirm
//   })
```

### Redactable

```tsx
// components/data/redactable.tsx
// Respects org-level data visibility settings.
// When org.data_visibility === 'restricted', renders masked value with reveal toggle.
//
// <Redactable value="user@example.com" type="email" />
// <Redactable value="192.168.1.1" type="ip" />
// <Redactable value="sk_live_abc..." type="api-key" />
//
// Reveal is per-session and logged as a 'data.access' audit event via POST /v1/events/ingest.
```

### CopyButton

```tsx
// components/ui/copy-button.tsx
// Copies value to clipboard. Shows checkmark for 2s on success.
// Used alongside any ID, hash, or API key display.
```

### TimeAgo

```tsx
// components/ui/time-ago.tsx
// Renders relative time ("3 minutes ago") with ISO tooltip on hover.
// Updates every 30s using useEffect + setInterval (cleanup-safe).
// <TimeAgo date={event.occurred_at} />
```

---

## 1.5 Motion System

All animation uses Framer Motion. Use sparingly — only for interactions that carry meaningful state transitions.

```tsx
// lib/motion.ts — shared animation variants

export const fadeIn = {
  initial: { opacity: 0 },
  animate: { opacity: 1, transition: { duration: 0.15 } },
  exit:    { opacity: 0, transition: { duration: 0.1 } },
}

export const slideUp = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.2, ease: 'easeOut' } },
  exit:    { opacity: 0, y: 8, transition: { duration: 0.1 } },
}

export const staggerChildren = {
  animate: { transition: { staggerChildren: 0.05 } },
}
```

**Permitted uses:**
- Page-level route transitions: `slideUp` on the main content area.
- Drawer / sheet open/close: `x` translate from right.
- Toast notifications: `slideUp` + `fadeIn`.
- Badge pulse for CRITICAL alerts: CSS `animate-ping` (not Framer Motion — performance-critical).
- List item entrance: `staggerChildren` on first load only (not on every refetch).

**Forbidden uses:**
- Animating table rows on every data update (causes layout thrash at 50k events/s).
- Continuous loops on data cells.
- Transitions longer than 300ms on any interactive element.

---

## 1.6 Iconography

Use Lucide React exclusively. All icons rendered via a typed wrapper:

```tsx
// components/ui/icon.tsx
import { LucideIcon } from 'lucide-react'

interface IconProps {
  icon: LucideIcon
  size?: 'sm' | 'md' | 'lg'
  className?: string
}

const sizes = { sm: 14, md: 16, lg: 20 }

export function Icon({ icon: LIcon, size = 'md', className }: IconProps) {
  return <LIcon size={sizes[size]} className={className} strokeWidth={1.5} />
}
```

**Icon vocabulary (canonical — do not use alternates):**

| Concept | Icon |
|---|---|
| Connector / App | `Plug2` |
| Policy | `Shield` |
| Audit log | `ScrollText` |
| Threat / Alert | `AlertTriangle` |
| Compliance | `ClipboardCheck` |
| DLP | `ScanLine` |
| User | `User` |
| Org settings | `Settings2` |
| System / Admin | `Terminal` |
| Suspend | `PauseCircle` |
| Activate | `PlayCircle` |
| Revoke | `XCircle` |
| Delete | `Trash2` |
| Copy | `Copy` |
| Webhook | `Webhook` |
| MFA / Security | `KeyRound` |
| API Key | `Hash` |
| Circuit breaker | `Zap` |
| Kafka / Queue | `Layers` |
| Hash chain | `Link` |
| Export | `Download` |

---

## 1.7 Layout Grid

```tsx
// The dashboard uses a fixed sidebar + flexible main area layout.
// Sidebar: 240px fixed width (collapsible to 56px icon-only on mobile)
// Main: flex-1, scrollable, padded

// Responsive breakpoints (Tailwind):
// sm:  640px — tablet portrait
// md:  768px — tablet landscape
// lg: 1024px — desktop (sidebar becomes persistent)
// xl: 1280px — wide desktop
// 2xl:1536px — max content width cap
```

---

## 1.8 Toast / Notification System

```tsx
// Uses a Zustand store + fixed-position toast container (top-right).
// Max 3 visible toasts. FIFO eviction.
//
// Types:
//   toast.success(message)   → green, 4s auto-dismiss
//   toast.error(message)     → red, 8s auto-dismiss (longer — errors need reading time)
//   toast.warning(message)   → amber, 6s
//   toast.info(message)      → blue, 4s
//   toast.loading(message)   → spinner, no auto-dismiss → must be dismissed via toast.dismiss(id)
//
// For long-running operations (report generation, bulk exports):
//   const id = toast.loading('Generating GDPR report...')
//   // on completion:
//   toast.dismiss(id)
//   toast.success('Report ready — click to download', { action: { label: 'Download', onClick: ... } })
```
