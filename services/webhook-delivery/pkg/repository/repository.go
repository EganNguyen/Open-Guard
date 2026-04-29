package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookDelivery struct {
	ID           string
	OrgID        string
	ConnectorID  string
	EventID      string
	TargetURL    string
	Payload      json.RawMessage
	Attempts     int
	Status       string
	LastError    string
	NextRetryAt  *time.Time
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create records a new delivery attempt.
func (r *Repository) Create(ctx context.Context, d *WebhookDelivery) (string, error) {
	query := `
		INSERT INTO webhook_deliveries (org_id, connector_id, event_id, target_url, payload, attempts, status, last_error, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`
	err := r.db.QueryRow(ctx, query,
		d.OrgID, d.ConnectorID, d.EventID, d.TargetURL, d.Payload, d.Attempts, d.Status, d.LastError, d.NextRetryAt,
	).Scan(&d.ID)
	
	if err != nil {
		return "", fmt.Errorf("failed to create webhook delivery record: %w", err)
	}
	return d.ID, nil
}

// Update records the outcome of a delivery attempt.
func (r *Repository) Update(ctx context.Context, id string, attempts int, status, lastError string, nextRetryAt *time.Time) error {
	query := `
		UPDATE webhook_deliveries
		SET attempts = $1, status = $2, last_error = $3, next_retry_at = $4, updated_at = NOW()
		WHERE id = $5
	`
	_, err := r.db.Exec(ctx, query, attempts, status, lastError, nextRetryAt, id)
	if err != nil {
		return fmt.Errorf("failed to update webhook delivery record: %w", err)
	}
	return nil
}
