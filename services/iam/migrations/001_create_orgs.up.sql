CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE orgs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,
    plan         TEXT NOT NULL DEFAULT 'free',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
