package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type AuthCode struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	ClientID    string    `json:"client_id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	RedirectURI string    `json:"redirect_uri"`
	Scope       string    `json:"scope"`
	State       string    `json:"state"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r *Repository) CreateAuthCode(ctx context.Context, tx pgx.Tx, code *AuthCode) error {
	query := `
		INSERT INTO oidc_auth_codes (
			code, client_id, user_id, org_id, redirect_uri, scope, state, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING id, created_at
	`
	return tx.QueryRow(ctx, query,
		code.Code, code.ClientID, code.UserID, code.OrgID,
		code.RedirectURI, code.Scope, code.State, code.ExpiresAt,
	).Scan(&code.ID, &code.CreatedAt)
}

func (r *Repository) GetAuthCodeByCode(ctx context.Context, tx pgx.Tx, codeStr string) (*AuthCode, error) {
	query := `
		SELECT 
			id, code, client_id, user_id, org_id, redirect_uri, scope, state, expires_at, created_at
		FROM oidc_auth_codes
		WHERE code = $1 AND expires_at > NOW()
	`
	var c AuthCode
	err := tx.QueryRow(ctx, query, codeStr).Scan(
		&c.ID, &c.Code, &c.ClientID, &c.UserID, &c.OrgID,
		&c.RedirectURI, &c.Scope, &c.State, &c.ExpiresAt, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) DeleteAuthCode(ctx context.Context, tx pgx.Tx, id string) error {
	query := `DELETE FROM oidc_auth_codes WHERE id = $1`
	_, err := tx.Exec(ctx, query, id)
	return err
}
