package repository

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user entity.
type User struct {
	ID               string     `json:"id"`
	OrgID            string     `json:"org_id"`
	Email            string     `json:"email"`
	PasswordHash     string     `json:"-"`
	DisplayName      string     `json:"display_name"`
	Role             string     `json:"role"`
	Status           string     `json:"status"`
	FailedLoginCount int        `json:"failed_login_count"`
	LockedUntil      *time.Time `json:"locked_until"`
	SCIMExternalID   *string    `json:"scim_external_id"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Session represents an active user session.
type Session struct {
	JTI       string `json:"jti"`
	UserAgent string `json:"user_agent"`
	IPAddress string `json:"ip_address"`
}

// MFAConfig represents an MFA configuration for a user.
type MFAConfig struct {
	MFAType         string `json:"mfa_type"`
	SecretEncrypted string `json:"secret_encrypted"`
}

// Connector represents an OAuth2/OIDC connector.
type Connector struct {
	ID           string   `json:"id"`
	OrgID        *string  `json:"org_id"` // Can be null for system connectors
	Name         string   `json:"name"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

// RefreshToken represents an OAuth2 refresh token.
type RefreshToken struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id"`
	FamilyID  uuid.UUID `json:"family_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// WebAuthnCredential represents a WebAuthn credential record.
type WebAuthnCredential struct {
	CredentialID    string `json:"credential_id"`
	PublicKey       string `json:"public_key"`
	AttestationType string `json:"attestation_type"`
	SignCount       int32  `json:"sign_count"`
}
