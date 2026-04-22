package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
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

// ValidateOutboundURL checks if the given URL is safe for outbound requests.
// Returns an error if the URL resolves to a blocked IP range.
//
// Usage in webhook delivery handlers:
//
//	if err := middleware.ValidateOutboundURL(webhookURL); err != nil {
//	    return fmt.Errorf("SSRF protection: %w", err)
//	}
func ValidateOutboundURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http/https schemes
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("disallowed scheme: %q (only http/https allowed)", parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Resolve hostname to IPs
	addrs, err := net.LookupHost(host)
	if err != nil {
		// If we can't resolve, fail-closed
		return fmt.Errorf("cannot resolve host %q: %w", host, err)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("invalid IP address resolved: %q", addr)
		}
		if isBlockedIP(ip) {
			return fmt.Errorf("SSRF blocked: host %q resolves to private/reserved IP %s", host, ip)
		}
	}

	return nil
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
			targetURL := r.Header.Get(urlHeader)
			if targetURL == "" {
				// If no URL header, let the handler decide (may not need SSRF protection)
				next.ServeHTTP(w, r)
				return
			}

			if err := ValidateOutboundURL(targetURL); err != nil {
				http.Error(w, fmt.Sprintf("Forbidden: %v", err), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
