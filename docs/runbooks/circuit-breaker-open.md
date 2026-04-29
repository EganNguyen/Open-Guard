# Runbook: circuit-breaker-open

## Symptoms
- Dashboard shows `503 Service Unavailable` with code `CAPACITY_EXCEEDED` or `CIRCUIT_OPEN`.
- Prometheus alert `CircuitBreakerOpen` is firing.
- Logs show `circuit breaker state changed: closed -> open`.

## Troubleshooting Steps

1. **Identify the Breaker:**
   Check the `name` label in Prometheus or logs. Common breakers:
   - `cb-policy`: Policy service calls failing.
   - `iam-redis`: IAM cannot reach Redis.
   - `audit-mongo`: Audit consumer cannot reach MongoDB.

2. **Check Downstream Health:**
   Check the health of the dependency causing the trip:
   ```bash
   # If cb-policy is open
   curl http://policy-service:8080/health
   ```

3. **Analyze Error Rates:**
   Look for the specific error causing the trip in the logs:
   ```bash
   kubectl logs -l app=iam-service | grep "circuit breaker" -A 5
   ```

## Remediation

1. **Address Upstream Failure:**
   - Scale up the downstream service if it's CPU bound.
   - Fix network/DNS issues if connection is refused.
   - Check downstream database locks or performance.

2. **Wait for Half-Open:**
   Our breakers automatically move to `half-open` after 30 seconds. If the downstream is healthy, the next few requests will close the breaker.

3. **Manual Reset (Force Close):**
   If the downstream is verified healthy but the breaker is stuck, restart the service pod to reset the in-memory breaker state.
   ```bash
   kubectl rollout restart deployment/iam-service
   ```
