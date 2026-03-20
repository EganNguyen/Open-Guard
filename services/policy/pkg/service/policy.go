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

// PolicyService is the thin business logic layer wrapping the repository.
type PolicyService struct {
	repo *repository.PolicyRepository
}

func NewPolicyService(repo *repository.PolicyRepository) *PolicyService {
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
