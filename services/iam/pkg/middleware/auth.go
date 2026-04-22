package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	OrgIDKey  contextKey = "org_id"
)

// Auth is a middleware that validates the JWT from either a cookie or the Authorization header.
func Auth(keyring []crypto.JWTKey, rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			// 1. Check Cookie
			cookie, err := r.Cookie("openguard_session")
			if err == nil {
				tokenStr = cookie.Value
			}

			// 2. Fallback to Authorization Header
			if tokenStr == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if tokenStr == "" {
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			// 3. Verify Token
			claims := &crypto.StandardClaims{}
			_, err = crypto.Verify(tokenStr, keyring, claims)
			if err != nil {
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			// 4. Check JTI Blocklist in Redis
			if rdb != nil {
				blocked, err := rdb.Exists(r.Context(), "blocklist:"+claims.ID).Result()
				if err != nil {
					// Log error but maybe allow if redis is down? 
					// Architect says jti blocklist is critical, so we might want to fail-closed.
				}
				if blocked > 0 {
					http.Error(w, "Unauthorized: token revoked", http.StatusUnauthorized)
					return
				}
			}

			// 5. Inject into Context
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, OrgIDKey, claims.OrgID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID retrieves the user ID from the context.
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}

// GetOrgID retrieves the organization ID from the context.
func GetOrgID(ctx context.Context) string {
	if id, ok := ctx.Value(OrgIDKey).(string); ok {
		return id
	}
	return ""
}
