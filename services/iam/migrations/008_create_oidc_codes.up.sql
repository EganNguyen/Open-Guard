-- migration: 008_create_oidc_codes.up.sql

CREATE TABLE IF NOT EXISTS oidc_auth_codes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                TEXT NOT NULL UNIQUE,
    client_id           TEXT NOT NULL,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id              UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    redirect_uri        TEXT NOT NULL,
    scope               TEXT NOT NULL DEFAULT 'openid profile email',
    state               TEXT,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_oidc_auth_codes_code ON oidc_auth_codes(code);

ALTER TABLE oidc_auth_codes ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_policies
        WHERE schemaname = 'public'
          AND tablename = 'oidc_auth_codes'
          AND policyname = 'tenant_isolation'
    ) THEN
        CREATE POLICY tenant_isolation
            ON oidc_auth_codes
            USING (org_id::text = current_setting('app.org_id', true));
    END IF;
END
$$;
