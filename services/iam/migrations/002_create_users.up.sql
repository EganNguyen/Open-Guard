CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id              UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    email               TEXT NOT NULL,
    display_name        TEXT NOT NULL DEFAULT '',
    password_hash       TEXT,            -- bcrypt, cost 12
    status              TEXT NOT NULL DEFAULT 'active',
    mfa_enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_method          TEXT,
    scim_external_id    TEXT,
    provisioning_status TEXT NOT NULL DEFAULT 'complete',
    tier_isolation      TEXT NOT NULL DEFAULT 'shared',
    version             INT NOT NULL DEFAULT 1,  -- Atomic increment for SCIM ETags
    last_login_at       TIMESTAMPTZ,
    last_login_ip       INET,
    failed_login_count  INT NOT NULL DEFAULT 0,
    locked_until        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,
    UNIQUE (org_id, email)
);

CREATE INDEX idx_users_org_id ON users(org_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_scim_id ON users(scim_external_id) WHERE scim_external_id IS NOT NULL;

-- Enable RLS
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

CREATE POLICY users_org_isolation ON users
    USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
    WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON users TO openguard_app;
