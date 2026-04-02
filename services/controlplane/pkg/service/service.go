package service

import (
	"context"
	"log/slog"

	"github.com/openguard/shared/models"
)

type Repository interface {
	List(ctx context.Context, orgID string) ([]*models.Connector, error)
	Create(ctx context.Context, c *models.Connector) error
	IngestEvents(ctx context.Context, orgID string, connectorID string, events []models.EventEnvelope) error
	GetByHash(ctx context.Context, hash string) (*models.Connector, error)
}

type Service struct {
	repo   Repository
	logger *slog.Logger
	isDev  bool
}

func New(repo Repository, logger *slog.Logger, isDev bool) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
		isDev:  isDev,
	}
}

func (s *Service) ListConnectors(ctx context.Context, orgID string) ([]*models.Connector, error) {
	return s.repo.List(ctx, orgID)
}

func (s *Service) CreateConnector(ctx context.Context, c *models.Connector) error {
	return s.repo.Create(ctx, c)
}

func (s *Service) IngestEvents(ctx context.Context, orgID string, connectorID string, events []models.EventEnvelope) error {
	return s.repo.IngestEvents(ctx, orgID, connectorID, events)
}
