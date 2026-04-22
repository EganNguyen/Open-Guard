CREATE TABLE sessions (
    id            UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id        UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    jti           TEXT NOT NULL UNIQUE,
    user_agent    TEXT,
    ip_address    INET,
    revoked       BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_org_id ON sessions(org_id);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_jti ON sessions(jti);

-- Enable RLS
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;

CREATE POLICY sessions_org_isolation ON sessions
    USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
    WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON sessions TO openguard_app;
