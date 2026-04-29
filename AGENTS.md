# AGENTS.md

## 🔍 Navigation & Intent
Before starting any task, read the **Index Layer** to understand the system map and architectural intent:
- [**ARCHITECTURE.md**](docs/index/ARCHITECTURE.md): Core design patterns (Outbox, mTLS, RLS).
- [**INDEX.md**](docs/index/INDEX.md): Service registry, ports, and dependencies.
- [**SYSTEM_MAP.md**](docs/index/SYSTEM_MAP.md): Visual topology of event flows and shared logic.
- [**INTENT_MAP.md**](docs/index/INTENT_MAP.md): Architectural decision log (The "Why").
- [**HOTSPOTS.md**](docs/index/HOTSPOTS.md): High-risk areas and brittle logic.
- [**LEARNING.md**](docs/index/LEARNING.md): Long-term memory and cross-job discoveries.

## 🤖 AI-Native Specifications
Structured phase definitions for automated guidance:
- [.opencode/phase5-detectors.yaml](.opencode/phase5-detectors.yaml)
- [.opencode/phase6-compliance.yaml](.opencode/phase6-compliance.yaml)
- [.opencode/phase7-security.yaml](.opencode/phase7-security.yaml)
- [.opencode/phase10-dlp.yaml](.opencode/phase10-dlp.yaml)

## Git Workflow

- Sync main branch
  ```bash
  git checkout main
  git pull origin main
  ```
- Create branch + worktree
  ```bash
  git worktree add -b <branch_name> <path> main
  ```
- Develop only in worktree directory
- Commit and push only from worktree directory
- Open PR → Review PR → merge into main
- Cleanup after merge
  ```bash
  git worktree remove <path>
  git branch -d <branch_name>
  ```


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

## Verification & Merge Rules
**CRITICAL:** Every Pull Request **MUST** be verified locally by the agent before being merged to `main`. The local verification suite is the authoritative "Gold Standard" and includes checks not present in the remote GitHub Actions pipeline.

### Mandatory Pre-Merge Steps
The following steps must pass in your local environment (or worktree) before you are permitted to merge a PR:

1. **Build All Modules:** `make build`
   - Ensures zero compilation errors across the Go workspace and Angular frontend.
2. **Linting:** `make lint`
   - Verifies Go code style (`golangci-lint`), SQL schemas (`sqlfluff`), and Frontend standards (`npm run lint`).
3. **AI-Audit:** `make ai-check`
   - Enforces architectural discipline (Context usage, RLS compliance, state management).
4. **Unit Tests:** `make test`
   - Runs all Go tests with `-race` detection enabled.
5. **Acceptance Tests:** `make test-acceptance`
   - Executes the 45-step end-to-end functional scenario (requires Docker).
6. **Documentation:** `make visualize` and `make index`
   - Updates the system map and ctags for AI navigation if any structural changes were made.

### Post-Task Loop
After every successful job:
- Use `make remember` to ingest learnings into the Experience Ledger.
- Use `make visualize` to update `SYSTEM_MAP.md` if the topology changed.

## Gotchas
- **mTLS:** Services will fail to start or connect if certificates in `certs/` are missing or expired.
- **Kafka Topics:** Run `make create-topics` once if Kafka is fresh; consumers will crash if topics are missing.
- **Fail-Closed SDK:** During development, if you stop the control plane, the SDK/Example App will start denying requests after 60 seconds.
- **Sensitive Logs:** Use `telemetry.SafeAttr` to redact passwords/keys in structured logs.
