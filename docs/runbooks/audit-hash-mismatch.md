# Runbook: audit-hash-mismatch

## Symptoms
- Prometheus alert `AuditChainIntegrityFailure` is firing.
- `GET /v1/audit/integrity` returns `ok: false`.
- High-severity alert `auth.audit.chain_broken` visible in dashboard.

## Troubleshooting Steps

1. **Identify Mismatch Point:**
   The integrity API response contains the `first_mismatched_sequence`.
   ```bash
   curl -s http://audit-service:8080/v1/audit/integrity | jq .
   ```

2. **Inspect Records at Boundary:**
   Query MongoDB for the record at the mismatch sequence and the one immediately preceding it:
   ```javascript
   // In Mongo Shell
   db.audit_events.find({sequence: {$in: [SEQ, SEQ-1]}}).sort({sequence: 1})
   ```

3. **Verify Hashes Manually:**
   Compute the expected hash for sequence `N` using the payload of `N` and the hash of `N-1`.
   Rationale: `hash(payload | prev_hash)`.

## Root Causes
- **Unauthorized Deletion:** A database administrator manually deleted a row from MongoDB.
- **Unauthorized Modification:** A field in an existing audit record was changed (tampered with).
- **Relay Logic Error:** A bug in the outbox relay hashing logic during a version upgrade.
- **Secret Key Rotation:** `AUDIT_SECRET_KEY` was rotated without re-keying historical data.

## Remediation

1. **Verify Secret:** Ensure the `AUDIT_SECRET_KEY` in env matches the one used to sign the records.
2. **Seal Chain:** If the gap is due to intentional deletion (e.g., GDPR right-to-be-forgotten), a new "Genesis Record" must be inserted at the gap to re-start the chain.
3. **Investigation:** Treat any unexplained mismatch as a **Security Incident**. Audit database access logs to identify who or what modified the production data.
