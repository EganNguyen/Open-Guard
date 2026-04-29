CREATE TABLE webhook_deliveries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    connector_id  UUID NOT NULL,
    event_id      UUID NOT NULL,
    target_url    TEXT NOT NULL,
    payload       JSONB NOT NULL,
    attempts      INT DEFAULT 0,
    status        TEXT DEFAULT 'pending', -- pending, delivered, failed, dlq
    last_error    TEXT,
    next_retry_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);
