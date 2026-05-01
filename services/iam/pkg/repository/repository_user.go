package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

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

// GetUserByEmail returns a user by email (login path, uses openguard_login role).
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
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

// GetUserByID returns a user by ID.
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

// IncrementFailedLogin increments the failed login count for a user.
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

// ResetFailedLogin resets the failed login count and locked_until for a user.
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

// LockAccount locks a user account until a specified time.
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

// ListUsers lists users in an org with optional SCIM filter.
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

// ListUsersPaginated lists users with pagination and optional SCIM filter.
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

// GetUserByExternalID returns a user by SCIM external ID.
func (r *Repository) GetUserByExternalID(ctx context.Context, orgID, externalID string) (*User, error) {
	return r.getUser(ctx, "org_id = $1 AND scim_external_id = $2", orgID, externalID)
}

// UpdateUserSCIM updates a user's SCIM external ID and status.
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

// EnableUserMFA enables or disables MFA for a user.
func (r *Repository) EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE users SET mfa_enabled = $1, mfa_method = $2 WHERE id = $3
		`, enabled, method, userID)
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

// getUser is a helper to get a user by a where clause.
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
