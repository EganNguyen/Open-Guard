package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
func (r *Repository) CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role, status string) (string, error) {
	var id string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			INSERT INTO users (org_id, email, password_hash, display_name, role, status)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
		`, orgID, email, passwordHash, displayName, role, status).Scan(&id)
	})
	return id, err
}

// CreateOutboxEvent inserts a new event into the outbox table within a transaction.
func (r *Repository) CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_records (org_id, topic, key, payload)
		VALUES ($1, $2, $3, $4)
	`, orgID, topic, key, payload)
	return err
}

// UpdateUserStatus updates the status of a user.
func (r *Repository) UpdateUserStatus(ctx context.Context, userID, status string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET status = $1, version = version + 1, updated_at = NOW()
			WHERE id = $2
		`, status, userID)
		return err
	})
}
// CreateSession inserts a new session record.
func (r *Repository) CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO sessions (org_id, user_id, jti, user_agent, ip_address, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, orgID, userID, jti, userAgent, ipAddress, expiresAt)
		return err
	})
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error) {
	// 1. Initial lookup to get org_id (Login path)
	// This uses a non-RLS query by setting the role to openguard_login
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "SET ROLE openguard_login")
	if err != nil {
		return nil, fmt.Errorf("set login role: %w", err)
	}

	var id, orgID, pwdHash, displayName, role, status string
	var failedCount int
	var lockedUntil *time.Time
	err = conn.QueryRow(ctx, `
		SELECT id, org_id, password_hash, display_name, role, status, failed_login_count, locked_until 
		FROM users WHERE email = $1
	`, email).Scan(&id, &orgID, &pwdHash, &displayName, &role, &status, &failedCount, &lockedUntil)
	
	// Reset role back to default (openguard_app) before releasing
	_, _ = conn.Exec(ctx, "RESET ROLE")

	if err != nil {
		return nil, err
	}

	// 2. Re-query with RLS enforced using withOrgContext
	var user = make(map[string]interface{})
	err = r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT id, org_id, password_hash, display_name, role, status, failed_login_count, locked_until 
			FROM users WHERE id = $1
		`, id).Scan(&id, &orgID, &pwdHash, &displayName, &role, &status, &failedCount, &lockedUntil)
	})

	if err != nil {
		return nil, fmt.Errorf("rls verification: %w", err)
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
	orgID := rls.OrgID(ctx)
	var user = make(map[string]interface{})
	var email, displayName, role, status string
	var orgIDRes string

	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT org_id, email, display_name, role, status FROM users WHERE id = $1
		`, id).Scan(&orgIDRes, &email, &displayName, &role, &status)
	})

	if err != nil {
		return nil, err
	}
	user["id"] = id
	user["org_id"] = orgIDRes
	user["email"] = email
	user["display_name"] = displayName
	user["role"] = role
	user["status"] = status
	return user, nil
}
func (r *Repository) IncrementFailedLogin(ctx context.Context, email string) (int, error) {
	var count int
	// We use the login lookup pattern here if we don't have orgID.
	// But IncrementFailedLogin is called after a failed login attempt.
	// We might not have orgID in context.
	// For simplicity and since this is a write to a specific user by email,
	// we use the openguard_login role to bypass RLS for this specific update if needed,
	// or we find the orgID first.
	// Let's find the orgID first to stay consistent.
	
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()
	
	_, _ = conn.Exec(ctx, "SET ROLE openguard_login")
	var orgID string
	_ = conn.QueryRow(ctx, "SELECT org_id FROM users WHERE email = $1", email).Scan(&orgID)
	_, _ = conn.Exec(ctx, "RESET ROLE")
	
	if orgID == "" {
		return 0, ErrNotFound
	}

	err = r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			UPDATE users SET failed_login_count = failed_login_count + 1, updated_at = NOW()
			WHERE email = $1 RETURNING failed_login_count
		`, email).Scan(&count)
	})
	return count, err
}

func (r *Repository) ResetFailedLogin(ctx context.Context, email string) error {
	orgID := rls.OrgID(ctx)
	if orgID == "" {
		// If not in context, we need to find it (similar to above)
		conn, err := r.pool.Acquire(ctx)
		if err == nil {
			_, _ = conn.Exec(ctx, "SET ROLE openguard_login")
			_ = conn.QueryRow(ctx, "SELECT org_id FROM users WHERE email = $1", email).Scan(&orgID)
			_, _ = conn.Exec(ctx, "RESET ROLE")
			conn.Release()
		}
	}

	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET failed_login_count = 0, locked_until = NULL, updated_at = NOW()
			WHERE email = $1
		`, email)
		return err
	})
}

func (r *Repository) LockAccount(ctx context.Context, email string, until time.Time) error {
	orgID := rls.OrgID(ctx)
	if orgID == "" {
		conn, err := r.pool.Acquire(ctx)
		if err == nil {
			_, _ = conn.Exec(ctx, "SET ROLE openguard_login")
			_ = conn.QueryRow(ctx, "SELECT org_id FROM users WHERE email = $1", email).Scan(&orgID)
			_, _ = conn.Exec(ctx, "RESET ROLE")
			conn.Release()
		}
	}
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET locked_until = $2, updated_at = NOW()
			WHERE email = $1
		`, email, until)
		return err
	})
}

func (r *Repository) GetMFAConfig(ctx context.Context, userID, mfaType string) (map[string]interface{}, error) {
	orgID := rls.OrgID(ctx)
	var config = make(map[string]interface{})
	var secretEncrypted string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT secret_encrypted FROM mfa_configs WHERE user_id = $1 AND mfa_type = $2
		`, userID, mfaType).Scan(&secretEncrypted)
	})
	if err != nil {
		return nil, err
	}
	config["secret_encrypted"] = secretEncrypted
	return config, nil
}

func (r *Repository) UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secretEncrypted string) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO mfa_configs (org_id, user_id, mfa_type, secret_encrypted)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, mfa_type) DO UPDATE SET 
				secret_encrypted = EXCLUDED.secret_encrypted, 
				updated_at = NOW()
		`, orgID, userID, mfaType, secretEncrypted)
		return err
	})
}

func (r *Repository) EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET mfa_enabled = $1, mfa_method = $2 WHERE id = $3
		`, enabled, method, userID)
		return err
	})
}

func (r *Repository) StoreBackupCodes(ctx context.Context, userID string, hashes []string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE mfa_configs SET backup_code_hashes = $1, updated_at = NOW()
			WHERE user_id = $2 AND mfa_type = 'totp'
		`, hashes, userID)
		return err
	})
}

func (r *Repository) ConsumeBackupCode(ctx context.Context, userID string, codeHash string) (bool, error) {
	orgID := rls.OrgID(ctx)
	var rowsAffected int64
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		res, err := conn.Exec(ctx, `
			UPDATE mfa_configs 
			SET backup_code_hashes = array_remove(backup_code_hashes, $1), updated_at = NOW()
			WHERE user_id = $2 AND mfa_type = 'totp' AND $1 = ANY(backup_code_hashes)
		`, codeHash, userID)
		if err != nil {
			return err
		}
		rowsAffected = res.RowsAffected()
		return nil
	})
	return rowsAffected > 0, err
}

func (r *Repository) ListMFAConfigs(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	orgID := rls.OrgID(ctx)
	var configs []map[string]interface{}
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT mfa_type, secret_encrypted FROM mfa_configs WHERE user_id = $1
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var mfaType, secret string
			if err := rows.Scan(&mfaType, &secret); err != nil {
				return err
			}
			configs = append(configs, map[string]interface{}{
				"mfa_type":         mfaType,
				"secret_encrypted": secret,
			})
		}
		return nil
	})
	return configs, err
}

func (r *Repository) GetConnectorByID(ctx context.Context, id string) (map[string]interface{}, error) {
	orgID := rls.OrgID(ctx)
	var connector = make(map[string]interface{})
	var name, secret string
	var uris []string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT name, client_secret, redirect_uris FROM connectors WHERE id = $1
		`, id).Scan(&name, &secret, &uris)
	})
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
	orgID := rls.OrgID(ctx)
	var connectors []map[string]interface{}
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, name, redirect_uris FROM connectors
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name string
			var orgIDStr *string // Can be null
			var uris []string
			if err := rows.Scan(&id, &orgIDStr, &name, &uris); err != nil {
				return err
			}
			connectors = append(connectors, map[string]interface{}{
				"id":            id,
				"org_id":        orgIDStr,
				"name":          name,
				"redirect_uris": uris,
			})
		}
		return nil
	})
	return connectors, err
}
func (r *Repository) ListUsers(ctx context.Context, orgID string, filter string) ([]map[string]interface{}, error) {
	var users []map[string]interface{}
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		query := `SELECT id, org_id, email, display_name, role, status, scim_external_id, version, created_at, updated_at FROM users WHERE org_id = $1`
		args := []interface{}{orgID}

		if filter != "" {
			if strings.Contains(filter, `userName eq "`) {
				email := strings.Split(strings.Split(filter, `userName eq "`)[1], `"`)[0]
				query += ` AND email = $2`
				args = append(args, email)
			}
		}

		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id, orgIDStr, email, name, role, status string
			var externalID *string
			var version int
			var created, updated time.Time
			if err := rows.Scan(&id, &orgIDStr, &email, &name, &role, &status, &externalID, &version, &created, &updated); err != nil {
				return err
			}
			user := map[string]interface{}{
				"id":               id,
				"org_id":           orgIDStr,
				"email":            email,
				"display_name":     name,
				"role":             role,
				"status":           status,
				"scim_external_id": externalID,
				"version":          version,
				"created_at":       created,
				"updated_at":       updated,
			}
			users = append(users, user)
		}
		return nil
	})
	return users, err
}

func (r *Repository) GetUserByExternalID(ctx context.Context, orgID, externalID string) (map[string]interface{}, error) {
	return r.getUser(ctx, "org_id = $1 AND scim_external_id = $2", orgID, externalID)
}

func (r *Repository) UpdateUserSCIM(ctx context.Context, id string, externalID string, status string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET scim_external_id = $1, status = $2, version = version + 1, updated_at = NOW()
			WHERE id = $3
		`, externalID, status, id)
		return err
	})
}

func (r *Repository) CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// 1. Create Org (Global or special role might be needed if RLS is on orgs)
	var orgID string
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + id
	err = tx.QueryRow(ctx, `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id
	`, name, slug).Scan(&orgID)
	if err != nil {
		return "", err
	}

	// Set RLS for the following operations in this transaction
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
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
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE connectors SET name = $2, redirect_uris = $3 WHERE id = $1
		`, id, name, uris)
		return err
	})
}

func (r *Repository) DeleteConnector(ctx context.Context, id string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			DELETE FROM connectors WHERE id = $1
		`, id)
		return err
	})
}

func (r *Repository) CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO refresh_tokens (org_id, user_id, token_hash, family_id, expires_at)
			VALUES ($1, $2, $3, $4, $5)
		`, orgID, userID, tokenHash, familyID, expiresAt)
		return err
	})
}

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (map[string]interface{}, error) {
	orgID := rls.OrgID(ctx)
	var rt = make(map[string]interface{})
	var id, orgIDRes, userID string
	var familyID uuid.UUID
	var expiresAt time.Time
	var revoked bool

	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT id, org_id, user_id, family_id, expires_at, revoked 
			FROM refresh_tokens WHERE token_hash = $1
		`, tokenHash).Scan(&id, &orgIDRes, &userID, &familyID, &expiresAt, &revoked)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	rt["id"] = id
	rt["org_id"] = orgIDRes
	rt["user_id"] = userID
	rt["family_id"] = familyID
	rt["expires_at"] = expiresAt
	rt["revoked"] = revoked
	return rt, nil
}

func (r *Repository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE refresh_tokens SET revoked = TRUE WHERE family_id = $1
		`, familyID)
		return err
	})
}

func (r *Repository) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			DELETE FROM refresh_tokens WHERE token_hash = $1
		`, tokenHash)
		return err
	})
}
func (r *Repository) getUser(ctx context.Context, where string, args ...interface{}) (map[string]interface{}, error) {
	orgID := rls.OrgID(ctx)
	query := `SELECT id, org_id, email, display_name, role, status FROM users WHERE ` + where
	var id, orgIDRes, email, displayName, role, status string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, query, args...).Scan(&id, &orgIDRes, &email, &displayName, &role, &status)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return map[string]interface{}{
		"id":           id,
		"org_id":       orgIDRes,
		"email":        email,
		"display_name": displayName,
		"role":         role,
		"status":       status,
	}, nil
}
