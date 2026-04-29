# IAM Service

**Core Intent:** Identity, Auth, MFA, SCIM provisioning.
**Boundaries:**
- `service/service_core.go` / `auth.go` / `mfa.go` / `users.go`: Business logic separated by domain.
- `repository/repo_*.go`: Postgres data access, uses Row-Level Security (RLS) via `withOrgContext`.
- `handlers/handler_*.go`: HTTP layer.

**AI Rules:**
- Never edit multiple files if a task only requires one domain (e.g. MFA).
- Read specific files (like `auth.go`) instead of the whole folder.
