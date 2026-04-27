package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
)

type MockRoundTripper struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func TestCircuitBreakerTransport(t *testing.T) {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "test-cb",
		MaxRequests: 1,
		Interval:    0,
		Timeout:     1 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	})

	mockRT := &MockRoundTripper{}
	transport := &CircuitBreakerTransport{cb: cb, rt: mockRT}

	t.Run("Success Request", func(t *testing.T) {
		mockRT.roundTrip = func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		}
		req := httptest.NewRequest("GET", "http://test", nil)
		resp, err := transport.RoundTrip(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("Failure Trips Breaker", func(t *testing.T) {
		mockRT.roundTrip = func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network error")
		}
		req := httptest.NewRequest("GET", "http://test", nil)
		
		// First call: failure
		_, err := transport.RoundTrip(req)
		assert.Error(t, err)

		// Second call: breaker should be open
		_, err = transport.RoundTrip(req)
		assert.Error(t, err)
		assert.Equal(t, gobreaker.ErrOpenState, err)
	})
}
