package consumer

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/openguard/services/dlp/pkg/repository"
	"github.com/openguard/services/dlp/pkg/scanner"
	"github.com/openguard/shared/kafka"
	"github.com/segmentio/kafka-go"
)

type DLPEvent struct {
	OrgID    string                 `json:"org_id"`
	EventID  string                 `json:"event_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Consumer struct {
	reader *kafka.Reader
	repo   *repository.Repository
	logger *slog.Logger
}

func NewConsumer(brokers []string, topic string, groupID string, repo *repository.Repository, logger *slog.Logger) *Consumer {
	reader := kafka.NewReader(brokers, topic, groupID)
	return &Consumer{
		reader: reader,
		repo:   repo,
		logger: logger,
	}
}

func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("DLP consumer starting", "topic", c.reader.Config().Topic)
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("failed to fetch message", "error", err)
			continue
		}

		var event DLPEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			c.logger.Error("failed to unmarshal event", "error", err)
			c.reader.CommitMessages(ctx, msg)
			continue
		}

		c.processEvent(ctx, event)

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("failed to commit message", "error", err)
		}
	}
}

func (c *Consumer) processEvent(ctx context.Context, event DLPEvent) {
	// Extract content from metadata (e.g. content, body, message)
	contentFields := []string{"content", "body", "message", "description"}
	for _, field := range contentFields {
		if val, ok := event.Metadata[field]; ok {
			if strVal, ok := val.(string); ok {
				findings := scanner.ScanRegex(strVal)
				for _, f := range findings {
					c.logger.Warn("DLP finding detected", "org_id", event.OrgID, "event_id", event.EventID, "kind", f.Kind)
					// Redact value before saving
					redacted := "REDACTED"
					err := c.repo.SaveFinding(ctx, &repository.DLPFinding{
						OrgID:         event.OrgID,
						EventID:       event.EventID,
						FindingType:   f.Kind,
						Confidence:    f.RiskScore,
						MatchedField:  field,
						RedactedValue: redacted,
					})
					if err != nil {
						c.logger.Error("failed to save finding", "error", err)
					}
				}
			}
		}
	}
}
