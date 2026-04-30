package service

import (
	"context"
	"fmt"

	iam_repo "github.com/openguard/services/iam/pkg/repository"
)

// UpsertSAMLProvider stores or replaces the SAML IdP configuration for an org.
func (s *Service) UpsertSAMLProvider(ctx context.Context, orgID string, p *iam_repo.SAMLProvider) (*iam_repo.SAMLProvider, error) {
	if orgID == "" {
		return nil, fmt.Errorf("org_id required")
	}
	if p.EntityID == "" || p.SSOURL == "" || p.MetadataXML == "" {
		return nil, fmt.Errorf("entity_id, sso_url, and metadata_xml are required")
	}
	p.Enabled = true
	return s.repo.UpsertSAMLProvider(ctx, orgID, p)
}

// GetSAMLProvider retrieves the SAML IdP configuration for an org.
func (s *Service) GetSAMLProvider(ctx context.Context, orgID string) (*iam_repo.SAMLProvider, error) {
	return s.repo.GetSAMLProvider(ctx, orgID)
}

// ListSAMLProviders returns all SAML providers for an org.
func (s *Service) ListSAMLProviders(ctx context.Context, orgID string) ([]*iam_repo.SAMLProvider, error) {
	return s.repo.ListSAMLProviders(ctx, orgID)
}

// ProvisionOrGetSAMLUser looks up a user by their SAML NameID (external ID).
// If none is found, a new user is provisioned with an empty password hash
// (password login is disabled for SAML-federated users).
func (s *Service) ProvisionOrGetSAMLUser(ctx context.Context, orgID, nameID, email, displayName string) (*iam_repo.User, error) {
	// Try to find by external ID first (idempotent re-entry on repeated SSO)
	existing, err := s.repo.GetUserByExternalID(ctx, orgID, nameID)
	if err == nil && existing != nil {
		return existing, nil
	}

	// Fall back to lookup by email in case the user was pre-provisioned
	if email != "" {
		byEmail, err := s.repo.GetUserByEmail(ctx, email)
		if err == nil && byEmail != nil {
			// Wire the external SAML ID to the existing account
			_ = s.repo.UpdateUserSCIM(ctx, byEmail.ID, nameID, "active")
			return byEmail, nil
		}
	}

	// First-time SSO login — JIT provision a new active user.
	// No password: SAML users cannot log in via password flow.
	if displayName == "" {
		displayName = email
	}
	userID, err := s.repo.CreateUser(ctx, orgID, email, "", displayName, "member", "active")
	if err != nil {
		return nil, fmt.Errorf("saml jit provision: %w", err)
	}
	// Link the NameID as the SCIM external ID so future logins resolve correctly.
	if err := s.repo.UpdateUserSCIM(ctx, userID, nameID, "active"); err != nil {
		return nil, fmt.Errorf("saml link external id: %w", err)
	}
	return s.repo.GetUserByID(ctx, userID)
}
