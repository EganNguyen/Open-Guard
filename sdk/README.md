# Open-Guard Go SDK

The Open-Guard Go SDK is the primary integration surface for connecting your applications to the Open-Guard Policy Service.

## Installation

```bash
go get github.com/openguard/sdk
```

## Usage

```go
client := sdk.NewClient("http://localhost:8080", "your-api-key",
    sdk.WithCircuitBreaker(3, 5*time.Second),
    sdk.WithRetry(2, 100*time.Millisecond),
)
defer client.Close()

allowed, err := client.Allow(ctx, "user-123", "read", "document-456")
if err != nil {
    // This will only be an error if the SDK logic itself fails.
    // Network errors are handled by fail-closed/fail-open logic.
}

if allowed {
    // Proceed with action
}
```

## Resilience Features

### Fail-Closed (Default)
In production, the SDK defaults to "fail-closed". If the policy service is unreachable and there is no cached decision, the SDK will deny access (`allowed=false`) without returning an error. This ensures security even when the control plane is down.

### Stale-While-Unavailable (Grace Period)
The SDK implements a 60-second grace period for cached decisions. If the policy service is down, the SDK will continue to serve cached values for up to 60 seconds after they have officially expired.

### Fail-Open (Dev Mode)
For development environments where you want to avoid being blocked by a local policy service outage, you can enable fail-open mode:
```go
client := sdk.NewClient(url, key, sdk.WithFailOpen(true))
```

### Circuit Breaker
Use `WithCircuitBreaker` to prevent the SDK from hammering a failing policy service. When the threshold of consecutive failures is met, the breaker opens, and the SDK will immediately fail-closed (or fail-open if configured) until the timeout expires.

### Retries
Use `WithRetry` to configure automatic retries for transient network failures.

## Configuration

- `WithCacheTTL(ttl)`: Sets the local cache TTL (default 60s). Values above 60s are not recommended for security reasons.
- `WithFailOpen(bool)`: Enables/disables fail-open behavior.
- `WithCircuitBreaker(threshold, timeout)`: Configures the circuit breaker.
- `WithRetry(attempts, delay)`: Configures retry logic.
