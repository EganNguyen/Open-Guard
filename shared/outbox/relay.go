package outbox

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxDB abstracts the database connection for the relay.
type OutboxDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// OutboxProducer abstracts the Kafka producer for the relay.
type OutboxProducer interface {
	PublishRaw(ctx context.Context, topic string, key []byte, payload []byte) error
}

type Relay struct {
	db        OutboxDB
	producer  OutboxProducer
	TableName string
}

func NewRelay(db OutboxDB, producer OutboxProducer) *Relay {
	return &Relay{db: db, producer: producer, TableName: "outbox_records"}
}

func (r *Relay) getTableName() string {
	if r.TableName == "" {
		return "outbox_records"
	}
	return r.TableName
}

func (r *Relay) Start(ctx context.Context) {
	log.Println("Starting Outbox Relay Daemon")

	notifyCh := make(chan struct{}, 1)

	go func() {
		channel := "outbox_new"
		if r.getTableName() == "policy_outbox_records" {
			channel = "policy_outbox_new"
		}
		for {
			if ctx.Err() != nil {
				return
			}
			conn, err := r.db.Acquire(ctx)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}
			_, err = conn.Exec(ctx, "LISTEN "+channel)
			if err != nil {
				conn.Release()
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}
			for {
				if ctx.Err() != nil {
					conn.Release()
					return
				}
				_, err := conn.Conn().WaitForNotification(ctx)
				if err != nil {
					conn.Release()
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
					break // break out to re-acquire connection
				}
				select {
				case notifyCh <- struct{}{}:
				default:
				}
			}
		}
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Outbox Relay stopping due to context cancellation")
			return
		case <-notifyCh:
			if err := r.processBatch(ctx); err != nil {
				log.Printf("Outbox Relay batch error (notified): %v", err)
			}
		case <-ticker.C:
			if err := r.processBatch(ctx); err != nil {
				log.Printf("Outbox Relay batch error (tick): %v", err)
			}
		}
	}
}

func (r *Relay) processBatch(ctx context.Context) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT id, topic, key, payload 
		FROM ` + r.getTableName() + ` 
		WHERE status = 'pending' 
		ORDER BY created_at ASC 
		LIMIT 100 
		FOR UPDATE SKIP LOCKED
	`
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	type record struct {
		ID      string
		Topic   string
		Key     string
		Payload []byte
	}
	var batch []record

	for rows.Next() {
		var rec record
		if err := rows.Scan(&rec.ID, &rec.Topic, &rec.Key, &rec.Payload); err != nil {
			return err
		}
		batch = append(batch, rec)
	}
	rows.Close()

	if len(batch) == 0 {
		return nil
	}

	for _, rec := range batch {
		recordCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		pubErr := r.producer.PublishRaw(recordCtx, rec.Topic, []byte(rec.Key), rec.Payload)
		cancel()

		if pubErr != nil {
			log.Printf("Failed to publish record %s: %v", rec.ID, pubErr)
			_, _ = tx.Exec(ctx, `UPDATE `+r.getTableName()+` SET attempts = attempts + 1, last_error = $1 WHERE id = $2`, pubErr.Error(), rec.ID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE `+r.getTableName()+` SET status = 'published', published_at = NOW() WHERE id = $1`, rec.ID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}
