# Runbook: Incident Response

Guidelines for responding to security alerts triggered by the Threat service.

## 1. Brute Force Alert
- **Detector**: `BruteForceDetector`
- **Response**:
  1. Identify the source IP and target accounts.
  2. Temporary block the source IP at the WAF/Gateway level.
  3. Force password reset for target accounts if login was successful.
  4. Audit logs for any data exfiltration from those accounts.

## 2. Impossible Travel Alert
- **Detector**: `ImpossibleTravelDetector`
- **Response**:
  1. Revoke all active sessions for the user.
  2. Trigger MFA re-verification on next login.
  3. Contact the user to verify activity.

## 3. Data Exfiltration (DLP)
- **Detector**: `DLP Async Scanner`
- **Response**:
  1. Identify the event ID and org ID.
  2. Locate the source of the data (which connected app?).
  3. Purge sensitive data from logs/ClickHouse if necessary.
  4. Notify the Org administrator.
