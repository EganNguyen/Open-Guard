CREATE TABLE IF NOT EXISTS outbox_records (
    id           UUID PRIMARY KEY,
    org_id       UUID NOT NULL, -- required for RLS
    topic        TEXT NOT NULL,
    key          TEXT NOT NULL,
    payload      BYTEA NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    dead_at      TIMESTAMPTZ
);

-- Ensure org_id exists if table was already created in a previous run
ALTER TABLE outbox_records ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_outbox_org_id ON outbox_records(org_id);

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_records(created_at) WHERE status = 'pending';
