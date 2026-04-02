-- migration: 001_todos.up.sql

CREATE TABLE IF NOT EXISTS todos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    user_id TEXT NOT NULL,
    title TEXT NOT NULL,
    completed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Enable RLS
ALTER TABLE todos ENABLE ROW LEVEL SECURITY;
ALTER TABLE todos FORCE ROW LEVEL SECURITY;

-- Policy: Only allow access if app.org_id matches the row's org_id
CREATE POLICY todos_org_isolation ON todos
    USING (org_id = nullif(current_setting('app.org_id', true), '')::UUID)
    WITH CHECK (org_id = nullif(current_setting('app.org_id', true), '')::UUID);

-- Index for performance
CREATE INDEX IF NOT EXISTS idx_todos_org_user ON todos(org_id, user_id);
