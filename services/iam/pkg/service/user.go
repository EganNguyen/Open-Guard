package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
)

// UserService handles user management operations.
type UserService struct {
	users    *repository.UserRepository
	sessions *repository.SessionRepository
	tokens   *repository.APITokenRepository
	producer *kafka.Producer
	logger   *slog.Logger
}

// NewUserService creates a new UserService.
func NewUserService(
	users *repository.UserRepository,
	sessions *repository.SessionRepository,
	tokens *repository.APITokenRepository,
	producer *kafka.Producer,
	logger *slog.Logger,
) *UserService {
	return &UserService{
		users:    users,
		sessions: sessions,
		tokens:   tokens,
		producer: producer,
		logger:   logger,
	}
}

// ListUsers returns paginated users for an org.
func (s *UserService) ListUsers(ctx context.Context, orgID string, page, perPage int) ([]*repository.User, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	return s.users.ListByOrg(ctx, orgID, page, perPage)
}

// GetUser returns a single user.
func (s *UserService) GetUser(ctx context.Context, id string) (*repository.User, error) {
	return s.users.GetByID(ctx, id)
}

// CreateUserRequest is the input for creating a user.
type CreateUserRequest struct {
	OrgID       string `json:"org_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// CreateUser creates a new user in an org (without password — SSO or invite-based).
func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) (*repository.User, error) {
	if req.Email == "" {
		return nil, fmt.Errorf("email is required")
	}
	user, err := s.users.Create(ctx, req.OrgID, req.Email, req.DisplayName, nil)
	if err != nil {
		return nil, err
	}
	s.publishAuditEvent(ctx, "user.created", user.OrgID, user.ID)
	return user, nil
}

// UpdateUserRequest is the input for updating a user.
type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

// UpdateUser updates a user's display name and/or status.
func (s *UserService) UpdateUser(ctx context.Context, id string, req UpdateUserRequest) (*repository.User, error) {
	user, err := s.users.Update(ctx, id, req.DisplayName, req.Status)
	if err != nil {
		return nil, err
	}
	s.publishAuditEvent(ctx, "user.updated", user.OrgID, user.ID)
	return user, nil
}

// DeleteUser soft-deletes a user.
func (s *UserService) DeleteUser(ctx context.Context, id, orgID string) error {
	if err := s.users.SoftDelete(ctx, id); err != nil {
		return err
	}
	s.publishAuditEvent(ctx, "user.deleted", orgID, id)
	return nil
}

// SuspendUser sets a user's status to suspended.
func (s *UserService) SuspendUser(ctx context.Context, id string) (*repository.User, error) {
	user, err := s.users.UpdateStatus(ctx, id, string(models.UserStatusSuspended))
	if err != nil {
		return nil, err
	}
	s.publishAuditEvent(ctx, "user.suspended", user.OrgID, user.ID)
	return user, nil
}

// ActivateUser sets a user's status to active.
func (s *UserService) ActivateUser(ctx context.Context, id string) (*repository.User, error) {
	user, err := s.users.UpdateStatus(ctx, id, string(models.UserStatusActive))
	if err != nil {
		return nil, err
	}
	s.publishAuditEvent(ctx, "user.updated", user.OrgID, user.ID)
	return user, nil
}

// ListSessions returns active sessions for a user.
func (s *UserService) ListSessions(ctx context.Context, userID string) ([]*repository.Session, error) {
	return s.sessions.ListByUser(ctx, userID)
}

// RevokeSession revokes a specific session.
func (s *UserService) RevokeSession(ctx context.Context, sessionID string) error {
	return s.sessions.Revoke(ctx, sessionID)
}

// ListAPITokens returns all API tokens for a user.
func (s *UserService) ListAPITokens(ctx context.Context, userID string) ([]*repository.APIToken, error) {
	return s.tokens.ListByUser(ctx, userID)
}

// RevokeAPIToken revokes an API token.
func (s *UserService) RevokeAPIToken(ctx context.Context, tokenID string) error {
	return s.tokens.Revoke(ctx, tokenID)
}

func (s *UserService) publishAuditEvent(ctx context.Context, eventType, orgID, actorID string) {
	if s.producer == nil {
		return
	}
	envelope := models.EventEnvelope{
		Type:      eventType,
		OrgID:     orgID,
		ActorID:   actorID,
		ActorType: "user",
		Source:    "iam",
		SchemaVer: "1.0",
		Payload:   []byte(`{}`),
	}
	if err := s.producer.PublishEvent(ctx, kafka.TopicAuditTrail, envelope); err != nil {
		s.logger.Error("failed to publish audit event", "event_type", eventType, "error", err)
	}
}
