package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/openguard/shared/crypto"
)

// AuthJWT is a middleware that validates the JWT from the Authorization header or cookie.
func AuthJWT(keyring []crypto.JWTKey) func(http.Handler) http.Handler {
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

			// 4. Inject into Context
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, OrgIDKey, claims.OrgID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
