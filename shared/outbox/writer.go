package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openguard/shared/models"
)

// Writer writes events to the transactional outbox table in the same DB transaction.
type Writer struct{}

func NewWriter() *Writer {
	return &Writer{}
}

// Execer interface handles *pgxpool.Conn and pgx.Tx
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (w *Writer) Write(ctx context.Context, tx Execer, topic string, partitionKey string, envelope models.EventEnvelope) error {
	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO outbox_records (id, topic, key, payload, status, attempts, created_at)
		VALUES ($1, $2, $3, $4, 'pending', 0, $5)
	`
	_, err = tx.Exec(ctx, query, uuid.NewString(), topic, partitionKey, payloadBytes, time.Now())
	return err
}
