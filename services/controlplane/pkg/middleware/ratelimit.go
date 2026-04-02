package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/openguard/shared/models"
	"github.com/redis/go-redis/v9"
)

// RateLimiter is a Redis-based sliding window rate limiter.
type RateLimiter struct {
	client        redis.UniversalClient
	logger        *slog.Logger
	anonLimit     int           // requests per window for unauthenticated
	authLimit     int           // requests per window for authenticated
	window        time.Duration // sliding window duration
}

// NewRateLimiter creates a new rate limiter.
// anonLimit: max requests per window for unauthenticated users (by IP).
// authLimit: max requests per window for authenticated users (by user ID).
func NewRateLimiter(client redis.UniversalClient, logger *slog.Logger, anonLimit, authLimit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		client:    client,
		logger:    logger,
		anonLimit: anonLimit,
		authLimit: authLimit,
		window:    window,
	}
}

// Middleware returns an HTTP middleware that enforces rate limits.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Determine key and limit based on authentication
			userID := r.Header.Get("X-User-ID")
			var key string
			var limit int
			if userID != "" {
				key = fmt.Sprintf("rl:user:%s", userID)
				limit = rl.authLimit
			} else {
				key = fmt.Sprintf("rl:ip:%s", r.RemoteAddr)
				limit = rl.anonLimit
			}

			allowed, remaining, err := rl.check(ctx, key, limit)
			if err != nil {
				// Fail open in dev — log and allow request
				rl.logger.Warn("rate limiter error, failing open",
					"error", err,
					"key", key,
				)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if !allowed {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(rl.window.Seconds()), 10))
				models.WriteError(w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Rate limit exceeded. Please try again later.", r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// check uses a Redis sorted set sliding window to enforce rate limiting.
func (rl *RateLimiter) check(ctx context.Context, key string, limit int) (allowed bool, remaining int, err error) {
	now := time.Now()
	windowStart := now.Add(-rl.window)

	pipe := rl.client.Pipeline()

	// Remove entries outside the window
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixMicro(), 10))
	// Count entries in the window
	countCmd := pipe.ZCard(ctx, key)
	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixMicro()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})
	// Set expiry on the key
	pipe.Expire(ctx, key, rl.window+time.Second)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return false, 0, err
	}

	count := int(countCmd.Val())
	if count >= limit {
		return false, 0, nil
	}
	return true, limit - count - 1, nil
}
