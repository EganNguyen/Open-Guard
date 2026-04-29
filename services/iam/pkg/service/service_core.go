package service

import (
	"context"
	"fmt"
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
	ErrSessionRevokedRisk = fmt.Errorf("SESSION_REVOKED_RISK")
	ErrSessionCompromised = fmt.Errorf("SESSION_COMPROMISED")
)

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
	ClaimRefreshToken(ctx context.Context, tokenHash string) (map[string]interface{}, error)
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error
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
	ListUsersPaginated(ctx context.Context, orgID string, filter string, offset, limit int) ([]map[string]interface{}, int, error)
	BeginTx(ctx context.Context) (pgx.Tx, error)
	CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error
	UpdateUserStatus(ctx context.Context, userID, status string) error
	UpdateUserDisplayName(ctx context.Context, userID, displayName string) error
	UpdateUserSCIM(ctx context.Context, userID, externalID, status string) error
	GetActiveJTIs(ctx context.Context, userID string) ([]string, error)
	GetSessionTTL(ctx context.Context, jti string) time.Duration
	RevokeSessions(ctx context.Context, userID string) error
	GetSessionByUserID(ctx context.Context, userID string) (map[string]interface{}, error)
	DeprovisionAllUsers(ctx context.Context, orgID string) error
	GetUserByExternalID(ctx context.Context, orgID, externalID string) (map[string]interface{}, error)
	SaveWebAuthnCredential(ctx context.Context, orgID, userID string, cred map[string]interface{}) error
	ListWebAuthnCredentials(ctx context.Context, userID string) ([]map[string]interface{}, error)
	UpsertSAMLProvider(ctx context.Context, orgID string, p *iam_repo.SAMLProvider) (*iam_repo.SAMLProvider, error)
	GetSAMLProvider(ctx context.Context, orgID string) (*iam_repo.SAMLProvider, error)
	ListSAMLProviders(ctx context.Context, orgID string) ([]*iam_repo.SAMLProvider, error)

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
