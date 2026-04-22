# §03 — Auth & Session

Mirrors the IAM service (BE spec §10.3). All auth is OIDC-based, with TOTP and WebAuthn MFA as a required second factor when enabled by the org.

---

## 3.1 Angular Auth Service (OIDC)

In Angular, we use a custom `AuthService` (often wrapping `angular-auth-oidc-client` or similar) to manage the OIDC flow, tokens, and session state.

```typescript
// src/app/core/auth/auth.service.ts
import { Injectable, signal, computed } from '@angular/core';

export interface AuthSession {
  accessToken: string;
  orgId: string;
  mfaRequired: boolean;
  mfaVerified: boolean;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  // Signals for reactive session state
  readonly session = signal<AuthSession | null>(null);
  readonly isAuthenticated = computed(() => !!this.session());
  readonly accessToken = computed(() => this.session()?.accessToken);
  readonly orgId = computed(() => this.session()?.orgId);

  login() {
    // Initiate OIDC PKCE redirect flow
  }

  handleCallback() {
    // Exchange code for tokens, update session signal
  }

  logout() {
    this.session.set(null);
    // Call revocation endpoint, redirect to login
  }
}
```

### Auth Guard

Angular Guards protect routes based on the session state.

```typescript
// src/app/core/auth/auth.guard.ts
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { AuthService } from './auth.service';

export const authGuard = () => {
  const auth = inject(AuthService);
  const router = inject(Router);

  if (!auth.isAuthenticated()) {
    return router.parseUrl('/login');
  }

  const session = auth.session();
  if (session?.mfaRequired && !session?.mfaVerified) {
    return router.parseUrl('/mfa/totp');
  }

  return true;
};
```

### Token Refresh

Refresh is handled within the `AuthService` or via an `HttpInterceptor`.

```typescript
// src/app/core/auth/auth.service.ts (continued)
private refreshToken() {
  return this.http.post(`${environment.iamIssuer}/oauth/token`, {
    grant_type: 'refresh_token',
    refresh_token: this.currentRefreshToken,
    client_id: environment.iamClientId
  }).pipe(
    tap(data => this.updateSession(data)),
    catchError(() => {
      this.logout();
      return EMPTY;
    })
  );
}
```

**Session error handling in the app shell:** If `session.error === 'RefreshAccessTokenError'`, display a full-page overlay: "Your session has expired. Please log in again." with a sign-in button. Do not silently fail API calls.

---

## 3.2 Login Page

```
Route: /login  (app/(auth)/login/page.tsx)
```

**Layout:** Centered card on dark background. OpenGuard wordmark (JetBrains Mono, medium weight) + a single "Sign in with OpenGuard" button that triggers the OIDC redirect.

**OIDC redirect flow:**
1. User clicks "Sign in".
2. NextAuth generates PKCE `code_verifier` + `code_challenge` (S256).
3. Redirect to `IAM_OIDC_ISSUER/oauth/authorize` with `code_challenge`, `state`, and `redirect_uri`.
4. IAM authenticates, returns authorization code.
5. NextAuth exchanges code + `code_verifier` for tokens.
6. If `mfa_required: true` in the token payload → redirect to `/mfa/totp` or `/mfa/webauthn`.

**Error states:**
- `OAuthCallbackError` → "Sign-in failed. Please try again."
- `RefreshAccessTokenError` → "Session expired. Please sign in again."
- User status `initializing` → "Your account is being set up. This usually takes under a minute. Please try again shortly." (BE: saga in progress — §2.5)
- User status `provisioning_failed` → "Account setup failed. Please contact your administrator."
- User status `suspended` → "Your account has been suspended. Please contact your administrator."

---

## 3.3 TOTP MFA Screen

```
Route: /mfa/totp  (app/(auth)/mfa/totp/page.tsx)
```

**UI:** Single 6-digit OTP input. Auto-submits when 6 digits are entered. "Use a backup code" link below.

```tsx
// Calls POST /auth/mfa/challenge via NextAuth custom action
// On success: session.mfaVerified = true → redirect to /overview
// On error TOTP_REPLAY_DETECTED: "This code was already used. Please wait for the next one."
// On error INVALID_TOTP: "Incorrect code. Please try again."
// After 5 failures: brief cooldown (30s countdown displayed)
```

**Backup code flow:**
- Modal with single text input. Formats input as `XXXX-XXXX` on change.
- On success: warning toast "One backup code used. You have N remaining."
- On 0 remaining: banner "You have no backup codes left. Generate new codes in your profile."

---

## 3.4 WebAuthn MFA Screen

```
Route: /mfa/webauthn  (app/(auth)/mfa/webauthn/page.tsx)
```

**UI:** Large "Verify with passkey" button with security key icon. Triggers `navigator.credentials.get()`.

```tsx
'use client'
// 1. GET /auth/webauthn/login/begin → challenge options from server
// 2. navigator.credentials.get(options) → browser prompts authenticator
// 3. POST /auth/webauthn/login/finish → verify assertion
// 4. On success: session updated, redirect to /overview
//
// Fallback: "Use TOTP instead" link if user has both enrolled.
//
// Error handling:
//   NotAllowedError (user cancelled) → "Touch cancelled. Try again when ready."
//   Timeout → "Request timed out. Please try again."
//   InvalidStateError → "Authenticator not recognized. Try a different security key."
```

---

## 3.5 useOrg Replacement

In Angular, components inject the `AuthService` and use its `orgId` signal.

```typescript
// src/app/features/dashboard/dashboard.component.ts
export class DashboardComponent {
  private auth = inject(AuthService);
  orgId = this.auth.orgId; // Computed Signal
}
```

`orgId` is always sourced from the authenticated session. It is never read from URL params, headers, or local storage.

---

## 3.6 User Status Banner (SCIM Provisioning States)

When the logged-in user's provisioning status is not `complete`, display a non-dismissible banner at the top of the app shell:

```tsx
// components/layout/provisioning-banner.tsx
// user.provisioning_status values:
//   'complete'            → no banner
//   'initializing'        → "Your account is being provisioned. Some features may be limited."
//   'provisioning_failed' → "Account provisioning failed. Contact your administrator."
//                           [Retry button if admin] → POST /users/:id/reprovision
//
// Banner persists until status transitions to 'complete'.
// Polling: useQuery with refetchInterval=10000 while status !== 'complete'.
```

---

## 3.7 Session Revocation Awareness

The BE immediately revokes JTIs on logout or `user.deleted` events. The frontend handles:

1. **Normal logout:** `signOut()` → calls `POST /auth/logout` → redirects to `/login`.

2. **Session revoked server-side** (e.g. admin suspended user mid-session):
   - The next API call returns `401 SESSION_REVOKED_RISK` or `401 FORBIDDEN`.
   - The API client interceptor catches 401 with these codes and calls `signOut({ callbackUrl: '/login?reason=revoked' })`.
   - Login page shows banner: "You were signed out because your session was revoked."

3. **High-risk session refresh revocation** (`SESSION_REVOKED_RISK`):
   - API client handles 401 from `/auth/refresh`.
   - Forces sign-out with banner: "Your session was ended due to suspicious activity. Please sign in again."

---

## 3.8 MFA Enrollment Flow (Profile Settings)

```
Route: /org/settings → "Security" tab → "Multi-factor authentication"
```

**TOTP enrollment:**
1. `POST /auth/mfa/enroll` → returns `{ secret, otpauth_uri }`.
2. Display QR code (`otpauth_uri` → `<QRCode />` using `qrcode.react`).
3. Display base32 secret as text (copy button).
4. 6-digit verification input → `POST /auth/mfa/verify`.
5. On success: display 8 backup codes in a one-time reveal panel. User must click "I've saved these codes" to dismiss.

**WebAuthn enrollment:**
1. `POST /auth/webauthn/register/begin` → options.
2. `navigator.credentials.create(options)`.
3. `POST /auth/webauthn/register/finish`.
4. Prompt user to name the credential (e.g. "YubiKey 5C").
5. Show registered authenticators in a list with "Remove" per device.

---

## 3.9 SCIM Provisioning State UI (Admin View)

Org admins see a user's provisioning saga state in `/users/[id]`:

```tsx
// Saga step visualization — shows which step is active/failed
// Maps to the saga in BE spec §2.5:
const SAGA_STEPS = [
  { key: 'user_created',        label: 'User record created' },
  { key: 'policy_assigned',     label: 'Default policies assigned' },
  { key: 'threat_baseline',     label: 'Threat baseline initialized' },
  { key: 'alert_prefs',         label: 'Alert preferences configured' },
  { key: 'saga_completed',      label: 'Account activated' },
]

// Each step shows: pending (gray dot) | in_progress (cyan pulse) | complete (green check) | failed (red X)
// If status = 'provisioning_failed', show retry button (POST /users/:id/reprovision)
```
