package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

// CreateSession inserts a new session record.
func (r *Repository) CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error {
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO sessions (org_id, user_id, jti, user_agent, ip_address, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, orgID, userID, jti, userAgent, ipAddress, expiresAt)
		return err
	})
}

// GetActiveJTIs returns all active JTIs for a user.
func (r *Repository) GetActiveJTIs(ctx context.Context, userID string) ([]string, error) {
	orgID := rls.OrgID(ctx)
	var jtis []string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT jti FROM sessions WHERE user_id = $1 AND expires_at > NOW()
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var jti string
			if err := rows.Scan(&jti); err != nil {
				return err
			}
			jtis = append(jtis, jti)
		}
		return nil
	})
	return jtis, err
}

// GetSessionTTL returns the remaining TTL for a session.
func (r *Repository) GetSessionTTL(ctx context.Context, jti string) time.Duration {
	orgID := rls.OrgID(ctx)
	var expiresAt time.Time
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT expires_at FROM sessions WHERE jti = $1
		`, jti).Scan(&expiresAt)
	})
	if err != nil {
		return 0
	}
	ttl := time.Until(expiresAt)
	if ttl < 0 {
		return 0
	}
	return ttl
}

// RevokeSessions revokes all sessions for a user in the database.
func (r *Repository) RevokeSessions(ctx context.Context, userID string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			DELETE FROM sessions WHERE user_id = $1
		`, userID)
		return err
	})
}

// GetSessionByUserID returns the most recent active session for a user.
func (r *Repository) GetSessionByUserID(ctx context.Context, userID string) (*Session, error) {
	orgID := rls.OrgID(ctx)
	var jti, ua, ip string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT jti, user_agent, ip_address FROM sessions
			WHERE user_id = $1 AND expires_at > NOW()
			ORDER BY created_at DESC LIMIT 1
		`, userID).Scan(&jti, &ua, &ip)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Session{
		JTI:       jti,
		UserAgent: ua,
		IPAddress: ip,
	}, nil
}
