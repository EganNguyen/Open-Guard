-- 001_create_policies.up.sql
-- Creates the policies table with RLS enforcement.

CREATE TABLE IF NOT EXISTS policies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL,                    -- data_export | anon_access | ip_allowlist | session_limit
    rules       JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_policies_org ON policies(org_id);
CREATE INDEX IF NOT EXISTS idx_policies_org_type ON policies(org_id, type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_policies_org_name ON policies(org_id, name);

ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies FORCE ROW LEVEL SECURITY;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_policies
        WHERE schemaname = 'public'
          AND tablename = 'policies'
          AND policyname = 'policies_org_isolation'
    ) THEN
        CREATE POLICY policies_org_isolation
            ON policies
            USING (org_id = current_setting('app.org_id', true)::UUID)
            WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
    END IF;
END
$$;
