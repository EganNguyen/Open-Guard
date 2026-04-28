CREATE TABLE mfa_configs (
    id UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id UUID NOT NULL REFERENCES orgs (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    mfa_type TEXT NOT NULL, -- 'totp', 'webauthn'
    secret_encrypted TEXT,          -- encrypted with shared/crypto/aes
    backup_code_hashes TEXT [],        -- HMAC-SHA256 hashes
    webauthn_id BYTEA,
    webauthn_public_key BYTEA,
    sign_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, mfa_type)
);

CREATE INDEX idx_mfa_configs_org_id ON mfa_configs (org_id);
CREATE INDEX idx_mfa_configs_user_id ON mfa_configs (user_id);

-- Enable RLS
ALTER TABLE mfa_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE mfa_configs FORCE ROW LEVEL SECURITY;

CREATE POLICY mfa_configs_org_isolation ON mfa_configs
USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON mfa_configs TO openguard_app;
