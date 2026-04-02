CREATE TABLE IF NOT EXISTS sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    refresh_hash TEXT NOT NULL UNIQUE,
    ip_address   INET,
    user_agent   TEXT,
    country_code TEXT,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_policies
        WHERE schemaname = 'public'
          AND tablename = 'sessions'
          AND policyname = 'tenant_isolation'
    ) THEN
        CREATE POLICY tenant_isolation
            ON sessions
            USING (org_id::text = current_setting('app.org_id', true));
    END IF;
END
$$;
