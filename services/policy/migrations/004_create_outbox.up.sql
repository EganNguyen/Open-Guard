-- Outbox table for policy service event publishing per spec §4.1
-- Publishes policy.changes events to Kafka on policy mutations

CREATE TABLE outbox_records (
    id UUID PRIMARY KEY DEFAULT GEN_RANDOM_UUID(),
    org_id UUID NOT NULL,
    topic TEXT NOT NULL,
    key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- pending, published, failed, dead
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    dead_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_status ON outbox_records (status) WHERE status
= 'pending';
CREATE INDEX idx_outbox_org_id ON outbox_records (org_id);

GRANT SELECT, INSERT, UPDATE ON outbox_records TO openguard_app;

-- Trigger to notify outbox relay via LISTEN/NOTIFY on insert
CREATE OR REPLACE FUNCTION NOTIFY_OUTBOX_INSERT()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('outbox_new', NEW.id::TEXT);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_notify
AFTER INSERT ON outbox_records
FOR EACH ROW EXECUTE FUNCTION NOTIFY_OUTBOX_INSERT();
