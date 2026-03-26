-- 005_outbox_rls_trigger.up.sql

ALTER TABLE policy_outbox_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_outbox_records FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_policies
        WHERE schemaname = 'public'
          AND tablename = 'policy_outbox_records'
          AND policyname = 'policy_outbox_org_isolation'
    ) THEN
        CREATE POLICY policy_outbox_org_isolation
            ON policy_outbox_records
            USING (key = current_setting('app.org_id', true));
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION notify_policy_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('policy_outbox_new', NEW.id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_trigger
        WHERE tgname = 'policy_outbox_insert_notify'
          AND tgrelid = 'policy_outbox_records'::regclass
    ) THEN
        CREATE TRIGGER policy_outbox_insert_notify
            AFTER INSERT ON policy_outbox_records
            FOR EACH ROW EXECUTE FUNCTION notify_policy_outbox();
    END IF;
END
$$;
