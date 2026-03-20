package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/openguard/shared/models"
	kafkago "github.com/segmentio/kafka-go"
)

// CacheInvalidator subscribes to policy.changes events and clears the Redis cache per org.
type CacheInvalidator struct {
	evaluator *EvaluatorService
	reader    *kafkago.Reader
	logger    *slog.Logger
}

// NewCacheInvalidator creates a Kafka consumer that listens on policy.changes
// and calls InvalidateCacheForOrg whenever a policy is created, updated, or deleted.
func NewCacheInvalidator(evaluator *EvaluatorService, brokers []string, logger *slog.Logger) *CacheInvalidator {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:  brokers,
		Topic:    "policy.changes",
		GroupID:  "openguard-policy-v1",
		MinBytes: 1e3,  // 1KB
		MaxBytes: 1e6,  // 1MB
	})

	return &CacheInvalidator{
		evaluator: evaluator,
		reader:    reader,
		logger:    logger,
	}
}

// Start runs the invalidation loop. Blocks until ctx is cancelled.
func (c *CacheInvalidator) Start(ctx context.Context) {
	c.logger.Info("policy cache invalidator started")
	defer c.reader.Close()

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // context cancelled — clean shutdown
			}
			c.logger.Error("kafka fetch error", "error", err)
			continue
		}

		var envelope models.EventEnvelope
		if err := json.Unmarshal(msg.Value, &envelope); err != nil {
			c.logger.Error("failed to unmarshal policy change event", "error", err)
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}

		if envelope.OrgID != "" {
			if err := c.evaluator.InvalidateCacheForOrg(ctx, envelope.OrgID); err != nil {
				c.logger.Error("cache invalidation failed", "org_id", envelope.OrgID, "error", err)
			}
		}

		_ = c.reader.CommitMessages(ctx, msg)
	}
}

// Close shuts down the consumer.
func (c *CacheInvalidator) Close() error {
	return c.reader.Close()
}
