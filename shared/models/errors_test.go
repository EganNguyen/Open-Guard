package models

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	WriteError(rr, http.StatusNotFound, "NOT_FOUND", "resource missing", req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":{"code":"NOT_FOUND","message":"resource missing","request_id":"","retryable":false,"trace_id":""}}`, rr.Body.String())
}
