# System Understanding, User Flows, and Function Flows

This document provides a structured foundation of the Open-Guard system's behavior, mapping user journeys and internal system flows.

## 1. System Understanding

Open-Guard is a high-performance security control plane designed with a "beside, not in front" architecture, ensuring security enforcement without adding latency to the critical application path.

### Architecture Overview
*   **Frontend**: Angular 19 Admin Dashboard using Tailwind CSS, Chart.js, and Angular Signals for reactive state management.
*   **Backend**: Go 1.22+ microservices communicating via **mTLS**. Key services include:
    *   **IAM**: Identity and Access Management (Auth, MFA, OAuth, SAML, SCIM).
    *   **Policy**: Policy engine for CRUD and evaluation of security rules.
    *   **Control Plane**: High-availability entry point for SDKs to check access.
    *   **Threat/Alerting**: Real-time threat detection and notification delivery.
    *   **Audit/Compliance**: Exactly-once audit logging and compliance reporting.
*   **Databases**:
    *   **PostgreSQL**: Primary source of truth for IAM and Policies (uses Row-Level Security).
    *   **Redis**: High-speed cache for policies and rate-limiting.
    *   **MongoDB**: Flexible storage for recent audit trails and alert metadata.
    *   **ClickHouse**: High-volume security analytics and compliance history.
*   **Event Infrastructure**: **Kafka** serves as the backbone for asynchronous, reliable event delivery using the **Transactional Outbox** pattern.

### Key Components & Responsibilities
| Component | Responsibility |
| :--- | :--- |
| **Fail-Closed SDK** | Enforces policies within applications. Denies access if the control plane is unreachable for >60s. |
| **Policy Engine** | Evaluates JSON-based logic to make allow/deny decisions. |
| **Transactional Outbox** | Ensures exactly-once delivery of audit logs by coupling DB changes with event publishing. |
| **mTLS Middleware** | Enforces service-to-service authentication and redacts sensitive logs. |

---

# User Flows

## 1. Authentication and Identity Management

### 1.1 Standard Login with MFA
Primary journey for administrative access to the dashboard.
*   **Step 1 (UI → API)**: User enters email and password. Dashboard calls `POST /auth/login` on IAM service.
*   **Step 2 (System)**: IAM validates credentials in PostgreSQL.
*   **Step 3 (Response)**: 
    *   If MFA enabled: Returns `202 Accepted` with an `mfa_challenge`.
    *   If MFA disabled: Returns `200 OK` with a JWT session cookie.
*   **Step 4 (UI → API)**: User enters TOTP code or uses WebAuthn. Dashboard calls `POST /auth/mfa/verify`.
*   **Step 5 (Response)**: Returns `200 OK` and sets the `openguard_session` HttpOnly cookie.
*   **Alternative Path**: User uses a backup code via `POST /auth/mfa/backup-verify` if they lost their TOTP device.
*   **Failure Scenario**: Invalid password returns `401 Unauthorized`. Account setup pending returns `403 Forbidden`.

### 1.2 MFA Setup (TOTP)
*   **Step 1 (UI → API)**: User clicks "Enable TOTP". Dashboard calls `GET /auth/mfa/totp/setup`.
*   **Step 2 (System)**: IAM generates a secret and returns a QR code URL.
*   **Step 3 (UI → API)**: User scans QR and enters the code. Dashboard calls `POST /auth/mfa/totp/enable`.
*   **Step 4 (System)**: IAM validates code, enables MFA, and returns backup codes.
*   **Failure Scenario**: Invalid setup code returns `400 Bad Request`.

## 2. Security Policy Management

### 2.1 Policy Creation and Assignment
*   **Step 1 (UI → API)**: Admin creates a policy with JSON logic via `POST /v1/policies`.
*   **Step 2 (System)**: Policy Service validates JSON, persists to PG, and invalidates Redis cache.
*   **Step 3 (UI → API)**: Admin assigns policy to a user via `POST /v1/assignments`.
*   **Step 4 (System)**: Assignment is persisted. The user now has the policy applied.
*   **Edge Case**: Creating a policy with invalid JSON logic returns `400 Bad Request`.
*   **Failure Scenario**: Attempting to delete a policy that is still assigned returns a constraint error (handled by PG RLS/FK).

## 3. Real-time Monitoring

### 3.1 Threat Dashboard
*   **Step 1 (UI)**: User navigates to Threats page.
*   **Step 2 (System)**: Dashboard establishes an SSE (Server-Sent Events) connection to `ThreatService`.
*   **Step 3 (Flow)**: As Kafka events arrive (from Control Plane/Audit), Threat service processes them and pushes alerts to the UI.
*   **Edge Case**: Network disconnect results in UI showing "Offline" and attempting reconnection with exponential backoff.

---

# Function Flows

## 1. Policy Evaluation Flow (The "Hot Path")
Maps how an access request is decided by the system.

1.  **SDK Call**: Application SDK calls the **Control Plane** (or Policy Service directly) with `Subject`, `Action`, and `Resource`.
2.  **Context Injection**: Shared middleware extracts the `OrgID` from mTLS certificate or JWT and sets the PG session variable (`rls.SetSessionVar`).
3.  **Cache Check**: 
    *   Check **L1 (Local Memory)** via Singleflight.
    *   Check **L2 (Redis)**.
4.  **Database Fetch**: On cache miss, query **PostgreSQL** (filtered by RLS).
5.  **Logic Execution**: Engine evaluates the JSON policy against the request context.
6.  **Transactional Audit**: 
    *   In a single transaction: Write decision to `eval_logs` AND insert an event into the `outbox` table.
7.  **Response**: Return `Allow` or `Deny` to the SDK.
8.  **Async Delivery**: Outbox Relayer background worker detects the new `outbox` entry and publishes it to **Kafka**.

## 2. Event-Driven Audit & Compliance
How data moves across components after a change.

1.  **Event Trigger**: A policy is updated or an access is denied.
2.  **Local Persistence**: The source service (e.g., IAM or Policy) commits the change to its database.
3.  **Outbox Publishing**: The **Relayer** pushes the event to Kafka topic (e.g., `policy.changes` or `data.access`).
4.  **Downstream Ingestion**:
    *   **Audit Service**: Ingests from Kafka and saves to **MongoDB** for fast searching.
    *   **Compliance Service**: Ingests from Kafka and saves to **ClickHouse** for long-term reporting.
    *   **Threat Service**: Analyzes the event for anomalies (e.g., rapid failures) and triggers alerts.

## 3. Service-to-Service mTLS Handshake
1.  **Request Initiation**: Service A attempts to connect to Service B.
2.  **Certificate Exchange**: Both services present certificates from `/certs` (signed by internal Root CA).
3.  **Validation**: `shared/crypto` validates the chain and peer identity.
4.  **Session Setup**: Connection is established. Middleware injects `Correlation-ID` and `Identity-Context` for tracing.

## Critical Business Logic Paths
*   **Fail-Closed Enforcement**: Ensuring SDKs deny access if the control plane TTL expires.
*   **Exactly-Once Audit**: Ensuring no audit log is lost even if a service crashes after a DB commit.
*   **RLS Isolation**: Verification that `OrgID` cannot be spoofed to access another tenant's data.
