# §15 — Phase 7: Security Hardening & Secret Rotation

---

## 15.1 HTTP Security Headers

```go
// shared/middleware/security.go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Content-Security-Policy", "default-src 'none'")
        w.Header().Set("Referrer-Policy", "no-referrer")
        w.Header().Set("X-Request-ID", generateRequestID())
        next.ServeHTTP(w, r)
    })
}
```

Applied to every service router.

---

## 15.2 SSRF Protection

All outgoing webhook URLs are validated at registration time AND re-validated on every delivery attempt. **Do not cache the resolved IP across deliveries.**

```go
func resolveAndValidateWebhookURL(raw string) (resolvedIP string, err error) {
    u, err := url.Parse(raw)
    if err != nil {
        return "", fmt.Errorf("invalid URL: %w", err)
    }
    if u.Scheme != "https" {
        return "", errors.New("webhook URL must use HTTPS")
    }
    ips, err := net.LookupHost(u.Hostname())
    if err != nil || len(ips) == 0 {
        return "", fmt.Errorf("DNS resolution failed: %w", err)
    }
    for _, ip := range ips {
        parsed := net.ParseIP(ip)
        if parsed == nil { continue }
        if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
            return "", fmt.Errorf("webhook URL resolves to restricted IP %s (SSRF blocked)", ip)
        }
    }
    return ips[0], nil
}

// NewBoundDeliveryClient creates an http.Client bound to a specific resolved IP.
// Called per-delivery AFTER re-validation. Never cached across deliveries.
func NewBoundDeliveryClient(resolvedIP string) *http.Client {
    dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
    return &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
                _, port, _ := net.SplitHostPort(addr)
                return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
            },
        },
    }
}
```

`WEBHOOK_IP_REVALIDATE_INTERVAL_SECONDS` (default: 300) controls re-resolution frequency.

---

## 15.3 Safe Logger

```go
// shared/telemetry/logger.go

// sensitiveKeys: read-only, initialized once (named exception per §0.2)
var sensitiveKeys = []string{
    "password", "secret", "token", "key", "auth", "credential",
    "private", "bearer", "authorization", "cookie", "session",
}

func SafeAttr(key string, value any) slog.Attr {
    keyLower := strings.ToLower(key)
    for _, s := range sensitiveKeys {
        if strings.Contains(keyLower, s) {
            return slog.String(key, "[REDACTED]")
        }
    }
    return slog.Any(key, value)
}
```

---

## 15.4 Secret Rotation Runbooks

Document in `docs/runbooks/secret-rotation.md`:

**JWT key rotation (zero-downtime):**
1. `scripts/rotate-jwt-keys.sh new` — generates new key.
2. Update `IAM_JWT_KEYS_JSON`: add new key as `active`, set old to `verify_only`.
3. Rolling deploy IAM.
4. Wait `IAM_JWT_EXPIRY_SECONDS`.
5. Remove old key. Rolling deploy IAM.

**MFA encryption key rotation (zero-downtime):**
1. Add new key as `active`, old as `verify_only`.
2. Deploy IAM.
3. Run `scripts/re-encrypt-mfa.sh` — batches of 100, `time.Sleep(50ms)` between batches (operational script, named exception per §0.13).
4. Remove old key. Deploy IAM.

**Connector API key rotation (with maintenance window):**
1. `DELETE /v1/admin/connectors/:id/api-key` — invalidates existing key immediately.
2. `POST /v1/admin/connectors/:id/api-key` — issues new key.
3. Update connected app's configuration.

**mTLS certificate rotation:** See §2.9.

**Kafka SASL credential rotation:** Add new credential → update password → rolling deploy → remove old credential.

---

## 15.5 Idempotency Key Constraints

- Maximum replay cache entry size: 64KB. Entries larger than 64KB are not cached.
- List endpoints and export download endpoints are excluded.
- Redis key: `"idempotent:{service}:{idempotency_key}"`, TTL 24 hours.

---

## 15.6 Phase 7 Acceptance Criteria

- [ ] Security headers on every response from every service.
- [ ] SSRF: connector webhook URL `http://localhost/internal` rejected at registration.
- [ ] SSRF: SIEM URL `http://169.254.169.254/latest/meta-data/` rejected at startup.
- [ ] Safe logger: log entry containing `password=secret123` → value appears as `[REDACTED]`.
- [ ] JWT rotation runbook executed end-to-end: old tokens rejected after rotation.
- [ ] MFA re-encryption: TOTP codes valid before and after re-encryption.
- [ ] `go mod verify` passes in CI.
- [ ] `govulncheck ./...` and `npm audit --audit-level=high` report zero issues.
- [ ] Idempotency: POST with same `Idempotency-Key` twice → second response identical with `Idempotency-Replayed: true`.
- [ ] Idempotency replay cache entry > 64KB is not cached (next request re-executes).
