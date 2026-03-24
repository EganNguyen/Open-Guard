-- 007_outbox_rls_trigger.up.sql

ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;

CREATE POLICY outbox_org_isolation ON outbox_records
    USING (key = current_setting('app.org_id', true));

CREATE OR REPLACE FUNCTION notify_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('outbox_new', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
    AFTER INSERT ON outbox_records
    FOR EACH ROW EXECUTE FUNCTION notify_outbox();
