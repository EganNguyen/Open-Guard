package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

// CreateRefreshToken inserts a new refresh token.
func (r *Repository) CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO refresh_tokens (org_id, user_id, token_hash, family_id, expires_at)
			VALUES ($1, $2, $3, $4, $5)
		`, orgID, userID, tokenHash, familyID, expiresAt)
		return err
	})
}

// GetRefreshToken returns a refresh token by its hash.
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

// RevokeRefreshTokenFamily revokes all tokens in a family.
func (r *Repository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE refresh_tokens SET revoked = TRUE WHERE family_id = $1
		`, familyID)
		return err
	})
}

// ClaimRefreshToken claims (revokes) a refresh token by hash and returns it if valid.
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

// RevokeRefreshTokenFamilyByHash revokes all tokens in the family of the given token hash.
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

// DeleteRefreshToken deletes a refresh token by its hash.
func (r *Repository) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			DELETE FROM refresh_tokens WHERE token_hash = $1
		`, tokenHash)
		return err
	})
}
