package proxy

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openguard/shared/resilience"
	"github.com/stretchr/testify/assert"
)

func TestCircuitBreakerTransport(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "test-breaker",
		FailureThreshold: 1, // Trip after 1 failure
		OpenDuration:     1 * time.Second,
	})

	// 1. Success case
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transport := &CircuitBreakerTransport{
		breaker:   breaker,
		transport: http.DefaultTransport,
		timeout:   2 * time.Second,
	}

	req := httptest.NewRequest("GET", ts.URL, nil)
	resp, err := transport.RoundTrip(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 2. Failure leads to circuit trip
	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // Doesn't trip breaker by itself, but we can force error
	}))
	defer tsFail.Close()

	// Simulate a real error (like connection refused) by using a bad URL
	reqBad := httptest.NewRequest("GET", "http://localhost:12345", nil)
	_, err = transport.RoundTrip(reqBad)
	assert.Error(t, err)

	// 3. Circuit should now be open
	time.Sleep(100 * time.Millisecond) // Give time for breaker state to update
	_, err = transport.RoundTrip(req)
	assert.Contains(t, err.Error(), "circuit breaker is open")
}

func TestNewReverseProxy(t *testing.T) {
	logger := slog.Default()
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "proxy-test"})
	
	proxy, err := NewReverseProxy("http://localhost:8081", logger, breaker, nil)
	assert.NoError(t, err)
	assert.NotNil(t, proxy)

	// Test ErrorHandler
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	
	// Case 1: Generic error
	proxy.ErrorHandler(rec, req, errors.New("upstream timeout"))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "SERVICE_UNAVAILABLE")

	// Case 2: Circuit open error
	rec2 := httptest.NewRecorder()
	proxy.ErrorHandler(rec2, req, resilience.ErrCircuitOpen)
	assert.Equal(t, http.StatusServiceUnavailable, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "CIRCUIT_OPEN")
}

func TestNewReverseProxy_InvalidURL(t *testing.T) {
	_, err := NewReverseProxy(":%:invalid", slog.Default(), nil, nil)
	assert.Error(t, err)
}
