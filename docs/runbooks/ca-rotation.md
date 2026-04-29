# Runbook: CA Rotation

## Context
Rotating the mTLS Certificate Authority requires careful coordination because OpenGuard relies on mTLS for all service-to-service communication.

## Steps
1. Generate the new CA alongside the old CA.
2. Distribute a combined trust bundle (`ca-bundle.crt`) containing both the old and new CA certificates to all services.
3. Perform a rolling restart of all services so they load the new trust bundle.
4. Issue new leaf certificates signed by the new CA.
5. Perform another rolling restart of all services to use the new leaf certificates.
6. Remove the old CA from the trust bundle and distribute the final bundle.
7. Perform a final rolling restart.
