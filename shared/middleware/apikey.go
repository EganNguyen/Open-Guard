package middleware

import (
	"context"
	"net/http"
	"strings"
)

// APIKeyValidator defines how a key is verified against the connector registry.
type APIKeyValidator interface {
	// ValidateKey takes the raw API key and returns the orgID and connectorID if valid.
	ValidateKey(ctx context.Context, token string) (orgID string, connectorID string, err error)
}

func APIKeyAuth(validator APIKeyValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":{"code":"unauthorized","message":"missing or invalid Authorization header"}}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			orgID, connectorID, err := validator.ValidateKey(r.Context(), token)
			if err != nil || orgID == "" {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid API key"}}`, http.StatusUnauthorized)
				return
			}

			// Store values in context
			ctx := context.WithValue(r.Context(), TenantIDKey, orgID)
			ctx = context.WithValue(ctx, ConnectorIDKey, connectorID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
