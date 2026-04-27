package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
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
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if allowed {
		t.Error("expected denied (fail-closed)")
	}
}

func TestClient_Allow_FailOpen(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", WithFailOpen(true))
	defer c.Close()

	allowed, err := c.Allow(context.Background(), "user-1", "read", "file-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !allowed {
		t.Error("expected allowed (fail-open)")
	}
}

func TestClient_Allow_StaleCache(t *testing.T) {
	var count int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		if atomic.LoadInt32(&count) == 1 {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"allowed": true}`)
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
	if err != nil || !allowed {
		t.Fatalf("expected (true, nil), got (%v, %v)", allowed, err)
	}

	// 2. Wait for expiry but within grace period (60s)
	time.Sleep(200 * time.Millisecond)

	// 3. Second call: server fails, should serve stale from cache
	allowed, err = c.Allow(context.Background(), "user-1", "read", "file-1")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !allowed {
		t.Error("expected allowed from stale cache")
	}
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
	c.Allow(context.Background(), "u1", "a1", "r1")
	// Call 2: failure, should trip breaker
	c.Allow(context.Background(), "u1", "a1", "r1")

	initialCount := atomic.LoadInt32(&count)
	
	// Call 3: breaker should be open, no request to server
	c.Allow(context.Background(), "u1", "a1", "r1")
	
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
		fmt.Fprint(w, `{"allowed": true}`)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", WithRetry(2, 10*time.Millisecond, false))
	defer c.Close()

	allowed, err := c.Allow(context.Background(), "u1", "a1", "r1")
	if err != nil || !allowed {
		t.Fatalf("expected (true, nil) after retries, got (%v, %v)", allowed, err)
	}

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("expected 3 total attempts, got %d", atomic.LoadInt32(&count))
	}
}
