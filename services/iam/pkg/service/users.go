package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	iam_repo "github.com/openguard/services/iam/pkg/repository"
	"github.com/redis/go-redis/v9"
)

type ScimPatchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func (s *Service) RegisterOrg(ctx context.Context, name string) (string, error) {
	return s.repo.CreateOrg(ctx, name)
}

func (s *Service) RegisterUser(ctx context.Context, req RegisterUserRequest) (string, bool, error) {
	if req.SCIMExternalID != "" {
		user, err := s.repo.GetUserByExternalID(ctx, req.OrgID, req.SCIMExternalID)
		if err == nil && user != nil {
			if user.Status == "deprovisioned" {
				return "", false, fmt.Errorf("CONFLICT:user was deprovisioned; create a new SCIM user or reprovision")
			}
			return user.ID, false, nil
		}
	}

	hash, err := s.pool.Generate(ctx, req.Password)
	if err != nil {
		return "", false, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID, err := s.repo.CreateUser(ctx, req.OrgID, req.Email, string(hash), req.DisplayName, req.Role, "initializing")
	if err != nil {
		return "", false, err
	}

	if req.SCIMExternalID != "" {
		if err := s.repo.UpdateUserSCIM(ctx, userID, req.SCIMExternalID, "initializing"); err != nil {
			return "", false, err
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"event":   "user.created",
		"user_id": userID,
		"org_id":  req.OrgID,
		"email":   req.Email,
		"status":  "initializing",
		"ts":      time.Now().Unix(),
	})
	if err := s.repo.CreateOutboxEvent(ctx, tx, req.OrgID, "saga.orchestration", userID, payload); err != nil {
		return "", false, fmt.Errorf("outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}

	if s.rdb != nil {
		deadline := time.Now().Add(40 * time.Second).Unix()
		s.rdb.ZAdd(ctx, "saga:deadlines", redis.Z{
			Score:  float64(deadline),
			Member: userID,
		})
	}

	return userID, true, nil
}

func (s *Service) ReprovisionUser(ctx context.Context, orgID, userID string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.UpdateUserStatus(ctx, userID, "initializing"); err != nil {
		return err
	}

	event := map[string]interface{}{
		"event":   "user.reprovision",
		"user_id": userID,
		"org_id":  orgID,
		"email":   user.Email,
		"status":  "initializing",
		"ts":      time.Now().Unix(),
	}
	payload, _ := json.Marshal(event)
	if err := s.repo.CreateOutboxEvent(ctx, tx, orgID, "saga.orchestration", userID, payload); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	jtis, err := s.repo.GetActiveJTIs(ctx, userID)
	if err != nil {
		return err
	}

	if s.rdb != nil {
		pipe := s.rdb.Pipeline()
		for _, jti := range jtis {
			ttl := s.repo.GetSessionTTL(ctx, jti)
			if ttl > 0 {
				pipe.SetEx(ctx, "blocklist:"+jti, "revoked", ttl)
			}
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}

	if err := s.repo.RevokeSessions(ctx, userID); err != nil {
		return err
	}

	if err := s.repo.UpdateUserStatus(ctx, userID, "deprovisioned"); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"event":   "user.deleted",
		"user_id": userID,
		"status":  "deprovisioned",
		"ts":      time.Now().Unix(),
	})
	if err := s.repo.CreateOutboxEvent(ctx, tx, "", "saga.orchestration", userID, payload); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Service) PatchUser(ctx context.Context, id string, ops []ScimPatchOp) (*iam_repo.User, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, op := range ops {
		if op.Op != "replace" {
			continue
		}
		switch op.Path {
		case "active":
			var active bool
			if err := json.Unmarshal(op.Value, &active); err != nil {
				return nil, fmt.Errorf("invalid active value: %w", err)
			}
			status := "active"
			if !active {
				status = "suspended"
			}
			if err := s.repo.UpdateUserStatus(ctx, id, status); err != nil {
				return nil, err
			}
		case "displayName":
			var displayName string
			if err := json.Unmarshal(op.Value, &displayName); err != nil {
				return nil, fmt.Errorf("invalid displayName value: %w", err)
			}
			if err := s.repo.UpdateUserDisplayName(ctx, id, displayName); err != nil {
				return nil, err
			}
		}
	}

	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{
		"event":   "user.updated",
		"user_id": id,
		"org_id":  user.OrgID,
		"status":  user.Status,
		"ts":      time.Now().Unix(),
	})
	if err := s.repo.CreateOutboxEvent(ctx, tx, user.OrgID, "saga.orchestration", id, payload); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) UpdateUserStatus(ctx context.Context, userID, status string) error {
	return s.repo.UpdateUserStatus(ctx, userID, status)
}

func (s *Service) GetCurrentUser(ctx context.Context, userID string) (*iam_repo.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

func (s *Service) ListUsers(ctx context.Context, orgID string, filter string) ([]iam_repo.User, error) {
	return s.repo.ListUsers(ctx, orgID, filter)
}

func (s *Service) ListUsersPaginated(ctx context.Context, orgID string, filter string, offset, limit int) ([]iam_repo.User, int, error) {
	return s.repo.ListUsersPaginated(ctx, orgID, filter, offset, limit)
}

func (s *Service) GetConnector(ctx context.Context, id string) (*iam_repo.Connector, error) {
	return s.repo.GetConnectorByID(ctx, id)
}
func (s *Service) ListConnectors(ctx context.Context) ([]iam_repo.Connector, error) {
	return s.repo.ListConnectors(ctx)
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

func (s *Service) OffboardOrg(ctx context.Context, orgID string) error {
	users, err := s.repo.ListUsers(ctx, orgID, "")
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	for _, u := range users {
		userID := u.ID
		jtis, err := s.repo.GetActiveJTIs(ctx, userID)
		if err == nil && s.rdb != nil {
			pipe := s.rdb.Pipeline()
			for _, jti := range jtis {
				ttl := s.repo.GetSessionTTL(ctx, jti)
				if ttl > 0 {
					pipe.SetEx(ctx, "blocklist:"+jti, "revoked", ttl)
				}
			}
			_, _ = pipe.Exec(ctx)
		}
		_ = s.repo.RevokeSessions(ctx, userID)
	}

	if err := s.repo.DeprovisionAllUsers(ctx, orgID); err != nil {
		return fmt.Errorf("deprovision all users: %w", err)
	}

	event := map[string]interface{}{
		"event":  "org.iam.offboarded",
		"org_id": orgID,
		"status": "completed",
		"ts":     time.Now().Unix(),
	}
	payload, _ := json.Marshal(event)

	tx, err := s.repo.BeginTx(ctx)
	if err == nil {
		defer func() { _ = tx.Rollback(ctx) }()
		if err := s.repo.CreateOutboxEvent(ctx, tx, orgID, "saga.orchestration", orgID, payload); err == nil {
			_ = tx.Commit(ctx)
		}
	}
	return nil
}
