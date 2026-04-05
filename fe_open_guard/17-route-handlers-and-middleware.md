# §17 — Route Handlers & Middleware

Next.js route handlers serve two purposes: proxying SSE streams from the backend (so auth tokens stay server-side), and providing thin API shims where the frontend needs to aggregate or transform BE responses.

---

## 17.1 Middleware

```ts
// middleware.ts
// Runs on every request before it reaches a page or route handler.
// Responsibilities:
//   1. Auth guard — redirect unauthenticated users to /login
//   2. MFA guard — redirect un-verified MFA users to /mfa/totp or /mfa/webauthn
//   3. Session refresh — detect expired tokens and trigger refresh before page load
//   4. CSP nonce injection — per-request nonce for script-src

import { auth } from '@/lib/auth/config'
import { NextResponse, type NextRequest } from 'next/server'

const PUBLIC_PATHS = ['/login', '/api/auth', '/_next', '/favicon.ico']

export default auth(async function middleware(req: NextRequest & { auth?: any }) {
  const pathname = req.nextUrl.pathname
  const isPublic = PUBLIC_PATHS.some(p => pathname.startsWith(p))

  if (isPublic) return NextResponse.next()

  const session = req.auth

  // 1. Not authenticated → /login
  if (!session) {
    return NextResponse.redirect(new URL(`/login?callbackUrl=${encodeURIComponent(pathname)}`, req.url))
  }

  // 2. Session error (e.g. refresh token expired) → /login
  if (session.error === 'RefreshAccessTokenError') {
    return NextResponse.redirect(new URL('/login?reason=session_expired', req.url))
  }

  // 3. MFA required but not yet verified → /mfa
  const isMFAPage = pathname.startsWith('/mfa')
  if (session.mfaRequired && !session.mfaVerified && !isMFAPage) {
    const mfaRoute = session.user?.mfaMethod === 'webauthn' ? '/mfa/webauthn' : '/mfa/totp'
    return NextResponse.redirect(new URL(mfaRoute, req.url))
  }

  // 4. Inject CSP nonce
  const nonce = Buffer.from(crypto.randomUUID()).toString('base64')
  const res = NextResponse.next({
    request: {
      headers: new Headers({ ...Object.fromEntries(req.headers), 'x-nonce': nonce }),
    },
  })
  // Update CSP header with nonce (replaces {nonce} placeholder from next.config.js)
  const csp = res.headers.get('Content-Security-Policy')
  if (csp) {
    res.headers.set('Content-Security-Policy', csp.replace('{nonce}', nonce))
  }
  return res
})

export const config = {
  matcher: ['/((?!_next/static|_next/image|favicon.ico).*)'],
}
```

---

## 17.2 SSE Route Handlers

### Audit Stream

```ts
// app/api/stream/audit/route.ts
// Proxies the audit event SSE stream from the audit service.
// Auth token is added server-side — never exposed to the browser.

import { auth } from '@/lib/auth/config'
import { NextRequest } from 'next/server'

export const runtime = 'nodejs'    // SSE requires Node.js runtime (not Edge)
export const dynamic = 'force-dynamic'

export async function GET(req: NextRequest) {
  const session = await auth()

  if (!session?.accessToken) {
    return new Response('Unauthorized', { status: 401 })
  }

  // Forward org_id from session (not from URL params — security boundary)
  const orgId = session.orgId

  const upstream = await fetch(
    `${process.env.INTERNAL_API_URL}/audit/stream?org_id=${orgId}`,
    {
      headers: {
        Authorization:  `Bearer ${session.accessToken}`,
        Accept:         'text/event-stream',
        'Cache-Control': 'no-cache',
      },
      // AbortController tied to request lifecycle
      signal: req.signal,
    }
  )

  if (!upstream.ok) {
    return new Response(upstream.statusText, { status: upstream.status })
  }

  return new Response(upstream.body, {
    headers: {
      'Content-Type':  'text/event-stream',
      'Cache-Control': 'no-cache, no-transform',
      Connection:      'keep-alive',
      'X-Accel-Buffering': 'no',    // disable nginx buffering (important for SSE)
    },
  })
}
```

### Threat Alert Stream

```ts
// app/api/stream/threats/route.ts
// Same pattern as audit stream — proxies the threat alerting SSE stream.
// Forwards only new 'open' alerts (server-side filter).

export async function GET(req: NextRequest) {
  const session = await auth()
  if (!session?.accessToken) return new Response('Unauthorized', { status: 401 })

  const upstream = await fetch(
    `${process.env.INTERNAL_API_URL}/v1/threats/stream`,
    {
      headers: {
        Authorization:  `Bearer ${session.accessToken}`,
        Accept:         'text/event-stream',
      },
      signal: req.signal,
    }
  )

  return new Response(upstream.body, {
    headers: {
      'Content-Type':  'text/event-stream',
      'Cache-Control': 'no-cache, no-transform',
      Connection:      'keep-alive',
      'X-Accel-Buffering': 'no',
    },
  })
}
```

---

## 17.3 Aggregation Route Handlers

Some dashboard views need data from multiple BE services. These are composed server-side to avoid client-side waterfall requests.

### System Status (aggregates all service health checks)

```ts
// app/api/admin/system-status/route.ts
import { auth } from '@/lib/auth/config'

export async function GET() {
  const session = await auth()
  if (!session?.accessToken) return Response.json({ error: 'Unauthorized' }, { status: 401 })

  // Parallel fetch from all services
  const [services, outbox, circuitBreakers] = await Promise.allSettled([
    fetch(`${process.env.INTERNAL_API_URL}/admin/health/services`, {
      headers: { Authorization: `Bearer ${session.accessToken}` },
    }).then(r => r.json()),
    fetch(`${process.env.INTERNAL_API_URL}/admin/metrics/outbox`, {
      headers: { Authorization: `Bearer ${session.accessToken}` },
    }).then(r => r.json()),
    fetch(`${process.env.INTERNAL_API_URL}/admin/metrics/circuit-breakers`, {
      headers: { Authorization: `Bearer ${session.accessToken}` },
    }).then(r => r.json()),
  ])

  return Response.json({
    services:         services.status === 'fulfilled' ? services.value : [],
    outbox:           outbox.status === 'fulfilled' ? outbox.value : [],
    circuit_breakers: circuitBreakers.status === 'fulfilled' ? circuitBreakers.value : [],
  })
}
```

### Overview Page Data (parallel fetch)

```ts
// app/api/overview/route.ts
// Aggregates: alert counts, recent audit events, compliance posture, event stats
// Returns in a single response to avoid 5 sequential client fetches on page load.

export async function GET() {
  const session = await auth()
  if (!session?.accessToken) return Response.json({ error: 'Unauthorized' }, { status: 401 })

  const h = { Authorization: `Bearer ${session.accessToken}` }
  const base = process.env.INTERNAL_API_URL

  const [alerts, auditStats, posture, eventCounts] = await Promise.allSettled([
    fetch(`${base}/v1/threats/stats`, { headers: h }).then(r => r.json()),
    fetch(`${base}/audit/stats`, { headers: h }).then(r => r.json()),
    fetch(`${base}/v1/compliance/posture`, { headers: h }).then(r => r.json()),
    fetch(`${base}/v1/compliance/stats`, { headers: h }).then(r => r.json()),
  ])

  return Response.json({
    alerts:       alerts.status === 'fulfilled' ? alerts.value : null,
    audit_stats:  auditStats.status === 'fulfilled' ? auditStats.value : null,
    posture:      posture.status === 'fulfilled' ? posture.value : null,
    event_counts: eventCounts.status === 'fulfilled' ? eventCounts.value : null,
  })
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
