# OpenGuard Todo App — Secure Integration Specimen

> [!NOTE] 
> This is a reference implementation of a multi-tenant web application integrated with the OpenGuard Security Control Plane. It demonstrates best practices for Identity (OIDC), Centralized Authorization (Policy Engine), and Continuous Audit.

## 🏗️ Architecture & Security Lifecycle

The Todo App delegates the "Heavy Lifting" of security to OpenGuard. Every request undergoes a zero-trust verification cycle before any data is touched.

```mermaid
sequenceDiagram
    participant User
    participant App as Todo App
    participant DB as Postgres (RLS)
    participant OG as OpenGuard CP

    User->>App: GET /api/v1/todos (JWT)
    App->>App: Validate JWT Signature
    App->>OG: POST /policy/evaluate (Can User Read?)
    OG-->>App: decision: PERMIT
    App->>DB: SET app.org_id = '...'
    App->>DB: SELECT * FROM todos
    DB-->>App: Filtered Result
    App->>OG: Ingest Audit Event (BATCHED)
    App-->>User: 200 OK (Tasks)
```

### Core Security Patterns
- **Identity Federation**: Authenticates via OpenGuard IAM (OIDC). No passwords are stored in the app.
- **Fail-Closed Policy**: Evaluates permissions via the Policy Engine. If the service is down, it uses a 60s cache before defaulting to `DENY`.
- **Infrastructure-Level Isolation**: Uses Row-Level Security (RLS) in PostgreSQL, tied to the `org_id` claims in the JWT.
- **Async Audit Batching**: High-performance audit event shipping via the OpenGuard SDK to prevent latency overhead.

---

## 🚀 Quick Start

### 1. Requirements
Ensure the core [OpenGuard Services](file:///services/docker-compose.yml) are running:
```bash
cd services && docker-compose up -d
```

### 2. Run with Docker Compose
The Todo App includes a dedicated `docker-compose.yml` that initializes both the application and its private PostgreSQL instance. It is pre-configured to join the `services_default` network.

```bash
cd examples/todoapp

# Start the app and database
docker-compose up -d
```
The Todo App will be accessible at [http://localhost:8083](http://localhost:8083).

---

## ⚙️ Configuration Reference

All settings can be updated via Environment Variables. Default values in the `docker-compose.yml` are optimized for integration with the core OpenGuard stack.

| Variable | Description | Default (Compose) |
| :--- | :--- | :--- |
| `PORT` | Listening port for the HTTP server | `8083` |
| `POSTGRES_URL` | PostgreSQL connection string | `postgres://postgres:postgres@todo-db:5432/todoapp?sslmode=disable` |
| `OPENGUARD_URL` | URL for the OpenGuard Control Plane | `http://openguard-controlplane:8080` |
| `TODO_OPENGUARD_API_KEY` | Connector API Key for Audit/Policy | `${TODO_OPENGUARD_API_KEY}` |
| `OPENGUARD_WEBHOOK_SECRET` | Secret used to verify OpenGuard webhooks | `test-webhook-secret` |
| `OPENGUARD_OIDC_ISSUER` | OpenGuard IAM Issuer Discovery URL | `http://openguard-iam:8081` |
| `OPENGUARD_OIDC_CLIENT_ID` | Client ID registered in OpenGuard IAM | `todo-app` |
| `OPENGUARD_OIDC_CLIENT_SECRET`| Client Secret for OIDC exchange | `todo-app-secret` |
| `FRONTEND_URL` | Public-facing URL for the Todo App UI | `http://localhost:8083` |
| `SDK_EVENT_BATCH_SIZE` | Max audit events to buffer before flush | `100` |
| `SDK_EVENT_FLUSH_INTERVAL_MS`| Max time to wait before flushing audit buffer | `2000` |
| `SDK_POLICY_CACHE_SIZE` | Number of policy decisions to cache | `1000` |
| `SKIP_TLS_VERIFY` | Disable TLS check for internal dev traffic | `false` |


---

## 🛡️ Security Implementation Details

### Row-Level Security (RLS)
The Todo App repository layer enforces multi-tenancy at the database level. Even if a bug exists in the application logic, a user cannot access data from another tenant.
```go
// From pkg/repository/todo.go
return r.db.ExecuteWithRLS(ctx, orgID, func(tx pgx.Tx) error {
    query := `SELECT * FROM todos WHERE user_id = $1`
    // The Postgres RLS policy automatically appends: AND org_id = current_setting('app.org_id')
    return tx.Query(ctx, query, userID)
})
```

### Webhook Handling
The app exposes a signed endpoint at `/webhooks/openguard` to respond to lifecycle events such as:
- **`user.deleted`**: Triggers immediate cleanup of user tokens and sessions.
- **`threat.alert.created`**: Receives security alerts from the Threat Engine to lock compromised accounts.

---

## 🛠️ Local Development

1. **Initialize Database**
   ```bash
   psql $POSTGRES_URL -f examples/todoapp/migrations/schema.sql
   ```

2. **Run Backend (Go)**
   ```bash
   go run examples/todoapp/main.go
   ```

3. **Explore the UI**
   Open [http://localhost:8082](http://localhost:8082) in your browser.
