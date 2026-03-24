package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// APIKeyValidator defines how a key is verified against the connector registry.
// Implementations should query the database for the active ConnectedApp.
type APIKeyValidator interface {
	// ValidateKey takes a SHA-256 hash of the API key and returns the org_id if valid.
	ValidateKey(ctx context.Context, keyHash string) (string, error)
}

// APIKeyAuth validates an incoming Bearer token as an API key.
// It hashes the token, looks it up via the validator, and sets the org_id in the context.
func APIKeyAuth(validator APIKeyValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":{"code":"unauthorized","message":"missing or invalid Authorization header"}}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			hash := sha256.Sum256([]byte(token))
			hashHex := hex.EncodeToString(hash[:])

			orgID, err := validator.ValidateKey(r.Context(), hashHex)
			if err != nil {
				// We don't distinguish between not found and db error to the caller
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid API key"}}`, http.StatusUnauthorized)
				return
			}
			if orgID == "" {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid API key"}}`, http.StatusUnauthorized)
				return
			}

			// Store the orgID in the context using the existing TenantIDKey
			ctx := context.WithValue(r.Context(), TenantIDKey, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
