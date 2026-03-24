package middleware_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openguard/shared/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockValidator struct {
	validHashes map[string]string
}

func (m *mockValidator) ValidateKey(ctx context.Context, keyHash string) (string, error) {
	if orgID, ok := m.validHashes[keyHash]; ok {
		return orgID, nil
	}
	return "", errors.New("not found")
}

func TestAPIKeyAuth(t *testing.T) {
	token := "my-secret-token"
	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])

	validator := &mockValidator{
		validHashes: map[string]string{
			hashHex: "org-1",
		},
	}

	handler := middleware.APIKeyAuth(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := r.Context().Value(middleware.TenantIDKey)
		if orgID != nil {
			w.Header().Set("X-Found-Org-ID", orgID.(string))
		}
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("valid key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "org-1", rr.Header().Get("X-Found-Org-ID"))
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("invalid key format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Basic user:pass")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("wrong key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}
