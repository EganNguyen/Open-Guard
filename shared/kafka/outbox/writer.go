package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Writer handles writing events to the outbox table.
type Writer struct {
	tableName string
}

// NewWriter creates a new outbox writer.
func NewWriter(tableName string) *Writer {
	return &Writer{tableName: tableName}
}

// WriteTx writes an event to the outbox within an existing transaction.
func (w *Writer) WriteTx(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (org_id, topic, key, payload)
		VALUES ($1, $2, $3, $4)
	`, w.tableName)

	_, err := tx.Exec(ctx, query, orgID, topic, key, payload)
	if err != nil {
		return fmt.Errorf("write to outbox: %w", err)
	}
	return nil
}
