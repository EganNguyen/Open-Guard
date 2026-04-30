package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
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

// UserStore defines operations for user management.
type UserStore interface {
	CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role, status string) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*iam_repo.User, error)
	GetUserByID(ctx context.Context, id string) (*iam_repo.User, error)
	GetUserByExternalID(ctx context.Context, orgID, externalID string) (*iam_repo.User, error)
	ListUsers(ctx context.Context, orgID string, filter string) ([]iam_repo.User, error)
	ListUsersPaginated(ctx context.Context, orgID string, filter string, offset, limit int) ([]iam_repo.User, int, error)
	UpdateUserStatus(ctx context.Context, userID, status string) error
	UpdateUserDisplayName(ctx context.Context, userID, displayName string) error
	UpdateUserSCIM(ctx context.Context, userID, externalID, status string) error
	IncrementFailedLogin(ctx context.Context, email string) (int, error)
	ResetFailedLogin(ctx context.Context, email string) error
	LockAccount(ctx context.Context, email string, until time.Time) error
	DeprovisionAllUsers(ctx context.Context, orgID string) error
}

// SessionStore defines operations for session management.
type SessionStore interface {
	CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error
	GetSessionByUserID(ctx context.Context, userID string) (*iam_repo.Session, error)
	GetActiveJTIs(ctx context.Context, userID string) ([]string, error)
	GetSessionTTL(ctx context.Context, jti string) time.Duration
	RevokeSessions(ctx context.Context, userID string) error
}

// TokenStore defines operations for refresh token management.
type TokenStore interface {
	CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*iam_repo.RefreshToken, error)
	ClaimRefreshToken(ctx context.Context, tokenHash string) (*iam_repo.RefreshToken, error)
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
}

// MFAStore defines operations for MFA configuration.
type MFAStore interface {
	ListMFAConfigs(ctx context.Context, userID string) ([]iam_repo.MFAConfig, error)
	GetMFAConfig(ctx context.Context, userID, mfaType string) (*iam_repo.MFAConfig, error)
	UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secretEncrypted string) error
	EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error
	StoreBackupCodes(ctx context.Context, userID string, hashes []string) error
	ConsumeBackupCode(ctx context.Context, userID string, codeHash string) (bool, error)
}

// ConnectorStore defines operations for OAuth2/OIDC connectors.
type ConnectorStore interface {
	GetConnectorByID(ctx context.Context, id string) (*iam_repo.Connector, error)
	ListConnectors(ctx context.Context) ([]iam_repo.Connector, error)
	CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error)
	UpdateConnector(ctx context.Context, id, name string, uris []string) error
	DeleteConnector(ctx context.Context, id string) error
}

// WebAuthnStore defines operations for WebAuthn credentials.
type WebAuthnStore interface {
	SaveWebAuthnCredential(ctx context.Context, orgID, userID string, cred iam_repo.WebAuthnCredential) error
	ListWebAuthnCredentials(ctx context.Context, userID string) ([]iam_repo.WebAuthnCredential, error)
}

// SAMLStore defines operations for SAML configuration.
type SAMLStore interface {
	UpsertSAMLProvider(ctx context.Context, orgID string, p *iam_repo.SAMLProvider) (*iam_repo.SAMLProvider, error)
	GetSAMLProvider(ctx context.Context, orgID string) (*iam_repo.SAMLProvider, error)
	ListSAMLProviders(ctx context.Context, orgID string) ([]*iam_repo.SAMLProvider, error)
}

// OrgStore defines operations for organization management.
type OrgStore interface {
	CreateOrg(ctx context.Context, name string) (string, error)
}

// OutboxStore defines operations for transactional outbox.
type OutboxStore interface {
	CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error
}

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

// Repository combines all sub-interfaces.
type Repository interface {
	UserStore
	SessionStore
	TokenStore
	MFAStore
	ConnectorStore
	WebAuthnStore
	SAMLStore
	OrgStore
	OutboxStore
	BeginTx(ctx context.Context) (pgx.Tx, error)
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

// Redis returns the underlying redis client.
func (s *Service) Redis() *redis.Client {
	return s.rdb
}

// GetKeyring returns the JWT keyring.
func (s *Service) GetKeyring() []crypto.JWTKey {
	return s.keyring
}
