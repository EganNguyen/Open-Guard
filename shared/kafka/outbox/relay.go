package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KafkaPublisher defines the interface for publishing to Kafka.
type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

const (
	// maxAttempts is the number of publish attempts before a record is moved to DLQ.
	maxAttempts = 5
	// notifyChannel is the pg_notify channel name — matches the trigger in migrations.
	notifyChannel = "outbox_new"
)

// @AI-INTENT: [Pattern: Hybrid Pull/Push Transactional Outbox]
// [Rationale: Exactly-once delivery with low latency. Combining pg_notify (push) with polling (pull)
// ensures that events are delivered immediately when the system is healthy, while remaining 
// resilient to missed notifications or Kafka downtime.]

// Relay polls the outbox table and publishes events to Kafka.
// It also listens on the pg_notify channel "outbox_new" to wake up immediately
// on insert rather than waiting for the polling ticker (best of both worlds).
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

// Run starts the relay. It combines two wakeup strategies:
//  1. pg_notify LISTEN: wake immediately on new outbox insert (low latency)
//  2. Polling ticker: fallback for missed notifications and stale 'failed' retries
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	notifications := make(chan string, 100)
	go r.listenLoop(ctx, notifications)

	r.logger.Info("outbox relay started", "table", r.tableName, "interval", r.interval)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopping")
			return

		case <-ticker.C:
			// Periodic drain
			r.drain(ctx)

		case <-notifications:
			// pg_notify wakeup
			r.logger.Debug("pg_notify received, draining")
			r.drain(ctx)
		}
	}
}

func (r *Relay) listenLoop(ctx context.Context, notifications chan<- string) {
	for {
		if ctx.Err() != nil {
			return
		}

		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			r.logger.Error("failed to acquire listen connection, retrying in 5s", "error", err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}

		_, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", notifyChannel))
		if err != nil {
			r.logger.Error("failed to issue LISTEN command, retrying", "error", err)
			conn.Release()
			time.Sleep(1 * time.Second)
			continue
		}
		r.logger.Info("pg_notify LISTEN established", "channel", notifyChannel)

		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				if ctx.Err() != nil {
					conn.Release()
					return
				}
				r.logger.Warn("LISTEN connection lost, reconnecting", "error", err)
				conn.Release()
				break // reconnect outer loop
			}
			notifications <- notification.Payload
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

	// Fetch pending records using SKIP LOCKED for concurrent relay safety.
	// Also re-attempt 'failed' records that haven't exceeded maxAttempts.
	query := fmt.Sprintf(`
		SELECT id, topic, key, payload, attempts
		FROM %s
		WHERE (status = 'pending' OR (status = 'failed' AND attempts < $1))
		  AND dead_at IS NULL
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 100
	`, r.tableName)

	rows, err := tx.Query(ctx, query, maxAttempts)
	if err != nil {
		r.logger.Error("failed to fetch outbox records", "error", err)
		return
	}

	type record struct {
		id       string
		topic    string
		key      string
		payload  []byte
		attempts int
	}

	var records []record
	for rows.Next() {
		var rec record
		if err := rows.Scan(&rec.id, &rec.topic, &rec.key, &rec.payload, &rec.attempts); err != nil {
			r.logger.Error("failed to scan outbox record", "error", err)
			continue
		}
		records = append(records, rec)
	}
	rows.Close()

	if len(records) == 0 {
		tx.Rollback(ctx)
		return
	}

	r.logger.Debug("draining outbox records", "count", len(records))

	for _, rec := range records {
		if err := r.publisher.Publish(ctx, rec.topic, rec.key, rec.payload); err != nil {
			r.logger.Error("failed to publish to kafka", "id", rec.id, "attempts", rec.attempts+1, "error", err)

			newAttempts := rec.attempts + 1
			if newAttempts >= maxAttempts {
				// Promote to DLQ — stop retrying
				r.logger.Warn("moving outbox record to dead letter", "id", rec.id, "attempts", newAttempts)
				r.updateRecord(ctx, tx, rec.id, "dead", err.Error(), newAttempts, true)
			} else {
				r.updateRecord(ctx, tx, rec.id, "failed", err.Error(), newAttempts, false)
			}
			continue
		}

		// Success: mark as published
		_, dbErr := tx.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET status = 'published', published_at = NOW(), attempts = $1 WHERE id = $2", r.tableName),
			rec.attempts+1, rec.id,
		)
		if dbErr != nil {
			r.logger.Error("failed to update outbox status", "id", rec.id, "error", dbErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		r.logger.Error("failed to commit outbox transaction", "error", err)
	}
}

// updateRecord updates a record's status, error, and optionally sets dead_at.
func (r *Relay) updateRecord(ctx context.Context, tx pgx.Tx, id, status, lastErr string, attempts int, isDead bool) {
	var query string
	if isDead {
		query = fmt.Sprintf(
			"UPDATE %s SET status = $1, last_error = $2, attempts = $3, dead_at = NOW() WHERE id = $4",
			r.tableName,
		)
	} else {
		query = fmt.Sprintf(
			"UPDATE %s SET status = $1, last_error = $2, attempts = $3 WHERE id = $4",
			r.tableName,
		)
	}
	if _, err := tx.Exec(ctx, query, status, lastErr, attempts, id); err != nil {
		r.logger.Error("failed to update outbox record", "id", id, "error", err)
	}
}
