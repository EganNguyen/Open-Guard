package rls

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
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

// Execer interface covers pgxpool.Conn and pgx.Tx so SetSessionVar can be used with both.
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// SetSessionVar sets the PostgreSQL session variable for RLS.
// Must be called before every query on a pooled connection or transaction.
func SetSessionVar(ctx context.Context, execer Execer, orgID string) error {
	if orgID == "" {
		// Unset the variable — this results in no rows for RLS-protected tables
		_, err := execer.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
		return err
	}
	_, err := execer.Exec(ctx, fmt.Sprintf("SELECT set_config('app.org_id', '%s', false)", orgID))
	return err
}
