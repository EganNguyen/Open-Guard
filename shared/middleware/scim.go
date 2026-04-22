package middleware

import (
	"context"
	"net/http"
	"strings"
)

// SCIMClaimsKey is the context key for SCIM auth claims.
type scimContextKey string

const SCIMOrgIDKey scimContextKey = "scim_org_id"

// SCIMTokenValidator validates a SCIM bearer token and returns the org_id it belongs to.
// Implementations should look up the token in the database and return the associated org.
type SCIMTokenValidator interface {
	ValidateSCIMToken(ctx context.Context, token string) (orgID string, err error)
}

// SCIMAuth is a middleware that enforces SCIM bearer token authentication per spec §2.7.
// The org_id is derived exclusively from the token (never from request headers or params).
//
// This prevents org escalation attacks where a caller could set X-Org-ID to another tenant's ID.
func SCIMAuth(validator SCIMTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "Unauthorized: SCIM bearer token required", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				http.Error(w, "Unauthorized: empty bearer token", http.StatusUnauthorized)
				return
			}

			orgID, err := validator.ValidateSCIMToken(r.Context(), token)
			if err != nil {
				http.Error(w, "Unauthorized: invalid SCIM token", http.StatusUnauthorized)
				return
			}

			if orgID == "" {
				http.Error(w, "Unauthorized: token has no associated org", http.StatusUnauthorized)
				return
			}

			// Inject org_id from token — never from request parameters
			ctx := context.WithValue(r.Context(), SCIMOrgIDKey, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetSCIMOrgID retrieves the SCIM org_id from the context.
// Always use this — never read org_id from URL params in SCIM handlers.
func GetSCIMOrgID(ctx context.Context) string {
	if id, ok := ctx.Value(SCIMOrgIDKey).(string); ok {
		return id
	}
	return ""
}
