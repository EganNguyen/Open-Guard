# OpenGuard

Open-source, self-hostable **organization security platform** inspired by Atlassian Guard.

## Features

- **Identity & Access Management (IAM):** SSO (SAML 2.0 / OIDC), SCIM provisioning, MFA enforcement, API token lifecycle.
- **Policy Engine:** Data security rules — export restrictions, anonymous access controls, RBAC.
- **Threat Detection:** Real-time anomaly scoring on login and data-access event streams.
- **Audit Log:** Immutable, queryable record of all admin, user, and system actions.
- **Alerting:** Rule-based and ML-scored alerts with SIEM webhook export.
- **Compliance Reporting:** GDPR, SOC 2, HIPAA report generation.
- **Admin Dashboard:** Next.js web console for all of the above.

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/openguard/openguard.git && cd openguard

# 2. Copy environment file
cp .env.example .env

# 3. Start all infrastructure and services for local development
make dev

# 4. Run migrations
make migrate

# 5. Testing
# Unit tests:
make test
# Integration tests:
make test-integration
# End-to-end tests:
# make test-e2e
```

## Documentation

- [Architecture](docs/architecture.md)
- [Contributing](docs/contributing.md)
- [API Documentation](docs/api/)
