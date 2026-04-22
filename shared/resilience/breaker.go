package resilience

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

// BreakerConfig defines the settings for a circuit breaker.
type BreakerConfig struct {
	Name             string
	MaxRequests      uint32        // requests allowed in half-open state
	Interval         time.Duration // stat collection window
	FailureThreshold uint32        // consecutive failures before opening
	OpenDuration     time.Duration // time before moving to half-open
}

// NewBreaker creates a new gobreaker.CircuitBreaker with the given configuration.
func NewBreaker(cfg BreakerConfig, logger *slog.Logger) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.OpenDuration,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.FailureThreshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Warn("circuit breaker state changed",
				"name", name, "from", from.String(), "to", to.String())
		},
	})
}

// Call wraps a function call with a timeout and a circuit breaker.
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker,
	timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := cb.Execute(func() (any, error) { return fn(ctx) })
	if err != nil {
		var zero T
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return zero, fmt.Errorf("%w: %s", ErrCircuitOpen, cb.Name())
		}
		return zero, err
	}

	if result == nil {
		var zero T
		return zero, nil
	}

	return result.(T), nil
}
