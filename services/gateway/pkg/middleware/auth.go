package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/models"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	OrgIDKey  contextKey = "org_id"
	EmailKey  contextKey = "user_email"
)

// JWTAuth validates Bearer tokens and injects user identity headers for downstream services.
func JWTAuth(keyring *crypto.JWTKeyring) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				models.WriteError(w, http.StatusUnauthorized, "MISSING_TOKEN",
					"Authorization header is required", r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				models.WriteError(w, http.StatusUnauthorized, "INVALID_TOKEN",
					"Authorization header must be: Bearer <token>", r)
				return
			}

			tokenStr := parts[1]
			claims, err := keyring.Verify(tokenStr)
			if err != nil {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED",
					"Invalid or expired token", r)
				return
			}

			userID, _ := claims["sub"].(string)
			orgID, _ := claims["org_id"].(string)
			email, _ := claims["email"].(string)

			// Inject identity headers for downstream services
			r.Header.Set("X-User-ID", userID)
			r.Header.Set("X-Org-ID", orgID)
			r.Header.Set("X-User-Email", email)

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			ctx = context.WithValue(ctx, OrgIDKey, orgID)
			ctx = context.WithValue(ctx, EmailKey, email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
