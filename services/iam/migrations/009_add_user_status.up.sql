ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'initializing';
CREATE INDEX idx_users_status ON users(status);
