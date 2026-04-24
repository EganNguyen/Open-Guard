-- Create application roles
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'openguard_app') THEN
        CREATE ROLE openguard_app;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'openguard_login') THEN
        CREATE ROLE openguard_login;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'openguard_outbox') THEN
        CREATE ROLE openguard_outbox;
    END IF;
END
$$;

-- Create databases for isolation
SELECT 'CREATE DATABASE openguard_iam' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'openguard_iam')\gexec
SELECT 'CREATE DATABASE openguard_policy' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'openguard_policy')\gexec
SELECT 'CREATE DATABASE openguard_dlp' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'openguard_dlp')\gexec

-- Grant permissions (will be executed against the database specified at connection time)
-- Note: Permissions on databases need to be granted while connected to that DB, but init scripts run as superuser.
ALTER DATABASE openguard_iam OWNER TO openguard;
ALTER DATABASE openguard_policy OWNER TO openguard;
ALTER DATABASE openguard_dlp OWNER TO openguard;
