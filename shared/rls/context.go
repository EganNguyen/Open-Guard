package rls

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	rlsSetDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "openguard_rls_session_set_duration_seconds",
		Help:    "Duration of RLS session variable SET calls",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
	})
)

type contextKey struct{}

// @AI-INTENT: [Pattern: Row-Level Security (RLS) Session Management]
// [Rationale: High-assurance multi-tenancy. By setting 'app.org_id' in the DB session,
// we delegate tenant isolation to the database engine rather than relying solely on app-layer WHERE clauses.]

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
	start := time.Now()
	defer func() {
		rlsSetDuration.Observe(time.Since(start).Seconds())
	}()

	_, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
	return err
}

// TxSetSessionVar sets the app.org_id session variable in the PostgreSQL transaction.
func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
	start := time.Now()
	defer func() {
		rlsSetDuration.Observe(time.Since(start).Seconds())
	}()

	_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
	if err != nil {
		return fmt.Errorf("set rls session var in tx: %w", err)
	}
	return nil
}
