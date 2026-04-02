package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "max-age=63072000; includeSubDomains; preload", rr.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	assert.Equal(t, "0", rr.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", rr.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "no-store, no-cache, max-age=0, must-revalidate, proxy-revalidate", rr.Header().Get("Cache-Control"))
}
