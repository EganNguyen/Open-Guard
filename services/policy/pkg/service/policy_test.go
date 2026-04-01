package service

import (
	"context"
	"errors"
	"testing"

	"github.com/openguard/policy/pkg/repository"
	"github.com/openguard/shared/models"
)

type mockRepo struct {
	createErr error
	updateErr error
}

func (m *mockRepo) Create(ctx context.Context, p *models.Policy) error             { return m.createErr }
func (m *mockRepo) GetByID(ctx context.Context, o, p string) (*models.Policy, error) { return nil, repository.ErrNotFound }
func (m *mockRepo) ListByOrg(ctx context.Context, o string) ([]*models.Policy, error) { return []*models.Policy{}, nil }
func (m *mockRepo) ListEnabledForOrg(ctx context.Context, o string) ([]*models.Policy, error) {
	return []*models.Policy{}, nil
}
func (m *mockRepo) Update(ctx context.Context, p *models.Policy) error { return m.updateErr }
func (m *mockRepo) Delete(ctx context.Context, o, p string) error    { return nil }
func (m *mockRepo) LogEvaluation(ctx context.Context, l *repository.EvalLog) error { return nil }

func TestPolicyService_Create(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, nil, 0, nil)
	ctx := context.Background()

	t.Run("valid creation", func(t *testing.T) {
		p := &models.Policy{Name: "Test Policy", OrgID: "org1"}
		err := svc.Create(ctx, p)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		p := &models.Policy{OrgID: "org1"}
		err := svc.Create(ctx, p)
		if !errors.Is(err, models.ErrBadRequest) {
			t.Errorf("expected ErrBadRequest, got %v", err)
		}
	})

	t.Run("missing orgID", func(t *testing.T) {
		p := &models.Policy{Name: "Test"}
		err := svc.Create(ctx, p)
		if !errors.Is(err, models.ErrBadRequest) {
			t.Errorf("expected ErrBadRequest, got %v", err)
		}
	})
}

func TestPolicyService_Update(t *testing.T) {
	repo := &mockRepo{}
	svc := New(repo, nil, 0, nil)
	ctx := context.Background()

	t.Run("valid update", func(t *testing.T) {
		p := &models.Policy{ID: "p1", Name: "Updated Name", OrgID: "org1"}
		err := svc.Update(ctx, p)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		p := &models.Policy{ID: "p1", OrgID: "org1"}
		err := svc.Update(ctx, p)
		if !errors.Is(err, models.ErrBadRequest) {
			t.Errorf("expected ErrBadRequest, got %v", err)
		}
	})
}
