# Todo App — OpenGuard Integration Spec

## 1. What the app does

A multi-tenant todo app: users belong to orgs, create/edit/delete todos, and assign them to teammates. OpenGuard handles all security — auth, authorization, audit, and threat detection. The app never touches passwords or issues tokens directly.

---

## 2. Registration

Register the todo app as a connector in OpenGuard:

```bash
POST /v1/admin/connectors
{
  "name": "todo-app",
  "webhook_url": "https://todo.example.com/webhooks/openguard",
  "scopes": ["events:write", "policy:evaluate", "audit:read"]
}
# Response: { "connector_id": "...", "api_key_plaintext": "abcdefgh..." }
# Store api_key_plaintext as TODO_OPENGUARD_API_KEY in env. Never stored again.
```

---

## 3. Authentication flow

The app delegates login entirely to OpenGuard IAM via OIDC.

```
User → GET /oauth/authorize   (OpenGuard IAM)
     ← redirect with auth code
App  → POST /oauth/token      (exchange code + PKCE verifier)
     ← { access_token (JWT), refresh_token }
App  → validate JWT signature via GET /oauth/jwks
App  → on every request: check jti blocklist in Redis (fail closed on Redis error)
```

**SCIM provisioning** (if org uses Okta/Azure AD): configure `IAM_SCIM_TOKENS_JSON` with a per-org token. OpenGuard handles `POST /scim/v2/Users` and the provisioning saga. The todo app receives a `saga.completed` webhook when a new user is ready.

---

## 4. Authorization

The SDK is embedded in the Go backend. Every mutating endpoint calls `policy/evaluate` before touching the database.

**Roles:**
| Role | Permissions |
|---|---|
| `member` | Create/read own todos; read shared todos |
| `editor` | Create/read/update any todo in org |
| `admin` | All of the above + manage members |

**SDK call pattern (every protected handler):**

```go
decision, err := sdk.PolicyEvaluate(ctx, policy.Request{
    UserID:   claims.Sub,
    OrgID:    claims.OrgID,
    Action:   "todo:delete",
    Resource: fmt.Sprintf("org:%s/todo:%s", orgID, todoID),
})
if err != nil || !decision.Permitted {
    http.Error(w, "forbidden", http.StatusForbidden)
    return
}
```

**Fail-closed behavior:** If the policy service is unreachable, the SDK uses its local LRU cache for up to 60 seconds, then denies all requests. The todo app does not implement a fallback — denial is the correct behavior.

---

## 5. Audit events

All state-changing operations push audit events via `POST /v1/events/ingest` using the connector API key. Events are asynchronous (SDK buffers and batches).

| User action | Event type |
|---|---|
| Create todo | `todo.created` |
| Update todo | `todo.updated` |
| Delete todo | `todo.deleted` |
| Assign todo to user | `todo.assigned` |
| Mark complete | `todo.completed` |
| Login | emitted by OpenGuard IAM automatically |
| Logout | emitted by OpenGuard IAM automatically |

**Event payload shape:**

```json
{
  "id": "<uuidv4>",
  "type": "todo.created",
  "org_id": "<org_id>",
  "actor_id": "<user_id>",
  "actor_type": "user",
  "occurred_at": "2026-04-02T10:00:00Z",
  "source": "todo-app",
  "event_source": "connector:<connector_id>",
  "payload": {
    "todo_id": "...",
    "title": "Write spec",
    "assignee_id": "..."
  }
}
```

DLP note: the `title` field is scanned async by OpenGuard's DLP engine. If an org has `dlp_mode=block`, the ingest call may return `422 DLP_POLICY_VIOLATION` — the todo app must surface this as a user-facing validation error.

---

## 6. Webhook receiver

OpenGuard delivers signed webhooks to `POST /webhooks/openguard`. The todo app must verify the signature on every delivery.

```go
// Verify HMAC-SHA256 signature
sig := r.Header.Get("X-OpenGuard-Signature")         // "sha256=<hex>"
ts  := r.Header.Get("X-OpenGuard-Timestamp")         // unix seconds
id  := r.Header.Get("X-OpenGuard-Delivery")          // idempotency UUID

// Reject if |now - ts| > 300s (replay protection)
// Compute expected = HMAC-SHA256(secret, ts+"."+body)
// Constant-time compare sig vs expected
```

**Events the todo app handles:**

| Event type | Todo app action |
|---|---|
| `saga.completed` | Activate new SCIM-provisioned user account |
| `threat.alert.created` (HIGH/CRITICAL) | Email org admin; optionally lock affected user's todos |
| `user.deleted` | Soft-delete or reassign that user's todos |
| `user.suspended` | Block the user from opening the app (JWT check will already deny, but clean up UI state) |

Unknown event types must return `200 OK` and be ignored — OpenGuard will add new event types over time.

---

## 7. Environment variables

```dotenv
# OpenGuard
OPENGUARD_URL=https://api.openguard.example.com
TODO_OPENGUARD_API_KEY=abcdefgh...           # connector key (prefix + secret)
OPENGUARD_WEBHOOK_SECRET=change-me-32-bytes  # for HMAC verification
OPENGUARD_OIDC_ISSUER=https://accounts.example.com
OPENGUARD_OIDC_CLIENT_ID=todo-app
OPENGUARD_OIDC_CLIENT_SECRET=change-me

# SDK tuning
SDK_POLICY_CACHE_TTL_SECONDS=60
SDK_POLICY_EVALUATE_TIMEOUT_MS=100
SDK_EVENT_BATCH_SIZE=100
SDK_EVENT_FLUSH_INTERVAL_MS=2000
SDK_OFFLINE_RETRY_LIMIT=500

# Todo app database
POSTGRES_URL=postgres://...
```

---

## 8. What the todo app does NOT implement

- Password storage or hashing
- Session management or refresh token rotation
- Brute-force detection
- Audit log storage
- SCIM endpoint handling
- MFA flows
- Rate limiting (handled by OpenGuard's per-org quota)

All of these are owned by OpenGuard. The todo app calls the SDK and responds to webhooks.