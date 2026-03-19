package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/shared/models"
	kafkago "github.com/segmentio/kafka-go"
)

// Producer wraps a kafka-go writer for publishing EventEnvelopes.
type Producer struct {
	writers map[string]*kafkago.Writer
	logger  *slog.Logger
}

// NewProducer creates a producer that can write to the given topics.
// brokers is a comma-separated list of Kafka broker addresses.
func NewProducer(brokers []string, topics []string, logger *slog.Logger) *Producer {
	writers := make(map[string]*kafkago.Writer, len(topics))
	for _, topic := range topics {
		writers[topic] = &kafkago.Writer{
			Addr:         kafkago.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafkago.LeastBytes{},
			BatchTimeout: 10 * time.Millisecond,
			RequiredAcks: kafkago.RequireOne,
		}
	}
	return &Producer{writers: writers, logger: logger}
}

// PublishEvent publishes an EventEnvelope to the specified topic.
func (p *Producer) PublishEvent(ctx context.Context, topic string, envelope models.EventEnvelope) error {
	writer, ok := p.writers[topic]
	if !ok {
		return fmt.Errorf("no writer configured for topic %q", topic)
	}

	if envelope.ID == "" {
		envelope.ID = uuid.New().String()
	}
	if envelope.OccurredAt.IsZero() {
		envelope.OccurredAt = time.Now().UTC()
	}
	if envelope.SchemaVer == "" {
		envelope.SchemaVer = "1.0"
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	err = writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(envelope.OrgID),
		Value: data,
	})
	if err != nil {
		p.logger.Error("failed to publish event",
			"topic", topic,
			"event_type", envelope.Type,
			"error", err,
		)
		return fmt.Errorf("publish to %q: %w", topic, err)
	}

	p.logger.Debug("event published",
		"topic", topic,
		"event_type", envelope.Type,
		"event_id", envelope.ID,
	)
	return nil
}

// PublishRaw publishes a pre-serialized raw payload to the specified topic.
func (p *Producer) PublishRaw(ctx context.Context, topic string, key []byte, payload []byte) error {
	writer, ok := p.writers[topic]
	if !ok {
		return fmt.Errorf("no writer configured for topic %q", topic)
	}

	err := writer.WriteMessages(ctx, kafkago.Message{
		Key:   key,
		Value: payload,
	})
	if err != nil {
		p.logger.Error("failed to publish raw event",
			"topic", topic,
			"error", err,
		)
		return fmt.Errorf("publish raw to %q: %w", topic, err)
	}
	return nil
}

// Close closes all underlying writers.
func (p *Producer) Close() error {
	var lastErr error
	for topic, writer := range p.writers {
		if err := writer.Close(); err != nil {
			p.logger.Error("failed to close writer", "topic", topic, "error", err)
			lastErr = err
		}
	}
	return lastErr
}
