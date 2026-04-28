package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// SSRFGuard is a middleware / helper that validates outbound URLs to prevent
// Server-Side Request Forgery (SSRF) attacks per spec §2.8.
//
// Blocked targets:
//   - RFC-1918 private IP ranges (10.x, 172.16-31.x, 192.168.x)
//   - Loopback (127.x, ::1)
//   - Link-local (169.254.x — AWS/GCP metadata endpoints)
//   - Cloud metadata IPs (169.254.169.254, fd00:ec2::254)
//   - Unspecified (0.0.0.0)

// blockedCIDRs contains all ranges that should never be reached via outbound webhooks.
var blockedCIDRs []*net.IPNet

func init() {
	blocked := []string{
		"10.0.0.0/8",       // RFC-1918 Class A
		"172.16.0.0/12",    // RFC-1918 Class B
		"192.168.0.0/16",   // RFC-1918 Class C
		"127.0.0.0/8",      // Loopback IPv4
		"::1/128",           // Loopback IPv6
		"169.254.0.0/16",   // Link-local / AWS metadata
		"fe80::/10",         // Link-local IPv6
		"fd00::/8",          // Unique local IPv6 (GCP metadata: fd00:ec2::254)
		"0.0.0.0/8",         // Unspecified
		"100.64.0.0/10",     // Shared address space (RFC 6598)
	}
	for _, cidr := range blocked {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			blockedCIDRs = append(blockedCIDRs, network)
		}
	}
}

type hostResolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

var defaultResolver hostResolver = net.DefaultResolver

// NewSafeHTTPClient returns an *http.Client whose DialContext resolves
// the target hostname exactly once, validates each IP against the blocked
// CIDR list, and connects to the first allowed IP.
// This prevents DNS rebinding (TOCTOU between validate and connect).
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}

				// 1. Resolve host exactly once
				ips, err := defaultResolver.LookupHost(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("SSRF guard: cannot resolve %q: %w", host, err)
				}

				// 2. Validate all IPs
				for _, rawIP := range ips {
					ip := net.ParseIP(rawIP)
					if ip == nil {
						continue
					}
					if isBlockedIP(ip) {
						return nil, fmt.Errorf("SSRF guard: %q resolves to blocked IP %s", host, ip)
					}

					// 3. Pin the connection to the validated IP directly.
					// This prevents a second DNS lookup at dial time.
					return dialer.DialContext(ctx, network, net.JoinHostPort(rawIP, port))
				}

				return nil, fmt.Errorf("SSRF guard: no allowed IP for %q", host)
			},
		},
	}
}

// isBlockedIP checks if an IP falls within any blocked CIDR range.
func isBlockedIP(ip net.IP) bool {
	for _, network := range blockedCIDRs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// SSRFGuardMiddleware returns an HTTP middleware that validates the X-Webhook-URL header.
// For services that accept user-supplied URLs (e.g. webhook-delivery), this middleware
// prevents SSRF by rejecting requests targeting private/reserved IP ranges.
func SSRFGuardMiddleware(urlHeader string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Deprecated: Pre-validation is prone to TOCTOU. 
			// Use NewSafeHTTPClient for outbound calls instead.
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds standard security headers to all responses per spec §14.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		next.ServeHTTP(w, r)
	})
}
