package handlers

import (
	"net/http"
	"github.com/openguard/shared/middleware"
)

// orgIDFromCtx extracts the organization ID from the request context (set by middleware)
// or falls back to the X-Org-ID header for compatibility.
func orgIDFromCtx(r *http.Request) string {
	if v, ok := r.Context().Value(middleware.TenantIDKey).(string); ok {
		return v
	}
	return r.Header.Get("X-Org-ID")
}

// userIDFromCtx extracts the user ID from the request context or headers.
func userIDFromCtx(r *http.Request) string {
	if v, ok := r.Context().Value("user_id").(string); ok {
		return v
	}
	return r.Header.Get("X-User-ID")
}
