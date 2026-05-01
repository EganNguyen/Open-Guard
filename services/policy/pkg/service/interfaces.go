package service

import (
    "context"
    "encoding/json"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/openguard/services/policy/pkg/repository"
)

// PolicyStore handles policy CRUD operations.
type PolicyStore interface {
    GetMatchingPolicies(ctx context.Context, orgID string, subjectID string, userGroups []string) ([]repository.Policy, error)
    CreatePolicyTx(ctx context.Context, tx pgx.Tx, orgID, name, description string, logic json.RawMessage) (*repository.Policy, error)
    UpdatePolicyTx(ctx context.Context, tx pgx.Tx, orgID, policyID, name, description string, logic json.RawMessage) (*repository.Policy, error)
    DeletePolicyTx(ctx context.Context, tx pgx.Tx, orgID, policyID string) error
    ListPolicies(ctx context.Context, orgID string) ([]repository.Policy, error)
    GetPolicy(ctx context.Context, orgID, policyID string) (*repository.Policy, error)
}

// EvalLogStore handles evaluation logging.
type EvalLogStore interface {
    WriteEvalLog(ctx context.Context, log repository.EvalLog) error
    ListEvalLogs(ctx context.Context, orgID string, limit int) ([]repository.EvalLog, error)
}

// AssignmentStore handles policy assignments.
type AssignmentStore interface {
    ListAssignments(ctx context.Context, orgID string) ([]repository.Assignment, error)
    CreateAssignment(ctx context.Context, orgID, policyID, subjectID, subjectType string) (*repository.Assignment, error)
    DeleteAssignment(ctx context.Context, orgID, assignmentID string) error
}

// PolicyRepository combines all domain interfaces.
type PolicyRepository interface {
    PolicyStore
    EvalLogStore
    AssignmentStore
    Pool() *pgxpool.Pool
}
