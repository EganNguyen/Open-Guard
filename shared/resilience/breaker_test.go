package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBreaker_Call(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cb := NewBreaker(BreakerConfig{
			Name:             "test",
			MaxRequests:      1,
			Interval:         1 * time.Second,
			FailureThreshold: 2,
			OpenDuration:     1 * time.Second,
		})

		res, err := Call(context.Background(), cb, 1*time.Second, func(ctx context.Context) (string, error) {
			return "ok", nil
		})
		assert.NoError(t, err)
		assert.Equal(t, "ok", res)
	})

	t.Run("trip breaker", func(t *testing.T) {
		cb := NewBreaker(BreakerConfig{
			Name:             "test-trip",
			MaxRequests:      1,
			Interval:         1 * time.Second,
			FailureThreshold: 1,
			OpenDuration:     5 * time.Second,
		})

		// First call fails
		_, err := Call(context.Background(), cb, 1*time.Second, func(ctx context.Context) (string, error) {
			return "", errors.New("underlying error")
		})
		assert.ErrorContains(t, err, "underlying error")

		// Second call should fast-fail due to open circuit
		_, err = Call(context.Background(), cb, 1*time.Second, func(ctx context.Context) (string, error) {
			return "ignored", nil
		})
		assert.ErrorIs(t, err, ErrCircuitOpen)
	})
}
