package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func (r *Repository) CreateConnector(ctx context.Context, id, orgID, name, secret string, uris []string, prefix, hash string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO connectors (id, org_id, name, client_secret, redirect_uris, api_key_prefix, api_key_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, orgID, name, secret, uris, prefix, hash)
	return err
}

func (r *Repository) GetConnectorByID(ctx context.Context, id string) (map[string]interface{}, error) {
	var idStr, orgID, name, secret, prefix, hash string
	var uris []string
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, client_secret, redirect_uris, api_key_prefix, api_key_hash, created_at, updated_at
		FROM connectors WHERE id = $1
	`, id).Scan(&idStr, &orgID, &name, &secret, &uris, &prefix, &hash, &createdAt, &updatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":             idStr,
		"org_id":         orgID,
		"name":           name,
		"client_secret":  secret,
		"redirect_uris":  uris,
		"api_key_prefix": prefix,
		"api_key_hash":   hash,
		"created_at":    createdAt,
		"updated_at":    updatedAt,
	}, nil
}

func (r *Repository) FindByPrefix(ctx context.Context, prefix string) (map[string]interface{}, error) {
	var idStr, orgID, name, secret, hash string
	var uris []string

	err := r.pool.QueryRow(ctx, `
		SELECT id, org_id, name, client_secret, redirect_uris, api_key_hash
		FROM connectors WHERE api_key_prefix = $1
	`, prefix).Scan(&idStr, &orgID, &name, &secret, &uris, &hash)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":            idStr,
		"org_id":        orgID,
		"name":          name,
		"client_secret": secret,
		"redirect_uris": uris,
		"api_key_hash":  hash,
	}, nil
}
func (r *Repository) DeleteConnector(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM connectors WHERE id = $1", id)
	return err
}
