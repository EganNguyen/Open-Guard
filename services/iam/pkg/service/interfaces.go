package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
)

// UserRepository handles user-related operations.
type UserRepository interface {
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

// SessionRepository handles session-related operations.
type SessionRepository interface {
	CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error
	GetSessionByUserID(ctx context.Context, userID string) (*iam_repo.Session, error)
	GetActiveJTIs(ctx context.Context, userID string) ([]string, error)
	GetSessionTTL(ctx context.Context, jti string) time.Duration
	RevokeSessions(ctx context.Context, userID string) error
}

// TokenRepository handles token-related operations.
type TokenRepository interface {
	CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*iam_repo.RefreshToken, error)
	ClaimRefreshToken(ctx context.Context, tokenHash string) (*iam_repo.RefreshToken, error)
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
}

// MFARepository handles MFA-related operations.
type MFARepository interface {
	ListMFAConfigs(ctx context.Context, userID string) ([]iam_repo.MFAConfig, error)
	GetMFAConfig(ctx context.Context, userID, mfaType string) (*iam_repo.MFAConfig, error)
	UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secretEncrypted string) error
	EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error
	StoreBackupCodes(ctx context.Context, userID string, hashes []string) error
	ConsumeBackupCode(ctx context.Context, userID string, codeHash string) (bool, error)
}

// ConnectorRepository handles connector-related operations.
type ConnectorRepository interface {
	GetConnectorByID(ctx context.Context, id string) (*iam_repo.Connector, error)
	ListConnectors(ctx context.Context) ([]iam_repo.Connector, error)
	CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error)
	UpdateConnector(ctx context.Context, id, name string, uris []string) error
	DeleteConnector(ctx context.Context, id string) error
}

// WebAuthnRepository handles WebAuthn-related operations.
type WebAuthnRepository interface {
	SaveWebAuthnCredential(ctx context.Context, orgID, userID string, cred iam_repo.WebAuthnCredential) error
	ListWebAuthnCredentials(ctx context.Context, userID string) ([]iam_repo.WebAuthnCredential, error)
}

// SAMLRepository handles SAML-related operations.
type SAMLRepository interface {
	UpsertSAMLProvider(ctx context.Context, orgID string, p *iam_repo.SAMLProvider) (*iam_repo.SAMLProvider, error)
	GetSAMLProvider(ctx context.Context, orgID string) (*iam_repo.SAMLProvider, error)
	ListSAMLProviders(ctx context.Context, orgID string) ([]*iam_repo.SAMLProvider, error)
}

// OrgRepository handles organization-related operations.
type OrgRepository interface {
	CreateOrg(ctx context.Context, name string) (string, error)
}

// OutboxRepository handles outbox-related operations.
type OutboxRepository interface {
	CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error
}

// Repository combines all domain-specific interfaces.
type Repository interface {
	UserRepository
	SessionRepository
	TokenRepository
	MFARepository
	ConnectorRepository
	WebAuthnRepository
	SAMLRepository
	OrgRepository
	OutboxRepository
	BeginTx(ctx context.Context) (pgx.Tx, error)
}
