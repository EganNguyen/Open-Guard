package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
)

func TestAuthJWTWithBlocklist_Alerting(t *testing.T) {
	// 1. Start miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	// 2. Create keyring and a valid token
	keyring := []crypto.JWTKey{
		{Kid: "test-key", Secret: "super-secret-key-that-is-at-least-32-bytes", Algorithm: "HS256", Status: "active"},
	}

	// Create a token
	claims := crypto.NewStandardClaims("org-456", "user-123", "jti-test-123", 1*time.Hour)

	tokenStr, err := crypto.Sign(claims, keyring)
	assert.NoError(t, err)

	// 3. Init Middleware with dummy handler
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name: "test-breaker",
	}, nil)
	middlewareFunc := middleware.AuthJWTWithBlocklist(keyring, rdb, breaker)
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	testHandler := middlewareFunc(dummyHandler)

	// 4. Test missing token
	req, _ := http.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	testHandler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	// 5. Test valid token
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr = httptest.NewRecorder()
	testHandler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// 6. Blocklist the token
	err = rdb.Set(req.Context(), "blocklist:jti-test-123", "revoked", 1*time.Hour).Err()
	assert.NoError(t, err)

	// 7. Test revoked token -> returns 401
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rr = httptest.NewRecorder()
	testHandler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
