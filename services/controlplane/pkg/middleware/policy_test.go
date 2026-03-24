package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPolicyMiddleware_Permitted(t *testing.T) {
	// Mock Policy Service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/policies/evaluate", r.URL.Path)
		assert.Equal(t, "org-1", r.Header.Get("X-Org-ID"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EvalResponse{Permitted: true})
	}))
	defer server.Close()

	pc := NewPolicyClient(server.URL, slog.Default())
	middleware := pc.Middleware()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-Org-ID", "org-1")
	req.Header.Set("X-User-ID", "user-1")
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPolicyMiddleware_Denied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(EvalResponse{Permitted: false})
	}))
	defer server.Close()

	pc := NewPolicyClient(server.URL, slog.Default())
	middleware := pc.Middleware()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-Org-ID", "org-1")
	rec := httptest.NewRecorder()

	middleware(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPolicyMiddleware_MissingOrg(t *testing.T) {
	pc := NewPolicyClient("http://localhost", slog.Default())
	middleware := pc.Middleware()

	req := httptest.NewRequest("GET", "/api/data", nil)
	rec := httptest.NewRecorder()

	middleware(nil).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPolicyMiddleware_FailClosed_ServiceDown(t *testing.T) {
	// Wrong address to simulate connection error
	pc := NewPolicyClient("http://localhost:12345", slog.Default())
	middleware := pc.Middleware()

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-Org-ID", "org-1")
	rec := httptest.NewRecorder()

	middleware(nil).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "POLICY_ERROR")
}

func TestPolicyMiddleware_FailClosed_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	pc := NewPolicyClient(server.URL, slog.Default())
	middleware := pc.Middleware()

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-Org-ID", "org-1")
	rec := httptest.NewRecorder()

	middleware(nil).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPolicyMiddleware_FailClosed_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pc := NewPolicyClient(server.URL, slog.Default())
	middleware := pc.Middleware()

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-Org-ID", "org-1")
	rec := httptest.NewRecorder()

	middleware(nil).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPolicyMiddleware_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second) // Timeout is 2s
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pc := NewPolicyClient(server.URL, slog.Default())
	middleware := pc.Middleware()

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("X-Org-ID", "org-1")
	rec := httptest.NewRecorder()

	middleware(nil).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
