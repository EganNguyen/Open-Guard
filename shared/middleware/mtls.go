package middleware

import (
	"net/http"
	"os"
)

// RequireMTLS enforces that a request provides a valid client certificate info.
// Note: Actual TLS termination happens at Load Balancer,
// which forwards the cert info in headers, or we check r.TLS directly.
func RequireMTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In development, we allow bypassing mTLS for ease of testing
		if os.Getenv("APP_ENV") == "development" {
			next.ServeHTTP(w, r)
			return
		}

		certHeader := r.Header.Get("X-Forwarded-Client-Cert")
		hasPeerCert := r.TLS != nil && len(r.TLS.PeerCertificates) > 0

		if certHeader == "" && !hasPeerCert {
			http.Error(w, `{"error":{"code":"mtls_required","message":"Client certificate required for this endpoint"}}`, http.StatusForbidden)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

