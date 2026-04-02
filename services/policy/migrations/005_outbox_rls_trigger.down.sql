-- 005_outbox_rls_trigger.down.sql

DROP TRIGGER IF EXISTS policy_outbox_insert_notify ON policy_outbox_records;
DROP FUNCTION IF EXISTS notify_policy_outbox();

DROP POLICY IF EXISTS policy_outbox_org_isolation ON policy_outbox_records;

ALTER TABLE policy_outbox_records NO FORCE ROW LEVEL SECURITY;
ALTER TABLE policy_outbox_records DISABLE ROW LEVEL SECURITY;
