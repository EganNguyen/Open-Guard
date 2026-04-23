package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter implements a simple memory-based rate limiter.
type RateLimiter struct {
	ips map[string]*rate.Limiter
	mu  sync.Mutex
	r   rate.Limit
	b   int
}

// NewRateLimiter creates a new rate limiter with r requests per second and burst b.
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	l := &RateLimiter{
		ips: make(map[string]*rate.Limiter),
		r:   r,
		b:   b,
	}

	// Periodic cleanup to prevent memory leak (R-01)
	go func() {
		for range time.Tick(15 * time.Minute) {
			l.mu.Lock()
			// Simple strategy: clear all. More advanced would be to check last access time.
			l.ips = make(map[string]*rate.Limiter)
			l.mu.Unlock()
		}
	}()

	return l
}

func (l *RateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	limiter, exists := l.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(l.r, l.b)
		l.ips[ip] = limiter
	}

	return limiter
}

// Limit is a middleware that rate limits requests based on IP address.
func (l *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		limiter := l.getLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
