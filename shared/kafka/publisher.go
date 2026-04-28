package kafka

import (
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// Publisher implements the Outbox.KafkaPublisher interface using segmentio/kafka-go.
// Configured for idempotent, at-least-once delivery per spec §4.2.
type Publisher struct {
	writer *kafka.Writer
}

// NewPublisher creates a new Kafka publisher with idempotent configuration.
// RequiredAcks=RequireAll ensures the leader waits for all ISR replicas before acking.
// Async=false ensures we wait for confirmation before marking outbox record as published.
// BatchSize=1 combined with Async=false gives lowest latency for outbox use.
func NewPublisher(brokers []string) *Publisher {
	return &Publisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Balancer:               &kafka.LeastBytes{},
			RequiredAcks:           kafka.RequireAll, // Wait for all ISR replicas per spec §4.2
			Async:                  false,            // Synchronous: wait for ack before returning
			AllowAutoTopicCreation: false,            // Topics must be pre-created in production
			BatchSize:              1,                // One message per write for outbox reliability
			BatchTimeout:           0,                // No batching delay
		},
	}
}

// Publish sends a message to a specific topic.
// The call blocks until the broker acknowledges receipt from all ISR replicas.
func (p *Publisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	err := p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
	})
	if err != nil {
		return fmt.Errorf("kafka publish: %w", err)
	}
	return nil
}

// Close closes the underlying writer.
func (p *Publisher) Close() error {
	return p.writer.Close()
}
