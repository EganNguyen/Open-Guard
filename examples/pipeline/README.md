# OpenGuard Integration & Showcase

This directory contains a simple, interactive showcase of **OpenGuard**'s protection pipeline. It demonstrates how OpenGuard acts as a security gateway between your users and your main product, enforcing authentication, authorization, rate limiting, and audit logging.

## The OpenGuard Pipeline

When you integrate OpenGuard into your architecture, it sits at the edge of your network. The request flow is as follows:

1. **User Request**: A client application sends an HTTP request to an endpoint.
2. **OpenGuard Gateway**: The Gateway intercepts the request.
   - **Rate Limiting**: Checks if the user has exceeded their configured limits.
   - **Authentication (IAM)**: Validates the JWT, checks for token revocation, and identifies the user and tenant (`org_id`).
3. **Policy Engine**: The Gateway asks the Policy Engine if this specific user has permissions to perform the requested action on the target resource.
4. **Main Product (Upstream)**: If all checks pass, the Gateway forwards the request to your actual backend service (the Main Product).
5. **Audit Logging**: An `EventEnvelope` is asynchronously sent through the Transactional Outbox and Kafka relay to the Audit service to record the interaction for compliance.

## Integration Guide

To protect your main product with OpenGuard, follow these steps:

### 1. Route Traffic Through OpenGuard
Your main product should no longer be exposed directly to the public internet. Instead, route all external traffic to the OpenGuard Gateway. 
- The Gateway will handle TLS termination, rate limiting, and authentication.
- Configure the Gateway to proxy traffic to your internal service endpoints.

### 2. Configure IAM and Policies
- Use the **IAM Service** to manage users, organizations, and issue authentication tokens (JWTs).
- Use the **Policy Engine** to define Role-Based Access Control (RBAC) rules for your product's endpoints.

### 3. Trust the Gateway (Internal Network)
Configure your main product to **only** accept requests from the OpenGuard Gateway (e.g., via mTLS or a private VPC). 
- The Gateway will inject trusted headers (e.g., `X-User-ID`, `X-Org-ID`, `X-Request-ID`, `Traceparent`) into the forwarded request.
- Your main product can safely trust these headers to process the business logic without re-authenticating the user.

## Showcase Use Cases

To help you understand how OpenGuard protects your product, this examples folder contains an interactive visualization.

### Running the Showcase
1. Open `index.html` in any modern web browser (no build steps, Node.js, or servers required).
2. Interact with the control panel to simulate various network requests.

### Demonstrated Scenarios

- ✅ **Valid Request (Happy Path)**
  - A properly authenticated and authorized user. The request flows completely through to the Main Product, and an audit log is emitted.
- ❌ **Unauthenticated (Blocked by Gateway)**
  - A request with an invalid, missing, or expired token. The Gateway drops the request immediately with a `401 Unauthorized`.
- ❌ **Unauthorized (Blocked by Policy Engine)**
  - A user with a valid token attempting to access an endpoint without the proper roles. The Policy Engine denies the request and the Gateway returns `403 Forbidden`.
- ❌ **Rate Limited (Blocked by Gateway)**
  - A user making too many requests too quickly. The Gateway drops the request to protect your Main Product, returning `429 Too Many Requests`.
- ❌ **Malicious Payload (Threat Detection)**
  - A request containing potential SQL injection or XSS. The threat detection middleware blocks it before it reaches your product.

### Design Aesthetics
The showcase features a modern, dynamic UI with glassmorphism and subtle CSS keyframe animations to track the lifecycle of a request as it is processed by the OpenGuard modules.
