# Architecture Map

## 1. Core Philosophy: "Beside, Not In Front"
Open-Guard acts as a security control plane that sits *beside* the application request flow.
- **Fail-Closed SDK:** If the control plane is unreachable, the SDK continues to enforce the last known policy for 60s (TTL), then denies access.
- **Async Auditing:** Requests are not blocked by the audit trail. Audits are written to a **Transactional Outbox** and delivered via Kafka.

## 2. Key Design Patterns

### Transactional Outbox
Used for all high-value events (Auth, Policy Changes, Access).
1. Service writes to local Postgres `outbox` table in the same transaction as the business logic.
2. An **Outbox Relayer** (within each service) polls and publishes to Kafka.
3. Consumers (Audit, Threat, Compliance) ingest from Kafka.

### Service-to-Service Security (mTLS)
- Every service communication is encrypted and authenticated via mTLS.
- Certificates are generated/rotated via `scripts/gen-mtls-certs.sh`.
- Environment variables in `docker-compose.yml` point to `/certs`.

### Multi-Tenancy via RLS
- PostgreSQL **Row-Level Security (RLS)** is mandatory for `org_id` isolation.
- Every query MUST be wrapped in a transaction that sets the `app.current_org_id` session variable.

## 3. Data Strategy
- **Postgres:** Primary source of truth for IAM, Policy, and configuration.
- **Redis:** Real-time policy caching and rate-limiting.
- **MongoDB:** Flexible storage for recent Audit trails and Alerting metadata.
- **ClickHouse:** High-volume compliance reporting and long-term security analytics.
