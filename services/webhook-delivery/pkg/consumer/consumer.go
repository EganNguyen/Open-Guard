package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
}

type Deliverer interface {
	Deliver(ctx context.Context, messageKey, target, payload, secret string) error
}

type WebhookConsumer struct {
	reader    KafkaReader
	deliverer Deliverer
	publisher KafkaPublisher
	logger    *slog.Logger
}

type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

type WebhookDeliveryRequest struct {
	Target  string `json:"target"`
	Payload string `json:"payload"`
	Secret  string `json:"secret"`
	OrgID   string `json:"org_id"`
}

func NewWebhookConsumer(brokers string, groupID string, topic string, d Deliverer, pub KafkaPublisher, logger *slog.Logger) *WebhookConsumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        strings.Split(brokers, ","),
		GroupID:        groupID,
		Topic:          topic,
		CommitInterval: 0,
	})

	return &WebhookConsumer{
		reader:    r,
		deliverer: d,
		publisher: pub,
		logger:    logger,
	}
}

func (c *WebhookConsumer) Start(ctx context.Context) error {
	sem := make(chan struct{}, 50) // max 50 concurrent deliveries
	var wg sync.WaitGroup

	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return nil
			}
			c.logger.Error("failed to fetch message", "error", err)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(msg kafka.Message) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("panic in message processing", "error", r, "key", string(msg.Key))
				}
			}()

			if err := c.processMessage(ctx, msg); err != nil {
				c.logger.Error("message processing failed after all retries", "error", err)
			}

			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				c.logger.Error("failed to commit offset", "error", err)
			}
		}(m)
	}
}

func (c *WebhookConsumer) processMessage(ctx context.Context, m kafka.Message) error {
	var req WebhookDeliveryRequest
	if err := json.Unmarshal(m.Value, &req); err != nil {
		c.logger.Error("failed to unmarshal webhook request", "error", err)
		return nil
	}

	// Retry loop
	var lastErr error
	for i := 0; i < 5; i++ {
		err := c.deliverer.Deliver(ctx, string(m.Key), req.Target, req.Payload, req.Secret)
		if err == nil {
			c.logger.Info("webhook delivered", "target", req.Target, "org_id", req.OrgID)
			return nil
		}
		lastErr = err
		c.logger.Warn("webhook delivery attempt failed", "attempt", i+1, "target", req.Target, "error", err)

		// Backoff: 1s, 2s, 4s, 8s, 16s (context-aware)
		backoff := time.Duration(1<<i) * time.Second
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Final failure -> DLQ
	c.logger.Error("webhook delivery failed after 5 attempts, routing to DLQ", "target", req.Target, "error", lastErr)
	dlqPayload, _ := json.Marshal(map[string]interface{}{
		"request":   req,
		"error":     lastErr.Error(),
		"failed_at": time.Now(),
	})
	if err := c.publisher.Publish(ctx, "webhook.dlq", string(m.Key), dlqPayload); err != nil {
		c.logger.Error("failed to publish to DLQ", "error", err)
		return fmt.Errorf("DLQ publish failed: %w", err)
	}
	return nil
}
