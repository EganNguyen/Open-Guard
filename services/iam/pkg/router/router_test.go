package router

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openguard/iam/pkg/handlers"
	"github.com/stretchr/testify/assert"
)

func TestNewRouter(t *testing.T) {
	cfg := Config{
		AuthHandler:  &handlers.AuthHandler{},
		UserHandler:  &handlers.UserHandler{},
		MFAHandler:   &handlers.MFAHandler{},
		SCIMHandler:  &handlers.SCIMHandler{},
		TokenHandler: &handlers.TokenHandler{},
		Logger:       slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	r := New(cfg)
	assert.NotNil(t, r)

	req := httptest.NewRequest("GET", "/health/live", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rr.Body.String())
}
