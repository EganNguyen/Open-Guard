# §14 — Environment & Angular Configuration

In Angular, configuration is managed via `src/environments/environment.ts` (build-time) or a runtime config service.

---

## 14.1 Environment Configuration

```typescript
// src/environments/environment.ts
export const environment = {
  production: false,
  apiUrl: 'http://localhost:8080',
  appUrl: 'http://localhost:4200',
  iamIssuer: 'http://localhost:8081',
  iamClientId: 'openguard-dashboard',
  
  features: {
    dlpBlockMode: true,
    webauthn: true,
    scim: true
  }
};
```

### Runtime Configuration (Preferred for Production)

For production, we use a `config.json` fetched at startup to avoid re-building for different environments.

```typescript
// src/app/core/config/config.service.ts
@Injectable({ providedIn: 'root' })
export class ConfigService {
  private config = signal<AppConfig | null>(null);

  loadConfig() {
    return this.http.get<AppConfig>('/assets/config.json').pipe(
      tap(cfg => this.config.set(cfg))
    );
  }

  get<K extends keyof AppConfig>(key: K): AppConfig[K] {
    return this.config()![key];
  }
}
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
```

---

## 14.2 `angular.json` & Proxy

In development, we use `proxy.conf.json` to route API calls to the local backend.

```json
// proxy.conf.json
{
  "/v1": {
    "target": "http://localhost:8080",
    "secure": false,
    "changeOrigin": true
  }
}
```

### Security Headers (SSR)

If using Angular SSR, security headers are configured in the Node.js server wrapper (`server.ts`).

```typescript
// server.ts
server.get('*', (req, res, next) => {
  res.setHeader('Strict-Transport-Security', 'max-age=63072000');
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('X-Frame-Options', 'DENY');
  res.setHeader('Content-Security-Policy', "default-src 'self'; ...");
  next();
});
```

---

## 14.3 Tailwind Configuration

```typescript
// tailwind.config.ts
export default {
  content: [
    "./src/**/*.{html,ts}",
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

Angular projects use a standard `tsconfig.json` with strict mode enabled.

```json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "target": "ES2022"
  }
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
