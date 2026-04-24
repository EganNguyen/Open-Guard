package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/webhook-delivery/pkg/deliverer"
)

type WebhookConsumer struct {
	reader    *kafka.Reader
	deliverer *deliverer.Deliverer
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

func NewWebhookConsumer(brokers string, groupID string, topic string, d *deliverer.Deliverer, pub KafkaPublisher, logger *slog.Logger) *WebhookConsumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		GroupID: groupID,
		Topic:   topic,
	})

	return &WebhookConsumer{
		reader:    r,
		deliverer: d,
		publisher: pub,
		logger:    logger,
	}
}

func (c *WebhookConsumer) Start(ctx context.Context) error {
	for {
		m, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("failed to read message", "error", err)
			continue
		}

		go c.processMessage(ctx, m)
	}
}

func (c *WebhookConsumer) processMessage(ctx context.Context, m kafka.Message) {
	var req WebhookDeliveryRequest
	if err := json.Unmarshal(m.Value, &req); err != nil {
		c.logger.Error("failed to unmarshal webhook request", "error", err)
		return
	}

	// Retry loop
	var lastErr error
	for i := 0; i < 5; i++ {
		err := c.deliverer.Deliver(ctx, req.Target, req.Payload, req.Secret)
		if err == nil {
			c.logger.Info("webhook delivered", "target", req.Target, "org_id", req.OrgID)
			return
		}
		lastErr = err
		c.logger.Warn("webhook delivery attempt failed", "attempt", i+1, "target", req.Target, "error", err)
		
		// Backoff: 1s, 2s, 4s, 8s, 16s
		time.Sleep(time.Duration(1<<i) * time.Second)
	}

	// Final failure -> DLQ
	c.logger.Error("webhook delivery failed after 5 attempts, routing to DLQ", "target", req.Target, "error", lastErr)
	dlqPayload, _ := json.Marshal(map[string]interface{}{
		"request": req,
		"error":   lastErr.Error(),
		"failed_at": time.Now(),
	})
	if err := c.publisher.Publish(ctx, "webhook.dlq", string(m.Key), dlqPayload); err != nil {
		c.logger.Error("failed to publish to DLQ", "error", err)
	}
}
