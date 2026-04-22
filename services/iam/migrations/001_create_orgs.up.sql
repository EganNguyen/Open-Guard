CREATE TABLE orgs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Enable RLS
ALTER TABLE orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE orgs FORCE ROW LEVEL SECURITY;

-- Self-read policy
CREATE POLICY orgs_self_isolation ON orgs
    USING (id = NULLIF(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (id = NULLIF(current_setting('app.org_id', true), '')::UUID);

GRANT SELECT, UPDATE ON orgs TO openguard_app;
