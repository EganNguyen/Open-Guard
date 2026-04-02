package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/models"
)

// Writer writes events to a transactional outbox table.
type Writer struct {
	TableName string
}

func NewWriter() *Writer {
	return &Writer{TableName: "outbox_records"}
}

func (w *Writer) getTableName() string {
	if w.TableName == "" {
		return "outbox_records"
	}
	return w.TableName
}

func (w *Writer) Write(ctx context.Context, tx pgx.Tx, topic string, partitionKey string, orgID string, envelope models.EventEnvelope) error {
	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	query := `INSERT INTO ` + w.getTableName() + ` (id, org_id, topic, key, payload, status, attempts, created_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', 0, $6)`
	_, err = tx.Exec(ctx, query, uuid.NewString(), orgID, topic, partitionKey, payloadBytes, time.Now())
	return err
}
