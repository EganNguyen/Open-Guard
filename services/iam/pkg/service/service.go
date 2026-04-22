package service

import (
	"context"
	"fmt"

	"github.com/openguard/services/iam/pkg/repository"
	"golang.org/x/crypto/bcrypt"
)

// Service handles business logic for the IAM service.
type Service struct {
	repo *repository.Repository
	pool *AuthWorkerPool
}

// NewService creates a new service instance.
func NewService(repo *repository.Repository, pool *AuthWorkerPool) *Service {
	return &Service{
		repo: repo,
		pool: pool,
	}
}

func (s *Service) RegisterOrg(ctx context.Context, name string) (string, error) {
	return s.repo.CreateOrg(ctx, name)
}

func (s *Service) RegisterUser(ctx context.Context, orgID, email, password, displayName, role string) (string, error) {
	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return s.repo.CreateUser(ctx, orgID, email, string(hash), displayName, role)
}

func (s *Service) Login(ctx context.Context, email, password string) (map[string]interface{}, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// Use worker pool for bcrypt comparison
	err = s.pool.Compare(ctx, password, user["password_hash"].(string))
	if err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	delete(user, "password_hash")
	return user, nil
}

func (s *Service) GetConnector(ctx context.Context, id string) (map[string]interface{}, error) {
	return s.repo.GetConnectorByID(ctx, id)
}
func (s *Service) ListConnectors(ctx context.Context) ([]map[string]interface{}, error) {
	return s.repo.ListConnectors(ctx)
}
func (s *Service) ListUsers(ctx context.Context) ([]map[string]interface{}, error) {
	return s.repo.ListUsers(ctx)
}

func (s *Service) CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error) {
	return s.repo.CreateConnector(ctx, id, name, secret, uris)
}

func (s *Service) UpdateConnector(ctx context.Context, id, name string, uris []string) error {
	return s.repo.UpdateConnector(ctx, id, name, uris)
}

func (s *Service) DeleteConnector(ctx context.Context, id string) error {
	return s.repo.DeleteConnector(ctx, id)
}
