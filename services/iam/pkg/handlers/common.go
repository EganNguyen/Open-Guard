package handlers

import (
	"net/http"
	"github.com/openguard/shared/middleware"
)

func orgIDFromCtx(r *http.Request) string {
	if v, ok := r.Context().Value(middleware.TenantIDKey).(string); ok {
		return v
	}
	return r.Header.Get("X-Org-ID")
}
