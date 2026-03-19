package middleware

import (
	"net/http"
)

// RequireMTLS enforces that a request provides a valid client certificate info.
// Note: Actual TLS termination happens at Load Balancer,
// which forwards the cert info in headers.
func RequireMTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		certHeader := r.Header.Get("X-Forwarded-Client-Cert")
		if certHeader == "" {
			http.Error(w, `{"error":{"code":"mtls_required","message":"Client certificate required for this endpoint"}}`, http.StatusForbidden)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}
