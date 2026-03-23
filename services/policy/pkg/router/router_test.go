package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openguard/policy/pkg/tenant"
	"github.com/stretchr/testify/assert"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
}

func TestInjectOrgContext(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgIDFromContext(r.Context())
		userID := UserIDFromContext(r.Context())
		w.Header().Set("X-Got-Org", orgID)
		w.Header().Set("X-Got-User", userID)
		w.WriteHeader(http.StatusOK)
	})

	handlerToTest := injectOrgContext(nextHandler)

	t.Run("missing org id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handlerToTest.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "MISSING_ORG_CONTEXT")
	})

	t.Run("valid headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Org-ID", "org-1")
		req.Header.Set("X-User-ID", "user-1")
		rr := httptest.NewRecorder()
		
		handlerToTest.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "org-1", rr.Header().Get("X-Got-Org"))
		assert.Equal(t, "user-1", rr.Header().Get("X-Got-User"))
	})
}

func TestContextExtractors(t *testing.T) {
	ctx := context.WithValue(context.Background(), tenant.OrgIDKey, "org1")
	ctx = context.WithValue(ctx, tenant.UserIDKey, "user1")

	assert.Equal(t, "org1", OrgIDFromContext(ctx))
	assert.Equal(t, "user1", UserIDFromContext(ctx))
}

func TestNewRouter(t *testing.T) {
	// A basic configuration check
	r := New(Config{})
	assert.NotNil(t, r)
	
	// Ensure health endpoint is registered
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}
