# Runbook: Secret Rotation

This document outlines the procedure for rotating critical secrets in the Open-Guard ecosystem.

## 1. JWT Key Rotation
JWT keys are stored in a keyring. To rotate:
1. Generate a new key with a new `kid`.
2. Add the new key to the `IAM_JWT_KEYS` secret in Secrets Manager with status `active`.
3. Keep the old key with status `deprecated`.
4. Wait for at least 1 hour (longer than the maximum token lifetime).
5. Remove the deprecated key from the secret.

## 2. AES Key Rotation (MFA)
Used for encrypting MFA secrets in the database.
1. Add a new AES-256 key to the `IAM_AES_KEYS` keyring.
2. Mark the new key as `active`.
3. The IAM service will automatically use the new key for new MFA enrollments.
4. Existing secrets will continue to be decrypted using the `kid` stored in the database.

## 3. CA Certificate Rotation
Follows the dual-CA trust period procedure (Spec §2.9).
1. Generate a new Intermediate CA.
2. Update all services to trust BOTH the old and new CA certificates.
3. Issue new leaf certificates for all services using the new CA.
4. Once all services have updated certificates, remove the old CA from the trust store.

## 4. Database Password Rotation
1. Update the password in PostgreSQL.
2. Use pgBouncer `PAUSE` to hold connections.
3. Update the `DATABASE_URL` in Secrets Manager/Kubernetes.
4. Restart services or wait for secret sync.
5. Use pgBouncer `RESUME`.
