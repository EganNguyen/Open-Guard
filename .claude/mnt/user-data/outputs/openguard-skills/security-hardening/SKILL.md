---
name: openguard-security-hardening
description: >
  Use this skill for any security-sensitive feature in OpenGuard. Triggers:
  "connector API key", "JWT rotation", "PBKDF2", "bcrypt", "MFA", "TOTP",
  "WebAuthn", "SCIM auth", "session management", "refresh token", "jti blocklist",
  "SSRF protection", "webhook signing", "HMAC", "secret rotation", "safe logger",
  "token revocation", "account enumeration", "DLP", "encryption key", or any
  feature in shared/crypto/, shared/middleware/apikey.go, shared/middleware/scim.go,
  services/iam/pkg/service/auth.go, or services/alerting/. Also use when reviewing
  authentication flows, authorization logic, or any code that touches credentials.
  Security bugs in this codebase are customer data breaches.
---

# OpenGuard Security Hardening Skill

OpenGuard is a security control plane. Every bug in an auth flow, credential
handler, or session manager is a potential customer data breach or compliance
violation. This skill covers the security patterns that must be implemented
exactly as specified — not approximated.

---

## 1. Connector API Key Authentication (Two-Tier Scheme)

The connector auth scheme must sustain 20,000 req/s event ingest. Direct PBKDF2
on every request would add ~400ms per request and is forbidden.

**Key anatomy:**
```
Full API key = prefix (8 chars, non-secret) + secret (24 chars)
Example:     "aB3xK9mZ" + "rT7vQ2sN4wP6yL8uC1dE5fG0"
```

**Auth flow:**

```go
// shared/middleware/apikey.go
func APIKeyMiddleware(cache ConnectorCache, repo ConnectorReader) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if len(raw) < 16 { // minimum: 8 prefix + 8 secret
                writeError(w, r, http.StatusUnauthorized, "INVALID_KEY", "invalid API key format")
                return
            }

            prefix := raw[:8]
            secret := raw[8:]
            fastHash := sha256Hex(prefix) // SHA-256(prefix) — O(microseconds)

            // Tier 1: Redis cache lookup by fast-hash of prefix
            app, ok, err := cache.Get(r.Context(), fastHash)
            if err != nil {
                slog.ErrorContext(r.Context(), "connector cache error", "error", err)
                // Cache error → fall through to DB path
            }
            if ok {
                // Cache hit: check if full PBKDF2 re-verification is needed
                // (only every 5 minutes to avoid ~400ms per request)
                if time.Since(app.LastVerifiedAt) > 5*time.Minute {
                    if !pbkdf2Verify(secret, app.APIKeyHash) {
                        writeError(w, r, http.StatusUnauthorized, "INVALID_KEY", "invalid credentials")
                        return
                    }
                    app.LastVerifiedAt = time.Now()
                    cache.Set(r.Context(), fastHash, app, 30*time.Second)
                }
                finalizeConnectorContext(w, r, next, app)
                return
            }

            // Tier 2: Cache miss → DB path (full PBKDF2, ~400ms, rare)
            fullHash := pbkdf2Hash(raw) // PBKDF2-HMAC-SHA512, 600k iterations
            dbApp, err := repo.GetByKeyHash(r.Context(), fullHash)
            if err != nil {
                // Uniform error regardless of whether user exists (timing attack)
                writeError(w, r, http.StatusUnauthorized, "INVALID_KEY", "invalid credentials")
                return
            }
            dbApp.LastVerifiedAt = time.Now()
            cache.Set(r.Context(), fastHash, dbApp, 30*time.Second)
            finalizeConnectorContext(w, r, next, dbApp)
        })
    }
}

func finalizeConnectorContext(w http.ResponseWriter, r *http.Request,
    next http.Handler, app *ConnectedApp) {
    if app.Status != "active" {
        writeError(w, r, http.StatusUnauthorized, "CONNECTOR_SUSPENDED", "connector is suspended")
        return
    }
    ctx := rls.WithOrgID(r.Context(), app.OrgID)
    ctx = withConnectorID(ctx, app.ID)
    ctx = withConnectorScopes(ctx, app.Scopes)
    next.ServeHTTP(w, r.WithContext(ctx))
}
```

**Cache invalidation on connector update:**
```go
// PATCH /v1/admin/connectors/:id handler
func (h *Handler) UpdateConnector(w http.ResponseWriter, r *http.Request) {
    // ... bind, validate ...
    updated, err := h.svc.UpdateConnector(r.Context(), id, req)
    if err != nil {
        h.handleServiceError(w, r, err)
        return
    }
    // Invalidate cache BEFORE returning — client must re-authenticate
    fastHash := sha256Hex(updated.APIKeyPrefix)
    if err := h.cache.Delete(r.Context(), fastHash); err != nil {
        slog.ErrorContext(r.Context(), "failed to invalidate connector cache",
            "connector_id", id, "error", err)
        // Non-fatal: cache will expire in 30s. Log for monitoring.
    }
    h.respond(w, r, http.StatusOK, updated)
}
```

---

## 2. PBKDF2 Key Hashing

```go
// shared/crypto/pbkdf2.go
import "golang.org/x/crypto/pbkdf2"

const (
    pbkdf2Iterations = 600_000
    pbkdf2KeyLen     = 32
)

// Hash computes PBKDF2-HMAC-SHA512 of the full API key.
// Used for DB storage. ~400ms on modern hardware — never call on hot path.
// Salt is per-org, stored separately, sourced from CONTROL_PLANE_API_KEY_SALT.
func Hash(key, salt string) string {
    dk := pbkdf2.Key([]byte(key), []byte(salt), pbkdf2Iterations, pbkdf2KeyLen, sha512.New)
    return hex.EncodeToString(dk)
}

// Verify checks a key against a stored hash in constant time.
func Verify(key, salt, storedHash string) bool {
    computed := Hash(key, salt)
    return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}
```

**PBKDF2 is only called in two places:**
1. At connector registration time (one-off, not in the request path)
2. At cache miss during auth (rare; cached for 30s after first use)

Never compute PBKDF2 on every authenticated request. This is explicitly forbidden
in the spec's forbidden patterns table.

---

## 3. JWT Multi-Key Keyring

```go
// shared/crypto/jwt.go
type JWTKey struct {
    Kid       string `json:"kid"`
    Secret    string `json:"secret"`
    Algorithm string `json:"algorithm"` // "HS256"
    Status    string `json:"status"`    // "active" | "verify_only"
}

type JWTKeyring struct{ keys []JWTKey }

// Sign uses the FIRST active key. Includes kid in JWT header.
// Claims must include jti (unique token ID for revocation).
func (k *JWTKeyring) Sign(claims jwt.MapClaims) (string, error) {
    var active *JWTKey
    for i := range k.keys {
        if k.keys[i].Status == "active" {
            active = &k.keys[i]
            break
        }
    }
    if active == nil {
        return "", errors.New("no active JWT signing key")
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    token.Header["kid"] = active.Kid
    return token.SignedString([]byte(active.Secret))
}

// Verify extracts kid from header, finds matching key (active OR verify_only).
// Also checks jti against Redis blocklist on every call.
func (k *JWTKeyring) Verify(ctx context.Context, tokenStr string,
    blocklist JTIBlocklist) (jwt.MapClaims, error) {

    // Parse without verification to get kid from header
    unverified, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
    if err != nil {
        return nil, models.ErrUnauthorized
    }
    kid, _ := unverified.Header["kid"].(string)

    // Find key by kid
    var signingKey *JWTKey
    for i := range k.keys {
        if k.keys[i].Kid == kid {
            signingKey = &k.keys[i]
            break
        }
    }
    if signingKey == nil {
        return nil, models.ErrUnauthorized
    }

    // Verify signature and expiry
    token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
        if t.Method.Alg() != signingKey.Algorithm {
            return nil, fmt.Errorf("unexpected algorithm: %s", t.Method.Alg())
        }
        return []byte(signingKey.Secret), nil
    })
    if err != nil || !token.Valid {
        return nil, models.ErrUnauthorized
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return nil, models.ErrUnauthorized
    }

    // Check jti against blocklist on every request
    jti, _ := claims["jti"].(string)
    if jti == "" {
        return nil, models.ErrUnauthorized // jti is mandatory
    }
    revoked, err := blocklist.IsRevoked(ctx, jti)
    if err != nil {
        // Blocklist check failure: fail closed — reject token
        return nil, fmt.Errorf("blocklist check failed: %w", models.ErrUnauthorized)
    }
    if revoked {
        return nil, models.ErrUnauthorized
    }

    return claims, nil
}
```

**JWT Rotation (Zero-Downtime):**
1. Add new key as `active` to `IAM_JWT_KEYS_JSON`. Set old key to `verify_only`.
2. Rolling deploy IAM. New tokens use new key; old tokens still verify via `verify_only`.
3. Wait `IAM_JWT_EXPIRY_SECONDS` (900s = 15 min).
4. Remove old key from JSON. Rolling deploy IAM.

**jti blocklist (Redis):**
```go
// On logout or session revocation:
func (b *JTIBlocklist) Revoke(ctx context.Context, jti string, expiry time.Duration) error {
    return b.redis.Set(ctx, "jti:"+jti, "1", expiry).Err()
    // TTL = token's remaining lifetime, not a fixed duration
}

func (b *JTIBlocklist) IsRevoked(ctx context.Context, jti string) (bool, error) {
    exists, err := b.redis.Exists(ctx, "jti:"+jti).Result()
    if err != nil {
        return false, err
    }
    return exists > 0, nil
}
```

---

## 4. SCIM Authentication (org_id From Token, Never From Header)

```go
// shared/middleware/scim.go
// org_id is ALWAYS derived from the token configuration.
// A client-supplied X-Org-ID header is NEVER trusted.
func SCIMAuthMiddleware(tokens []SCIMToken) func(http.Handler) http.Handler {
    tokenMap := make(map[string]string, len(tokens))
    for _, t := range tokens {
        tokenMap[t.Token] = t.OrgID // token string → org_id
    }
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            orgID, ok := tokenMap[raw]
            if !ok {
                writeError(w, r, http.StatusUnauthorized,
                    "INVALID_SCIM_TOKEN", "invalid SCIM bearer token")
                return
            }
            // org_id from token config — never from request
            ctx := rls.WithOrgID(r.Context(), orgID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

Accepting `X-Org-ID` from a SCIM client is an org_id spoofing vulnerability.
A compromised IdP could provision users into arbitrary orgs. The token map is
built at startup from `IAM_SCIM_TOKENS_JSON`. Tokens are per-org.

---

## 5. Account Enumeration Prevention

Login endpoints must not reveal whether an account exists via timing or error messages:

```go
// services/iam/pkg/service/auth.go
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy"), 12)

func (s *AuthService) Login(ctx context.Context, email, password string) (*models.User, error) {
    user, err := s.repo.GetByEmail(ctx, email)
    if err != nil {
        if errors.Is(err, models.ErrNotFound) {
            // User doesn't exist — run bcrypt anyway to equalize timing
            s.pool.Verify(ctx, string(dummyHash), password)
            return nil, models.ErrUnauthorized // identical error to wrong password
        }
        return nil, fmt.Errorf("get user: %w", err)
    }

    // User exists — verify password through worker pool
    if err := s.pool.Verify(ctx, user.PasswordHash, password); err != nil {
        s.incrementFailedLoginCount(ctx, user)
        return nil, models.ErrUnauthorized
    }

    return user, nil
}
```

**Rules:**
- Same error message for "user not found" and "wrong password": `ErrUnauthorized`
- Always run bcrypt (against dummy hash for missing users) to equalize timing
- Never log whether the user existed or not in the error path

---

## 6. Session Risk Scoring

Applied at `POST /auth/refresh`. Revoke if risk score ≥ 80:

```go
func (s *AuthService) scoreSessionRisk(existing, incoming *SessionContext) int {
    score := 0

    // User agent family change (Chrome → Firefox): +60
    if uaFamily(existing.UserAgent) != uaFamily(incoming.UserAgent) {
        score += 60
    }
    // UA version change, same family (Chrome 119 → 122): +20
    if uaFamily(existing.UserAgent) == uaFamily(incoming.UserAgent) &&
        uaVersion(existing.UserAgent) != uaVersion(incoming.UserAgent) {
        score += 20
    }
    // IP /16 subnet change: +40
    if subnet16(existing.IP) != subnet16(incoming.IP) {
        score += 40
    } else if existing.IP != incoming.IP {
        // Same /16, different host: +15
        score += 15
    }

    return score
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
    session, err := s.repo.GetSessionByRefreshHash(ctx, hashRefreshToken(refreshToken))
    if err != nil {
        return nil, models.ErrUnauthorized
    }

    incoming := sessionContextFromRequest(ctx)
    if score := s.scoreSessionRisk(session, incoming); score >= 80 {
        s.revokeSession(ctx, session, "risk_score_exceeded")
        s.outbox.Write(ctx, tx, kafka.TopicAuthEvents, ..., buildSessionRevokedEnvelope(...))
        return nil, models.ErrUnauthorized // SESSION_REVOKED_RISK
    }

    return s.rotateRefreshToken(ctx, session, incoming)
}
```

**Refresh token grace window (prevents false logout on concurrent retries):**
```go
// On successful refresh:
// 1. Generate new refresh token
// 2. Store: prev_refresh_hash = old_hash, prev_hash_expiry = NOW() + grace_seconds
// 3. If incoming token matches prev_refresh_hash AND within expiry: accept idempotently
// 4. If incoming token matches prev_refresh_hash but EXPIRED: REVOKE (token reuse = compromise)
```

---

## 7. SSRF Protection

All outgoing webhook URLs must pass this check at registration time AND at delivery time:

```go
// shared/middleware/ssrf.go
func ValidateWebhookURL(raw string) error {
    u, err := url.Parse(raw)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }
    if u.Scheme != "https" {
        return errors.New("webhook URL must use HTTPS")
    }
    host := u.Hostname()
    if host == "" {
        return errors.New("webhook URL missing host")
    }

    ips, err := net.LookupHost(host)
    if err != nil {
        return fmt.Errorf("DNS resolution failed for %s: %w", host, err)
    }

    for _, ipStr := range ips {
        ip := net.ParseIP(ipStr)
        if ip == nil {
            continue
        }
        if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
            ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
            return fmt.Errorf("webhook URL resolves to restricted IP %s", ipStr)
        }
        // Block AWS metadata endpoint explicitly
        if ip.Equal(net.ParseIP("169.254.169.254")) {
            return fmt.Errorf("webhook URL resolves to metadata endpoint (SSRF blocked)")
        }
    }
    return nil
}
```

Call at: connector registration (`POST /v1/admin/connectors`), connector update
(`PATCH`), and at delivery time in the webhook delivery service before each POST.

---

## 8. Webhook Signing (Outbound)

```go
// shared/crypto/hmac.go
// Sign computes HMAC-SHA256 over "<unix_timestamp>.<payload>".
// The timestamp prevents replay attacks.
func SignWebhookPayload(secret string, timestamp int64, payload []byte) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
    mac.Write([]byte("."))
    mac.Write(payload)
    return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Headers added to every outbound webhook POST:
//   X-OpenGuard-Signature:  sha256=<hmac>
//   X-OpenGuard-Delivery:   <uuid>   (for receiver idempotency)
//   X-OpenGuard-Timestamp:  <unix_seconds>

// Receiver replay protection: reject if abs(now - timestamp) > 300 seconds
func ValidateReplayWindow(timestamp int64, toleranceSeconds int64) error {
    diff := time.Now().Unix() - timestamp
    if diff < 0 {
        diff = -diff
    }
    if diff > toleranceSeconds {
        return fmt.Errorf("request timestamp %d is outside replay window", timestamp)
    }
    return nil
}
```

---

## 9. MFA Secret Encryption (AES-256-GCM Multi-Key)

```go
// shared/crypto/aes.go
// Format: "<kid>:<base64(nonce+ciphertext)>"
// The kid prefix allows key rotation without re-encrypting everything at once.

func (k *EncryptionKeyring) Encrypt(plaintext []byte) (string, error) {
    active := k.activeKey()
    if active == nil {
        return "", errors.New("no active encryption key")
    }
    keyBytes, err := base64.StdEncoding.DecodeString(active.Key)
    if err != nil {
        return "", fmt.Errorf("decode key: %w", err)
    }
    block, err := aes.NewCipher(keyBytes)
    if err != nil {
        return "", fmt.Errorf("create cipher: %w", err)
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", fmt.Errorf("create GCM: %w", err)
    }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", fmt.Errorf("generate nonce: %w", err)
    }
    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil) // nonce prepended
    return active.Kid + ":" + base64.StdEncoding.EncodeToString(ciphertext), nil
}
```

**MFA backup codes (HMAC, not bcrypt array):**
```go
// Backup codes are HMAC-SHA256(code, IAM_MFA_BACKUP_CODE_HMAC_SECRET).
// NOT bcrypt — bcrypt array lookup is O(N × 300ms) = ~3s for 8 codes.
// HMAC lookup is O(1) with a DB array query.
func hashBackupCode(code, secret string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(code))
    return hex.EncodeToString(mac.Sum(nil))
}
```

---

## 10. Safe Logger

```go
// shared/telemetry/logger.go
// Read-only slice — named exception to §0.2 package-level variable rule
var sensitiveKeys = []string{
    "password", "secret", "token", "key", "auth", "credential",
    "private", "bearer", "authorization", "cookie", "session",
}

// SafeAttr wraps slog.Attr and redacts values whose key contains sensitive keywords.
// Use for ANY attribute that might contain credentials, even if you think it's safe.
func SafeAttr(key string, value any) slog.Attr {
    lower := strings.ToLower(key)
    for _, s := range sensitiveKeys {
        if strings.Contains(lower, s) {
            return slog.String(key, "[REDACTED]")
        }
    }
    return slog.Any(key, value)
}

// Usage:
slog.InfoContext(ctx, "connector authenticated",
    telemetry.SafeAttr("api_key_prefix", prefix), // "api_key" contains "key" → redacted
    "connector_id", connectorID,                   // safe
    "org_id", orgID,                               // safe
)
```

---

## 11. Security Checklist

Before submitting any auth/security code:

- [ ] JWT `jti` included in every token; blocklist checked on every authenticated request
- [ ] JWT `kid` included in header; keyring supports multiple keys for rotation
- [ ] Connector auth uses fast-hash prefix → Redis; PBKDF2 only on DB miss
- [ ] Redis cache invalidated (`DEL`) on connector update before returning response
- [ ] SCIM `org_id` derived from token map, never from `X-Org-ID` header
- [ ] Login always runs bcrypt (even for nonexistent users) to prevent timing attacks
- [ ] Session risk score computed on refresh; score ≥ 80 revokes session
- [ ] Outbound webhook URLs validated at registration AND at delivery time (SSRF)
- [ ] Webhook delivery includes `X-OpenGuard-Signature`, `X-OpenGuard-Delivery`, `X-OpenGuard-Timestamp`
- [ ] MFA backup codes use HMAC, not bcrypt array
- [ ] MFA secrets encrypted with AES-256-GCM with kid prefix for rotation
- [ ] All sensitive log attributes go through `SafeAttr`
- [ ] `subtle.ConstantTimeCompare` used for all credential comparisons
- [ ] Error messages don't reveal whether account exists (uniform `ErrUnauthorized`)
