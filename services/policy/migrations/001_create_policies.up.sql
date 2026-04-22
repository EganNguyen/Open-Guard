CREATE TABLE policies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL, -- references orgs in iam, but we use loose coupling
    name        TEXT NOT NULL,
    effect      TEXT NOT NULL DEFAULT 'allow', -- allow, deny
    actions     TEXT[] NOT NULL,
    resources   TEXT[] NOT NULL,
    conditions  JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policies_org_id ON policies(org_id);

-- Enable RLS
ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies FORCE ROW LEVEL SECURITY;

CREATE POLICY policies_org_isolation ON policies
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON policies TO openguard_app;
