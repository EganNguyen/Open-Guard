package rls

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type contextKey struct{}

// WithOrgID stores the org ID in the Go context.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, contextKey{}, orgID)
}

// OrgID retrieves the org ID from context. Returns "" if not set.
func OrgID(ctx context.Context) string {
	v, _ := ctx.Value(contextKey{}).(string)
	return v
}

// SetSessionVar sets the PostgreSQL session variable for RLS.
// Must be called before every query on a pooled connection or transaction.
func SetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
	if orgID == "" {
		// Unset the variable — this results in no rows for RLS-protected tables
		_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
		return err
	}
	_, err := tx.Exec(ctx, fmt.Sprintf("SELECT set_config('app.org_id', '%s', false)", orgID))
	return err
}
