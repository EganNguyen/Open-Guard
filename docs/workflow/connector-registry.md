# Connector Registry — Workflow

## Level 1: High-Level Architecture

```
                             ┌──────────────────────────────────────────────────────────────────────────┐
                             │                          CLIENTS                                         │
                             │                                                                            │
                             │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐       │
                             │  │  Angular UI      │  │  SDK/Connected   │  │  SCIM IdP        │       │
                             │  │  (Admin Dashboard│  │  App             │  │  (external)      │       │
                             │  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘       │
                             │           │                     │                     │                   │
                             │           │  JWT + mTLS        │  X-API-Key           │                   │
                             │           ▼                     ▼                     ▼                   │
                             │  ┌──────────────────────────────────────────────────────────────────────┐│
                             │  │                    CONTROL PLANE (port 8081)                         ││
                             │  │  Proxy: /v1/admin/connectors/* → Connector-Registry:8090           ││
                             │  └──────────────────────────────┬───────────────────────────────────────┘│
                             └─────────────────────────────────┼──────────────────────────────────────────┘
                                                               │
                                                               ▼
                             ┌──────────────────────────────────────────────────────────────────────────┐
                             │                  CONNECTOR REGISTRY (port 8090)                           │
                             │                                                                            │
                             │  ┌──────────────────────────────────────────────────────────────────────┐ │
                             │  │                      MIDDLEWARE STACK                                │ │
                             │  │  RequestID → RealIP → Logger → Recoverer → SecurityHeaders           │ │
                             │  │  → RateLimiter(1000/s, burst 2000) → AuthJWT + Blocklist            │ │
                             │  │  → Idempotency (POST) → Handler                                     │ │
                             │  └──────────────────────────────────────────────────────────────────────┘ │
                             │                                                                            │
                             │  ┌──────────────────────────────────────────────────────────────────────┐ │
                             │  │                     HANDLER LAYER (handler.go)                        │ │
                             │  │                                                                        │ │
                             │  │  POST   /v1/connectors          → Register  (idempotent)             │ │
                             │  │  POST   /v1/connectors/validate  → ValidateAPIKey                    │ │
                             │  │  GET    /health                  → Health                            │ │
                             │  └──────────────────────────────────────────────────────────────────────┘ │
                             │                                                                            │
                             │  ┌──────────────────────────────────────────────────────────────────────┐ │
                             │  │                     SERVICE LAYER (service.go)                        │ │
                             │  │                                                                        │ │
                             │  │  ┌──────────────────────────────────────────────────────────────┐    │ │
                             │  │  │  REGISTER CONNECTOR                                           │    │ │
                             │  │  │  1. Generate API key (ogk_ + 24 random bytes base64)         │    │ │
                             │  │  │  2. Prefix = key[:12] for fast lookup                        │    │ │
                             │  │  │  3. Hash key with PBKDF2                                     │    │ │
                             │  │  │  4. Generate OAuth2 client secret (32 chars)                  │    │ │
                             │  │  │  5. repo.CreateConnector → PostgreSQL                        │    │ │
                             │  │  │  6. Return plaintext API key (shown once, never stored)       │    │ │
                             │  │  └──────────────────────────────────────────────────────────────┘    │ │
                             │  │                                                                        │ │
                             │  │  ┌──────────────────────────────────────────────────────────────┐    │ │
                             │  │  │  VALIDATE API KEY                                            │    │ │
                             │  │  │  1. Parse prefix from apiKey[:12]                           │    │ │
                             │  │  │  2. Try Redis cache:                                        │    │ │
                             │  │  │     ├── Hit → verify PBKDF2, check status, return connector │    │ │
                             │  │  │     └── Miss → DB lookup by prefix (indexed)               │    │ │
                             │  │  │  3. Full PBKDF2 verify                                      │    │ │
                             │  │  │  4. Check status != "suspended"                             │    │ │
                             │  │  │  5. Cache result in Redis (5 min TTL)                       │    │ │
                             │  │  │  6. Return connector metadata                               │    │ │
                             │  │  └──────────────────────────────────────────────────────────────┘    │ │
                             │  │                                                                        │ │
                             │  │  ┌──────────────────────────────────────────────────────────────┐    │ │
                             │  │  │  SUSPEND / DELETE                                           │    │ │
                             │  │  │  1. Get connector to find prefix                            │    │ │
                             │  │  │  2. Delete Redis cache keys                                 │    │ │
                             │  │  │     (apikey:hash:{prefix}, apikey:data:{prefix})            │    │ │
                             │  │  │  3. Update status / delete row in PostgreSQL                │    │ │
                             │  │  └──────────────────────────────────────────────────────────────┘    │ │
                             │  └──────────────────────────────────────────────────────────────────────┘ │
                             │                                                                            │
                             │  ┌──────────────────────────────────────────────────────────────────────┐ │
                             │  │                    REPOSITORY LAYER (repository.go)                   │ │
                             │  │                                                                        │ │
                             │  │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐   │ │
                             │  │  │ CreateConnector  │  │ FindByPrefix     │  │ GetConnectorByID │   │ │
                             │  │  │ - INSERT INTO    │  │ - SELECT by      │  │ - SELECT by id   │   │ │
                             │  │  │   connectors     │  │   api_key_prefix │  │                   │   │ │
                             │  │  │   (RLS enforced) │  │   (indexed)      │  │                   │   │ │
                             │  │  └──────────────────┘  └──────────────────┘  └──────────────────┘   │ │
                             │  │  ┌──────────────────┐  ┌──────────────────┐                           │ │
                             │  │  │ DeleteConnector  │  │ UpdateStatus     │                           │ │
                             │  │  │ - DELETE WHERE   │  │ - UPDATE status  │                           │ │
                             │  │  │   id = $1        │  │   WHERE id = $1  │                           │ │
                             │  │  └──────────────────┘  └──────────────────┘                           │ │
                             │  └──────────────────────────────────────────────────────────────────────┘ │
                             └──────────────────────────────────────────────────────────────────────────┘
                                                               │
                                      ┌────────────────────────┼────────────────────────┐
                                      ▼                        ▼                        ▼
                             ┌────────────────┐       ┌──────────────────┐      ┌──────────────────┐
                             │  PostgreSQL    │       │  Redis (db 2)   │      │  Kafka           │
                             │  connectors    │       │  apikey:hash:*  │      │  connector.events│
                             │  (RLS)         │       │  apikey:data:*  │      │  webhook.delivery│
                             └────────────────┘       └──────────────────┘      └──────────────────┘
```

---

## Level 2: API Key Validation Flow

```
  Connected App         Connector Registry          Redis Cache            PostgreSQL
       │                        │                       │                      │
       │  POST /v1/connectors/validate                  │                      │
       │  X-API-Key: ogk_abc12345_secret                │                      │
       │──────────────────────>│                       │                      │
       │                       │  Parse prefix = key[:12]                   │
       │                       │                       │                      │
       │                       │  GET apikey:hash:ogk_abc12345              │
       │                       │──────────────────────>│                      │
       │                       │                       │                      │
       │                       │  ┌─ Cache HIT ─────── │                      │
       │                       │  │  Hash found        │                      │
       │                       │  │  Verify PBKDF2     │                      │
       │                       │  │    │                │                      │
       │                       │  │  GET apikey:data:ogk_abc12345            │
       │                       │  │──────────────────────>│                  │
       │                       │  │  │  Connector JSON  │                      │
       │                       │  │  │  Check status    │                      │
       │                       │  │  │                  │                      │
       │                       │  │  └── status=active → RETURN connector    │
       │                       │  │  └── status=suspended → RETURN 401       │
       │                       │                       │                      │
       │                       │  ┌─ Cache MISS ────── │                      │
       │                       │  │  No hash in Redis  │                      │
       │                       │  │                     │                      │
       │                       │  │  SELECT by api_key_prefix                │
       │                       │  │─────────────────────────────────────────>│
       │                       │  │                     │                      │
       │                       │  │  Connector row      │                      │
       │                       │  │<──────────────────────────────────────────│
       │                       │  │                     │                      │
       │                       │  │  Verify PBKDF2     │                      │
       │                       │  │  Check status      │                      │
       │                       │  │                     │                      │
       │                       │  │  SET apikey:hash:{prefix} (TTL 5min)    │
       │                       │  │──────────────────────>│                  │
       │                       │  │  SET apikey:data:{prefix} (TTL 5min)    │
       │                       │  │──────────────────────>│                  │
       │                       │  │                     │                      │
       │                       │  RETURN 200 + connector metadata            │
       │<──────────────────────│                       │                      │
```

### Registration Flow

```
  Admin UI                   Connector Registry         PostgreSQL              Response
       │                        │                       │                       │
       │  POST /v1/connectors   │                       │                       │
       │  { id, org_id, name,  │                       │                       │
       │    redirect_uris }     │                       │                       │
       │  (idempotency key)     │                       │                       │
       │──────────────────────>│                       │                       │
       │                        │                       │                       │
       │                        │  1. Generate API key:                        │
       │                        │     "ogk_" + 24 bytes random (base64)        │
       │                        │     → prefix = key[:12]                     │
       │                        │                       │                       │
       │                        │  2. Hash key with PBKDF2                    │
       │                        │                       │                       │
       │                        │  3. Generate client_secret (32 chars random)│
       │                        │                       │                       │
       │                        │  4. INSERT INTO connectors                   │
       │                        │     (id, org_id, name, client_secret,        │
       │                        │      redirect_uris, api_key_prefix,          │
       │                        │      api_key_hash)                           │
       │                        │──────────────────────>│                       │
       │                        │   INSERT OK           │                       │
       │                        │<──────────────────────│                       │
       │                        │                       │                       │
       │  201 Created           │                       │                       │
       │  { id, api_key }      │                       │                       │
       │  (API KEY SHOWN ONCE) │                       │                       │
       │<──────────────────────│                       │                       │
```

### Suspension / Deletion Flow

```
  Admin UI                   Connector Registry         Redis                  PostgreSQL
       │                        │                       │                       │
       │  SUSPEND connector      │                       │                       │
       │──────────────────────>│                       │                       │
       │                        │  GetConnectorByID(id) │                       │
       │                        │──────────────────────────────────────────────>│
       │                        │  { api_key_prefix }   │                       │
       │                        │<──────────────────────────────────────────────│
       │                        │                       │                       │
       │                        │  DEL apikey:hash:{prefix}                    │
       │                        │──────────────────────>│                       │
       │                        │  DEL apikey:data:{prefix}                    │
       │                        │──────────────────────>│                       │
       │                        │                       │                       │
       │                        │  UPDATE status='suspended'                   │
       │                        │──────────────────────────────────────────────>│
       │                        │                       │                       │
       │  200 OK                │                       │                       │
       │<──────────────────────│                       │                       │
       │                        │                       │                       │
  (Subsequent validation requests will miss cache, hit DB, and see "suspended")
```

---

## Level 3: Internals

### Connector Lifecycle State Machine

```
                        ┌──────────┐
                        │ PENDING  │  (initial creation, before first use)
                        └────┬─────┘
                             │
                        ┌────▼─────┐
                        │  ACTIVE  │  (can authenticate)
                        └────┬─────┘
                             │
                    ┌────────┴────────┐
                    │                 │
              ┌─────▼─────┐    ┌─────▼─────┐
              │ SUSPENDED │    │  DELETED  │
              │ (reversible)   │ (terminal)│
              └─────┬─────┘    └───────────┘
                    │
              ┌─────▼─────┐
              │  ACTIVE   │  (reactivated)
              └───────────┘
```

### Redis Cache Keys

| Key Pattern | Value | TTL | Purpose | Invalidation Trigger |
|-------------|-------|-----|---------|---------------------|
| `apikey:hash:{prefix}` | PBKDF2 hash string | 5 min | Skip DB on cache hit | Suspend / Delete |
| `apikey:data:{prefix}` | JSON connector object | 5 min | Full connector metadata | Suspend / Delete |

### Database Schema

```sql
CREATE TABLE connectors (
    id              TEXT PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES orgs (id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    client_secret   TEXT NOT NULL,
    redirect_uris   TEXT[] NOT NULL,
    api_key_prefix  TEXT UNIQUE,            -- ogk_ + 8 chars
    api_key_hash    TEXT UNIQUE,            -- PBKDF2 hash
    status          TEXT NOT NULL DEFAULT 'active',  -- active | suspended
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connectors_org_id ON connectors (org_id);
CREATE INDEX idx_connectors_api_key_prefix ON connectors (api_key_prefix);

ALTER TABLE connectors ENABLE ROW LEVEL SECURITY;
ALTER TABLE connectors FORCE ROW LEVEL SECURITY;

CREATE POLICY connectors_org_isolation ON connectors
USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);
```

### API Key Format

```
ogk_<base64(24 random bytes)>
│     └──── 32 chars ──────┘
└─ prefix[:12]
```

- Prefix (first 12 chars: `ogk_` + 8 chars) is stored and indexed — used for fast lookup
- Full key is hashed with PBKDF2 — **never stored in plaintext**
- Plaintext API key is returned **once** at creation (one-time reveal)

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Admin UI (Angular)** | Connector registration wizard, API key reveal screen, suspension, delivery log |
| **Connector Registry** | API key generation, PBKDF2 hashing, Redis caching, CRUD operations |
| **Redis** | Hot-path cache: fast hash prefix lookup avoids PBKDF2 on every request |
| **PostgreSQL (RLS)** | Persistent storage with RLS isolation per org |
| **Kafka** | Designed: `connector.events` for lifecycle events, `webhook.delivery` for outbound webhooks |

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **Redis down** | Cache | ValidateAPIKey falls back to full DB lookup + PBKDF2 verify (slower, still works) |
| **PostgreSQL down** | DB | Cannot register/validate/suspend — service returns 500 |
| **PBKDF2 verify failure** | Service | Returns 401 Unauthorized |
| **CSPRNG failure** | Service | Falls back to `crypto.GenerateRandomString` for API key generation |
| **Duplicate registration** | Idempotency | Idempotency middleware prevents double-creation |
| **Suspended connector** | Validation | Cache invalidation → next request fetches from DB, sees `suspended`, returns 401 |

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_connector_registry_requests_total` | Counter | `operation`, `status` | Connector Registry |
| `openguard_connector_validation_cache_hits_total` | Counter | — | Service layer |

### Audit Events

| Event | When | Payload |
|-------|------|---------|
| `connector.created` | Connector registered | connector_id, org_id, scopes |
| `connector.suspended` | Connector suspended | connector_id, org_id |
| `connector.deleted` | Connector deleted | connector_id, org_id |
