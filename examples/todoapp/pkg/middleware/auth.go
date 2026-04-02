package middleware

import (
	"context"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/openguard/todoapp/pkg/sdk"
)

type TokenVerifier interface {
	VerifyToken(ctx context.Context, rawToken string) (*oidc.IDToken, error)
}

func Auth(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := sdk.ExtractToken(r)
			if token == "" {
				http.Error(w, "unauthorized: missing_token", http.StatusUnauthorized)
				return
			}

			idToken, err := verifier.VerifyToken(r.Context(), token)
			if err != nil {
				http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
				return
			}

			// Extract claims
			var claims struct {
				Sub   string `json:"sub"`
				OrgID string `json:"org_id"`
			}
			if err := idToken.Claims(&claims); err != nil {
				http.Error(w, "invalid_claims", http.StatusUnauthorized)
				return
			}

			// Add to headers for handlers to consume
			r.Header.Set("X-User-ID", claims.Sub)
			r.Header.Set("X-Org-ID", claims.OrgID)

			next.ServeHTTP(w, r)
		})
	}
}
