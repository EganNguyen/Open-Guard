# Runbook: CA Certificate Rotation

This runbook follows the strict procedure defined in Spec §2.9 for mTLS trust.

## 1. Phase 1: Dual Trust
1. Generate New CA (Root + Intermediate).
2. Update all services' `ca.crt` (Secrets Manager: `IAM_CA_BUNDLE`) to include BOTH old and new certificates.
3. Deploy services (rolling update).
4. **Verification**: Services can still talk to each other using old certificates.

## 2. Phase 2: Issue New Leaf Certs
1. Issue new client/server certificates for all services using the New CA.
2. Update service secrets (`tls.crt`, `tls.key`).
3. Deploy services (rolling update).
4. **Verification**: Services are now presenting new certificates, trusted via the dual-trust bundle.

## 3. Phase 3: Cleanup
1. Remove the Old CA from the `ca.crt` bundle.
2. Deploy services.
3. **Result**: Old certificates are now rejected.
