# §13 — Testing & Quality

---

## 13.1 Testing Strategy

| Layer | Tool | Scope | Threshold |
|---|---|---|---|
| Unit (utils, validators, services) | Jasmine/Karma | Pure functions, Services | 80% coverage |
| Component tests | Angular Testing Library | UI behavior, not implementation | 80% coverage |
| Integration (API services) | HttpClientTestingModule | Request/response contracts | 100% of API modules |
| E2E (critical paths) | Playwright | Full user journeys | All flows in §13.5 |
| Accessibility | axe-playwright | Every E2E page | 0 WCAG AA violations |
| Visual regression | Playwright screenshots | Design system primitives | Opt-in per PR |
| Performance | Lighthouse CI | Core Web Vitals | See §13.6 |

---

## 13.2 Unit Tests (Jasmine)

### What to test

- All functions in `src/app/core/utils/` (formatting, cursor encoding/decoding, idempotency key generation).
- All validators and types.
- Error message mapping.

### Example: cursor encoding

```typescript
// src/app/core/utils/pagination.spec.ts
import { encodeCursor, decodeCursor } from './pagination';

describe('cursor encoding', () => {
  it('should round-trip a cursor', () => {
    const original = { t: 1705329600000, id: 'evt_01j...' };
    expect(decodeCursor(encodeCursor(original.t, original.id))).toEqual(original);
  });

  it('should return null for malformed cursor', () => {
    expect(decodeCursor('not-valid-base64!!')).toBeNull();
  });
});
```

---

## 13.3 Component Tests (Angular Testing Library)

### Principles

- Test user-visible behavior.
- Mock API services using Jasmine spies or mock classes.

### Example: ConnectorCard suspend

```typescript
// src/app/features/connectors/connector-card.spec.ts
import { render, screen, fireEvent } from '@testing-library/angular';
import { ConnectorCardComponent } from './connector-card.component';

it('shows confirm dialog before suspending', async () => {
  const onSuspend = jasmine.createSpy('onSuspend');
  await render(ConnectorCardComponent, {
    componentProperties: { connector: mockConnector, suspend: onSuspend }
  });

  const button = screen.getByRole('button', { name: /suspend/i });
  fireEvent.click(button);

  // Confirm dialog should appear (verification logic)
  expect(screen.getByRole('dialog')).toBeTruthy();
});
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
    baseURL: 'http://localhost:4200',
    storageState: 'e2e/.auth/admin.json', // pre-authenticated state
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile',   use: { ...devices['iPhone 14'] } },
  ],
  webServer: {
    command: 'npm run dev',
    url: 'http://localhost:4200',
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
