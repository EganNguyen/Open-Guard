CREATE TABLE orgs (
    id UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active',
    tier_isolation TEXT NOT NULL DEFAULT 'shared',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Enable RLS
ALTER TABLE orgs ENABLE ROW LEVEL SECURITY;
ALTER TABLE orgs FORCE ROW LEVEL SECURITY;

-- Self-read policy
CREATE POLICY orgs_self_isolation ON orgs
USING (id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
WITH CHECK (id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, UPDATE ON orgs TO openguard_app;
