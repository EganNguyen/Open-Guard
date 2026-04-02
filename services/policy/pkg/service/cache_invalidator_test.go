package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheInvalidator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c := NewCacheInvalidator(nil, []string{"127.0.0.1:65535"}, logger)
	assert.NotNil(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Exits cleanly if context is respected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start blocked after context cancellation")
	}

	err := c.Close()
	assert.NoError(t, err)
}
