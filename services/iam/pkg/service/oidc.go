package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/iam/pkg/repository"
)

func (s *Service) CreateAuthCode(ctx context.Context, userID, orgID, clientID, redirectURI, scope, state string) (string, error) {
	code := uuid.New().String()
	expiresAt := time.Now().Add(10 * time.Minute) // 10 minutes expiry

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	err = s.repo.CreateAuthCode(ctx, tx, &repository.AuthCode{
		Code:        code,
		UserID:      userID,
		OrgID:       orgID,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scope:       scope,
		State:       state,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return code, nil
}

func (s *Service) ExchangeAuthCode(ctx context.Context, codeStr, clientID string) (*LoginResponse, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	code, err := s.repo.GetAuthCodeByCode(ctx, tx, codeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired authorization code")
	}

	if code.ClientID != clientID {
		return nil, fmt.Errorf("client_id mismatch")
	}

	user, err := s.repo.GetUserByID(ctx, tx, code.OrgID, code.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	token, err := s.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	refreshToken := uuid.New().String()
	hashSum := sha256.Sum256([]byte(refreshToken))
	refreshHash := hex.EncodeToString(hashSum[:])

	sessionExpiresAt := time.Now().Add(s.sessionIdleTimeout)
	_, err = s.repo.CreateSession(ctx, tx, user.ID, user.OrgID, refreshHash, nil, nil, nil, sessionExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Consume the code
	if err := s.repo.DeleteAuthCode(ctx, tx, code.ID); err != nil {
		s.logger.Error("failed to delete consumed auth code", "error", err, "code_id", code.ID)
	}

	org, _ := s.repo.GetOrgByID(ctx, tx, user.OrgID)

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &LoginResponse{
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.jwtExpiry.Seconds()),
		User:         user,
		Org:          org,
	}, nil
}
