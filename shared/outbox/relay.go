package outbox

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/kafka"
)

type Relay struct {
	db        *pgxpool.Pool
	producer  *kafka.Producer
	TableName string
}

func NewRelay(db *pgxpool.Pool, producer *kafka.Producer) *Relay {
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

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Outbox Relay stopping due to context cancellation")
			return
		case <-ticker.C:
			if err := r.processBatch(ctx); err != nil {
				log.Printf("Outbox Relay batch error: %v", err)
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
