# Open-Guard SDK

**Core Intent:** Fail-closed security SDK for Node.js/Go. Connects to `control-plane`.
**Key Files:**
- `client.go`: Main entry point.

**Fail-Closed Behavior:**
- SDK caches policies for 60s TTL. If `control-plane` goes offline, after 60s, SDK denies all requests.
