-- 008_policy_resource_action.up.sql
-- Add action and resource columns to policies table
ALTER TABLE policies ADD COLUMN action TEXT NOT NULL DEFAULT '';
ALTER TABLE policies ADD COLUMN resource TEXT NOT NULL DEFAULT '';

-- Add a composite index for faster matching during evaluation
CREATE INDEX idx_policies_org_action_resource ON policies(org_id, action, resource);
