package sdk

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Allow_FailClosed(t *testing.T) {
	// Server that always fails
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key")
	defer c.Close()

	allowed, err := c.Allow(context.Background(), "user-1", "read", "file-1")
	require.NoError(t, err)
	require.False(t, allowed, "expected denied (fail-closed)")
}

func TestClient_Allow_FailOpen(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", WithFailOpen(true))
	defer c.Close()

	allowed, err := c.Allow(context.Background(), "user-1", "read", "file-1")
	require.NoError(t, err)
	require.True(t, allowed, "expected allowed (fail-open)")
}

func TestClient_Allow_StaleCache(t *testing.T) {
	var count int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		if atomic.LoadInt32(&count) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"allowed": true}`)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	// Short TTL for testing
	c := NewClient(ts.URL, "test-key", WithCacheTTL(100*time.Millisecond))
	defer c.Close()

	// 1. First call: success, cached
	allowed, err := c.Allow(context.Background(), "user-1", "read", "file-1")
	require.NoError(t, err)
	require.True(t, allowed)

	// 2. Wait for expiry but within grace period (60s)
	time.Sleep(200 * time.Millisecond)

	// 3. Second call: server fails, should serve stale from cache
	allowed, err = c.Allow(context.Background(), "user-1", "read", "file-1")
	require.NoError(t, err)
	require.True(t, allowed, "expected allowed from stale cache")
}

func TestClient_Allow_StaleGracePeriod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", WithCacheTTL(100*time.Millisecond))
	defer c.Close()

	// Pre-seed cache with an expired entry but within 60s grace
	c.cache.mu.Lock()
	c.cache.data["user-1:read:file-1"] = cacheEntry{
		value:     true,
		expiresAt: time.Now().Add(-1 * time.Second), // Expired 1s ago
	}
	c.cache.mu.Unlock()

	// Should return true (stale) because server is down and we are within 60s grace
	allowed, err := c.Allow(context.Background(), "user-1", "read", "file-1")
	require.NoError(t, err)
	require.True(t, allowed, "expected allowed (stale-while-unavailable) within grace period")

	// Move entry beyond grace period
	c.cache.mu.Lock()
	c.cache.data["user-1:read:file-1"] = cacheEntry{
		value:     true,
		expiresAt: time.Now().Add(-61 * time.Second), // Expired 61s ago
	}
	c.cache.mu.Unlock()

	// Should return false (fail-closed) because grace period exceeded
	allowed, err = c.Allow(context.Background(), "user-1", "read", "file-1")
	require.NoError(t, err)
	require.False(t, allowed, "expected denied because grace period exceeded")
}

func TestClient_CircuitBreaker(t *testing.T) {
	var count int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	// Threshold 2
	c := NewClient(ts.URL, "test-key", WithCircuitBreaker(2, 1*time.Second))
	defer c.Close()

	// Call 1: failure
	_, _ = c.Allow(context.Background(), "u1", "a1", "r1")
	// Call 2: failure, should trip breaker
	_, _ = c.Allow(context.Background(), "u1", "a1", "r1")

	initialCount := atomic.LoadInt32(&count)

	// Call 3: breaker should be open, no request to server
	_, _ = c.Allow(context.Background(), "u1", "a1", "r1")

	if atomic.LoadInt32(&count) != initialCount {
		t.Errorf("expected no more requests after breaker tripped, got %d", atomic.LoadInt32(&count))
	}
}

func TestClient_Retry(t *testing.T) {
	var count int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		if atomic.LoadInt32(&count) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"allowed": true}`)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", WithRetry(2, 10*time.Millisecond, false))
	defer c.Close()

	allowed, err := c.Allow(context.Background(), "u1", "a1", "r1")
	require.NoError(t, err)
	require.True(t, allowed, "expected (true, nil) after retries")

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("expected 3 total attempts, got %d", atomic.LoadInt32(&count))
	}
}

func TestWithMTLS_VerifiesCertificate(t *testing.T) {
	// 1. Create a TLS server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"allowed": true}`)
	}))
	defer ts.Close()

	// 2. Save server cert to temp file
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ts.Certificate().Raw,
	})
	tmpCert, err := os.CreateTemp("", "ca.pem")
	require.NoError(t, err)

	defer func() {
		_ = os.Remove(tmpCert.Name())
	}()

	_, err = tmpCert.Write(certPEM)
	require.NoError(t, err)

	err = tmpCert.Close()
	require.NoError(t, err)

	// 3. Create client with WithMTLS providing the server's cert as CA
	c := NewClient(ts.URL, "test-key", WithMTLS(tmpCert.Name(), "", ""))
	defer c.Close()

	allowed, err := c.Allow(context.Background(), "u1", "a1", "r1")
	require.NoError(t, err)
	require.True(t, allowed, "expected allowed")
}

func TestWithMTLS_RejectsBadCertificate(t *testing.T) {
	// 1. Create a TLS server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"allowed": true}`)
	}))
	defer ts.Close()

	// 2. Create client WITHOUT providing the server's CA
	c := NewClient(ts.URL, "test-key")
	defer c.Close()

	// 3. Evaluation should return false (fail-closed) and nil error
	allowed, err := c.Allow(context.Background(), "u1", "a1", "r1")
	require.NoError(t, err, "expected nil error (fail-closed)")
	require.False(t, allowed, "expected denied (fail-closed) on TLS error")
}

func TestWithInsecureSkipVerify_SkipsVerification(t *testing.T) {
	// 1. Create a TLS server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"allowed": true}`)
	}))
	defer ts.Close()

	// 2. Create client with WithInsecureSkipVerify()
	c := NewClient(ts.URL, "test-key", WithInsecureSkipVerify())
	defer c.Close()

	// 3. Evaluation should succeed despite untrusted cert
	allowed, err := c.Allow(context.Background(), "u1", "a1", "r1")
	require.NoError(t, err)
	require.True(t, allowed, "expected allowed")
}
