package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	
	middleware := Logger(logger)
	
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("ok"))
	})

	handlerToTest := middleware(nextHandler)
	
	req := httptest.NewRequest("GET", "/test-log", nil)
	rec := httptest.NewRecorder()
	
	handlerToTest.ServeHTTP(rec, req)
	
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, buf.String(), "gateway request")
	assert.Contains(t, buf.String(), "method=GET")
	assert.Contains(t, buf.String(), "path=/test-log")
	assert.Contains(t, buf.String(), "status=202")
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: 0}
	
	rw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, rw.statusCode)
	assert.Equal(t, http.StatusCreated, rec.Code)
}
