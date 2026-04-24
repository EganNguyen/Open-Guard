// Package middleware provides shared HTTP middleware for all OpenGuard services.
package middleware

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// APIKeyAuthResult holds the result of API key authentication.
type APIKeyAuthResult struct {
	ConnectorID string
	OrgID       string
	KeyPrefix   string
}

// APIKeyLookup is the interface that auth backends must implement.
type APIKeyLookup interface {
	// FindByPrefix looks up a connector API key by its public prefix (fast path).
	// Returns the connector ID, org ID, and PBKDF2 hash for full verification.
	FindByPrefix(ctx context.Context, prefix string) (connectorID, orgID, pbkdf2Hash string, err error)
	// CacheGet checks Redis for a fast-path cached auth decision.
	CacheGet(ctx context.Context, keyHash string) (*APIKeyAuthResult, error)
	// CacheSet stores an auth decision in Redis (TTL: 5 min).
	CacheSet(ctx context.Context, keyHash string, result *APIKeyAuthResult, ttl time.Duration) error
}

const (
	apiKeyHeaderName  = "X-OpenGuard-Key"
	apiKeyPrefix      = "ogk_"       // Connector API keys start with this prefix
	apiKeyCacheTTL    = 5 * time.Minute
	apiKeyPrefixLen   = 8            // "ogk_" + 4 chars = 8 chars for prefix matching
)

// APIKeyAuth is a middleware that authenticates requests using a simple fixed API key.
// Used for internal service-to-service calls per spec §11.
func APIKeyAuth(expectedKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Internal-Key")
			if key == "" || subtle.ConstantTimeCompare([]byte(key), []byte(expectedKey)) != 1 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// For internal calls, we trust the org ID header if the internal key is valid.
			if orgID := r.Header.Get("X-OpenGuard-Org-ID"); orgID != "" {
				ctx := context.WithValue(r.Context(), OrgIDKey, orgID)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyAuthComplex is a middleware that authenticates connector requests using the
// fast-hash prefix → Redis → PBKDF2 fallback chain per spec §2.6.
func APIKeyAuthComplex(lookup APIKeyLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := r.Header.Get(apiKeyHeaderName)
			if rawKey == "" {
				http.Error(w, "Unauthorized: missing API key", http.StatusUnauthorized)
				return
			}

			if !strings.HasPrefix(rawKey, apiKeyPrefix) {
				http.Error(w, "Unauthorized: invalid API key format", http.StatusUnauthorized)
				return
			}

			// Fast hash: SHA-256 of the full key for Redis lookup
			sum := sha256.Sum256([]byte(rawKey))
			keyHash := fmt.Sprintf("%x", sum)

			ctx := r.Context()

			// Tier 1: Redis cache
			cached, err := lookup.CacheGet(ctx, keyHash)
			if err == nil && cached != nil {
				ctx = context.WithValue(ctx, ConnectorIDKey, cached.ConnectorID)
				ctx = context.WithValue(ctx, OrgIDKey, cached.OrgID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Tier 2: DB prefix lookup + PBKDF2 verification
			if len(rawKey) < apiKeyPrefixLen {
				http.Error(w, "Unauthorized: API key too short", http.StatusUnauthorized)
				return
			}
			prefix := rawKey[:apiKeyPrefixLen]
			connectorID, orgID, pbkdf2Hash, err := lookup.FindByPrefix(ctx, prefix)
			if err != nil {
				http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
				return
			}

			// PBKDF2 verification: compare hash of submitted key against stored hash
			if !verifyPBKDF2(rawKey, pbkdf2Hash) {
				http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
				return
			}

			result := &APIKeyAuthResult{
				ConnectorID: connectorID,
				OrgID:       orgID,
				KeyPrefix:   prefix,
			}

			// Cache success for 5 minutes (non-blocking)
			go lookup.CacheSet(context.Background(), keyHash, result, apiKeyCacheTTL)

			ctx = context.WithValue(ctx, ConnectorIDKey, connectorID)
			ctx = context.WithValue(ctx, OrgIDKey, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// verifyPBKDF2 compares a raw API key against a stored PBKDF2 hash.
// The stored hash format is: "pbkdf2$sha512$<iterations>$<salt_hex>$<hash_hex>"
// This is intentionally slow (security property) — use the Redis cache to avoid repeated calls.
func verifyPBKDF2(rawKey, storedHash string) bool {
	// Parse stored hash: pbkdf2$sha512$iterations$salt$hash
	parts := strings.Split(storedHash, "$")
	if len(parts) != 5 || parts[0] != "pbkdf2" || parts[1] != "sha512" {
		return false
	}

	var iterations int
	fmt.Sscanf(parts[2], "%d", &iterations)
	if iterations < 600000 {
		return false // reject weak hashes
	}

	// Re-derive and compare — constant time via crypto/subtle is ideal but
	// for the purposes of this middleware the comparison is done in the DB layer.
	// The stored hash is checked by recomputing the PBKDF2 and doing a constant-time compare.
	saltHex := parts[3]
	expectedHex := parts[4]

	derived := derivePBKDF2(rawKey, saltHex, iterations)
	return constantTimeCompare(derived, expectedHex)
}

// derivePBKDF2 re-derives the PBKDF2 hash for comparison.
// Uses SHA-512 with the given salt and iteration count.
func derivePBKDF2(key, saltHex string, iterations int) string {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return ""
	}
	// spec §2.6: PBKDF2-HMAC-SHA512, 600,000 iterations, 64-byte output
	hash := pbkdf2.Key([]byte(key), salt, iterations, 64, sha512.New)
	return hex.EncodeToString(hash)
}

// constantTimeCompare does a constant-time string comparison.
func constantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
