# Runbook: PostgreSQL Failover

## Context
In the event of a primary database failure, Patroni or the managed cloud provider should handle failover automatically.

## Steps
1. Verify the new primary is accepting writes.
2. Check application logs for `pgxpool` reconnection errors. The connection pool should recover automatically.
3. If services are stuck in a crash loop, perform a rolling restart.
4. Verify the outbox relay has resumed polling and pushing events.
