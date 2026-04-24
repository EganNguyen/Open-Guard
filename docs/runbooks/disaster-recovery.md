# Runbook: Disaster Recovery

Disaster Recovery plan per Spec §18.

## 1. Targets
- **RTO (Recovery Time Objective)**: 4 hours
- **RPO (Recovery Point Objective)**: 5 minutes

## 2. Backup Strategy
- **PostgreSQL**: Continuous archiving (WAL) + Daily snapshots.
- **MongoDB**: Replica set oplog + Daily snapshots.
- **ClickHouse**: Periodic data export to S3.
- **Redis**: RDB snapshots (not critical, mostly cache/sessions).

## 3. Restore Procedure
1. Provision new infrastructure (Terraform).
2. **Restore PostgreSQL**:
   - Install base backup.
   - Replay WAL files until the point of failure.
3. **Restore MongoDB**:
   - Restore snapshot.
   - Apply oplog.
4. **Deploy Services**:
   - Order: Shared (Redis, Kafka) -> Data (Postgres, Mongo) -> Core (IAM, Policy) -> Frontend.

## 4. Verification
- Run integration tests (`make test`).
- Run load tests (`make load-test`) to ensure performance stability.
