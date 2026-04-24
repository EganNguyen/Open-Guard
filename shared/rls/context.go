package rls

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey struct{}

// WithOrgID returns a new context with the org_id value.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, contextKey{}, orgID)
}

// OrgID extracts the org_id from the context.
func OrgID(ctx context.Context) string {
	v, _ := ctx.Value(contextKey{}).(string)
	return v
}

// SetSessionVar sets the app.org_id session variable in the PostgreSQL connection.
// This is used by Row-Level Security policies.
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
	_, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
	if err != nil {
		return fmt.Errorf("set rls session var: %w", err)
	}
	return nil
}

// TxSetSessionVar sets the app.org_id session variable in the PostgreSQL transaction.
// This is used by Row-Level Security policies.
func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
	_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
	if err != nil {
		return fmt.Errorf("set rls session var in tx: %w", err)
	}
	return nil
}
