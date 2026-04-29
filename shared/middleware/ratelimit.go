package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

type entry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// RateLimiter implements a Redis-backed sliding window rate limiter.
type RateLimiter struct {
	ips map[string]*entry
	mu  sync.Mutex
	rdb *redis.Client
	r   rate.Limit
	b   int
}

// NewRateLimiter creates a new rate limiter with r requests per second and burst b.
func NewRateLimiter(rdb *redis.Client, r rate.Limit, b int, stop <-chan struct{}) *RateLimiter {
	l := &RateLimiter{
		ips: make(map[string]*entry),
		rdb: rdb,
		r:   r,
		b:   b,
	}

	// Periodic cleanup to prevent memory leak (R-01)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.cleanup()
			case <-stop:
				return
			}
		}
	}()

	return l
}

func (l *RateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	for ip, e := range l.ips {
		if e.lastAccess.Before(cutoff) {
			delete(l.ips, ip)
		}
	}
}

func extractIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For can be a comma-separated list; take the first
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *RateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, exists := l.ips[ip]
	if !exists {
		e = &entry{
			limiter: rate.NewLimiter(l.r, l.b),
		}
		l.ips[ip] = e
	}

	e.lastAccess = time.Now()
	return e.limiter
}

func (l *RateLimiter) isAllowed(ctx context.Context, ip string) bool {
	// 1. Fast path: In-memory check (for performance and as fallback)
	limiter := l.getLimiter(ip)
	if !limiter.Allow() {
		return false
	}

	// 2. Distributed path: Redis sliding window
	if l.rdb == nil {
		return true // Fallback to memory-only if Redis is missing
	}

	now := time.Now().UnixNano() / int64(time.Millisecond)
	window := now - int64(time.Minute/time.Millisecond) // 1 minute window
	key := "ratelimit:" + ip

	pipe := l.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", window))
	pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: now})
	pipe.Expire(ctx, key, time.Minute)
	cmds, err := pipe.Exec(ctx)

	if err != nil {
		return true // Fallback to memory on Redis error
	}

	count := cmds[1].(*redis.IntCmd).Val()
	return count <= int64(l.b)
}

// Limit is a middleware that rate limits requests based on IP address.
func (l *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !l.isAllowed(r.Context(), ip) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
