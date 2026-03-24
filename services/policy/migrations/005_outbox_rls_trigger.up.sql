-- 005_outbox_rls_trigger.up.sql

ALTER TABLE policy_outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_outbox_records FORCE ROW LEVEL SECURITY;

CREATE POLICY policy_outbox_org_isolation ON policy_outbox_records
    USING (key = current_setting('app.org_id', true));

CREATE OR REPLACE FUNCTION notify_policy_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('policy_outbox_new', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER policy_outbox_insert_notify
    AFTER INSERT ON policy_outbox_records
    FOR EACH ROW EXECUTE FUNCTION notify_policy_outbox();
