CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email               TEXT NOT NULL,
    display_name        TEXT NOT NULL DEFAULT '',
    password_hash       TEXT,
    status              TEXT NOT NULL DEFAULT 'active',
    mfa_enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method          TEXT,
    scim_external_id    TEXT,
    provisioning_status TEXT NOT NULL DEFAULT 'complete',
    tier_isolation      TEXT NOT NULL DEFAULT 'shared',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_users_org_id ON users(org_id);
CREATE INDEX idx_users_email  ON users(email);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users USING (org_id::text = current_setting('app.org_id', true));
