# Workflow Index

| Document | Level 2 Flows | Level 3 |
| :--- | :--- | :--- |
| [Control Plane Gateway](control-plane-gateway.md) | Request Lifecycle | State Transitions |
| [Authentication & IAM](authentication-iam.md) | Login, MFA, Token Refresh, Logout, JWT Key Rotation | State Transitions |
| [Policy Engine](policies-engine.md) | Policy Evaluation, Policy CRUD | State Transitions |
| [Threat Detection](threat-detection.md) | Detection Pipeline, Alert Saga | State Transitions |
| [Audit & Event Pipeline](audit-event-pipeline.md) | Event Ingest, Hash Chain, SSE Streaming | State Transitions |
| [Compliance & Reporting](compliance-reporting.md) | ClickHouse Ingestion, Report Generation | Compliance Scoring |
| [DLP Scanning](dlp-scanning.md) | Sync DLP (Block), Async DLP (Monitor) | Scanner Internals |
| [Notifications & Webhooks](webhooks-notifications.md) | Webhook Delivery Sequence | State Transitions |
| [Connector Registry](connector-registry.md) | Registration, API Key Validation, Suspend/Delete | Cache Internals, Lifecycle State Machine |
| [Alerting & SIEM](alerting-siem.md) | Alert Saga, SIEM Webhook Delivery, Alert Lifecycle | SIEM Formatting, Replay Protection, Retry Backoff |
