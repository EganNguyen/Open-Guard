# Open Guard: Enterprise Scaling Strategies
### Designing for Atlassian Guard-Scale Workloads
> **Target Audience:** Senior Backend Engineers & System Architects
> **Version:** 1.0 | **Classification:** Engineering Reference

---

## Table of Contents

1. [System Scale Targets](#1-system-scale-targets)
2. [High-Level Architecture Overview](#2-high-level-architecture-overview)
3. [Multi-Tenant Architecture](#3-multi-tenant-architecture)
4. [Horizontal Scaling](#4-horizontal-scaling)
5. [Distributed Caching](#5-distributed-caching)
6. [Database Sharding & Partitioning](#6-database-sharding--partitioning)
7. [Event-Driven Architecture & Kafka Streaming](#7-event-driven-architecture--kafka-streaming)
8. [Authentication & Session Scaling](#8-authentication--session-scaling)
9. [RBAC & Permission Evaluation](#9-rbac--permission-evaluation)
10. [Audit Log Pipelines](#10-audit-log-pipelines)
11. [Observability & Monitoring](#11-observability--monitoring)
12. [Rate Limiting & DDoS Protection](#12-rate-limiting--ddos-protection)
13. [Regional & Global Deployment](#13-regional--global-deployment)
14. [Disaster Recovery & Failover](#14-disaster-recovery--failover)
15. [Security Isolation Between Tenants](#15-security-isolation-between-tenants)
16. [Kubernetes & Container Orchestration](#16-kubernetes--container-orchestration)
17. [CI/CD & Platform Engineering](#17-cicd--platform-engineering)
18. [Handling Sudden Traffic Spikes](#18-handling-sudden-traffic-spikes)
19. [Consistency vs. Availability Tradeoffs](#19-consistency-vs-availability-tradeoffs)
20. [Request Flow Under Heavy Load](#20-request-flow-under-heavy-load)
21. [Technology Stack Summary](#21-technology-stack-summary)

---

## 1. System Scale Targets

| Dimension | Target | Atlassian Guard Benchmark |
|---|---|---|
| Enterprise Tenants | 50,000+ | ~10,000+ organizations |
| Total Users | 100M+ | Tens of millions |
| Auth Throughput | 500,000 req/s peak | High five figures/s |
| Policy Evaluations | 2M evaluations/s | Sub-millisecond enforcement |
| Audit Events | 10B+ events/day | Petabyte-scale retention |
| API Latency (p99) | < 50ms globally | < 100ms |
| Availability | 99.999% (5 nines) | 99.99%+ SLA |
| RTO | < 60 seconds | Minutes |
| RPO | < 5 seconds | Near-zero |

These targets require a fundamentally different architecture than a standard SaaS system. Every layer — from the edge to the database — must be designed for horizontal elasticity, fault isolation, and data locality.

---

## 2. High-Level Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          GLOBAL EDGE LAYER                                      │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│   │  Cloudflare  │  │  AWS Shield  │  │  Anycast DNS │  │  GeoDNS LB   │       │
│   │  WAF / DDoS  │  │  Advanced    │  │  (Route 53)  │  │  Failover    │       │
│   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │
└──────────┼─────────────────┼─────────────────┼─────────────────┼───────────────┘
           │                 │                 │                 │
           └─────────────────┴────────┬────────┴─────────────────┘
                                      │
┌─────────────────────────────────────▼───────────────────────────────────────────┐
│                         REGIONAL API GATEWAY CLUSTER                            │
│   ┌────────────────────────────────────────────────────────────────────────┐    │
│   │  Kong / Envoy Gateway (per region: us-east-1, eu-west-1, ap-southeast) │    │
│   │  • Rate Limiting   • Auth Token Validation   • Tenant Routing          │    │
│   │  • Circuit Breaker • mTLS Termination        • Request Tracing         │    │
│   └────────────────────────────────────────────────────────────────────────┘    │
└──────────────┬─────────────────────────┬────────────────────────┬───────────────┘
               │                         │                        │
┌──────────────▼──────────┐  ┌───────────▼──────────┐  ┌─────────▼──────────────┐
│    AUTH SERVICE MESH    │  │   POLICY ENGINE MESH │  │  AUDIT SERVICE MESH    │
│  (Stateless JWT/OIDC)   │  │  (OPA / Casbin pods) │  │  (Kafka + Flink)       │
│  • Token issuance       │  │  • RBAC evaluation   │  │  • Event ingestion     │
│  • Session validation   │  │  • Attribute checks  │  │  • Stream processing   │
│  • MFA orchestration    │  │  • Policy caching    │  │  • Hot/cold tiering    │
└──────────┬──────────────┘  └──────────┬───────────┘  └──────────┬─────────────┘
           │                            │                          │
┌──────────▼────────────────────────────▼──────────────────────────▼─────────────┐
│                        DISTRIBUTED CACHE TIER                                   │
│            Redis Cluster (Sentinel/Cluster mode) — per region                  │
│   • Sessions   • Permission Decisions   • Tenant Config   • Rate Limit State   │
└──────────────────────────────────┬──────────────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────────────┐
│                          DATA TIER (Sharded)                                    │
│  ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────────────┐   │
│  │  CockroachDB /    │  │  Cassandra /      │  │  ClickHouse / BigQuery    │   │
│  │  Vitess (MySQL)   │  │  ScyllaDB         │  │  (Audit / Analytics)      │   │
│  │  Shard by tenant  │  │  Time-series data │  │  Columnar, partitioned    │   │
│  └───────────────────┘  └───────────────────┘  └───────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

**Key Architectural Principles:**

- **Shared-nothing between tenants** at the data layer
- **Shared-everything** (safely) at the compute layer with strict namespace isolation
- **Event-first:** every state change is an event — no direct DB fan-out
- **Defense in depth:** security controls at edge, gateway, service, and data layers
- **Observability-native:** traces, metrics, and logs from day one — not retrofitted

---

## 3. Multi-Tenant Architecture

### Why It's Needed

At 50,000+ tenants, the system must serve wildly different workload shapes simultaneously: a 5-user startup alongside a 500,000-user bank. Without deliberate multi-tenancy design, a single noisy tenant will degrade every other tenant — the "noisy neighbor" problem.

### Bottleneck It Solves

- Single-tenant databases don't scale cost-effectively beyond ~1,000 tenants
- Shared databases without isolation become contention hotspots
- Schema-per-tenant approaches collapse at scale due to connection pool exhaustion

### Architecture: Tiered Tenancy Model

```
Tenant Tiers
────────────
┌─────────────────────────────────────────────────────────────────┐
│  TIER 1: Enterprise (>10,000 users)                             │
│  • Dedicated compute pool (isolated K8s node group)             │
│  • Dedicated DB shard                                           │
│  • Guaranteed QoS / burst capacity                              │
│  • Custom SLA, private link ingress                             │
├─────────────────────────────────────────────────────────────────┤
│  TIER 2: Business (500–10,000 users)                            │
│  • Shared compute pool with resource quotas (K8s ResourceQuota) │
│  • Shared shard group (1 shard per 50 tenants)                  │
│  • Soft rate limits, burst allowed                              │
├─────────────────────────────────────────────────────────────────┤
│  TIER 3: Starter (<500 users)                                   │
│  • Fully shared compute                                         │
│  • Shared shard (1 shard per 500 tenants)                       │
│  • Hard rate limits via token bucket                            │
└─────────────────────────────────────────────────────────────────┘
```

### Tenant Context Propagation

Every internal service call carries a `TenantContext` header:

```
X-Tenant-ID: <uuid>
X-Tenant-Tier: enterprise
X-Tenant-Shard: shard-023
X-Request-ID: <trace-id>
```

Services use this context to:
- Route DB queries to the correct shard
- Apply tenant-specific rate limits
- Scope cache namespaces (`tenant:{id}:sessions:*`)
- Emit correctly labeled metrics and traces

### Isolation Mechanisms

| Layer | Mechanism |
|---|---|
| Network | Kubernetes NetworkPolicy, per-tenant VPC (Tier 1) |
| Compute | ResourceQuota, LimitRange per namespace |
| Database | Row-level tenancy filter (mandatory WHERE clause) |
| Cache | Prefixed key namespacing |
| Secrets | Vault dynamic secrets with tenant-scoped policies |
| Logs | Structured fields + RBAC-controlled log access |

### Tradeoffs

- **Cost:** Tier 1 isolation is expensive — offset by premium pricing
- **Complexity:** Tenant context must be threaded through every layer
- **Operational overhead:** More moving parts to monitor and upgrade

### Estimated Scalability Impact

Tiered tenancy allows 10–50x more tenants per infrastructure dollar compared to fully isolated deployments, while maintaining the isolation guarantees that enterprises demand.

---

## 4. Horizontal Scaling

### Why It's Needed

Vertical scaling (larger machines) hits hard limits at cloud instance sizes and creates single points of failure. Horizontal scaling (more instances) is the only path to unlimited throughput with high availability.

### Bottleneck It Solves

- Stateful services that can't scale out without session affinity
- Database connection pools that saturate on large instances
- Single-region deployments that can't handle regional traffic bursts

### Stateless Service Design

All services in Open Guard must be **stateless by default**:

```
Stateful (bad for horizontal scaling)          Stateless (correct)
────────────────────────────────────           ──────────────────────────────────
Store session in local memory              →   Store session in Redis
Store config in local cache               →   Load from config service on startup
Write to local disk                       →   Write to object storage (S3/GCS)
Maintain WebSocket state in process       →   Use sticky sessions + Redis pub/sub
```

### Horizontal Pod Autoscaler (HPA) Configuration

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: auth-service-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: auth-service
  minReplicas: 10
  maxReplicas: 500
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 60
    - type: Pods
      pods:
        metric:
          name: auth_requests_per_second
        target:
          type: AverageValue
          averageValue: "1000"
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 30
      policies:
        - type: Pods
          value: 50
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
```

### Connection Pooling at Scale

Direct DB connections from 500 pods × 10 connections = 5,000 connections — most databases choke above 1,000 concurrent connections.

**Solution: PgBouncer / ProxySQL as a connection pool sidecar**

```
Auth Service Pod (×500)
   └── PgBouncer sidecar (pool: 5 connections per pod)
          └── PostgreSQL Primary (max 500 total from poolers)
```

### Tradeoffs

- **Cold start latency:** More pods mean more startup time during scale-up events
- **Debugging complexity:** Distributed tracing is mandatory; logs across 500 pods are useless without correlation
- **Cost:** Minimum replica counts for availability add baseline cost even at low load

### Estimated Scalability Impact

With proper stateless design and HPA, auth throughput scales linearly: 10 pods → 100 pods → 10× throughput with no architecture changes.

---

## 5. Distributed Caching

### Why It's Needed

At 500,000 auth requests/second, hitting the database for every session validation, permission check, and tenant config lookup would require thousands of database cores. Caching is the single biggest scalability lever.

### Bottleneck It Solves

- DB read IOPS saturation on permission lookups (millions/second)
- Repeated deserialization of large policy objects
- Cross-region latency for frequently accessed configuration

### Cache Hierarchy

```
┌─────────────────────────────────────────────────────────────────┐
│ L1: In-Process Cache (Caffeine / Guava)                         │
│ • TTL: 5–30 seconds                                             │
│ • Size: 10,000 entries per pod                                  │
│ • Use: Hot permission decisions, feature flags                  │
│ • Hit rate target: 70%                                          │
├─────────────────────────────────────────────────────────────────┤
│ L2: Regional Redis Cluster                                      │
│ • TTL: 5–15 minutes                                             │
│ • Size: 500GB per region (3 primaries, 3 replicas per shard)    │
│ • Use: Sessions, tenant config, RBAC roles, token validation    │
│ • Hit rate target: 25%                                          │
├─────────────────────────────────────────────────────────────────┤
│ L3: Database (Source of Truth)                                  │
│ • Only 5% of requests should reach here                         │
│ • All writes go here first, invalidate cache via event          │
└─────────────────────────────────────────────────────────────────┘
```

### Redis Cluster Topology

```
Region: us-east-1
  Shard 0:  redis-us-east-primary-0  +  redis-us-east-replica-0a, 0b
  Shard 1:  redis-us-east-primary-1  +  redis-us-east-replica-1a, 1b
  Shard 2:  redis-us-east-primary-2  +  redis-us-east-replica-2a, 2b

Keyspace Distribution (consistent hashing via Redis Cluster):
  {tenant:abc}:session:*   → Shard 0
  {tenant:xyz}:roles:*     → Shard 1
  {tenant:def}:config:*    → Shard 2
```

### Cache Invalidation Strategy

**Problem:** Cache invalidation is famously hard. At scale, stale permissions can be a security incident, not just a bug.

```
Write Path (Policy Update):
  1. Admin updates RBAC policy
  2. Policy Service writes to DB
  3. Policy Service publishes kafka event: policy.updated { tenant_id, policy_hash }
  4. All regional Redis caches subscribe to this topic via Kafka Consumer
  5. Cache entries for affected tenant are invalidated (DEL pattern)
  6. L1 caches expire within their TTL window (max 30s lag acceptable)

Read Path (Cache Miss):
  1. Service checks L1 → miss
  2. Service checks Redis → miss
  3. Service acquires distributed lock (Redis SETNX) to prevent cache stampede
  4. Service reads from DB
  5. Service writes to Redis with appropriate TTL
  6. Service populates L1
  7. Lock is released
```

### Cache Stampede Prevention

```python
# Probabilistic early expiration (XFetch algorithm)
def get_with_early_expiry(key, ttl, delta=0.1, beta=1.0):
    cached = redis.get(key)
    if cached:
        remaining_ttl = redis.ttl(key)
        # Recompute early with probability proportional to remaining TTL
        if time.time() - delta * beta * math.log(random.random()) >= remaining_ttl:
            # Recompute before expiry to avoid stampede
            return recompute_and_cache(key, ttl)
        return cached
    return recompute_and_cache(key, ttl)
```

### Tradeoffs

- **Consistency risk:** Stale permissions for up to 30 seconds after a policy change (mitigated by event-driven invalidation for security-critical changes)
- **Memory cost:** 500GB Redis per region adds significant infra cost
- **Operational complexity:** Redis Cluster split-brain scenarios require careful monitoring

### Estimated Scalability Impact

Proper L1+L2 caching reduces database load by 95%, enabling a 20× increase in throughput without scaling the database tier.

---

## 6. Database Sharding & Partitioning

### Why It's Needed

A single PostgreSQL instance tops out at ~100,000 writes/second under real-world conditions. At 10B audit events/day and millions of active users across 50,000 tenants, the data tier must be distributed.

### Bottleneck It Solves

- Single-writer bottleneck in traditional RDBMS
- Table lock contention on large tables during bulk operations
- Cross-tenant query interference (one slow analytical query blocking auth queries)

### Sharding Strategy

**Primary Strategy: Tenant-Based Hash Sharding**

```
Shard Assignment:
  shard_id = hash(tenant_id) % num_shards

Example with 64 shards:
  tenant "acme-corp"  → shard_017
  tenant "initech"    → shard_042
  tenant "globodyne"  → shard_003

Database Cluster Map:
  Shards 0–15   → db-cluster-us-east-a  (CockroachDB nodes: 3 primaries)
  Shards 16–31  → db-cluster-us-east-b
  Shards 32–47  → db-cluster-eu-west-a
  Shards 48–63  → db-cluster-ap-southeast-a
```

**Why tenant-based sharding:**
- All data for a tenant lives on one shard → no cross-shard joins for 99% of queries
- Tenant isolation is automatic
- Resharding can be done gradually by migrating tenants

### Recommended Database Technologies by Use Case

| Data Type | Technology | Why |
|---|---|---|
| Core identity & auth data | CockroachDB or Vitess (MySQL) | Distributed SQL, horizontal scale, ACID |
| Policy & RBAC rules | PostgreSQL (sharded via Citus) | Complex queries, small dataset per tenant |
| Session data | Redis + Cassandra (overflow) | High write throughput, TTL native |
| Audit events (hot) | Apache Cassandra / ScyllaDB | Write-optimized, time-series native |
| Audit events (cold/analytics) | ClickHouse or BigQuery | Columnar, fast aggregations |
| Feature flags & config | etcd or Consul | Strongly consistent, watch-based updates |

### Table Partitioning for Audit Logs

```sql
-- Audit events partitioned by month + tenant bucket
CREATE TABLE audit_events (
    id          UUID DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    event_type  VARCHAR(100) NOT NULL,
    actor_id    UUID,
    resource_id UUID,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

-- Monthly partitions (auto-created via pg_partman or custom job)
CREATE TABLE audit_events_2025_01 PARTITION OF audit_events
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

-- Old partitions detached and moved to cold storage (S3 Parquet)
-- after 90 days
```

### Resharding Strategy

```
Phase 1: Double-write (new shard + old shard)
Phase 2: Backfill new shard from old
Phase 3: Verify checksums match
Phase 4: Switch reads to new shard
Phase 5: Drain writes from old shard
Phase 6: Decommission old shard

Zero downtime. Rollback at any phase.
```

### Tradeoffs

- **Cross-tenant reporting** becomes expensive (requires scatter-gather across shards)
- **Schema migrations** must be applied to all shards simultaneously (blue-green via Flyway/Liquibase)
- **Operational complexity** scales with shard count

### Estimated Scalability Impact

64 shards with CockroachDB can sustain 5M+ writes/second globally with p99 latency under 10ms.

---

## 7. Event-Driven Architecture & Kafka Streaming

### Why It's Needed

In a system with thousands of tenants and dozens of microservices, synchronous point-to-point communication creates a dependency web that fails at scale. Event-driven architecture decouples producers from consumers, enabling independent scaling and graceful degradation.

### Bottleneck It Solves

- Synchronous API chains that fail if any service is slow or down
- Fan-out problems (one policy change needs to notify 15 services)
- Audit log write amplification blocking request paths
- Real-time policy enforcement requiring immediate propagation

### Kafka Topology

```
Kafka Cluster (3 brokers minimum per region, 9 for production):
  Replication factor: 3
  Min ISR: 2
  Retention: 7 days (hot), unlimited in S3 via Tiered Storage

Topic Design:
┌─────────────────────────────────────────────────────────────────┐
│  Topic: auth.events                                             │
│  Partitions: 256 (keyed by tenant_id)                           │
│  Consumers: Audit Pipeline, Anomaly Detection, SIEM Exporter    │
├─────────────────────────────────────────────────────────────────┤
│  Topic: policy.changes                                          │
│  Partitions: 64 (keyed by tenant_id)                            │
│  Consumers: Cache Invalidator, Policy Sync, Compliance Reporter │
├─────────────────────────────────────────────────────────────────┤
│  Topic: audit.raw                                               │
│  Partitions: 512 (keyed by tenant_id for ordering)              │
│  Consumers: Audit Processor, ClickHouse Sink, S3 Sink           │
├─────────────────────────────────────────────────────────────────┤
│  Topic: user.lifecycle                                          │
│  Partitions: 128                                                │
│  Consumers: Permission Sync, Notification, Directory Sync       │
└─────────────────────────────────────────────────────────────────┘
```

### Event Schema (CloudEvents standard)

```json
{
  "specversion": "1.0",
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source": "//openguard/auth-service/us-east-1",
  "type": "com.openguard.auth.login.success",
  "tenantid": "acme-corp",
  "time": "2025-01-15T10:30:00Z",
  "datacontenttype": "application/json",
  "data": {
    "user_id": "user_123",
    "ip_address": "203.0.113.42",
    "user_agent": "Mozilla/5.0...",
    "mfa_method": "totp",
    "risk_score": 12,
    "session_id": "sess_abc"
  }
}
```

### Event Sourcing for Policy State

Critical policy changes use Event Sourcing — the policy state is derived from an ordered log of events, not a mutable record:

```
PolicyCreated { tenant, policy_id, definition }
PolicyUpdated { tenant, policy_id, delta, version }
PolicyEnabled { tenant, policy_id }
PolicyDisabled { tenant, policy_id }

Current policy state = replay of all events for that policy_id
Snapshots taken every 100 events to avoid full replay
```

**Why:** Provides complete audit trail, enables point-in-time policy reconstruction, and allows safe rollback to any historical state.

### Kafka Streams Processing

```
auth.events topic
      │
      ▼
┌─────────────────────────────────────────────────┐
│  Kafka Streams Job: Anomaly Detection           │
│  • Window: 5-minute tumbling                    │
│  • Alert if: >50 failed logins from same IP     │
│  • Alert if: Login from 2 countries in <1 hour  │
│  • Output → security.alerts topic               │
└─────────────────────────────────────────────────┘
      │
      ▼
┌─────────────────────────────────────────────────┐
│  Flink Job: Real-time Policy Enforcement        │
│  • Enrich events with current policy            │
│  • Evaluate compliance rules                    │
│  • Output → policy.violations topic             │
└─────────────────────────────────────────────────┘
```

### Tradeoffs

- **Eventual consistency:** Consumers may lag behind producers by milliseconds to seconds
- **Operational complexity:** Kafka requires careful tuning (partition count, retention, rebalancing)
- **Debugging difficulty:** Tracing an event through 5 consumer hops requires distributed tracing
- **Schema evolution:** Breaking changes to event schemas can crash consumers (mitigated with Schema Registry + Avro)

### Estimated Scalability Impact

Kafka can sustain 10M+ events/second per cluster. Decoupling producers from consumers allows the audit pipeline to absorb traffic bursts without back-pressuring the auth service.

---

## 8. Authentication & Session Scaling

### Why It's Needed

Authentication is the hottest path in the system — every API call passes through it. At 500,000 requests/second, session validation must complete in under 1ms to not dominate total request latency.

### Bottleneck It Solves

- Centralized session store becoming a single point of failure
- Session database reads adding 10–50ms to every request
- Token revocation at scale (invalidating tokens issued to millions of users)

### JWT-First Architecture

```
Token Issuance (AuthN Service):
  User credentials → validate → issue JWT (RS256)
  JWT payload:
  {
    "sub": "user_123",
    "tid": "tenant_acme",  // tenant ID
    "iat": 1704067200,
    "exp": 1704070800,     // 1-hour access token
    "jti": "jwt_abc",      // unique ID for revocation
    "roles": ["admin"],    // pre-embedded for fast evaluation
    "shard": "17"          // DB shard hint
  }

Token Validation (every service, local):
  1. Verify RS256 signature against cached public key (in-process, 0 DB calls)
  2. Check exp claim (no network call)
  3. Check jti against revocation bloom filter (in-process, probabilistic)
  4. Done — < 0.1ms
```

### JWKS Key Rotation Without Downtime

```
Key Rotation Protocol:
  T+0:   Generate new keypair (key_v2)
  T+0:   Publish key_v2 to JWKS endpoint (alongside key_v1)
  T+5m:  Begin issuing new tokens signed with key_v2
  T+1h:  All tokens signed with key_v1 have expired (max TTL = 1h)
  T+1h:  Remove key_v1 from JWKS endpoint
  
All services cache JWKS for 5 minutes with background refresh.
No service restart required.
```

### Session Scaling with Redis

```
Session Store Layout (Redis):
  Key:    session:{jti}
  Value:  {user_id, tenant_id, ip, created_at, last_active, mfa_verified}
  TTL:    sliding window, reset on each request (max 8h idle timeout)

Write path: Auth service writes on login
Read path:  Gateway reads on every request (L2 cache, ~0.3ms)
Invalidation: DEL on logout; pub/sub to invalidate across regions
```

### Token Revocation at Scale

**Problem:** Revoking a JWT before expiry requires either short TTLs (bad UX) or a revocation check (latency).

**Solution: Bloom Filter Revocation List**

```
Revocation Bloom Filter:
  - Stored in Redis as a bit array (2MB for 10M revoked JTIs)
  - False positive rate: 0.1% (acceptable — triggers a full DB check)
  - Updated via Kafka consumer (logout.events topic)
  - Replicated to all regional Redis clusters
  - No DB lookup for 99.9% of requests
  
On revocation:
  1. Auth service writes jti to revoked_tokens DB table
  2. Publishes to revocation.events Kafka topic
  3. All regions' Bloom filter update consumers add jti to filter
  4. Regional Redis Bloom filters updated within ~500ms
```

### Multi-Factor Authentication Scaling

MFA challenges (TOTP, WebAuthn, push) are stateful. Scale them with:

```
MFA Challenge State:
  Key:    mfa_challenge:{challenge_id}
  Value:  {user_id, method, issued_at, attempts}
  TTL:    5 minutes
  
WebAuthn Registration Ceremonies:
  State stored in Redis with 10-minute TTL
  Credential public keys stored in DB (per user, per device)
```

### Tradeoffs

- **Short JWT TTL vs. revocation complexity:** 15-minute access tokens + refresh tokens balance security and UX
- **Bloom filter false positives:** 0.1% extra DB reads is acceptable overhead for eliminating 99.9% of revocation lookups
- **Regional consistency of revocation:** ~500ms propagation lag means a stolen token may be used once after revocation in a remote region

### Estimated Scalability Impact

JWT-first architecture with Redis session validation enables 500,000+ auth validations/second per region with p99 latency under 2ms.

---

## 9. RBAC & Permission Evaluation

### Why It's Needed

Enterprise systems have complex, nested permission hierarchies. Evaluating "can user X perform action Y on resource Z?" for every API request at millions of requests/second requires a purpose-built evaluation engine, not a database query.

### Bottleneck It Solves

- DB-based permission checks adding 20–100ms per request
- Complex permission logic scattered across application code
- Policy changes requiring service restarts to take effect

### Permission Evaluation Architecture

```
Permission Check Request Flow:

API Request
    │
    ▼
Gateway extracts: { user_id, tenant_id, action, resource, context }
    │
    ▼
L1 Cache (in-process): Check decision cache (key: user+action+resource)
    │ Miss
    ▼
Policy Engine Pod (OPA / Open Policy Agent):
    │
    ├── Load policy bundle (cached, refreshed on policy.changes event)
    │
    ├── Load user roles (from Redis cache)
    │
    ├── Evaluate Rego policy:
    │     allow {
    │       role := data.tenant[tenant_id].role_assignments[user_id][_]
    │       permission := data.roles[role].permissions[_]
    │       permission.action == input.action
    │       permission.resource_type == input.resource_type
    │     }
    │
    └── Return: { allow: true/false, reason: "...", ttl: 60 }
    │
    ▼
Cache result in Redis (TTL: 60s) and L1 (TTL: 10s)
```

### OPA Policy Bundle Distribution

```
Policy Update Flow:
  1. Admin modifies RBAC policy in UI
  2. Policy Service validates and writes to DB
  3. Policy Service publishes bundle update to Kafka (policy.changes)
  4. OPA Bundle Server regenerates bundle (tar.gz of all Rego files + data.json)
  5. OPA pods fetch updated bundle from Bundle Server (HTTP long-poll or push)
  6. OPA pods reload policy in-memory (atomic swap, no downtime)
  7. Old cached decisions expire via TTL or explicit cache invalidation

Bundle Server:
  • Serves policy bundles from object storage (S3)
  • Uses ETag/If-None-Match for efficient polling
  • CDN-cacheable for global distribution
  • Bundle generation p99: < 2 seconds
```

### Permission Decision Caching Strategy

```
Cache Key Design:
  sha256(tenant_id + user_id + action + resource_type + resource_id + context_hash)

Invalidation Events:
  - User role changed    → invalidate all decisions for user
  - Role permission changed → invalidate all decisions for role members
  - Resource ownership changed → invalidate decisions for resource
  
Cache Invalidation via Kafka:
  - Policy service publishes invalidation event with scope
  - Redis consumer uses SCAN + DEL pattern for user-scoped invalidation
  - Cluster-wide invalidation uses Redis keyspace notifications
```

### Attribute-Based Access Control (ABAC) for Complex Policies

For enterprise tenants requiring fine-grained control:

```
Policy Example (Rego):
  # Users can only access resources in their department
  # unless they have global_admin role
  allow {
    not has_global_admin
    user_department := data.users[input.user_id].department
    resource_department := data.resources[input.resource_id].department
    user_department == resource_department
    basic_read_allowed
  }

  allow {
    has_global_admin
  }

  has_global_admin {
    "global_admin" == data.roles[input.user_id][_]
  }
```

### Tradeoffs

- **OPA warm-up time:** Policy evaluation on first request after bundle update is slower (mitigated by pre-warming)
- **Consistency window:** Policy changes propagate within 2–30 seconds depending on cache TTLs
- **Rego complexity:** Complex policies are hard to debug — invest in OPA test suites and policy simulation tooling

### Estimated Scalability Impact

OPA with in-process evaluation completes permission checks in < 0.5ms. With L1 caching, 95% of checks complete in < 0.1ms. A 100-pod fleet can sustain 10M+ permission evaluations/second.

---

## 10. Audit Log Pipelines

### Why It's Needed

Enterprise compliance requires immutable, queryable audit logs for SOC 2, ISO 27001, HIPAA, and GDPR. At 10B events/day across 50,000 tenants, the audit pipeline must handle massive write throughput without impacting the auth path.

### Bottleneck It Solves

- Synchronous audit writes blocking auth API responses
- Single-table audit tables hitting write IOPS limits
- Cross-tenant audit data leakage in shared tables
- Compliance reporting requiring full-table scans

### Audit Pipeline Architecture

```
Event Source (Auth/Policy Services)
       │  (fire-and-forget, async)
       ▼
Kafka Topic: audit.raw (512 partitions)
       │
       ├──────────────────────────────────┐
       ▼                                  ▼
┌──────────────────┐            ┌─────────────────────┐
│  Flink Streaming │            │  Direct S3 Sink      │
│  Processor       │            │  (raw backup, Avro)  │
│  • Enrichment    │            └─────────────────────-┘
│  • Deduplication │
│  • Schema valid. │
│  • PII masking   │
└──────┬───────────┘
       │
       ├────────────────────────────────────────┐
       ▼                                        ▼
┌──────────────────┐                   ┌────────────────────┐
│  ClickHouse      │                   │  Cassandra         │
│  (analytics)     │                   │  (hot queryable,   │
│  • Columnar      │                   │   last 90 days)    │
│  • Fast GROUP BY │                   │  • Tenant-keyed    │
│  • Partitioned   │                   │  • Time-ordered    │
│  by month        │                   └────────────────────┘
└──────────────────┘
```

### Hot/Warm/Cold Storage Tiering

```
Tier    Duration    Storage          Query Latency    Cost/GB/month
─────   ─────────   ─────────────    ─────────────    ─────────────
Hot     0–90 days   ScyllaDB         < 10ms           $0.25
Warm    90d–2yr     ClickHouse       < 1s             $0.04
Cold    2yr–7yr     S3 + Parquet     Minutes (Athena) $0.023
Archive 7yr+        S3 Glacier       Hours            $0.004
```

### Audit Log Immutability

Audit logs must be tamper-evident. Achieve this with:

```
Cryptographic Chaining:
  Each audit batch contains:
    batch_id     : UUID
    tenant_id    : UUID
    events       : [...event hashes...]
    prev_hash    : hash of previous batch
    batch_hash   : sha256(events + prev_hash)
    signature    : RSA sign(batch_hash) using tenant audit key

Verification:
  Compliance reporter can verify chain integrity by recomputing hashes
  Any tampering breaks the chain — detectable immediately

Storage:
  Batch hashes stored in separate immutable log (Amazon QLDB or custom)
  Enables selective event querying with chain verification
```

### Audit Query API

```
GET /api/v1/audit?
  tenant_id=acme-corp
  &from=2025-01-01T00:00:00Z
  &to=2025-01-31T23:59:59Z
  &event_type=auth.login
  &actor_id=user_123
  &page_token=<cursor>
  &limit=1000

Response (paginated, cursor-based):
  {
    "events": [...],
    "next_page_token": "...",
    "total_count": 847291
  }

Query routing:
  - Date range within 90 days → ScyllaDB (fast)
  - Date range older than 90 days → ClickHouse (batch)
  - Compliance export (full range) → Async job + S3 presigned URL
```

### Tradeoffs

- **Eventual consistency in audit log:** Events arrive out of order due to Kafka partitioning — Flink watermarking handles reordering up to 30-second delay
- **PII in audit logs:** Must mask or encrypt sensitive fields before cold storage (GDPR right to erasure)
- **Storage cost:** 10B events/day × 500 bytes average = 5TB/day — tiering is economically essential

### Estimated Scalability Impact

Decoupled Kafka-based ingestion sustains 500,000+ audit writes/second without impacting auth latency. ClickHouse aggregates across 1B events in under 30 seconds.

---

## 11. Observability & Monitoring

### Why It's Needed

At this scale, problems cannot be debugged by looking at logs on a single server. Distributed tracing, structured logs, and metrics correlation are the only way to understand system behavior when a request touches 12 services across 3 regions.

### Three Pillars of Observability

**1. Distributed Tracing (OpenTelemetry)**

```
Every request receives a trace_id at the edge. Each service adds spans:

Trace: auth.login (total: 47ms)
  ├── gateway.validate_token (2ms)
  ├── auth_service.authenticate (35ms)
  │     ├── redis.session_lookup (0.5ms)
  │     ├── db.user_lookup (shard_17, 4ms)
  │     ├── mfa.totp_verify (1ms)
  │     └── policy.check_login_allowed (1.5ms)
  ├── session.create (5ms)
  │     └── redis.session_write (0.8ms)
  └── audit.emit (async, non-blocking)

Storage: Jaeger / Grafana Tempo (sampled at 1% baseline, 100% on error)
```

**2. Structured Logging**

```json
{
  "timestamp": "2025-01-15T10:30:00.123Z",
  "level": "INFO",
  "service": "auth-service",
  "version": "v2.4.1",
  "region": "us-east-1",
  "pod": "auth-service-7f9b8c-xk2p4",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "tenant_id": "acme-corp",
  "user_id": "user_123",
  "event": "auth.login.success",
  "duration_ms": 47,
  "mfa_method": "totp",
  "risk_score": 12
}
```

All logs ship to a centralized platform (Grafana Loki or Elasticsearch) and are indexed by `tenant_id`, `trace_id`, and `event` type.

**3. Metrics (Prometheus / VictoriaMetrics)**

```
Key Metrics per Service:

Auth Service:
  auth_requests_total{tenant, method, status}
  auth_request_duration_seconds{tenant, method, quantile}
  auth_token_validations_total{result}
  auth_mfa_challenges_total{method, result}
  auth_session_cache_hit_ratio

Policy Engine:
  policy_evaluations_total{tenant, decision}
  policy_evaluation_duration_ms{quantile}
  policy_bundle_version{tenant}
  policy_cache_hit_ratio

Kafka Pipeline:
  kafka_consumer_lag{topic, partition, consumer_group}
  audit_events_ingested_total{tenant}
  audit_pipeline_processing_latency_seconds

Infrastructure:
  node_cpu_utilization
  redis_memory_used_bytes
  db_connection_pool_utilization{shard}
  db_query_duration_seconds{shard, query_type}
```

### SLO Alerting

```yaml
# SLO: 99.9% of auth requests complete in < 200ms
- alert: AuthSLOBudgetBurning
  expr: |
    (1 - (
      rate(auth_requests_total{status="success",duration_bucket="0.2"}[5m]) /
      rate(auth_requests_total[5m])
    )) > 0.001
  for: 5m
  labels:
    severity: critical
    team: auth-platform
  annotations:
    summary: "Auth SLO error budget burning at 50x rate"
    runbook: "https://runbooks.openguard.io/auth-slo-burn"
```

### Tenant-Level Observability

Each enterprise tenant gets a dedicated Grafana dashboard showing their own:
- Auth success/failure rates
- Policy evaluation trends
- Audit event volumes
- Active session counts
- Security alerts

This dashboard is served from a read-only metrics namespace scoped to that tenant's data.

---

## 12. Rate Limiting & DDoS Protection

### Why It's Needed

A single compromised account attempting credential stuffing at 10,000 requests/second can saturate auth service resources and deny service to legitimate tenants. Rate limiting and DDoS protection are mandatory at this scale.

### Bottleneck It Solves

- Credential stuffing attacks exhausting auth service CPU
- Noisy tenants consuming disproportionate API quota
- Layer 7 DDoS attacks bypassing network-level defenses

### Multi-Layer Rate Limiting

```
Layer 1: CDN/Edge (Cloudflare)
  • IP-based rate limiting (10,000 req/min per IP globally)
  • Challenge (CAPTCHA) on suspicious IPs
  • Bot detection and fingerprinting
  • Anycast routing to absorb volumetric DDoS

Layer 2: API Gateway (Kong)
  • Tenant-level rate limits (configurable per tier)
    - Starter: 100 req/s
    - Business: 1,000 req/s
    - Enterprise: 10,000 req/s (custom)
  • Endpoint-specific limits (auth: 10 req/s per user)
  • Response: HTTP 429 with Retry-After header

Layer 3: Service-Level (application)
  • Per-user rate limits enforced via Redis token bucket
  • Per-IP rate limits for unauthenticated endpoints
  • Adaptive limits based on risk score
```

### Token Bucket Algorithm (Redis-based)

```lua
-- Lua script executed atomically in Redis
local key = KEYS[1]
local rate = tonumber(ARGV[1])        -- tokens per second
local burst = tonumber(ARGV[2])       -- max burst
local now = tonumber(ARGV[3])         -- current timestamp (ms)
local requested = tonumber(ARGV[4])  -- tokens requested

local state = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(state[1]) or burst
local last_refill = tonumber(state[2]) or now

-- Refill tokens based on elapsed time
local elapsed = math.max(0, now - last_refill)
local refill = (elapsed / 1000) * rate
tokens = math.min(burst, tokens + refill)

if tokens >= requested then
  tokens = tokens - requested
  redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
  redis.call('EXPIRE', key, 3600)
  return {1, math.floor(tokens)}  -- allowed
else
  redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
  return {0, math.floor(tokens)}  -- denied
end
```

### Adaptive Rate Limiting

```
Risk-Based Rate Adjustment:
  Normal user, known IP, low risk score    → full quota
  New device, unknown IP, medium risk      → 50% quota + step-up auth
  Known bad IP, high failed attempts       → 10% quota + CAPTCHA
  IP on threat feed                        → block at edge
  
Risk Score Calculation (real-time, Kafka Streams):
  + impossible_travel: +40 points
  + failed_login_burst: +30 points
  + tor_exit_node: +25 points
  + new_device: +10 points
  + unusual_hour: +5 points
  → score >= 60: trigger MFA regardless of session state
  → score >= 80: block and alert
```

### DDoS Response Playbook

```
L3/L4 Volumetric Attack (>10 Gbps):
  Auto-mitigation: Cloudflare/AWS Shield absorbs at edge
  No service impact expected

L7 Application DDoS (smart bots):
  Detection: Anomalous spike in 429/401 rates + low bytes-per-request
  Response:
    1. Enable challenge mode at CDN layer (auto)
    2. Deploy shadow mode — rate limit at 10% of normal (auto)
    3. Isolate affected tenant if targeted attack (manual, < 5 minutes)
    4. Engage CDN DDoS response team if sustained > 15 minutes

Tenant-Targeted Attack:
  Detect: Single tenant's error rate > 10x normal
  Response: Temporarily route tenant to isolated compute pool
            Enable enhanced challenge mode for tenant's subdomain
```

---

## 13. Regional & Global Deployment

### Why It's Needed

A user in Tokyo authenticating against a US-East region incurs 200–300ms of network round-trip latency before any processing begins. Global enterprises require sub-50ms latency everywhere, which demands data and compute locality.

### Bottleneck It Solves

- Cross-ocean network latency dominating request time
- Single-region outages taking down global service
- Data residency compliance (GDPR, PDPA, etc.)

### Active-Active Multi-Region Topology

```
┌─────────────────────────────────────────────────────────────────┐
│                    GLOBAL ROUTING LAYER                         │
│              GeoDNS (Route53 Latency-Based)                     │
│              Anycast via Cloudflare (200+ PoPs)                 │
└────────┬───────────────────┬───────────────────┬───────────────-┘
         │                   │                   │
         ▼                   ▼                   ▼
┌────────────────┐  ┌────────────────┐  ┌────────────────┐
│  us-east-1     │  │  eu-west-1     │  │  ap-southeast-1│
│  PRIMARY       │  │  PRIMARY       │  │  PRIMARY       │
│                │  │                │  │                │
│  All services  │  │  All services  │  │  All services  │
│  Redis Cluster │  │  Redis Cluster │  │  Redis Cluster │
│  DB Shards     │  │  DB Shards     │  │  DB Shards     │
│  Kafka Cluster │  │  Kafka Cluster │  │  Kafka Cluster │
└────────────────┘  └────────────────┘  └────────────────┘
         │                   │                   │
         └───────────────────┴───────────────────┘
                   Global Kafka MirrorMaker 2
                   (cross-region replication)
```

### Tenant-to-Region Assignment

```
Tenant Placement Rules:
  1. GDPR tenants → eu-west-1 only (data residency)
  2. PDPA tenants → ap-southeast-1 only
  3. US-only enterprise → us-east-1 primary, us-west-2 DR
  4. Global enterprise → closest region primary, others replica

Tenant config stored in global config store (CockroachDB Global Tables)
Request routing: GeoDNS → API Gateway → tenant home region lookup → proxy if needed
```

### Cross-Region Data Replication

```
Write-Local, Replicate-Async:
  1. Write lands in tenant's home region
  2. Committed to local DB (strong consistency within region)
  3. CDC event published to regional Kafka
  4. Kafka MirrorMaker 2 replicates to other regions
  5. Remote regions apply writes within 200–500ms

Read Strategy:
  - Session validation: read from local region (may be 500ms stale — acceptable)
  - Permission checks: read from local region (stale by 1 policy bundle update cycle)
  - User creation/deletion: write to home region + strong read after write
  - Compliance audit export: always read from home region (source of truth)
```

### Latency Budget by Region

```
Target: p99 < 50ms from any region to nearest Open Guard PoP

User                   Nearest PoP   Network RTT   Processing   Total
─────                  ───────────   ───────────   ──────────   ─────
New York               us-east-1     5ms           15ms         20ms ✓
London                 eu-west-1     8ms           15ms         23ms ✓
Singapore              ap-southeast  10ms          15ms         25ms ✓
São Paulo              us-east-1     120ms         15ms         135ms ✗
                       → Action: Add sa-east-1 region
```

---

## 14. Disaster Recovery & Failover

### Why It's Needed

At 99.999% availability target, the system can be down for at most 5.26 minutes per year. A single regional outage without automatic failover burns through that budget in minutes.

### Recovery Objectives

| Scenario | RTO | RPO | Strategy |
|---|---|---|---|
| Pod crash | < 30s | 0 | K8s pod restart |
| AZ failure | < 60s | 0 | Multi-AZ deployment |
| Region failure | < 3min | < 5s | Active-active failover |
| DB shard failure | < 60s | 0 | Synchronous replica promotion |
| Kafka broker failure | < 30s | 0 | ISR replication |
| Total region loss | < 5min | < 30s | Traffic reroute + async replicas |

### Automated Failover Flow

```
Region Failure Detection & Response:

T+0s:    us-east-1 health checks begin failing
T+15s:   Route53 health checker detects failure (3 consecutive failures)
T+20s:   Route53 removes us-east-1 from DNS (TTL: 10s)
T+30s:   Traffic begins routing to eu-west-1 and ap-southeast-1
T+45s:   Tenant placement service promotes eu-west-1 as primary
         for US tenants (async DB replicas already up to date)
T+60s:   All traffic fully rerouted, 100% available in other regions
T+3min:  Kafka MirrorMaker promotes remote topics to primary
T+5min:  Incident declared, on-call team engaged for recovery
```

### Backup Strategy

```
Database Backups:
  Continuous WAL archiving to S3 (point-in-time recovery to any second)
  Daily full snapshots (cross-region replication to 2nd bucket)
  Weekly snapshots retained for 90 days
  Monthly snapshots retained for 7 years (compliance)

Backup Verification:
  Automated daily restore test in isolated environment
  Checksums verified against production data
  Alerting if restore time > RTO target
```

### Chaos Engineering

Regularly validate failover procedures:

```
Monthly Game Days:
  - Kill a random DB replica in production during low traffic
  - Simulate AZ failure (block AZ traffic via firewall rules)
  - Inject 500ms latency between services (Toxiproxy)
  - Kill 50% of auth service pods suddenly

Automated Chaos (Chaos Monkey / LitmusChaos):
  - Runs daily in staging environment
  - Weekly in production during maintenance windows
  - Metrics: MTTR, error rate during chaos, cascading failures
```

---

## 15. Security Isolation Between Tenants

### Why It's Needed

A security breach in one tenant's data must never expose another tenant's data. This is the most critical non-functional requirement for a multi-tenant security product — a violation destroys trust in the entire platform.

### Isolation Layers

**Layer 1: Application-Level Tenant Filtering**

Every database query is wrapped in a mandatory tenant filter enforced at the ORM/query builder level:

```python
class TenantAwareQuerySet:
    def __init__(self, tenant_id: str):
        self._tenant_id = tenant_id
    
    def execute(self, query):
        # Mandatory tenant filter — cannot be bypassed
        if "WHERE" not in query.upper():
            raise SecurityException("Query must include WHERE clause")
        if f"tenant_id = '{self._tenant_id}'" not in query:
            raise SecurityException("Query must filter by tenant_id")
        return self._db.execute(query)
```

**Layer 2: Database Row-Level Security (PostgreSQL)**

```sql
-- Enable RLS on all tables
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Policy: every row must belong to the current tenant
CREATE POLICY tenant_isolation ON users
    USING (tenant_id = current_setting('app.current_tenant_id')::uuid);

-- Application sets tenant context at connection start
SET app.current_tenant_id = 'acme-corp-uuid';
-- All subsequent queries automatically filtered
```

**Layer 3: Kubernetes Network Policies**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: tenant-isolation
  namespace: tenant-acme-corp  # dedicated namespace per enterprise tenant
spec:
  podSelector: {}
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: api-gateway    # only gateway can ingress
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              name: shared-infrastructure  # only to shared services
```

**Layer 4: Cache Key Namespacing**

```
All Redis keys prefixed with tenant namespace:
  tenant:{tenant_id}:session:{session_id}
  tenant:{tenant_id}:roles:{user_id}
  tenant:{tenant_id}:config:*

Tenant A cannot access Tenant B's keys — different prefixes
Even a compromised service cannot read cross-tenant cache entries
without knowing the other tenant's ID (which is also access-controlled)
```

**Layer 5: Encryption at Rest with Per-Tenant Keys**

```
Key Management (AWS KMS / HashiCorp Vault):
  Each tenant has a unique data encryption key (DEK)
  DEKs are encrypted with a Key Encryption Key (KEK) per tenant
  KEKs stored in HSM-backed KMS

Encryption Flow:
  Write: plaintext → AES-256-GCM(DEK) → ciphertext → DB
  Read:  ciphertext → DB → AES-256-GCM decrypt(DEK) → plaintext

Key Rotation:
  DEKs rotated annually (or on tenant request)
  Old DEKs retained for 30 days for in-flight decryption
  Tenant offboarding: DEK deletion = cryptographic erasure
```

---

## 16. Kubernetes & Container Orchestration

### Why It's Needed

Managing 500+ microservice pods across 3 regions manually is operationally impossible. Kubernetes provides the automation layer for scheduling, scaling, health management, and zero-downtime deployments.

### Cluster Architecture

```
Per-Region K8s Cluster (EKS / GKE / AKS):
  
  Control Plane: 3 managed nodes (HA)
  
  Node Groups:
  ┌──────────────────────────────────────────────────┐
  │  auth-pool: m6i.2xlarge × 50 nodes              │
  │  • CPU optimized for JWT signing/validation      │
  │  • Node affinity: auth-service pods only         │
  ├──────────────────────────────────────────────────┤
  │  policy-pool: c6i.4xlarge × 20 nodes            │
  │  • High CPU for OPA policy evaluation            │
  ├──────────────────────────────────────────────────┤
  │  data-pipeline-pool: r6i.4xlarge × 30 nodes     │
  │  • Memory optimized for Kafka consumers/Flink    │
  ├──────────────────────────────────────────────────┤
  │  general-pool: m6i.xlarge × 100 nodes (Spot)    │
  │  • API services, background jobs                 │
  │  • Spot instances (60% cost savings)             │
  └──────────────────────────────────────────────────┘
```

### Pod Disruption Budgets

```yaml
# Never allow more than 20% of auth pods to be unavailable simultaneously
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: auth-service-pdb
spec:
  maxUnavailable: "20%"
  selector:
    matchLabels:
      app: auth-service
```

### Resource Quotas & Limits

```yaml
# Per-namespace quota (prevents resource starvation between services)
apiVersion: v1
kind: ResourceQuota
metadata:
  name: auth-namespace-quota
  namespace: auth-services
spec:
  hard:
    requests.cpu: "200"
    requests.memory: 400Gi
    limits.cpu: "400"
    limits.memory: 800Gi
    pods: "500"
---
# Per-pod limits (prevents single pod consuming node)
apiVersion: v1
kind: LimitRange
metadata:
  name: auth-pod-limits
spec:
  limits:
    - type: Container
      default:
        cpu: "2"
        memory: 4Gi
      defaultRequest:
        cpu: "500m"
        memory: 1Gi
```

### Rolling Deployments with Zero Downtime

```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: "25%"        # Spin up 25% extra pods before killing old ones
      maxUnavailable: "0%"   # Never reduce below desired replica count
  template:
    spec:
      containers:
        - name: auth-service
          readinessProbe:
            httpGet:
              path: /health/ready
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 5
            failureThreshold: 3
          lifecycle:
            preStop:
              exec:
                command: ["/bin/sh", "-c", "sleep 15"]  # Drain in-flight requests
```

---

## 17. CI/CD & Platform Engineering

### Why It's Needed

Deploying 50+ microservices across 3 regions multiple times per day requires automation. Manual deployments at this scale introduce human error and slow down iteration velocity.

### Pipeline Architecture

```
Developer Push
     │
     ▼
GitHub / GitLab (trunk-based development)
     │
     ▼
CI Pipeline (GitHub Actions / GitLab CI):
  ├── Unit tests (parallel, < 2 min)
  ├── Integration tests (Docker Compose, < 5 min)
  ├── Security scan (Trivy, Snyk, SonarQube)
  ├── Contract tests (Pact — API compatibility)
  ├── Build Docker image (buildkit, layer cache)
  └── Push to ECR / Artifact Registry
     │
     ▼
CD Pipeline (ArgoCD / Flux GitOps):
  ├── Deploy to staging (auto on merge to main)
  ├── Run E2E tests (Playwright / k6 load test)
  ├── Smoke tests against staging
  ├── Manual approval gate (for production)
  └── Progressive delivery to production:
        Stage 1: Deploy to us-east-1 (canary 5%)
        Stage 2: Monitor error rate + latency (15 min)
        Stage 3: Promote to 100% us-east-1
        Stage 4: Repeat for eu-west-1, ap-southeast-1
```

### Feature Flags for Safe Releases

```python
# LaunchDarkly / Unleash / custom flagsmith
if feature_flags.is_enabled("new_policy_engine_v2", tenant_id=tenant_id):
    result = policy_engine_v2.evaluate(request)
else:
    result = policy_engine_v1.evaluate(request)

# Rollout strategy:
# 0%  → internal testing
# 5%  → beta tenants
# 25% → Tier 3 tenants
# 75% → all tenants except Tier 1
# 100% → all tenants
# Flag killed → code cleaned up in next sprint
```

### Database Migration Safety

```
Migration Rules:
  1. Expand before contract: add new column before dropping old
  2. Always backward-compatible: old code must run against new schema
  3. Three-phase migrations:
     Phase 1: Add new column (nullable) — deploy
     Phase 2: Backfill + dual-write — deploy + verify
     Phase 3: Drop old column — deploy (after all traffic uses new column)
  4. Migration applied to all shards atomically via migration orchestrator
  5. Rollback: down migrations always provided and tested
```

---

## 18. Handling Sudden Traffic Spikes

### Why It's Needed

Global enterprises trigger authentication spikes predictably (start of business in each timezone) and unpredictably (product launches, M&A announcements, security incidents). The system must absorb 10× normal traffic within seconds.

### Spike Sources and Mitigation

```
Spike Type             Trigger                   Peak Multiplier   Mitigation
────────────────────   ──────────────────────    ───────────────   ─────────────────────────
Morning login wave     9 AM in each timezone     5×                Pre-scaling via KEDA cron
Tenant onboarding      100,000 user import       10× for tenant    Tenant-level throttling
Security incident      Force password reset all  50×               Async job queue
Product launch         External traffic spike    8×                CDN + HPA
Conference (OAuth)     SSO flow for event        15×               Dedicated auth pool
```

### Predictive Pre-Scaling

```python
# KEDA (Kubernetes Event Driven Autoscaling) cron-based pre-scaling
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
spec:
  triggers:
    - type: cron
      metadata:
        timezone: "America/New_York"
        start: "45 8 * * 1-5"   # 8:45 AM weekdays
        end:   "30 9 * * 1-5"   # 9:30 AM weekdays
        desiredReplicas: "200"   # Pre-scale before spike
    - type: kafka
      metadata:
        topic: auth.requests
        lagThreshold: "1000"     # Also scale on queue backlog
```

### Load Shedding Strategy

When the system approaches capacity, it must prioritize gracefully:

```
Priority Queue for Auth Requests:
  P0 (never shed): Security operations (password reset, suspicious login block)
  P1 (shed last):  Enterprise Tier 1 auth
  P2 (shed early): Tier 2 auth (degrade: allow cached sessions longer)
  P3 (shed first): Tier 3 new session creation
  P4 (always shed when overloaded): Non-auth API calls

Response when shedding:
  HTTP 503 with Retry-After: 5
  + X-Retry-Reason: capacity
  Client SDK auto-retries with exponential backoff + jitter
```

### Circuit Breaker Pattern

```
Auth Service → Policy Engine circuit breaker:

CLOSED (normal):  Requests flow through
  If error rate > 50% over 10s window:
    → OPEN

OPEN (failure):   Requests use cached policy decisions
  After 30 seconds:
    → HALF-OPEN

HALF-OPEN:        10% of requests routed to Policy Engine
  If successful:  → CLOSED
  If failing:     → OPEN

Policy during OPEN state:
  Use last-known-good cached policy (acceptable for < 30s degradation)
  Log "policy engine unavailable — using cached policy" warning
  Alert on-call if OPEN state persists > 60 seconds
```

---

## 19. Consistency vs. Availability Tradeoffs

### The CAP Theorem in Practice

Open Guard cannot be both perfectly consistent and perfectly available during a network partition. Different data types require different tradeoffs:

```
Data Type                  Consistency Model    Rationale
─────────────────────────  ─────────────────    ─────────────────────────────────────
User authentication        Strong (CP)          Wrong password should never succeed
Session validity           Eventual (AP)        Stale session ok for 500ms
Permission decisions       Eventual (AP)        Cached ok for 60s (TTL-based)
Policy changes             Strong (CP)          Security policy must not be stale
Audit log ordering         Causal consistency   Events within a session are ordered
Account lockout state      Strong (CP)          Cannot allow login after lockout
Rate limit counters        Eventual (AP)        Over-counting by 10% acceptable
Feature flag state         Eventual (AP)        100ms stale acceptable
Cross-region replication   Eventual (AP)        500ms lag acceptable
Billing/quota enforcement  Strong (CP)          Cannot allow over-quota usage
```

### Implementation of Different Consistency Levels

```
Strong Consistency (synchronous replication):
  Used for: account lockout, critical policy changes
  Implementation: CockroachDB with serializable isolation
  Cost: Higher latency (cross-region write: 100–200ms)
  Example: Account lockout written to primary, read from primary only

Causal Consistency (monotonic reads):
  Used for: user-visible operations (create user → immediately readable)
  Implementation: CockroachDB follower reads with AS OF SYSTEM TIME
  Cost: Slightly stale reads (bounded: < 10s)
  Example: User creates resource, immediately queries it — must see it

Eventual Consistency (async replication):
  Used for: permission caches, session data, rate limit counters
  Implementation: Redis with async Kafka-based propagation
  Cost: Stale reads for TTL period
  Example: RBAC role change propagates within 60s — acceptable for most use cases
```

### Practical Consistency Budget

```
For a typical API request, latency budget allocation:
  Network (edge to region):    5ms
  Gateway processing:          2ms
  Auth token validation:       1ms   (local, no DB)
  Permission check (cache):    1ms   (Redis)
  Business logic:              10ms
  DB read (if needed):         5ms
  Response serialization:      1ms
  ──────────────────────────────────
  Total budget:               25ms  (p99 target: 50ms)

Consistency choices that blow the budget:
  Permission check via DB:  +15ms  → use cache (eventual consistency)
  Cross-region write:       +150ms → use async replication
  Strong read from replica: +20ms  → accept stale read for non-critical data
```

---

## 20. Request Flow Under Heavy Load

### Scenario: 500,000 Login Requests/Second (Peak)

```
Step 1: Edge Layer (Cloudflare, 200+ PoPs)
────────────────────────────────────────────
  • 500,000 req/s distributed across PoPs
  • Rate limit check at edge: 450,000 pass, 50,000 rate-limited (HTTP 429)
  • WAF: 1,000 blocked (known malicious IPs)
  • Latency added: < 5ms

Step 2: Regional API Gateway (Kong, 30 pods per region)
────────────────────────────────────────────────────────
  • 150,000 req/s to us-east-1 gateway
  • JWT signature pre-validation (for refresh flows)
  • Tenant routing: read X-Tenant-ID, determine shard
  • Rate limit state check: Redis (0.5ms)
  • mTLS termination
  • Latency added: 2ms

Step 3: Auth Service (200 pods, stateless)
──────────────────────────────────────────
  • 750 req/s per pod (sustainable: 1,500 req/s per pod)
  • Validate credentials against bcrypt hash (from Redis cache)
  • L1 cache miss → Redis: check if user exists (0.3ms)
  • DB hit (if cache miss, 5%): 4ms on shard_017
  • Issue JWT (RS256 signing: 0.5ms)
  • Publish auth.events to Kafka (async, non-blocking)
  • Latency added: 10ms

Step 4: Session Creation (async, non-blocking for response)
────────────────────────────────────────────────────────────
  • Kafka consumer writes session to Redis (< 1ms, async)
  • Audit log written to Kafka audit.raw topic (async)
  • Policy evaluation logged for compliance (async)

Step 5: Response
─────────────────
  • JWT access token (15 min TTL)
  • Refresh token (7 day TTL, stored in HttpOnly cookie)
  • HTTP 200, Total latency: ~22ms (p50), ~45ms (p99)

Step 6: Background Processing (Kafka Consumers)
────────────────────────────────────────────────
  • Audit event enriched with geo, risk score, device info
  • Written to ScyllaDB hot tier (batch, 10,000 events/batch)
  • Risk score computed: Kafka Streams anomaly detection
  • If risk > 60: publish to security.alerts topic → notify SIEM
```

### Load Distribution During Regional Failover

```
Normal Operation:
  us-east-1:     40% of traffic (Americas)
  eu-west-1:     35% of traffic (EMEA)
  ap-southeast:  25% of traffic (APAC)

us-east-1 Failure:
  T+0s:   us-east-1 unhealthy
  T+30s:  GeoDNS reroutes Americas traffic:
            eu-west-1: 35% → 70% (+100% surge)
            ap-southeast: 25% → 55% (+120% surge)
  T+45s:  HPA triggers: eu-west-1 scales from 200 → 380 pods
          ap-southeast scales from 100 → 200 pods
  T+3min: Stable at new configuration
  T+45min: us-east-1 recovers, traffic gradually returned
```

---

## 21. Technology Stack Summary

### Recommended Stack for Open Guard Enterprise

| Component | Technology | Alternatives | Rationale |
|---|---|---|---|
| **API Gateway** | Kong + Envoy | Nginx, AWS API GW | Plugin ecosystem, gRPC support, Lua scripting |
| **Service Mesh** | Istio | Linkerd, Consul Connect | mTLS, traffic management, observability |
| **Auth Framework** | Keycloak (extended) | Auth0, custom | OIDC/OAuth2 native, enterprise SSO |
| **Policy Engine** | OPA (Open Policy Agent) | Casbin, Cedar | Language-agnostic, bundle distribution |
| **Primary DB** | CockroachDB | Vitess/MySQL, Spanner | Distributed SQL, ACID, geo-partitioning |
| **Time-Series DB** | ScyllaDB | Cassandra | C++ rewrite, 10× throughput of Cassandra |
| **Analytics DB** | ClickHouse | BigQuery, Redshift | Fastest columnar for audit queries |
| **Cache** | Redis Cluster | Dragonfly, Memcached | Industry standard, Lua scripting, streams |
| **Message Queue** | Apache Kafka | Pulsar, Kinesis | Battle-tested, ecosystem, MirrorMaker |
| **Stream Processing** | Apache Flink | Kafka Streams, Spark | Stateful, exactly-once, event time |
| **Container Orchestration** | Kubernetes (EKS/GKE) | Nomad | Industry standard, ecosystem |
| **Autoscaling** | KEDA + HPA | VPA | Event-driven scaling beyond CPU/memory |
| **Service Discovery** | Consul | CoreDNS + K8s SVC | Cross-cluster, health checks |
| **Secrets Management** | HashiCorp Vault | AWS Secrets Manager | Dynamic secrets, PKI, audit |
| **Observability** | Grafana + Prometheus + Tempo + Loki | Datadog, New Relic | Open source, vendor-neutral |
| **Tracing** | OpenTelemetry + Jaeger | Zipkin, X-Ray | Standard protocol |
| **CDN/DDoS** | Cloudflare Enterprise | AWS Shield + CF | Best-in-class DDoS, Anycast |
| **CI/CD** | GitHub Actions + ArgoCD | GitLab CI + Flux | GitOps native |
| **Feature Flags** | LaunchDarkly | Unleash, Flagsmith | Enterprise, targeting rules |
| **Chaos Engineering** | LitmusChaos | Chaos Monkey | K8s-native |
| **Load Testing** | k6 | Gatling, Locust | JS scripting, CI integration |

### Scaling Milestones

```
Phase 1 (MVP): 1,000 tenants, 100K users
  Single region, PostgreSQL, Redis sentinel
  Cost: ~$15,000/month AWS
  Team: 5 engineers

Phase 2 (Growth): 10,000 tenants, 5M users
  3 regions, CockroachDB sharded (16 shards), Redis Cluster
  Kafka introduced for audit pipeline
  Cost: ~$80,000/month
  Team: 15 engineers

Phase 3 (Scale): 50,000 tenants, 50M users
  Full architecture as described in this document
  64 DB shards, OPA policy engine, full Flink pipeline
  Cost: ~$400,000/month
  Team: 40 engineers (including SRE)

Phase 4 (Hyperscale): 200,000 tenants, 500M users
  256 DB shards, Spanner-class globally distributed DB
  Regional Kafka clusters with MirrorMaker 2
  Custom ASIC-accelerated JWT validation at edge
  Cost: ~$2M/month
  Team: 150 engineers
```

---

## Key Architectural Patterns Used by Hyperscale SaaS

| Pattern | Description | Used In |
|---|---|---|
| **CQRS** | Separate read and write models | Audit log queries vs. writes |
| **Event Sourcing** | State as ordered event log | Policy change history |
| **Saga Pattern** | Distributed transactions without 2PC | User onboarding workflow |
| **Bulkhead** | Isolate tenant pools to prevent cascade failure | Tier 1 compute isolation |
| **Strangler Fig** | Gradually replace monolithic components | Auth service extraction |
| **Sidecar** | Deploy cross-cutting concerns alongside service | PgBouncer, Envoy, log shipper |
| **Ambassador** | Proxy that handles external calls on service's behalf | API gateway per service |
| **Competing Consumers** | Multiple consumers from same Kafka topic | Audit processing parallelism |
| **Claim Check** | Large messages stored externally, reference passed | Audit events with large payloads |
| **Backpressure** | Slow consumers signal producers to slow down | Flink → ScyllaDB write path |
| **Scatter-Gather** | Fan-out to shards, aggregate results | Cross-tenant reporting |
| **Two-Phase Commit** | Atomic cross-shard writes (avoided where possible) | Critical financial operations |

---