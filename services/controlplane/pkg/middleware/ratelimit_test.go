package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockPipeliner mocks the redis.Pipeliner interface
type MockPipeliner struct {
	mock.Mock
	redis.Pipeliner
}

func (m *MockPipeliner) ZRemRangeByScore(ctx context.Context, key, min, max string) *redis.IntCmd {
	m.Called(ctx, key, min, max)
	return redis.NewIntCmd(ctx)
}

func (m *MockPipeliner) ZCard(ctx context.Context, key string) *redis.IntCmd {
	args := m.Called(ctx, key)
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(int64(args.Int(0)))
	return cmd
}

func (m *MockPipeliner) ZAdd(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	m.Called(ctx, key, members)
	return redis.NewIntCmd(ctx)
}

func (m *MockPipeliner) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	m.Called(ctx, key, expiration)
	return redis.NewBoolCmd(ctx)
}

func (m *MockPipeliner) Exec(ctx context.Context) ([]redis.Cmder, error) {
	args := m.Called(ctx)
	return nil, args.Error(0)
}

// MockRedis mocks the redis.UniversalClient interface
type MockRedis struct {
	mock.Mock
	redis.UniversalClient
}

func (m *MockRedis) Pipeline() redis.Pipeliner {
	args := m.Called()
	return args.Get(0).(redis.Pipeliner)
}

func TestRateLimiter_Middleware_Authenticated(t *testing.T) {
	mr := new(MockRedis)
	mp := new(MockPipeliner)
	logger := slog.Default()
	rl := NewRateLimiter(mr, logger, 5, 10, time.Minute)

	mr.On("Pipeline").Return(mp)
	mp.On("ZRemRangeByScore", mock.Anything, "rl:user:u1", "0", mock.Anything).Return()
	mp.On("ZCard", mock.Anything, "rl:user:u1").Return(0) // 0 existing requests
	mp.On("ZAdd", mock.Anything, "rl:user:u1", mock.Anything).Return()
	mp.On("Expire", mock.Anything, "rl:user:u1", mock.Anything).Return()
	mp.On("Exec", mock.Anything).Return(nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-User-ID", "u1")
	rec := httptest.NewRecorder()

	rl.Middleware()(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "9", rec.Header().Get("X-RateLimit-Remaining"))
}

func TestRateLimiter_Middleware_Exceeded(t *testing.T) {
	mr := new(MockRedis)
	mp := new(MockPipeliner)
	logger := slog.Default()
	rl := NewRateLimiter(mr, logger, 5, 10, time.Minute)

	mr.On("Pipeline").Return(mp)
	mp.On("ZRemRangeByScore", mock.Anything, "rl:ip:127.0.0.1", "0", mock.Anything).Return()
	mp.On("ZCard", mock.Anything, "rl:ip:127.0.0.1").Return(5) // Limit is 5
	mp.On("ZAdd", mock.Anything, "rl:ip:127.0.0.1", mock.Anything).Return()
	mp.On("Expire", mock.Anything, "rl:ip:127.0.0.1", mock.Anything).Return()
	mp.On("Exec", mock.Anything).Return(nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1"
	rec := httptest.NewRecorder()

	rl.Middleware()(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimiter_Middleware_RedisError_FailOpen(t *testing.T) {
	mr := new(MockRedis)
	mp := new(MockPipeliner)
	logger := slog.Default()
	rl := NewRateLimiter(mr, logger, 5, 10, time.Minute)

	mr.On("Pipeline").Return(mp)
	mp.On("ZRemRangeByScore", mock.Anything, "rl:ip:127.0.0.1", "0", mock.Anything).Return()
	mp.On("ZCard", mock.Anything, "rl:ip:127.0.0.1").Return(0)
	mp.On("ZAdd", mock.Anything, "rl:ip:127.0.0.1", mock.Anything).Return()
	mp.On("Expire", mock.Anything, "rl:ip:127.0.0.1", mock.Anything).Return()
	mp.On("Exec", mock.Anything).Return(errors.New("redis down"))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1"
	rec := httptest.NewRecorder()

	rl.Middleware()(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
