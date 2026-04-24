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
	"github.com/openguard/shared/rls"
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
	CacheHit         string    `json:"cache_hit"`
	LatencyMs        int       `json:"latency_ms"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
}

// Assignment represents a policy assignment to a subject.
type Assignment struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	PolicyID    string    `json:"policy_id"`
	SubjectID   string    `json:"subject_id"`
	SubjectType string    `json:"subject_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// Repository handles all database interactions for the policy service.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new policy repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Pool returns the underlying database pool.
func (r *Repository) Pool() *pgxpool.Pool {
	return r.pool
}

// withOrgContext acquires a connection from the pool, sets app.org_id for
// this session, executes fn, and releases the connection.
// This ensures PostgreSQL RLS policies filter by the correct tenant.
func (r *Repository) withOrgContext(ctx context.Context, fn func(ctx context.Context, conn *pgxpool.Conn) error) error {
	orgID := rls.OrgID(ctx)
	if orgID == "" {
		return fmt.Errorf("withOrgContext: org_id missing from context")
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("withOrgContext: acquire connection: %w", err)
	}
	defer conn.Release()

	if err := rls.SetSessionVar(ctx, conn, orgID); err != nil {
		return fmt.Errorf("withOrgContext: set session var: %w", err)
	}

	return fn(ctx, conn)
}

// ListPolicies returns all policies for an org.
func (r *Repository) ListPolicies(ctx context.Context, orgID string) ([]Policy, error) {
	var policies []Policy
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, name, description, logic, version, created_at, updated_at
			FROM policies
			ORDER BY created_at DESC
		`)
		if err != nil {
			return fmt.Errorf("list policies: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p Policy
			var logicBytes []byte
			if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt); err != nil {
				return fmt.Errorf("scan policy: %w", err)
			}
			p.Logic = json.RawMessage(logicBytes)
			policies = append(policies, p)
		}
		return nil
	})
	return policies, err
}

// GetPolicy returns a single policy by ID.
func (r *Repository) GetPolicy(ctx context.Context, orgID, policyID string) (*Policy, error) {
	var p Policy
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		var logicBytes []byte
		err := conn.QueryRow(ctx, `
			SELECT id, org_id, name, description, logic, version, created_at, updated_at
			FROM policies WHERE id = $1
		`, policyID).Scan(&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("get policy: %w", err)
		}
		p.Logic = json.RawMessage(logicBytes)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePolicy inserts a new policy and returns the new record.
func (r *Repository) CreatePolicy(ctx context.Context, orgID, name, description string, logic json.RawMessage) (*Policy, error) {
	var p Policy
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		id := uuid.New().String()
		var logicBytes []byte
		err := conn.QueryRow(ctx, `
			INSERT INTO policies (id, org_id, name, description, logic)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, org_id, name, description, logic, version, created_at, updated_at
		`, id, orgID, name, description, []byte(logic)).Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("create policy: %w", err)
		}
		p.Logic = json.RawMessage(logicBytes)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePolicy updates a policy and increments its version.
func (r *Repository) UpdatePolicy(ctx context.Context, orgID, policyID, name, description string, logic json.RawMessage) (*Policy, error) {
	var p Policy
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		var logicBytes []byte
		err := conn.QueryRow(ctx, `
			UPDATE policies
			SET name = $3, description = $4, logic = $5, version = version + 1, updated_at = NOW()
			WHERE id = $1 AND org_id = $2
			RETURNING id, org_id, name, description, logic, version, created_at, updated_at
		`, policyID, orgID, name, description, []byte(logic)).Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Description, &logicBytes, &p.Version, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("update policy: %w", err)
		}
		p.Logic = json.RawMessage(logicBytes)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// DeletePolicy removes a policy by ID.
func (r *Repository) DeletePolicy(ctx context.Context, orgID, policyID string) error {
	return r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		ct, err := conn.Exec(ctx, `DELETE FROM policies WHERE id = $1 AND org_id = $2`, policyID, orgID)
		if err != nil {
			return fmt.Errorf("delete policy: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CreatePolicyTx inserts a new policy within an existing transaction.
func (r *Repository) CreatePolicyTx(ctx context.Context, tx pgx.Tx, orgID, name, description string, logic json.RawMessage) (*Policy, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	id := uuid.New().String()
	var p Policy
	var logicBytes []byte
	err := tx.QueryRow(ctx, `
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
	return &p, nil
}

// UpdatePolicyTx updates a policy within an existing transaction.
func (r *Repository) UpdatePolicyTx(ctx context.Context, tx pgx.Tx, orgID, policyID, name, description string, logic json.RawMessage) (*Policy, error) {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return nil, fmt.Errorf("set rls: %w", err)
	}

	var p Policy
	var logicBytes []byte
	err := tx.QueryRow(ctx, `
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
	return &p, nil
}

// DeletePolicyTx removes a policy within an existing transaction.
func (r *Repository) DeletePolicyTx(ctx context.Context, tx pgx.Tx, orgID, policyID string) error {
	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return fmt.Errorf("set rls: %w", err)
	}

	ct, err := tx.Exec(ctx, `DELETE FROM policies WHERE id = $1 AND org_id = $2`, policyID, orgID)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetMatchingPolicies fetches all policies for an org that could match a given action and resource.
// It joins with policy_assignments to filter by subject if provided.
func (r *Repository) GetMatchingPolicies(ctx context.Context, orgID string, subjectID string, userGroups []string) ([]Policy, error) {
	var policies []Policy
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		query := `
			SELECT DISTINCT p.id, p.org_id, p.name, p.description, p.logic, p.version, p.created_at, p.updated_at
			FROM policies p
			LEFT JOIN policy_assignments pa ON p.id = pa.policy_id
			WHERE p.org_id = $1
			AND (
				pa.id IS NULL OR 
				(pa.subject_id = $2::UUID AND pa.subject_type = 'user') OR 
				(pa.subject_id = ANY($3::UUID[]) AND pa.subject_type = 'group')
			)
			ORDER BY p.created_at ASC
		`

		if _, err := uuid.Parse(subjectID); err != nil {
			query = `
				SELECT p.id, p.org_id, p.name, p.description, p.logic, p.version, p.created_at, p.updated_at
				FROM policies p
				LEFT JOIN policy_assignments pa ON p.id = pa.policy_id
				WHERE p.org_id = $1 AND pa.id IS NULL
				ORDER BY p.created_at ASC
			`
			rows, err := conn.Query(ctx, query, orgID)
			if err != nil {
				return fmt.Errorf("get global policies: %w", err)
			}
			defer rows.Close()
			pols, err := r.scanPolicies(rows)
			policies = pols
			return err
		}

		validGroups := []string{}
		for _, g := range userGroups {
			if _, err := uuid.Parse(g); err == nil {
				validGroups = append(validGroups, g)
			}
		}

		rows, err := conn.Query(ctx, query, orgID, subjectID, validGroups)
		if err != nil {
			return fmt.Errorf("get matching policies: %w", err)
		}
		defer rows.Close()

		pols, err := r.scanPolicies(rows)
		policies = pols
		return err
	})
	return policies, err
}

func (r *Repository) scanPolicies(rows pgx.Rows) ([]Policy, error) {
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
	return policies, nil
}

// WriteEvalLog records a policy evaluation result under the tenant RLS context.
func (r *Repository) WriteEvalLog(ctx context.Context, log EvalLog) error {
	return r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
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
	})
}

// ListEvalLogs returns recent evaluation logs for an org.
func (r *Repository) ListEvalLogs(ctx context.Context, orgID string, limit int) ([]EvalLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var logs []EvalLog
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, subject_id, action, resource, effect, matched_policy_ids, cache_hit, latency_ms, evaluated_at
			FROM policy_eval_log
			WHERE org_id = $1
			ORDER BY evaluated_at DESC
			LIMIT $2
		`, orgID, limit)
		if err != nil {
			return fmt.Errorf("list eval logs: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var l EvalLog
			if err := rows.Scan(&l.ID, &l.OrgID, &l.SubjectID, &l.Action, &l.Resource,
				&l.Effect, &l.MatchedPolicyIDs, &l.CacheHit, &l.LatencyMs, &l.EvaluatedAt); err != nil {
				return fmt.Errorf("scan eval log: %w", err)
			}
			logs = append(logs, l)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *Repository) ListAssignments(ctx context.Context, orgID string) ([]Assignment, error) {
	var assignments []Assignment
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, policy_id, subject_id, subject_type, created_at
			FROM policy_assignments
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a Assignment
			if err := rows.Scan(&a.ID, &a.OrgID, &a.PolicyID, &a.SubjectID, &a.SubjectType, &a.CreatedAt); err != nil {
				return err
			}
			assignments = append(assignments, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return assignments, nil
}

func (r *Repository) CreateAssignment(ctx context.Context, orgID, policyID, subjectID, subjectType string) (*Assignment, error) {
	var a Assignment
	err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		err := conn.QueryRow(ctx, `
			INSERT INTO policy_assignments (org_id, policy_id, subject_id, subject_type)
			VALUES ($1, $2, $3, $4)
			RETURNING id, org_id, policy_id, subject_id, subject_type, created_at
		`, orgID, policyID, subjectID, subjectType).Scan(&a.ID, &a.OrgID, &a.PolicyID, &a.SubjectID, &a.SubjectType, &a.CreatedAt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) DeleteAssignment(ctx context.Context, orgID, assignmentID string) error {
	return r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
		ct, err := conn.Exec(ctx, `DELETE FROM policy_assignments WHERE id = $1 AND org_id = $2`, assignmentID, orgID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
