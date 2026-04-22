package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// KafkaPublisher defines the interface for publishing to Kafka.
type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

// Relay polls the outbox table and publishes events to Kafka.
type Relay struct {
	pool      *pgxpool.Pool
	publisher KafkaPublisher
	tableName string
	interval  time.Duration
	logger    *slog.Logger
}

// NewRelay creates a new outbox relay.
func NewRelay(pool *pgxpool.Pool, publisher KafkaPublisher, tableName string, interval time.Duration, logger *slog.Logger) *Relay {
	return &Relay{
		pool:      pool,
		publisher: publisher,
		tableName: tableName,
		interval:  interval,
		logger:    logger,
	}
}

// Run starts the relay polling loop.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("outbox relay started", "table", r.tableName)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopping")
			return
		case <-ticker.C:
			r.drain(ctx)
		}
	}
}

func (r *Relay) drain(ctx context.Context) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		r.logger.Error("failed to start transaction", "error", err)
		return
	}
	defer tx.Rollback(ctx)

	// 1. Fetch pending records using SKIP LOCKED for concurrent relays
	query := `
		SELECT id, topic, key, payload 
		FROM ` + r.tableName + ` 
		WHERE status = 'pending' 
		FOR UPDATE SKIP LOCKED 
		LIMIT 100
	`
	rows, err := tx.Query(ctx, query)
	if err != nil {
		r.logger.Error("failed to fetch outbox records", "error", err)
		return
	}
	defer rows.Close()

	type record struct {
		id      string
		topic   string
		key     string
		payload []byte
	}
	var records []record
	for rows.Next() {
		var rec record
		if err := rows.Scan(&rec.id, &rec.topic, &rec.key, &rec.payload); err != nil {
			r.logger.Error("failed to scan outbox record", "error", err)
			continue
		}
		records = append(records, rec)
	}
	rows.Close()

	// 2. Publish to Kafka and update status
	for _, rec := range records {
		if err := r.publisher.Publish(ctx, rec.topic, rec.key, rec.payload); err != nil {
			r.logger.Error("failed to publish to kafka", "id", rec.id, "error", err)
			_, _ = tx.Exec(ctx, "UPDATE "+r.tableName+" SET status = 'failed', last_error = $1, attempts = attempts + 1 WHERE id = $2", err.Error(), rec.id)
			continue
		}

		_, err = tx.Exec(ctx, "UPDATE "+r.tableName+" SET status = 'published', published_at = NOW() WHERE id = $1", rec.id)
		if err != nil {
			r.logger.Error("failed to update outbox status", "id", rec.id, "error", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("failed to commit outbox transaction", "error", err)
	}
}
