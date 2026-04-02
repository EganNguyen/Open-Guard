-- 007_outbox_rls_trigger.up.sql

ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_records FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_policies
        WHERE schemaname = 'public'
          AND tablename = 'outbox_records'
          AND policyname = 'outbox_org_isolation'
    ) THEN
        CREATE POLICY outbox_org_isolation
            ON outbox_records
            USING (key = current_setting('app.org_id', true));
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION notify_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('outbox_new', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_trigger
        WHERE tgname = 'outbox_insert_notify'
          AND tgrelid = 'outbox_records'::regclass
    ) THEN
        CREATE TRIGGER outbox_insert_notify
            AFTER INSERT ON outbox_records
            FOR EACH ROW EXECUTE FUNCTION notify_outbox();
    END IF;
END
$$;
