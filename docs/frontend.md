# OpenGuard — Frontend Architecture & Component Specification

> **Document status:** Authoritative. Companion to the Backend Specification.
> **Audience:** Frontend engineers, design system contributors, security reviewers.
> **Stack:** Next.js 16 (App Router), TypeScript 5, Tailwind CSS v3.4, TanStack Query v5, Zustand v5.

---

## Table of Contents

0. [Frontend Quality Standards](#0-frontend-quality-standards)
1. [Stack Decisions & Rationale](#1-stack-decisions--rationale)
2. [Repository Layout](#2-repository-layout)
3. [Routing Architecture](#3-routing-architecture)
4. [Authentication & Session Management](#4-authentication--session-management)
5. [API Client Layer](#5-api-client-layer)
6. [State Management](#6-state-management)
7. [Design System](#7-design-system)
8. [Feature Modules](#8-feature-modules)
   - 8.1 [Connected Apps / Connectors](#81-connected-apps--connectors)
   - 8.2 [Audit Log](#82-audit-log)
   - 8.3 [Threat Detection & Alerts](#83-threat-detection--alerts)
   - 8.4 [Policy Management](#84-policy-management)
   - 8.5 [Compliance & Reports](#85-compliance--reports)
   - 8.6 [DLP / Content Scanning](#86-dlp--content-scanning)
   - 8.7 [Identity & User Management](#87-identity--user-management)
   - 8.8 [Organization Settings](#88-organization-settings)
9. [Real-Time Data (SSE / WebSocket)](#9-real-time-data-sse--websocket)
10. [Security Constraints](#10-security-constraints)
11. [Observability & Error Handling](#11-observability--error-handling)
12. [Performance Targets](#12-performance-targets)
13. [Testing Standards](#13-testing-standards)
14. [CI/CD Pipeline](#14-cicd-pipeline)
15. [Environment Variables](#15-environment-variables)
16. [Full-System Acceptance Criteria](#16-full-system-acceptance-criteria)

---

## 0. Frontend Quality Standards

These standards mirror the philosophy in the backend spec §0. CI enforces all of them.

### 0.1 Philosophy

**Correctness before convenience.** Security-critical UI (API key reveal, MFA enrollment, session revocation) must be correct even at the cost of UX complexity. A confused user is better than a leaked credential.

**Types everywhere.** Every API response, every Zustand slice, every component prop is typed. `any` is forbidden outside `JSON.stringify` / `JSON.parse` boundaries, where it must be immediately cast.

**Server components by default.** Pages and layouts that do not require client interactivity are React Server Components. Client components are explicitly marked `"use client"` and kept as leaves in the component tree.

**Boring UI is good UI.** Animate nothing that does not convey state change. No decorative motion.

### 0.2 TypeScript Rules

```ts
// Forbidden patterns
const x: any = response;           // ❌ — use `unknown` then narrow
const y = (data as any).field;     // ❌ — derive the type from the API schema
// @ts-ignore                       // ❌ — fix the type error instead
// @ts-expect-error                 // ⚠️ allowed only in test files with a comment

// Required patterns
function parseUser(raw: unknown): User {
  if (!isUser(raw)) throw new ParseError("expected User");
  return raw;
}
```

Compiler options in `tsconfig.json`:
```json
{
  "strict": true,
  "noUncheckedIndexedAccess": true,
  "exactOptionalPropertyTypes": true,
  "noImplicitReturns": true,
  "noFallthroughCasesInSwitch": true
}
```

### 0.3 Naming Conventions

| Thing | Convention | Example |
|---|---|---|
| React components | PascalCase | `ConnectorTable` |
| Hooks | `use` prefix, camelCase | `useConnectors` |
| API client functions | verb + noun | `fetchConnectors`, `createConnector` |
| Zustand stores | camelCase, `Store` suffix | `useSessionStore` |
| Route params | kebab-case folders | `app/(dashboard)/connectors/[connectorId]/` |
| Environment vars | `NEXT_PUBLIC_` prefix for browser-exposed | `NEXT_PUBLIC_API_URL` |
| CSS tokens | `--og-*` prefix for project-specific overrides | `--og-severity-critical` |

### 0.4 Forbidden Patterns

| Pattern | Why forbidden |
|---|---|
| `localStorage` / `sessionStorage` for auth tokens | XSS readable. Tokens in `HttpOnly` cookies only. |
| Inline `style` with hardcoded colors | Breaks dark mode. Use Tailwind tokens or CSS vars. |
| `console.log` in production code | Leaks sensitive fields. Use the `logger` utility. |
| `dangerouslySetInnerHTML` | XSS vector. Use `DOMPurify` if HTML rendering is unavoidable (audit event display). |
| Direct `fetch` from components | Bypasses the API client (no retry, no circuit breaker, no token refresh). |
| Hardcoded base URLs | Use `NEXT_PUBLIC_API_URL`. |
| Storing API key plaintext beyond its reveal lifecycle | The key is shown once, then discarded from state. |
| Rendering user-supplied strings without escaping | All data from APIs is treated as untrusted until cast to a known type. |

### 0.5 Code Review Checklist

**Types & correctness**
- [ ] No `any` outside parse boundaries
- [ ] All API responses typed via generated SDK types
- [ ] No non-null assertions (`!`) on data that may genuinely be null

**Security**
- [ ] No auth token in `localStorage`
- [ ] API key not stored in component state after the reveal dialog closes
- [ ] User-supplied data not passed to `dangerouslySetInnerHTML`
- [ ] No `window.location.href` redirect with user-controlled input (open redirect)
- [ ] `Idempotency-Key` header sent on all `POST` mutations

**Accessibility**
- [ ] All interactive elements reachable by keyboard
- [ ] `aria-label` or visible label on every form control
- [ ] Color is not the sole indicator of state (severity badges include text)

**Performance**
- [ ] Server components used where possible (no `"use client"` without reason)
- [ ] Large tables use virtual scrolling (`@tanstack/react-virtual`)
- [ ] Images use `next/image` with explicit `width` and `height`

---

## 1. Stack Decisions & Rationale

| Concern | Choice | Rationale |
|---|---|---|
| Framework | Next.js 16 (App Router) | Server components reduce JS bundle; React 19 (Async Transitions, Action Hooks) handles optimistic UI natively. |
| Language | TypeScript 5 (strict) | Type-safety catches the class of bug (wrong field name, missing null check) most common in dashboard UIs. |
| Styling | Tailwind CSS v3.4 | Zero-runtime. Stable v3 engine with plugin support. Tokens map directly to the design system. |
| Component primitives | Radix UI (headless) | Accessibility-correct primitives (dialogs, dropdowns, tooltips) without styling opinions. |
| Data fetching | TanStack Query v5 | Stale-while-revalidate, optimistic updates, deduplication. Aligns with the backend's cache TTL design. |
| Global state | Zustand v5 | Tiny footprint. Current version handles immutability and hydration natively. |
| Forms | React Hook Form + Zod | Client-side validation mirrors server-side validation schema. Zod schemas are shared with the API client type layer. |
| Tables | TanStack Table v8 + `@tanstack/react-virtual` | Virtual scrolling required for audit log (potentially millions of rows per org). |
| Charts | Recharts | Sufficient for event volume bar charts and MTTR line charts without a heavy dependency. |
| Testing | Vitest + Testing Library + Playwright | Vitest for unit/component; Playwright for E2E (matches backend k6 load test phase). |
| Auth session | `next-auth` v4 (JWT strategy) | Handles OIDC callback, token refresh, and secure cookie management. Session data stored in `HttpOnly` cookie, never `localStorage`. |
| Icons | Lucide React | Tree-shakeable; consistent stroke style. |
| Date handling | `date-fns` | Locale-aware formatting for audit timestamps. No `moment.js`. |

---

## 2. Repository Layout

```
web/
├── app/
│   ├── layout.tsx                    # Root layout: providers, fonts, global meta
│   ├── not-found.tsx
│   ├── error.tsx
│   │
│   ├── (public)/                     # Unauthenticated routes
│   │   ├── login/
│   │   │   └── page.tsx              # Password login + SSO entry
│   │   ├── login/mfa/
│   │   │   └── page.tsx              # TOTP / WebAuthn challenge
│   │   ├── auth/
│   │   │   ├── [...nextauth]/
│   │   │   │   └── route.ts          # next-auth handler (OIDC callback)
│   │   │   └── saml-acs/
│   │   │       └── route.ts          # SAML ACS relay (posts to IAM)
│   │   └── setup/
│   │       └── page.tsx              # First-run org registration
│   │
│   ├── (dashboard)/                  # Protected: middleware guards all routes
│   │   ├── layout.tsx                # Shell: sidebar + header + toast region
│   │   ├── page.tsx                  # Overview / home dashboard
│   │   │
│   │   ├── connectors/
│   │   │   ├── page.tsx              # Connector list
│   │   │   ├── new/page.tsx          # Registration wizard
│   │   │   └── [connectorId]/
│   │   │       ├── page.tsx          # Detail: metadata, delivery log, chart
│   │   │       └── edit/page.tsx     # Edit webhook URL, scopes
│   │   │
│   │   ├── audit/
│   │   │   ├── page.tsx              # Audit event log
│   │   │   └── integrity/page.tsx    # Hash chain verifier
│   │   │
│   │   ├── threats/
│   │   │   ├── page.tsx              # Alert list + stats
│   │   │   └── [alertId]/page.tsx    # Alert detail + saga steps
│   │   │
│   │   ├── policies/
│   │   │   ├── page.tsx              # Policy list
│   │   │   ├── new/page.tsx          # Policy creator
│   │   │   └── [policyId]/
│   │   │       └── page.tsx          # Editor + eval log
│   │   │
│   │   ├── compliance/
│   │   │   ├── page.tsx              # Report list + posture score
│   │   │   └── reports/[reportId]/
│   │   │       └── page.tsx          # Status poller + download
│   │   │
│   │   ├── dlp/
│   │   │   ├── page.tsx              # Findings list + stats
│   │   │   └── policies/page.tsx     # DLP policy editor
│   │   │
│   │   ├── users/
│   │   │   ├── page.tsx              # User list
│   │   │   └── [userId]/page.tsx     # User detail: sessions, tokens, MFA
│   │   │
│   │   └── settings/
│   │       ├── page.tsx              # Org settings (plan, SSO, MFA policy)
│   │       ├── scim/page.tsx         # SCIM token management
│   │       └── security/page.tsx     # JWT rotation status, cert expiry
│   │
│   └── api/                          # Next.js API routes (BFF layer)
│       ├── auth/[...nextauth]/
│       │   └── route.ts
│       └── proxy/
│           └── [...path]/
│               └── route.ts          # Authenticated proxy; adds Bearer, forwards to services
│
├── components/
│   ├── ui/                           # Design system primitives (Section 7)
│   │   ├── button.tsx
│   │   ├── input.tsx
│   │   ├── badge.tsx
│   │   ├── table.tsx
│   │   ├── dialog.tsx
│   │   ├── drawer.tsx
│   │   ├── toast.tsx
│   │   ├── tooltip.tsx
│   │   ├── skeleton.tsx
│   │   ├── pagination.tsx
│   │   └── ...
│   │
│   ├── charts/
│   │   ├── event-volume-bar.tsx      # Events/day for connector detail
│   │   ├── threat-timeline.tsx       # Alert rate over time
│   │   └── mttr-line.tsx             # Mean time to resolve trend
│   │
│   ├── security/
│   │   ├── api-key-reveal.tsx        # One-time key display with copy + mask
│   │   ├── mfa-enroll.tsx            # TOTP QR code + verification flow
│   │   ├── webauthn-register.tsx     # WebAuthn registration wizard
│   │   └── session-list.tsx          # Active sessions with revoke action
│   │
│   └── layout/
│       ├── sidebar.tsx
│       ├── header.tsx
│       ├── breadcrumb.tsx
│       └── page-header.tsx
│
├── hooks/
│   ├── use-connectors.ts
│   ├── use-audit-events.ts
│   ├── use-threat-alerts.ts
│   ├── use-policies.ts
│   ├── use-compliance-report.ts
│   ├── use-dlp-findings.ts
│   ├── use-users.ts
│   ├── use-org.ts
│   ├── use-live-alerts.ts            # SSE subscription hook
│   └── use-idempotency-key.ts        # Generates and tracks idempotency keys
│
├── lib/
│   ├── api/
│   │   ├── client.ts                 # Base fetch wrapper (auth, retry, CB)
│   │   ├── connectors.ts
│   │   ├── audit.ts
│   │   ├── threats.ts
│   │   ├── policies.ts
│   │   ├── compliance.ts
│   │   ├── dlp.ts
│   │   ├── users.ts
│   │   └── org.ts
│   ├── auth.ts                       # next-auth config
│   ├── logger.ts                     # Browser-side structured logger (no console.log)
│   ├── errors.ts                     # Typed API error parser
│   ├── validators/                   # Zod schemas mirroring backend validation
│   │   ├── connector.ts
│   │   ├── policy.ts
│   │   └── user.ts
│   └── utils/
│       ├── cursor.ts                 # Decode/encode base64 cursor tokens
│       ├── format-date.ts
│       ├── severity.ts               # Map threat score → label + color token
│       └── sanitize.ts               # DOMPurify wrapper for audit event display
│
├── store/
│   ├── session.ts                    # Zustand: session, org_id, user
│   ├── ui.ts                         # Zustand: sidebar collapsed, toast queue
│   └── index.ts
│
├── types/
│   ├── api.ts                        # Generated from OpenAPI specs (Section 5.1)
│   ├── models.ts                     # Canonical frontend types (mirrors shared/models)
│   └── env.d.ts                      # Type augmentation for process.env
│
├── middleware.ts                     # Next.js middleware: auth guard + CSP header
├── next.config.js
├── tailwind.config.ts
├── tsconfig.json
├── vitest.config.ts
├── playwright.config.ts
└── package.json
```

---

## 3. Routing Architecture

### 3.1 Route Group Strategy

Two top-level route groups with distinct layouts:

- `(public)` — no authentication required. Minimal layout (logo + centered card). Handles login, OIDC/SAML callbacks, and first-run setup.
- `(dashboard)` — requires authentication. Full shell layout with sidebar navigation, breadcrumb, and toast region. Middleware redirects unauthenticated requests to `/login`.

### 3.2 Middleware Guard

`middleware.ts` runs on every request to `(dashboard)/*`. It reads the `next-auth` session from the encrypted `HttpOnly` cookie and redirects to `/login` if the session is absent or expired. It also injects the `Content-Security-Policy` header.

```ts
// middleware.ts
import { withAuth } from "next-auth/middleware";
import { NextResponse } from "next/server";

export default withAuth(
  function middleware(req) {
    const response = NextResponse.next();
    response.headers.set(
      "Content-Security-Policy",
      [
        "default-src 'self'",
        "script-src 'self' 'nonce-{NONCE}'",
        "style-src 'self' 'unsafe-inline'",   // Tailwind requires this
        "img-src 'self' data:",
        "connect-src 'self' " + process.env.NEXT_PUBLIC_API_URL,
        "frame-ancestors 'none'",
      ].join("; ")
    );
    return response;
  },
  {
    callbacks: {
      authorized: ({ token }) => !!token,
    },
  }
);

export const config = {
  matcher: ["/(dashboard)/:path*"],
};
```

**Nonce generation:** Each request generates a fresh nonce for `script-src`. The nonce is passed to the root layout via a request header. This prevents inline script injection.

### 3.3 User Status Guard

After authentication, the layout checks the user's `status` field from the session. Users with `status: "initializing"` (mid-provisioning saga) see a "Your account is being set up" holding page instead of the dashboard. Users with `status: "suspended"` or `"deprovisioned"` are redirected to `/login` with an `account_status` error query param.

### 3.4 Parallel Routes & Loading States

Each dashboard section uses the Next.js `loading.tsx` convention to stream skeleton UI while server data loads. Heavy routes (audit log, compliance report list) use React Suspense boundaries to avoid blocking the shell layout.

---

## 4. Authentication & Session Management

### 4.1 next-auth Configuration

```ts
// lib/auth.ts
import NextAuth from "next-auth";
import type { NextAuthOptions } from "next-auth";

export const authOptions: NextAuthOptions = {
  session: {
    strategy: "jwt",
    maxAge: 30 * 24 * 60 * 60,     // mirrors IAM_REFRESH_TOKEN_EXPIRY_DAYS
  },
  cookies: {
    sessionToken: {
      name: "__Secure-next-auth.session-token",
      options: {
        httpOnly: true,
        sameSite: "strict",
        path: "/",
        secure: true,               // always; no HTTP
      },
    },
  },
  providers: [
    // OIDC provider: points to IAM service
    // Credentials provider: password login (calls IAM POST /auth/login)
  ],
  callbacks: {
    async jwt({ token, account, user }) {
      // Store the IAM access token and refresh token inside the encrypted JWT.
      // These are NEVER exposed to client JS — they live only in the HttpOnly cookie.
      if (account) {
        token.iamAccessToken = account.access_token;
        token.iamRefreshToken = account.refresh_token;
        token.iamTokenExpiry = account.expires_at;
      }
      // Proactive token refresh: if expiry is within 60s, refresh now.
      if (Date.now() < (token.iamTokenExpiry as number) * 1000 - 60_000) {
        return token;
      }
      return refreshIAMToken(token);
    },
    async session({ session, token }) {
      // Only expose non-sensitive fields to the client session.
      session.user.id = token.sub!;
      session.user.orgId = token.orgId as string;
      session.user.status = token.status as string;
      session.user.mfaEnabled = token.mfaEnabled as boolean;
      // iamAccessToken is NOT attached to session — BFF proxy adds it server-side.
      return session;
    },
  },
};
```

**Critical:** the IAM access token and refresh token are stored inside the next-auth JWT which lives in an `HttpOnly` cookie. They are never accessible to client-side JavaScript.

### 4.2 BFF Proxy (API route)

All API calls from the browser go to `/api/proxy/[...path]`, which is a Next.js API route that:

1. Reads the IAM access token from the server-side session.
2. Forwards the request to the backend service with `Authorization: Bearer <iamAccessToken>`.
3. Returns the response.

This pattern means no auth token ever reaches the browser's JS environment. The browser only has the opaque next-auth session cookie.

```ts
// app/api/proxy/[...path]/route.ts
import { getServerSession } from "next-auth";
import { authOptions } from "@/lib/auth";

export async function GET(req: Request, { params }: { params: { path: string[] } }) {
  const session = await getServerSession(authOptions);
  if (!session) return new Response(null, { status: 401 });

  const targetURL = `${process.env.API_INTERNAL_URL}/${params.path.join("/")}`;
  const upstream = await fetch(targetURL, {
    headers: {
      Authorization: `Bearer ${(session as any)._iamAccessToken}`,
      "X-Request-ID": req.headers.get("X-Request-ID") ?? crypto.randomUUID(),
      "X-Org-ID": session.user.orgId,     // informational only; backend derives from token
      "Content-Type": req.headers.get("Content-Type") ?? "application/json",
    },
  });
  return upstream;
}
// POST, PATCH, DELETE handlers follow the same pattern, passing through the body.
```

### 4.3 MFA Flow

The login page renders a two-step form:

1. Email + password → `POST /auth/login` → success returns `mfa_required: true` + a short-lived pre-MFA session cookie.
2. If MFA required: the browser is redirected to `/login/mfa` which presents either the TOTP input or a WebAuthn prompt depending on `mfa_method` in the session.
3. On MFA success: the pre-MFA session is destroyed and a full session cookie is issued. The component must not persist the TOTP code in state after submission.

**TOTP replay protection:** If `POST /auth/mfa/challenge` returns `401 TOTP_REPLAY_DETECTED`, the form shows: "This code was already used. Please wait for the next 30-second code." It does not expose that replay detection was the reason for rejection via any machine-readable field.

### 4.4 Session Expiry Handling

TanStack Query's `onError` global callback inspects `APIError.code`. When it sees `TOKEN_EXPIRED` or `SESSION_REVOKED_RISK`:

1. The Zustand `useSessionStore` sets `sessionExpired: true`.
2. A full-screen overlay renders: "Your session has ended. Please log in again." with a single "Log in" button.
3. On click: `signOut({ callbackUrl: "/login" })`.

No inline error is shown on whatever request was in-flight; the overlay supersedes everything.

---

## 5. API Client Layer

### 5.1 Type Generation

Backend OpenAPI specs (`docs/api/<service>.openapi.json`) are compiled to TypeScript types as part of the CI pipeline:

```bash
# package.json scripts
"generate:types": "openapi-typescript docs/api/control-plane.openapi.json -o types/api.ts && openapi-typescript docs/api/iam.openapi.json -o types/iam.ts"
```

All API functions use these generated types for request and response bodies. Manual type definitions in `types/models.ts` are only for types not covered by the OpenAPI specs (e.g. derived UI-only types).

### 5.2 Base Client

```ts
// lib/api/client.ts
import { logger } from "@/lib/logger";

export interface APIError {
  code: string;
  message: string;
  requestId: string;
  traceId: string;
  retryable: boolean;
}

export class OpenGuardAPIError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: APIError
  ) {
    super(body.message);
  }
}

const MAX_RETRIES = 3;
const RETRYABLE_STATUSES = new Set([429, 502, 503, 504]);

export async function apiFetch<T>(
  path: string,
  init: RequestInit & { idempotencyKey?: string } = {}
): Promise<T> {
  const { idempotencyKey, ...fetchInit } = init;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "X-Request-ID": crypto.randomUUID(),
    ...(fetchInit.headers as Record<string, string>),
  };

  if (idempotencyKey) {
    headers["Idempotency-Key"] = idempotencyKey;
  }

  let lastError: OpenGuardAPIError | null = null;
  for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
    if (attempt > 0) {
      const delay = Math.min(100 * 2 ** attempt + Math.random() * 100, 10_000);
      await new Promise((r) => setTimeout(r, delay));
    }

    const res = await fetch(`/api/proxy${path}`, {
      ...fetchInit,
      headers,
    });

    if (res.ok) {
      return res.json() as Promise<T>;
    }

    const body = await res.json().catch(() => ({
      code: "PARSE_ERROR",
      message: "Could not parse error response",
      requestId: headers["X-Request-ID"],
      traceId: "",
      retryable: false,
    }));

    lastError = new OpenGuardAPIError(res.status, body);

    // Non-retryable: auth errors, validation errors, not-found
    if (!RETRYABLE_STATUSES.has(res.status) && !body.retryable) {
      throw lastError;
    }
  }

  logger.error("API request failed after retries", {
    path,
    error: lastError?.body.code,
    traceId: lastError?.body.traceId,
  });
  throw lastError;
}
```

### 5.3 Per-Resource Client Modules

Each feature has a typed client module. Example for connectors:

```ts
// lib/api/connectors.ts
import { apiFetch } from "./client";
import type { paths } from "@/types/api";

type ConnectorListResponse =
  paths["/v1/admin/connectors"]["get"]["responses"]["200"]["content"]["application/json"];

type CreateConnectorBody =
  paths["/v1/admin/connectors"]["post"]["requestBody"]["content"]["application/json"];

type CreateConnectorResponse =
  paths["/v1/admin/connectors"]["post"]["responses"]["200"]["content"]["application/json"];

export const connectorsApi = {
  list: (params?: { page?: number; per_page?: number }) =>
    apiFetch<ConnectorListResponse>(`/v1/admin/connectors?${new URLSearchParams(params as any)}`),

  create: (body: CreateConnectorBody, idempotencyKey: string) =>
    apiFetch<CreateConnectorResponse>("/v1/admin/connectors", {
      method: "POST",
      body: JSON.stringify(body),
      idempotencyKey,
    }),

  suspend: (id: string) =>
    apiFetch<void>(`/v1/admin/connectors/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ status: "suspended" }),
    }),

  activate: (id: string) =>
    apiFetch<void>(`/v1/admin/connectors/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ status: "active" }),
    }),
};
```

### 5.4 Cursor Pagination Helper

```ts
// lib/utils/cursor.ts
export interface CursorPage<T> {
  data: T[];
  meta: {
    next_cursor: string | null;
    per_page: number;
    total: number | null;       // null for cursor-paginated endpoints
  };
}

export function decodeCursor(cursor: string): { t: number; id: string } {
  return JSON.parse(atob(cursor));
}

export function encodeCursor(t: number, id: string): string {
  return btoa(JSON.stringify({ t, id }));
}
```

TanStack Query's `useInfiniteQuery` is used for all cursor-paginated endpoints (audit log, threat alerts, DLP findings). Offset-based endpoints (users, policies) use regular `useQuery` with explicit `page` state.

---

## 6. State Management

Only three concerns require global state. Everything else is server state (TanStack Query) or local component state.

### 6.1 Session Store

```ts
// store/session.ts
import { create } from "zustand";

interface SessionState {
  userId: string | null;
  orgId: string | null;
  userStatus: string | null;
  mfaEnabled: boolean;
  sessionExpired: boolean;

  setSession: (data: Pick<SessionState, "userId" | "orgId" | "userStatus" | "mfaEnabled">) => void;
  setSessionExpired: () => void;
  clearSession: () => void;
}

export const useSessionStore = create<SessionState>()((set) => ({
  userId: null,
  orgId: null,
  userStatus: null,
  mfaEnabled: false,
  sessionExpired: false,

  setSession: (data) => set(data),
  setSessionExpired: () => set({ sessionExpired: true }),
  clearSession: () => set({ userId: null, orgId: null, sessionExpired: false }),
}));
```

### 6.2 UI Store

```ts
// store/ui.ts
import { create } from "zustand";

interface Toast {
  id: string;
  variant: "success" | "error" | "warning" | "info";
  title: string;
  description?: string;
}

interface UIState {
  sidebarCollapsed: boolean;
  toasts: Toast[];

  toggleSidebar: () => void;
  addToast: (toast: Omit<Toast, "id">) => void;
  removeToast: (id: string) => void;
}

export const useUIStore = create<UIState>()((set) => ({
  sidebarCollapsed: false,
  toasts: [],

  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  addToast: (toast) =>
    set((s) => ({
      toasts: [...s.toasts, { ...toast, id: crypto.randomUUID() }],
    })),
  removeToast: (id) =>
    set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}));
```

### 6.3 TanStack Query Configuration

```ts
// app/layout.tsx (QueryClientProvider setup)
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,          // matches POLICY_CACHE_TTL_SECONDS
      gcTime: 5 * 60_000,
      retry: (failureCount, error) => {
        if (error instanceof OpenGuardAPIError) {
          if (!error.body.retryable) return false;
          if (error.status === 401 || error.status === 403) return false;
        }
        return failureCount < 3;
      },
      refetchOnWindowFocus: true,
    },
    mutations: {
      onError: (error) => {
        if (error instanceof OpenGuardAPIError) {
          if (error.status === 401) {
            useSessionStore.getState().setSessionExpired();
          } else {
            useUIStore.getState().addToast({
              variant: "error",
              title: error.body.code,
              description: error.body.message,
            });
          }
        }
      },
    },
  },
});
```

---

## 7. Design System

### 7.1 Tokens

Design tokens extend Tailwind's config. All semantic color decisions happen in the token layer, not in components.

```ts
// tailwind.config.ts (excerpt)
export default {
  theme: {
    extend: {
      colors: {
        severity: {
          critical: "#A32D2D",
          high:     "#BA7517",
          medium:   "#185FA5",
          low:      "#3B6D11",
          info:     "#534AB7",
        },
        status: {
          active:           "#3B6D11",
          suspended:        "#BA7517",
          deprovisioned:    "#888780",
          initializing:     "#185FA5",
          provisioning_failed: "#A32D2D",
        },
      },
    },
  },
};
```

### 7.2 Component Inventory

#### Primitives (`components/ui/`)

| Component | Radix primitive | Notes |
|---|---|---|
| `Button` | — | Variants: `default`, `destructive`, `outline`, `ghost`. Always has `aria-label` when icon-only. |
| `Input` | — | Error state shows red border + `role="alert"` error message below. |
| `Select` | `Select.Root` | For enum fields (connector scopes, report type). |
| `Checkbox` | `Checkbox.Root` | Used in scope multi-select. |
| `Badge` | — | `variant` prop: `active`, `suspended`, `critical`, `high`, `medium`, `low`. Never uses color alone — always includes text. |
| `Skeleton` | — | Used in `loading.tsx` files and Suspense fallbacks. |
| `Dialog` | `Dialog.Root` | Focus trap, ESC closes. Used for destructive confirmations. |
| `Drawer` | — | Right-side panel for detail views without full navigation (connector edit, policy detail). |
| `Toast` | — | Rendered in root layout. Pulled from `useUIStore`. Auto-dismisses after 5s; error toasts stay until dismissed. |
| `Tooltip` | `Tooltip.Root` | For truncated text (long org names, connector names). |
| `Pagination` | — | Wraps TanStack Table pagination controls. Shows `page X of Y` for offset; `Load more` for cursor. |
| `CopyButton` | — | Copies text to clipboard. Shows checkmark confirmation for 2s. Used for API key reveal, trace IDs. |

#### Data Display (`components/charts/`)

All charts are client components. They are wrapped in Suspense with a `Skeleton` fallback.

| Chart | Used in | Data source |
|---|---|---|
| `EventVolumeBar` | Connector detail | `event_counts_daily` ClickHouse materialized view |
| `ThreatTimeline` | Threats overview | `alert_stats` ClickHouse table |
| `MttrLine` | Threats detail | `GET /v1/threats/stats` |
| `DlpFindingsPie` | DLP overview | `GET /v1/dlp/stats` |
| `PolicyEvalBar` | Policy detail | `GET /v1/policy/eval-logs` |

All charts use Recharts. `ResponsiveContainer` wraps every chart. Axes use `date-fns` formatting. Colors use the severity token map.

#### Security UI (`components/security/`)

These components have stricter rules than ordinary UI.

**`ApiKeyReveal`**
- Renders the plaintext API key exactly once, in a masked (`••••••••`) field.
- A "Reveal" button (confirmation dialog: "Show the API key? This is the only time it will be displayed.") toggles visibility for 30 seconds then automatically masks.
- A `CopyButton` copies to clipboard without requiring reveal.
- On unmount or dialog close, the key string is overwritten with an empty string in component state. React's reconciler garbage-collects the string.
- The key is **never** stored in Zustand, TanStack Query cache, or `localStorage`.
- Prominent callout: "Save this key now. It will not be shown again."

```ts
// components/security/api-key-reveal.tsx
"use client";
import { useState, useEffect, useCallback } from "react";

export function ApiKeyReveal({ apiKey }: { apiKey: string }) {
  const [revealed, setRevealed] = useState(false);
  const [confirming, setConfirming] = useState(false);

  // Auto-mask after 30s
  useEffect(() => {
    if (!revealed) return;
    const timer = setTimeout(() => setRevealed(false), 30_000);
    return () => clearTimeout(timer);
  }, [revealed]);

  // Overwrite key in state on unmount
  const destroy = useCallback(() => {
    setRevealed(false);
  }, []);
  useEffect(() => destroy, [destroy]);

  return (/* ... */);
}
```

**`MfaEnroll`**
- Calls `POST /auth/mfa/enroll` on mount to fetch the TOTP secret + `otpauth://` URI.
- Renders a QR code (using `qrcode.react`) and the base32 secret as a copyable string.
- A 6-digit input verifies the enrollment via `POST /auth/mfa/verify`.
- The TOTP secret is cleared from component state after successful verification.
- If the component unmounts before verification, the pending enrollment is abandoned (no cleanup call needed; unverified enrollments do not take effect).

**`WebAuthnRegister`**
- Calls `POST /auth/webauthn/register/begin` to get the challenge.
- Calls `navigator.credentials.create()` with the challenge options.
- POSTs the credential to `POST /auth/webauthn/register/finish`.
- The `session_id` used for the challenge is stored in the `HttpOnly` cookie by the BFF proxy. It is not accessible to this component.

**`SessionList`**
- Renders a table of active sessions with: IP, user agent, country, last active, created.
- Each row has a "Revoke" button that calls `DELETE /users/:id/sessions/:sid` with a confirmation dialog.
- "Revoke all other sessions" triggers `DELETE /users/:id/sessions` (except current).
- Current session row is highlighted and its revoke button is disabled.

---

## 8. Feature Modules

### 8.1 Connected Apps / Connectors

**Route:** `/connectors`

#### List page (`page.tsx`)
Server component. Fetches connector list from `GET /v1/admin/connectors` via the BFF proxy. Passes data to the client component `ConnectorTable`.

**`ConnectorTable`** (client component):
- TanStack Table with columns: Name, Status badge, Scopes, Created, Last event, Event volume (30d).
- Per-row dropdown actions: View, Edit webhook, Suspend/Activate, Delete (with confirmation dialog).
- "Register app" button opens the registration wizard.
- Bulk actions: Suspend selected, Delete selected.
- Filtering: status filter (active / suspended / pending), scope filter.

#### Registration wizard (`new/page.tsx`)
Three-step wizard (client component):

1. **App details** — Name (required), Webhook URL (optional, validated for HTTPS on client and server), Scopes (multi-select checkboxes).
2. **Review** — Summary of settings. "Register" button.
3. **Credential reveal** — `ApiKeyReveal` component. The API key is taken from the mutation response, displayed once, then discarded. Wizard cannot be re-entered after this step (navigation to `/connectors/:id`).

`useIdempotencyKey` generates a stable key for the registration mutation so that network retries do not register duplicate connectors.

#### Detail page (`[connectorId]/page.tsx`)
Mixed: shell is server component, interactive panels are client components.

**Panels:**
- Metadata card (name, status, scopes, created, connector ID).
- Webhook delivery log — TanStack Table, cursor-paginated, columns: Timestamp, Event type, HTTP status, Latency, Attempts. Status badge: `delivered`, `failed`, `retrying`, `dead`.
- Event volume chart — 30-day bar chart from `event_counts_daily`.
- "Send test webhook" button — calls `POST /v1/admin/connectors/:id/test`, shows delivery result in a toast.
- Edit panel (Drawer) — webhook URL + scopes, saves via `PATCH`.
- Danger zone — Suspend / Activate toggle, Delete (two-confirmation dialog: type connector name to confirm).

### 8.2 Audit Log

**Route:** `/audit`

#### Event list (`page.tsx`)
Client component. Uses `useInfiniteQuery` with `GET /audit/events` (cursor-paginated by `(occurred_at, event_id)`).

**`AuditTable`:**
- TanStack Table + `@tanstack/react-virtual` for virtual scrolling (mandatory; audit log may contain millions of rows per org).
- Columns: Timestamp (relative + absolute on hover), Actor, Actor type, Event type, Source.
- Clicking a row opens an expandable detail panel (not a new page) showing the full event payload rendered as a JSON tree.
- **Payload display:** User-supplied strings in event payloads are passed through `DOMPurify.sanitize()` before rendering. Masked DLP fields (`[REDACTED:ssn]` etc.) render with a `Badge` in `info` variant.
- Filters: event type (multi-select), actor ID, date range picker, source (internal / connector).
- Export button: triggers `POST /audit/export` then polls `GET /audit/export/:jobId` with a progress indicator. Download link appears when `status: "completed"`.

#### Integrity verifier (`integrity/page.tsx`)
Client component. Calls `GET /audit/integrity` and displays:
- Overall status: `ok` (green badge) or gap count (red badge).
- If gaps exist: table of `chain_seq` gaps with timestamp range and missing count.
- "Re-verify" button refetches.

### 8.3 Threat Detection & Alerts

**Route:** `/threats`

#### Alert list (`page.tsx`)
Client component. `useInfiniteQuery` with `GET /v1/threats/alerts`.

**`AlertTable`:**
- Columns: Severity badge, Title, Detector, Actor, Created, Status, MTTR (for resolved).
- Severity filter (critical / high / medium / low), status filter (open / acknowledged / resolved), date range.
- Live updates via SSE (`useLiveAlerts` hook — see Section 9). New alerts prepend to the list with a brief highlight animation.
- Stats cards above the table: open alerts, critical count, mean MTTR, detections in last 24h.

#### Alert detail (`[alertId]/page.tsx`)
Server component shell + client interactive sections.
- Alert metadata: score, detectors fired, composite weight breakdown.
- Saga step timeline: ordered list of steps (e.g. "Alert created → Notification sent → SIEM webhook fired → Audit logged") with timestamps and status icons.
- Actor context: user details, recent login history for the affected actor (linked to Audit Log with pre-applied filter).
- Action buttons: "Acknowledge" (requires comment), "Resolve" (computes MTTR). Both are guarded by a confirmation dialog and use idempotency keys.

### 8.4 Policy Management

**Route:** `/policies`

#### Policy list (`page.tsx`)
Server component. Renders a table: Name, Version, Created, Last updated, Eval count (last 7d).
"New policy" button navigates to `/policies/new`.

#### Policy creator/editor
Client component. Form fields:
- Name, Description.
- Logic builder: a structured JSON editor (using CodeMirror with JSON schema validation) for the `logic` JSONB field. The schema is surfaced as inline autocomplete.
- "Save" uses `POST /v1/policies` or `PUT /v1/policies/:id`. Version / ETag conflict (HTTP 412) is handled with a "Someone else edited this policy" banner and a reload prompt.

**`PolicyEvalLog` panel** (on detail page):
- Table: Timestamp, User, Action, Resource, Result (permit/deny badge), Cache hit (none / redis / sdk), Latency.
- This panel helps operators understand why a policy decision was made.

### 8.5 Compliance & Reports

**Route:** `/compliance`

#### Report list (`page.tsx`)
Server component. Cards for each report: type (GDPR / SOC 2 / HIPAA), status badge, generated date, download button.
"Generate report" button opens a dialog: report type, date range picker, format (PDF).

**Report polling:** After `POST /v1/compliance/reports`, TanStack Query polls `GET /v1/compliance/reports/:id` every 3 seconds until `status: "completed"` or `status: "failed"`. A progress indicator with elapsed time is shown. On completion, the card gains a download button linking to the pre-signed S3 URL.

**Posture score panel:** A summary card showing the compliance score and control coverage, sourced from `GET /v1/compliance/posture`. Updated on report completion.

### 8.6 DLP / Content Scanning

**Route:** `/dlp`

#### Findings list (`page.tsx`)
Client component. `useInfiniteQuery` with `GET /v1/dlp/findings`.

**`DlpFindingsTable`:**
- Columns: Timestamp, Finding type badge (PII / credential / financial), Kind (email / SSN / credit_card / api_key / high_entropy), Event ID (link to audit log), JSON path, Action taken (monitor / mask / block).
- Filter: finding type, date range, action taken.
- Stats: finding counts by type (pie chart), top finding kinds (bar chart).

#### DLP policy editor
Client component. Form per policy: name, rules (pattern matchers, entity types), mode (monitor / block), enabled toggle. Mode change shows a confirmation dialog explaining the latency implications of `block` mode.

### 8.7 Identity & User Management

**Route:** `/users`

#### User list (`page.tsx`)
Server component. Table: Name, Email, Status badge, MFA badge, Last login, Created. Filters: status, MFA enabled.
"Invite user" opens a dialog (email, role). SCIM-provisioned users show a SCIM badge; their email and name fields are read-only in the UI (managed by IdP).

#### User detail (`[userId]/page.tsx`)
Tabs:
- **Overview** — user metadata, status actions (suspend, activate, unlock).
- **Sessions** — `SessionList` component (Section 7.2).
- **API tokens** — list of tokens with name, scopes, last used, revoke action.
- **MFA** — MFA method, `MfaEnroll` / `WebAuthnRegister` components, backup codes (regenerate action).
- **Audit** — link to Audit log pre-filtered to this user.

### 8.8 Organization Settings

**Route:** `/settings`

Tabs:
- **General** — org name, slug (read-only after creation), plan display.
- **Security policy** — MFA required toggle, SSO required toggle, max sessions per user, session timeout.
- **SSO** — SAML IdP metadata URL, entity ID display, ACS URL display, test SSO button.
- **SCIM** — SCIM base URL display, SCIM token management (generate, revoke). Token value shown via `ApiKeyReveal` component.
- **JWT rotation** — list of active JWT key kids, rotation status, link to the rotation runbook.
- **Certificate expiry** — table of mTLS cert names and expiry dates; red badge if within 30 days.

---

## 9. Real-Time Data (SSE / WebSocket)

### 9.1 Live Threat Alerts (SSE)

The threat service exposes `GET /v1/threats/alerts/stream` as a Server-Sent Events endpoint (requires the `events:read` scope). The frontend subscribes when the Threats section is mounted.

```ts
// hooks/use-live-alerts.ts
"use client";
import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

export function useLiveAlerts(orgId: string) {
  const queryClient = useQueryClient();

  useEffect(() => {
    const es = new EventSource(`/api/proxy/v1/threats/alerts/stream`);

    es.onmessage = (event) => {
      const alert = JSON.parse(event.data);
      // Prepend new alert to the existing TanStack Query cache
      queryClient.setQueryData(["threats", "alerts"], (old: any) => {
        if (!old) return old;
        const firstPage = { ...old.pages[0], data: [alert, ...old.pages[0].data] };
        return { ...old, pages: [firstPage, ...old.pages.slice(1)] };
      });
    };

    es.onerror = () => {
      // EventSource auto-reconnects. No manual retry needed.
    };

    return () => es.close();
  }, [orgId, queryClient]);
}
```

The SSE connection is established via the BFF proxy, which injects the auth token. The browser never sees the token.

### 9.2 Audit Log Stream

`GET /audit/events/stream` is an optional live-append mode for the audit log page. When the user activates "Live mode" (a toggle on the audit page), new events are prepended to the virtual table using the same pattern as `useLiveAlerts`. Live mode is paused when the user applies a filter or scrolls past the 50th row (to avoid disorienting the view).

---

## 10. Security Constraints

### 10.1 Content Security Policy

Set in `middleware.ts` for all responses. Key directives:

- `default-src 'self'` — no inline resources from unknown origins.
- `script-src 'self' 'nonce-{NONCE}'` — all inline scripts require the per-request nonce. This is generated by Next.js middleware and passed to the root layout via a header.
- `connect-src 'self' NEXT_PUBLIC_API_URL` — API calls only to the known backend.
- `frame-ancestors 'none'` — prevents clickjacking.
- `img-src 'self' data:` — `data:` needed for QR code rendering (`qrcode.react`).

### 10.2 CSRF Protection

The BFF proxy pattern provides natural CSRF protection: the `POST /api/proxy/...` route is a Next.js API route which is same-origin only. Requests from cross-origin pages cannot include the `HttpOnly` session cookie in cross-origin fetches (SameSite=Strict).

All state-changing proxy routes additionally verify the `Origin` header matches `NEXTAUTH_URL`.

### 10.3 Sensitive Data in Logs

The `logger` utility in `lib/logger.ts` mirrors the backend's `SafeAttr` pattern. Any key containing `password`, `secret`, `token`, `key`, `auth`, or `credential` has its value replaced with `[REDACTED]` before the log entry is emitted.

```ts
// lib/logger.ts
const SENSITIVE_KEYS = ["password","secret","token","key","auth","credential","bearer","cookie","session"];

function redact(obj: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(obj).map(([k, v]) => [
      k,
      SENSITIVE_KEYS.some((s) => k.toLowerCase().includes(s)) ? "[REDACTED]" : v,
    ])
  );
}

export const logger = {
  info: (msg: string, ctx?: Record<string, unknown>) =>
    console.info(JSON.stringify({ level: "info", msg, ...redact(ctx ?? {}) })),
  error: (msg: string, ctx?: Record<string, unknown>) =>
    console.error(JSON.stringify({ level: "error", msg, ...redact(ctx ?? {}) })),
};
```

### 10.4 Dependency Auditing

CI runs `npm audit --audit-level=high` on every push and blocks merge if any high or critical vulnerability is found. Dependabot is configured to auto-merge patch-level updates to dev dependencies.

### 10.5 Open Redirect Protection

The `/login` page accepts a `callbackUrl` query parameter (set by next-auth middleware). Before redirecting, the value is validated: it must be a relative path (`/`) or match `NEXTAUTH_URL`. External URLs are silently replaced with `/`.

```ts
function safeCallbackUrl(raw: string | null): string {
  if (!raw) return "/";
  if (raw.startsWith("/") && !raw.startsWith("//")) return raw;
  try {
    const u = new URL(raw);
    if (u.origin === process.env.NEXTAUTH_URL) return u.pathname + u.search;
  } catch {}
  return "/";
}
```

---

## 11. Observability & Error Handling

### 11.1 OpenTelemetry (Browser)

The `app/layout.tsx` initializes `@opentelemetry/sdk-web` with the OTLP exporter pointing to `NEXT_PUBLIC_OTEL_ENDPOINT`. Instrumentation covers:
- Page navigation spans (route transitions).
- Fetch spans for all API calls (via the base client, which propagates `traceparent` headers so spans link to backend traces).

Sampling rate: `NEXT_PUBLIC_OTEL_SAMPLING_RATE` (default 0.1 in production).

### 11.2 Error Boundaries

Every feature module route is wrapped in an `error.tsx` error boundary. The boundary renders a generic "Something went wrong" message with the `request_id` and `trace_id` from the last failed API call (stored in a ref on the component). It does not expose internal error messages.

For `404` responses from the API, the boundary renders a "Not found" page with a "Go back" link.

### 11.3 Request ID Propagation

The base client generates a `X-Request-ID` UUID on every request. The BFF proxy forwards this to the backend. On error, the `APIError` body includes the `request_id` and `trace_id`. Both are displayed in the error toast and error boundary so users can provide them when reporting issues.

---

## 12. Performance Targets

| Metric | Target | Mechanism |
|---|---|---|
| First Contentful Paint (dashboard) | < 1.5s | Server components, static shell, streaming HTML |
| Largest Contentful Paint | < 2.5s (Core Web Vitals Good) | `next/image`, no layout shift |
| Audit log scroll (10k rows) | No jank at 60fps | `@tanstack/react-virtual` |
| Policy evaluate modal open | < 100ms | Client-side only; no network |
| Live alert prepend | < 50ms perceived | Optimistic update to TanStack Query cache |
| JS bundle (initial, gzip) | < 150KB | Server components, code splitting per route |
| Connector list (50 connectors) | < 200ms render | Server component, static HTML from BFF |

Bundle budget enforcement: `next build` runs `@next/bundle-analyzer` in CI. Any route whose client JS exceeds 80KB (gzip) fails the build.

---

## 13. Testing Standards

### 13.1 Unit & Component Tests (Vitest + Testing Library)

Coverage floor: 70% per module (mirrors backend standard).

```ts
// Example: ApiKeyReveal test
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiKeyReveal } from "@/components/security/api-key-reveal";

test("key is masked by default", () => {
  render(<ApiKeyReveal apiKey="sk_test_abcdef1234567890" />);
  expect(screen.queryByText("sk_test_abcdef1234567890")).not.toBeInTheDocument();
  expect(screen.getByLabelText("API key")).toHaveValue("••••••••••••••••••••");
});

test("reveal requires confirmation", async () => {
  render(<ApiKeyReveal apiKey="sk_test_abcdef1234567890" />);
  await userEvent.click(screen.getByRole("button", { name: /reveal/i }));
  // Confirmation dialog must appear before the key is shown
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(screen.queryByText("sk_test_abcdef1234567890")).not.toBeInTheDocument();
});

test("key is masked again after 30 seconds", async () => {
  vi.useFakeTimers();
  render(<ApiKeyReveal apiKey="sk_test_abcdef1234567890" />);
  // ... reveal flow ...
  vi.advanceTimersByTime(30_001);
  await waitFor(() =>
    expect(screen.queryByText("sk_test_abcdef1234567890")).not.toBeInTheDocument()
  );
  vi.useRealTimers();
});
```

MSW (`msw`) is used to mock API responses in component tests. No real HTTP calls in unit tests.

### 13.2 E2E Tests (Playwright)

Key scenarios in `playwright/`:

| Test file | Scenario |
|---|---|
| `auth.spec.ts` | Login, TOTP challenge, session expiry overlay, logout |
| `connectors.spec.ts` | Register connector, copy API key, suspend, test webhook |
| `audit.spec.ts` | Filter events, cursor pagination, export job polling |
| `threats.spec.ts` | Alert list, acknowledge, resolve, MTTR computed |
| `policy.spec.ts` | Create policy, ETag conflict handling |
| `compliance.spec.ts` | Trigger GDPR report, poll to completion, download |
| `dlp.spec.ts` | Create block-mode policy, verify 422 on ingest |
| `mfa.spec.ts` | TOTP enrollment, replay rejection, WebAuthn registration |
| `rbac.spec.ts` | Org admin vs member permission gates |

E2E tests run against `docker compose up` (all real services). They use test-seeded orgs with known credentials (`scripts/seed.sh`).

### 13.3 Accessibility Tests

`axe-playwright` runs on every Playwright test. Any `critical` or `serious` axe violation fails the test.

---

## 14. CI/CD Pipeline

```yaml
# .github/workflows/ci.yml (web job)
web-test:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with: { node-version: '20', cache: 'npm', cache-dependency-path: web/package-lock.json }
    - run: cd web && npm ci
    - run: cd web && npm run type-check          # tsc --noEmit
    - run: cd web && npm run lint                 # eslint --max-warnings 0
    - run: cd web && npm run test                 # vitest run --coverage
    - name: Coverage gate (70% floor)
      run: cd web && npx vitest --coverage --reporter=json | node scripts/check-coverage.mjs
    - run: cd web && npm run build               # next build (also runs bundle analysis)
    - run: cd web && npm audit --audit-level=high

web-e2e:
  runs-on: ubuntu-latest
  needs: [go-test, web-test]                    # E2E requires all services healthy
  services: { ... }                             # Same service stack as go-test
  steps:
    - run: npm run e2e                           # playwright test
```

---

## 15. Environment Variables

```dotenv
# ── Next.js public (exposed to browser) ──────────────────────────────
NEXT_PUBLIC_API_URL=https://api.openguard.example.com
NEXT_PUBLIC_OTEL_ENDPOINT=https://otel.openguard.example.com
NEXT_PUBLIC_OTEL_SAMPLING_RATE=0.1

# ── Server-only (BFF proxy, never reaches browser) ───────────────────
NEXTAUTH_URL=https://openguard.example.com
NEXTAUTH_SECRET=change-me-32-bytes             # next-auth JWT encryption key
API_INTERNAL_URL=http://control-plane:8080     # Internal service URL (no public TLS needed)
IAM_INTERNAL_URL=http://iam:8081

# ── OIDC provider (for next-auth) ────────────────────────────────────
OIDC_CLIENT_ID=openguard-web
OIDC_CLIENT_SECRET=change-me
OIDC_ISSUER=https://accounts.openguard.example.com

# ── Feature flags ────────────────────────────────────────────────────
NEXT_PUBLIC_ENABLE_WEBAUTHN=true
NEXT_PUBLIC_ENABLE_LIVE_ALERTS=true
NEXT_PUBLIC_BUNDLE_ANALYZE=false              # Set true to open bundle report
```

All server-only variables are validated at build time via `types/env.d.ts` + a startup check that panics if any required variable is absent (mirrors the backend `config.Must` pattern).

---

## 16. Full-System Acceptance Criteria

These criteria cover the frontend's role in the backend spec's §20 end-to-end scenario.

- [ ] Login with valid credentials → JWT session cookie set (`HttpOnly`, `Secure`, `SameSite=Strict`). Token never appears in browser JS.
- [ ] Login with invalid credentials → generic `INVALID_CREDENTIALS` error. Account locked status not revealed.
- [ ] TOTP challenge → correct code succeeds; replayed code returns `TOTP_REPLAY_DETECTED` banner without exposing the reason code to the user.
- [ ] WebAuthn registration completes; credential appears in `SessionList`.
- [ ] Connector registration wizard: 3 steps, API key shown in `ApiKeyReveal`, masked by default, auto-masks after 30s, discarded from state on unmount.
- [ ] Connector suspension: table row status badge updates optimistically; toast confirms.
- [ ] Audit log: 10,000 rows render without scroll jank (virtual scrolling). Cursor pagination loads next page on "Load more".
- [ ] Audit log filter by actor → query param reflected in URL (shareable link).
- [ ] DLP masked field (`[REDACTED:ssn]`) renders as info badge in audit event payload.
- [ ] Threat alert appears in the list within 5s of backend detection (SSE).
- [ ] Alert acknowledge → saga timeline updates. Alert resolve → MTTR appears on row.
- [ ] Compliance report: trigger GDPR → polling progress indicator → download link appears on completion.
- [ ] Session expiry overlay: any `401 TOKEN_EXPIRED` response triggers full-screen overlay, blocking all interaction.
- [ ] Session risk revocation: `401 SESSION_REVOKED_RISK` triggers the same overlay with a "unusual activity" message.
- [ ] SCIM-provisioned user with `status: initializing` → login rejected with informative message, dashboard not shown.
- [ ] JWT key rotation: old access token returns `401` → silent refresh → new token used → user session continues uninterrupted.
- [ ] `npm audit --audit-level=high` → zero findings.
- [ ] `axe-playwright` → zero critical or serious violations across all E2E scenarios.
- [ ] Bundle analysis: no initial client route exceeds 80KB (gzip).
- [ ] `npm run type-check` → zero TypeScript errors.
- [ ] All Playwright E2E scenarios pass against a live `docker compose up` stack.

---

## Appendix A — Component ↔ Backend Endpoint Mapping

| Component / Hook | Backend endpoint | Service |
|---|---|---|
| `ConnectorTable` | `GET /v1/admin/connectors` | control-plane |
| `ConnectorRegistrationWizard` | `POST /v1/admin/connectors` | control-plane |
| `ConnectorDetail` | `GET /v1/admin/connectors/:id` | control-plane |
| `WebhookDeliveryLog` | `GET /v1/admin/connectors/:id/deliveries` | webhook-delivery |
| `EventVolumeBar` | ClickHouse `event_counts_daily` via compliance service | compliance |
| `AuditTable` | `GET /audit/events` | audit |
| `AuditIntegrity` | `GET /audit/integrity` | audit |
| `AlertTable` | `GET /v1/threats/alerts` | threat |
| `useLiveAlerts` | `GET /v1/threats/alerts/stream` (SSE) | threat |
| `AlertDetail` | `GET /v1/threats/alerts/:id` | threat |
| `PolicyEditor` | `POST /PUT /v1/policies/:id` | policy |
| `PolicyEvalLog` | `GET /v1/policy/eval-logs` | policy |
| `ComplianceReportList` | `GET /v1/compliance/reports` | compliance |
| `DlpFindingsTable` | `GET /v1/dlp/findings` | dlp |
| `DlpPolicyEditor` | `POST/PUT /v1/dlp/policies` | dlp |
| `UserList` | `GET /users` | iam |
| `SessionList` | `GET /users/:id/sessions` | iam |
| `ApiKeyReveal` | Response from `POST /v1/admin/connectors` | control-plane |
| `MfaEnroll` | `POST /auth/mfa/enroll` + `POST /auth/mfa/verify` | iam |
| `WebAuthnRegister` | `POST /auth/webauthn/register/begin` + `/finish` | iam |
| `OrgSettings` | `GET/PATCH /orgs/me` | iam |
| `ScimTokenManager` | `IAM_SCIM_TOKENS_JSON` (read-only display) | iam |

---

## Appendix B — Known Trade-offs

| Decision | Alternative | Reason |
|---|---|---|
| BFF proxy for all API calls | Direct browser → backend | Auth token never reaches browser JS. SSRF validation happens server-side. |
| `HttpOnly` cookies for session | `localStorage` | `localStorage` is XSS-readable. `HttpOnly` cookies are not accessible to JavaScript. |
| TanStack Query for server state | SWR | More granular cache control; `useInfiniteQuery` covers cursor pagination cleanly. |
| Virtual scrolling for audit log | Server-side pagination only | Loading 50 rows per page is fast but requires full page reloads for navigation. Virtual scrolling with `useInfiniteQuery` provides a smoother investigative experience. |
| API key destroyed from state on unmount | Store in Zustand for "show again" | The spec requires one-time display. Any persistent storage reintroduces the risk of the key being recovered from memory dumps or devtools. |
| Recharts for charts | Chart.js / D3 | Recharts is composable React components. D3 is lower-level; Chart.js is imperative. Recharts matches the declarative Next.js model. |
| Code-splitting per route | Single bundle | Audit log, compliance, and threat modules pull in heavy dependencies (virtualization, charting). Per-route splitting keeps the initial dashboard load fast. |
