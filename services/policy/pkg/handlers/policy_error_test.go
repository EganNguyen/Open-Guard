package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolicyHandler_Evaluate_BadJSON(t *testing.T) {
	_, r := setup(t)
	reqBuf := bytes.NewBuffer([]byte(`{bad json`))
	req := httptest.NewRequest("POST", "/policies/evaluate", reqBuf)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPolicyHandler_Update_NotFound(t *testing.T) {
	_, r := setup(t)
	reqBuf := bytes.NewBuffer([]byte(`{"name":"My Policy"}`))
	req := httptest.NewRequest("PUT", "/policies/not-found", reqBuf)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPolicyHandler_Delete_NotFound(t *testing.T) {
	_, r := setup(t)
	req := httptest.NewRequest("DELETE", "/policies/not-found", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPolicyHandler_Update_BadJSON(t *testing.T) {
	_, r := setup(t)
	reqBuf := bytes.NewBuffer([]byte(`{bad json`))
	req := httptest.NewRequest("PUT", "/policies/p123", reqBuf)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
