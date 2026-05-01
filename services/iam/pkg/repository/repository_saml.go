package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UpsertSAMLProvider creates or replaces the SAML provider for an org (one per org).
func (r *Repository) UpsertSAMLProvider(ctx context.Context, orgID string, p *SAMLProvider) (*SAMLProvider, error) {
	attrMap := p.AttributeMap
	if attrMap == nil {
		attrMap = json.RawMessage("{}")
	}

	var out SAMLProvider
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			INSERT INTO saml_providers
				(org_id, entity_id, sso_url, slo_url, metadata_xml, sp_cert_pem, sp_key_pem, name_id_format, attribute_map, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (org_id) DO UPDATE SET
				entity_id      = EXCLUDED.entity_id,
				sso_url        = EXCLUDED.sso_url,
				slo_url        = EXCLUDED.slo_url,
				metadata_xml   = EXCLUDED.metadata_xml,
				sp_cert_pem    = EXCLUDED.sp_cert_pem,
				sp_key_pem     = EXCLUDED.sp_key_pem,
				name_id_format = EXCLUDED.name_id_format,
				attribute_map  = EXCLUDED.attribute_map,
				enabled        = EXCLUDED.enabled,
				updated_at     = now()
			RETURNING id, org_id, entity_id, sso_url, COALESCE(slo_url,''), metadata_xml, sp_cert_pem, sp_key_pem, name_id_format, attribute_map, enabled, created_at, updated_at
		`, orgID, p.EntityID, p.SSOURL, p.SLOURL, p.MetadataXML, p.SPCertPEM, p.SPKeyPEM, p.NameIDFormat, attrMap, p.Enabled,
		).Scan(
			&out.ID, &out.OrgID, &out.EntityID, &out.SSOURL, &out.SLOURL,
			&out.MetadataXML, &out.SPCertPEM, &out.SPKeyPEM, &out.NameIDFormat,
			&out.AttributeMap, &out.Enabled, &out.CreatedAt, &out.UpdatedAt,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("upsert saml provider: %w", err)
	}
	return &out, nil
}

// GetSAMLProvider retrieves the SAML provider configuration for an org.
func (r *Repository) GetSAMLProvider(ctx context.Context, orgID string) (*SAMLProvider, error) {
	var out SAMLProvider
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT id, org_id, entity_id, sso_url, COALESCE(slo_url,''), metadata_xml, sp_cert_pem, sp_key_pem, name_id_format, attribute_map, enabled, created_at, updated_at
			FROM saml_providers WHERE org_id = $1
		`, orgID).Scan(
			&out.ID, &out.OrgID, &out.EntityID, &out.SSOURL, &out.SLOURL,
			&out.MetadataXML, &out.SPCertPEM, &out.SPKeyPEM, &out.NameIDFormat,
			&out.AttributeMap, &out.Enabled, &out.CreatedAt, &out.UpdatedAt,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("get saml provider: %w", err)
	}
	return &out, nil
}

// ListSAMLProviders returns all SAML providers visible in the current RLS session.
func (r *Repository) ListSAMLProviders(ctx context.Context, orgID string) ([]*SAMLProvider, error) {
	var providers []*SAMLProvider
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, entity_id, sso_url, COALESCE(slo_url,''), metadata_xml, sp_cert_pem, sp_key_pem, name_id_format, attribute_map, enabled, created_at, updated_at
			FROM saml_providers WHERE org_id = $1 ORDER BY created_at DESC
		`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p SAMLProvider
			if err := rows.Scan(
				&p.ID, &p.OrgID, &p.EntityID, &p.SSOURL, &p.SLOURL,
				&p.MetadataXML, &p.SPCertPEM, &p.SPKeyPEM, &p.NameIDFormat,
				&p.AttributeMap, &p.Enabled, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			providers = append(providers, &p)
		}
		return rows.Err()
	})
	return providers, err
}
