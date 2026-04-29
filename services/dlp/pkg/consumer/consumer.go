package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/openguard/services/dlp/pkg/repository"
	"github.com/openguard/services/dlp/pkg/scanner"
	sharedkafka "github.com/openguard/shared/kafka"
	"github.com/segmentio/kafka-go"
)

type DLPEvent struct {
	OrgID    string                 `json:"org_id"`
	EventID  string                 `json:"event_id"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Repository interface {
	SaveFinding(ctx context.Context, f *repository.DLPFinding) error
}

type Consumer struct {
	reader                 *kafka.Reader
	repo                   Repository
	logger                 *slog.Logger
	consecutiveFailures    int
	maxConsecutiveFailures int
	dlqWriter              *kafka.Writer
}

func NewConsumer(brokers []string, topic string, groupID string, repo Repository, logger *slog.Logger, dlqWriter *kafka.Writer, maxConsecutiveFailures int) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	if maxConsecutiveFailures <= 0 {
		maxConsecutiveFailures = 5
	}
	return &Consumer{
		reader:                 reader,
		repo:                   repo,
		logger:                 logger,
		dlqWriter:              dlqWriter,
		maxConsecutiveFailures: maxConsecutiveFailures,
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
			commitStart := time.Now()
			c.reader.CommitMessages(ctx, msg)
			sharedkafka.OffsetCommitDuration.Observe(time.Since(commitStart).Seconds())
			continue
		}

		if err := c.processEvent(ctx, event); err != nil {
			c.logger.Error("DLP processing failed — not committing offset",
				"event_id", event.EventID, "error", err)
			c.consecutiveFailures++
			if c.consecutiveFailures >= c.maxConsecutiveFailures {
				c.logger.Error("DLP consumer exceeded max consecutive failures, sending to DLQ",
					"event_id", event.EventID)
				c.sendToDLQ(ctx, msg)
				commitStart := time.Now()
				if err := c.reader.CommitMessages(ctx, msg); err != nil {
					c.logger.Error("DLQ commit failed", "error", err)
				}
				sharedkafka.OffsetCommitDuration.Observe(time.Since(commitStart).Seconds())
				c.consecutiveFailures = 0
			}
			continue
		}

		c.consecutiveFailures = 0
		commitStart := time.Now()
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Error("failed to commit message", "error", err)
		}
		sharedkafka.OffsetCommitDuration.Observe(time.Since(commitStart).Seconds())
	}
}

func (c *Consumer) processEvent(ctx context.Context, event DLPEvent) error {
	// Extract content from metadata (e.g. content, body, message)
	contentFields := []string{"content", "body", "message", "description"}
	for _, field := range contentFields {
		if val, ok := event.Metadata[field]; ok {
			if strVal, ok := val.(string); ok {
				result := scanner.ScanContent(strVal)
				for _, f := range result.Findings {
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
						return err
					}
				}
			}
		}
	}
	return nil
}

func (c *Consumer) sendToDLQ(ctx context.Context, msg kafka.Message) {
	if c.dlqWriter == nil {
		c.logger.Warn("no DLQ writer configured, message dropped", "offset", msg.Offset)
		return
	}
	err := c.dlqWriter.WriteMessages(ctx, kafka.Message{
		Value: msg.Value,
		Headers: append(msg.Headers, kafka.Header{
			Key:   "x-dlq-reason",
			Value: []byte("max-consecutive-failures"),
		}),
	})
	if err != nil {
		c.logger.Error("failed to write to DLQ", "error", err)
	}
}
