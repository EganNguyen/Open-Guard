package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/rls"
)

type Session struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	RefreshHash string    `json:"-"`
	IPAddress   *string   `json:"ip_address,omitempty"`
	UserAgent   *string   `json:"user_agent,omitempty"`
	CountryCode *string   `json:"country_code,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Revoked     bool      `json:"revoked"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r *Repository) CreateSession(ctx context.Context, tx pgx.Tx, userID, orgID, refreshHash string, ipAddress, userAgent, countryCode *string, expiresAt time.Time) (*Session, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	s := &Session{}
	err := tx.QueryRow(ctx,
		`INSERT INTO sessions (user_id, org_id, refresh_hash, ip_address, user_agent, country_code, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, user_id, org_id, refresh_hash, ip_address::TEXT, user_agent, country_code, expires_at, revoked, created_at`,
		userID, orgID, refreshHash, ipAddress, userAgent, countryCode, expiresAt,
	).Scan(&s.ID, &s.UserID, &s.OrgID, &s.RefreshHash, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

func (r *Repository) GetSessionByID(ctx context.Context, tx pgx.Tx, orgID, id string) (*Session, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	s := &Session{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, org_id, refresh_hash, ip_address::TEXT, user_agent, country_code, expires_at, revoked, created_at
		 FROM sessions WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.UserID, &s.OrgID, &s.RefreshHash, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return s, nil
}

func (r *Repository) GetActiveSession(ctx context.Context, tx pgx.Tx, orgID, id string) (*Session, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	s := &Session{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, org_id, refresh_hash, ip_address::TEXT, user_agent, country_code, expires_at, revoked, created_at
		 FROM sessions WHERE id = $1 AND revoked = FALSE AND expires_at > $2`,
		id, time.Now(),
	).Scan(&s.ID, &s.UserID, &s.OrgID, &s.RefreshHash, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("session not found or expired")
		}
		return nil, fmt.Errorf("get active session: %w", err)
	}
	return s, nil
}

func (r *Repository) GetActiveSessionByHashGlobal(ctx context.Context, tx pgx.Tx, refreshHash string) (*Session, error) {
	if err := rls.SetSessionVar(ctx, tx, ""); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	s := &Session{}
	err := tx.QueryRow(ctx,
		`SELECT id, user_id, org_id, refresh_hash, ip_address::TEXT, user_agent, country_code, expires_at, revoked, created_at
		 FROM sessions WHERE refresh_hash = $1 AND revoked = FALSE AND expires_at > $2`,
		refreshHash, time.Now(),
	).Scan(&s.ID, &s.UserID, &s.OrgID, &s.RefreshHash, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("session not found or expired")
		}
		return nil, fmt.Errorf("get active session by hash: %w", err)
	}

	// Restore RLS setting for this transaction
	if err := rls.SetSessionVar(ctx, tx, s.OrgID); err != nil {
		return nil, fmt.Errorf("rls config restore: %w", err)
	}

	return s, nil
}

func (r *Repository) UpdateSessionCredentials(ctx context.Context, tx pgx.Tx, orgID, id, newRefreshHash string, ipAddress, userAgent *string, newExpiresAt time.Time) error {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}
	_, err := tx.Exec(ctx,
		`UPDATE sessions SET refresh_hash = $1, ip_address = $2, user_agent = $3, expires_at = $4 WHERE id = $5`,
		newRefreshHash, ipAddress, userAgent, newExpiresAt, id)
	return err
}

func (r *Repository) ExtendSessionExpiry(ctx context.Context, tx pgx.Tx, orgID, id string, newExpiresAt time.Time) error {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}
	_, err := tx.Exec(ctx, `UPDATE sessions SET expires_at = $1 WHERE id = $2`, newExpiresAt, id)
	return err
}

func (r *Repository) RevokeSession(ctx context.Context, tx pgx.Tx, orgID, id string) error {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}
	_, err := tx.Exec(ctx, `UPDATE sessions SET revoked = TRUE WHERE id = $1`, id)
	return err
}

func (r *Repository) ListSessionsByUser(ctx context.Context, tx pgx.Tx, orgID, userID string) ([]*Session, error) {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT id, user_id, org_id, refresh_hash, ip_address::TEXT, user_agent, country_code, expires_at, revoked, created_at
		 FROM sessions WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.OrgID, &s.RefreshHash, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
