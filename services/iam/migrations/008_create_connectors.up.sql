-- 008_create_connectors.up.sql
CREATE TABLE IF NOT EXISTS connectors (
    id TEXT PRIMARY KEY, -- usually a slug or UUID string
    org_id UUID NOT NULL REFERENCES orgs (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    client_secret TEXT NOT NULL,    -- OAuth2 secret
    redirect_uris TEXT [] NOT NULL,
    api_key_prefix TEXT UNIQUE,      -- ogk_ prefix for fast lookup (R-08)
    api_key_hash TEXT UNIQUE,      -- PBKDF2 hash of the API key (R-08)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_connectors_org_id ON connectors (org_id);
CREATE INDEX IF NOT EXISTS idx_connectors_api_key_prefix ON connectors (
    api_key_prefix
);

-- Enable RLS
ALTER TABLE connectors ENABLE ROW LEVEL SECURITY;
ALTER TABLE connectors FORCE ROW LEVEL SECURITY;

CREATE POLICY connectors_org_isolation ON connectors
USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON connectors TO openguard_app;
