-- Grant SELECT to openguard_login for cross-tenant login lookup
GRANT SELECT ON users TO openguard_login;
GRANT SELECT ON orgs TO openguard_login;

-- Grant permissions to openguard_outbox for event relay
GRANT SELECT, DELETE ON outbox_records TO openguard_outbox;
