# Opencode Development Workflow

The `.opencode` workflow is a **Spec-First** process used in Open-Guard to maintain architectural consistency across all microservices.

## 1. The Core Lifecycle

| Step | Action | Command |
| :--- | :--- | :--- |
| **Define** | Update `.opencode/opencode.manifest.yaml` | - |
| **Generate**| Scaffold boilerplate and models | `make generate` |
| **Implement**| Add business logic in generated files | - |
| **Verify** | Run integration tests | `make test-integration` |

## 2. Manifest Usage

### Adding a New Service
1. Open `.opencode/opencode.manifest.yaml`.
2. Add a new service entry under `services:`.
3. Specify its `port`, `database`, and `features` (e.g., `rls`, `outbox`).
4. Run `make generate`.

### Adding a New Threat Detector
1. Open `.opencode/phase5-detectors.yaml`.
2. Define the new detector's `signal`, `threshold`, and `risk_score`.
3. Run `make phase5`.

## 3. Developer Guidelines

*   **No Manual Boilerplate:** Do not manually create HTTP handlers, DB connection logic, or mTLS configurations. These must be generated from the manifest to ensure they follow the `shared/` library standards.
*   **Security by Design:** Every service generated via Opencode automatically includes:
    *   **mTLS:** Server and client certificates for secure internal traffic.
    *   **RLS:** Mandatory `rls.SetSessionVar` in every repository method.
    *   **Outbox:** Atomically consistent event publishing via the Transactional Outbox.
*   **Sync Workspace:** Always run `go work sync` after generating a new service to update the monorepo dependencies.
