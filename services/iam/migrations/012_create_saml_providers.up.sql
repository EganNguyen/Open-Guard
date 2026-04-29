-- 012_create_saml_providers.up.sql
CREATE TABLE IF NOT EXISTS saml_providers (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    entity_id       TEXT        NOT NULL,           -- IdP Entity ID (issuer)
    sso_url         TEXT        NOT NULL,           -- IdP SSO URL (HTTP-POST or HTTP-Redirect)
    slo_url         TEXT,                           -- IdP Single Logout URL (optional)
    metadata_xml    TEXT        NOT NULL,           -- Raw IdP metadata XML (source of truth)
    sp_cert_pem     TEXT        NOT NULL,           -- SP signing/encryption certificate (PEM)
    sp_key_pem      TEXT        NOT NULL,           -- SP private key (PEM, encrypted at rest via column-level encryption)
    name_id_format  TEXT        NOT NULL DEFAULT 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
    attribute_map   JSONB       NOT NULL DEFAULT '{}', -- IdP attribute → OpenGuard claim mapping
    enabled         BOOLEAN     NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT saml_providers_org_unique UNIQUE (org_id)   -- one IdP per org
);
ALTER TABLE saml_providers ENABLE ROW LEVEL SECURITY;
CREATE POLICY saml_providers_org_isolation ON saml_providers
    USING (org_id::text = current_setting('app.org_id', true));
CREATE INDEX IF NOT EXISTS saml_providers_org_id_idx ON saml_providers (org_id);
CREATE TRIGGER saml_providers_updated_at
    BEFORE UPDATE ON saml_providers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
