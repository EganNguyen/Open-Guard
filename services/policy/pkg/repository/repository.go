package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("policy not found")

// Policy represents a stored policy record.
type Policy struct {
	ID          string          `json:"id"`
	OrgID       string          `json:"org_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Logic       json.RawMessage `json:"logic"`
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// EvalLog represents a policy evaluation log entry.
type EvalLog struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	SubjectID        string    `json:"subject_id"`
	Action           string    `json:"action"`
	Resource         string    `json:"resource"`
	Effect           string    `json:"effect"`
	MatchedPolicyIDs []string  `json:"matched_policy_ids"`
	CacheHit         bool      `json:"cache_hit"`
	LatencyMs        int       `json:"latency_ms"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
}

// Repository handles all database interactions for the policy service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new policy repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// SetRLS sets the RLS org_id context variable on a connection.
func SetRLS(ctx context.Context, tx pgx.Tx, orgID string) error {
	_, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
	return err
}

// ListPolicies returns all policies for an org.
func (r *Repository) ListPolicies(ctx context.Context, orgID string) ([]Policy, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetRLS(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, org_id, name, description, logic, version, created_at, updated_at
		FROM policies
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		var logicBytes []byte
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		p.Logic = json.RawMessage(logicBytes)
		policies = append(policies, p)
	}
	tx.Commit(ctx)
	return policies, nil
}

// GetPolicy returns a single policy by ID.
func (r *Repository) GetPolicy(ctx context.Context, orgID, policyID string) (*Policy, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetRLS(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	var p Policy
	var logicBytes []byte
	err = tx.QueryRow(ctx, `
		SELECT id, org_id, name, description, logic, version, created_at, updated_at
		FROM policies WHERE id = $1
	`, policyID).Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policy: %w", err)
	}
	p.Logic = json.RawMessage(logicBytes)
	tx.Commit(ctx)
	return &p, nil
}

// CreatePolicy inserts a new policy and returns the new record.
func (r *Repository) CreatePolicy(ctx context.Context, orgID, name, description string, logic json.RawMessage) (*Policy, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetRLS(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	id := uuid.New().String()
	var p Policy
	var logicBytes []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO policies (id, org_id, name, description, logic)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, org_id, name, description, logic, version, created_at, updated_at
	`, id, orgID, name, description, []byte(logic)).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create policy: %w", err)
	}
	p.Logic = json.RawMessage(logicBytes)
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &p, nil
}

// UpdatePolicy updates a policy and increments its version.
func (r *Repository) UpdatePolicy(ctx context.Context, orgID, policyID, name, description string, logic json.RawMessage) (*Policy, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetRLS(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	var p Policy
	var logicBytes []byte
	err = tx.QueryRow(ctx, `
		UPDATE policies
		SET name = $3, description = $4, logic = $5, version = version + 1, updated_at = NOW()
		WHERE id = $1 AND org_id = $2
		RETURNING id, org_id, name, description, logic, version, created_at, updated_at
	`, policyID, orgID, name, description, []byte(logic)).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update policy: %w", err)
	}
	p.Logic = json.RawMessage(logicBytes)
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &p, nil
}

// DeletePolicy removes a policy by ID.
func (r *Repository) DeletePolicy(ctx context.Context, orgID, policyID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetRLS(ctx, tx, orgID); err != nil {
		return fmt.Errorf("set rls: %w", err)
	}

	ct, err := tx.Exec(ctx, `DELETE FROM policies WHERE id = $1 AND org_id = $2`, policyID, orgID)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// GetMatchingPolicies fetches all policies for an org that could match a given action and resource.
// The JSONB logic field is returned for evaluation by the service layer.
func (r *Repository) GetMatchingPolicies(ctx context.Context, orgID string) ([]Policy, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := SetRLS(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id, org_id, name, description, logic, version, created_at, updated_at
		FROM policies
		WHERE org_id = $1
		ORDER BY created_at ASC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("get matching policies: %w", err)
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		var logicBytes []byte
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		p.Logic = json.RawMessage(logicBytes)
		policies = append(policies, p)
	}
	tx.Commit(ctx)
	return policies, nil
}

// WriteEvalLog records a policy evaluation result. Uses a plain pool connection (not RLS-gated).
func (r *Repository) WriteEvalLog(ctx context.Context, log EvalLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO policy_eval_log
			(id, org_id, subject_id, action, resource, effect, matched_policy_ids, cache_hit, latency_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		uuid.New().String(),
		log.OrgID,
		log.SubjectID,
		log.Action,
		log.Resource,
		log.Effect,
		log.MatchedPolicyIDs,
		log.CacheHit,
		log.LatencyMs,
	)
	return err
}

// ListEvalLogs returns recent evaluation logs for an org.
func (r *Repository) ListEvalLogs(ctx context.Context, orgID string, limit int) ([]EvalLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, subject_id, action, resource, effect, matched_policy_ids, cache_hit, latency_ms, evaluated_at
		FROM policy_eval_log
		WHERE org_id = $1
		ORDER BY evaluated_at DESC
		LIMIT $2
	`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("list eval logs: %w", err)
	}
	defer rows.Close()

	var logs []EvalLog
	for rows.Next() {
		var l EvalLog
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SubjectID, &l.Action, &l.Resource,
			&l.Effect, &l.MatchedPolicyIDs, &l.CacheHit, &l.LatencyMs, &l.EvaluatedAt); err != nil {
			return nil, fmt.Errorf("scan eval log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, nil
}
