package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/go-webauthn/webauthn/protocol"
	"net/http"
)

type ScimPatchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// Repository defines the interface for data persistence.
type Repository interface {
	CreateOrg(ctx context.Context, name string) (string, error)
	CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role, status string) (string, error)
	GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error)
	GetUserByID(ctx context.Context, id string) (map[string]interface{}, error)
	IncrementFailedLogin(ctx context.Context, email string) (int, error)
	ResetFailedLogin(ctx context.Context, email string) error
	LockAccount(ctx context.Context, email string, until time.Time) error
	ListMFAConfigs(ctx context.Context, userID string) ([]map[string]interface{}, error)
	CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error
	CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (map[string]interface{}, error)
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
	UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secretEncrypted string) error
	EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error
	StoreBackupCodes(ctx context.Context, userID string, hashes []string) error
	ConsumeBackupCode(ctx context.Context, userID string, codeHash string) (bool, error)
	GetMFAConfig(ctx context.Context, userID, mfaType string) (map[string]interface{}, error)
	GetConnectorByID(ctx context.Context, id string) (map[string]interface{}, error)
	ListConnectors(ctx context.Context) ([]map[string]interface{}, error)
	CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error)
	UpdateConnector(ctx context.Context, id, name string, uris []string) error
	DeleteConnector(ctx context.Context, id string) error
	ListUsers(ctx context.Context, orgID string, filter string) ([]map[string]interface{}, error)
	BeginTx(ctx context.Context) (pgx.Tx, error)
	CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error
	UpdateUserStatus(ctx context.Context, userID, status string) error
	UpdateUserDisplayName(ctx context.Context, userID, displayName string) error
	UpdateUserSCIM(ctx context.Context, userID, externalID, status string) error
	GetActiveJTIs(ctx context.Context, userID string) ([]string, error)
	GetSessionTTL(ctx context.Context, jti string) time.Duration
	RevokeSessions(ctx context.Context, userID string) error
	GetUserByExternalID(ctx context.Context, orgID, externalID string) (map[string]interface{}, error)
	SaveWebAuthnCredential(ctx context.Context, orgID, userID string, cred map[string]interface{}) error
	ListWebAuthnCredentials(ctx context.Context, userID string) ([]map[string]interface{}, error)
}

// Service handles business logic for the IAM service.
type Service struct {
	repo         Repository
	pool         *AuthWorkerPool
	keyring      []crypto.JWTKey
	aesKeyring   []crypto.EncryptionKey
	rdb          *redis.Client
	redisBreaker *gobreaker.CircuitBreaker
	webauthn     *webauthn.WebAuthn
}

// NewService creates a new service instance.
// NewService creates a new service instance.
func NewService(repo Repository, pool *AuthWorkerPool, keyring []crypto.JWTKey, aesKeyring []crypto.EncryptionKey, rdb *redis.Client) *Service {
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

// SetWebAuthn initializes the WebAuthn instance.
func (s *Service) SetWebAuthn(w *webauthn.WebAuthn) {
	s.webauthn = w
}

// WebAuthnUser implements webauthn.User interface.
type WebAuthnUser struct {
	id          []byte
	displayName string
	name        string
	credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte                         { return u.id }
func (u *WebAuthnUser) WebAuthnName() string                       { return u.name }
func (u *WebAuthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *WebAuthnUser) WebAuthnIcon() string                       { return "" }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *Service) BeginWebAuthnRegistration(ctx context.Context, userID string) (*webauthn.SessionData, *protocol.CredentialCreation, error) {
	if s.webauthn == nil {
		return nil, nil, fmt.Errorf("webauthn not configured")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user["display_name"].(string),
		name:        user["email"].(string),
	}

	options, session, err := s.webauthn.BeginRegistration(wUser)
	if err != nil {
		return nil, nil, err
	}

	// Store session in Redis
	sessionJSON, _ := json.Marshal(session)
	if err := s.rdb.Set(ctx, "webauthn:reg:"+userID, sessionJSON, 5*time.Minute).Err(); err != nil {
		return nil, nil, err
	}

	return session, options, nil
}

func (s *Service) FinishWebAuthnRegistration(ctx context.Context, orgID, userID string, response *http.Request) error {
	if s.webauthn == nil {
		return fmt.Errorf("webauthn not configured")
	}

	val, err := s.rdb.Get(ctx, "webauthn:reg:"+userID).Result()
	if err != nil {
		return fmt.Errorf("registration session expired or invalid")
	}
	var session webauthn.SessionData
	json.Unmarshal([]byte(val), &session)

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user["display_name"].(string),
		name:        user["email"].(string),
	}

	credential, err := s.webauthn.FinishRegistration(wUser, session, response)
	if err != nil {
		return err
	}

	// Persist credential
	credMap := map[string]interface{}{
		"id":              hex.EncodeToString(credential.ID),
		"public_key":      hex.EncodeToString(credential.PublicKey),
		"attestation_type": credential.AttestationType,
		"sign_count":      credential.Authenticator.SignCount,
	}

	if err := s.repo.SaveWebAuthnCredential(ctx, orgID, userID, credMap); err != nil {
		return err
	}

	s.rdb.Del(ctx, "webauthn:reg:"+userID)
	return nil
}

func (s *Service) BeginWebAuthnLogin(ctx context.Context, email string) (*webauthn.SessionData, *protocol.CredentialAssertion, error) {
	if s.webauthn == nil {
		return nil, nil, fmt.Errorf("webauthn not configured")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, nil, err
	}

	userID := user["id"].(string)
	credentials, _ := s.repo.ListWebAuthnCredentials(ctx, userID)
	
	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user["display_name"].(string),
		name:        user["email"].(string),
	}

	for _, c := range credentials {
		id, _ := hex.DecodeString(c["credential_id"].(string))
		pubKey, _ := hex.DecodeString(c["public_key"].(string))
		wUser.credentials = append(wUser.credentials, webauthn.Credential{
			ID:        id,
			PublicKey: pubKey,
			Authenticator: webauthn.Authenticator{
				SignCount: uint32(c["sign_count"].(int32)),
			},
		})
	}

	options, session, err := s.webauthn.BeginLogin(wUser)
	if err != nil {
		return nil, nil, err
	}

	sessionJSON, _ := json.Marshal(session)
	if err := s.rdb.Set(ctx, "webauthn:login:"+userID, sessionJSON, 5*time.Minute).Err(); err != nil {
		return nil, nil, err
	}

	return session, options, nil
}

func (s *Service) FinishWebAuthnLogin(ctx context.Context, email string, userAgent, ip string, response *http.Request) (map[string]interface{}, string, error) {
	if s.webauthn == nil {
		return nil, "", fmt.Errorf("webauthn not configured")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}

	userID := user["id"].(string)
	val, err := s.rdb.Get(ctx, "webauthn:login:"+userID).Result()
	if err != nil {
		return nil, "", fmt.Errorf("login session expired or invalid")
	}
	var session webauthn.SessionData
	json.Unmarshal([]byte(val), &session)

	credentials, _ := s.repo.ListWebAuthnCredentials(ctx, userID)
	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user["display_name"].(string),
		name:        user["email"].(string),
	}
	for _, c := range credentials {
		id, _ := hex.DecodeString(c["credential_id"].(string))
		pubKey, _ := hex.DecodeString(c["public_key"].(string))
		wUser.credentials = append(wUser.credentials, webauthn.Credential{
			ID:        id,
			PublicKey: pubKey,
			Authenticator: webauthn.Authenticator{
				SignCount: uint32(c["sign_count"].(int32)),
			},
		})
	}

	_, err = s.webauthn.FinishLogin(wUser, session, response)
	if err != nil {
		return nil, "", err
	}

	s.rdb.Del(ctx, "webauthn:login:"+userID)

	// Issue tokens
	res, err := s.IssueTokens(ctx, user["org_id"].(string), userID, userAgent, ip, uuid.New())
	if err != nil {
		return nil, "", err
	}

	return user, res["access_token"].(string), nil
}

func (s *Service) RegisterOrg(ctx context.Context, name string) (string, error) {
	return s.repo.CreateOrg(ctx, name)
}

func (s *Service) RegisterUser(ctx context.Context, orgID, email, password, displayName, role string, scimExternalID string) (string, bool, error) {
	// 0. Idempotency check for SCIM (spec §2.5)
	if scimExternalID != "" {
		// Check for existing user by external ID (any status)
		user, err := s.repo.GetUserByExternalID(ctx, orgID, scimExternalID)
		if err == nil && user != nil {
			if user["status"].(string) == "deprovisioned" {
				return "", false, fmt.Errorf("CONFLICT:user was deprovisioned; create a new SCIM user or reprovision")
			}
			return user["id"].(string), false, nil
		}
	}

	// Hash password (R-02)
	hash, err := s.pool.Generate(ctx, password)
	if err != nil {
		return "", false, fmt.Errorf("hash password: %w", err)
	}

	// Use transaction for user creation + outbox event
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)

	// 1. Create with status='initializing'
	userID, err := s.repo.CreateUser(ctx, orgID, email, string(hash), displayName, role, "initializing")
	if err != nil {
		return "", false, err
	}

	// 1b. Update SCIM External ID if provided
	if scimExternalID != "" {
		if err := s.repo.UpdateUserSCIM(ctx, userID, scimExternalID, "initializing"); err != nil {
			return "", false, err
		}
	}

	// 2. Publish user.created to saga.orchestration (via outbox)
	event := map[string]interface{}{
		"event":   "user.created",
		"user_id": userID,
		"org_id":  orgID,
		"email":   email,
		"status":  "initializing",
		"ts":      time.Now().Unix(),
	}
	payload, _ := json.Marshal(event)
	if err := s.repo.CreateOutboxEvent(ctx, tx, orgID, "saga.orchestration", userID, payload); err != nil {
		return "", false, fmt.Errorf("outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}

	// 3. Saga timeout: write deadline to Redis (spec §2.5)
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

	// Publish retry event
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Reset status to initializing
	if err := s.repo.UpdateUserStatus(ctx, userID, "initializing"); err != nil {
		return err
	}

	event := map[string]interface{}{
		"event":   "user.reprovision",
		"user_id": userID,
		"org_id":  orgID,
		"email":   user["email"].(string),
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
	defer tx.Rollback(ctx)

	// a. Fetch all active JTIs for this user
	jtis, err := s.repo.GetActiveJTIs(ctx, userID)
	if err != nil {
		return err
	}

	// b. Pipeline: SETEX blocklist:<jti> <ttl> "revoked" for each
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

	// c. Revoke all sessions in DB
	if err := s.repo.RevokeSessions(ctx, userID); err != nil {
		return err
	}

	// d. Set user status to deprovisioned
	if err := s.repo.UpdateUserStatus(ctx, userID, "deprovisioned"); err != nil {
		return err
	}

	// e. Publish user.deleted via outbox
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

func (s *Service) PatchUser(ctx context.Context, id string, ops []ScimPatchOp) (map[string]interface{}, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, op := range ops {
		if op.Op != "replace" {
			continue // Only replace is supported for now
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

	// Publish mutation to saga.orchestration
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{
		"event":   "user.updated",
		"user_id": id,
		"org_id":  user["org_id"],
		"status":  user["status"],
		"ts":      time.Now().Unix(),
	})
	if err := s.repo.CreateOutboxEvent(ctx, tx, user["org_id"].(string), "saga.orchestration", id, payload); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (map[string]interface{}, string, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}

	// 1. Check if locked or initializing
	if user["status"].(string) == "initializing" {
		return nil, "", fmt.Errorf("USER_PROVISIONING_IN_PROGRESS")
	}

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

func (s *Service) VerifyBackupCodeAndLogin(ctx context.Context, challengeToken, code, userAgent, ip string) (map[string]interface{}, string, error) {
	// 1. Get userID from challenge
	res, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return s.rdb.Get(ctx, "mfa_challenge:"+challengeToken).Result()
	})
	if err != nil {
		return nil, "", fmt.Errorf("invalid or expired challenge")
	}
	userID := res.(string)

	// 2. Verify and consume backup code
	ok, err := s.VerifyBackupCode(ctx, userID, code)
	if err != nil || !ok {
		return nil, "", fmt.Errorf("invalid backup code")
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

func (s *Service) UpdateUserStatus(ctx context.Context, userID, status string) error {
	return s.repo.UpdateUserStatus(ctx, userID, status)
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

func (s *Service) EnableTOTP(ctx context.Context, orgID, userID, code, secret string) ([]string, error) {
	// 1. Verify code
	if !totp.Validate(code, secret) {
		return nil, fmt.Errorf("invalid totp code")
	}

	// 2. Encrypt secret
	encrypted, err := crypto.Encrypt([]byte(secret), s.aesKeyring)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}

	// 3. Store config
	if err := s.repo.UpsertMFAConfig(ctx, orgID, userID, "totp", encrypted); err != nil {
		return nil, err
	}

	// 4. Enable MFA on user
	if err := s.repo.EnableUserMFA(ctx, userID, true, "totp"); err != nil {
		return nil, err
	}

	// 5. Generate backup codes per spec
	return s.GenerateBackupCodes(ctx, orgID, userID)
}

// GenerateBackupCodes generates 8 single-use 8-character backup codes.
// Returns plaintext codes (shown to user once) and stores HMAC hashes.
func (s *Service) GenerateBackupCodes(ctx context.Context, orgID, userID string) ([]string, error) {
	secret := os.Getenv("IAM_MFA_BACKUP_CODE_HMAC_SECRET")
	if secret == "" {
		secret = "dev-backup-code-secret-fixed-value-for-dev"
	}
	codes := make([]string, 8)
	hashes := make([]string, 8)
	for i := range codes {
		raw := crypto.GenerateRandomString(8)
		codes[i] = raw
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(raw))
		hashes[i] = hex.EncodeToString(mac.Sum(nil))
	}
	if err := s.repo.StoreBackupCodes(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// VerifyBackupCode verifies and consumes a single backup code.
func (s *Service) VerifyBackupCode(ctx context.Context, userID, code string) (bool, error) {
	secret := os.Getenv("IAM_MFA_BACKUP_CODE_HMAC_SECRET")
	if secret == "" {
		secret = "dev-backup-code-secret-fixed-value-for-dev"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(code))
	codeHash := hex.EncodeToString(mac.Sum(nil))
	return s.repo.ConsumeBackupCode(ctx, userID, codeHash)
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
