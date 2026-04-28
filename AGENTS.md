# AGENTS.md

## 🔍 Navigation & Intent
Before starting any task, read the **Index Layer** to understand the system map and architectural intent:
- [**ARCHITECTURE.md**](docs/index/ARCHITECTURE.md): Core design patterns (Outbox, mTLS, RLS).
- [**INDEX.md**](docs/index/INDEX.md): Service registry, ports, and dependencies.
- [**INTENT_MAP.md**](docs/index/INTENT_MAP.md): Architectural decision log (The "Why").
- [**HOTSPOTS.md**](docs/index/HOTSPOTS.md): High-risk areas and brittle logic.

## High-Signal Context
Open-Guard is a high-performance security control plane using a "beside, not in front" architecture.
- **Backend:** Go 1.22+ (using `go.work` workspace). Microservices communicate via **mTLS**.
- **Frontend:** Angular 19+ (Admin Dashboard) and React (Example App).
- **Communication:** Exactly-once audit via **Transactional Outbox** → Kafka → MongoDB/ClickHouse.
- **Security:** "Fail-closed" design. If the control plane is down, SDKs deny access after a 60s TTL.

## Development Workflow
### Critical Commands
- `make certs`: Generates required mTLS certificates for service-to-service communication. **Required for startup.**
- `make dev`: Starts infrastructure (Postgres, Redis, Kafka, MongoDB, ClickHouse) + all Go services + Angular dashboard.
- `make migrate`: Runs PostgreSQL migrations.
- `make test-acceptance`: Runs the full 45-step end-to-end scenario. **Run this before any major PR.**

### Go Backend Conventions
- **Context Handling:** `ctx context.Context` MUST be the first parameter of I/O functions. NEVER use `context.TODO()` in production code. Use `context.Background()` only at startup/entry points.
- **Service Layout:** Each service lives in `services/<name>/`. Code is in `services/<name>/pkg/`.
- **Database (RLS):** PostgreSQL Row-Level Security (RLS) is mandatory. Always call `rls.SetSessionVar` (via `db.WithOrgID`) before queries.
- **Error Handling:** Log at the outermost layer only (HTTP handler or Kafka consumer). Wrap errors at boundaries: `fmt.Errorf("context: %w", err)`.
- **Concurrency:** Every goroutine must have an owner (use `errgroup.WithContext`) and handle `ctx.Done()`.

### Angular Dashboard Conventions
- **Tech Stack:** Angular 19+, Tailwind CSS, Chart.js.
- **State:** Prefer Signals (`signal`, `computed`) over `BehaviorSubject` for component state.
- **API:** Use `ThreatService` for alert data. Charts should use `viewChild<ElementRef<HTMLCanvasElement>>` and initialize in `ngAfterViewInit`.

## Verification Steps
1. **Lint:** `golangci-lint run ./...` (Go) and `npx prettier --check .`
2. **Build:** `go build ./...`
3. **Test:** `go test -race ./...` (Unit) and `make test-acceptance` (System).
4. **SLO:** Verify p99 latency hasn't regressed using `make load-test`.

## Gotchas
- **mTLS:** Services will fail to start or connect if certificates in `certs/` are missing or expired.
- **Kafka Topics:** Run `make create-topics` once if Kafka is fresh; consumers will crash if topics are missing.
- **Fail-Closed SDK:** During development, if you stop the control plane, the SDK/Example App will start denying requests after 60 seconds.
- **Sensitive Logs:** Use `telemetry.SafeAttr` to redact passwords/keys in structured logs.
