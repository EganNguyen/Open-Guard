# §18.5–18.6 — Disaster Recovery & Multi-Region Topology

---

## 18.5 Disaster Recovery Plan

### RTO/RPO Targets

| Component | RPO (Max Data Loss) | RTO (Max Recovery Time) |
|---|---|---|
| PostgreSQL (IAM, Policy, Connectors) | 5 minutes | 30 minutes |
| MongoDB (Audit Log) | 1 hour | 2 hours |
| ClickHouse (Compliance Analytics) | 24 hours | 4 hours |
| Redis (Cache, Rate Limits, Blocklist) | 0 (in-memory, ephemeral) | 5 minutes (failover to replica) |
| Kafka (Event Bus) | 0 (replicated log) | 15 minutes |

### PostgreSQL Recovery

1. Aurora PostgreSQL PITR enabled. WAL streamed to S3 continuously.
2. To restore: `aws rds restore-db-cluster-to-point-in-time` targeting the desired timestamp.
3. Validate: run the read-only smoke test suite against the restored cluster before promoting.
4. Automated: daily snapshot triggered by backup job. Restore tested in staging weekly via CI pipeline.

### MongoDB Recovery

1. Atlas continuous backup enabled with 1-hour snapshot interval.
2. Oplog tailing allows PITR at 1-minute granularity within the retention window.
3. Hash chain integrity verified post-restore using `scripts/verify-audit-chain.sh`.

### Chaos Drill & DR Test Schedule

- **Quarterly Redis Failover Test:** Kill the Redis primary. Verify Sentinel promotes a replica within 30 seconds. Verify auth blocklist remains functional. Verify rate limiting fails open.
- **Monthly PostgreSQL Restore Test:** Restore previous night's snapshot to staging. Run the full acceptance criteria suite (§20).
- **Bi-annual Full DR Drill:** Simulate region failure. Verify promotion of standby region succeeds within the 30-minute RTO target.

---

## 18.6 Multi-Region Topology

> **Active-active multi-region write is NOT supported in v1.** All writes go to the primary region. The standby region is a hot standby for failover only. Read traffic may be served from the standby region for ClickHouse and MongoDB read queries.

### 18.6.1 Kafka — MirrorMaker 2

```yaml
# infra/kafka/mirrormaker2.yaml
clusters:
  - alias: primary
    bootstrap.servers: kafka-primary.internal:9092
    sasl.mechanism: SCRAM-SHA-512
    security.protocol: SASL_SSL
  - alias: standby
    bootstrap.servers: kafka-standby.internal:9092
    sasl.mechanism: SCRAM-SHA-512
    security.protocol: SASL_SSL

mirrors:
  - source.cluster.alias: primary
    target.cluster.alias: standby
    topics:
      - audit.trail
      - threat.alerts
      - connector.events
      - data.access
      - auth.events
      - policy.changes
      - outbox.dlq
      - webhook.dlq
    # Do NOT replicate saga.orchestration — sagas only run in the primary region.
    topics.blacklist: saga.orchestration,webhook.delivery,notifications.outbound
    replication.factor: 3
    sync.topic.configs.enabled: true
    sync.topic.acls.enabled: true
    emit.checkpoints.enabled: true
    emit.checkpoints.interval.seconds: 60
```

**Replication lag alert:**
```yaml
  - alert: MirrorMakerLagHigh
    expr: kafka_mirror_maker_record_age_ms_max > 30000
    for: 5m
    labels: { severity: warning }
    annotations:
      summary: "MirrorMaker 2 replication lag > 30s — standby region is falling behind"
      runbook: "docs/runbooks/kafka-consumer-lag.md"
```

**Failover procedure:**
1. Update DNS CNAME for Kafka bootstrap to point to the standby cluster.
2. MM2 consumer offset checkpoints allow consumers to resume from the last replicated position.
3. Remaining in-flight messages in the primary cluster (RPO window) will be reprocessed from the outbox when PostgreSQL is promoted.

**Topic naming:** MM2 prefixes replicated topics with the source alias (`primary.audit.trail`). Consumers in the standby region must be configured to consume from prefixed topic names during failover.

### 18.6.2 PostgreSQL — Streaming Replication

```
Primary Region: Aurora PostgreSQL (writer + 2 readers, Multi-AZ)
                    │
                    │ async streaming replication (WAL shipping)
                    ▼
Standby Region: Aurora PostgreSQL Read Replica (promoted to writer during failover)
```

**Failover procedure:**
1. Detect primary region failure (health check fails for > 60s).
2. Promote standby Aurora cluster: `aws rds promote-read-replica-db-cluster --db-cluster-identifier standby-cluster`.
3. Update application connection strings to the new primary.
4. Outbox relay in the standby region begins draining the outbox (replicated via WAL).
5. Any outbox records written after the last replicated WAL position are permanently lost (within RPO of ~5 minutes).

**RLS in the standby region:** RLS policies are replicated via WAL — identical on the standby. No schema changes required for failover.

**Connection routing:** Use `pgbouncer` with a `host` override file pointing to the current writer. During failover: update pgbouncer config and reload (`pgbouncer -R`).

**Replication lag alert:**
```yaml
  - alert: PostgreSQLReplicationLagHigh
    expr: aws_rds_replica_lag_average > 30
    for: 5m
    labels: { severity: warning }
    annotations:
      summary: "PostgreSQL standby replica lag > 30s — RPO window expanding"
```

### 18.6.3 MongoDB — Atlas Global Clusters

**Topology:** MongoDB Atlas Global Clusters with zone-based sharding. Zone A = primary region (all writes). Zone B = standby region (replication + reads).

**Zone mapping:**
```js
db.adminCommand({
  customAction: "createOrUpdateGeoZoneMapping",
  zoneMapping: [
    { location: "us-east-1", zone: "Zone A" },
    { location: "eu-west-1", zone: "Zone B" }
  ]
})
```

**Connection strings:**
```
MONGO_URI_PRIMARY=mongodb+srv://cluster.mongodb.net/?readPreference=primary&appName=openguard-write
MONGO_URI_SECONDARY=mongodb+srv://cluster.mongodb.net/?readPreference=secondaryPreferred&appName=openguard-read
```

**Failover:** Atlas handles automatic failover. If Zone A loses quorum, Atlas promotes a secondary in Zone B to primary within ~30 seconds. The application connection string does not change (Atlas handles DNS failover via the `+srv` SRV record).

**Audit chain note:** After failover to Zone B, any events written to Zone A after the last replication checkpoint are not in Zone B, creating a `chain_seq` gap. The integrity verifier will report the exact gap per org.

### 18.6.4 Redis — Sentinel HA with Cross-Region Warm Standby

**Primary region:** Redis Sentinel (3 nodes, Multi-AZ) — authoritative.

**Standby region:** Empty Redis instance. Services start with a cold cache after failover.

**Blocklist recovery after failover:** Immediately after promoting the standby PostgreSQL cluster, run `scripts/rebuild-jti-blocklist.sh`. This queries the `sessions` table for all `revoked=true` entries with unexpired JWTs and re-populates Redis.

```bash
# scripts/rebuild-jti-blocklist.sh
# Run immediately after standby region promotion.
# Queries sessions table for revoked sessions with non-expired JWTs
# and populates Redis jti:blocklist:{jti} keys with remaining TTL.
# Run time: < 60 seconds for typical deployments (< 100k active sessions).
```

### 18.6.5 Multi-Region Acceptance Criteria

- [ ] MM2 replication lag < 30s under 50,000 events/s sustained load.
- [ ] PostgreSQL standby replication lag < 30s under normal write load.
- [ ] MongoDB Zone B election completes within 30s of Zone A primary failure (Atlas SLA).
- [ ] After PostgreSQL failover: outbox relay resumes draining within 60s.
- [ ] After MongoDB failover: `verify-audit-chain.sh` reports gap at expected `chain_seq` (not silent corruption).
- [ ] After Redis failover: `rebuild-jti-blocklist.sh` completes in < 60s. Revoked tokens correctly blocked.
- [ ] RTO for full region failover: < 30 minutes.
- [ ] `MirrorMakerLagHigh` alert fires correctly during simulated MM2 shutdown.
- [ ] Standby region read traffic (audit queries) serves correctly from MongoDB Zone B secondary.
