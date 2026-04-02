CREATE TABLE IF NOT EXISTS outbox_records (
    id           UUID PRIMARY KEY,
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

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_records(created_at) WHERE status = 'pending';
