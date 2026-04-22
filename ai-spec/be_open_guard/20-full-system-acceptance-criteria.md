# §20 — Full-System Acceptance Criteria

The following end-to-end scenario must execute without manual intervention. Run as a CI job on every release candidate. Every step is a CI assertion. The release pipeline does not publish unless all 45 steps pass.

---

```
1.  docker compose up -d
    → all services healthy

2.  POST /auth/register
    → org "Acme" + admin user; single transaction

3.  POST /oauth/token (IAM OIDC, password grant)
    → access_token + refresh_token; kid in JWT header

4.  POST /v1/admin/connectors (admin JWT)
    → connector "AcmeApp" with scopes [policy:evaluate, events:write]
    → one-time API key returned (Prefix/Secret scheme)

5.  POST /v1/admin/connectors (second, scim:read only)
    → connector "AcmeApp2"

6.  POST /v1/policies (admin JWT)
    → IP allowlist policy created

7.  POST /v1/policy/evaluate (AcmeApp key)
    → blocked IP: permitted:false; cache_hit:none

8.  POST /v1/policy/evaluate (same inputs, AcmeApp key)
    → permitted:false; cache_hit:redis

9.  POST /v1/policy/evaluate (AcmeApp2 key)
    → 403 INSUFFICIENT_SCOPE

10. POST /v1/events/ingest (AcmeApp, 50 events)
    → 200; 50 outbox records in one transaction
    → all 50 in GET /audit/events within 5s
    → EventSource="connector:<id>" on each

11. Simulate 11 failed login events via POST /v1/events/ingest
    → HIGH alert in MongoDB within 5s

12. GET /v1/threats/alerts
    → alert visible; severity=high

13. Verify SIEM webhook mock received payload
    → HMAC signature valid

14. GET /audit/events
    → all events from steps 2–11 present

15. GET /audit/integrity
    → ok:true; no chain gaps

16. POST /compliance/reports {type:"gdpr"}
    → report job created

17. Poll GET /compliance/reports/:id
    → status=completed within 60s

18. GET /compliance/reports/:id/download
    → valid PDF; all 5 GDPR sections present

19. POST /v1/events/ingest (event containing SSN field, AcmeApp, dlp_mode=monitor org)
    → 200; event accepted
    → audit log field masked within 5s

20. PATCH /v1/admin/connectors/:id2 {status:"suspended"}
    → AcmeApp2 suspended
    → connector cache invalidated immediately

21. POST /v1/events/ingest (AcmeApp2 key)
    → 403 INSUFFICIENT_SCOPE (after cache miss)

22. POST /v1/admin/connectors/:id/test
    → test webhook delivered; HMAC valid

23. GET /v1/admin/connectors/:id/deliveries
    → delivery log shows test + policy-change webhooks

24. POST /auth/refresh (valid refresh token)
    → new token issued; old token invalid after grace window

25. POST /auth/refresh (same client, high-risk UA change)
    → 401 SESSION_REVOKED_RISK

26. JWT key rotation: add new key → deploy IAM
    → old tokens still verify

27. JWT key rotation: remove old key → deploy IAM
    → old tokens return 401

28. Kill policy service
    → SDK falls back to local cache (60s)

29. After TTL: SDK /v1/policy/evaluate returns DenyDecision

30. Restart policy service
    → circuit breaker resets; evaluate succeeds

31. Kill Kafka
    → POST /v1/events/ingest succeeds; outbox pending

32. Restart Kafka
    → outbox records published within 30s

33. Crash audit consumer before offset commit
    → on restart: events reprocessed

34. MongoDB duplicate key errors skipped
    → audit log has no duplicate event_ids

35. go test ./... -race
    → all tests pass

36. k6 run loadtest/auth.js
    → p99 < 150ms at 2,000 req/s

37. k6 run loadtest/policy-evaluate.js
    → p99 < 5ms (cached); p99 < 30ms (uncached)

38. SDK local cache: second call produces 0 spans
    → verified via Jaeger

39. docker compose down
    → clean shutdown; no data loss

40. POST /auth/mfa/enroll (admin user)
    → TOTP secret + otpauth:// URI returned

41. POST /auth/mfa/verify (valid TOTP code)
    → MFA enrolled

42. POST /auth/mfa/challenge (same TOTP code, within 90s)
    → 401 TOTP_REPLAY_DETECTED

43. POST /scim/v2/Users (SCIM provision new user)
    → user.status = initializing; login attempt rejected

44. After saga completes (all services respond)
    → user.status = active; login succeeds

45. PATCH /v1/admin/connectors/:id {status:"suspended"}
    → Redis sentinel key written; cached hits return CONNECTOR_SUSPENDED
```
