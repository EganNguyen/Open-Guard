-- 002_create_policy_assignments.up.sql
-- Creates policy assignments (which policies apply to which principals) with RLS.

CREATE TABLE IF NOT EXISTS policy_assignments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL,
    policy_id      UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    principal_id   UUID NOT NULL,                -- user_id or group_id
    principal_type TEXT NOT NULL DEFAULT 'user', -- user | group | role
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_assignments_org ON policy_assignments(org_id);
CREATE INDEX idx_policy_assignments_policy ON policy_assignments(policy_id);
CREATE INDEX idx_policy_assignments_principal ON policy_assignments(org_id, principal_id, principal_type);
CREATE UNIQUE INDEX idx_policy_assignments_unique ON policy_assignments(policy_id, principal_id, principal_type);

ALTER TABLE policy_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_assignments FORCE ROW LEVEL SECURITY;
CREATE POLICY policy_assignments_org_isolation ON policy_assignments
    USING (org_id = current_setting('app.org_id', true)::UUID)
    WITH CHECK (org_id = current_setting('app.org_id', true)::UUID);
