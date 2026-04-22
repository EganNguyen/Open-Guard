package kafka

import (
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
)

// Publisher implements the Outbox.KafkaPublisher interface using segmentio/kafka-go.
type Publisher struct {
	writer *kafka.Writer
}

// NewPublisher creates a new Kafka publisher.
func NewPublisher(brokers []string) *Publisher {
	return &Publisher{
		writer: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Balancer: &kafka.LeastBytes{},
			Async:    false, // Wait for confirmation for outbox reliability
		},
	}
}

// Publish sends a message to a specific topic.
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
