package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
	"github.com/openguard/shared/outbox"
)

type Repository struct {
	pool   *pgxpool.Pool
	outbox *outbox.Writer
}

func New(pool *pgxpool.Pool, outboxWriter *outbox.Writer) *Repository {
	return &Repository{pool: pool, outbox: outboxWriter}
}

// ValidateKey hashes the token and looks up the orgID and connectorID.
// Implements shared/middleware/APIKeyValidator.
func (r *Repository) ValidateKey(ctx context.Context, token string) (string, string, error) {
	hasher := &crypto.PBKDF2Hasher{}
	hash := hasher.Hash(token)

	connector, err := r.GetByHash(ctx, hash)
	if err != nil {
		return "", "", err
	}
	return connector.OrgID, connector.ID, nil
}

func (r *Repository) GetByHash(ctx context.Context, hash string) (*models.Connector, error) {
	query := `SELECT id, org_id, name, webhook_url, api_key, status, created_by, created_at, updated_at 
	          FROM connectors WHERE api_key = $1 AND status = 'active'`
	
	var c models.Connector
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&c.ID, &c.OrgID, &c.Name, &c.WebhookURL, &c.APIKey, &c.Status, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get connector by hash: %w", err)
	}
	return &c, nil
}

func (r *Repository) List(ctx context.Context, orgID string) ([]*models.Connector, error) {
	query := `SELECT id, org_id, name, webhook_url, status, created_by, created_at, updated_at 
	          FROM connectors WHERE org_id = $1 ORDER BY created_at DESC`
	
	rows, err := r.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("query connectors: %w", err)
	}
	defer rows.Close()

	var connectors []*models.Connector
	for rows.Next() {
		var c models.Connector
		err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.WebhookURL, &c.Status, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan connector: %w", err)
		}
		connectors = append(connectors, &c)
	}
	return connectors, nil
}

func (r *Repository) Create(ctx context.Context, c *models.Connector) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `INSERT INTO connectors (id, org_id, name, webhook_url, api_key, status, created_by, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	
	now := time.Now()
	if c.CreatedAt.IsZero() { c.CreatedAt = now }
	if c.UpdatedAt.IsZero() { c.UpdatedAt = now }

	_, err = tx.Exec(ctx, query, c.ID, c.OrgID, c.Name, c.WebhookURL, c.APIKey, c.Status, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert connector: %w", err)
	}

	if err := r.publishEvent(ctx, tx, kafka.TopicAuditTrail, "connector.create", c.OrgID, c.CreatedBy, c.ID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) IngestEvents(ctx context.Context, orgID string, connectorID string, events []models.EventEnvelope) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, event := range events {
		event.EventSource = "connector:" + connectorID
		event.OrgID = orgID

		if err := r.outbox.Write(ctx, tx, kafka.TopicConnectorEvents, orgID, orgID, event); err != nil {
			return fmt.Errorf("write outbox for event %s: %w", event.ID, err)
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) publishEvent(ctx context.Context, tx pgx.Tx, topic, eventType, orgID, actorID, resourceID string) error {
	if r.outbox == nil {
		return nil
	}

	payload, _ := json.Marshal(map[string]string{
		"resource_id": resourceID,
	})
	envelope := models.EventEnvelope{
		ID:         uuid.NewString(),
		Type:       eventType,
		OrgID:      orgID,
		ActorID:    actorID,
		ActorType:  "user",
		Source:     "control-plane",
		SchemaVer:  "2.0",
		Payload:    payload,
		OccurredAt: time.Now(),
	}

	return r.outbox.Write(ctx, tx, topic, actorID, orgID, envelope)
}
