package rls

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// SetSessionVar sets the PostgreSQL session variable for RLS on a pooled connection.
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
	if orgID == "" {
		_, err := conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
		return err
	}
	_, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
	return err
}

// TxSetSessionVar sets the RLS variable within an existing transaction.
func TxSetSessionVar(ctx context.Context, tx pgx.Tx, orgID string) error {
	if orgID == "" {
		_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
		return err
	}
	_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, false)", orgID)
	return err
}

// OrgPool wraps pgxpool.Pool and automatically sets the RLS session variable
// on every acquired connection.
type OrgPool struct {
	pool *pgxpool.Pool
}

func NewOrgPool(pool *pgxpool.Pool) *OrgPool {
	return &OrgPool{pool: pool}
}

func (p *OrgPool) Acquire(ctx context.Context) (*pgxpool.Conn, error) {
	orgID := OrgID(ctx)
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	if err := SetSessionVar(ctx, conn, orgID); err != nil {
		conn.Release()
		return nil, fmt.Errorf("set rls session var: %w", err)
	}
	return conn, nil
}

func (p *OrgPool) Begin(ctx context.Context) (pgx.Tx, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	if err := TxSetSessionVar(ctx, tx, OrgID(ctx)); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return nil, fmt.Errorf("set rls in tx: %w", err)
	}
	return &orgTx{Tx: tx, conn: conn}, nil
}

type orgTx struct {
	pgx.Tx
	conn *pgxpool.Conn
}

func (t *orgTx) Commit(ctx context.Context) error {
	defer t.conn.Release()
	return t.Tx.Commit(ctx)
}

func (t *orgTx) Rollback(ctx context.Context) error {
	defer t.conn.Release()
	return t.Tx.Rollback(ctx)
}

func (p *OrgPool) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, func(), error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("acquire connection: %w", err)
	}
	tx, err := conn.BeginTx(ctx, opts)
	if err != nil {
		conn.Release()
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	if err := TxSetSessionVar(ctx, tx, OrgID(ctx)); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		return nil, nil, fmt.Errorf("set rls in tx: %w", err)
	}
	cleanup := func() {
		_ = tx.Rollback(ctx) // no-op if already committed
		conn.Release()
	}
	return tx, cleanup, nil
}

func (p *OrgPool) WithConn(ctx context.Context, fn func(conn *pgxpool.Conn) error) error {
	conn, err := p.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return fn(conn)
}
