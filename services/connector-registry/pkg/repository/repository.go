package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

var (
	ErrNotFound = errors.New("connector not found")
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) withConn(ctx context.Context, fn func(conn *pgxpool.Conn) error) error {
	orgID := rls.OrgID(ctx)
	if orgID == "" {
		return errors.New("missing org_id in context")
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if err := rls.SetSessionVar(ctx, conn, orgID); err != nil {
		return err
	}

	return fn(conn)
}

func (r *Repository) CreateConnector(ctx context.Context, id, orgID, name, secret string, uris []string, prefix, hash string) error {
	return r.withConn(ctx, func(conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO connectors (id, org_id, name, client_secret, redirect_uris, api_key_prefix, api_key_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, id, orgID, name, secret, uris, prefix, hash)
		return err
	})
}

func (r *Repository) GetConnectorByID(ctx context.Context, id string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := r.withConn(ctx, func(conn *pgxpool.Conn) error {
		var idStr, orgID, name, secret, prefix, hash string
		var uris []string
		var createdAt, updatedAt time.Time

		err := conn.QueryRow(ctx, `
			SELECT id, org_id, name, client_secret, redirect_uris, api_key_prefix, api_key_hash, created_at, updated_at
			FROM connectors WHERE id = $1
		`, id).Scan(&idStr, &orgID, &name, &secret, &uris, &prefix, &hash, &createdAt, &updatedAt)

		if err != nil {
			return err
		}

		result = map[string]interface{}{
			"id":             idStr,
			"org_id":         orgID,
			"name":           name,
			"client_secret":  secret,
			"redirect_uris":  uris,
			"api_key_prefix": prefix,
			"api_key_hash":   hash,
			"created_at":    createdAt,
			"updated_at":    updatedAt,
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return result, nil
}

func (r *Repository) FindByPrefix(ctx context.Context, prefix string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := r.withConn(ctx, func(conn *pgxpool.Conn) error {
		var idStr, orgID, name, secret, hash string
		var uris []string

		err := conn.QueryRow(ctx, `
			SELECT id, org_id, name, client_secret, redirect_uris, api_key_hash
			FROM connectors WHERE api_key_prefix = $1
		`, prefix).Scan(&idStr, &orgID, &name, &secret, &uris, &hash)

		if err != nil {
			return err
		}

		result = map[string]interface{}{
			"id":            idStr,
			"org_id":        orgID,
			"name":          name,
			"client_secret": secret,
			"redirect_uris": uris,
			"api_key_hash":  hash,
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return result, nil
}

func (r *Repository) DeleteConnector(ctx context.Context, id string) error {
	return r.withConn(ctx, func(conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, "DELETE FROM connectors WHERE id = $1", id)
		return err
	})
}
