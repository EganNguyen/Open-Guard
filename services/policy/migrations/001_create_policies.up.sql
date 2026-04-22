-- Fix policies table schema per spec §11.1:
-- Replace array model (actions TEXT[], resources TEXT[], effect TEXT)
-- with flexible JSONB logic expression + version + ETag support

-- Drop and recreate with correct schema
DROP TABLE IF EXISTS policies CASCADE;

CREATE TABLE policies (
    id          UUID        PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id      UUID        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    logic       JSONB       NOT NULL,          -- flexible rule expression per spec §11.1
    version     INT         NOT NULL DEFAULT 1, -- increments on each mutation for ETag
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policies_org_id ON policies(org_id);

-- Enable RLS
ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies FORCE ROW LEVEL SECURITY;

CREATE POLICY policies_org_isolation ON policies
    USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
    WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, UPDATE, DELETE ON policies TO openguard_app;
