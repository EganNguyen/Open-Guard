# Threat Detection & Alerting Flows

## 1. Overview
This module covers behavioral analytics and security incident management.

## 2. Threat Detection

### TC-THR-001: Brute Force Detection
- **Steps:** Submit 10 failed logins within 60s.
- **Expected Results:** `type = 'brute_force'` alert in MongoDB.
- **System Verifications:** Kafka `auth.login_failed` -> Threat detector -> MongoDB insertion with `severity=HIGH`.

### TC-THR-002: Impossible Travel Detection
- **Steps:** Login from NYC, then from Tokyo 5 mins later.
- **System Verifications:** Haversine distance calculation (> 500km) + Time delta check; Alert created in MongoDB with distance metadata.

### TC-THR-003: Off-Hours Access Detection
- **Steps:** Login at 3 AM UTC (outside 09:00-18:00 window).
- **System Verifications:** Server-side timestamp validation against business hours.

### TC-THR-004: Privilege Escalation Detection
- **Steps:** Non-admin user granted broad permissions via policy change.
- **System Verifications:** Delta analysis on `policy.changes` Kafka events.

### TC-THR-005: Data Exfiltration Detection
- **Steps:** Simulate 50 data access events for the same user within 10 minutes totaling > exfiltration threshold.
- **System Verifications:** Redis sliding window counter; `threat.alerts` Kafka event; MongoDB alert record with volume metadata.

### TC-THR-006: Account Takeover (ATO) Detection
- **Steps:** 1. `POST /auth/password/change`. 2. `POST /auth/login` from a new device fingerprint within 24 hours.
- **System Verifications:** Detector identifies high-risk sequence (password change + new device); risk score 0.7; Alert type `account_takeover`.

## 3. Alerting Management

### TC-ALT-001: List and Filter Alerts
- **Steps:** `GET /v1/threats/alerts?status=open&severity=high`.
- **System Verifications:** MongoDB compound index query; Cursor-based pagination.

### TC-ALT-002: Acknowledge and Resolve Alert
- **Steps:** `POST /v1/threats/alerts/{id}/acknowledge`.
- **Expected Results:** Status transitions to `acknowledged` then `resolved`.
