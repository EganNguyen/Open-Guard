package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		assert.NotEmpty(t, id)
	}))

	// Case 1: no pre-existing ID
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.NotEmpty(t, rr.Header().Get("X-Request-ID"))

	// Case 2: existing ID
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Request-ID", "req-1234")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, "req-1234", rr2.Header().Get("X-Request-ID"))
}

func TestGetRequestID(t *testing.T) {
	assert.Empty(t, GetRequestID(context.Background()))
}

func TestLogging(t *testing.T) {
	logger := slog.Default()
	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)
}
