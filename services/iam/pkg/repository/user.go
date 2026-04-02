package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/rls"
)

type User struct {
	ID                 string     `json:"id"`
	OrgID              string     `json:"org_id"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"display_name"`
	PasswordHash       *string    `json:"-"`
	Status             string     `json:"status"`
	MFAEnabled         bool       `json:"mfa_enabled"`
	MFAMethod          *string    `json:"mfa_method"`
	SCIMExtID          *string    `json:"scim_external_id,omitempty"`
	ProvisioningStatus string     `json:"provisioning_status"`
	TierIsolation      string     `json:"tier_isolation"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

func scanUser(row pgx.Row) (*User, error) {
	u := &User{}
	err := row.Scan(
		&u.ID, &u.OrgID, &u.Email, &u.DisplayName,
		&u.PasswordHash, &u.Status, &u.MFAEnabled, &u.MFAMethod,
		&u.SCIMExtID, &u.ProvisioningStatus, &u.TierIsolation, 
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) CreateUser(ctx context.Context, tx pgx.Tx, orgID, email, displayName string, passwordHash *string) (*User, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO users (org_id, email, display_name, password_hash, provisioning_status, tier_isolation)
		 VALUES ($1, $2, $3, $4, 'complete', 'shared')
		 RETURNING id, org_id, email, display_name, password_hash, status, mfa_enabled, mfa_method, scim_external_id, provisioning_status, tier_isolation, created_at, updated_at, deleted_at`,
		orgID, email, displayName, passwordHash,
	)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *Repository) GetUserByID(ctx context.Context, tx pgx.Tx, orgID, id string) (*User, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	row := tx.QueryRow(ctx,
		`SELECT id, org_id, email, display_name, password_hash, status, mfa_enabled, mfa_method, scim_external_id, provisioning_status, tier_isolation, created_at, updated_at, deleted_at FROM users WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	return scanUser(row)
}

func (r *Repository) GetUserByEmail(ctx context.Context, tx pgx.Tx, orgID, email string) (*User, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	row := tx.QueryRow(ctx,
		`SELECT id, org_id, email, display_name, password_hash, status, mfa_enabled, mfa_method, scim_external_id, provisioning_status, tier_isolation, created_at, updated_at, deleted_at FROM users WHERE org_id = $1 AND email = $2 AND deleted_at IS NULL`,
		orgID, email,
	)
	return scanUser(row)
}

func (r *Repository) GetUserByEmailGlobal(ctx context.Context, tx pgx.Tx, email string) (*User, error) {
	// Global lookup requires bypassing RLS
	if err := rls.TxSetSessionVar(ctx, tx, ""); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	row := tx.QueryRow(ctx,
		`SELECT id, org_id, email, display_name, password_hash, status, mfa_enabled, mfa_method, scim_external_id, provisioning_status, tier_isolation, created_at, updated_at, deleted_at FROM users WHERE email = $1 AND deleted_at IS NULL LIMIT 1`,
		email,
	)
	return scanUser(row)
}

func (r *Repository) ListUsersByOrg(ctx context.Context, tx pgx.Tx, orgID string, page, perPage int) ([]*User, int, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, 0, fmt.Errorf("rls config: %w", err)
	}

	var total int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := tx.Query(ctx,
		`SELECT id, org_id, email, display_name, password_hash, status, mfa_enabled, mfa_method, scim_external_id, provisioning_status, tier_isolation, created_at, updated_at, deleted_at FROM users WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}

	return users, total, nil
}

func (r *Repository) UpdateUserStatus(ctx context.Context, tx pgx.Tx, orgID, id, status string) (*User, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	row := tx.QueryRow(ctx,
		`UPDATE users SET status = $1, updated_at = NOW()
		 WHERE id = $2 AND deleted_at IS NULL RETURNING id, org_id, email, display_name, password_hash, status, mfa_enabled, mfa_method, scim_external_id, provisioning_status, tier_isolation, created_at, updated_at, deleted_at`,
		status, id,
	)
	return scanUser(row)
}

func (r *Repository) SoftDeleteUser(ctx context.Context, tx pgx.Tx, orgID, id string) error {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}
	_, err := tx.Exec(ctx, `UPDATE users SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}
