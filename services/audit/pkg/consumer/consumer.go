package consumer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/audit/pkg/repository"
)

type AuditConsumer struct {
	reader *kafka.Reader
	repo   *repository.AuditRepository
	logger *slog.Logger
}

func NewAuditConsumer(brokers string, groupID string, topic string, repo *repository.AuditRepository, logger *slog.Logger) (*AuditConsumer, error) {
	brokerList := strings.Split(brokers, ",")
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokerList,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
		CommitInterval: 0, // Manual commit (R-07)
	})

	return &AuditConsumer{
		reader: r,
		repo:   repo,
		logger: logger,
	}, nil
}

func (c *AuditConsumer) Start(ctx context.Context) error {
	batchSize := 100
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var batch []kafka.Message

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if len(batch) > 0 {
				c.flush(ctx, batch)
				batch = nil
			}
		default:
			// FetchMessage handles reading and preparing for commit
			m, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				c.logger.Error("failed to fetch kafka message", "error", err)
				continue
			}

			batch = append(batch, m)
			if len(batch) >= batchSize {
				c.flush(ctx, batch)
				batch = nil
			}
		}
	}
}

func (c *AuditConsumer) flush(ctx context.Context, batch []kafka.Message) {
	c.logger.Info("flushing audit batch to mongodb", "size", len(batch))
	
	secretKey := os.Getenv("AUDIT_SECRET_KEY")
	if secretKey == "" {
		c.logger.Warn("AUDIT_SECRET_KEY not set, skipping hash chain")
	}

	var events []interface{}
	for _, m := range batch {
		var event map[string]interface{}
		if err := json.Unmarshal(m.Value, &event); err != nil {
			c.logger.Error("failed to unmarshal kafka message", "error", err)
			continue
		}
		event["timestamp"] = time.Now()

		if secretKey != "" {
			prevHash := ""
			lastEvent, err := c.repo.GetLatestEvent(ctx)
			if err != nil {
				c.logger.Error("failed to get latest event", "error", err)
			} else if lastEvent != nil {
				if h, ok := lastEvent["integrity_hash"].(string); ok {
					prevHash = h
				}
			}

			eventData := fmt.Sprintf("%s|%s", event["event_id"], prevHash)
			mac := hmac.New(sha256.New, []byte(secretKey))
			mac.Write([]byte(eventData))
			event["integrity_hash"] = hex.EncodeToString(mac.Sum(nil))
		}

		events = append(events, event)
	}

	if len(events) == 0 {
		return
	}

	err := c.repo.BulkWrite(ctx, events)
	if err != nil {
		c.logger.Error("failed to bulk write to mongodb", "error", err)
		return
	}

	// Commit messages after successful DB write (R-07)
	err = c.reader.CommitMessages(ctx, batch...)
	if err != nil {
		c.logger.Error("failed to commit kafka offsets", "error", err)
	}
}

func (c *AuditConsumer) Close() {
	c.reader.Close()
}
