-- 004_create_outbox_records.up.sql
-- Transactional outbox for the policy service.
-- Every policy mutation publishes a policy.changes event via this table.

CREATE TABLE IF NOT EXISTS policy_outbox_records (
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

CREATE INDEX idx_policy_outbox_pending ON policy_outbox_records(created_at) WHERE status = 'pending';
CREATE INDEX idx_policy_outbox_status ON policy_outbox_records(status);
