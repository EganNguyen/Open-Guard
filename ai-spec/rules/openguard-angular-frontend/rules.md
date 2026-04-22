---
name: openguard-angular-frontend
description: >
  Use this rule whenever writing, reviewing, or extending any Angular frontend
  code in the OpenGuard Admin Dashboard (web/). Covers all mandatory patterns:
  Angular 19+ standalone components, Signals for reactive state, HttpClient
  services for API access, OIDC + MFA guarding, SSE real-time streams via
  SseService, Reactive Forms + Zod, and security hardening (CSP, secure
  cookies, Redactable component). All rules below are CI-enforced — violation =
  PR blocked.
license: Internal — OpenGuard Engineering
---

# OpenGuard — Angular Frontend Rule

> Read files 00–03 of the FE spec before any feature work.
> Every pattern here is canonical and CI-enforced.

---

## 0. Absolute Rules (CI-enforced, no exceptions)

```
✗ No raw fetch in components — all API calls through src/app/core/services/*
✗ No tokens or org_id in localStorage — secure cookies via AuthService only
✗ No org-scoped route without AuthGuard + OrgGuard check
✗ No org_id interpolated from URL params — always from authenticated session
✗ No uncontrolled inputs — all forms use Angular Reactive Forms + Zod
✗ No raw WebSocket connections from client — use SseService for all real-time streams
✗ No single-click destructive actions — ConfirmDialog with resource name typed
✗ No page without error boundary — unhandled errors must show recoverable UI
✗ No sensitive data (email, ip_address, token_prefix) outside RedactableComponent
✗ No inline scripts or inline styles outside Scoped CSS / Tailwind
✗ No any type — defeats TypeScript — CI lint failure
✗ No console.log in committed code — leaks sensitive data to browser DevTools
✗ No manual subscriptions for data — use async pipe or toSignal()
✗ No polling with setInterval — use RxJS timer() or signal-based polling
✗ No hard-coded org_id strings — use AuthService.currentOrgId() signal
```

---

## 1. Project Structure

```
web/
├── src/
│   ├── app/
│   │   ├── core/                   # Singleton services, guards, interceptors, models
│   │   │   ├── services/           # ApiService, AuthService, SseService
│   │   │   ├── guards/             # AuthGuard, OrgGuard
│   │   │   ├── interceptors/       # AuthInterceptor, ErrorInterceptor
│   │   │   └── models/             # Domain models (mirrors BE)
│   │   ├── features/               # Smart components (feature modules)
│   │   │   ├── dashboard/
│   │   │   ├── connectors/
│   │   │   ├── audit/
│   │   │   └── ...
│   │   ├── shared/                 # Dumb components, pipes, directives
│   │   │   ├── components/         # Button, Input, Redactable
│   │   │   ├── pipes/
│   │   │   └── directives/
│   │   ├── app.config.ts           # Application providers
│   │   ├── app.routes.ts           # Routing configuration
│   │   └── app.component.ts        # Root component
│   ├── assets/
│   ├── environments/
│   └── styles/                     # Global styles, Tailwind
├── angular.json
├── package.json
└── tsconfig.json
```

---

## 2. Angular Core Rules — Standalone & Reactive

### 2.1 Standalone by default

Every component, directive, and pipe MUST be standalone. Modular Angular (NgModules) is forbidden.

```ts
// ✓ correct — Standalone Component
@Component({
  selector: 'app-connector-list',
  standalone: true,
  imports: [CommonModule, ConnectorCardComponent],
  templateUrl: './connector-list.component.html',
})
export class ConnectorListComponent { ... }
```

### 2.2 Signals for State

Signals are the mandatory way to manage reactive state. Do not use manual RxJS subscriptions in components where a Signal or `toSignal()` suffices.

```ts
// ✓ correct — Signal-based state
export class AuditStreamComponent {
  private auditService = inject(AuditService);
  
  // Convert Observable to Signal
  events = toSignal(this.auditService.stream$, { initialValue: [] });
  
  // Derived state
  eventCount = computed(() => this.events().length);
}
```

### 2.3 Functional Guards & Interceptors

Use functional patterns for Router Guards and Http Interceptors. Class-based guards/interceptors are deprecated in this project.

```ts
// src/app/core/guards/auth.guard.ts
export const authGuard: CanActivateFn = (route, state) => {
  const authService = inject(AuthService);
  const router = inject(Router);

  if (authService.isAuthenticated()) return true;
  return router.parseUrl('/login');
};
```

### 2.4 Control Flow Syntax

Always use the `@if`, `@for`, `@switch` block syntax. Legacy structural directives (`*ngIf`, `*ngFor`) are forbidden.

```html
@for (connector of connectors(); track connector.id) {
  <app-connector-card [connector]="connector" />
} @empty {
  <p>No connectors found.</p>
}
```

---

## 3. TypeScript — strict: true

```json
// tsconfig.json
{
  "compilerOptions": {
    "strict": true,          // required — no exceptions
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true
  }
}
```

### 3.1 Template Typing — use `interface` for Inputs/Outputs

```ts
// ✓ correct
@Component({ ... })
export class ConnectorCardComponent {
  // Use Signal Inputs for better reactivity
  connector = input.required<Connector>();
  
  // Use Output emitters
  suspend = output<string>();
  
  // Strict typing for class members
  isLoading = signal(false);
}
```

### 3.2 Type guard pattern for SSE events

```ts
// src/app/core/models/events.ts
export function isAuditEvent(e: unknown): e is AuditEvent {
  return typeof e === 'object' && e !== null && (e as any).event_type?.startsWith('audit.');
}
```

### 3.3 Domain Models (Shared Contracts)

All domain types must be imported from `src/app/core/models/`. These types must mirror the backend shared models.

```ts
import { Connector, AuditEvent } from '@core/models';
```

---

## 4. API Client Layer

### 4.1 ApiService (HttpClient wrapper)

The `ApiService` is the only way to communicate with the backend. It handles base URL injection, headers, and error mapping via `HttpInterceptor`.

```ts
// src/app/core/services/api.service.ts
@Injectable({ providedIn: 'root' })
export class ApiService {
  private http = inject(HttpClient);
  private env = inject(EnvironmentService);

  get<T>(path: string, params?: HttpParams): Observable<T> {
    return this.http.get<T>(`${this.env.apiUrl}${path}`, { params });
  }

  post<T>(path: string, body: any, options?: { idempotencyKey?: string }): Observable<T> {
    let headers = new HttpHeaders();
    if (options?.idempotencyKey) {
      headers = headers.set('Idempotency-Key', options.idempotencyKey);
    }
    return this.http.post<T>(`${this.env.apiUrl}${path}`, body, { headers });
  }
}
```

### 4.2 Error Interceptor

The `ErrorInterceptor` maps backend error codes to user-friendly messages using the `ERROR_MESSAGES` constant.

```ts
// src/app/core/interceptors/error.interceptor.ts
export const errorInterceptor: HttpInterceptorFn = (req, next) => {
  return next(req).pipe(
    catchError((error: HttpErrorResponse) => {
      const apiError = error.error?.error as APIError;
      const message = ERROR_MESSAGES[apiError?.code] || 'An unexpected error occurred';
      // notify ToastService
      return throwError(() => new Error(message));
    })
  );
};
```

---

## 5. State Management

### 5.1 Signals for Reactive State

Signals are the primary tool for state management. Services should expose `WritableSignal` for internal mutation and `Signal` (via `asReadonly()`) for consumers.

```ts
// src/app/core/services/ui.service.ts
@Injectable({ providedIn: 'root' })
export class UiService {
  private sidebarCollapsed = signal(false);
  isSidebarCollapsed = this.sidebarCollapsed.asReadonly();

  toggleSidebar(): void {
    this.sidebarCollapsed.update(v => !v);
  }
}
```

### 5.2 Shared State Rules

- **Never** manually subscribe to observables in components if `toSignal()` or `async` pipe exists.
- **Never** duplicate server data in local component state.
- **Always** use `computed()` for derived state to ensure optimal change detection.

---

## 6. Authentication & Session

### 6.1 AuthService (OIDC & Session)

The `AuthService` handles OIDC login flows, token refresh, and exposes the current user/org as signals.

```ts
// src/app/core/services/auth.service.ts
@Injectable({ providedIn: 'root' })
export class AuthService {
  private currentUser = signal<User | null>(null);
  user = this.currentUser.asReadonly();
  
  currentOrgId = computed(() => this.currentUser()?.org_id);

  isAuthenticated = computed(() => !!this.currentUser());

  login(): void { /* trigger OIDC redirect */ }
  logout(): void { /* clear cookies and redirect */ }
}
```

### 6.2 Session Security

- **Secure Cookies**: All tokens MUST be stored in secure, same-site cookies.
- **LocalStorage Forbidden**: Never store sensitive data like `access_token` or `org_id` in localStorage.
- **CSRF Protection**: All mutations must include the `X-XSRF-TOKEN` header (handled by `HttpClientXsrfModule`).

---

## 7. Forms & Validation

Angular Reactive Forms are the ONLY permitted form management system. Integration with Zod is preferred for complex validation logic.

```ts
// ✓ correct — Reactive Forms
@Component({ ... })
export class ConnectorFormComponent {
  private fb = inject(FormBuilder);
  
  form = this.fb.group({
    name: ['', [Validators.required, Validators.minLength(2)]],
    url: ['', [Validators.required, Validators.pattern(URL_REGEX)]],
    scopes: this.fb.array([])
  });

  onSubmit(): void {
    if (this.form.valid) {
      // call api
    }
  }
}
```

---

---

## 8. Real-Time SSE

Real-time data is handled by the `SseService`, which manages the `EventSource` lifecycle and reconnection logic.

```ts
// src/app/core/services/sse.service.ts
@Injectable({ providedIn: 'root' })
export class SseService {
  stream<T>(path: string): Observable<T> {
    return new Observable<T>(observer => {
      const source = new EventSource(`${environment.apiUrl}${path}`, { withCredentials: true });
      source.onmessage = e => observer.next(JSON.parse(e.data));
      source.onerror = err => observer.error(err);
      return () => source.close();
    });
  }
}
```

---

---

## 9. Canonical Component Patterns

### 9.1 Data Table with Pagination

Always use the shared `AppTableComponent`. Offset pagination is the default for CRUD lists; cursor pagination is mandatory for audit/threat streams.

```html
<app-table 
  [columns]="cols" 
  [data]="connectors()" 
  [paginator]="true"
  (pageChange)="onPage($event)" />
```

---

## 10. Security — Frontend

### 10.1 XSS Protection

Angular's `DomSanitizer` is the primary defense against XSS. Directly using `innerHTML` or bypassing security is forbidden unless justified and reviewed.

### 10.2 CSP Policy

All templates must adhere to the global CSP policy. No inline `<script>` or `<style>` blocks.

---

## 11. Testing Standards

- **Unit Tests**: Mandatory for all logic in services, guards, and interceptors. Use **Jasmine** and **Karma**.
- **Component Tests**: Use **Angular Testing Library**. Avoid `TestBed` boilerplate where possible.
- **E2E**: Critical paths must be covered by **Playwright**.

---

## 12. Environment Variables

Store application configuration in `src/environments/environment.ts` and `environment.prod.ts`.

---

## 13. Tech Stack Reference

| Concern | Choice |
|---|---|
| Framework | Angular 19+ (Standalone) |
| State | Angular Signals |
| API Client | HttpClient |
| Routing | Angular Router (Functional Guards) |
| Styling | Tailwind CSS |
| Testing | Jasmine, Karma, Testing Library, Playwright |
| Forms | Reactive Forms |
