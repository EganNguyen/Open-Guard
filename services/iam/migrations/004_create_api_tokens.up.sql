CREATE TABLE api_tokens (
    id            UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id        UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    token_hash    TEXT NOT NULL,
    token_prefix  TEXT NOT NULL,
    scopes        TEXT[] NOT NULL DEFAULT '{}',
    revoked       BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at    TIMESTAMPTZ,
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_tokens_org_id ON api_tokens(org_id);
CREATE INDEX idx_api_tokens_prefix ON api_tokens(token_prefix);

-- Enable RLS
ALTER TABLE api_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY api_tokens_org_isolation ON api_tokens
    USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
    WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON api_tokens TO openguard_app;
