# Runbook: Secret Rotation

## Context
Rotating JWT signing keys or API keys.

## Steps
1. Update `.env` or the Secrets Manager with the new key in the JSON array (for JWT/AES rings).
2. Set the old key status to `deprecated` (so it still verifies but is not used for signing).
3. Restart IAM and downstream services.
4. Wait for the maximum token TTL (e.g., 1 hour).
5. Remove the old key entirely from the keyring.
