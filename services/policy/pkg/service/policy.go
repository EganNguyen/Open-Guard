package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/openguard/policy/pkg/repository"
	"github.com/openguard/shared/models"
)

// ErrBadRequest is returned for invalid input validation.
var ErrBadRequest = errors.New("bad request")

// ErrNotFound is returned when a resource is not found.
var ErrNotFound = errors.New("not found")

// PolicyRepository defines the persistence interface for policies.
type PolicyRepository interface {
	Create(ctx context.Context, p *models.Policy) error
	GetByID(ctx context.Context, orgID, policyID string) (*models.Policy, error)
	ListByOrg(ctx context.Context, orgID string) ([]*models.Policy, error)
	ListEnabledForOrg(ctx context.Context, orgID string) ([]*models.Policy, error)
	Update(ctx context.Context, p *models.Policy) error
	Delete(ctx context.Context, orgID, policyID string) error
	LogEvaluation(ctx context.Context, log *repository.EvalLog) error
}

// PolicyService is the thin business logic layer wrapping the repository.
type PolicyService struct {
	repo PolicyRepository
}

func NewPolicyService(repo PolicyRepository) *PolicyService {
	return &PolicyService{repo: repo}
}

func (s *PolicyService) Create(ctx context.Context, p *models.Policy) error {
	if p.Name == "" {
		return fmt.Errorf("%w: policy name is required", ErrBadRequest)
	}
	if p.OrgID == "" {
		return fmt.Errorf("%w: org_id is required", ErrBadRequest)
	}
	return s.repo.Create(ctx, p)
}

func (s *PolicyService) Get(ctx context.Context, orgID, policyID string) (*models.Policy, error) {
	return s.repo.GetByID(ctx, orgID, policyID)
}

func (s *PolicyService) List(ctx context.Context, orgID string) ([]*models.Policy, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

func (s *PolicyService) Update(ctx context.Context, p *models.Policy) error {
	if p.Name == "" {
		return fmt.Errorf("%w: policy name is required", ErrBadRequest)
	}
	return s.repo.Update(ctx, p)
}

func (s *PolicyService) Delete(ctx context.Context, orgID, policyID string) error {
	return s.repo.Delete(ctx, orgID, policyID)
}
