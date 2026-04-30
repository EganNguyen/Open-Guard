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

// UpdateUserDisplayName updates the display name of a user.
func (r *Repository) UpdateUserDisplayName(ctx context.Context, userID, displayName string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET display_name = $1, version = version + 1, updated_at = NOW()
			WHERE id = $2
		`, displayName, userID)
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

// GetActiveJTIs returns all active JTIs for a user.
func (r *Repository) GetActiveJTIs(ctx context.Context, userID string) ([]string, error) {
	orgID := rls.OrgID(ctx)
	var jtis []string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT jti FROM sessions WHERE user_id = $1 AND expires_at > NOW()
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var jti string
			if err := rows.Scan(&jti); err != nil {
				return err
			}
			jtis = append(jtis, jti)
		}
		return nil
	})
	return jtis, err
}

// GetSessionTTL returns the remaining TTL for a session.
func (r *Repository) GetSessionTTL(ctx context.Context, jti string) time.Duration {
	orgID := rls.OrgID(ctx)
	var expiresAt time.Time
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT expires_at FROM sessions WHERE jti = $1
		`, jti).Scan(&expiresAt)
	})
	if err != nil {
		return 0
	}
	ttl := time.Until(expiresAt)
	if ttl < 0 {
		return 0
	}
	return ttl
}

// RevokeSessions revokes all sessions for a user in the database.
func (r *Repository) RevokeSessions(ctx context.Context, userID string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			DELETE FROM sessions WHERE user_id = $1
		`, userID)
		return err
	})
}

// DeprovisionAllUsers sets all users for an organization to deprovisioned.
func (r *Repository) DeprovisionAllUsers(ctx context.Context, orgID string) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET status = 'deprovisioned', version = version + 1, updated_at = NOW()
			WHERE org_id = $1
		`, orgID)
		return err
	})
}

// GetSessionByUserID returns the most recent active session for a user.
func (r *Repository) GetSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	orgID := rls.OrgID(ctx)
	var jti, ua, ip string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT jti, user_agent, ip_address FROM sessions 
			WHERE user_id = $1 AND expires_at > NOW()
			ORDER BY created_at DESC LIMIT 1
		`, userID).Scan(&jti, &ua, &ip)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Session{
		JTI:       jti,
		UserAgent: ua,
		IPAddress: ip,
	}, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
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

	var u User
	err = conn.QueryRow(ctx, `
		SELECT id, org_id, password_hash, display_name, role, status, failed_login_count, locked_until 
		FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.OrgID, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Status, &u.FailedLoginCount, &u.LockedUntil)

	// Reset role back to default (openguard_app) before releasing
	_, _ = conn.Exec(ctx, "RESET ROLE")

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	u.Email = email
	return &u, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	orgID := rls.OrgID(ctx)
	var u User

	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT org_id, email, display_name, role, status FROM users WHERE id = $1
		`, id).Scan(&u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Status)
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.ID = id
	return &u, nil
}

func (r *Repository) IncrementFailedLogin(ctx context.Context, email string) (int, error) {
	var count int
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

func (r *Repository) GetMFAConfig(ctx context.Context, userID, mfaType string) (*MFAConfig, error) {
	orgID := rls.OrgID(ctx)
	var secretEncrypted string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT secret_encrypted FROM mfa_configs WHERE user_id = $1 AND mfa_type = $2
		`, userID, mfaType).Scan(&secretEncrypted)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &MFAConfig{
		MFAType:         mfaType,
		SecretEncrypted: secretEncrypted,
	}, nil
}

func (r *Repository) ListMFAConfigs(ctx context.Context, userID string) ([]MFAConfig, error) {
	orgID := rls.OrgID(ctx)
	var configs []MFAConfig
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
			configs = append(configs, MFAConfig{
				MFAType:         mfaType,
				SecretEncrypted: secret,
			})
		}
		return nil
	})
	return configs, err
}

func (r *Repository) GetConnectorByID(ctx context.Context, id string) (*Connector, error) {
	orgID := rls.OrgID(ctx)
	var name, secret string
	var orgIDRes *string
	var uris []string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT org_id, name, client_secret, redirect_uris FROM connectors WHERE id = $1
		`, id).Scan(&orgIDRes, &name, &secret, &uris)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Connector{
		ID:           id,
		OrgID:        orgIDRes,
		Name:         name,
		ClientSecret: secret,
		RedirectURIs: uris,
	}, nil
}

func (r *Repository) ListConnectors(ctx context.Context) ([]Connector, error) {
	orgID := rls.OrgID(ctx)
	isSystem := orgID == "00000000-0000-0000-0000-000000000000"
	var connectors []Connector

	if isSystem {
		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Release()

		_, err = conn.Exec(ctx, "SET ROLE openguard_login")
		if err != nil {
			return nil, fmt.Errorf("set login role: %w", err)
		}
		defer func() { _, _ = conn.Exec(ctx, "RESET ROLE") }()

		rows, err := conn.Query(ctx, `SELECT id, org_id, name, client_secret, redirect_uris FROM connectors`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name, secret string
			var orgIDStr *string
			var uris []string
			if err := rows.Scan(&id, &orgIDStr, &name, &secret, &uris); err != nil {
				return nil, err
			}
			connectors = append(connectors, Connector{
				ID:           id,
				OrgID:        orgIDStr,
				Name:         name,
				ClientSecret: secret,
				RedirectURIs: uris,
			})
		}
		return connectors, nil
	}

	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, name, client_secret, redirect_uris FROM connectors
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name, secret string
			var orgIDStr *string
			var uris []string
			if err := rows.Scan(&id, &orgIDStr, &name, &secret, &uris); err != nil {
				return err
			}
			connectors = append(connectors, Connector{
				ID:           id,
				OrgID:        orgIDStr,
				Name:         name,
				ClientSecret: secret,
				RedirectURIs: uris,
			})
		}
		return nil
	})
	return connectors, err
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

func (r *Repository) ListUsers(ctx context.Context, orgID string, filter string) ([]User, error) {
	var users []User
	isSystem := orgID == "00000000-0000-0000-0000-000000000000"

	if isSystem {
		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Release()

		_, err = conn.Exec(ctx, "SET ROLE openguard_login")
		if err != nil {
			return nil, fmt.Errorf("set login role: %w", err)
		}
		defer func() { _, _ = conn.Exec(ctx, "RESET ROLE") }()

		query := `SELECT id, org_id, email, display_name, role, status, scim_external_id, version, created_at, updated_at FROM users`
		var args []interface{}

		if filter != "" {
			if strings.Contains(filter, `userName eq "`) {
				email := strings.Split(strings.Split(filter, `userName eq "`)[1], `"`)[0]
				query += ` WHERE email = $1`
				args = append(args, email)
			}
		}

		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var u User
			if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.SCIMExternalID, &u.Version, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return nil, err
			}
			users = append(users, u)
		}
		return users, nil
	}

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
			var u User
			if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.SCIMExternalID, &u.Version, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return err
			}
			users = append(users, u)
		}
		return nil
	})
	return users, err
}

func (r *Repository) ListUsersPaginated(ctx context.Context, orgID string, filter string, offset, limit int) ([]User, int, error) {
	var users []User
	var total int
	isSystem := orgID == "00000000-0000-0000-0000-000000000000"

	if isSystem {
		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			return nil, 0, err
		}
		defer conn.Release()

		_, err = conn.Exec(ctx, "SET ROLE openguard_login")
		if err != nil {
			return nil, 0, fmt.Errorf("set login role: %w", err)
		}
		defer func() { _, _ = conn.Exec(ctx, "RESET ROLE") }()

		baseQuery := `FROM users`
		whereClause := ""
		var args []interface{}

		if filter != "" {
			if strings.Contains(filter, `userName eq "`) {
				email := strings.Split(strings.Split(filter, `userName eq "`)[1], `"`)[0]
				whereClause = ` WHERE email = $1`
				args = append(args, email)
			}
		}

		err = conn.QueryRow(ctx, `SELECT COUNT(*) `+baseQuery+whereClause, args...).Scan(&total)
		if err != nil {
			return nil, 0, err
		}

		query := `SELECT id, org_id, email, display_name, role, status, scim_external_id, version, created_at, updated_at ` + baseQuery + whereClause + ` ORDER BY created_at OFFSET $` + fmt.Sprint(len(args)+1) + ` LIMIT $` + fmt.Sprint(len(args)+2)
		args = append(args, offset, limit)

		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()

		for rows.Next() {
			var u User
			if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.SCIMExternalID, &u.Version, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return nil, 0, err
			}
			users = append(users, u)
		}
		return users, total, nil
	}

	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		baseQuery := `FROM users WHERE org_id = $1`
		args := []interface{}{orgID}

		if filter != "" {
			if strings.Contains(filter, `userName eq "`) {
				email := strings.Split(strings.Split(filter, `userName eq "`)[1], `"`)[0]
				baseQuery += ` AND email = $2`
				args = append(args, email)
			}
		}

		err := conn.QueryRow(ctx, `SELECT COUNT(*) `+baseQuery, args...).Scan(&total)
		if err != nil {
			return err
		}

		query := `SELECT id, org_id, email, display_name, role, status, scim_external_id, version, created_at, updated_at ` + baseQuery + ` ORDER BY created_at OFFSET $` + fmt.Sprint(len(args)+1) + ` LIMIT $` + fmt.Sprint(len(args)+2)
		args = append(args, offset, limit)

		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var u User
			if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.SCIMExternalID, &u.Version, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return err
			}
			users = append(users, u)
		}
		return nil
	})

	return users, total, err
}

func (r *Repository) GetUserByExternalID(ctx context.Context, orgID, externalID string) (*User, error) {
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

	var orgID string
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + id
	err = tx.QueryRow(ctx, `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id
	`, name, slug).Scan(&orgID)
	if err != nil {
		return "", err
	}

	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return "", err
	}

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

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	orgID := rls.OrgID(ctx)
	var rt RefreshToken
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT id, org_id, user_id, family_id, expires_at, revoked 
			FROM refresh_tokens WHERE token_hash = $1
		`, tokenHash).Scan(&rt.ID, &rt.OrgID, &rt.UserID, &rt.FamilyID, &rt.ExpiresAt, &rt.Revoked)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rt, nil
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

func (r *Repository) ClaimRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	orgID := rls.OrgID(ctx)
	var rt RefreshToken
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			UPDATE refresh_tokens
			SET revoked = TRUE
			WHERE token_hash = $1
			  AND revoked = FALSE
			  AND expires_at > NOW()
			RETURNING id, org_id, user_id, family_id, expires_at
		`, tokenHash).Scan(&rt.ID, &rt.OrgID, &rt.UserID, &rt.FamilyID, &rt.ExpiresAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rt.Revoked = true
	return &rt, nil
}

func (r *Repository) RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE refresh_tokens 
			SET revoked = TRUE 
			WHERE family_id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1)
		`, tokenHash)
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

func (r *Repository) getUser(ctx context.Context, where string, args ...interface{}) (*User, error) {
	orgID := rls.OrgID(ctx)
	query := `SELECT id, org_id, email, display_name, role, status FROM users WHERE ` + where
	var u User
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, query, args...).Scan(&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Status)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) SaveWebAuthnCredential(ctx context.Context, orgID, userID string, cred WebAuthnCredential) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO webauthn_credentials (org_id, user_id, credential_id, public_key, attestation_type, sign_count)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, credential_id) DO UPDATE SET
				sign_count = EXCLUDED.sign_count,
				updated_at = NOW()
		`, orgID, userID, cred.CredentialID, cred.PublicKey, cred.AttestationType, cred.SignCount)
		return err
	})
}

func (r *Repository) ListWebAuthnCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	orgID := rls.OrgID(ctx)
	var credentials []WebAuthnCredential
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT credential_id, public_key, attestation_type, sign_count FROM webauthn_credentials WHERE user_id = $1
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c WebAuthnCredential
			if err := rows.Scan(&c.CredentialID, &c.PublicKey, &c.AttestationType, &c.SignCount); err != nil {
				return err
			}
			credentials = append(credentials, c)
		}
		return nil
	})
	return credentials, err
}

