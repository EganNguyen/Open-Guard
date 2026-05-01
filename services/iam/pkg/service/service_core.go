package service

import (
	"errors"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

// @AI-INTENT: [Pattern: Heuristic Risk Scoring for Session Revocation]
const (
	riskScoreUAFamilyChange  = 60
	riskScoreIPSubnetChange  = 40
	riskScoreIPHostChange    = 15
	riskScoreUAVersionChange = 20
	riskThresholdRevoke      = 80
)

var (
	ErrSessionRevokedRisk = errors.New("SESSION_REVOKED_RISK")
	ErrSessionCompromised = errors.New("SESSION_COMPROMISED")
	ErrInvalidCredentials = errors.New("INVALID_CREDENTIALS")
	ErrAccountSetup       = errors.New("ACCOUNT_SETUP_PENDING")
	ErrAccountLocked      = errors.New("ACCOUNT_LOCKED")
)

// RegisterUserRequest contains all fields needed to create a new user.
type RegisterUserRequest struct {
	OrgID          string
	Email          string
	Password       string
	DisplayName    string
	Role           string
	SCIMExternalID string
}

// IssueTokensRequest contains all fields needed to issue a new token pair.
type IssueTokensRequest struct {
	OrgID     string
	UserID    string
	UserAgent string
	IPAddress string
	FamilyID  uuid.UUID
}

// TokenResponse is the typed response for token issuance.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Service handles business logic for the IAM service.
type Service struct {
	userRepo      UserRepository
	sessionRepo   SessionRepository
	tokenRepo     TokenRepository
	mfaRepo       MFARepository
	connectorRepo ConnectorRepository
	webauthnRepo  WebAuthnRepository
	samlRepo      SAMLRepository
	orgRepo       OrgRepository
	outboxRepo    OutboxRepository
	pool          *AuthWorkerPool
	keyring       []crypto.JWTKey
	aesKeyring    []crypto.EncryptionKey
	rdb           *redis.Client
	redisBreaker  *gobreaker.CircuitBreaker
	webauthn      *webauthn.WebAuthn
}

// NewService creates a new service instance.
func NewService(
	userRepo UserRepository,
	sessionRepo SessionRepository,
	tokenRepo TokenRepository,
	mfaRepo MFARepository,
	connectorRepo ConnectorRepository,
	webauthnRepo WebAuthnRepository,
	samlRepo SAMLRepository,
	orgRepo OrgRepository,
	outboxRepo OutboxRepository,
	pool *AuthWorkerPool,
	keyring []crypto.JWTKey,
	aesKeyring []crypto.EncryptionKey,
	rdb *redis.Client,
) *Service {
	return &Service{
		userRepo:      userRepo,
		sessionRepo:   sessionRepo,
		tokenRepo:     tokenRepo,
		mfaRepo:       mfaRepo,
		connectorRepo: connectorRepo,
		webauthnRepo:  webauthnRepo,
		samlRepo:      samlRepo,
		orgRepo:       orgRepo,
		outboxRepo:    outboxRepo,
		pool:          pool,
		keyring:       keyring,
		aesKeyring:    aesKeyring,
		rdb:           rdb,
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

// Redis returns the underlying redis client.
func (s *Service) Redis() *redis.Client {
	return s.rdb
}

// GetKeyring returns the JWT keyring.
func (s *Service) GetKeyring() []crypto.JWTKey {
	return s.keyring
}
