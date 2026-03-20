-- 003_create_policy_eval_log.down.sql
DROP POLICY IF EXISTS policy_eval_org_isolation ON policy_eval_log;
DROP TABLE IF EXISTS policy_eval_log;
