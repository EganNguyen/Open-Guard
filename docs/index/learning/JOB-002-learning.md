# JOB-002 Learning: SSRF DNS Rebinding Prevention

## Discovery (The "Aha!" Moment)
Pre-validating a URL before making an HTTP request is insufficient to prevent SSRF if the attacker controls the DNS. Go's `http.Client` will perform a second resolution during the dial if not pinned.

## The Trap
Initially, I thought about keeping `ValidateOutboundURL` and just warning callers. However, this is a dangerous pattern that should be removed entirely to prevent future regressions. The only safe way is to validate *during* the dial and connect directly to the validated IP.

## Implementation Nuance
In unit tests, `httptest.NewServer` runs on `127.0.0.1`, which is blocked by the safe client. To allow tests to pass while maintaining strict production security, I implemented `SetClient` (or a similar dependency injection) to allow tests to swap in a standard client.

## Updated Hotspots
- **`shared/middleware/security.go`**: Now a critical security path for all outbound communication (webhooks, SIEM). Any change to `NewSafeHTTPClient` has a high blast radius.
- **DNS Resolution**: The system now relies on `net.DefaultResolver`. In high-security environments, this might need to be hardcoded to a trusted upstream resolver.

## Agent Playbook Update
When adding a new service that makes outbound HTTP calls (e.g., a new notification provider), **MUST** use `middleware.NewSafeHTTPClient()` instead of `http.Client{}`.
