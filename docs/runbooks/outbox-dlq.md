# Runbook: outbox-dlq

## Symptoms
- Prometheus alert `OutboxLagHigh` is firing.
- `openguard_outbox_records_dead_total` metric is increasing.
- Events (Audit/Policy/Threat) are not appearing in their respective consumers.

## Troubleshooting Steps

1. **Check Relay Logs:**
   Identify which service is failing to publish:
   ```bash
   kubectl logs -l component=outbox-relay
   ```
   Look for `outbox relay: kafka publish failed` or `marking record as dead`.

2. **Inspect Dead Records:**
   Query the `outbox_records` table for records marked as `dead`:
   ```sql
   SELECT id, topic, last_error, attempts, created_at 
   FROM outbox_records 
   WHERE status = 'dead' 
   ORDER BY created_at DESC LIMIT 10;
   ```

3. **Identify Root Cause:**
   - **Kafka Down:** Ensure Kafka brokers are reachable and topics exist.
   - **Message Too Large:** Check `last_error` for `MessageSizeTooLarge` errors.
   - **Network Partition:** Verify mTLS connectivity between relay and Kafka.

## Remediation

1. **Fix Upstream:**
   Address the root cause identified in the troubleshooting phase (e.g., restart Kafka, increase topic partition count).

2. **Replay Dead Records:**
   Once the root cause is fixed, reset the status of dead records to `pending` to trigger a retry:
   ```sql
   UPDATE outbox_records 
   SET status = 'pending', attempts = 0 
   WHERE status = 'dead' AND created_at > NOW() - INTERVAL '24 hours';
   ```

3. **Monitor Drain:**
   Watch the pending count decrease:
   ```sql
   SELECT count(*) FROM outbox_records WHERE status = 'pending';
   ```
