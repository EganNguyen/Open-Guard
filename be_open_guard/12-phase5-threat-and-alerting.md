# §13 — Phase 5: Threat Detection & Alerting

**Goal:** Real-time detection via Redis-backed counters. Composite risk scoring. Saga-based alert lifecycle. SIEM payloads signed with HMAC and replay-protected.

---

## 13.1 Threat Detectors

All detectors consume from `TopicAuthEvents`, `TopicPolicyChanges`, or `TopicConnectorEvents`. Each maintains state in Redis.

| Detector | Signal | Threshold | Risk Score |
|---|---|---|---|
| Brute force | `auth.login.failure` for same `email` within window | `THREAT_MAX_FAILED_LOGINS` in `THREAT_ANOMALY_WINDOW_MINUTES` | 0.8 |
| Impossible travel | Login from IP1 then IP2, distance > `THREAT_GEO_CHANGE_THRESHOLD_KM` within 1hr | Physical impossibility | 0.9 |
| Off-hours access | Login outside 06:00–22:00 org local time for 3+ consecutive days previously all in-hours | Historical pattern deviation | 0.5 |
| Data exfiltration | `data.access` count for single user exceeds org baseline by 3σ within 1hr | Statistical anomaly | 0.7 |
| Account takeover (ATO) | Login from new device fingerprint within 24hr of password change | New device + recent credential change | 0.7 |
| Privilege escalation | `policy.changes` with `role.grant` for user who logged in within 60min | Login → immediate admin grant | 0.9 |

**Composite scoring:** `max(individual_scores)` weighted by recency. Score ≥ 0.5 → alert. Score ≥ 0.8 → HIGH. Score ≥ 0.95 → CRITICAL.

---

## 13.2 Alert Lifecycle Saga

```
threat.alert.created   →  Step 1: persist alert in MongoDB
                       →  Step 2: enqueue notification (notifications.outbound)
                       →  Step 3: fire SIEM webhook (if configured)
                       →  Step 4: write audit event (audit.trail)
threat.alert.acknowledged → update alert status, write audit event
threat.alert.resolved  → update status, compute MTTR, write audit event
```

MTTR (mean time to resolve) is tracked per org per severity.

---

## 13.3 SIEM Webhook Signing and Replay Protection

Every SIEM webhook POST includes:
```
X-OpenGuard-Signature: sha256=<hmac-sha256-hex>
X-OpenGuard-Delivery: <uuid>
X-OpenGuard-Timestamp: <unix seconds>
```

HMAC is computed over `"<timestamp>.<payload_bytes>"` using `ALERTING_SIEM_WEBHOOK_HMAC_SECRET`.

**Replay protection:** Reject requests where `abs(now - timestamp) > ALERTING_SIEM_REPLAY_TOLERANCE_SECONDS` (default 300s).

**SSRF protection:** Outgoing SIEM webhook URLs are validated at startup and on update. Must be HTTPS. Must not resolve to RFC 1918 / loopback addresses.

---

## 13.4 Threat & Alerting API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/threats/alerts` | List alerts (status, severity filters, cursor paginated) |
| `GET` | `/v1/threats/alerts/:id` | Alert detail + saga step status |
| `POST` | `/v1/threats/alerts/:id/acknowledge` | Mark acknowledged |
| `POST` | `/v1/threats/alerts/:id/resolve` | Mark resolved (computes MTTR) |
| `GET` | `/v1/threats/stats` | Alert counts and MTTR |
| `GET` | `/v1/threats/detectors` | Active detectors and weights |

---

## 13.5 Phase 5 Acceptance Criteria

- [ ] 11 failed logins within window → HIGH alert in MongoDB within 5s.
- [ ] Privilege escalation detector fires within 5s of role grant event.
- [ ] SIEM webhook includes valid HMAC signature. Receiver can verify.
- [ ] Webhook with timestamp 6 minutes old → rejected (replay protection).
- [ ] Alert saga: all 4 steps produce audit events in `audit.trail`.
- [ ] MTTR computed correctly on resolution.
- [ ] ATO detector fires when login from new device follows password change within 24h.
- [ ] SSRF: SIEM URL `http://169.254.169.254/latest/meta-data/` rejected at configuration time.
