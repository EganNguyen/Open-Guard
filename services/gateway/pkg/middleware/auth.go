package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openguard/shared/models"
	sharedmw "github.com/openguard/shared/middleware"
)

type contextKey string

const (
	UserIDKey  contextKey = "user_id"
	OrgIDKey   contextKey = "org_id"
	EmailKey   contextKey = "user_email"
)

// JWTAuth validates Bearer tokens and injects user identity headers for downstream services.
func JWTAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := sharedmw.GetRequestID(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				models.WriteError(w, http.StatusUnauthorized, "MISSING_TOKEN",
					"Authorization header is required", reqID)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				models.WriteError(w, http.StatusUnauthorized, "INVALID_TOKEN",
					"Authorization header must be: Bearer <token>", reqID)
				return
			}

			tokenStr := parts[1]
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED",
					"Invalid or expired token", reqID)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED",
					"Invalid token claims", reqID)
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
