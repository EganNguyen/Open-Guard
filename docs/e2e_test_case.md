# OpenGuard E2E Lifecycle Test Case

This test case covers the end-to-end user lifecycle across all phases of the OpenGuard system implementation, ensuring architectural integrity, multi-tenancy isolation, and cross-service resilience.

## Overview
The lifecycle follows an organization named **"Acme"** from registration through connector management, policy enforcement, audit trail generation, and system recovery.

---

## Phase 1: Foundation & Onboarding
**Goal:** Verify organization registration and Identity Provider (IdP) functionality.

1.  **System Startup**
    - Run `make dev`.
    - Verify all services are healthy and migrations completed successfully.
2.  **Organization Registration**
    - Navigate to `/register` and create organization "Acme".
    - **Expect:** Org created, slugified to `acme`. Admin user created in IAM PostgreSQL.
3.  **Authentication (OIDC Flow)**
    - Perform OIDC login via the dashboard or `POST /oauth/token`.
    - **Expect:** Access Token (JWT) + Refresh Token issued. `kid` present in JWT header.
4.  **Connector Registration**
    - Navigate to `/dashboard/connectors` and register app "AcmeApp".
    - **Expect:** Connector registered. Plaintext API Key returned once and encrypted in Control Plane PostgreSQL.
    - **Verification:** `GET /v1/admin/connectors/:id` returns `status=active`.

---

## Phase 2: Policy Enforcement
**Goal:** Verify the Zero Trust model and policy evaluation performance.

5.  **Policy Creation**
    - Create an IP allowlist policy via the dashboard (Admin JWT required).
    - **Expect:** Policy record created for Org Acme.
6.  **Policy Evaluation (Direct Access)**
    - Call `/v1/policy/evaluate` using the AcmeApp API Key.
    - **Expect:** `permitted: false` for blocked IP; `permitted: true` for allowed.
7.  **Cache Verification (Phase 9)**
    - Repeat evaluation with identical inputs.
    - **Expect:** `permitted: true, cached: true` (Redis hit).
8.  **Scope Enforcement**
    - Register a second connector "AcmeApp2" with narrow scope (e.g., `audit:write`).
    - Attempt a policy evaluation with AcmeApp2 key.
    - **Expect:** `403 INSUFFICIENT_SCOPE`.

---

## Phase 3: Audit & Event Bus
**Goal:** Verify the Transactional Outbox pattern and audit integrity.

9.  **Event Ingestion**
    - Push a batch of 50 events via `POST /v1/events/ingest`.
    - **Expect:** `200 OK`, `accepted: 50`.
10. **Asynchronous Propagation**
    - Verify events appear in the Dashboard Audit log within 5s.
    - **Verification:** `EventSource` on each event is correctly attributed to "connector:<id>".
11. **Threat Detection Integration (Phase 4)**
    - Simulate 11 failed login events.
    - **Expect:** `HIGH` severity alert generated in MongoDB and visible in the Dashboard `Threats` section.
12. **Audit Integrity (Phase 3)**
    - Call `GET /audit/integrity`.
    - **Expect:** `ok: true`. The cryptographic chain (ClickHouse/MongoDB) is contiguous.

---

## Phase 4: Resilience & Failover
**Goal:** Verify "Fail Closed" behavior and data persistence under failure.

13. **Circuit Breaker Verification**
    - Kill the `policy` service container.
    - **Expect:** Dashboard or SDK falls back to local cache (60s grace).
    - **Expect:** Subsequent evaluations return `503 SERVICE_UNAVAILABLE` (fail-closed).
14. **Outbox Persistence**
    - Kill the `kafka` broker container.
    - Perform a connector registration or event ingestion.
    - **Expect:** `200 OK`. Records are safely buffered in the `outbox_records` table.
15. **System Recovery**
    - Restart Kafka.
    - **Expect:** Outbox Relay publishes buffered records within 30s. Audit logs reflect all events from the "outage".

---

## Performance Targets
- **Login:** p99 < 150ms @ 2,000 req/s.
- **Policy Eval:** p99 < 5ms (cached), < 30ms (uncached) @ 10,000 req/s.
- **Ingest:** p99 < 50ms @ 20,000 req/s.
