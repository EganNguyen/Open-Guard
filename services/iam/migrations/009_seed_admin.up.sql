-- migration: 009_seed_admin.up.sql

-- Seed System Admin Organization
INSERT INTO orgs (id, name, slug, plan)
VALUES ('00000000-0000-0000-0000-000000000001', 'OpenGuard System', 'openguard-system', 'enterprise')
ON CONFLICT (id) DO NOTHING;

-- Seed System Admin User
-- Password: admin-password-123
INSERT INTO users (id, org_id, email, display_name, password_hash, status)
VALUES ('00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'admin@openguard.io', 'System Admin', '$2a$12$6K8Z696h696h696h696h6u6K8Z696h696h696h696h6u6K8Z6', 'active')
ON CONFLICT (id) DO NOTHING;
