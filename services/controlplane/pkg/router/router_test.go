package router

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

type mockRedis struct {
	redis.UniversalClient
}

func (m *mockRedis) Pipeline() redis.Pipeliner {
	return &mockPipe{}
}

type mockPipe struct {
	redis.Pipeliner
}

func (m *mockPipe) ZRemRangeByScore(ctx context.Context, key, min, max string) *redis.IntCmd {
	return redis.NewIntCmd(ctx)
}
func (m *mockPipe) ZCard(ctx context.Context, key string) *redis.IntCmd {
	return redis.NewIntCmd(ctx)
}
func (m *mockPipe) ZAdd(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	return redis.NewIntCmd(ctx)
}
func (m *mockPipe) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	return redis.NewBoolCmd(ctx)
}
func (m *mockPipe) Exec(ctx context.Context) ([]redis.Cmder, error) {
	return nil, nil
}

func TestRouter_Config(t *testing.T) {
	keyring := crypto.NewJWTKeyring([]crypto.JWTKey{{Kid: "k1", Secret: "s1", Algorithm: "HS256", Status: "active"}})
	
	cfg := Config{
		JWTKeyring: keyring,
		Redis:      &mockRedis{},
		Logger:     slog.New(slog.NewTextHandler(os.Stderr, nil)),
		IAMAddr:    "http://localhost:8081",
		PolicyAddr: "http://localhost:8082",
	}

	r, err := New(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, r)

	// Test health endpoints
	tests := []string{"/health", "/health/live", "/health/ready"}
	for _, path := range tests {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"status":"ok"`)
	}
}

func TestServiceUnavailableHandler(t *testing.T) {
	h := serviceUnavailableHandler("test-svc", "", slog.Default(), nil)
	
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	rec := httptest.NewRecorder()
	
	h.ServeHTTP(rec, req)
	
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "test-svc service is not available yet")
}

func TestServiceUnavailableHandler_WithAddr(t *testing.T) {
	// Should create a proxy
	h := serviceUnavailableHandler("test-svc", "http://localhost:12345", slog.Default(), nil)
	assert.NotNil(t, h)
	
	// Since there is no actual server, it should return 503 if we call it
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
