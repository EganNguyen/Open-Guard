CREATE TABLE mfa_configs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type         TEXT NOT NULL DEFAULT 'totp',
    secret       TEXT NOT NULL,
    backup_codes TEXT[] NOT NULL DEFAULT '{}',
    verified     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE mfa_configs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mfa_configs USING (org_id::text = current_setting('app.org_id', true));
