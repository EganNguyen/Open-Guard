package resilience

import (
	"context"
	"errors"

	"golang.org/x/sync/semaphore"
)

var ErrBulkheadFull = errors.New("bulkhead full")

type Bulkhead struct {
	sem *semaphore.Weighted
	cap int64
}

func NewBulkhead(capacity int) *Bulkhead {
	return &Bulkhead{
		sem: semaphore.NewWeighted(int64(capacity)),
		cap: int64(capacity),
	}
}

func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
	if !b.sem.TryAcquire(1) {
		return ErrBulkheadFull
	}
	defer b.sem.Release(1)
	return fn()
}
