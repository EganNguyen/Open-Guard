package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User represents a user row in the database (includes password_hash).
type User struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	PasswordHash *string    `json:"-"` // never exposed in JSON
	Status       string     `json:"status"`
	MFAEnabled   bool       `json:"mfa_enabled"`
	SCIMExtID    *string    `json:"scim_external_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// UserRepository handles user CRUD operations.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(
		&u.ID, &u.OrgID, &u.Email, &u.DisplayName,
		&u.PasswordHash, &u.Status, &u.MFAEnabled,
		&u.SCIMExtID, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

const userColumns = `id, org_id, email, display_name, password_hash, status, mfa_enabled, scim_external_id, created_at, updated_at, deleted_at`

// Create inserts a new user.
func (r *UserRepository) Create(ctx context.Context, orgID, email, displayName string, passwordHash *string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO users (org_id, email, display_name, password_hash)
		 VALUES ($1, $2, $3, $4)
		 RETURNING %s`, userColumns),
		orgID, email, displayName, passwordHash,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// GetByID retrieves a user by ID (excludes soft-deleted).
func (r *UserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM users WHERE id = $1 AND deleted_at IS NULL`, userColumns),
		id,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// GetByEmail retrieves a user by org_id and email.
func (r *UserRepository) GetByEmail(ctx context.Context, orgID, email string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM users WHERE org_id = $1 AND email = $2 AND deleted_at IS NULL`, userColumns),
		orgID, email,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// GetByEmailGlobal retrieves a user by email across all orgs (for login).
func (r *UserRepository) GetByEmailGlobal(ctx context.Context, email string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM users WHERE email = $1 AND deleted_at IS NULL LIMIT 1`, userColumns),
		email,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("get user by email global: %w", err)
	}
	return u, nil
}

// ListByOrg returns paginated users for an org.
func (r *UserRepository) ListByOrg(ctx context.Context, orgID string, page, perPage int) ([]*User, int, error) {
	// Count total
	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE org_id = $1 AND deleted_at IS NULL`,
		orgID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM users WHERE org_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userColumns),
		orgID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		err := rows.Scan(
			&u.ID, &u.OrgID, &u.Email, &u.DisplayName,
			&u.PasswordHash, &u.Status, &u.MFAEnabled,
			&u.SCIMExtID, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}

	return users, total, nil
}

// Update updates user fields.
func (r *UserRepository) Update(ctx context.Context, id, displayName, status string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE users SET display_name = $2, status = $3, updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING %s`, userColumns),
		id, displayName, status,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return u, nil
}

// SoftDelete sets the deleted_at timestamp.
func (r *UserRepository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}
	return nil
}

// UpdateStatus updates only the user's status.
func (r *UserRepository) UpdateStatus(ctx context.Context, id, status string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE users SET status = $2, updated_at = NOW()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING %s`, userColumns),
		id, status,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("update user status: %w", err)
	}
	return u, nil
}
