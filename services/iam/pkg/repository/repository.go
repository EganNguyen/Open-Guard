package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
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
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + uuid.New().String()[:8]
	err := r.pool.QueryRow(ctx, `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id
	`, name, slug).Scan(&id)
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
// CreateSession inserts a new session record.
func (r *Repository) CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (org_id, user_id, jti, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, orgID, userID, jti, userAgent, ipAddress, expiresAt)
	return err
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error) {
	var user = make(map[string]interface{})
	var id, orgID, pwdHash, displayName, role, status string
	var failedCount int
	var lockedUntil *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, org_id, password_hash, display_name, role, status, failed_login_count, locked_until 
		FROM users WHERE email = $1
	`, email).Scan(&id, &orgID, &pwdHash, &displayName, &role, &status, &failedCount, &lockedUntil)
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
	user["failed_login_count"] = failedCount
	user["locked_until"] = lockedUntil
	return user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (map[string]interface{}, error) {
	var user = make(map[string]interface{})
	var orgID, email, displayName, role, status string
	err := r.pool.QueryRow(ctx, `
		SELECT org_id, email, display_name, role, status FROM users WHERE id = $1
	`, id).Scan(&orgID, &email, &displayName, &role, &status)
	if err != nil {
		return nil, err
	}
	user["id"] = id
	user["org_id"] = orgID
	user["email"] = email
	user["display_name"] = displayName
	user["role"] = role
	user["status"] = status
	return user, nil
}
func (r *Repository) IncrementFailedLogin(ctx context.Context, email string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		UPDATE users SET failed_login_count = failed_login_count + 1, updated_at = NOW()
		WHERE email = $1 RETURNING failed_login_count
	`, email).Scan(&count)
	return count, err
}

func (r *Repository) ResetFailedLogin(ctx context.Context, email string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET failed_login_count = 0, locked_until = NULL, updated_at = NOW()
		WHERE email = $1
	`, email)
	return err
}

func (r *Repository) LockAccount(ctx context.Context, email string, until time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET locked_until = $2, updated_at = NOW()
		WHERE email = $1
	`, email, until)
	return err
}

func (r *Repository) GetMFAConfig(ctx context.Context, userID, mfaType string) (map[string]interface{}, error) {
	var config = make(map[string]interface{})
	var secretEncrypted string
	err := r.pool.QueryRow(ctx, `
		SELECT secret_encrypted FROM mfa_configs WHERE user_id = $1 AND mfa_type = $2
	`, userID, mfaType).Scan(&secretEncrypted)
	if err != nil {
		return nil, err
	}
	config["secret_encrypted"] = secretEncrypted
	return config, nil
}

func (r *Repository) UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secretEncrypted string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mfa_configs (org_id, user_id, mfa_type, secret_encrypted)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, mfa_type) DO UPDATE SET 
			secret_encrypted = EXCLUDED.secret_encrypted, 
			updated_at = NOW()
	`, orgID, userID, mfaType, secretEncrypted)
	return err
}

func (r *Repository) EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET mfa_enabled = $1, mfa_method = $2 WHERE id = $3
	`, enabled, method, userID)
	return err
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
