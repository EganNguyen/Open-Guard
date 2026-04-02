package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetry(t *testing.T) {
	t.Run("success on first try", func(t *testing.T) {
		calls := 0
		err := Retry(context.Background(), 3, 1*time.Millisecond, func() error {
			calls++
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("success on third try", func(t *testing.T) {
		calls := 0
		err := Retry(context.Background(), 3, 1*time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return errors.New("transient error")
			}
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("fails all retries", func(t *testing.T) {
		calls := 0
		err := Retry(context.Background(), 2, 1*time.Millisecond, func() error {
			calls++
			return errors.New("fatal error")
		})
		assert.ErrorContains(t, err, "fatal error")
		assert.Equal(t, 2, calls)
	})

	t.Run("context cancelled mid-flight", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately
		
		calls := 0
		err := Retry(ctx, 3, 1*time.Millisecond, func() error {
			calls++
			return errors.New("error to trigger retry")
		})
		// It tries once, fails, checks context, and abandons
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, calls)
	})
}
