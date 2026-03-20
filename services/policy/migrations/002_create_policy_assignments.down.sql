-- 002_create_policy_assignments.down.sql
DROP POLICY IF EXISTS policy_assignments_org_isolation ON policy_assignments;
DROP TABLE IF EXISTS policy_assignments;
