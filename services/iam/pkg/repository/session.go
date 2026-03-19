package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Session represents a user session in the database.
type Session struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	IPAddress   *string   `json:"ip_address,omitempty"`
	UserAgent   *string   `json:"user_agent,omitempty"`
	CountryCode *string   `json:"country_code,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Revoked     bool      `json:"revoked"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionRepository handles session CRUD operations.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository creates a new SessionRepository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Create inserts a new session.
func (r *SessionRepository) Create(ctx context.Context, userID, orgID string, ipAddress, userAgent *string, expiresAt time.Time) (*Session, error) {
	s := &Session{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO sessions (user_id, org_id, ip_address, user_agent, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, org_id, ip_address, user_agent, country_code, expires_at, revoked, created_at`,
		userID, orgID, ipAddress, userAgent, expiresAt,
	).Scan(&s.ID, &s.UserID, &s.OrgID, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// GetByID retrieves a session by ID.
func (r *SessionRepository) GetByID(ctx context.Context, id string) (*Session, error) {
	s := &Session{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, org_id, ip_address, user_agent, country_code, expires_at, revoked, created_at
		 FROM sessions WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.UserID, &s.OrgID, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return s, nil
}

// ListByUser returns active sessions for a user.
func (r *SessionRepository) ListByUser(ctx context.Context, userID string) ([]*Session, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, org_id, ip_address, user_agent, country_code, expires_at, revoked, created_at
		 FROM sessions WHERE user_id = $1 AND revoked = FALSE AND expires_at > NOW()
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.OrgID, &s.IPAddress, &s.UserAgent, &s.CountryCode, &s.ExpiresAt, &s.Revoked, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// Revoke marks a session as revoked.
func (r *SessionRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sessions SET revoked = TRUE WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
