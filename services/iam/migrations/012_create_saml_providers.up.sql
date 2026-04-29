-- 012_create_saml_providers.up.sql
CREATE TABLE IF NOT EXISTS saml_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    entity_id TEXT NOT NULL,
    sso_url TEXT NOT NULL,
    slo_url TEXT,
    metadata_xml TEXT NOT NULL,
    sp_cert_pem TEXT NOT NULL,
    sp_key_pem TEXT NOT NULL,
    name_id_format TEXT NOT NULL DEFAULT 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
    attribute_map JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT saml_providers_org_unique UNIQUE (org_id)
);

ALTER TABLE saml_providers ENABLE ROW LEVEL SECURITY;

CREATE POLICY saml_providers_org_isolation ON saml_providers
    USING (org_id::text = current_setting('app.org_id', true));

CREATE INDEX IF NOT EXISTS saml_providers_org_id_idx ON saml_providers (org_id);

CREATE TRIGGER saml_providers_updated_at
    BEFORE UPDATE ON saml_providers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
