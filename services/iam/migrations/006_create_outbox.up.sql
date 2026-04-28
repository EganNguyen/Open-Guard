CREATE TABLE outbox_records (
    id UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id UUID NOT NULL,
    topic TEXT NOT NULL,
    key TEXT NOT NULL,
    payload BYTEA NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON outbox_records (created_at) WHERE status
= 'pending';

-- NOTIFY trigger for immediate relay wake-up
CREATE OR REPLACE FUNCTION NOTIFY_OUTBOX() RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('outbox_new', NEW.id::TEXT);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
AFTER INSERT ON outbox_records
FOR EACH ROW EXECUTE FUNCTION NOTIFY_OUTBOX();

-- RLS: Outbox relay role bypasses RLS on this table, but app role is restricted
ALTER TABLE outbox_records ENABLE ROW LEVEL SECURITY;

CREATE POLICY outbox_org_isolation ON outbox_records
USING (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID)
WITH CHECK (org_id = NULLIF(CURRENT_SETTING('app.org_id', TRUE), '')::UUID);

GRANT SELECT, INSERT ON outbox_records TO openguard_app;
GRANT SELECT, UPDATE, DELETE ON outbox_records TO openguard_outbox;
