package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DLPPolicy struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Name      string    `json:"name"`
	Rules     []string  `json:"rules"` // email, ssn, etc.
	Action    string    `json:"action"` // audit, block, mask
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type DLPFinding struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	EventID       string    `json:"event_id"`
	FindingType   string    `json:"finding_type"`
	Confidence    float64   `json:"confidence"`
	MatchedField  string    `json:"matched_field"`
	RedactedValue string    `json:"redacted_value"`
	CreatedAt     time.Time `json:"created_at"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(ctx context.Context, connStr string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, err
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) InitSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS dlp_policies (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id TEXT NOT NULL,
			name TEXT NOT NULL,
			rules TEXT[] NOT NULL,
			action TEXT NOT NULL,
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS dlp_findings (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id TEXT NOT NULL,
			event_id TEXT,
			finding_type TEXT NOT NULL,
			confidence DOUBLE PRECISION,
			matched_field TEXT,
			redacted_value TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,
	}
	for _, q := range queries {
		if _, err := r.pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListPolicies(ctx context.Context, orgID string) ([]DLPPolicy, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, org_id, name, rules, action, enabled, created_at FROM dlp_policies WHERE org_id = $1", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []DLPPolicy
	for rows.Next() {
		var p DLPPolicy
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Rules, &p.Action, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, nil
}

func (r *Repository) CreatePolicy(ctx context.Context, p *DLPPolicy) error {
	return r.pool.QueryRow(ctx, 
		"INSERT INTO dlp_policies (org_id, name, rules, action, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at",
		p.OrgID, p.Name, p.Rules, p.Action, p.Enabled,
	).Scan(&p.ID, &p.CreatedAt)
}

func (r *Repository) SaveFinding(ctx context.Context, f *DLPFinding) error {
	return r.pool.QueryRow(ctx,
		"INSERT INTO dlp_findings (org_id, event_id, finding_type, confidence, matched_field, redacted_value) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at",
		f.OrgID, f.EventID, f.FindingType, f.Confidence, f.MatchedField, f.RedactedValue,
	).Scan(&f.ID, &f.CreatedAt)
}

func (r *Repository) ListFindings(ctx context.Context, orgID string) ([]DLPFinding, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, org_id, event_id, finding_type, confidence, matched_field, redacted_value, created_at FROM dlp_findings WHERE org_id = $1 ORDER BY created_at DESC", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var findings []DLPFinding
	for rows.Next() {
		var f DLPFinding
		if err := rows.Scan(&f.ID, &f.OrgID, &f.EventID, &f.FindingType, &f.Confidence, &f.MatchedField, &f.RedactedValue, &f.CreatedAt); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, nil
}
