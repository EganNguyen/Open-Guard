package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConnect(t *testing.T) {
	t.Run("parse error", func(t *testing.T) {
		_, err := Connect(context.Background(), "invalid-dsn::bad_port")
		assert.ErrorContains(t, err, "parse dsn")
	})

	t.Run("ping error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		pool, err := Connect(ctx, "postgres://fake:fake@127.0.0.1:65535/iamdb?sslmode=disable")
		assert.ErrorContains(t, err, "ping")
		assert.Nil(t, pool)
	})
}
