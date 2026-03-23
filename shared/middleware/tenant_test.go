package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequireTenant(t *testing.T) {
	handler := RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid := r.Context().Value(TenantIDKey).(string)
		assert.Equal(t, "org-1", tid)
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("missing tenant header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing_tenant")
	})

	t.Run("with tenant header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Org-ID", "org-1")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
