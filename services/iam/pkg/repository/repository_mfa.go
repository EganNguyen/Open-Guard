package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

// GetMFAConfig returns an MFA config for a user and type.
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

// ListMFAConfigs lists all MFA configs for a user.
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

// UpsertMFAConfig creates or updates an MFA config for a user.
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

// StoreBackupCodes stores backup codes for a user's TOTP config.
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

// ConsumeBackupCode removes a used backup code and returns true if found.
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
