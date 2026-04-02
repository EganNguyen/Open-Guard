# OpenGuard — Codex Context

## Mandatory Rules (CI-enforced — never violate)
- Every Kafka publish goes through the Outbox relay. No direct producer calls.
- Every org-scoped table has RLS with explicit `org_id UUID NOT NULL`.
- Every inter-service HTTP call wraps a circuit breaker with timeout and fallback.
- Policy engine failure mode: fail closed. Cache grace: 60s. After expiry: deny.
- No string concatenation in SQL. Parameterized queries only.
- No `time.Sleep`. Use `time.NewTicker` inside `select{}` for polling.
- Interfaces defined in the consuming package, never in `shared/`.
- Kafka offsets committed only after successful downstream write.
- SCIM org_id derived from bearer token config, never from client headers.
- bcrypt runs inside a bounded worker pool, never in raw goroutines.

## Architecture in One Paragraph
OpenGuard is a security control plane. Connected apps push events to it; 
user traffic never flows through it. PostgreSQL + RLS for multi-tenancy. 
Kafka via Transactional Outbox for all events. MongoDB for immutable audit 
log with HMAC hash chaining. Redis for caching and JWT blocklist. 
ClickHouse for compliance analytics.

## Key Patterns
- **Outbox**: every write that produces an event uses `outbox.Writer.Write()` 
  inside the same transaction. See `shared/kafka/outbox/writer.go`.
- **RLS**: every service uses `*rls.OrgPool`, never `*pgxpool.Pool` directly.
  RLS policy always uses `NULLIF(current_setting('app.org_id', true), '')::UUID`.
- **Circuit breaker**: use `resilience.Call[T]()` from `shared/resilience/breaker.go`.
- **Config**: use `config.Must()` / `config.MustJSON()`. Never `os.Getenv` 
  in business packages.

## Service Port Map
control-plane:8080, iam:8081, policy:8082, threat:8083, audit:8084,
alerting:8085, compliance:8086, dlp:8087, connector-registry:8090, 
webhook-delivery:8091

## Forbidden Patterns
- `init()` for side effects
- `log.Fatal` / `os.Exit` outside `main.go`
- `time.Sleep` in service code
- String interpolation in SQL
- Direct Kafka publish from business handlers
- `os.Getenv` from business packages
- `any` / `interface{}` as parameter type (except JSON marshal)

## Where to Find Things
- Shared contracts: `shared/models/`
- Kafka topics: `shared/kafka/topics.go`
- Canonical env vars: `.env.example`
- Full spec: `docs/architecture.md`
- Frontend Architecture & Component Specification: `docs/frontend.md`
- Runbooks: `docs/runbooks/`
- Migration patterns: any `services/*/migrations/001_*.up.sql`

## Testing Requirements
- 70% coverage floor per package
- All tests run with `-race`
- Integration tests use `testcontainers-go`
- Fakes over mocks for interfaces < 5 methods