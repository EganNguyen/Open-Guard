package middleware

import (
	"context"
	"net/http"
)

type tenantContextKey string

const TenantIDKey tenantContextKey = "tenant_id"

// RequireTenant ensures the request is targeting a specific organization (tenant).
func RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Org-ID")
		if tenantID == "" {
			http.Error(w, `{"error":{"code":"missing_tenant","message":"X-Org-ID header is required"}}`, http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
