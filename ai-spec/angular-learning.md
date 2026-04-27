# Angular Frontend Mastery Guide
### Intermediate → Production-Ready Engineer

> Built from the **OpenGuard** production codebase  
> Stack: Angular 19 · Signals · Standalone · TailwindCSS 4 · Playwright · TypeScript strict

---

## How To Use This Guide

Every topic follows this structure: **what it is → why it matters → real-world usage → common mistakes**.

Three tiers throughout:
- **MUST-KNOW** — Required to ship production code without causing fires
- **SHOULD-KNOW** — Patterns that separate senior engineers from intermediate ones
- **NICE-TO-KNOW** — Advanced optimisations for large-scale systems

Sections marked with **★** contain patterns extracted directly from the OpenGuard codebase.

---

# Part 1 — Core Web Foundations

## 1.1 Browser Rendering Pipeline

**MUST-KNOW** — Every Angular performance problem ultimately bottlenecks here.

When a user navigates to your app, the browser runs the **critical rendering path**:

```
Parse HTML → Build DOM → Parse CSS → Build CSSOM → Render Tree → Layout → Paint → Composite
```

Angular's job is to update the DOM efficiently. Understanding what triggers **reflow** vs **repaint** is essential.

### What causes Reflow (expensive)
- Reading layout properties after mutations: `offsetWidth`, `getBoundingClientRect`, `scrollTop`
- Changing `width`, `height`, `margin`, `padding`, `font-size`, `position`
- Adding/removing DOM nodes that affect document flow
- **Angular implication:** avoid host element mutations; use CSS `transform` instead of `top`/`left` for animation

### What causes only Repaint (cheap)
- Changing `color`, `background-color`, `visibility`, `outline`
- CSS `opacity` and `transform` — GPU-accelerated compositing, no layout recalculation

> **★ Real-World Lesson from OpenGuard**
>
> The audit stream table renders up to 200 events. Early implementations used `*ngFor` with inline style changes for row highlighting — this caused constant reflows at 50 events/second. The fix: use Tailwind classes for status colours, CSS `transform` for animations. The rule is now CI-enforced: *"Animating table rows on every data update causes layout thrash at 50k events/s."*

---

## 1.2 The JavaScript Event Loop

**MUST-KNOW** — Understanding this is the difference between async code that works and async code that *sometimes* works.

JavaScript is single-threaded. The event loop processes tasks from three queues:

| Queue | What goes here | Priority |
|---|---|---|
| Call Stack | Synchronous code | Immediate |
| Microtask Queue | Promises, `queueMicrotask`, `MutationObserver` | After every task, before repaint |
| Macrotask Queue | `setTimeout`, `setInterval`, I/O, UI events | Next iteration |

**Microtasks drain completely after every macrotask** — all `.then()` callbacks run before the next `setTimeout`, before the browser repaints.

### Angular Change Detection and the Event Loop

Zone.js monkey-patches `setTimeout`, `setInterval`, `Promise`, XHR, and WebSocket. Every time you call these, Zone.js notifies Angular to run change detection. This is why removing Zone.js with **zoneless mode** dramatically reduces unnecessary CD cycles.

```typescript
// Traditional (Zone.js) — CD runs after every setTimeout
setTimeout(() => { this.count++; }, 1000); // triggers CD automatically

// Zoneless (Angular 18+) — Signals handle reactivity
this.count.update(c => c + 1); // no Zone.js needed — Signal notifies only affected views
```

---

## 1.3 HTTP, Caching, and CDN

**MUST-KNOW** for any app that talks to an API.

HTTP/2 multiplexes requests over a single TCP connection — no more domain sharding hacks. Angular's `HttpClient` handles HTTP/2 and HTTP/3 transparently.

### Cache-Control Strategy for Angular Apps

| Resource | Cache-Control | Reason |
|---|---|---|
| `index.html` | `no-cache, no-store` | Must always be fresh — contains hashed asset paths |
| `main.[hash].js` | `max-age=31536000, immutable` | Hash changes on every build — safe to cache forever |
| `assets/fonts/*` | `max-age=31536000, immutable` | Fonts never change once published |
| API responses | `ETag` + `stale-while-revalidate` | OpenGuard uses 60s stale time in the service layer |
| `/assets/config.json` | `no-cache` | Runtime config — must be fresh on every deploy |

### ETag and Optimistic Concurrency

OpenGuard uses `ETag` headers for optimistic concurrency on policy and SCIM resource updates. The frontend reads the `version` field from the cached response, formats it as an ETag string, and sends it in the `If-Match` header on PUT requests. A `412 Precondition Failed` means a concurrent edit occurred — the UI warns the user and refetches.

---

## 1.4 JavaScript Deep Fundamentals

**MUST-KNOW** — These gaps silently cause bugs in Angular services and RxJS pipelines.

### Closures

A closure is a function that captures variables from its outer scope. Every Angular component method, every RxJS operator callback, every `setTimeout` handler is a closure.

```typescript
// Common Angular bug: stale closure in interval
ngOnInit() {
  const count = this.count(); // captures value at creation time
  setInterval(() => console.log(count), 1000); // always logs initial value!

  // FIX: read the signal inside the callback
  setInterval(() => console.log(this.count()), 1000);
}
```

### Memory Management and Leak Prevention

Memory leaks in Angular almost always come from subscriptions not cleaned up. The canonical patterns:

```typescript
// ✅ toSignal() — auto-unsubscribes when component is destroyed
events = toSignal(this.sseService.stream('/audit'), { initialValue: [] });

// ✅ async pipe — subscribes and auto-unsubscribes
// <div *ngIf="connectors$ | async as connectors">

// ✅ takeUntilDestroyed — when you must subscribe manually
this.service.list().pipe(
  takeUntilDestroyed(this.destroyRef)
).subscribe(...);

// ❌ Never use setInterval directly — use RxJS interval() which you can unsubscribe
```

> **★ OpenGuard Rule (CI-Enforced)**
>
> *"No manual subscriptions for data — use async pipe or `toSignal()`. No polling with `setInterval` — use RxJS `timer()` or signal-based polling."*

---

## 1.5 TypeScript Mastery for Large-Scale Apps

**MUST-KNOW** — TypeScript is your first line of defence against production bugs.

OpenGuard enforces `strict: true`, `noUncheckedIndexedAccess: true`, and `exactOptionalPropertyTypes: true`. These are not style preferences — they catch entire categories of runtime null-pointer exceptions at compile time.

### The Strict Flags That Matter

```json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true,
    "noImplicitReturns": true
  }
}
```

**`noUncheckedIndexedAccess`** changes `array[0]` from returning `T` to `T | undefined`:

```typescript
// Without noUncheckedIndexedAccess — compiles but crashes at runtime
const first = connectors[0].name; // TypeError if array is empty!

// With noUncheckedIndexedAccess — TypeScript forces you to handle it
const first = connectors[0]?.name ?? 'Unknown'; // safe
```

### Utility Types You Must Know

| Type | What it does | When to use it |
|---|---|---|
| `Partial<T>` | Makes all properties optional | Form patch values, update inputs |
| `Required<T>` | Makes all properties required | After validation — guaranteed shape |
| `Pick<T, K>` | Selects subset of properties | DTO inputs, API response slices |
| `Omit<T, K>` | Removes properties | Form models without server-generated fields |
| `Record<K, V>` | Typed key-value map | Lookup maps, enum-to-value maps |
| `z.infer<typeof schema>` | Derives TS type from Zod schema | Form inputs — single source of truth |
| `NonNullable<T>` | Removes null and undefined | After a null check narrows a type |

### Type Guards

```typescript
// ★ src/app/core/models/events.ts — OpenGuard pattern
export function isAuditEvent(e: unknown): e is AuditEvent {
  return typeof e === 'object'
    && e !== null
    && (e as any).event_type?.startsWith('audit.');
}

export function isAPIError(value: unknown): value is { error: APIErrorBody } {
  return typeof value === 'object'
    && value !== null
    && 'error' in value
    && typeof (value as any).error.code === 'string';
}
```

---

# Part 2 — Angular Fundamentals (Deep Dive)

## 2.1 Architecture: Modules vs Standalone APIs

**MUST-KNOW** — The Angular ecosystem has shifted entirely to Standalone. All new code must use it.

> **★ OpenGuard Rule (CI-Enforced)**
>
> *"Every component, directive, and pipe MUST be standalone. Modular Angular (NgModules) is forbidden. This is enforced by ESLint and will block your PR."*

| NgModules (Legacy) | Standalone (Modern — use this) |
|---|---|
| Requires `@NgModule` with declarations/imports/exports arrays | Each component declares its own imports directly |
| Circular dependency risk from module inter-dependencies | Dependencies are explicit and co-located |
| Lazy loading at module boundaries only | Lazy loading at component level via `loadComponent` |
| Harder to tree-shake — entire module loads together | Better tree-shaking — unused imports excluded |
| More boilerplate, harder to test in isolation | Direct `TestBed` injection, cleaner tests |

```typescript
// ✅ CORRECT — Standalone Component
@Component({
  selector: 'app-connector-list',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    ConnectorCardComponent,
    BadgeComponent,
    PaginatorComponent,
  ],
  templateUrl: './connector-list.component.html',
})
export class ConnectorListComponent { ... }

// ✅ App Config — no AppModule needed
// src/app/app.config.ts
export const appConfig: ApplicationConfig = {
  providers: [
    provideRouter(routes, withPreloading(PreloadAllModules)),
    provideHttpClient(withInterceptors([authInterceptor, errorInterceptor])),
    provideAnimations(),
  ]
};
```

---

## 2.2 Components, Templates, and New Control Flow

**MUST-KNOW** — Angular 17+ built-in control flow replaces structural directives.

> **★ OpenGuard Rule (CI-Enforced)**
>
> *"Always use `@if`, `@for`, `@switch` block syntax. Legacy `*ngIf` and `*ngFor` directives are forbidden."*

```html
<!-- ✅ CORRECT — New control flow syntax -->
@if (isLoading()) {
  <app-skeleton-rows />
} @else if (error()) {
  <app-error-boundary [message]="error()" />
} @else {
  @for (connector of connectors(); track connector.id) {
    <app-connector-card [connector]="connector" />
  } @empty {
    <p class="text-og-text-secondary text-sm">No connectors found.</p>
  }
}

<!-- @switch for status badges -->
@switch (alert.severity) {
  @case ('critical') { <og-badge variant="critical">CRITICAL</og-badge> }
  @case ('high')     { <og-badge variant="danger">HIGH</og-badge> }
  @default           { <og-badge variant="muted">{{ alert.severity | uppercase }}</og-badge> }
}
```

### Signal Inputs and Outputs (Angular 17.1+)

```typescript
@Component({ ... })
export class ConnectorCardComponent {
  // Signal Input — creates a computed signal from the parent binding
  connector = input.required<Connector>();

  // Signal Input with default
  size = input<'sm' | 'md' | 'lg'>('md');

  // Typed Output
  suspend = output<string>(); // emits the connector ID

  // Derived state — only re-computes when connector() changes
  isSuspended = computed(() => this.connector().status === 'suspended');
  badgeVariant = computed(() => this.isSuspended() ? 'danger' : 'success');
}
```

---

## 2.3 Dependency Injection — Hierarchical Injectors

**MUST-KNOW** — DI is Angular's superpower. Understanding the injector tree prevents hard-to-debug errors.

Angular maintains a tree of injectors mirroring the component tree. When a component requests a service, Angular walks up the tree until it finds a provider.

| Level | How to configure | Use case |
|---|---|---|
| Root injector | `providedIn: 'root'` | Singletons: `AuthService`, `UiService`, `NotificationService` |
| Component injector | `providers: [...]` in `@Component` | Feature-specific state scoped to a subtree |
| Lazy route injector | Providers in lazy-loaded routes | True module isolation — not available in eager parts |

```typescript
// Root-level singleton (most common in OpenGuard)
@Injectable({ providedIn: 'root' })
export class AuthService { ... }

// Component-scoped instance (creates a NEW instance per usage)
@Component({
  providers: [ConnectorFormStateService]
})
export class ConnectorFormComponent { ... }

// Functional inject() — preferred in standalone components
export class AuditListComponent {
  private auth = inject(AuthService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
}
```

**Common DI mistakes:**
- Providing a service in `providers: []` when you meant `providedIn: 'root'` — creates multiple instances with divergent state
- Injecting a service from a child injector in a parent — throws `NullInjectorError`
- Forgetting that lazy-loaded routes create a child injector — their services are unavailable in eager parts

---

## 2.4 Change Detection: Default vs OnPush vs Signals

**MUST-KNOW** — Change detection is the #1 Angular performance topic.

| Strategy | When it re-renders | Use case |
|---|---|---|
| Default (CheckAlways) | After every event, timer, HTTP response — entire tree | Legacy code only; avoid for new development |
| OnPush | When `@Input` reference changes, Observable emits via `async` pipe, or `markForCheck()` called | All presentational components |
| Signal-based (Zoneless) | Only when a Signal it reads changes — fine-grained | New Angular 17+ apps — OpenGuard uses this |

```typescript
// OnPush — for non-signal components
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ConnectorCardComponent {
  @Input() connector!: Connector; // CD runs only when reference changes
}

// Signal-based — optimal for Angular 17+
@Component({ ... })
export class ConnectorCardComponent {
  connector = input.required<Connector>();

  // Angular tracks which signals were read during rendering
  // Only re-renders when connector() changes
  isSuspended = computed(() => this.connector().status === 'suspended');
}
```

> **The Golden Rule:** Use `OnPush` for all components not using Signals. Use Signal inputs and `computed()` for all new components. Never mutate objects in place — always replace the reference (immutable updates).

---

## 2.5 RxJS in Angular — Core Patterns and Anti-Patterns

**MUST-KNOW** — RxJS is deeply integrated into Angular. You cannot avoid it, but you can use it well.

### Operators You Must Know

| Operator | Purpose | When to use |
|---|---|---|
| `switchMap` | Cancels previous inner observable on new emission | Search-as-you-type, route changes — latest request wins |
| `mergeMap` | Runs inner observables concurrently | Independent parallel API calls |
| `concatMap` | Queues inner observables — preserves order | Sequential form submissions |
| `exhaustMap` | Ignores new emissions while inner observable runs | Login button — prevent double-submission |
| `catchError` | Handles errors, returns fallback observable | HTTP interceptors, service methods |
| `takeUntilDestroyed` | Completes observable when component is destroyed | Canonical subscription cleanup |
| `toSignal` | Converts Observable to Signal | Components — avoid `async` pipe, get better type safety |
| `debounceTime` | Delays emission until source is quiet | Search inputs |
| `distinctUntilChanged` | Suppresses duplicate consecutive values | Form `valueChanges` |

### Anti-Patterns

```typescript
// ❌ WRONG — nested subscriptions (callback hell reimagined)
this.auth.orgId$.subscribe(orgId => {
  this.connectorService.list(orgId).subscribe(connectors => {
    this.connectors = connectors; // mutable assignment + memory leak
  });
});

// ✅ CORRECT — use switchMap and toSignal
connectors = toSignal(
  this.auth.orgId$.pipe(
    filter(Boolean),
    switchMap(orgId => this.connectorService.list(orgId)),
    map(page => page.data)
  ),
  { initialValue: [] }
);
```

**Other anti-patterns:**
- `Subject` to share HTTP data — use `HttpClient` directly; let the service handle caching
- RxJS for simple one-off operations — a plain `Promise` with `async/await` is simpler for single-emission operations
- Forgetting `catchError` in effects — an unhandled error kills the observable permanently

---

# Part 3 — State Management

## 3.1 When to Use State Management vs Simple Services

**MUST-KNOW** — Over-engineering state management is one of the most common senior-level mistakes.

| Data type | Where it lives | Tool |
|---|---|---|
| Server data (lists, details) | HttpClient + Signal in service | Injectable service |
| Global UI state (sidebar, modals) | Root-provided service with Signals | `UiService` |
| Notifications / toasts | Root-provided service with `Signal<Toast[]>` | `NotificationService` |
| Auth session / org context | `AuthService` (Signals) | Computed signals from auth state |
| URL state (filters, pagination) | Angular Router (`queryParams`) | `Router.navigate` + `queryParamsHandling: 'merge'` |
| Real-time stream data | Local Signal in component | `toSignal()` or `signal.update()` |
| Complex multi-feature shared state | NgRx Store | Only when 3+ features share the same data |
| Form state | Angular Reactive Forms | `FormBuilder`, `FormGroup`, `FormControl` |

---

## 3.2 Signal-Based State — The OpenGuard Pattern ★

**MUST-KNOW** for Angular 17+ projects.

```typescript
// ★ src/app/core/state/ui.service.ts
@Injectable({ providedIn: 'root' })
export class UiService {
  // Private WritableSignal — internal mutation only
  private _sidebarCollapsed = signal(
    JSON.parse(localStorage.getItem('og:ui:sidebar') ?? 'false')
  );

  // Public read-only Signal — consumers cannot mutate directly
  isSidebarCollapsed = this._sidebarCollapsed.asReadonly();

  constructor() {
    // Persist as side effect
    effect(() => {
      localStorage.setItem('og:ui:sidebar',
        JSON.stringify(this._sidebarCollapsed()));
    });
  }

  toggleSidebar(): void {
    this._sidebarCollapsed.update(v => !v);
  }

  // Async confirm dialog — imperative API backed by Signals
  readonly confirmDialog = signal<ConfirmDialogState | null>(null);
  private resolveConfirm?: (confirmed: boolean) => void;

  async confirm(options: ConfirmDialogState): Promise<boolean> {
    this.confirmDialog.set(options);
    return new Promise(resolve => {
      this.resolveConfirm = confirmed => {
        this.confirmDialog.set(null);
        resolve(confirmed);
      };
    });
  }

  handleConfirm(confirmed: boolean) {
    this.resolveConfirm?.(confirmed);
  }
}

// Any component — inject and use
export class ConnectorListComponent {
  private ui = inject(UiService);

  async handleSuspend(connector: Connector) {
    const confirmed = await this.ui.confirm({
      title: 'Suspend connector?',
      description: `${connector.name} will immediately lose API access.`,
      confirmLabel: 'Suspend',
      variant: 'destructive',
    });
    if (confirmed) this.suspendConnector(connector.id);
  }
}
```

---

## 3.3 NgRx — When, Why, and How

**SHOULD-KNOW** — Use NgRx when multiple feature modules share state that changes independently.

### When NgRx is the right choice
- Three or more features share the same server data and each can trigger updates
- You need time-travel debugging or action logging for compliance/audit
- Large team needs strict conventions enforced by the library
- Complex optimistic update rollback scenarios

### When NgRx is overkill
- A service with Signals covers 80% of Angular state needs with 20% of the boilerplate
- The app has one or two features and a small team
- All your state is server data — use `HttpClient` directly

```typescript
// Modern NgRx — functional API (NgRx 16+)
export const ConnectorActions = createActionGroup({
  source: 'Connector',
  events: {
    'Load Connectors': props<{ orgId: string }>(),
    'Load Connectors Success': props<{ connectors: Connector[] }>(),
    'Load Connectors Failure': props<{ error: string }>(),
  }
});

export const connectorFeature = createFeature({
  name: 'connectors',
  reducer: createReducer(
    { connectors: [] as Connector[], loading: false, error: null as string | null },
    on(ConnectorActions.loadConnectors,
      state => ({ ...state, loading: true })),
    on(ConnectorActions.loadConnectorsSuccess,
      (state, { connectors }) => ({ ...state, loading: false, connectors })),
    on(ConnectorActions.loadConnectorsFailure,
      (state, { error }) => ({ ...state, loading: false, error })),
  )
});

// Functional effect
export const loadConnectors$ = createEffect(
  (actions$ = inject(Actions), svc = inject(ConnectorsService)) =>
    actions$.pipe(
      ofType(ConnectorActions.loadConnectors),
      switchMap(({ orgId }) =>
        svc.list(orgId).pipe(
          map(page => ConnectorActions.loadConnectorsSuccess({ connectors: page.data })),
          catchError(err => of(ConnectorActions.loadConnectorsFailure({ error: err.message })))
        )
      )
    ),
  { functional: true }
);
```

---

## 3.4 Trade-offs: NgRx vs Signals vs Services

| Dimension | Signals + Services | NgRx |
|---|---|---|
| Boilerplate | Minimal — one `Injectable` service | High — actions, reducers, effects, selectors |
| Learning curve | Low — familiar patterns | Steep — Redux mental model required |
| Debugging | Angular DevTools signal inspection | Redux DevTools with time-travel |
| Testability | Easy — inject service, test methods | Requires `MockStore` setup |
| Scale | Good up to ~10 features | Excellent for 20+ features, large teams |
| Real-time data | Natural fit with `toSignal()` | Effect complexity increases |
| OpenGuard choice | ✓ Used for all state | Not used — Signals sufficient |

---

# Part 4 — Styling & UI Systems

# Part 4.1 — TailwindCSS in Angular: The Complete Production Playbook

> **Stack context:** Angular 19 · TailwindCSS 4 · Standalone components · TypeScript strict  
> Sections marked **★** contain patterns from the OpenGuard production codebase.

---

## How This Section Works

Every topic follows: **concept → when to use it → production example → common mistakes**.

The goal is not to be a Tailwind reference (the docs cover that). The goal is to teach you how to *think* in Tailwind at scale — making decisions that stay maintainable at 50 components and 10 engineers.

---

## 4.1.1 — The Mental Model: Utility-First at Scale

Before touching a class, internalise the core shift:

**Tailwind is not CSS shorthand. It is a constraint system.**

Every utility maps to a design token. `p-4` is not "16px of padding" — it is "one unit of your spacing scale." When you use `p-[17px]`, you are *leaving the system*. The moment you leave the system, two things break: visual consistency and the ability to retheme.

**The three-tier decision:**

```
Can I express this with a Tailwind token?   → Use the utility class
Can I express it with a CSS variable?        → Use @apply or CSS var + utility
Is this truly one-off and unconstrained?    → Write raw CSS (rare, and you should feel bad)
```

**Tailwind 4 vs Tailwind 3 — what changed:**

Tailwind 4 ships with a CSS-first config. Instead of `tailwind.config.ts`, you define your design tokens directly in your CSS file using `@theme`. This matters for Angular integration:

```css
/* src/styles/globals.css — Tailwind 4 config (CSS-first) */
@import "tailwindcss";

@theme {
  --color-og-bg-base:      #09090B;
  --color-og-bg-surface:   #111113;
  --color-og-bg-elevated:  #18181B;
  --color-og-accent:       #06B6D4;
  --color-og-danger:       #EF4444;
  --color-og-critical:     #FF2056;
  --color-og-text-primary: #FAFAFA;
  --color-og-text-secondary: #A1A1AA;
  --color-og-text-muted:   #52525B;
  --font-family-display:   '"JetBrains Mono", monospace';
  --font-family-body:      '"IBM Plex Sans", sans-serif';
  --font-family-mono:      '"JetBrains Mono", monospace';
  --spacing-18:            4.5rem;
}
```

With this, `text-og-accent` and `bg-og-bg-surface` become valid utility classes automatically.

---

## 4.1.2 — Layout: Flexbox, Grid, and Container Patterns

### Flexbox

**Concept:** One-dimensional layout. Use for rows of items, navigation bars, icon+label pairs, and anything that needs to stretch or shrink along a single axis.

**The alignment mental model:**

```
flex-row  → main axis is horizontal (→)
flex-col  → main axis is vertical (↓)

justify-*  → distributes items along the main axis
items-*    → aligns items along the cross axis
self-*     → overrides alignment for a single item
content-*  → controls multi-row/column packing (when flex-wrap is active)
```

**Production example — header with logo, nav, and actions:**

```html
<!-- Header: three-zone layout that degrades gracefully -->
<header class="flex items-center justify-between
               px-6 py-3 border-b border-zinc-800">

  <!-- Logo zone: shrinks last -->
  <div class="flex items-center gap-3 min-w-0 shrink-0">
    <app-logo class="size-7" />
    <span class="font-display text-sm text-og-text-primary truncate">
      OpenGuard
    </span>
  </div>

  <!-- Nav: takes remaining space, scrolls on overflow -->
  <nav class="flex items-center gap-1 overflow-x-auto scrollbar-none mx-4 grow">
    @for (item of navItems(); track item.path) {
      <a [routerLink]="item.path"
         routerLinkActive="bg-og-bg-elevated text-og-text-primary"
         class="flex items-center gap-2 px-3 py-1.5 rounded-md
                text-sm text-og-text-secondary whitespace-nowrap
                transition-colors hover:text-og-text-primary">
        <i-lucide [name]="item.icon" [size]="14" aria-hidden="true" />
        {{ item.label }}
      </a>
    }
  </nav>

  <!-- Actions: shrinks last, never wraps -->
  <div class="flex items-center gap-2 shrink-0">
    <app-notification-bell />
    <app-user-menu />
  </div>
</header>
```

**Common Flexbox mistakes:**

```html
<!-- ❌ width: 100% inside flex — doesn't mean what you think -->
<div class="flex">
  <div class="w-full">...</div>   <!-- This means "take up all remaining space" -->
  <div class="w-24">...</div>     <!-- This gets pushed off screen -->
</div>

<!-- ✅ Use grow/shrink instead -->
<div class="flex">
  <div class="grow min-w-0">...</div>   <!-- min-w-0 prevents overflow of text -->
  <div class="shrink-0 w-24">...</div>
</div>
```

> **★ OpenGuard pattern:** `min-w-0` is almost always required on `grow` containers that contain text. Without it, text overflows its container because the browser won't shrink a flex item below its intrinsic content width.

---

### CSS Grid

**Concept:** Two-dimensional layout. Use for page-level structure, data grids, card galleries, and anything that needs items aligned in both rows and columns simultaneously.

**When to choose Grid over Flex:**
- Items need to align to a shared grid (dashboards, cards)
- You need areas that span multiple rows or columns
- Layout is genuinely two-dimensional

**Production example — dashboard grid layout:**

```html
<!-- Dashboard: sidebar + main + detail panel -->
<div class="grid grid-cols-[240px_1fr] grid-rows-[64px_1fr]
            h-screen min-h-0">

  <!-- Topbar spans full width -->
  <header class="col-span-2 border-b border-zinc-800">
    <app-topbar />
  </header>

  <!-- Sidebar: fixed width, full remaining height -->
  <aside class="border-r border-zinc-800 overflow-y-auto">
    <app-sidebar />
  </aside>

  <!-- Main content: fills remaining space -->
  <main class="overflow-y-auto p-6">
    <router-outlet />
  </main>
</div>
```

**Auto-responsive card grid (no media queries):**

```html
<!-- Cards auto-arrange: fills row, minimum 280px per card -->
<div class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
  @for (connector of connectors(); track connector.id) {
    <app-connector-card [connector]="connector" />
  }
</div>
```

**Named template areas ★ — for complex layouts:**

```css
/* globals.css — declare the named grid */
.dashboard-layout {
  display: grid;
  grid-template-areas:
    "header  header"
    "sidebar main  ";
  grid-template-columns: 240px 1fr;
  grid-template-rows: 64px 1fr;
  height: 100vh;
}

.dashboard-layout > .header  { grid-area: header; }
.dashboard-layout > .sidebar { grid-area: sidebar; }
.dashboard-layout > .main    { grid-area: main; }
```

```html
<div class="dashboard-layout">
  <header class="header">...</header>
  <aside class="sidebar">...</aside>
  <main class="main">...</main>
</div>
```

---

### Container Patterns

**Concept:** Consistent horizontal padding and max-width that keeps content readable on large screens.

```html
<!-- ✅ Production container: centers content, readable on ultrawide -->
<div class="mx-auto w-full max-w-7xl px-4 sm:px-6 lg:px-8">
  ...
</div>

<!-- ✅ Narrower container for prose/forms -->
<div class="mx-auto w-full max-w-2xl px-4">
  ...
</div>
```

**Angular component pattern — container as host element:**

```typescript
// connector-list.component.ts
@Component({
  selector: 'app-connector-list',
  host: {
    class: 'block mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 py-8'
  },
  template: `...`
})
export class ConnectorListComponent {}
```

Using `host.class` applies layout to the custom element itself — no wrapper div needed in the template.

---

## 4.1.3 — Spacing System

**Concept:** Tailwind's spacing scale (4px base unit) is non-negotiable. Break it and you break visual rhythm.

| Scale | Value | When to use |
|---|---|---|
| `gap-1` | 4px | Icon-to-label, tight badge padding |
| `gap-2` | 8px | List item internal spacing |
| `gap-3` | 12px | Button icon+text, form field groups |
| `gap-4` | 16px | Card padding, section content |
| `gap-6` | 24px | Between card sections |
| `gap-8` | 32px | Between page sections |
| `gap-12` | 48px | Major page section breaks |

**Logical spacing (LTR/RTL aware):**

```html
<!-- Physical spacing — breaks in RTL layouts -->
<div class="ml-4 mr-2">...</div>

<!-- ✅ Logical spacing — works in any writing direction -->
<div class="ms-4 me-2">...</div>
```

**Gap vs margin — the rule:**

```html
<!-- ✅ Use gap on the parent for distributed layouts -->
<div class="flex flex-col gap-3">
  <app-field />
  <app-field />
  <app-field />
</div>

<!-- ❌ Using margin-top on children for the same thing -->
<div class="flex flex-col">
  <app-field class="mt-3 first:mt-0" />  <!-- Fragile, leaks into component -->
</div>
```

> **★ OpenGuard rule:** Gap on the container, never margin on the children for distributing repeated items. Margin on children couples the child to its parent's layout context.

---

## 4.1.4 — Positioning

**Concept:** Most layouts should use Flex/Grid. Reach for `position` only when you need to break out of the document flow intentionally.

| Utility | Use case |
|---|---|
| `relative` | Anchor for absolutely positioned children (tooltips, badges) |
| `absolute` | Decorative overlays, badge counts, floating labels |
| `fixed` | Modals, toasts, persistent navigation |
| `sticky` | Table headers, section headings that follow scroll |

**Production example — notification badge on an icon:**

```html
<!-- Relative parent anchors the absolute badge -->
<button class="relative p-2 text-og-text-secondary hover:text-og-text-primary">
  <i-lucide name="bell" [size]="18" aria-hidden="true" />

  @if (unreadCount() > 0) {
    <span class="absolute -top-0.5 -right-0.5
                 flex size-4 items-center justify-center
                 rounded-full bg-og-critical
                 text-[10px] font-bold text-white">
      {{ unreadCount() > 9 ? '9+' : unreadCount() }}
    </span>
  }
</button>
```

**Sticky table header — ★ OpenGuard audit table:**

```html
<div class="relative overflow-auto max-h-[calc(100vh-200px)]">
  <table class="w-full text-sm">
    <thead class="sticky top-0 z-10 bg-og-bg-surface border-b border-zinc-800">
      <tr>
        <th class="px-4 py-3 text-left text-xs font-medium
                   text-og-text-muted uppercase tracking-wider">
          Time
        </th>
        <!-- ... -->
      </tr>
    </thead>
    <tbody>
      @for (event of events(); track event.id) {
        <tr>...</tr>
      }
    </tbody>
  </table>
</div>
```

**Z-index scale — define it, don't improvise it:**

```css
/* globals.css — explicit z-index layers */
@theme {
  --z-index-base:    0;
  --z-index-raised:  10;   /* Dropdown menus, tooltips */
  --z-index-overlay: 20;   /* Modals, drawers */
  --z-index-toast:   30;   /* Toast notifications (above modals) */
  --z-index-top:     40;   /* Command palette, global search */
}
```

```html
<!-- Use Tailwind's built-in z scale for predictable stacking -->
<div class="z-10">  <!-- Dropdown -->
<div class="z-20">  <!-- Modal -->
<div class="z-30">  <!-- Toast -->
```

---

## 4.1.5 — Responsive Design: Mobile-First

**Concept:** Tailwind is mobile-first. No prefix = applies everywhere. A prefix (`sm:`, `md:`, `lg:`, `xl:`) applies from that breakpoint *and up*.

| Prefix | Breakpoint | Width |
|---|---|---|
| *(none)* | Default | 0px+ |
| `sm:` | Small | 640px+ |
| `md:` | Medium | 768px+ |
| `lg:` | Large | 1024px+ |
| `xl:` | Extra Large | 1280px+ |
| `2xl:` | 2X Large | 1536px+ |

**Production example — responsive connector list:**

```html
<!-- Stack vertically on mobile, grid on desktop -->
<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
  @for (connector of connectors(); track connector.id) {
    <app-connector-card [connector]="connector" />
  }
</div>
```

**Responsive visibility:**

```html
<!-- Hide on mobile, show on desktop -->
<span class="hidden lg:inline-flex">Connector details</span>

<!-- Compact on mobile, full on desktop -->
<button class="px-2 sm:px-4">
  <span class="sr-only sm:not-sr-only sm:mr-2">Add</span>
  <i-lucide name="plus" [size]="16" aria-hidden="true" />
</button>
```

**Custom breakpoints in Tailwind 4:**

```css
@theme {
  --breakpoint-xs: 480px;     /* Adds xs: prefix */
  --breakpoint-3xl: 1920px;   /* Adds 3xl: prefix for ultrawide */
}
```

**Container queries — the future of responsive components:**

Container queries respond to the *parent container's width*, not the viewport. This is critical for components used in multiple layout contexts (sidebar vs main content).

```html
<!-- Enable container queries on the parent -->
<div class="@container">
  <div class="grid grid-cols-1 @md:grid-cols-2 @xl:grid-cols-3">
    <!-- Responds to parent width, not viewport width -->
  </div>
</div>
```

```typescript
// Angular component — enable @container on the host element
@Component({
  selector: 'app-metric-grid',
  host: { class: '@container block' },
  template: `
    <div class="grid grid-cols-1 gap-4
                @sm:grid-cols-2 @lg:grid-cols-4">
      @for (metric of metrics(); track metric.id) {
        <app-metric-card [metric]="metric" />
      }
    </div>
  `
})
```

> **When to use container queries vs media queries:**
> - Media queries: page-level layout decisions (sidebar width, nav collapse)
> - Container queries: component-level decisions (card layout, text truncation)
>
> **★ OpenGuard pattern:** All shared UI components (`BadgeComponent`, `MetricCard`, `DataTable`) use container queries so they adapt wherever they're placed.

---

## 4.1.6 — Typography System

**Concept:** Typography is not font size. It is the entire system of scale, weight, line height, and letter spacing working together.

**The OpenGuard type scale:**

```html
<!-- Page title: display font, monospace feel for security context -->
<h1 class="font-display text-2xl font-medium tracking-tight text-og-text-primary">
  Connectors
</h1>

<!-- Section heading -->
<h2 class="font-body text-base font-medium text-og-text-primary">
  Active connectors
</h2>

<!-- Body text: optimised for reading -->
<p class="font-body text-sm text-og-text-secondary leading-relaxed">
  Connectors allow third-party systems to ingest events into OpenGuard.
</p>

<!-- Labels, metadata -->
<span class="font-body text-xs text-og-text-muted uppercase tracking-wider">
  Last active
</span>

<!-- Technical values: IDs, hashes, tokens — monospace always -->
<code class="font-mono text-xs text-og-text-secondary bg-og-bg-elevated
             px-1.5 py-0.5 rounded">
  conn_01HZYX3F...
</code>
```

**Line clamp — truncating multi-line text:**

```html
<!-- Clamp to exactly 2 lines, show ellipsis -->
<p class="line-clamp-2 text-sm text-og-text-secondary">
  {{ connector.description }}
</p>

<!-- Clamp to 1 line (same as truncate but semantically clearer) -->
<span class="line-clamp-1 font-medium">{{ connector.name }}</span>

<!-- Remove clamp on hover (expand on hover) -->
<p class="line-clamp-3 hover:line-clamp-none transition-all duration-200">
  {{ alert.details }}
</p>
```

**Prose — for rich text content:**

```html
<!-- Tailwind's typography plugin for markdown/HTML content -->
<div class="prose prose-sm prose-invert max-w-none
            prose-headings:font-display prose-code:font-mono">
  <div [innerHTML]="safeHtml()"></div>
</div>
```

**Text overflow handling — the full toolkit:**

```html
<!-- Single line truncation -->
<span class="truncate">Very long connector name that will be cut off</span>

<!-- Break long words (URLs, tokens) -->
<span class="break-all font-mono text-xs">{{ longToken }}</span>

<!-- Break at word boundaries, avoid overflow -->
<p class="break-words text-sm">{{ description }}</p>

<!-- Nowrap — prevent a label from wrapping at all -->
<span class="whitespace-nowrap text-xs text-og-text-muted">
  Last seen 3 hours ago
</span>
```

---

## 4.1.7 — Color System and Theming

**Concept:** Colors in Tailwind are not hex values. They are semantic tokens that map to a value based on the current theme. If you hardcode hex values in your classes, you are building a system that cannot be rethemed.

**The three-layer color architecture:**

```
Layer 1: Raw values      → #09090B, #06B6D4 (in @theme)
Layer 2: Design tokens   → --color-og-accent, --color-og-bg-surface
Layer 3: Semantic roles  → bg-og-bg-surface, text-og-accent
```

Components should only use Layer 3. Never Layer 1.

**Complete color token setup (Tailwind 4):**

```css
@import "tailwindcss";

@theme {
  /* Backgrounds */
  --color-og-bg-base:       #09090B;  /* Page background */
  --color-og-bg-surface:    #111113;  /* Cards, panels */
  --color-og-bg-elevated:   #18181B;  /* Hover states, dropdowns */
  --color-og-bg-overlay:    #27272A;  /* Tooltips, popovers */

  /* Brand */
  --color-og-accent:        #06B6D4;  /* Primary CTA, links */
  --color-og-accent-hover:  #0891B2;  /* Hover state for accent */

  /* Status */
  --color-og-success:       #22C55E;
  --color-og-warning:       #F59E0B;
  --color-og-danger:        #EF4444;
  --color-og-critical:      #FF2056;

  /* Text */
  --color-og-text-primary:  #FAFAFA;
  --color-og-text-secondary:#A1A1AA;
  --color-og-text-muted:    #52525B;

  /* Borders */
  --color-og-border:        #27272A;
  --color-og-border-subtle: #18181B;
}
```

**Dynamic severity colors — the right pattern:**

```typescript
// ★ Map severity to variant — done ONCE in the component
@Component({
  selector: 'og-badge',
  template: `
    <span [class]="classes()">
      <ng-content />
    </span>
  `
})
export class BadgeComponent {
  variant = input<Severity>('info');

  protected classes = computed(() => {
    const base = 'inline-flex items-center px-2 py-0.5 rounded text-xs font-medium uppercase tracking-wide';
    const variants: Record<Severity, string> = {
      success:  `${base} bg-og-success/10 text-og-success`,
      warning:  `${base} bg-og-warning/10 text-og-warning`,
      danger:   `${base} bg-og-danger/10 text-og-danger`,
      critical: `${base} bg-og-critical/10 text-og-critical`,
      info:     `${base} bg-blue-500/10 text-blue-400`,
      muted:    `${base} bg-zinc-600/10 text-zinc-400`,
    };
    return variants[this.variant()];
  });
}
```

**The opacity modifier — shortcut for translucent colours:**

```html
<!-- bg-og-accent at 10% opacity — no separate token needed -->
<div class="bg-og-accent/10 text-og-accent">

<!-- Dynamic opacity in a computed class -->
<div [class]="'bg-og-danger/' + (isActive() ? '20' : '10')">
```

**Dark mode — class strategy:**

```typescript
// ui.service.ts — controls <html class="dark">
@Injectable({ providedIn: 'root' })
export class ThemeService {
  private _dark = signal(
    window.matchMedia('(prefers-color-scheme: dark)').matches
  );

  readonly isDark = this._dark.asReadonly();

  toggle() {
    this._dark.update(d => !d);
    document.documentElement.classList.toggle('dark', this._dark());
  }
}
```

```css
/* Light theme overrides when .light class is added to <html> */
@theme {
  --color-og-bg-base: #FFFFFF;
  --color-og-text-primary: #09090B;
}

/* Scoped light theme */
.light {
  --color-og-bg-base:      #FFFFFF;
  --color-og-bg-surface:   #F4F4F5;
  --color-og-text-primary: #09090B;
  --color-og-accent:       #0284C7;
}
```

---

## 4.1.8 — Component Patterns: Buttons, Cards, Forms

### Buttons

**The variant system — one component, all states:**

```typescript
// ★ button.component.ts — exhaustive variant map
@Component({
  selector: 'og-button',
  standalone: true,
  template: `
    <button
      [type]="type()"
      [disabled]="disabled() || pending()"
      [class]="buttonClasses()"
      [attr.aria-busy]="pending()">

      @if (pending()) {
        <span class="size-4 border-2 border-current border-t-transparent
                     rounded-full animate-spin" aria-hidden="true"></span>
      }
      <ng-content />
    </button>
  `
})
export class ButtonComponent {
  variant  = input<'primary' | 'secondary' | 'ghost' | 'danger'>('primary');
  size     = input<'sm' | 'md' | 'lg'>('md');
  type     = input<'button' | 'submit' | 'reset'>('button');
  disabled = input(false);
  pending  = input(false);

  protected buttonClasses = computed(() => {
    const base = [
      'inline-flex items-center justify-center gap-2',
      'font-medium rounded-md transition-colors',
      'focus-visible:outline-none focus-visible:ring-2',
      'focus-visible:ring-og-accent focus-visible:ring-offset-2',
      'focus-visible:ring-offset-og-bg-base',
      'disabled:opacity-40 disabled:pointer-events-none',
    ].join(' ');

    const sizes = {
      sm: 'h-7  px-3 text-xs gap-1.5',
      md: 'h-9  px-4 text-sm',
      lg: 'h-11 px-6 text-base',
    };

    const variants = {
      primary:   'bg-og-accent text-zinc-900 hover:bg-og-accent-hover',
      secondary: 'bg-og-bg-elevated text-og-text-primary border border-og-border hover:bg-og-bg-overlay',
      ghost:     'text-og-text-secondary hover:text-og-text-primary hover:bg-og-bg-elevated',
      danger:    'bg-og-danger/10 text-og-danger border border-og-danger/30 hover:bg-og-danger/20',
    };

    return `${base} ${sizes[this.size()]} ${variants[this.variant()]}`;
  });
}
```

```html
<!-- Usage -->
<og-button variant="primary" (click)="create()">Create connector</og-button>
<og-button variant="danger" [pending]="isSuspending()">Suspend</og-button>
<og-button variant="ghost" size="sm">Cancel</og-button>
```

---

### Cards

**Consistent surface pattern:**

```html
<!-- Base card -->
<div class="rounded-lg border border-og-border bg-og-bg-surface p-6">
  ...
</div>

<!-- Interactive card (clickable) -->
<div class="group rounded-lg border border-og-border bg-og-bg-surface p-6
            cursor-pointer transition-colors
            hover:border-zinc-700 hover:bg-og-bg-elevated">
  ...
</div>

<!-- ★ Connector card — production pattern -->
<article class="flex flex-col gap-4 rounded-lg border border-og-border
                bg-og-bg-surface p-5 transition-colors
                hover:border-zinc-700">

  <!-- Header: title + badge + menu -->
  <div class="flex items-start justify-between gap-3">
    <div class="min-w-0">
      <h3 class="truncate font-display text-sm font-medium text-og-text-primary">
        {{ connector().name }}
      </h3>
      <p class="mt-0.5 text-xs text-og-text-muted font-mono">
        {{ connector().id }}
      </p>
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <og-badge [variant]="connector().status">
        {{ connector().status | uppercase }}
      </og-badge>
      <app-connector-menu [connector]="connector()" />
    </div>
  </div>

  <!-- Scope list -->
  <div class="flex flex-wrap gap-1.5">
    @for (scope of connector().scopes; track scope) {
      <span class="inline-flex items-center rounded-full bg-og-bg-elevated
                   px-2 py-0.5 text-[11px] font-mono text-og-text-muted">
        {{ scope }}
      </span>
    }
  </div>

  <!-- Footer: metadata -->
  <div class="flex items-center justify-between border-t border-og-border-subtle pt-3">
    <span class="text-xs text-og-text-muted">
      Created {{ connector().created_at | timeAgo }}
    </span>
    <span class="text-xs text-og-text-muted">
      {{ connector().event_count | number }} events
    </span>
  </div>
</article>
```

---

### Forms

**Full form system — inputs, labels, error states:**

```html
<!-- Form field wrapper -->
<div class="flex flex-col gap-1.5">

  <!-- Label with required indicator -->
  <label class="text-sm font-medium text-og-text-secondary" [for]="id">
    {{ label }}
    @if (required) {
      <span class="text-og-danger ml-0.5" aria-hidden="true">*</span>
    }
  </label>

  <!-- Input -->
  <input
    [id]="id"
    [formControlName]="controlName"
    [placeholder]="placeholder"
    [class]="inputClasses()"
    [attr.aria-describedby]="errorId"
    [attr.aria-invalid]="isInvalid()" />

  <!-- Helper text (hidden when error shown) -->
  @if (!isInvalid() && helperText) {
    <p class="text-xs text-og-text-muted">{{ helperText }}</p>
  }

  <!-- Error message -->
  @if (isInvalid()) {
    <p [id]="errorId" class="text-xs text-og-danger" role="alert">
      {{ errorMessage() }}
    </p>
  }
</div>
```

```typescript
// Input classes — reactive to validation state
protected inputClasses = computed(() => {
  const base = [
    'w-full rounded-md border bg-og-bg-elevated px-3 py-2',
    'font-body text-sm text-og-text-primary placeholder:text-og-text-muted',
    'transition-colors outline-none',
    'focus:ring-2 focus:ring-offset-2 focus:ring-offset-og-bg-base',
  ].join(' ');

  const state = this.isInvalid()
    ? 'border-og-danger/50 focus:border-og-danger focus:ring-og-danger'
    : 'border-og-border   focus:border-og-accent  focus:ring-og-accent';

  return `${base} ${state}`;
});
```

---

## 4.1.9 — State Variants

**Concept:** State variants modify a utility based on an interactive or relational condition. Mastering them removes the need for manual class toggling in most cases.

### Interaction States

```html
<!-- hover, focus-visible, active, disabled -->
<button class="bg-og-accent
               hover:bg-og-accent-hover
               focus-visible:ring-2 focus-visible:ring-og-accent
               active:scale-[0.98]
               disabled:opacity-40 disabled:cursor-not-allowed
               transition-all">
  Submit
</button>
```

### Group Variant — parent controls children

**When to use:** When hovering a parent should affect a child element.

```html
<!-- group on parent, group-hover on children -->
<div class="group flex items-center gap-3 px-3 py-2 rounded-md
            cursor-pointer hover:bg-og-bg-elevated">

  <!-- Icon: dim by default, full opacity on group hover -->
  <i-lucide name="server" [size]="16"
            class="text-og-text-muted transition-colors
                   group-hover:text-og-accent" />

  <!-- Label: secondary by default, primary on group hover -->
  <span class="text-sm text-og-text-secondary transition-colors
               group-hover:text-og-text-primary">
    {{ label }}
  </span>

  <!-- Arrow: hidden by default, shown on group hover -->
  <i-lucide name="chevron-right" [size]="14"
            class="ml-auto text-og-text-muted opacity-0 transition-opacity
                   group-hover:opacity-100" />
</div>
```

**Named groups — when there are nested hoverable regions:**

```html
<div class="group/card hover:bg-og-bg-elevated">
  <div class="group/action">
    <!-- group/card vs group/action are independent -->
    <button class="opacity-0 group-hover/card:opacity-100
                   group-hover/action:text-og-danger">
      Delete
    </button>
  </div>
</div>
```

### Peer Variant — sibling-driven state

**When to use:** When a sibling element (usually a form input or checkbox) should affect adjacent elements.

```html
<!-- Checkbox controls its label styling -->
<label class="flex items-center gap-3 cursor-pointer">
  <input type="checkbox" [formControlName]="scope"
         class="peer size-4 rounded border-og-border
                bg-og-bg-elevated accent-og-accent
                focus-visible:ring-2 focus-visible:ring-og-accent" />

  <!-- Label: muted by default, primary when checked -->
  <span class="text-sm text-og-text-muted transition-colors
               peer-checked:text-og-text-primary
               peer-disabled:opacity-50">
    {{ scope }}
  </span>
</label>
```

```html
<!-- ★ Input with floating label using peer -->
<div class="relative">
  <input
    id="name-input"
    placeholder=" "
    class="peer w-full rounded-md border border-og-border bg-og-bg-elevated
           px-3 pt-5 pb-2 text-sm text-og-text-primary
           focus:outline-none focus:border-og-accent" />

  <label for="name-input"
         class="absolute left-3 top-3.5 text-sm text-og-text-muted
                transition-all cursor-text
                peer-focus:top-1.5 peer-focus:text-xs peer-focus:text-og-accent
                peer-[:not(:placeholder-shown)]:top-1.5
                peer-[:not(:placeholder-shown)]:text-xs">
    Connector name
  </label>
</div>
```

### Data Attribute Variants — Angular integration ★

```typescript
// Angular's routerLinkActive adds active class — but data attributes are cleaner
@Component({
  selector: 'app-nav-item',
  host: {
    '[attr.data-active]': 'isActive() ? "" : null',
    '[attr.data-disabled]': 'disabled() ? "" : null',
  },
  template: `
    <a [routerLink]="path()"
       class="flex items-center gap-2 px-3 py-2 rounded-md text-sm
              text-og-text-muted transition-colors
              data-[active]:bg-og-bg-elevated data-[active]:text-og-text-primary
              hover:bg-og-bg-elevated hover:text-og-text-primary
              data-[disabled]:opacity-40 data-[disabled]:pointer-events-none">
      <ng-content />
    </a>
  `
})
```

---

## 4.1.10 — Animation and Transitions

**Concept:** Tailwind's transition utilities handle the most common cases. For complex sequences, use CSS `@keyframes` via the `animate-` prefix. The rule: if it needs physics or complex sequencing, reach for Angular Animations.

**Transition system:**

```html
<!-- Base transition: all properties, 150ms (too aggressive for most things) -->
<div class="transition">

<!-- ✅ Targeted transitions: only the properties you're animating -->
<div class="transition-colors duration-150">        <!-- Color changes -->
<div class="transition-opacity duration-200">       <!-- Show/hide -->
<div class="transition-transform duration-200">     <!-- Movement, scale -->
<div class="transition-[height] duration-300 ease-in-out"> <!-- Height expand -->
```

**Micro-interactions — the ones that matter:**

```html
<!-- Button press: subtle scale-down for tactile feel -->
<button class="transition-transform active:scale-[0.97]">Click me</button>

<!-- Card lift on hover: shadow + translate (GPU-accelerated, no reflow) -->
<div class="transition-[transform,box-shadow] duration-200
            hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/20">

<!-- Badge pulse for live status -->
<span class="relative inline-flex size-2">
  <span class="absolute inline-flex size-full rounded-full
               bg-og-success opacity-75 animate-ping"></span>
  <span class="relative inline-flex size-2 rounded-full bg-og-success"></span>
</span>

<!-- Skeleton loading shimmer -->
<div class="animate-pulse rounded-md bg-og-bg-elevated h-4 w-3/4"></div>
```

**Custom animations — ★ OpenGuard SSE event flash:**

```css
/* globals.css — define keyframe */
@keyframes row-flash {
  0%   { background-color: theme(--color-og-accent / 0.15); }
  100% { background-color: transparent; }
}

@theme {
  --animate-row-flash: row-flash 1s ease-out forwards;
}
```

```html
<!-- Angular adds class when new SSE event arrives -->
<tr [class.animate-row-flash]="isNew()">
  ...
</tr>
```

**Respecting reduced motion — always:**

```html
<!-- Tailwind's motion-safe and motion-reduce prefixes -->
<div class="motion-safe:animate-spin motion-reduce:hidden">
  <span>Loading...</span>
</div>

<!-- More commonly: disable animations for prefer-reduced-motion users -->
<div class="animate-ping motion-reduce:animate-none">
```

---

## 4.1.11 — Reusability Patterns

### Component Extraction — the correct unit of reuse

**The rule: extract to an Angular component, not a CSS class.**

```typescript
// ✅ Extract to a component — testable, injectable, type-safe
@Component({
  selector: 'og-metric-card',
  template: `
    <div class="rounded-lg border border-og-border bg-og-bg-surface p-5">
      <p class="text-xs font-medium uppercase tracking-wider text-og-text-muted">
        {{ label() }}
      </p>
      <p class="mt-2 font-display text-3xl font-medium text-og-text-primary">
        {{ value() }}
      </p>
      @if (delta()) {
        <p class="mt-1 text-xs" [class]="deltaClasses()">
          {{ delta() }}
        </p>
      }
    </div>
  `
})
export class MetricCardComponent {
  label = input.required<string>();
  value = input.required<string | number>();
  delta = input<string>();
}
```

### When @apply Is Acceptable

**@apply should be rare.** Use it only when the same class sequence is repeated in raw CSS (not templates), or when you're styling elements you don't control (third-party library output, `[innerHTML]` content).

```css
/* ✅ Acceptable: styling markdown output you don't control */
.prose-custom code {
  @apply font-mono text-xs bg-og-bg-elevated px-1.5 py-0.5 rounded text-og-accent;
}

/* ✅ Acceptable: global element resets */
* {
  @apply border-og-border;
}

/* ❌ Not acceptable: for reuse in templates */
.btn-primary {
  @apply bg-og-accent text-zinc-900 px-4 py-2 rounded-md; /* Use a component instead */
}
```

### Angular Directive — apply classes without a component wrapper

When you need to apply a set of classes to an element without wrapping it in a new component:

```typescript
// ★ Directive: apply table cell styles to any <td>
@Directive({
  selector: 'td[ogCell]',
  standalone: true,
  host: {
    class: 'px-4 py-3 text-sm text-og-text-secondary border-b border-og-border-subtle whitespace-nowrap'
  }
})
export class CellDirective {}

// th[ogHeaderCell]
@Directive({
  selector: 'th[ogHeaderCell]',
  standalone: true,
  host: {
    class: 'px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-og-text-muted border-b border-og-border'
  }
})
export class HeaderCellDirective {}
```

```html
<!-- Usage: clean templates with consistent table styling -->
<table>
  <thead>
    <tr>
      <th ogHeaderCell>Name</th>
      <th ogHeaderCell>Status</th>
    </tr>
  </thead>
  <tbody>
    @for (row of rows(); track row.id) {
      <tr>
        <td ogCell>{{ row.name }}</td>
        <td ogCell>{{ row.status }}</td>
      </tr>
    }
  </tbody>
</table>
```

---

## 4.1.12 — Performance and Build Optimisation

### How Tailwind 4 Builds

Tailwind 4 uses a Rust-based scanner that analyses your source files and generates only the CSS classes you actually use. Unlike Tailwind 3 (which required explicit `content` configuration), Tailwind 4 auto-detects sources via your bundler integration.

**In Angular, the integration is through the CSS preprocessor:**

```json
// angular.json — Tailwind processes through the PostCSS pipeline automatically
// No extra configuration needed in Tailwind 4 when using the Angular CLI
```

### Safelisting Dynamic Classes

The scanner only finds classes that exist as complete strings in your source. Dynamic class construction breaks this:

```typescript
// ❌ Purged: scanner doesn't find these classes
const color = 'danger';
return `bg-og-${color}/10 text-og-${color}`;

// ✅ Safe: complete class strings in a lookup map
const variantClasses = {
  danger:  'bg-og-danger/10 text-og-danger',
  warning: 'bg-og-warning/10 text-og-warning',
  success: 'bg-og-success/10 text-og-success',
};
return variantClasses[variant];
```

**Tailwind 4 safelist (for truly dynamic cases):**

```css
/* globals.css */
@source "../src/**/*.ts";
@source "../src/**/*.html";

/* Force-include these regardless of scanner */
@utility bg-og-critical-10 { background-color: oklch(from #FF2056 l c h / 0.10); }
```

### Bundle Analysis

```bash
# Check what Tailwind is generating
ng build --configuration production

# The CSS output in dist/ shows exactly what was included
# Target: < 20KB gzipped for your global CSS
```

### Performance rules:

- **No dynamic class construction** — use lookup maps (see above)
- **CSS custom properties over repeated utilities** — `--og-bg-surface` once in `@theme`, not `#111113` across 50 files
- **`@layer` for custom CSS** — prevents specificity conflicts with utilities

```css
/* Correct layer ordering */
@layer base {
  /* Element resets */
  * { @apply border-og-border; }
}

@layer components {
  /* Complex component styles that can't be extracted to Angular components */
}

@layer utilities {
  /* Custom one-off utilities */
  .scrollbar-none { scrollbar-width: none; }
}
```

---

## 4.1.13 — Scaling in Large Codebases

### Naming Conventions

Tailwind removes the need for BEM or SMACSS. But you still need conventions for the things Tailwind doesn't cover:

```
Angular component: kebab-case selector              app-connector-card
CSS custom property: --color-og-{role}             --color-og-accent
Design token group: og-{category}-{name}           og-bg-surface, og-text-muted
Animation: animate-{verb}                          animate-row-flash, animate-fade-in
```

### The Variant Map Pattern ★

Every component that has multiple visual variants should use an explicit lookup object — never string concatenation:

```typescript
// ✅ Variant map — every possible class is a complete string
const SEVERITY_CLASSES = {
  critical: 'bg-og-critical/10 text-og-critical border-og-critical/30',
  danger:   'bg-og-danger/10   text-og-danger   border-og-danger/30',
  warning:  'bg-og-warning/10  text-og-warning  border-og-warning/30',
  success:  'bg-og-success/10  text-og-success  border-og-success/30',
  info:     'bg-blue-500/10    text-blue-400    border-blue-500/30',
  muted:    'bg-zinc-600/10    text-zinc-400    border-zinc-700/30',
} as const satisfies Record<Severity, string>;
```

### Enforced Forbidden Patterns ★

These rules are CI-enforced in OpenGuard. Consider adding them to your ESLint config:

| Pattern | Why forbidden | Lint rule |
|---|---|---|
| Arbitrary values `w-[347px]` | Bypasses the design system | Custom rule: no-tailwind-arbitrary |
| `!important` modifier `!text-red-500` | Overrides specificity improperly | Custom rule |
| Inline styles `style={{color: 'red'}}` | Bypasses CSP, unmaintainable | `@angular-eslint/no-host-metadata-property` |
| Hardcoded hex in classes `bg-[#FF0000]` | Use design tokens | Custom rule |
| `text-` or `bg-` without token prefix | Leaks raw Tailwind colors into design system | Code review |

```json
// .eslintrc — custom Tailwind rules
{
  "rules": {
    "no-restricted-syntax": [
      "error",
      {
        "selector": "Literal[value=/\\[\\d+px\\]/]",
        "message": "Use design token scale, not arbitrary px values"
      }
    ]
  }
}
```

### The Design Audit Checklist

Before shipping a new component, run through this:

```
□ All colors from design tokens (og-* prefix), no raw Tailwind color scale
□ No arbitrary values [347px] — use the spacing/sizing scale
□ No inline styles — use Tailwind classes or host.class
□ All interactive elements have focus-visible styles
□ Animations wrapped in motion-safe: (or motion-reduce:hidden)
□ Text contrast meets WCAG AA (4.5:1 for body, 3:1 for large text)
□ All icons have aria-hidden="true" or aria-label
□ Variant maps use complete class strings (not concatenation)
□ New design tokens added to @theme and documented
```

---

## 4.1.14 — Quick Reference: OpenGuard Class Patterns ★

Common patterns used throughout the codebase. Copy these exactly — they encode production decisions.

```html
<!-- Section header -->
<div class="flex items-center justify-between mb-6">
  <h2 class="font-display text-lg font-medium text-og-text-primary">{{ title }}</h2>
  <og-button size="sm" (click)="action()">{{ cta }}</og-button>
</div>

<!-- Divider -->
<hr class="border-og-border my-6" />

<!-- Empty state -->
<div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
  <i-lucide name="inbox" [size]="32" class="text-og-text-muted" />
  <p class="text-sm text-og-text-secondary">No connectors found.</p>
  <og-button size="sm" variant="secondary" (click)="create()">
    Create your first connector
  </og-button>
</div>

<!-- Loading skeleton row -->
<div class="flex items-center gap-4 px-4 py-3 border-b border-og-border-subtle">
  <div class="size-8 rounded-full bg-og-bg-elevated animate-pulse shrink-0"></div>
  <div class="flex flex-col gap-1.5 grow">
    <div class="h-3.5 w-32 rounded bg-og-bg-elevated animate-pulse"></div>
    <div class="h-3 w-48 rounded bg-og-bg-elevated animate-pulse"></div>
  </div>
  <div class="h-6 w-16 rounded-full bg-og-bg-elevated animate-pulse shrink-0"></div>
</div>

<!-- Scrollable container with hidden scrollbar -->
<div class="overflow-y-auto scrollbar-none max-h-[calc(100vh-theme(spacing.32))]">

<!-- Keyboard shortcut badge -->
<kbd class="inline-flex items-center rounded border border-og-border
            bg-og-bg-elevated px-1.5 py-0.5
            font-mono text-[10px] text-og-text-muted">
  ⌘K
</kbd>

<!-- Code/token display -->
<span class="font-mono text-xs text-og-text-secondary bg-og-bg-elevated
             px-2 py-1 rounded border border-og-border-subtle
             select-all cursor-text">
  {{ token }}
</span>
```

---


## 4.2 Design Systems and Component Libraries ★

**SHOULD-KNOW** — A consistent design system prevents UI entropy in large teams.

OpenGuard builds its own component library on Angular CDK — no pre-styled library. This gives full control over the visual language (dark, dense, security-focused).

```typescript
// ★ Core pattern: typed variant inputs on all UI primitives
@Component({
  selector: 'og-badge',
  standalone: true,
  template: `
    <span [class]="variantClasses[variant()]"
          class="inline-flex items-center px-2 py-0.5 rounded text-xs
                 font-medium uppercase tracking-wide">
      <ng-content />
    </span>
  `
})
export class BadgeComponent {
  variant = input<'success' | 'warning' | 'danger' | 'critical' | 'info' | 'muted'>('muted');

  protected readonly variantClasses: Record<string, string> = {
    success:  'bg-green-500/10 text-green-500',
    warning:  'bg-amber-500/10 text-amber-500',
    danger:   'bg-red-500/10 text-red-500',
    critical: 'bg-[#FF2056]/10 text-[#FF2056]',
    info:     'bg-blue-500/10 text-blue-500',
    muted:    'bg-zinc-600/10 text-zinc-400',
  };
}
```

---

## 4.3 Accessibility (a11y)

**MUST-KNOW** — Accessibility is a legal requirement in many jurisdictions and a quality signal.

OpenGuard enforces WCAG AA compliance using `axe-playwright` on every E2E run. Requirements:

- All interactive elements keyboard-navigable (Tab, Enter, Space, Arrow keys)
- All form inputs have associated `<label>` elements
- Colour is never the sole differentiator — status badges always include a text label
- Meaningful icons have `aria-label` or visible text
- Modal dialogs trap focus and restore on close (Angular CDK `FocusTrap`)
- Data tables include `role="grid"` and `aria-rowcount`
- Chart components include a visually-hidden `<table>` fallback for screen readers
- All animations use `motion-safe:` Tailwind prefix

```html
<!-- ✅ Accessible icon button -->
<button
  (click)="copyToClipboard(value)"
  aria-label="Copy API key to clipboard"
  [attr.aria-pressed]="copied()"
  class="text-og-text-secondary hover:text-og-text-primary transition-colors"
>
  @if (copied()) {
    <i-lucide name="check" [size]="16" aria-hidden="true" />
  } @else {
    <i-lucide name="copy" [size]="16" aria-hidden="true" />
  }
</button>
```

---

## 4.4 Theming and Dark Mode

**SHOULD-KNOW** — Implement theming at the CSS custom property level, not the component level.

```css
/* src/styles/globals.css */
:root {
  --og-bg-base:      #09090B;
  --og-bg-surface:   #111113;
  --og-text-primary: #FAFAFA;
  --og-accent:       #06B6D4;
  /* ... all design tokens */
}

/* When light mode is added in Phase 2 */
.light {
  --og-bg-base:      #FFFFFF;
  --og-text-primary: #09090B;
  --og-accent:       #0284C7;
}
```

OpenGuard ships **dark mode only in v1** — the dashboard targets NOC environments. The `class="dark"` approach lets you toggle globally without rebuilding.

---



# Part 5 — Performance Engineering

> OpenGuard sets Lighthouse CI budgets that **block PRs**: LCP < 2.5s, TBT < 200ms, CLS < 0.1, Performance ≥ 85. First load JS per route < 150KB gzipped.

## 5.1 Change Detection Optimisation

**MUST-KNOW** — Default CD on a table with 200 rows and real-time updates is an instant performance problem.

```typescript
// ❌ WRONG — method called in template runs on every CD cycle
get criticalCount() {
  return this.alerts.filter(a => a.severity === 'critical').length;
}

// ✅ CORRECT — computed() memoises the value; only re-runs when alerts() changes
export class AlertTableComponent {
  alerts = toSignal(this.service.alerts$, { initialValue: [] });
  criticalCount = computed(() =>
    this.alerts().filter(a => a.severity === 'critical').length
  );
}
```

**Rules:**
- Every presentational component must use `ChangeDetectionStrategy.OnPush`
- Use `computed()` for all derived state
- Never call methods in templates — use Signals or `computed()` getters
- Never mutate objects in place — always replace the reference

---

## 5.2 Lazy Loading and Code Splitting ★

**MUST-KNOW** — Lazy loading is the most impactful bundle optimisation in Angular.

```typescript
// app.routes.ts — OpenGuard lazy loading pattern
export const routes: Routes = [
  {
    path: 'overview',
    loadComponent: () =>
      import('./features/dashboard/dashboard.component')
        .then(m => m.DashboardComponent),
  },
  {
    path: 'connectors',
    loadChildren: () =>
      import('./features/connectors/connectors.routes')
        .then(m => m.CONNECTOR_ROUTES),
  },
];
```

**Preloading strategy:**
- `PreloadAllModules` — preloads all lazy modules after initial load. Best for internal apps
- `NoPreloading` — minimises data usage on mobile
- Custom `PreloadingStrategy` — preloads only routes the user is likely to visit next

---

## 5.3 Bundle Optimisation

**MUST-KNOW** — A large bundle is a slow app. Parse time costs conversions.

- **Tree shaking** — use ES modules (`import`/`export`), never CommonJS `require()` in application code
- **Analyse bundles** — `ng build --stats-json` then `webpack-bundle-analyzer dist/stats.json`
- **Defer heavy libraries** — Charts must be lazy-loaded; not in the initial bundle
- **Pure pipes** — use pipes for template transformations; they are memoised by Angular
- **`track` by stable identity** — prevents Angular from recreating DOM nodes for unchanged items

```html
<!-- ✅ track by stable identity — prevents full list re-render -->
@for (connector of connectors(); track connector.id) {
  <app-connector-card [connector]="connector" />
}
```

```typescript
// ✅ Pure pipe — runs only when input changes
@Pipe({ name: 'timeAgo', pure: true, standalone: true })
export class TimeAgoPipe implements PipeTransform {
  transform(value: string): string {
    return formatDistanceToNow(new Date(value), { addSuffix: true });
  }
}
```

---

## 5.4 Rendering Strategies

**SHOULD-KNOW** — Choose based on content type and SEO requirements.

| Strategy | What it does | When to use |
|---|---|---|
| CSR (Client-Side Rendering) | Rendered entirely in browser | Authenticated dashboards, admin apps — no SEO needed |
| SSR (Server-Side Rendering) | Angular Universal renders HTML per request | Public apps where SEO matters |
| SSG (Static Site Generation) | HTML generated at build time | Documentation, marketing sites |

OpenGuard uses **CSR** — security operations dashboard accessed only by authenticated users; SEO is irrelevant, and CSR eliminates SSR complexity (hydration mismatches, server-client state sync).

**SSR gotchas when you do use it:**
- Never use `window`, `document`, or `localStorage` without checking `isPlatformBrowser()`
- Enable non-destructive hydration: `provideClientHydration()` in `app.config.ts`
- Set security headers in `server.ts`, not `index.html`

---

# Part 6 — Architecture & Code Organisation

## 6.1 Project Structure — Feature-Based Architecture ★

**MUST-KNOW** — The structure you choose in week 1 determines how painful refactoring is in year 2.

```
web/
├── src/
│   ├── app/
│   │   ├── core/                   # Singleton services, guards, interceptors
│   │   │   ├── services/           # ApiService, AuthService, SseService
│   │   │   ├── guards/             # authGuard, orgGuard (functional)
│   │   │   ├── interceptors/       # authInterceptor, errorInterceptor
│   │   │   └── models/             # Domain types mirroring the backend
│   │   │
│   │   ├── features/               # Smart (container) components
│   │   │   ├── dashboard/
│   │   │   ├── connectors/
│   │   │   │   ├── connector-list.component.ts
│   │   │   │   ├── connector-card.component.ts
│   │   │   │   ├── connector-form.component.ts
│   │   │   │   └── connectors.routes.ts
│   │   │   ├── audit/
│   │   │   └── threats/
│   │   │
│   │   ├── shared/                 # Dumb (presentational) components
│   │   │   ├── components/
│   │   │   │   ├── ui/             # Badge, Button, Input, Redactable
│   │   │   │   ├── data/           # DataTable, CursorTable
│   │   │   │   └── feedback/       # ConfirmDialog, ToastContainer
│   │   │   ├── pipes/              # TimeAgo, TruncateId, SeverityColor
│   │   │   └── directives/
│   │   │
│   │   ├── app.config.ts
│   │   ├── app.routes.ts
│   │   └── app.component.ts
│   │
│   ├── environments/
│   └── styles/
├── angular.json
└── tailwind.config.ts
```

---

## 6.2 Smart vs Dumb Components

**MUST-KNOW** — This distinction is the foundation of maintainable Angular UI.

| Smart (Container) | Dumb (Presentational) |
|---|---|
| Lives in `features/` | Lives in `shared/components/` |
| Injects services, manages data flow | Receives data via `@Input` / `input()` |
| Connected to router, auth, state | Emits events via `@Output` / `output()` |
| Uses Signals (preferred) | Uses `ChangeDetectionStrategy.OnPush` always |
| Contains no complex display logic | Has no knowledge of services or state |
| `ConnectorListComponent` | `ConnectorCardComponent`, `BadgeComponent` |

> **The Two-Level Prop-Drilling Rule:** If a prop would pass through more than two component layers, move the data to a service or Signal-based state. Do not pass props through intermediary components that do not use them.

---

## 6.3 API Layer Abstraction ★

**MUST-KNOW** — No component should ever call `fetch()` or `HttpClient` directly.

> **★ OpenGuard Rule (CI-Enforced)**
>
> *"No raw fetch in components — all API calls through `src/app/core/services/*`."*

The API layer provides: base URL injection, auth header injection, error normalisation, idempotency key management, and full type safety.

```typescript
// ★ The Interceptor Chain — from OpenGuard
// src/app/core/interceptors/auth.interceptor.ts
export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(AuthService);
  return next(req.clone({
    setHeaders: {
      'Authorization': `Bearer ${auth.accessToken()}`,
      'X-Org-ID': auth.orgId() ?? '',
    }
  }));
};

// src/app/core/interceptors/error.interceptor.ts
export const errorInterceptor: HttpInterceptorFn = (req, next) => {
  const toast = inject(NotificationService);
  return next(req).pipe(
    catchError((error: HttpErrorResponse) => {
      const apiError = error.error?.error as APIError;
      const message = ERROR_MESSAGES[apiError?.code] ?? 'An unexpected error occurred';
      if (error.status !== 401) toast.error(message); // 401 handled by auth redirect
      return throwError(() => ({ ...apiError, message }));
    })
  );
};

// Register in app.config.ts — order matters (auth before error)
provideHttpClient(withInterceptors([authInterceptor, errorInterceptor]))
```

---

## 6.4 Forbidden Patterns ★

| Pattern | Why forbidden | Alternative |
|---|---|---|
| `localStorage` for tokens/org_id | XSS-accessible — security boundary violated | `httpOnly` cookies via `AuthService` |
| Raw `fetch()` in components | No auth injection, no error normalisation | Inject the relevant service |
| `any` type | Defeats TypeScript — type safety gone | Define proper types in `core/models/` |
| `inline style={{}}` for visual styling | Bypasses CSP, hard to maintain | Tailwind classes or component variants |
| Single-click destructive actions | Easy to trigger accidentally in production | `ConfirmDialog` requiring typed resource name |
| Hard-coded `org_id` strings | Breaks multi-tenancy completely | `auth.orgId()` signal from `AuthService` |
| `console.log` in committed code | Leaks sensitive data to browser DevTools | Remove before commit; use structured logging |
| `setInterval` for polling | Not cleanup-safe — leaks if component destroyed | RxJS `timer()` or `interval()` |
| Prop drilling beyond 2 levels | Tight coupling, maintenance nightmare | Service or Signal-based state |

---

# Part 7 — Testing Strategy

> The most common mistake: too many low-value unit tests on implementation details and not enough high-value integration and E2E tests on behaviour.

## 7.1 Testing Pyramid ★

| Layer | Tool | Scope | Threshold |
|---|---|---|---|
| Unit (utils, validators) | Jasmine / Jest | Pure functions, pipes, validators | 80% coverage |
| Component tests | Angular Testing Library | UI behaviour from user perspective | 80% coverage |
| Integration (API services) | `HttpClientTestingModule` | Request/response contracts | 100% of API modules |
| E2E (critical paths) | Playwright | Full user journeys | All critical flows |
| Accessibility | `axe-playwright` | Every E2E page | 0 WCAG AA violations |
| Performance | Lighthouse CI | Core Web Vitals | Per-PR budgets |

---

## 7.2 What to Test vs What Not to Test

### Test these
- All utility functions in `core/utils/` — cursor encoding, error message mapping, idempotency key generation
- All Zod validators — both valid and invalid inputs
- All Angular Guards — authenticated, unauthenticated, MFA-pending states
- All HTTP Interceptors — header injection, error normalisation, 401 redirect
- Component user interactions — clicks, form submissions, keyboard navigation
- Error states — what the user sees when an API call fails
- Loading states — skeleton screens, disabled buttons during pending operations

### Do NOT test these
- Angular framework behaviour — testing that `[disabled]` works is testing Angular, not your code
- Implementation details — private service method internals
- Styling — assert visible text, ARIA roles, interactive state; not CSS classes
- Third-party library internals — `HttpClient`, `Router`, `Forms`

---

## 7.3 Component Testing Pattern ★

```typescript
// ★ src/app/features/connectors/connector-card.spec.ts
import { render, screen, fireEvent } from '@testing-library/angular';
import { ConnectorCardComponent } from './connector-card.component';
import { UiService } from '@core/state/ui.service';

describe('ConnectorCardComponent', () => {
  const mockConnector: Connector = {
    id: 'conn_01',
    name: 'AcmeApp',
    status: 'active',
    scopes: ['events:write'],
    // ... other required fields
  };

  it('shows confirm dialog before suspending', async () => {
    const confirmSpy = jasmine.createSpy('confirm')
      .and.returnValue(Promise.resolve(true));

    await render(ConnectorCardComponent, {
      componentInputs: { connector: mockConnector },
      providers: [{ provide: UiService, useValue: { confirm: confirmSpy } }]
    });

    fireEvent.click(screen.getByRole('button', { name: /suspend/i }));
    expect(confirmSpy).toHaveBeenCalledWith(jasmine.objectContaining({
      title: 'Suspend connector?',
      variant: 'destructive',
    }));
  });

  it('shows suspended badge when status is suspended', async () => {
    await render(ConnectorCardComponent, {
      componentInputs: { connector: { ...mockConnector, status: 'suspended' } }
    });
    expect(screen.getByText('SUSPENDED')).toBeTruthy();
  });
});
```

---

## 7.4 E2E Testing with Playwright ★

**MUST-KNOW** — E2E tests on critical paths catch regressions that unit tests cannot.

OpenGuard's critical E2E paths (all must pass before release):

| Test file | Flow |
|---|---|
| `auth.spec.ts` | Login → TOTP MFA → redirect to overview |
| `connector-lifecycle.spec.ts` | Register → copy key → suspend → activate → delete |
| `audit-stream.spec.ts` | Open audit page → verify SSE events appear in table |
| `threat-alert.spec.ts` | View alert → acknowledge → resolve → verify MTTR shown |
| `session-revoke.spec.ts` | Revoke session from admin → user gets signed out |
| `key-reveal.spec.ts` | Connector registration → key reveal → confirm saved |

```typescript
// e2e/connector-lifecycle.spec.ts
import { test, expect } from '@playwright/test';

test('connector lifecycle — register to delete', async ({ page }) => {
  await page.goto('/connectors');
  await page.getByRole('button', { name: 'Register connector' }).click();

  // Fill form
  await page.getByLabel('Name').fill('Test App');
  await page.getByLabel('events:write').check();
  await page.getByRole('button', { name: 'Create' }).click();

  // API key reveal — must acknowledge saving
  await expect(page.getByText('Save this API key')).toBeVisible();
  await page.getByRole('button', { name: "I've saved the key securely" }).click();

  // Suspend — confirm dialog must appear
  await page.getByRole('button', { name: 'Suspend' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.getByRole('button', { name: 'Suspend', exact: true }).click();
  await expect(page.getByText('SUSPENDED')).toBeVisible();
});
```

---

## 7.5 HTTP Service Integration Testing

```typescript
describe('ConnectorsService', () => {
  let service: ConnectorsService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [HttpClientTestingModule],
    });
    service = TestBed.inject(ConnectorsService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify()); // assert no outstanding requests

  it('lists connectors for an org', (done) => {
    service.list('org_01').subscribe(page => {
      expect(page.data.length).toBe(1);
      done();
    });

    const req = http.expectOne('/v1/connectors?page=1&per_page=50');
    expect(req.request.headers.get('X-Org-ID')).toBe('org_01');
    req.flush({ data: [{ id: 'conn_01', name: 'Test' }], meta: { page: 1, total: 1 } });
  });
});
```

---

# Part 8 — Security

## 8.1 XSS, CSRF, and CORS

### XSS (Cross-Site Scripting)

Angular sanitises all dynamic content by default. The danger zones:

- `innerHTML` binding — Angular allows it but do not use it unless you fully control the content
- `bypassSecurityTrustHtml` — disables Angular's XSS protection; requires code review
- Inline `<script>` tags — forbidden by CSP; never dynamically inject them

```typescript
// ✅ Angular escapes this automatically
// <p>{{ userInput }}</p>

// ❌ Dangerous — bypasses sanitisation
// <div [innerHTML]="userInput"></div>

// ✅ If you must use innerHTML — sanitise explicitly
this.safeHtml = this.sanitizer.sanitize(SecurityContext.HTML, userInput);
```

### CSRF

Angular's `HttpClientXsrfModule` automatically reads the `XSRF-TOKEN` cookie and sends it as the `X-XSRF-TOKEN` header on non-GET requests.

```typescript
// app.config.ts
provideHttpClient(
  withXsrfConfiguration({
    cookieName: 'XSRF-TOKEN',
    headerName: 'X-XSRF-TOKEN',
  })
)
```

---

## 8.2 Authentication Patterns ★

**MUST-KNOW** — Auth is the most security-sensitive part of any web app.

### OpenGuard OIDC + PKCE + TOTP/WebAuthn Flow

1. User clicks Sign in — Angular generates PKCE `code_verifier` + `code_challenge`
2. Redirect to IAM provider with `code_challenge` and `state` parameter
3. IAM authenticates, returns authorization code
4. Angular exchanges `code` + `code_verifier` for tokens
5. If `mfa_required` in token → redirect to `/mfa/totp` or `/mfa/webauthn`
6. Session stored in `httpOnly` cookies — **never `localStorage`**

> **The localStorage Trap**
>
> NEVER store access tokens, refresh tokens, or `org_id` in `localStorage`. It is accessible by any JavaScript on the page — including injected third-party scripts and XSS payloads.
>
> **★ OpenGuard Rule (CI-Enforced):** *"No tokens or `org_id` in localStorage — secure cookies via `AuthService` only. Zero exceptions."*

```typescript
// ★ AuthService — Signals for session state
@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly session = signal<AuthSession | null>(null);

  readonly isAuthenticated = computed(() => !!this.session());
  readonly accessToken = computed(() => this.session()?.accessToken);
  readonly orgId = computed(() => this.session()?.orgId); // from session — NEVER from URL
  readonly mfaVerified = computed(() => this.session()?.mfaVerified ?? false);
}

// Functional Auth Guard
export const authGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);

  if (!auth.isAuthenticated()) return router.parseUrl('/login');
  if (!auth.mfaVerified()) return router.parseUrl('/mfa/totp');
  return true;
};
```

---

## 8.3 Sensitive Data Display ★

**MUST-KNOW** for apps handling PII, security keys, or credentials.

OpenGuard uses a `Redactable` component that respects org-level data visibility settings. When `data_visibility === 'restricted'`, sensitive values are masked with a reveal toggle. Reveals are per-session and logged as audit events.

```html
<!-- ✅ Always use Redactable for sensitive fields -->
<og-redactable [value]="event.actor_email" type="email" />
<og-redactable [value]="event.source_ip" type="ip" />
<og-redactable [value]="connector.api_key_prefix" type="api-key" />
```

> **★ OpenGuard Rule (CI-Enforced):** *"No sensitive data (email, ip_address, token_prefix) outside `RedactableComponent`."*

---

## 8.4 Content Security Policy

**SHOULD-KNOW** — CSP is your last line of defence against XSS.

```
Content-Security-Policy:
  default-src 'self';
  script-src  'self';
  style-src   'self' 'unsafe-inline';
  img-src     'self' data:;
  connect-src 'self' https://api.openguard.internal;
  font-src    'self';
  frame-ancestors 'none';
```

Set CSP in `server.ts` (Angular Universal) or your web server (Nginx/CloudFront) — not in `index.html`.

---

# Part 9 — Dev Experience & Tooling

## 9.1 Angular CLI Deep Usage

**MUST-KNOW** — The CLI is your primary productivity tool.

```bash
# Create new app — always standalone
ng new my-app --standalone --style css --routing

# Generate standalone component
ng generate component features/audit/audit-list --standalone

# Build and analyse
ng build --configuration production
ng build --stats-json && npx webpack-bundle-analyzer dist/stats.json

# Development with API proxy
ng serve --proxy-config proxy.conf.json

# Testing
ng test --code-coverage
ng test --watch=false --browsers=ChromeHeadless  # CI mode

# Migrations
ng update @angular/core @angular/cli
ng generate @angular/core:standalone-migration   # migrate NgModules
```

**`angular.json` configuration worth knowing:**
- `budgets` — set initial bundle size limits; CLI fails the build if exceeded
- `fileReplacements` — how environment files are swapped at build time
- `optimization: true` — enables Terser, CSS minification, tree shaking in production

---

## 9.2 ESLint Configuration ★

**MUST-KNOW** — Automated linting catches entire categories of bugs before code review.

```json
{
  "extends": ["@angular-eslint/recommended"],
  "rules": {
    "@angular-eslint/prefer-standalone": "error",
    "@angular-eslint/prefer-on-push-component-change-detection": "error",
    "@typescript-eslint/no-explicit-any": "error",
    "@typescript-eslint/no-unused-vars": "error",
    "no-console": "error",
    "@angular-eslint/no-lifecycle-call": "error",
    "prefer-const": "error",
    "eqeqeq": ["error", "always"]
  }
}
```

---

## 9.3 Monorepo with Nx

**SHOULD-KNOW** — Nx is the standard choice for large Angular monorepos.

```
apps/
  openguard-admin/       # Main Angular app
  openguard-public/      # Marketing site
libs/
  shared/
    ui/                  # Design system (BadgeComponent, ButtonComponent)
    models/              # Shared TypeScript types
    utils/               # Pure utility functions
    auth/                # Auth service and guards
```

**Key Nx benefits:**
- `nx affected:build` — only rebuilds projects affected by a change; CI time drops dramatically
- `nx graph` — visualises dependency graph; prevents circular dependencies
- Module boundary enforcement — feature modules cannot import from each other directly

---

## 9.4 CI/CD for Frontend ★

**MUST-KNOW** — Automated quality gates prevent regressions from reaching production.

```yaml
# .github/workflows/ci.yml — OpenGuard frontend jobs
jobs:
  fe-type-check:
    run: npx tsc --noEmit
    # Fails on any TypeScript error

  fe-lint:
    run: npm run lint
    # Enforces: no-console, no-explicit-any, prefer-standalone

  fe-test:
    run: npx karma start --single-run
    # Coverage gate: 80% statements per file

  fe-build:
    run: ng build --configuration production
    # Fails on: TypeScript errors, bundle size limits exceeded

  fe-e2e:
    run: npx playwright test
    # Runs all 12 critical path specs + axe accessibility

  fe-lighthouse:
    run: npx lhci autorun
    # LCP < 2.5s, Performance ≥ 85, Accessibility ≥ 95
```

---

## 9.5 Environment Management ★

**MUST-KNOW** — Mismanaged environment config is a common source of production incidents.

```typescript
// src/environments/environment.ts — build-time
export const environment = {
  production: false,
  apiUrl: 'http://localhost:8080',
  features: {
    dlpBlockMode: true,
    webauthn: true,
  }
};

// ConfigService — runtime configuration (preferred for production)
// Loaded from /assets/config.json at startup via APP_INITIALIZER
@Injectable({ providedIn: 'root' })
export class ConfigService {
  private config = signal<AppConfig | null>(null);

  loadConfig() {
    return this.http.get<AppConfig>('/assets/config.json').pipe(
      tap(cfg => this.config.set(cfg))
    );
  }
}

// app.config.ts — blocks bootstrap until config loads
{
  provide: APP_INITIALIZER,
  useFactory: (cfg: ConfigService) => () => cfg.loadConfig(),
  deps: [ConfigService],
  multi: true,
}
```

> **Environment Security Rules:**
> - Never commit `.env` files — use `.env.example` with placeholders
> - Never hardcode secrets in `environment.ts` — use server-side environment variables
> - Validate all required config at startup — fail loudly rather than silently at request time
> - Only expose values safe for the browser via `NEXT_PUBLIC_` or Angular environment flags

---

# Part 10 — Real-World Patterns

## 10.1 Forms — Reactive Forms with Zod Validation ★

**MUST-KNOW** — Reactive Forms are the only permitted form mechanism in OpenGuard.

Zod schemas serve as both the TypeScript type source and the validation logic — **one source of truth**.

```typescript
// ★ Step 1: Define Zod schema (single source of truth)
// src/app/core/validators/connector.validator.ts
export const connectorCreateSchema = z.object({
  name: z.string()
    .min(2, 'Name must be at least 2 characters')
    .max(64, 'Name must be at most 64 characters'),
  webhook_url: z.string()
    .url('Must be a valid URL')
    .refine(url => url.startsWith('https://'), 'Webhook URL must use HTTPS')
    .optional()
    .or(z.literal('')),
  scopes: z.array(z.enum(CONNECTOR_SCOPES))
    .min(1, 'At least one scope is required'),
});

export type ConnectorCreateInput = z.infer<typeof connectorCreateSchema>;

// Step 2: Use in component
@Component({ ... })
export class ConnectorFormComponent {
  private fb = inject(FormBuilder);

  form = this.fb.group({
    name: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(64)]],
    webhook_url: [''],
    scopes: this.fb.array([]),
  });

  isPending = signal(false);

  onSubmit() {
    if (this.form.invalid) { this.form.markAllAsTouched(); return; }

    // Validate with Zod before sending — catches edge cases TypeScript misses
    const result = connectorCreateSchema.safeParse(this.form.value);
    if (!result.success) {
      result.error.issues.forEach(issue => {
        const ctrl = this.form.get(issue.path[0].toString());
        ctrl?.setErrors({ zod: issue.message });
      });
      return;
    }

    this.isPending.set(true);
    this.service.create(this.auth.orgId()!, result.data, crypto.randomUUID())
      .subscribe({
        next: () => this.isPending.set(false),
        error: () => this.isPending.set(false),
      });
  }
}
```

---

## 10.2 Error Handling Strategy ★

**MUST-KNOW** — Good error handling is invisible to happy-path users and essential for everyone else.

OpenGuard's three-layer strategy:

1. **HTTP Interceptor (global)** — catches all HTTP errors, maps backend codes to user messages, shows toast for non-401 errors
2. **Component error state (local)** — for errors affecting what is displayed; show error boundary with retry button
3. **Form validation errors (field-level)** — inline beneath each field

```typescript
// Error message mapping — backend codes → user messages
export const ERROR_MESSAGES: Record<string, string> = {
  RESOURCE_NOT_FOUND:   'This resource no longer exists.',
  RESOURCE_CONFLICT:    'A resource with these details already exists.',
  FORBIDDEN:            'You do not have permission to perform this action.',
  UPSTREAM_UNAVAILABLE: 'A dependent service is temporarily unavailable. Please try again shortly.',
  DLP_POLICY_VIOLATION: 'Event blocked: DLP policy violation detected.',
  SESSION_REVOKED_RISK: 'Your session was revoked due to suspicious activity.',
  TOTP_REPLAY_DETECTED: 'This MFA code has already been used. Wait for the next one.',
};
```

```html
<!-- Template: error state with retry -->
@if (isLoading()) {
  <app-skeleton-table [rows]="8" />
} @else if (error()) {
  <app-error-boundary
    [message]="error()"
    (retry)="loadConnectors()" />
} @else {
  <!-- happy path -->
}
```

---

## 10.3 Real-Time SSE Streams ★

**MUST-KNOW** if your app has live data.

OpenGuard uses Server-Sent Events (SSE) for the audit log live stream and threat alert notifications. SSE is simpler than WebSockets for server-to-client-only streams: no bidirectional protocol overhead, native browser reconnection, works through HTTP/2.

```typescript
// ★ SseService — wraps EventSource in an Observable
@Injectable({ providedIn: 'root' })
export class SseService {
  connected = signal(false);

  stream<T>(path: string): Observable<T> {
    return new Observable<T>(observer => {
      const source = new EventSource(
        `${environment.apiUrl}${path}`,
        { withCredentials: true }
      );

      source.onopen = () => this.connected.set(true);
      source.onmessage = (e) => observer.next(JSON.parse(e.data) as T);
      source.onerror = (err) => {
        this.connected.set(false);
        observer.error(err);
      };

      // Cleanup — close EventSource on unsubscribe
      return () => source.close();
    });
  }
}

// Usage with automatic reconnection
export class AuditStreamComponent {
  private sse = inject(SseService);
  private destroyRef = inject(DestroyRef);

  events = signal<SSEAuditEvent[]>([]);

  ngOnInit() {
    this.sse.stream<SSEAuditEvent>('/api/stream/audit').pipe(
      takeUntilDestroyed(this.destroyRef),
      catchError((err, caught) =>
        timer(2000).pipe(switchMap(() => caught)) // reconnect after 2s
      )
    ).subscribe(event => {
      // Cap at 200 events to prevent memory growth
      this.events.update(prev => [event, ...prev].slice(0, 200));
    });
  }
}
```

---

## 10.4 Pagination Patterns ★

**MUST-KNOW** — Pagination strategy affects both UX and API performance.

| Offset Pagination | Cursor Pagination |
|---|---|
| For: CRUD lists (users, policies, connectors) | For: append-only streams (audit, alerts, DLP findings) |
| Supports total count and page numbers | No total count — "Load more" or infinite scroll |
| Breaks if items deleted between pages (row shift) | Stable — deletion does not affect cursor position |
| URL: `?page=2` | URL: `?cursor=<encoded>` |

```typescript
// ★ URL-synced pagination — source of truth is the URL
export class ConnectorListComponent {
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  // Derive current page from URL
  currentPage = toSignal(
    this.route.queryParams.pipe(map(p => Number(p['page'] ?? 1))),
    { initialValue: 1 }
  );

  // Load data when page signal changes
  connectorPage = toSignal(
    toObservable(this.currentPage).pipe(
      switchMap(page => this.service.list(this.auth.orgId()!, page))
    ),
    { initialValue: null }
  );

  setPage(page: number) {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { page },
      queryParamsHandling: 'merge', // preserve filters
    });
  }
}
```

---

## 10.5 Feature Flags

**SHOULD-KNOW** — Feature flags enable progressive deployment without code branching.

```typescript
// Environment flags (compile-time)
features: {
  dlpBlockMode: true,
  webauthn: true,
  scim: true,
}

// Runtime flags (preferred for production — no rebuild needed)
// Loaded from /assets/config.json via ConfigService

// Route guard using feature flag
export const dlpGuard: CanActivateFn = () => {
  const config = inject(ConfigService);
  return config.get('features').dlpBlockMode
    ? true
    : inject(Router).parseUrl('/404');
};
```

---

## 10.6 Logging and Monitoring

**MUST-KNOW** — You are flying blind in production without it.

```typescript
// Global error handler — catches all uncaught Angular errors
@Injectable()
export class GlobalErrorHandler implements ErrorHandler {
  private toast = inject(NotificationService);

  handleError(error: unknown): void {
    // Forward to Sentry with context for backend correlation
    Sentry.captureException(error, {
      tags: { source: 'angular-error-handler' }
    });

    // Show recoverable UI message — never expose raw errors to users
    this.toast.error('An unexpected error occurred. Our team has been notified.');
  }
}

// Register in app.config.ts
{ provide: ErrorHandler, useClass: GlobalErrorHandler }
```

> **★ OpenGuard Rules:**
> - `console.log` is forbidden in committed code — CI-enforced
> - All uncaught errors forwarded to Sentry with request ID and trace ID for backend correlation
> - Security-relevant user actions (reveal sensitive data, delete resources) logged as audit events via `POST /v1/events/ingest`

---

## 10.7 Internationalisation (i18n)

**NICE-TO-KNOW** unless your app has a global audience.

| Approach | Pros | Cons |
|---|---|---|
| Built-in Angular i18n | Framework native, compile-time extraction | Separate build per locale |
| ngx-translate | Runtime locale switching, flexible | Slightly larger bundle |
| Transloco | Signal-aware, great TypeScript support | Newer ecosystem |

Always use Angular's locale-aware pipes — never raw JavaScript date/number formatting in templates:

```html
<!-- ✅ Locale-aware pipes -->
{{ event.occurred_at | date:'medium' }}      <!-- "Jan 15, 2025, 3:24 PM" -->
{{ riskScore | number:'1.1-2' }}             <!-- "0.95" -->
{{ amount | currency:'USD':'symbol' }}       <!-- "$1,234.56" -->
```

---

# Part 11 — The Production Readiness Checklist

Before you ship an Angular feature to production, run through this checklist.

## 11.1 Code Quality
- [ ] TypeScript `strict: true` with no `@ts-ignore` or `any` escape hatches
- [ ] `noUncheckedIndexedAccess` enabled — all array access handles the `undefined` case
- [ ] ESLint passes with no `console.log`, no explicit `any`, no unused variables
- [ ] All components are standalone with `OnPush` or Signal-based change detection
- [ ] No `NgModules` in new code
- [ ] All templates use `@if`, `@for`, `@switch` — no legacy structural directives
- [ ] All observables managed with `toSignal()`, `async` pipe, or `takeUntilDestroyed()`

## 11.2 Security
- [ ] No tokens stored in `localStorage` or `sessionStorage` — `httpOnly` cookies only
- [ ] Auth guard applied to all authenticated routes
- [ ] CSRF protection enabled (`provideHttpClient(withXsrfConfiguration(...))`)
- [ ] CSP headers configured — no `unsafe-eval`, inline scripts restricted
- [ ] All sensitive data (emails, IPs, keys) routed through `Redactable` component
- [ ] All destructive actions require a confirmation dialog
- [ ] Session revocation handled — 401 responses trigger re-authentication

## 11.3 Performance
- [ ] Lighthouse CI passing: LCP < 2.5s, TBT < 200ms, CLS < 0.1, Performance ≥ 85
- [ ] All routes are lazy-loaded
- [ ] Heavy libraries (charts, PDF generators) are dynamically imported
- [ ] Bundle size per route < 150KB gzipped
- [ ] All lists use `track connector.id` — no unnecessary DOM recreation
- [ ] Pure pipes used for all template transformations
- [ ] `prefers-reduced-motion` respected for all animations

## 11.4 Testing
- [ ] Unit test coverage ≥ 80% for all utilities, validators, guards, interceptors
- [ ] Component tests cover: loading state, error state, empty state, happy path
- [ ] 100% of API service modules have integration tests
- [ ] All critical user journeys covered by Playwright E2E tests
- [ ] Zero WCAG AA violations in `axe-playwright` scans

## 11.5 Observability
- [ ] `GlobalErrorHandler` configured — all uncaught errors forwarded to Sentry/Datadog
- [ ] Request IDs attached to all API calls for backend correlation
- [ ] Performance monitoring active (Web Vitals, Lighthouse CI on PRs)
- [ ] No `console.log` in committed code — structured logging service in use

## 11.6 Architecture
- [ ] Feature-based folder structure — each feature owns its own components, services, routes
- [ ] Smart/dumb component separation — presentational components in `shared/`
- [ ] No prop drilling beyond two levels
- [ ] API layer abstraction — no raw `fetch`/`HttpClient` in components
- [ ] Error normalisation — all backend error codes mapped to user-friendly messages
- [ ] Forbidden patterns documented, linted, and enforced in CI

---

*Angular Frontend Mastery Guide · Built from the OpenGuard production codebase · Angular 19 · Signals · Standalone · Playwright*