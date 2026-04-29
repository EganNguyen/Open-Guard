# Intent Map (Decision Log)

## 1. Why MongoDB *and* ClickHouse?
- **MongoDB:** Chosen for the **Audit Service** (Audit Trail). It allows for rapid ingestion of schema-less JSON event data and efficient point-queries for specific users/resources. It is the "Operational Store" for security events.
- **ClickHouse:** Chosen for the **Compliance Service**. It is optimized for "Aggregated Reports" and multi-billion-row analytics. It allows for fast calculation of compliance scores and long-term trend analysis.

## 2. Why RLS (Row-Level Security)?
- **Alternative:** App-layer filtering (`WHERE org_id = ?`).
- **Intent:** Security-in-depth. By enforcing RLS at the database level, we prevent "cross-tenant leak" bugs even if the application code has a logic error. It ensures "Fail-Safe" multi-tenancy.

## 3. Why mTLS?
- **Alternative:** Bearer tokens or internal API Keys.
- **Intent:** Zero-Trust networking. Even if a container is compromised within the cluster, the attacker cannot spoof another service without the unique client certificate and private key.

## 4. Why the 60s SDK TTL?
- **Intent:** To balance "Security Freshness" with "Availability". A shorter TTL would increase load on the control plane; a longer TTL would leave a larger window for unauthorized access if a user's rights are revoked. 60s is the project's optimal trade-off for high-performance security.

## 5. Why Angular Signals?
- **Alternative:** BehaviorSubject / Observables.
- **Intent:** To leverage Angular 19's fine-grained reactivity. Signals reduce unnecessary change detection cycles, resulting in a more performant dashboard when handling high-volume real-time alert streams (SSE).

## 6. Why Constant-Time Login?
- **Intent:** Protect against account enumeration via timing side-channels. By running bcrypt even on user-not-found, we eliminate the measurable ~350ms difference that reveals email existence.

## 7. Why Redis SetNX for MFA Replay?
- **Intent:** Prevent reuse of single-use codes (TOTP/Backup) and SAML assertions within their validity window. A simple check-and-set isn't enough; atomic SetNX ensures a code can never be used twice.

## 8. Why SSE org_id Derivation from JWT?
- **Intent:** Prevent IDOR (Insecure Direct Object Reference) attacks on real-time streams. By deriving the tenant ID from the authenticated session rather than a URL parameter, we ensure users can only ever see events belonging to their own organization.
