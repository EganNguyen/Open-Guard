# §17 — Guards & Interceptors

In Angular, we use **Guards** for navigation control and **Interceptors** for request-level logic (auth, headers, error normalization).

---

## 17.1 Navigation Guards

```typescript
// src/app/core/auth/auth.guard.ts
import { inject } from '@angular/core';
import { Router, CanActivateFn } from '@angular/router';
import { AuthService } from './auth.service';

export const authGuard: CanActivateFn = (route, state) => {
  const auth = inject(AuthService);
  const router = inject(Router);

  // 1. Not authenticated -> /login
  if (!auth.isAuthenticated()) {
    return router.parseUrl(`/login?returnUrl=${encodeURIComponent(state.url)}`);
  }

  const session = auth.session();

  // 2. Session error -> /login
  if (session?.error === 'RefreshAccessTokenError') {
    return router.parseUrl('/login?reason=session_expired');
  }

  // 3. MFA required but not yet verified -> /mfa
  if (session?.mfaRequired && !session?.mfaVerified && !state.url.startsWith('/mfa')) {
    const mfaRoute = session.user?.mfaMethod === 'webauthn' ? '/mfa/webauthn' : '/mfa/totp';
    return router.parseUrl(mfaRoute);
  }

  return true;
};
```

## 17.2 HTTP Interceptor

```typescript
// src/app/core/api/api.interceptor.ts
import { HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { AuthService } from '../auth/auth.service';

export const apiInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(AuthService);
  
  const apiReq = req.clone({
    setHeaders: {
      Authorization: `Bearer ${auth.accessToken()}`,
      'X-Org-ID': auth.orgId() ?? '',
    }
  });

  return next(apiReq);
};
```

---

## 17.3 SSE Subscriptions (Direct to BE)

In Angular, the `SseService` connects directly to the backend. To handle authentication (since `EventSource` doesn't support headers), we use a short-lived **SSE session token** or a query parameter pattern.

```typescript
// src/app/core/api/sse.service.ts
connectToAuditStream(orgId: string) {
  const token = this.auth.accessToken();
  // Authorization via query param (Standard for EventSource)
  const url = `${environment.apiUrl}/audit/stream?org_id=${orgId}&token=${token}`;
  return new EventSource(url);
}
```

---

## 17.4 Client-Side Aggregation

In Angular, we use RxJS `forkJoin` or `combineLatest` within services to aggregate data when a dedicated BE endpoint is not available.

```typescript
// src/app/core/api/overview.service.ts
getOverviewData() {
  return forkJoin({
    alerts: this.threatsService.getStats(),
    audit: this.auditService.getStats(),
    posture: this.complianceService.getPosture()
  });
}
```

---

## 17.4 Report Download Redirect

```ts
// app/api/compliance/reports/[id]/download/route.ts
// Fetches the pre-signed S3 URL from the BE and redirects the browser to it.
// The pre-signed URL is not exposed in the frontend JS bundle.

import { auth } from '@/lib/auth/config'

export async function GET(
  _req: Request,
  { params }: { params: { id: string } }
) {
  const session = await auth()
  if (!session?.accessToken) return new Response('Unauthorized', { status: 401 })

  const res = await fetch(
    `${process.env.INTERNAL_API_URL}/v1/compliance/reports/${params.id}`,
    { headers: { Authorization: `Bearer ${session.accessToken}` } }
  )

  if (!res.ok) return new Response('Not found', { status: 404 })

  const report = await res.json()
  if (!report.download_url) return new Response('Not ready', { status: 409 })

  // Redirect browser to pre-signed S3 URL (1 hour TTL, per BE spec §14.3)
  return Response.redirect(report.download_url, 302)
}
```

---

## 17.5 SCIM Token Rotation Handler

```ts
// app/api/org/scim-token/route.ts
// Rotates the SCIM bearer token for the current org.
// Server Action alternative — kept as route handler for testability.

export async function POST() {
  const session = await auth()
  if (!session?.accessToken) return Response.json({ error: 'Unauthorized' }, { status: 401 })

  const res = await fetch(
    `${process.env.INTERNAL_IAM_URL}/orgs/${session.orgId}/scim-token/rotate`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${session.accessToken}` },
    }
  )

  if (!res.ok) {
    const err = await res.json()
    return Response.json(err, { status: res.status })
  }

  const { token } = await res.json()
  return Response.json({ token })
}
```

---

## 17.6 MFA Challenge Handler (Server Action)

```ts
// app/(auth)/mfa/totp/actions.ts
'use server'

import { auth, signIn } from '@/lib/auth/config'

export async function verifyTOTP(code: string) {
  const session = await auth()
  if (!session) throw new Error('Not authenticated')

  const res = await fetch(
    `${process.env.INTERNAL_IAM_URL}/auth/mfa/challenge`,
    {
      method: 'POST',
      headers: {
        'Content-Type':  'application/json',
        Authorization:   `Bearer ${session.accessToken}`,
      },
      body: JSON.stringify({ code }),
    }
  )

  if (!res.ok) {
    const err = await res.json()
    // Return typed error code — do NOT throw (server actions surface errors as client-visible messages)
    return { success: false, code: err.error?.code ?? 'UNKNOWN' }
  }

  // Update session to mark MFA as verified
  // NextAuth v5: update() triggers a session refresh with the new mfaVerified=true claim
  // This requires the IAM token endpoint to return a new token with the claim set.
  return { success: true }
}
```
