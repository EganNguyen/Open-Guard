package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// Service handles business logic for the IAM service.
type Service struct {
	repo       *repository.Repository
	pool       *AuthWorkerPool
	keyring    []crypto.JWTKey
	aesKeyring []crypto.EncryptionKey
	rdb        *redis.Client
}

// NewService creates a new service instance.
func NewService(repo *repository.Repository, pool *AuthWorkerPool, keyring []crypto.JWTKey, aesKeyring []crypto.EncryptionKey, rdb *redis.Client) *Service {
	return &Service{
		repo:       repo,
		pool:       pool,
		keyring:    keyring,
		aesKeyring: aesKeyring,
		rdb:        rdb,
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

func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (map[string]interface{}, string, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}

	// 1. Check if locked
	if user["locked_until"] != nil {
		until := user["locked_until"].(*time.Time)
		if until != nil && time.Now().Before(*until) {
			return nil, "", fmt.Errorf("account is locked until %v", until.Format(time.RFC3339))
		}
	}

	// 2. Use worker pool for bcrypt comparison
	err = s.pool.Compare(ctx, password, user["password_hash"].(string))
	if err != nil {
		// Increment failures
		count, _ := s.repo.IncrementFailedLogin(ctx, email)
		if count >= 10 {
			// Lock for 15 minutes
			until := time.Now().Add(15 * time.Minute)
			_ = s.repo.LockAccount(ctx, email, until)
			return nil, "", fmt.Errorf("account locked due to too many failed attempts")
		}
		return nil, "", fmt.Errorf("invalid password")
	}

	// 3. Reset failures on success
	_ = s.repo.ResetFailedLogin(ctx, email)

	delete(user, "password_hash")
	delete(user, "failed_login_count")
	delete(user, "locked_until")

	// Issue JWT
	jti := uuid.New().String()
	ttl := 1 * time.Hour
	token, err := s.SignToken(user["org_id"].(string), user["id"].(string), jti, ttl)
	if err != nil {
		return nil, "", err
	}

	// Create session record
	err = s.repo.CreateSession(ctx, user["org_id"].(string), user["id"].(string), jti, userAgent, ip, time.Now().Add(ttl))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	return user, token, nil
}

func (s *Service) SignToken(orgID, userID, jti string, ttl time.Duration) (string, error) {
	claims := crypto.NewStandardClaims(orgID, userID, jti, ttl)
	return crypto.Sign(claims, s.keyring)
}

func (s *Service) Logout(ctx context.Context, jti string, expiresAt time.Time) error {
	if s.rdb == nil {
		return nil
	}
	
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}

	return s.rdb.Set(ctx, "blocklist:"+jti, "revoked", ttl).Err()
}

func (s *Service) GetCurrentUser(ctx context.Context, userID string) (map[string]interface{}, error) {
	return s.repo.GetUserByID(ctx, userID)
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

func (s *Service) GenerateTOTPSetup(ctx context.Context, userID, email string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "OpenGuard",
		AccountName: email,
	})
	if err != nil {
		return "", "", err
	}

	return key.Secret(), key.URL(), nil
}

func (s *Service) EnableTOTP(ctx context.Context, orgID, userID, code, secret string) error {
	// 1. Verify code
	if !totp.Validate(code, secret) {
		return fmt.Errorf("invalid totp code")
	}

	// 2. Encrypt secret
	encrypted, err := crypto.Encrypt([]byte(secret), s.aesKeyring)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	// 3. Store config
	if err := s.repo.UpsertMFAConfig(ctx, orgID, userID, "totp", encrypted); err != nil {
		return err
	}

	// 4. Enable MFA on user
	return s.repo.EnableUserMFA(ctx, userID, true, "totp")
}

func (s *Service) VerifyTOTP(ctx context.Context, userID, code string) (bool, error) {
	// 1. Get config
	config, err := s.repo.GetMFAConfig(ctx, userID, "totp")
	if err != nil {
		return false, err
	}

	// 2. Decrypt secret
	secretBytes, err := crypto.Decrypt(config["secret_encrypted"].(string), s.aesKeyring)
	if err != nil {
		return false, fmt.Errorf("decrypt secret: %w", err)
	}

	// 3. Validate code (with ±1 window per spec)
	return totp.Validate(code, string(secretBytes)), nil
}
