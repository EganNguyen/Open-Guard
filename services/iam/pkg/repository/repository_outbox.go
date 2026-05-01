package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// CreateOutboxEvent inserts a new event into the outbox table within a transaction.
func (r *Repository) CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_records (org_id, topic, key, payload)
		VALUES ($1, $2, $3, $4)
	`, orgID, topic, key, payload)
	return err
}
