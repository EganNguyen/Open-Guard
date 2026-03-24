# OpenGuard SDK Pipeline & Showcase

This directory contains an interactive showcase of **OpenGuard**'s protection pipeline using the modern **Control Plane SDK** architecture. It demonstrates how connected applications use OpenGuard for centralized security governance (authentication, authorization, rate limiting, and audit logging) without putting OpenGuard inline as a reverse proxy.

## The SDK & Control Plane Pipeline

When you integrate OpenGuard into your application using the SDK, the request flow is:

1. **User Request**: A client application sends an HTTP request directly to your public-facing application endpoint.
2. **App SDK Intercept**: The OpenGuard SDK middleware running inside your app intercepts the request.
   - **Rate Limiting**: Checks local token buckets (synced with Control Plane) to deter spam.
   - **Authentication**: Validates the incoming JWT locally against JWKS provided by the IAM service.
3. **Control Plane Governance**: If valid, the SDK makes an API call (`POST /v1/policy/evaluate`) to the OpenGuard Control Plane.
   - The Control Plane checks the **Policy Engine** to ensure the specific user has permissions/roles to perform the action.
4. **App Logic**: The Control Plane responds `{"permitted": true}`. The SDK allows the request to reach your application's actual business logic.
5. **Audit Logging**: The SDK emits an `EventEnvelope` (`POST /v1/events/ingest`) back to the Control Plane asynchronously. The Control Plane normalizes it and relays it into the immutable Audit Service via Kafka outbox.

## Integration Guide

To protect your main product with OpenGuard:

### 1. Install & Configure the SDK
Import the OpenGuard SDK into your application (e.g., Go, Node.js). Initialize it with your **Connector API Key** and the URL of the OpenGuard Control Plane.

### 2. Wrap Your Endpoints
Apply the SDK middleware to your endpoints. Provide the required contextual attributes (Action, Resource, User).

### 3. Centralized Management
- Use the **IAM Service** to manage your identity pools and issue JSON Web Tokens (JWTs).
- Use the **OpenGuard Dashboard** to define your Role-Based Access Control (RBAC) rules. 

## Showcase Use Cases

To help you understand how OpenGuard protects your product at the edge, run the interactive visualization.

### Running the Showcase
1. Open `index.html` in any modern web browser (no build steps, Node.js, or servers required).
2. Interact with the control panel to simulate various network requests.

### Demonstrated Scenarios

- ✅ **Valid Request (Happy Path)**
  - A properly authenticated user. The SDK validates the JWT, the Control Plane confirms authorization, the business logic runs, and an audit log is emitted safely.
- ❌ **Unauthenticated (Blocked locally by SDK)**
  - A missing or expired token. The SDK drops the request safely on the application edge.
- ❌ **Unauthorized (Blocked by Control Plane Policy)**
  - A user with a valid token attempting an action without sufficient roles. The SDK asks the Control Plane, which returns a denial. The SDK issues a `403 Forbidden`.
- ❌ **Rate Limited (Blocked locally by SDK)**
  - A user exceeding limits. The SDK drops the request returning `429 Too Many Requests`.
- ❌ **Malicious Payload (Blocked locally by SDK)**
  - Signature-based threat detection within the SDK halts SQLi or XSS immediately.
