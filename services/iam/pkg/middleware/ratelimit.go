package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type entry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// RateLimiter implements a simple memory-based rate limiter with TTL eviction.
type RateLimiter struct {
	ips map[string]*entry
	mu  sync.Mutex
	r   rate.Limit
	b   int
}

// NewRateLimiter creates a new rate limiter with r requests per second and burst b.
func NewRateLimiter(r rate.Limit, b int, stop <-chan struct{}) *RateLimiter {
	l := &RateLimiter{
		ips: make(map[string]*entry),
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

// Limit is a middleware that rate limits requests based on IP address.
func (l *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		limiter := l.getLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
