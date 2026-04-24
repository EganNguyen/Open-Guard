# Runbook: Redis Failover

Redis is used for session blocklisting (IAM) and rate limiting.

## 1. Sentinel Failover
1. Sentinels detect master failure.
2. An election is held.
3. A slave is promoted to master.
4. Applications using a Sentinel-aware client (e.g. `go-redis` with `FailoverOptions`) automatically reconnect to the new master.

## 2. Manual Promotion
If Sentinel is not used:
1. Promote slave: `redis-cli SLAVEOF NO ONE`
2. Update application `REDIS_URL`.

## 3. Data Integrity
- Redis uses `appendonly yes` and `appendfsync everysec`.
- Max data loss on failover: ~1 second.
- **Impact**: Some JWTs might not be blocklisted for 1 second, or a rate limit counter might reset.
