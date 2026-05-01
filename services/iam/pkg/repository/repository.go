package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
)

// Repository handles database interactions for the IAM service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new repository instance.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Pool returns the underlying pgxpool.Pool.
func (r *Repository) Pool() *pgxpool.Pool {
	return r.pool
}

// BeginTx starts a new transaction.
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// withOrgContext acquires a connection, sets app.org_id, executes fn, returns conn.
func (r *Repository) withOrgContext(ctx context.Context, orgID string,
	fn func(ctx context.Context, conn *pgxpool.Conn) error) error {

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	if err := rls.SetSessionVar(ctx, conn, orgID); err != nil {
		return err
	}
	return fn(ctx, conn)
}
