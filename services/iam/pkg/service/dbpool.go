package service

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DBPool abstracts the pgxpool connection to allow for isolated unit testing
// without requiring an active PostgreSQL database.
type DBPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}
