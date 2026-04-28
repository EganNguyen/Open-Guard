-- Revoke permissions granted in 011_fix_login_permissions.up.sql
REVOKE SELECT ON users FROM openguard_login;
REVOKE SELECT ON orgs FROM openguard_login;
REVOKE SELECT, DELETE ON outbox_records FROM openguard_outbox;
