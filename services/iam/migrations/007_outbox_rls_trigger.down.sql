-- 007_outbox_rls_trigger.down.sql

DROP TRIGGER IF EXISTS outbox_insert_notify ON outbox_records;
DROP FUNCTION IF EXISTS notify_outbox();

DROP POLICY IF EXISTS outbox_org_isolation ON outbox_records;

ALTER TABLE outbox_records NO FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_records DISABLE ROW LEVEL SECURITY;
