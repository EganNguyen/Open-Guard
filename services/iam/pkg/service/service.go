package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

// Service handles business logic for the IAM service.
type Service struct {
	repo         *repository.Repository
	pool         *AuthWorkerPool
	keyring      []crypto.JWTKey
	aesKeyring   []crypto.EncryptionKey
	rdb          *redis.Client
	redisBreaker *gobreaker.CircuitBreaker
}

// NewService creates a new service instance.
func NewService(repo *repository.Repository, pool *AuthWorkerPool, keyring []crypto.JWTKey, aesKeyring []crypto.EncryptionKey, rdb *redis.Client) *Service {
	return &Service{
		repo:       repo,
		pool:       pool,
		keyring:    keyring,
		aesKeyring: aesKeyring,
		rdb:        rdb,
		redisBreaker: resilience.NewBreaker(resilience.BreakerConfig{
			Name:             "iam-redis",
			MaxRequests:      3,
			Interval:         5 * time.Second,
			FailureThreshold: 5,
			OpenDuration:     10 * time.Second,
		}, slog.Default()),
	}
}

func (s *Service) RegisterOrg(ctx context.Context, name string) (string, error) {
	return s.repo.CreateOrg(ctx, name)
}

func (s *Service) RegisterUser(ctx context.Context, orgID, email, password, displayName, role string) (string, error) {
	// Hash password (R-02)
	hash, err := s.pool.Generate(ctx, password)
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

	// 4. Check MFA requirements (R-11)
	mfaConfigs, _ := s.repo.ListMFAConfigs(ctx, user["id"].(string))
	if len(mfaConfigs) > 0 {
		// MFA required. Return 202 status in handler, but here we return a flag.
		// Issue a short-lived MFA challenge token in Redis
		challengeToken := uuid.New().String()
		_, _ = resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
			return nil, s.rdb.Set(ctx, "mfa_challenge:"+challengeToken, user["id"].(string), 5*time.Minute).Err()
		})
		
		return map[string]interface{}{
			"mfa_required":    true,
			"mfa_challenge": challengeToken,
			"user_id":         user["id"].(string),
		}, "", nil
	}

	delete(user, "password_hash")
	delete(user, "failed_login_count")
	delete(user, "locked_until")

	// Issue full tokens
	res, err := s.IssueTokens(ctx, user["org_id"].(string), user["id"].(string), userAgent, ip, uuid.New())
	if err != nil {
		return nil, "", err
	}

	return user, res["access_token"].(string), nil
}

func (s *Service) IssueTokens(ctx context.Context, orgID, userID, userAgent, ip string, familyID uuid.UUID) (map[string]interface{}, error) {
	// Access Token
	jti := uuid.New().String()
	ttl := 1 * time.Hour
	accessToken, err := s.SignToken(orgID, userID, jti, ttl)
	if err != nil {
		return nil, err
	}

	// Create session record
	err = s.repo.CreateSession(ctx, orgID, userID, jti, userAgent, ip, time.Now().Add(ttl))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Refresh Token (R-10)
	refreshToken := crypto.GenerateRandomString(64)
	rtHash := crypto.HashSHA256(refreshToken)
	rtTTL := 7 * 24 * time.Hour

	err = s.repo.CreateRefreshToken(ctx, orgID, userID, rtHash, familyID, time.Now().Add(rtTTL))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	return map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int(ttl.Seconds()),
	}, nil
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken, userAgent, ip string) (map[string]interface{}, error) {
	rtHash := crypto.HashSHA256(refreshToken)
	rt, err := s.repo.GetRefreshToken(ctx, rtHash)
	if err != nil {
		return nil, err
	}

	// Check revocation or expiry
	if rt["revoked"].(bool) || time.Now().After(rt["expires_at"].(time.Time)) {
		// Potential reuse attack or expired! Revoke the whole family (R-10)
		s.repo.RevokeRefreshTokenFamily(ctx, rt["family_id"].(uuid.UUID))
		return nil, fmt.Errorf("token revoked or expired")
	}

	// Rotate token: delete old, issue new (R-10)
	s.repo.DeleteRefreshToken(ctx, rtHash)

	return s.IssueTokens(ctx, rt["org_id"].(string), rt["user_id"].(string), userAgent, ip, rt["family_id"].(uuid.UUID))
}

func (s *Service) VerifyMFAAndLogin(ctx context.Context, challengeToken, code, userAgent, ip string) (map[string]interface{}, string, error) {
	// 1. Get userID from challenge
	res, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return s.rdb.Get(ctx, "mfa_challenge:"+challengeToken).Result()
	})
	if err != nil {
		return nil, "", fmt.Errorf("invalid or expired challenge")
	}
	userID := res.(string)

	// 2. Verify TOTP
	ok, err := s.VerifyTOTP(ctx, userID, code)
	if err != nil || !ok {
		return nil, "", fmt.Errorf("invalid mfa code")
	}

	// 3. Clear challenge
	_, _ = resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return nil, s.rdb.Del(ctx, "mfa_challenge:"+challengeToken).Err()
	})

	// 4. Get user and issue tokens
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}

	resToken, err := s.IssueTokens(ctx, user["org_id"].(string), user["id"].(string), userAgent, ip, uuid.New())
	if err != nil {
		return nil, "", err
	}

	return user, resToken["access_token"].(string), nil
}

func (s *Service) SignToken(orgID, userID, jti string, ttl time.Duration) (string, error) {
	claims := crypto.NewStandardClaims(orgID, userID, jti, ttl)
	return crypto.Sign(claims, s.keyring)
}

func (s *Service) isRevoked(ctx context.Context, jti string) bool {
	if s.rdb == nil {
		return false
	}
	
	val, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		res, err := s.rdb.Exists(ctx, "blocklist:"+jti).Result()
		return res > 0, err
	})
	if err != nil {
		return false
	}
	return val.(bool)
}

func (s *Service) Logout(ctx context.Context, jti string, expiresAt time.Time) error {
	if s.rdb == nil {
		return nil
	}
	
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}

	_, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return nil, s.rdb.Set(ctx, "blocklist:"+jti, "revoked", ttl).Err()
	})
	return err
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
func (s *Service) ListUsers(ctx context.Context, orgID string, filter string) ([]map[string]interface{}, error) {
	return s.repo.ListUsers(ctx, orgID, filter)
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

// VerifyTOTP validates a TOTP code against the stored config.
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

func (s *Service) StoreAuthCode(ctx context.Context, code, orgID, userID string) error {
	if s.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	data := fmt.Sprintf("%s:%s", orgID, userID)
	return s.rdb.Set(ctx, "auth_code:"+code, data, 10*time.Minute).Err()
}

func (s *Service) GetAuthCode(ctx context.Context, code string) (string, string, error) {
	if s.rdb == nil {
		return "", "", fmt.Errorf("redis not configured")
	}
	val, err := s.rdb.Get(ctx, "auth_code:"+code).Result()
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(val, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth code data")
	}
	// Delete code after use (one-time use)
	s.rdb.Del(ctx, "auth_code:"+code)
	return parts[0], parts[1], nil
}
