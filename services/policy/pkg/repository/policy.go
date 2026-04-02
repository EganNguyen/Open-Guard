package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/models"
	"github.com/openguard/shared/outbox"
	sharedkafka "github.com/openguard/shared/kafka"
	"github.com/openguard/shared/rls"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// DBPool abstracts the pgxpool connection to allow for isolated unit testing
// and RLS-wrapped pools from the shared package.
type DBPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PolicyRepository handles persistence of policies and assignments.
type PolicyRepository struct {
	pool   DBPool
	outbox *outbox.Writer
}

func NewPolicyRepository(pool DBPool, outbox *outbox.Writer) *PolicyRepository {
	return &PolicyRepository{pool: pool, outbox: outbox}
}

// Create inserts a new policy and writes a policy.changes event to the outbox — atomically.
func (r *PolicyRepository) Create(ctx context.Context, p *models.Policy) error {
	p.ID = uuid.NewString()
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	return r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, p.OrgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO policies (id, org_id, name, description, type, rules, enabled, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, p.ID, p.OrgID, p.Name, p.Description, p.Type, p.Rules, p.Enabled, p.CreatedBy, p.CreatedAt, p.UpdatedAt)
		if err != nil {
			return err
		}

		return r.writeOutbox(ctx, tx, p.OrgID, "policy.created", p)
	})
}

// Update modifies an existing policy and writes a policy.changes event.
func (r *PolicyRepository) Update(ctx context.Context, p *models.Policy) error {
	p.UpdatedAt = time.Now()

	return r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, p.OrgID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE policies
			SET name=$1, description=$2, type=$3, rules=$4, enabled=$5, updated_at=$6
			WHERE id=$7 AND org_id=$8
		`, p.Name, p.Description, p.Type, p.Rules, p.Enabled, p.UpdatedAt, p.ID, p.OrgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}

		return r.writeOutbox(ctx, tx, p.OrgID, "policy.updated", p)
	})
}

// Delete removes a policy and writes a policy.changes event.
func (r *PolicyRepository) Delete(ctx context.Context, orgID, policyID string) error {
	return r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM policies WHERE id=$1 AND org_id=$2`, policyID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}

		payload := map[string]string{"id": policyID, "org_id": orgID}
		return r.writeOutbox(ctx, tx, orgID, "policy.deleted", payload)
	})
}

// GetByID retrieves a single policy by ID.
func (r *PolicyRepository) GetByID(ctx context.Context, orgID, policyID string) (*models.Policy, error) {
	var p *models.Policy
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			SELECT id, org_id, name, description, type, rules, enabled, created_by, created_at, updated_at
			FROM policies WHERE id=$1 AND org_id=$2
		`, policyID, orgID)
		var scanErr error
		p, scanErr = scanPolicy(row)
		return scanErr
	})
	return p, err
}

// ListByOrg returns all policies for an organization.
func (r *PolicyRepository) ListByOrg(ctx context.Context, orgID string) ([]*models.Policy, error) {
	policies := []*models.Policy{}
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, org_id, name, description, type, rules, enabled, created_by, created_at, updated_at
			FROM policies WHERE org_id=$1 ORDER BY created_at DESC
		`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, err := scanPolicy(rows)
			if err != nil {
				return err
			}
			policies = append(policies, p)
		}
		return rows.Err()
	})
	return policies, err
}

// ListEnabledForOrg returns all enabled policies for an organization (used by evaluator).
func (r *PolicyRepository) ListEnabledForOrg(ctx context.Context, orgID string) ([]*models.Policy, error) {
	policies := []*models.Policy{}
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, org_id, name, description, type, rules, enabled, created_by, created_at, updated_at
			FROM policies WHERE org_id=$1 AND enabled=TRUE ORDER BY created_at
		`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, err := scanPolicy(rows)
			if err != nil {
				return err
			}
			policies = append(policies, p)
		}
		return rows.Err()
	})
	return policies, err
}

// LogEvaluation writes an entry to policy_eval_log for audit trail.
func (r *PolicyRepository) LogEvaluation(ctx context.Context, log *EvalLog) error {
	return r.withTx(ctx, func(tx pgx.Tx) error {
		if err := rls.TxSetSessionVar(ctx, tx, log.OrgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO policy_eval_log (id, org_id, user_id, action, resource, result, policy_ids, latency_ms, cached, evaluated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, uuid.NewString(), log.OrgID, log.UserID, log.Action, log.Resource,
			log.Result, log.PolicyIDs, log.LatencyMs, log.Cached, time.Now())
		return err
	})
}

// EvalLog represents a row in policy_eval_log.
type EvalLog struct {
	OrgID     string
	UserID    string
	Action    string
	Resource  string
	Result    bool
	PolicyIDs []string
	LatencyMs int
	Cached    bool
}

// withTx runs fn inside a transaction.
func (r *PolicyRepository) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// writeOutbox constructs an EventEnvelope and persists it to the outbox within the given transaction.
func (r *PolicyRepository) writeOutbox(ctx context.Context, tx pgx.Tx, orgID, eventType string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	envelope := models.EventEnvelope{
		ID:        uuid.NewString(),
		Type:      eventType,
		OrgID:     orgID,
		ActorType: "service",
		Source:    "policy",
		SchemaVer: "1.0",
		Idempotent: uuid.NewString(),
		OccurredAt: time.Now(),
		Payload:   payloadBytes,
	}

	return r.outbox.Write(ctx, tx, sharedkafka.TopicPolicyChanges, orgID, orgID, envelope)
}

// scanPolicy scans a policy row (works with pgx.Row and pgx.Rows via the scanner interface).
type policyScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row policyScanner) (*models.Policy, error) {
	var p models.Policy
	var rules []byte
	err := row.Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &p.Type, &rules,
		&p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Rules = rules
	return &p, nil
}
