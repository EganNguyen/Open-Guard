A comprehensive deployment guide with visual diagrams for each growth tier, then detailed prose for each stage.**Tier 0 — Minimum production release (0–1,000 users).** Everything on one VM. This is a real, hardened deployment — not a prototype. All the security guarantees from the spec hold (RLS, mTLS between processes via localhost loopback, bcrypt pool, outbox pattern). What you surrender is redundancy: one disk failure ends you. That's acceptable at this scale if you have daily S3 backups and a 4-hour RTO in your SLA.

The key compromises for this tier: Kafka runs single-broker with `replication=1` and topic partitions cut to 3 (down from 12–24 in the full spec); MongoDB runs as a 1-node pseudo-replica set (required by the Go driver's replica set connection string, but with no actual replication); Redis runs without Sentinel, which means the auth JTI blocklist fails open on Redis crash — document this limitation explicitly for your operators.

---**Tier 1 — Early growth (1,000–10,000 users).** The critical split here is separating app processes from data stores onto two dedicated nodes. This gives you independent vertical scaling paths (more CPU for apps, more RAM for databases) and lets you add a read replica to PostgreSQL without touching the app tier. Redis gets Sentinel at this point — this is the threshold where the auth JTI blocklist must be HA, because session revocations from a SCIM `DELETE` or an admin lockout silently fail on Redis downtime. The MongoDB "1→3 node" annotation means you add the two secondaries as soon as your budget allows, not at the next tier boundary.

---**Tier 2 — Scale-out (10,000–100,000 users).** This is the containerization threshold. If you haven't moved to Kubernetes or Nomad yet, do it now. The reason is horizontal pod autoscaling on two signals: CPU for the auth and control-plane pods, and `openguard_kafka_consumer_lag` for the worker pods (audit, threat, alerting). Without HPA on consumer lag, your audit log falls 30 seconds behind under bursty ingest — which violates the 2s p99 SLO. Kafka now runs 3 brokers with `replication=3`, restoring the full partition counts from the spec. PostgreSQL gets a streaming replica carrying all read traffic (audit event queries, compliance reporting).

---

Now let me give you the full operational playbook for each tier, including the concrete steps to move between them.

---

## Tier 0 — Minimum production (0–1,000 users)

**Target hardware:** One dedicated VM or bare-metal node. Minimum 8 vCPU, 32 GB RAM, 200 GB NVMe SSD. A $120–180/month Hetzner AX41, DigitalOcean CPU-optimised, or Vultr HF instance is appropriate.

**What you deploy:** All 11 services as Docker Compose containers on one host, plus Nginx terminating TLS with Let's Encrypt. The Outbox relay, Kafka, PostgreSQL, MongoDB, and Redis all live on the same machine.

**Critical configuration deltas from the full spec:**

*Kafka:* Set `KAFKA_CFG_NUM_PARTITIONS=3` and `KAFKA_CFG_DEFAULT_REPLICATION_FACTOR=1` in the Compose file. The `create-topics.sh` script from the spec detects single-broker mode and sets replication to 1 automatically. Reduce `AUDIT_BULK_INSERT_MAX_DOCS` from 500 to 100 to lower MongoDB write pressure on a shared disk.

*MongoDB:* Run a single `mongod` with `--replSet rs0` but only one member in `rs.initiate()`. The Go driver requires a replica set connection string for change streams and write concern; this satisfies it. Add `priority:0, votes:0` placeholder members only when you add real nodes in Tier 1.

*Redis:* Single node, no Sentinel. This means `redis_blocklist_fails_open = true` during Redis downtime. Mitigate by setting Redis `maxmemory-policy allkeys-lru` and monitoring `used_memory` — the blocklist entries have TTLs so they evict naturally. Document this limitation in your runbook.

*IAM bcrypt pool:* Set `IAM_BCRYPT_WORKER_COUNT=4` on an 8-core machine (half of NumCPU). This gives ~11 logins/sec sustained on one node. Adequate for 1,000 users.

*Services to skip at Tier 0:* DLP service (resource-intensive, low value at small scale) and the full ClickHouse compliance stack. Use the stub handler that returns a `503 COMPLIANCE_UNAVAILABLE` for report generation. Add these at Tier 1.

**Backup strategy (non-negotiable at Tier 0):**

```bash
# /etc/cron.d/openguard-backup
0 3 * * * root pg_dump -U openguard_app openguard | gzip | \
  aws s3 cp - s3://your-bucket/pg/$(date +%Y%m%d).sql.gz

30 3 * * * root mongodump --uri="mongodb://localhost:27017" \
  --archive | aws s3 cp - s3://your-bucket/mongo/$(date +%Y%m%d).archive
```

Test the restore. Run it monthly. A backup you haven't restored is not a backup.

**SLO adjustments:** At Tier 0, accept `POST /oauth/token` p99 < 500ms (not 150ms) and event ingest at 2,000 req/s maximum. These are not architectural concessions — they reflect single-node resource contention. The sub-100ms policy evaluation SLO is achievable because policy eval is CPU-light and Redis-cached.

**Monitoring minimum:** Run Prometheus and Grafana in the same Compose stack. The four dashboards you need from day one are: outbox pending record count, Kafka consumer lag, PostgreSQL connection pool utilisation, and Redis memory usage. If `openguard_outbox_pending_records` stays above 500 for more than 5 minutes, your single Kafka broker or relay is stuck — this is your primary operational alert at this tier.

---

## Migration: Tier 0 → Tier 1

The trigger for this migration is any of: outbox pending records consistently above 200, login p99 approaching 400ms, or MongoDB disk I/O contending with PostgreSQL writes (visible as `iowait` above 30% on the VM).

**Step 1:** Provision a second VM (the dedicated data node). Minimum 8 vCPU, 32 GB RAM. Move PostgreSQL, MongoDB, and Redis to it with zero downtime:

```bash
# On old node: take a snapshot, rsync the PostgreSQL data dir
pg_basebackup -h localhost -U openguard_migrate -D /tmp/pg-base -P
rsync -av /tmp/pg-base/ data-node:/var/lib/postgresql/16/main/
```

**Step 2:** Add two MongoDB secondaries by adding them to the replica set while the primary is running. This is a live operation:

```js
rs.add("mongo-secondary-1:27017")
rs.add("mongo-secondary-2:27017")
```

**Step 3:** Enable Redis Sentinel on the data node with two sentinel processes pointing at the Redis primary. Update all service configurations to use the Sentinel connection string rather than a direct Redis address. The IAM service's `jti` blocklist failover is now HA.

**Step 4:** Update all service `POSTGRES_HOST`, `MONGO_URI_PRIMARY`, `MONGO_URI_SECONDARY`, and `REDIS_ADDR` environment variables to point at the data node. Rolling restart services on the app node with a 30-second wait between each.

**Step 5:** Switch the Kafka broker to a separate node or managed service (Confluent Cloud, Aiven, RedpandaCloud). At 1,000–10,000 users, a managed Kafka tier is cheaper than operating Zookeeper and three broker VMs. Migrate topics by using `kafka-mirror-maker` to replicate while both clusters are live, then switch producer/consumer configs.

---

## Migration: Tier 1 → Tier 2

The triggers here are specific: IAM login error rate above 1% during peak hours (bcrypt pool exhausted), or Kafka consumer lag on `audit.trail` consistently above 10,000 (audit service single pod can't keep up).

**Containerise first.** Move from Docker Compose to Kubernetes (or Nomad). The spec's Helm chart in `infra/k8s/helm/openguard/` is the target. The minimum cluster to start with is 3 worker nodes at 4 vCPU / 8 GB each — total 12 vCPU for services, which is comparable to the Tier 1 app node but adds the critical ability to scale pods independently.

**Add HPA immediately.** The Helm chart spec defines autoscaling for `control-plane`, `iam`, `policy`, and `audit`. The two signals are:

```yaml
# For control-plane, iam, policy: CPU-based
- type: Resource
  resource:
    name: cpu
    target:
      type: Utilization
      averageUtilization: 70

# For audit (and other Kafka consumers): lag-based
- type: External
  external:
    metric:
      name: openguard_kafka_consumer_lag
      selector:
        matchLabels:
          topic: audit.trail
    target:
      type: AverageValue
      averageValue: "5000"
```

**Add the PostgreSQL read replica.** Direct all `GET /audit/events` query traffic to the replica by configuring `MONGO_URI_SECONDARY` in the audit read service. This alone typically cuts primary PostgreSQL load by 40–60% for event-heavy organisations.

**Restore full Kafka partition counts.** Now that you have 3 brokers, run `kafka-topics.sh --alter --partitions 12` for `auth.events` and `audit.trail`. Note that increasing partitions on an existing topic does not rebalance existing keys — the first time after the resize, some consumer group rebalancing will occur. Schedule this during low-traffic hours.

---

## Tier 3 — Enterprise scale (100,000+ users, multi-region)**Tier 3 — Enterprise (100,000+ users, multi-region).** The standby region runs at minimum replica counts (`min=1`) to keep warm — not to serve traffic. Failover is a controlled promotion: update the DNS record, scale up service pods in the standby region via an operator runbook, and promote the PostgreSQL replica to primary. The DR drill from spec Section 18.5 should rehearse this exact sequence quarterly.

At this tier you also implement the spec's **tenant isolation tiers** (Section 2.3): high-value enterprise orgs get their own dedicated PostgreSQL schema or instance rather than sharing RLS-isolated rows. This isn't a deployment change — it's a provisioning change: when `tier_isolation = 'schema'`, the SCIM provisioning saga creates a new PostgreSQL schema and migrates that org's tables into it. The connection pooler routes the org's traffic to the schema-scoped connection string.

---

## Cross-tier decisions and common mistakes

**Don't add Kubernetes at Tier 0.** The operational overhead of a K8s control plane (etcd, kube-apiserver, kubelet on every node) consumes roughly 2–4 GB of RAM on a small cluster — that's 10–15% of your Tier 0 node's total memory before a single service starts. Use Docker Compose until you have the specific scaling problems that K8s solves: multiple replicas of the same service, per-service autoscaling, and rolling deployments without downtime.

**Add Redis Sentinel before you need it.** The single hardest Tier 0→1 migration is retrofitting HA Redis into a live system where the auth JTI blocklist is already populated. Sessions in flight will not be revoked correctly during the Sentinel cutover window. Schedule this migration during a maintenance window and brief your on-call team.

**The Kafka consumer offset commit contract does not change across tiers.** This is the most common regression during tier migrations: engineers switch to auto-commit for "simplicity" when moving to a managed Kafka service. The spec's manual-commit requirement exists to prevent audit log gaps. An event that reaches the audit service's Kafka consumer but fails the MongoDB bulk write must be retried — auto-commit drops it permanently. Verify this in your migration acceptance test by killing the audit service pod mid-batch and confirming no `event_id` gaps exist after restart.

**The outbox pending record count is your primary production health signal.** More than RPS, more than error rate, more than latency — `openguard_outbox_pending_records` tells you whether your system is making forward progress. A spike here means either the Kafka broker is down, the outbox relay has stopped (a Go panic, an OOM kill, a database connection exhaustion), or you've hit a configuration limit. Wire this alert before anything else, at every tier.

**PgBouncer is mandatory before Tier 2.** The spec notes this in Section 14.8. At Tier 1 you can survive with pgxpool's direct connection management. At Tier 2, with 3+ replicas of control-plane each holding a pool of 15–25 connections, you'll hit PostgreSQL's `max_connections=200` default with 9 services × 2 replicas × 15 pool min = 270 connections at rest. Add PgBouncer in transaction pooling mode at the Tier 1→2 boundary, not after you've already hit the connection limit under production load.