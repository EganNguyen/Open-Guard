package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
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
	batchSize := 1000
	ticker := time.NewTicker(5 * time.Second)
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
		return
	}

	if err := w.repo.IngestEvents(ctx, events); err != nil {
		w.logger.Error("failed to ingest events to ClickHouse", "error", err)
		return
	}

	if err := w.reader.CommitMessages(ctx, messages...); err != nil {
		w.logger.Error("failed to commit offsets", "error", err)
	}
}
