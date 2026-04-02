-- 001_connectors.up.sql
CREATE TABLE IF NOT EXISTS connectors (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL,
    name TEXT NOT NULL,
    webhook_url TEXT NOT NULL,
    api_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_by UUID,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_connectors_org_id ON connectors(org_id);

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
