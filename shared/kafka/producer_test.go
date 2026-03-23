package kafka

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/openguard/shared/models"
	"github.com/stretchr/testify/assert"
)

func TestNewProducer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	p := NewProducer([]string{"localhost:9092"}, []string{"topic-A"}, logger)

	assert.NotNil(t, p.writers["topic-A"])
	assert.Nil(t, p.writers["topic-B"])
}

func TestPublishEvent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	p := NewProducer([]string{"localhost:9092"}, []string{"topic-A"}, logger)

	t.Run("invalid topic", func(t *testing.T) {
		err := p.PublishEvent(context.Background(), "unknown.topic", models.EventEnvelope{})
		assert.ErrorContains(t, err, "no writer configured for topic")
	})

	t.Run("broker error", func(t *testing.T) {
		pBad := NewProducer([]string{"127.0.0.1:65535"}, []string{"topic-A"}, logger)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := pBad.PublishEvent(ctx, "topic-A", models.EventEnvelope{})
		assert.Error(t, err) // Expect a timeout/connection refused error
	})
}

func TestPublishRaw(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	p := NewProducer([]string{"localhost:9092"}, []string{"topic-A"}, logger)

	t.Run("invalid topic", func(t *testing.T) {
		err := p.PublishRaw(context.Background(), "unknown.topic", []byte("key"), []byte("data"))
		assert.ErrorContains(t, err, "no writer configured for topic")
	})

	t.Run("broker error raw", func(t *testing.T) {
		pBad := NewProducer([]string{"127.0.0.1:65535"}, []string{"topic-A"}, logger)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := pBad.PublishRaw(ctx, "topic-A", []byte("key"), []byte("data"))
		assert.Error(t, err) 
	})

	t.Run("close", func(t *testing.T) {
		pBad := NewProducer([]string{"127.0.0.1:65535"}, []string{"topic-A"}, logger)
		err := pBad.Close()
		assert.NoError(t, err) // Close doesn't establish connection, it just cleans up Go resources
	})
}
