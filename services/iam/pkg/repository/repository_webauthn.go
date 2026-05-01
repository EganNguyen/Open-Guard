package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

// SaveWebAuthnCredential saves or updates a WebAuthn credential for a user.
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

// ListWebAuthnCredentials lists all WebAuthn credentials for a user.
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
