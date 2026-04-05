# §12 — Observability & Admin

Visible only to users with the `admin` role. Surfaces system health metrics that mirror the Prometheus alerts defined in BE spec §9.4.

---

## 12.1 System Health Page

```
Route: /admin/system
```

**Service status grid:**

```tsx
// One status card per service, 3-column grid
// Data from GET /admin/system/status (aggregates health check results)
//
// Card layout:
// ┌───────────────────────────────┐
// │ ● IAM                 HEALTHY │
// │ Uptime: 99.97%                │
// │ p99: 42ms (last 5min)         │
// │ Last check: 8s ago            │
// └───────────────────────────────┘
//
// Status colors: HEALTHY=green, DEGRADED=amber (some checks failing), DOWN=red
//
// Services shown:
// IAM, Control Plane, Policy, Threat, Audit, Alerting,
// Webhook Delivery, Compliance, DLP, Connector Registry

// Dependencies shown separately:
// PostgreSQL, MongoDB (Primary + Secondary), Redis, Kafka, ClickHouse
```

**Auto-refresh:** every 30 seconds. SSE not needed — polling is sufficient for admin use.

---

## 12.2 Circuit Breaker Status Panel

```tsx
// components/domain/circuit-breaker-panel.tsx
//
// Shows current state of all circuit breakers (from openguard_circuit_breaker_state metric)
//
// Circuit breakers:
//   cb-policy     CLOSED   ● (green)    consecutive_failures: 0
//   cb-iam        CLOSED   ● (green)    consecutive_failures: 0
//   cb-dlp        CLOSED   ● (green)    consecutive_failures: 0
//   cb-audit      OPEN     ● (red, animated)  opened: 2 minutes ago
//                          → "Policy evaluations will deny after SDK cache TTL expires."
//
// State colors: CLOSED=green | HALF_OPEN=amber (pulse) | OPEN=red (pulse)
// Numeric state: 0=closed, 1=half-open, 2=open (per BE spec §9.3)
//
// When OPEN: show the impact description (from BE spec §8.3 failure mode table):
//   cb-policy OPEN → "SDK uses cached decisions. After 60s TTL: all evaluations denied."
//   cb-iam OPEN    → "All login attempts are being rejected (503)."
```

---

## 12.3 Outbox Lag Gauge

```tsx
// components/domain/outbox-lag-gauge.tsx
//
// Gauge chart per service showing openguard_outbox_pending_records
//
// Services with outbox: IAM, Control Plane, Policy, Threat, Alerting
//
// ┌─────────────────────────┐
// │ IAM Outbox              │
// │  Pending: 12 records    │
// │  [██░░░░░░░░] 12/1000   │ ← 1000 = OutboxLagHigh threshold
// │  Relay: 100ms poll      │
// └─────────────────────────┘
//
// Color: 0-100=green, 100-500=amber, 500+=red
// Alert link: if pending > 1000, show "⚠ OutboxLagHigh alert active" with runbook link
//             → docs/runbooks/outbox-dlq.md
```

---

## 12.4 Kafka Consumer Lag Charts

```tsx
// components/domain/kafka-lag-chart.tsx
//
// LineChart per consumer group showing openguard_kafka_consumer_lag
//
// Groups shown (from BE spec §4.4):
//   openguard-audit-v1      → topic: audit.trail
//   openguard-threat-v1     → topic: auth.events, connector.events
//   openguard-alerting-v1   → topic: threat.alerts
//   openguard-compliance-v1 → topic: audit.trail
//   openguard-policy-v1     → topic: policy.changes
//   openguard-webhook-delivery-v1 → topic: webhook.delivery
//
// Chart: last 1 hour, 1-minute resolution
// Threshold line at 50,000 (KafkaConsumerLagHigh alert threshold per BE spec §9.4)
//
// If lag > 50,000: amber warning + runbook link (docs/runbooks/kafka-consumer-lag.md)
```

---

## 12.5 DLQ Inspector

```
Route: /admin/system → "Dead Letter Queue" tab
```

The DLQ inspector (matches BE spec §10.5 CLI tool, but as a UI):

**DLQ topics:** `outbox.dlq` | `webhook.dlq`

**Tabs per topic:**

**Outbox DLQ table:**

| Column | Data |
|---|---|
| Message ID | UUID (monospace) |
| Org | org_id |
| Topic | Target topic |
| Payload | Truncated JSON |
| Attempts | N |
| Dead at | `<TimeAgo>` |
| Actions | Replay / Discard |

**Replay action:**
```tsx
// "Replay" → ConfirmDialog:
// "Replay this message to [target_topic]?
//  This will re-publish the message to the Kafka topic.
//  The consumer must be idempotent (event_id dedup) to handle this safely."
// [Cancel]  [Replay]
//
// Calls POST /admin/dlq/:topic/:id/replay
// On success: message removed from DLQ table.
```

**Webhook DLQ table:** Similar structure. Replay re-queues the delivery attempt.

---

## 12.6 Audit Hash Chain Integrity Report

```
Route: /audit/integrity-report (linked from integrity badge when failures detected)
```

Triggered when `GET /audit/integrity` returns `ok: false`.

```
Hash Chain Integrity Report
Generated: 2024-01-15 14:30:00 UTC

Status: ⚠ INTEGRITY FAILURE DETECTED

Affected organization: Acme Corp (org_01j...)
Gap detected:
  Expected chain_seq: 4820
  Found chain_seq:    4821
  Missing sequence:   4820

  Last valid event:
    chain_seq:  4819
    event_id:   evt_01j...
    chain_hash: a3f8...d912
    time:       2024-01-15 14:22:58 UTC

  Next event after gap:
    chain_seq:  4821
    event_id:   evt_01j...
    prev_hash:  a3f8...d912 ← matches last valid (gap, not corruption)
    time:       2024-01-15 14:23:07 UTC

Gap interpretation:
  A single event is missing from the chain. This may indicate:
  - A MongoDB write that was lost during a primary failover (expected during DR)
  - Unauthorized deletion of an audit record (security incident)

Next steps:
  1. Check MongoDB failover logs for the time of the gap.
  2. Check the audit export for the missing 9-second window.
  3. Escalate to security team if no infrastructure event explains the gap.

  Runbook: docs/runbooks/audit-hash-mismatch.md   [Open]
```

---

## 12.7 Connection Pool Status

```tsx
// Simple table showing pool utilization per service + DB
// Data from GET /admin/system/pools
//
// Service      | DB          | Active | Idle | Max | Utilization
// IAM          | PostgreSQL  | 12     | 13   | 25  | 48%
// Audit (write)| MongoDB     | 3      | 7    | 10  | 30%
// etc.
//
// Amber warning at >80% utilization.
// Red at >95% (approaching pool exhaustion).
```
