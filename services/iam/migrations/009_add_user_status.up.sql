DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='status') THEN
        ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'initializing';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
