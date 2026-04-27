package saga

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/segmentio/kafka-go"
)

type UserStatusUpdater interface {
	UpdateUserStatus(ctx context.Context, userID, status string) error
}

type KafkaReader interface {
	ReadMessage(ctx context.Context) (kafka.Message, error)
	Close() error
}

type Consumer struct {
	reader KafkaReader
	svc    UserStatusUpdater
	logger *slog.Logger
}

func NewConsumer(brokers string, groupID string, topic string, svc UserStatusUpdater, logger *slog.Logger) *Consumer {
	brokerList := strings.Split(brokers, ",")
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokerList,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	return &Consumer{
		reader: r,
		svc:    svc,
		logger: logger,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	for {
		m, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("failed to read kafka message", "error", err)
			continue
		}

		var event struct {
			Event  string `json:"event"`
			UserID string `json:"user_id"`
			Status string `json:"status"`
			SagaID string `json:"saga_id"`
		}
		if err := json.Unmarshal(m.Value, &event); err != nil {
			c.logger.Error("failed to unmarshal saga event", "error", err)
			continue
		}

		switch event.Event {
		case "user.provisioning.failed":
			userID := event.SagaID // In timeout, SagaID is UserID
			if event.UserID != "" {
				userID = event.UserID
			}
			c.logger.Warn("handling provisioning failure", "user_id", userID)
			if err := c.svc.UpdateUserStatus(ctx, userID, "provisioning_failed"); err != nil {
				c.logger.Error("failed to update user status", "user_id", userID, "error", err)
			}
		case "user.scim.provisioned":
			c.logger.Info("handling provisioning success", "user_id", event.UserID)
			if err := c.svc.UpdateUserStatus(ctx, event.UserID, "active"); err != nil {
				c.logger.Error("failed to update user status", "user_id", event.UserID, "error", err)
			}
		}
	}
}

func (c *Consumer) Close() {
	c.reader.Close()
}
