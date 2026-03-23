package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBulkhead_Execute(t *testing.T) {
	b := NewBulkhead(1)

	// 1. Success on first try
	err := b.Execute(context.Background(), func() error { return nil })
	assert.NoError(t, err)

	// 2. Block the bulkhead
	blockCh := make(chan struct{})
	go b.Execute(context.Background(), func() error {
		<-blockCh
		return nil
	})
	time.Sleep(50 * time.Millisecond) // Ensure the goroutine acquires the semaphore

	// 3. Second request fails because of context timeout and full bulkhead
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = b.Execute(ctx, func() error { return nil })
	assert.ErrorIs(t, err, ErrBulkheadFull)
	cancel()

	// Release the bulkhead lock
	close(blockCh)
}

func TestBulkhead_InnerError(t *testing.T) {
	b := NewBulkhead(1)
	err := b.Execute(context.Background(), func() error {
		return errors.New("inner error")
	})
	assert.ErrorContains(t, err, "inner error")
}
