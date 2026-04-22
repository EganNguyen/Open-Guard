package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// BeginTx starts a new transaction.
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// CreateOrg inserts a new organization.
func (r *Repository) CreateOrg(ctx context.Context, name string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO orgs (name) VALUES ($1) RETURNING id
	`, name).Scan(&id)
	return id, err
}

// CreateUser inserts a new user within an org.
func (r *Repository) CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (org_id, email, password_hash, display_name, role)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, orgID, email, passwordHash, displayName, role).Scan(&id)
	return id, err
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error) {
	var user = make(map[string]interface{})
	var id, orgID, pwdHash, displayName, role, status string
	err := r.pool.QueryRow(ctx, `
		SELECT id, org_id, password_hash, display_name, role, status FROM users WHERE email = $1
	`, email).Scan(&id, &orgID, &pwdHash, &displayName, &role, &status)
	if err != nil {
		return nil, err
	}
	user["id"] = id
	user["org_id"] = orgID
	user["password_hash"] = pwdHash
	user["display_name"] = displayName
	user["role"] = role
	user["status"] = status
	user["email"] = email
	return user, nil
}

func (r *Repository) GetConnectorByID(ctx context.Context, id string) (map[string]interface{}, error) {
	var connector = make(map[string]interface{})
	var name, secret string
	var uris []string
	err := r.pool.QueryRow(ctx, `
		SELECT name, client_secret, redirect_uris FROM connectors WHERE id = $1
	`, id).Scan(&name, &secret, &uris)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	connector["id"] = id
	connector["name"] = name
	connector["client_secret"] = secret
	connector["redirect_uris"] = uris
	return connector, nil
}
func (r *Repository) ListConnectors(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, name, redirect_uris FROM connectors
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connectors []map[string]interface{}
	for rows.Next() {
		var id, name string
		var orgID *string // Can be null
		var uris []string
		if err := rows.Scan(&id, &orgID, &name, &uris); err != nil {
			return nil, err
		}
		connectors = append(connectors, map[string]interface{}{
			"id":            id,
			"org_id":        orgID,
			"name":          name,
			"redirect_uris": uris,
		})
	}
	return connectors, nil
}
func (r *Repository) ListUsers(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, email, display_name, role, status, created_at FROM users
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, orgID, email, name, role, status string
		var createdAt time.Time
		if err := rows.Scan(&id, &orgID, &email, &name, &role, &status, &createdAt); err != nil {
			return nil, err
		}
		users = append(users, map[string]interface{}{
			"id":           id,
			"org_id":       orgID,
			"email":        email,
			"display_name": name,
			"role":         role,
			"status":       status,
			"created_at":   createdAt,
		})
	}
	return users, nil
}

func (r *Repository) CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// 1. Create Org
	var orgID string
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + id
	err = tx.QueryRow(ctx, `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id
	`, name, slug).Scan(&orgID)
	if err != nil {
		return "", err
	}

	// 2. Create Connector
	_, err = tx.Exec(ctx, `
		INSERT INTO connectors (id, org_id, name, client_secret, redirect_uris)
		VALUES ($1, $2, $3, $4, $5)
	`, id, orgID, name, secret, uris)
	if err != nil {
		return "", err
	}

	return orgID, tx.Commit(ctx)
}

func (r *Repository) UpdateConnector(ctx context.Context, id, name string, uris []string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE connectors SET name = $2, redirect_uris = $3 WHERE id = $1
	`, id, name, uris)
	return err
}

func (r *Repository) DeleteConnector(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM connectors WHERE id = $1
	`, id)
	return err
}
