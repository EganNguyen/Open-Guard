package resilience

import (
	"context"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
)

// ErrCircuitOpen is returned when the breaker stops requests.
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")

type BreakerConfig struct {
	Name             string
	Timeout          time.Duration // request timeout
	MaxRequests      uint32        // max requests in half-open state
	Interval         time.Duration // stat collection window
	FailureThreshold uint32        // failures before opening
	OpenDuration     time.Duration // time before moving to half-open
}

// NewBreaker creates a circuit breaker with standard OpenGuard defaults.
func NewBreaker(cfg BreakerConfig) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.OpenDuration,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.FailureThreshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			// In a real application, emit prometheus metrics here
			// e.g. openguard_circuit_breaker_state_change{name, from, to}
		},
	})
}

// Call executes fn through the circuit breaker with a context timeout.
// Returns ErrCircuitOpen if the breaker is open.
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := cb.Execute(func() (any, error) {
		return fn(ctx)
	})
	if err != nil {
		var zero T
		if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
			return zero, fmt.Errorf("%w: %s", ErrCircuitOpen, cb.Name())
		}
		return zero, err
	}
	return result.(T), nil
}
