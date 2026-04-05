# §14 — Environment Variables & Next.js Configuration

Mirrors BE spec §5 (Environment & Configuration). All environment variables the frontend requires are defined here. The `next.config.js` enforces security headers and CSP.

---

## 14.1 Environment Variables

```dotenv
# ── Public (exposed to browser via NEXT_PUBLIC_ prefix) ──────────────
# Base URL for the control plane — used by the browser for API calls
NEXT_PUBLIC_API_URL=http://localhost:8080

# Auth
NEXT_PUBLIC_APP_URL=http://localhost:3000

# Feature flags (safe to expose)
NEXT_PUBLIC_DLP_BLOCK_MODE_ENABLED=true
NEXT_PUBLIC_WEBAUTHN_ENABLED=true
NEXT_PUBLIC_SCIM_ENABLED=true

# ── Server-only (never prefixed with NEXT_PUBLIC_) ────────────────────
# Internal API URL — used by Next.js server-side (RSCs, route handlers)
# Points directly to the control plane inside the cluster (bypasses public ingress)
INTERNAL_API_URL=http://control-plane:8080

# Internal IAM URL — used by NextAuth.js server-side for OIDC flows
INTERNAL_IAM_URL=http://iam:8081

# NextAuth.js
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me-32-bytes-min

# OIDC (IAM service)
IAM_OIDC_ISSUER=http://iam:8081
IAM_OIDC_CLIENT_ID=openguard-dashboard
IAM_OIDC_CLIENT_SECRET=change-me

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# ── Development only ─────────────────────────────────────────────────
# Disable HTTPS check on webhook URLs in dev
NEXT_PUBLIC_DEV_MODE=false
```

### Validation at startup

```ts
// lib/env.ts
// Validates required env vars at module load time (server-side only).
// Panics early rather than discovering missing config at request time —
// same philosophy as BE spec §0.9 (MustLoad pattern).

import { z } from 'zod'

const serverEnvSchema = z.object({
  NEXTAUTH_URL:            z.string().url(),
  NEXTAUTH_SECRET:         z.string().min(32),
  IAM_OIDC_ISSUER:         z.string().url(),
  IAM_OIDC_CLIENT_ID:      z.string().min(1),
  IAM_OIDC_CLIENT_SECRET:  z.string().min(1),
  INTERNAL_API_URL:        z.string().url(),
  INTERNAL_IAM_URL:        z.string().url(),
  NODE_ENV:                z.enum(['development', 'test', 'production']),
})

const clientEnvSchema = z.object({
  NEXT_PUBLIC_API_URL:     z.string().url(),
  NEXT_PUBLIC_APP_URL:     z.string().url(),
})

// Server-side validation (runs in Node.js process)
export const serverEnv = serverEnvSchema.parse(process.env)

// Client-side validation (runs in browser — only NEXT_PUBLIC_ vars)
export const clientEnv = clientEnvSchema.parse({
  NEXT_PUBLIC_API_URL:  process.env.NEXT_PUBLIC_API_URL,
  NEXT_PUBLIC_APP_URL:  process.env.NEXT_PUBLIC_APP_URL,
})
```

---

## 14.2 `next.config.js`

```js
// next.config.js
const { INTERNAL_API_URL, INTERNAL_IAM_URL } = process.env

/** @type {import('next').NextConfig} */
const nextConfig = {
  // ── Security headers ────────────────────────────────────────────────
  // Applied to every response (matches BE spec §15.1 HTTP security headers)
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: [
          {
            key: 'Strict-Transport-Security',
            value: 'max-age=63072000; includeSubDomains; preload',
          },
          {
            key: 'X-Content-Type-Options',
            value: 'nosniff',
          },
          {
            key: 'X-Frame-Options',
            value: 'DENY',
          },
          {
            key: 'Referrer-Policy',
            value: 'no-referrer',
          },
          {
            // CSP: no inline scripts. All scripts must be from same origin or CDN.
            // 'nonce' is injected per-request by the middleware for RSC script tags.
            key: 'Content-Security-Policy',
            value: [
              "default-src 'self'",
              "script-src 'self' 'nonce-{nonce}'",
              "style-src 'self' 'unsafe-inline'",    // Tailwind requires this
              "img-src 'self' data: blob:",
              "font-src 'self' https://fonts.gstatic.com",
              "connect-src 'self' https://fonts.googleapis.com",
              "frame-ancestors 'none'",
              "form-action 'self'",
            ].join('; '),
          },
          {
            key: 'Permissions-Policy',
            value: 'camera=(), microphone=(), geolocation=()',
          },
        ],
      },
    ]
  },

  // ── Rewrites: proxy API calls through Next.js in development ───────
  // In production, requests go directly to NEXT_PUBLIC_API_URL from the browser.
  // In development, this avoids CORS issues when running locally.
  async rewrites() {
    if (process.env.NODE_ENV !== 'development') return []
    return [
      {
        source: '/api/proxy/:path*',
        destination: `${INTERNAL_API_URL}/:path*`,
      },
    ]
  },

  // ── Image domains ───────────────────────────────────────────────────
  images: {
    domains: [], // no external image domains — all avatars are initials-based
  },

  // ── Bundle analysis ─────────────────────────────────────────────────
  // Run: ANALYZE=true npm run build
  ...(process.env.ANALYZE === 'true' && {
    webpack(config) {
      const { BundleAnalyzerPlugin } = require('webpack-bundle-analyzer')
      config.plugins.push(new BundleAnalyzerPlugin({ analyzerMode: 'static' }))
      return config
    },
  }),

  // ── TypeScript strict ───────────────────────────────────────────────
  typescript: {
    ignoreBuildErrors: false,
  },

  // ── ESLint ──────────────────────────────────────────────────────────
  eslint: {
    ignoreDuringBuilds: false,
  },

  // ── Experimental ────────────────────────────────────────────────────
  experimental: {
    // Server Actions are used for form submissions that need server-side logic
    serverActions: { allowedOrigins: [process.env.NEXTAUTH_URL ?? 'http://localhost:3000'] },
  },
}

module.exports = nextConfig
```

---

## 14.3 Tailwind Configuration

```ts
// tailwind.config.ts
import type { Config } from 'tailwindcss'
import { fontFamily } from 'tailwindcss/defaultTheme'

const config: Config = {
  content: [
    './app/**/*.{ts,tsx}',
    './components/**/*.{ts,tsx}',
    './lib/**/*.{ts,tsx}',
  ],
  darkMode: 'class',  // OpenGuard is always dark; class="dark" set on <html>
  theme: {
    extend: {
      fontFamily: {
        display: ['"JetBrains Mono"', ...fontFamily.mono],
        body:    ['"IBM Plex Sans"', ...fontFamily.sans],
        mono:    ['"JetBrains Mono"', ...fontFamily.mono],
      },
      colors: {
        og: {
          bg: {
            base:     '#09090B',
            surface:  '#111113',
            elevated: '#18181B',
            overlay:  '#27272A',
          },
          border:       '#27272A',
          'border-subtle': '#1C1C1F',
          text: {
            primary:   '#FAFAFA',
            secondary: '#A1A1AA',
            muted:     '#52525B',
            disabled:  '#3F3F46',
          },
          accent:       '#06B6D4',
          'accent-dim': '#0E7490',
          'accent-glow':'rgba(6,182,212,0.12)',
          success:      '#22C55E',
          'success-dim':'rgba(34,197,94,0.12)',
          warning:      '#F59E0B',
          'warning-dim':'rgba(245,158,11,0.12)',
          danger:       '#EF4444',
          'danger-dim': 'rgba(239,68,68,0.12)',
          critical:     '#FF2056',
          info:         '#3B82F6',
          sev: {
            low:      '#6B7280',
            medium:   '#F59E0B',
            high:     '#EF4444',
            critical: '#FF2056',
          },
        },
      },
      animation: {
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
      },
      keyframes: {
        shimmer: {
          '0%':   { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0'  },
        },
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),  // for prose content in docs/runbook links
  ],
}

export default config
```

---

## 14.4 `tsconfig.json`

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": false,
    "skipLibCheck": true,
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitOverride": true,
    "forceConsistentCasingInFileNames": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": {
      "@/*": ["./*"]
    }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules"]
}
```

`noUncheckedIndexedAccess: true` enforces safe array/object access — aligns with the BE spec philosophy of no `nil, nil` returns and no silent failures.

---

## 14.5 `.env.example`

```dotenv
# Copy to .env.local for local development
# Never commit .env.local to version control

NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_APP_URL=http://localhost:3000
NEXT_PUBLIC_DLP_BLOCK_MODE_ENABLED=true
NEXT_PUBLIC_WEBAUTHN_ENABLED=true
NEXT_PUBLIC_SCIM_ENABLED=true

INTERNAL_API_URL=http://localhost:8080
INTERNAL_IAM_URL=http://localhost:8081

NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me-at-least-32-characters-long

IAM_OIDC_ISSUER=http://localhost:8081
IAM_OIDC_CLIENT_ID=openguard-dashboard
IAM_OIDC_CLIENT_SECRET=change-me

OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

NODE_ENV=development
NEXT_PUBLIC_DEV_MODE=true
```
