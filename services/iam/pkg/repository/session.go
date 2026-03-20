package repository

import (
	"context"
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

type SessionRepository struct{}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{}
}

func (r *SessionRepository) Create(ctx context.Context, tx pgx.Tx, userID, orgID, refreshHash string, ipAddress, userAgent, countryCode *string, expiresAt time.Time) (*Session, error) {
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

func (r *SessionRepository) GetByID(ctx context.Context, tx pgx.Tx, orgID, id string) (*Session, error) {
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

func (r *SessionRepository) Revoke(ctx context.Context, tx pgx.Tx, orgID, id string) error {
	if err := rls.SetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("rls config: %w", err)
	}
	_, err := tx.Exec(ctx, `UPDATE sessions SET revoked = TRUE WHERE id = $1`, id)
	return err
}

func (r *SessionRepository) ListByUser(ctx context.Context, tx pgx.Tx, orgID, userID string) ([]*Session, error) {
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
