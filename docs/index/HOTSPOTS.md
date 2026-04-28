# Hotspots (Risk Map)

## 1. High-Volatility Areas (Touch with Caution)

### `shared/crypto`
- **Why:** Every service relies on this for JWT validation and mTLS. A breaking change here will crash the entire service mesh.
- **Risk:** Modifying the `JWTKey` struct or the mTLS loader logic.

### `infra/docker/docker-compose.yml` (Kafka Init)
- **Why:** The `kafka-init` container creates topics with specific partition counts (e.g., 24 for `audit.trail`). 
- **Risk:** Deleting or misconfiguring this container will cause consumers (Audit/Threat) to crash on startup.

### `scripts/gen-mtls-certs.sh`
- **Why:** mTLS is the backbone. If certificates expire or are generated with incorrect SANs, all service-to-service calls will fail.
- **Risk:** Manual execution is required after installation or when certificates expire.

## 2. Brittle Logic

### Database Seeding (`services/iam/cmd/seed`)
- **Why:** The project relies on specific `org_id` and `user_id` values for integration tests.
- **Risk:** Changing the seed data without updating `tests/integration/` and the Angular dashboard's default login will break the end-to-end flow.

### Angular Signal State (`web/src/app/core/services/auth.service.ts`)
- **Why:** Centralized identity state.
- **Risk:** Introducing side-effects inside `computed()` signals or manual Signal updates from within templates.

## 3. Deployment Hazards

### Postgres Migrations
- **Why:** Managed via `curl` to the IAM service or manual SQL.
- **Risk:** RLS policies (`CREATE POLICY`) are not idempotent in some SQL versions; check existence before creating.
