# Policy Service

**Core Intent:** RBAC and CEL policy evaluation engine.
**Boundaries:**
- `service/evaluation.go`: Core engine logic, caching (Redis), CEL execution.
- `service/rules.go`: CRUD operations for policies.
- `repository/repo_evaluation.go`: DB matching logic.

**AI Rules:**
- If fixing a policy evaluation bug, focus on `evaluation.go` and `evaluateFromDB`.
- If modifying CRUD, focus on `rules.go` and `handler_policies.go`.
