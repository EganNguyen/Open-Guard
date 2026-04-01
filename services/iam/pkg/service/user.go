package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) ListUsers(ctx context.Context, orgID string, page, perPage int) ([]*repository.User, int, error) {
	if page < 1 { page = 1 }
	if perPage < 1 || perPage > 100 { perPage = 50 }

	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, 0, err }
	defer tx.Rollback(ctx)

	return s.repo.ListUsersByOrg(ctx, tx, orgID, page, perPage)
}

func (s *Service) GetUser(ctx context.Context, orgID, id string) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	return s.repo.GetUserByID(ctx, tx, orgID, id)
}

type CreateUserRequest struct {
	OrgID       string `json:"org_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"` // optional initial password
}

func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest) (*repository.User, error) {
	if req.Email == "" { return nil, fmt.Errorf("email is required") }

	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	// Optionally hash a provided initial password
	var passwordHash *string
	if req.Password != "" {
		if len(req.Password) < 8 {
			return nil, fmt.Errorf("password must be at least 8 characters")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 4) // bcrypt.MinCost
		if err != nil { return nil, fmt.Errorf("hash password: %w", err) }
		hashStr := string(hash)
		passwordHash = &hashStr
	}

	user, err := s.repo.CreateUser(ctx, tx, req.OrgID, req.Email, req.DisplayName, passwordHash)
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.created", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

func (s *Service) UpdateUser(ctx context.Context, orgID, id string, req UpdateUserRequest) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.repo.UpdateUserStatus(ctx, tx, orgID, id, req.Status)
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.updated", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

func (s *Service) DeleteUser(ctx context.Context, orgID, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if err := s.repo.SoftDeleteUser(ctx, tx, orgID, id); err != nil { return err }

	s.publishAuditEvent(ctx, tx, "user.deleted", orgID, id)

	return tx.Commit(ctx)
}

func (s *Service) SuspendUser(ctx context.Context, orgID, id string) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.repo.UpdateUserStatus(ctx, tx, orgID, id, string(models.UserStatusSuspended))
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.suspended", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

func (s *Service) ActivateUser(ctx context.Context, orgID, id string) (*repository.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)

	user, err := s.repo.UpdateUserStatus(ctx, tx, orgID, id, string(models.UserStatusActive))
	if err != nil { return nil, err }

	s.publishAuditEvent(ctx, tx, "user.updated", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil { return nil, err }
	return user, nil
}

func (s *Service) ListAPITokens(ctx context.Context, orgID, userID string) ([]*repository.APIToken, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	return s.repo.ListAPITokensByUser(ctx, tx, orgID, userID)
}

func (s *Service) CreateAPIToken(ctx context.Context, orgID, userID, name string, scopes []string, expiresAt *time.Time) (*repository.APIToken, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, "", err }
	defer tx.Rollback(ctx)

	if scopes == nil {
		scopes = []string{}
	}

	rawToken := "og_pat_" + uuid.New().String()
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])
	prefix := rawToken[:10]

	token, err := s.repo.CreateAPIToken(ctx, tx, userID, orgID, name, tokenHash, prefix, scopes, expiresAt)
	if err != nil { return nil, "", err }

	s.publishAuditEvent(ctx, tx, "api_token.created", orgID, userID)

	if err := tx.Commit(ctx); err != nil { return nil, "", err }
	return token, rawToken, nil
}

func (s *Service) RevokeAPIToken(ctx context.Context, orgID, tokenID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if err := s.repo.RevokeAPIToken(ctx, tx, orgID, tokenID); err != nil { return err }
	return tx.Commit(ctx)
}

func (s *Service) ListSessions(ctx context.Context, orgID, userID string) ([]*repository.Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	return s.repo.ListSessionsByUser(ctx, tx, orgID, userID)
}

func (s *Service) RevokeSession(ctx context.Context, orgID, sessionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	if err := s.repo.RevokeSession(ctx, tx, orgID, sessionID); err != nil { return err }
	return tx.Commit(ctx)
}

func (s *Service) publishAuditEvent(ctx context.Context, tx pgx.Tx, eventType, orgID, actorID string) {
	s.publishEvent(ctx, tx, kafka.TopicAuditTrail, eventType, orgID, actorID)
}
