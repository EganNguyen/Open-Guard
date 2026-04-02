package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/rls"
)

type MFAConfig struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	Type        string    `json:"type"` // "totp"
	Secret      string    `json:"-"`    // encrypted AES ciphertext
	BackupCodes []string  `json:"-"`    // encrypted 
	Verified    bool      `json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r *Repository) CreateMFAConfig(ctx context.Context, tx pgx.Tx, orgID, userID, mfaType, secret string) (*MFAConfig, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	m := &MFAConfig{}
	err := tx.QueryRow(ctx,
		`INSERT INTO mfa_configs (user_id, org_id, type, secret)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, org_id, type, secret, backup_codes, verified, created_at`,
		userID, orgID, mfaType, secret,
	).Scan(&m.ID, &m.UserID, &m.OrgID, &m.Type, &m.Secret, &m.BackupCodes, &m.Verified, &m.CreatedAt)
	
	if err != nil {
		return nil, fmt.Errorf("create mfa config: %w", err)
	}
	return m, nil
}

func (r *Repository) VerifyMFA(ctx context.Context, tx pgx.Tx, orgID, userID string) error {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}

	_, err := tx.Exec(ctx, `UPDATE mfa_configs SET verified = TRUE WHERE user_id = $1`, userID)
	return err
}

func (r *Repository) GetMFAByUserID(ctx context.Context, tx pgx.Tx, orgID, userID string) (*MFAConfig, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	m := &MFAConfig{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, org_id, type, secret, backup_codes, verified, created_at
		 FROM mfa_configs WHERE user_id = $1`,
		userID,
	).Scan(&m.ID, &m.UserID, &m.OrgID, &m.Type, &m.Secret, &m.BackupCodes, &m.Verified, &m.CreatedAt)
	
	if err != nil {
		return nil, fmt.Errorf("get mfa config: %w", err)
	}
	return m, nil
}
