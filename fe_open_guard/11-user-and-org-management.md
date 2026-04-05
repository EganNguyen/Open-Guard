# §11 — User & Organization Management

Mirrors BE spec §10.3 (IAM Service). Covers user listing, detail, MFA management, sessions, and org settings.

---

## 11.1 User List Page

```
Route: /users
```

**Header:** "Users" title + "Invite user" button (manual user creation, non-SCIM).

**Status summary:** Active [N] | Suspended [N] | Locked [N] | Provisioning [N]

**Filter panel:**
- Status: Active | Suspended | Locked | Deprovisioned | All
- Provisioning: Complete | Initializing | Failed | All
- MFA: Enabled | Disabled | All
- Search by email or display name

**User table:**

| Column | Data |
|---|---|
| User | Avatar initials + display name + email |
| Status | `<Badge>` |
| MFA | `✅ TOTP` / `✅ WebAuthn` / `—` |
| Provisioning | Complete / Initializing / Failed |
| Last login | `<TimeAgo>` + IP (Redactable) |
| Created | `<TimeAgo>` |
| SCIM | `⊕` icon if SCIM-provisioned |
| Actions | Dropdown |

**Actions dropdown:** View, Suspend/Activate, Unlock (if locked), Revoke all sessions, Delete.

**Offset-based pagination** (page 1…N, 50 per page).

---

## 11.2 User Detail Page

```
Route: /users/[id]
```

### Profile section

```
[Avatar initials]  Display Name
                   user@example.com
                   ID: user_01j...    [Copy]
                   Org: Acme Corp
                   Status: ACTIVE
                   Created: Jan 15, 2024 (3 months ago)
                   Last login: 2 hours ago  from 203.0.113.42 [Redactable]
```

**Status actions (inline):**
- Active → [Suspend user] button
- Suspended → [Activate user] button
- Locked → [Unlock account] button (clears `locked_until`, `failed_login_count`)
- Deprovisioned → read-only; no actions

### MFA Section

```
Multi-factor authentication:  ENABLED (TOTP)

  Method: TOTP
  Enrolled: Jan 20, 2024
  Backup codes: 6 remaining

  [Revoke MFA]  ← admin force-reset; user must re-enroll on next login
```

If WebAuthn:
```
  Registered authenticators:
  ● YubiKey 5C             registered Jan 20, 2024    [Remove]
  ● Touch ID (MacBook Pro) registered Feb 1, 2024     [Remove]

  [Revoke all WebAuthn credentials]
```

### SCIM Provisioning Saga (visible for SCIM-provisioned users)

```tsx
// If user.scim_external_id is set, show saga step visualization (see §3.9)
// provisioning_status: complete | initializing | provisioning_failed
//
// If provisioning_failed:
//   [⚠ Provisioning failed]
//   Saga ID: saga_01j...   [Copy]
//   [Retry provisioning]  ← POST /users/:id/reprovision
```

### Active Sessions

```
Sessions (3 active)

  Desktop Chrome — New York, US         [Current]
  2 hours ago • 203.0.113.42 [Redactable]
  [Revoke this session]

  Mobile Safari — London, UK
  1 day ago • 185.34.22.100 [Redactable]
  [Revoke this session]

  Firefox — Berlin, DE
  3 days ago • 178.90.44.12 [Redactable]
  [Revoke this session]

[Revoke all sessions]
```

Revoking a session calls `DELETE /users/:id/sessions/:sid`. Revoking all calls `DELETE /users/:id/sessions`. The BE immediately adds the session's JTIs to the Redis blocklist (spec §2.5 `user.deleted` saga).

### API Tokens

```
API tokens (2)

  monitoring-script     ● Active    events:write    Created Jan 10  Last used 5min ago
  ci-pipeline           ● Active    audit:read      Created Feb 1   Last used 2d ago

[Create token]
```

Create token modal:
```
Name *             text input
Scopes *           multi-select (same as connector scope list)
Expiry             Optional date picker
[Create]
```
On success: one-time key reveal (same pattern as connector API key reveal §5.2).

---

## 11.3 Org Settings Page

```
Route: /org/settings
```

**Tabs:** General | Security | SCIM | Integrations | Danger Zone

### General tab

```
Organization name *   text input
Slug                  read-only (cannot change after creation — major version bump per BE spec §2.12)
Plan                  read-only
Isolation tier        read-only (shared / schema / shard)
Max users             number input (optional cap)
Max sessions          number input (default 5 per user)
```

### Security tab

```
MFA policy
  ○ Optional — users can enable MFA voluntarily
  ● Required — all users must enroll MFA to log in
  (maps to orgs.mfa_required)

SSO policy
  ○ Optional
  ● Required — all logins must use SSO (OIDC/SAML)
  (maps to orgs.sso_required)

Data visibility
  ● Standard — all authenticated users see full data
  ○ Restricted — sensitive fields (email, IP) require explicit reveal
                  (each reveal is logged as a data.access audit event)
```

### SCIM tab

```
SCIM provisioning
  Status: Enabled

  SCIM endpoint:   https://api.openguard.example.com/v1/scim/v2
  Bearer token:    scim-t1-...  [Copy masked]  [Rotate]

  Rotating the bearer token will require updating your identity provider.
  [Rotate token]  → ConfirmDialog
```

Token rotation calls the admin API to regenerate the SCIM token. The org cannot provision users via SCIM while the token is being rotated.

### Danger Zone tab

```
[Delete organization]

This will permanently delete:
  - All users (N)
  - All connectors (N)
  - All policies (N)
  - All audit events (after ORG_DATA_RETENTION_DAYS)
  - All compliance reports

This action starts the offboarding process (BE spec §2.11).
Data is retained for ORG_DATA_RETENTION_DAYS (2555 days) before permanent deletion.

[Delete this organization]  → ConfirmDialog with requireTyped: org.slug
```

---

## 11.4 Account Lockout Management

When a user's `failed_login_count` reaches the threshold (10 per BE spec §10.3.8):

- User list shows "🔒 Locked" badge.
- User detail shows: "Account locked until [time] due to 10 consecutive failed logins."
- "Unlock account" button → `POST /users/:id/unlock` → `failed_login_count = 0`, `locked_until = null`.
- Toast: "Account unlocked. User can log in again."

**The user receives no different error message at login time** (spec §10.3.8: "Generic Responses" — locked users get `INVALID_CREDENTIALS`, not `ACCOUNT_LOCKED`). This is intentional and the UI should not expose this either — the lock status is only visible to admins.
