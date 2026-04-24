package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/compliance/pkg/repository"
)

type ClickHouseWriter struct {
	reader *kafka.Reader
	repo   *repository.Repository
	logger *slog.Logger
}

func NewClickHouseWriter(brokers string, groupID string, topic string, repo *repository.Repository, logger *slog.Logger) *ClickHouseWriter {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		GroupID: groupID,
		Topic:   topic,
	})

	return &ClickHouseWriter{
		reader: r,
		repo:   repo,
		logger: logger,
	}
}

func (w *ClickHouseWriter) Start(ctx context.Context) error {
	batchSize := getEnvInt("CLICKHOUSE_BULK_FLUSH_ROWS", 5000)
	flushMs := getEnvInt("CLICKHOUSE_BULK_FLUSH_MS", 2000)

	ticker := time.NewTicker(time.Duration(flushMs) * time.Millisecond)
	defer ticker.Stop()

	var batch []kafka.Message

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(ctx, batch)
				batch = nil
			}
		default:
			m, err := w.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				w.logger.Error("failed to fetch message", "error", err)
				continue
			}

			batch = append(batch, m)
			if len(batch) >= batchSize {
				w.flush(ctx, batch)
				batch = nil
			}
		}
	}
}

func (w *ClickHouseWriter) flush(ctx context.Context, messages []kafka.Message) {
	var events []repository.Event
	for _, m := range messages {
		var e repository.Event
		if err := json.Unmarshal(m.Value, &e); err != nil {
			w.logger.Error("failed to unmarshal event", "error", err)
			continue
		}
		if e.OccurredAt.IsZero() {
			e.OccurredAt = time.Now()
		}
		events = append(events, e)
	}

	if len(events) == 0 {
		// Still commit so we don't re-process undecodable messages forever
		_ = w.reader.CommitMessages(ctx, messages...)
		return
	}

	// 1. Write to ClickHouse FIRST
	if err := w.repo.IngestEvents(ctx, events); err != nil {
		w.logger.Error("clickhouse ingest failed — NOT committing offsets", "error", err)
		return // messages will be re-delivered on restart
	}

	// 2. Commit offsets ONLY after successful write
	if err := w.reader.CommitMessages(ctx, messages...); err != nil {
		w.logger.Error("offset commit failed after successful ingest", "error", err)
	}
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return fallback
}
