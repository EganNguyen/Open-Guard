CREATE TABLE policy_assignments (
    id          UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id      UUID NOT NULL,
    policy_id   UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    subject_id  UUID NOT NULL, -- user_id or group_id
    subject_type TEXT NOT NULL, -- user, group
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_assignments_subject ON policy_assignments(subject_id, subject_type);
CREATE INDEX idx_assignments_org_id ON policy_assignments(org_id);

-- Enable RLS
ALTER TABLE policy_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_assignments FORCE ROW LEVEL SECURITY;

CREATE POLICY assignments_org_isolation ON policy_assignments
    USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
    WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT, DELETE ON policy_assignments TO openguard_app;
