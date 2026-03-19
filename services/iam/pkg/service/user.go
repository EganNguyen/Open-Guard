package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
	"github.com/openguard/shared/outbox"
)

type UserService struct {
	pool     *pgxpool.Pool
	users    *repository.UserRepository
	sessions *repository.SessionRepository
	tokens   *repository.APITokenRepository
	outbox   *outbox.Writer
	logger   *slog.Logger
}

func NewUserService(
	pool *pgxpool.Pool,
	users *repository.UserRepository,
	sessions *repository.SessionRepository,
	tokens *repository.APITokenRepository,
	outbox *outbox.Writer,
	logger *slog.Logger,
) *UserService {
	return &UserService{
		pool:     pool,
		users:    users,
		sessions: sessions,
		tokens:   tokens,
		outbox:   outbox,
		logger:   logger,
	}
}

func (s *UserService) ListUsers(ctx context.Context, orgID string, page, perPage int) ([]*repository.User, int, error) {
	if page < 1 { page = 1 }
	if perPage < 1 || perPage > 100 { perPage = 50 }

	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, 0, err }
	defer tx.Rollback(ctx)

	return s.users.ListByOrg(ctx, tx, orgID, page, perPage)
}

func (s *UserService) GetUser(ctx context.Context, orgID, id string) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	return s.users.GetByID(ctx, tx, orgID, id)
}

type CreateUserRequest struct {
	OrgID       string `json:"org_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*repository.User, error) {
	if req.Email == "" { return nil, fmt.Errorf("email is required") }

	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.users.Create(ctx, tx, req.OrgID, req.Email, req.DisplayName, nil)
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.created", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

func (s *UserService) UpdateUser(ctx context.Context, orgID, id string, req UpdateUserRequest) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.users.UpdateStatus(ctx, tx, orgID, id, req.Status)
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.updated", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

func (s *UserService) DeleteUser(ctx context.Context, orgID, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if err := s.users.SoftDelete(ctx, tx, orgID, id); err != nil { return err }

	s.publishAuditEvent(ctx, tx, "user.deleted", orgID, id)

	return tx.Commit(ctx)
}

func (s *UserService) SuspendUser(ctx context.Context, orgID, id string) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.users.UpdateStatus(ctx, tx, orgID, id, string(models.UserStatusSuspended))
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.suspended", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

func (s *UserService) ActivateUser(ctx context.Context, orgID, id string) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.users.UpdateStatus(ctx, tx, orgID, id, string(models.UserStatusActive))
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.updated", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

func (s *UserService) ListAPITokens(ctx context.Context, orgID, userID string) ([]*repository.APIToken, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	return s.tokens.ListByUser(ctx, tx, orgID, userID)
}

func (s *UserService) RevokeAPIToken(ctx context.Context, orgID, tokenID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if err := s.tokens.Revoke(ctx, tx, orgID, tokenID); err != nil { return err }
	return tx.Commit(ctx)
}

func (s *UserService) ListSessions(ctx context.Context, orgID, userID string) ([]*repository.Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	return s.sessions.ListByUser(ctx, tx, orgID, userID)
}

func (s *UserService) RevokeSession(ctx context.Context, orgID, sessionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if err := s.sessions.Revoke(ctx, tx, orgID, sessionID); err != nil { return err }
	return tx.Commit(ctx)
}

func (s *UserService) publishAuditEvent(ctx context.Context, tx pgx.Tx, eventType, orgID, actorID string) {
	if s.outbox == nil { return }
	envelope := models.EventEnvelope{
		ID:        uuid.New().String(),
		Type:      eventType,
		OrgID:     orgID,
		ActorID:   actorID,
		ActorType: "user",
		Source:    "iam",
		SchemaVer: "2.0",
		Payload:   []byte(`{}`),
	}
	s.outbox.Write(ctx, tx, kafka.TopicAuditTrail, actorID, envelope)
}
