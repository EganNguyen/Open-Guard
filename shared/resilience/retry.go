package resilience

import (
	"context"
	"math/rand"
	"time"
)

// Retry executes fn with exponential backoff and jitter.
func Retry(ctx context.Context, attempts int, initialDelay time.Duration, fn func() error) error {
	var err error
	delay := initialDelay

	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if i == attempts-1 {
			break
		}

		jitter := time.Duration(rand.Int63n(int64(delay) / 2))
		select {
		case <-time.After(delay + jitter):
		case <-ctx.Done():
			return ctx.Err()
		}
		delay *= 2
	}
	return err
}
