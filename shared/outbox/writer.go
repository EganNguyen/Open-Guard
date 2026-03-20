package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openguard/shared/models"
)

// Writer writes events to a transactional outbox table.
type Writer struct {
	TableName string
}

func NewWriter() *Writer {
	return &Writer{TableName: "outbox_records"}
}

// Execer interface handles *pgxpool.Conn and pgx.Tx
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (w *Writer) getTableName() string {
	if w.TableName == "" {
		return "outbox_records"
	}
	return w.TableName
}

func (w *Writer) Write(ctx context.Context, tx Execer, topic string, partitionKey string, envelope models.EventEnvelope) error {
	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	query := `INSERT INTO ` + w.getTableName() + ` (id, topic, key, payload, status, attempts, created_at)
		VALUES ($1, $2, $3, $4, 'pending', 0, $5)`
	_, err = tx.Exec(ctx, query, uuid.NewString(), topic, partitionKey, payloadBytes, time.Now())
	return err
}
