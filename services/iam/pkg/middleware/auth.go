package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
)


const (
	UserIDKey    contextKey = "user_id"
	OrgIDKey     contextKey = "org_id"
	JTIKey       contextKey = "jti"
	ExpiresAtKey contextKey = "expires_at"
)

// Auth is a middleware that validates the JWT from either a cookie or the Authorization header.
// It is FAIL-CLOSED: if Redis is unavailable, it returns 401 rather than allowing revoked tokens through.
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

			// 4. Check JTI Blocklist in Redis — FAIL-CLOSED per spec §8
			if rdb != nil {
				blocked, err := rdb.Exists(r.Context(), "blocklist:"+claims.ID).Result()
				if err != nil {
					// Redis is down: fail-closed. A revoked token could be a security breach.
					http.Error(w, "Unauthorized: auth service unavailable", http.StatusUnauthorized)
					return
				}
				if blocked > 0 {
					http.Error(w, "Unauthorized: token revoked", http.StatusUnauthorized)
					return
				}
			}

			// 5. Inject into Context (user_id, org_id, jti, expires_at for logout)
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, OrgIDKey, claims.OrgID)
			ctx = context.WithValue(ctx, JTIKey, claims.ID)
			ctx = context.WithValue(ctx, ExpiresAtKey, claims.ExpiresAt.Time)

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

// GetJTI retrieves the JWT ID (JTI) from the context.
func GetJTI(ctx context.Context) string {
	if jti, ok := ctx.Value(JTIKey).(string); ok {
		return jti
	}
	return ""
}

// GetExpiresAt retrieves the token expiry time from the context.
func GetExpiresAt(ctx context.Context) time.Time {
	if exp, ok := ctx.Value(ExpiresAtKey).(time.Time); ok {
		return exp
	}
	return time.Time{}
}
