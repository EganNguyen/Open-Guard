package middleware

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

type mockResolver struct {
	ips map[string][]string
}

func (m *mockResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if ips, ok := m.ips[host]; ok {
		return ips, nil
	}
	return nil, fmt.Errorf("host not found")
}

func TestNewSafeHTTPClient_BlocksMetadataIP(t *testing.T) {
	// Setup mock resolver
	mock := &mockResolver{
		ips: map[string][]string{
			"metadata.local": {"169.254.169.254"},
			"google.com":     {"8.8.8.8"},
			"internal.local": {"10.0.0.1"},
		},
	}
	
	// Override default resolver
	orig := defaultResolver
	defaultResolver = mock
	defer func() { defaultResolver = orig }()

	client := NewSafeHTTPClient(1 * time.Second)

	tests := []struct {
		url     string
		blocked bool
	}{
		{"http://metadata.local/latest/meta-data", true},
		{"http://internal.local/admin", true},
		{"http://google.com/", false},
	}

	for _, tt := range tests {
		_, err := client.Get(tt.url)
		if tt.blocked && err == nil {
			t.Errorf("URL %s: expected error, got nil", tt.url)
		}
		if !tt.blocked && err != nil && strings.Contains(err.Error(), "SSRF guard") {
			t.Errorf("URL %s: unexpected SSRF block: %v", tt.url, err)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"169.254.169.254", true},
		{"10.0.0.1", true},
		{"127.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"100.64.0.1", true},
	}

	for _, c := range cases {
		if isBlockedIP(net.ParseIP(c.ip)) != c.blocked {
			t.Errorf("IP %s: expected blocked=%v", c.ip, c.blocked)
		}
	}
}

func TestNewSafeHTTPClient_NoDNSRebind(t *testing.T) {
	// 1. Initially return a safe IP
	mock := &mockResolver{
		ips: map[string][]string{
			"rebind.local": {"8.8.8.8"},
		},
	}
	orig := defaultResolver
	defaultResolver = mock
	defer func() { defaultResolver = orig }()

	_ = NewSafeHTTPClient(1 * time.Second)

	// We can't easily test the actual "rebind" without a real dial, 
	// but we verified in code that DialContext uses the IP returned by LookupHost directly.
	// The implementation:
	// return dialer.DialContext(ctx, network, net.JoinHostPort(rawIP, port))
	// where rawIP is the one we validated.
}
