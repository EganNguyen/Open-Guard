package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/rls"
)

type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	OrgID      string     `json:"org_id"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Revoked    bool       `json:"revoked"`
	CreatedAt  time.Time  `json:"created_at"`
}

type APITokenRepository struct{}

func NewAPITokenRepository() *APITokenRepository {
	return &APITokenRepository{}
}

func (r *APITokenRepository) Create(ctx context.Context, tx pgx.Tx, userID, orgID, name, tokenHash, prefix string, scopes []string, expiresAt *time.Time) (*APIToken, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	t := &APIToken{}
	err := tx.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, org_id, name, token_hash, prefix, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, user_id, org_id, name, token_hash, prefix, scopes, expires_at, last_used_at, revoked, created_at`,
		userID, orgID, name, tokenHash, prefix, scopes, expiresAt,
	).Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.TokenHash, &t.Prefix, &t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.Revoked, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create api token: %w", err)
	}
	return t, nil
}

func (r *APITokenRepository) ListByUser(ctx context.Context, tx pgx.Tx, orgID, userID string) ([]*APIToken, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT id, user_id, org_id, name, token_hash, prefix, scopes, expires_at, last_used_at, revoked, created_at
		 FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		t := &APIToken{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.OrgID, &t.Name, &t.TokenHash, &t.Prefix, &t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.Revoked, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (r *APITokenRepository) Revoke(ctx context.Context, tx pgx.Tx, orgID, id string) error {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}

	_, err := tx.Exec(ctx, `UPDATE api_tokens SET revoked = TRUE WHERE id = $1`, id)
	return err
}
