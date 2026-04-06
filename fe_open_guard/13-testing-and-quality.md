# §13 — Testing & Quality

---

## 13.1 Testing Strategy

| Layer | Tool | Scope | Threshold |
|---|---|---|---|
| Unit (utils, validators, hooks) | Vitest | Pure functions, custom hooks | 80% coverage |
| Component tests | Vitest + Testing Library | UI behavior, not implementation | 80% coverage |
| Integration (API client layer) | Vitest + MSW | Request/response contracts | 100% of API modules |
| E2E (critical paths) | Playwright | Full user journeys | All flows in §13.5 |
| Accessibility | axe-playwright | Every E2E page | 0 WCAG AA violations |
| Visual regression | Playwright screenshots | Design system primitives | Opt-in per PR |
| Performance | Lighthouse CI | Core Web Vitals | See §13.6 |

---

## 13.2 Unit Tests (Vitest)

### What to test

- All functions in `lib/utils/` (formatting, cursor encoding/decoding, idempotency key generation).
- All Zod validators in `lib/validators/`.
- All query key factories in `lib/query/keys.ts`.
- Error message mapping (`lib/utils/error-messages.ts`).

### Example: cursor encoding

```ts
// lib/utils/pagination.test.ts
import { describe, it, expect } from 'vitest'
import { encodeCursor, decodeCursor } from '@/lib/api/pagination'

describe('cursor encoding', () => {
  it('round-trips a cursor', () => {
    const original = { t: 1705329600000, id: 'evt_01j...' }
    expect(decodeCursor(encodeCursor(original.t, original.id))).toEqual(original)
  })

  it('returns null for malformed cursor', () => {
    expect(decodeCursor('not-valid-base64!!')).toBeNull()
  })
})
```

---

## 13.3 Component Tests (Testing Library)

### Principles

- Test user-visible behavior, not implementation.
- Use `screen.getByRole`, `getByLabelText`, `getByText`. Avoid `getByTestId` unless no semantic alternative.
- Mock API calls with MSW (Mock Service Worker) — not with `jest.fn()` on the API module.

### MSW setup

```ts
// test/mocks/handlers.ts
import { http, HttpResponse } from 'msw'
import { NEXT_PUBLIC_API_URL } from './constants'

export const handlers = [
  http.get(`${NEXT_PUBLIC_API_URL}/v1/connectors`, () => {
    return HttpResponse.json({ data: mockConnectors, meta: mockMeta })
  }),

  http.patch(`${NEXT_PUBLIC_API_URL}/v1/admin/connectors/:id`, async ({ request, params }) => {
    const body = await request.json() as any
    if (body.status === 'suspended') {
      return HttpResponse.json({ ...mockConnectors[0], status: 'suspended' })
    }
    return HttpResponse.json(mockConnectors[0])
  }),
  // ... etc
]
```

### Example: ConnectorCard suspend

```tsx
// components/domain/connector-card.test.tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ConnectorCard } from './connector-card'
import { mockConnector } from '@/test/fixtures/connectors'

it('shows confirm dialog before suspending', async () => {
  const onSuspend = vi.fn()
  render(<ConnectorCard connector={mockConnector} onSuspend={onSuspend} />)

  fireEvent.click(screen.getByRole('button', { name: /actions/i }))
  fireEvent.click(screen.getByRole('menuitem', { name: /suspend/i }))

  // Confirm dialog should appear
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(screen.getByText(/type "AcmeApp" to confirm/i)).toBeInTheDocument()

  // Should NOT call onSuspend yet
  expect(onSuspend).not.toHaveBeenCalled()
})
```

---

## 13.4 API Client Tests

Every module in `lib/api/` has a corresponding test file that verifies:

1. Correct URL construction.
2. Auth header is included.
3. `idempotencyKey` is forwarded when provided.
4. `OpenGuardAPIError` is thrown with the correct `code` on non-2xx responses.
5. Pagination parameters are passed correctly.

```ts
// lib/api/connectors.test.ts
import { connectorsApi } from './connectors'
import { server } from '@/test/mocks/server'
import { http, HttpResponse } from 'msw'

it('suspends a connector with PATCH', async () => {
  let capturedBody: unknown
  server.use(
    http.patch('*/v1/admin/connectors/:id', async ({ request }) => {
      capturedBody = await request.json()
      return HttpResponse.json({ ...mockConnector, status: 'suspended' })
    })
  )

  await connectorsApi.suspend('org-1', 'conn-1')
  expect(capturedBody).toEqual({ status: 'suspended' })
})

it('throws OpenGuardAPIError on 403', async () => {
  server.use(
    http.patch('*/v1/admin/connectors/:id', () =>
      HttpResponse.json({ error: { code: 'FORBIDDEN', message: 'Forbidden', request_id: '', trace_id: '', retryable: false } }, { status: 403 })
    )
  )

  await expect(connectorsApi.suspend('org-1', 'conn-1')).rejects.toMatchObject({
    code: 'FORBIDDEN',
  })
})
```

---

## 13.5 E2E Tests (Playwright)

### Critical paths — all must pass before release

| Test file | Flow |
|---|---|
| `auth.spec.ts` | Login → TOTP MFA → redirect to overview |
| `auth-webauthn.spec.ts` | Login → WebAuthn MFA (mocked credential) → overview |
| `connector-lifecycle.spec.ts` | Register → copy key → suspend → activate → delete |
| `policy-evaluate.spec.ts` | Create policy → evaluate in playground → verify result |
| `audit-stream.spec.ts` | Open audit page → verify SSE events appear in table |
| `threat-alert.spec.ts` | View alert → acknowledge → resolve → verify MTTR shown |
| `compliance-report.spec.ts` | Generate GDPR report → poll until complete → download PDF |
| `dlp-block.spec.ts` | Enable block mode → verify ingest rejected in connector delivery log |
| `user-mfa-enroll.spec.ts` | Enroll TOTP → verify backup codes shown → use backup code |
| `user-provisioning.spec.ts` | SCIM-provisioned user in initializing state → login rejected |
| `session-revoke.spec.ts` | Revoke session from admin → user gets signed out on next request |
| `key-reveal.spec.ts` | Connector registration → key reveal → confirm saved → navigate away |

### Playwright config

```ts
// playwright.config.ts
export default defineConfig({
  testDir: './e2e',
  use: {
    baseURL: 'http://localhost:3000',
    storageState: 'e2e/.auth/admin.json', // pre-authenticated state
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile',   use: { ...devices['iPhone 14'] } },
  ],
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
  },
})
```

### Auth fixture

```ts
// e2e/fixtures/auth.ts
// Pre-authenticates as an admin user and saves session state.
// Used by all E2E tests that need an authenticated session.
// Run once before test suite: npx playwright test --setup
```

---

## 13.6 Performance Budgets (Lighthouse CI)

Run on every PR against the staging deployment:

| Metric | Target |
|---|---|
| First Contentful Paint | < 1.2s |
| Largest Contentful Paint | < 2.5s |
| Total Blocking Time | < 200ms |
| Cumulative Layout Shift | < 0.1 |
| Performance score | ≥ 85 |
| Accessibility score | ≥ 95 |

**Bundle size limits (webpack-bundle-analyzer):**
- First load JS (page level): < 150KB gzipped per route
- Shared chunks: < 300KB gzipped
- Charts (Recharts): lazy-loaded — not in initial bundle

---

## 13.7 Accessibility Requirements

- All interactive elements are keyboard-navigable (Tab, Enter, Space, Arrow keys).
- All form inputs have associated `<label>` elements.
- Color is never the sole differentiator (status badges always include text label).
- All icons that convey meaning have `aria-label` or are accompanied by visible text.
- Modal dialogs trap focus and restore it on close.
- `<DataTable>` includes `role="grid"` and `aria-rowcount`.
- All chart components include a visually-hidden `<table>` fallback for screen readers.
- `prefers-reduced-motion`: all animations respect this media query.

```tsx
// tailwind.config.ts — add to safelist for motion-safe
// Use: motion-safe:animate-ping  (not just animate-ping)
```

---

## 13.8 CI Integration

```yaml
# .github/workflows/ci.yml (frontend jobs)
jobs:
  fe-type-check:
    run: npx tsc --noEmit

  fe-lint:
    run: npm run lint  # ESLint + Prettier check

  fe-test:
    run: npx vitest run --coverage
    # Coverage gate: 80% statements per file

  fe-build:
    run: npm run build
    # Fails on any TypeScript error or Next.js build warning

  fe-e2e:
    run: npx playwright test
    # Runs all critical path specs
    # Uploads playwright report as artifact on failure

  fe-lighthouse:
    run: npx lhci autorun
    # Against staging deployment; fails PR if budgets exceeded
```
