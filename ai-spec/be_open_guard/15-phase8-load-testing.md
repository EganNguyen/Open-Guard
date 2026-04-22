# §16 — Phase 8: Load Testing & Performance Tuning

---

## 16.1 k6 Test Scripts

**`auth.js`** — OIDC token endpoint throughput:
```js
export const options = {
    stages: [
        { duration: '1m', target: 500 },
        { duration: '3m', target: 2000 },
        { duration: '1m', target: 0 },
    ],
    thresholds: {
        'http_req_duration': ['p(99)<150'],
        'http_req_failed': ['rate<0.01'],
    },
};
// POST /oauth/token with grant_type=password
// Pre-seeded users: 10k users across 100 orgs
```

**`policy-evaluate.js`** — policy evaluation:
```js
// Scenario 1: repeated inputs → Redis cache hits → p99 < 5ms
// Scenario 2: unique resource per VU → cache misses → p99 < 30ms
// Total: 10,000 req/s
// SDK local cache verification: second call from same VU produces no new spans
```

**`event-ingest.js`** — event push throughput:
```js
// POST /v1/events/ingest with batch of 10 events per request
// 2,000 req/s = 20,000 events/s
// p99 < 50ms
// Post-run: verify all events in audit log within 5s
```

**`audit-query.js`** — read path:
```js
// GET /audit/events with various filter combinations
// 1,000 req/s, p99 < 100ms
// Verify MongoDB readPreference=secondaryPreferred via explain()
```

**`kafka-throughput.js`** — event bus capacity:
```js
// xk6-kafka extension; direct Kafka producer
// 50,000 events/s to audit.trail
// Monitor: openguard_kafka_consumer_lag must stay < 10,000
```

---

## 16.2 Tuning Table

| SLO failing | Probable cause | Action |
|---|---|---|
| Login p99 > 150ms | bcrypt CPU-bound under load | Add IAM replicas |
| Policy p99 > 30ms (uncached) | Cold DB query | Ensure indexes on `policies(org_id, resource, action)` |
| Policy p99 > 5ms (cached) | Redis latency | Tune `REDIS_POOL_SIZE` |
| Event ingest p99 > 50ms | Outbox write contention | Increase control-plane replicas; tune `POSTGRES_POOL_MAX_CONNS` |
| Audit query p99 > 100ms | Missing MongoDB index | Run `explain()`, add compound index |
| Kafka consumer lag growing | Bulk writer too slow | Increase `AUDIT_BULK_INSERT_MAX_DOCS` |
| Connector auth p99 > 5ms (cached) | Redis pool exhausted | Increase `REDIS_POOL_SIZE` |
| Webhook delivery backlog | Delivery service under-scaled | Increase `webhook-delivery` replicas |
| MongoDB OOM | Bulk write buffer too large | Reduce `AUDIT_BULK_INSERT_MAX_DOCS` |
| Compliance report overcounts events | No `FINAL` modifier | Add `FINAL` to all `SELECT ... FROM events` compliance queries (§14.2) |

---

## 16.3 Phase 8 Acceptance Criteria

- [ ] `auth.js`: p99 < 150ms at 2,000 req/s, error rate < 1%.
- [ ] `policy-evaluate.js`: p99 < 5ms (Redis cached), p99 < 30ms (uncached) at 10,000 req/s.
- [ ] SDK local cache: second call produces 0 spans to policy service (Jaeger verification).
- [ ] `event-ingest.js`: p99 < 50ms at 20,000 req/s. All events in audit within 5s.
- [ ] `audit-query.js`: p99 < 100ms at 1,000 req/s.
- [ ] Kafka consumer lag < 10,000 during 50,000 events/s burst.
- [ ] Connector auth p99 < 5ms (Redis cached) at 20,000 req/s.
- [ ] All k6 HTML reports committed to `loadtest/results/`.
- [ ] Grafana screenshots showing all SLOs met under load committed to `docs/`.
