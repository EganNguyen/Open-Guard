package resilience

import (
	"context"
	"errors"
	"fmt"
)

var ErrBulkheadFull = errors.New("bulkhead full")

// Bulkhead limits concurrent executions of a function to protect system boundaries.
type Bulkhead struct {
	sem chan struct{}
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
	return &Bulkhead{sem: make(chan struct{}, maxConcurrent)}
}

func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
		return fn()
	case <-ctx.Done():
		return fmt.Errorf("%w: bulkhead execution cancelled due to timeout or capacity", ErrBulkheadFull)
	}
}
