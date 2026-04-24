# Runbook: PostgreSQL Failover

This runbook describes the procedure for handling a PostgreSQL primary failure.

## 1. Detection
- Grafana alert: `PostgresPrimaryDown`
- Service logs: `connection refused` or `read-only transaction` (if hitting a replica)

## 2. Automatic Failover (Patroni/Stolon)
If using Patroni:
1. Patroni detects leader loss.
2. Replicas initiate an election.
3. New leader is promoted.
4. HAProxy or Kubernetes Service points to the new leader.

## 3. Manual Failover (Emergency)
If automatic failover fails:
1. Identify the most up-to-date replica.
2. Stop the old primary (if still partially alive).
3. Promote the replica:
   ```bash
   touch /tmp/postgresql.trigger
   # OR
   pg_ctl promote -D /var/lib/postgresql/data
   ```
4. Update application `DATABASE_URL` if it doesn't use a generic endpoint.

## 4. Post-Failover
1. Re-provision the old primary as a replica.
2. Verify backup schedule is still running on the new primary.
3. Check replication lag on other replicas.
