package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"

	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
	"github.com/openguard/shared/rls"
)


// AuthJWTWithBlocklist is a middleware that validates the JWT and checks the Redis blocklist.
// It uses a circuit breaker and is FAIL-OPEN on Redis failures per spec §1.3.
func AuthJWTWithBlocklist(keyring []crypto.JWTKey, rdb *redis.Client, breaker *gobreaker.CircuitBreaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractToken(r)
			if tokenStr == "" {
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			// 1. Verify signature and exp (reject ErrTokenExpired)
			claims := &crypto.StandardClaims{}
			_, err := crypto.Verify(tokenStr, keyring, claims)
			if err != nil {
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			jti := claims.RegisteredClaims.ID

			// 2. Check Redis blocklist with fail-open behavior
			if rdb != nil && breaker != nil {
				revoked, err := resilience.Call(r.Context(), breaker, 50*time.Millisecond,
					func(ctx context.Context) (bool, error) {
						n, err := rdb.Exists(ctx, "blocklist:"+jti).Result()
						return n > 0, err
					})

				if err == nil && revoked {
					http.Error(w, "Unauthorized: token revoked", http.StatusUnauthorized)
					return
				}

				if err != nil {
					slog.Warn("blocklist check failed, failing open", "error", err, "jti", jti)
				}
			}

			// 3. Inject into Context
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, OrgIDKey, claims.OrgID)
			ctx = context.WithValue(ctx, JTIKey, jti)
			ctx = context.WithValue(ctx, ExpiresAtKey, claims.RegisteredClaims.ExpiresAt.Time)
			ctx = rls.WithOrgID(ctx, claims.OrgID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	// 1. Check Cookie
	cookie, err := r.Cookie("openguard_session")
	if err == nil {
		return cookie.Value
	}

	// 2. Fallback to Authorization Header
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}
