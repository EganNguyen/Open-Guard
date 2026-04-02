---
name: openguard-testing
description: >
  Use this skill when writing or reviewing any test in the OpenGuard project.
  Triggers: "write a test", "unit test", "integration test", "table-driven test",
  "testcontainers", "fake", "mock", "contract test", "acceptance criteria",
  "test the handler", "test the repository", "seed data", "test the consumer",
  "test RLS", "verify audit trail", "test the outbox", "load test", "k6",
  or any file ending in _test.go. Enforces the testing standards from the spec:
  70% coverage floor, race detector, behavior-not-implementation, real DB containers
  for integration tests, fakes over generated mocks for narrow interfaces.
---

# OpenGuard Testing Skill

Tests in OpenGuard verify behavior, not implementation. A test that asserts on
internal struct fields or calls unexported methods is a test of the wrong thing.
Tests should survive a complete internal refactor as long as external behavior
is unchanged.

---

## 1. Test Layer Responsibilities

| Layer | Test type | Tool | Databases |
|---|---|---|---|
| Repository | Integration | `testcontainers-go` | Real PostgreSQL, real MongoDB |
| Service | Unit | `testing` + fakes | None (in-memory fakes) |
| Handler | Unit | `net/http/httptest` | None (service is faked) |
| Consumer | Integration | `testcontainers-go` | Real MongoDB + real Kafka |
| Contract | Custom | `testing` | None |
| API (end-to-end) | Integration | `httptest` + real services | Real containers |

---

## 2. Unit Test Structure (Table-Driven, Mandatory)

```go
// pkg/service/user_test.go
package service_test  // external test package — cannot access unexported symbols

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/openguard/iam/pkg/service"
)

func TestService_CreateUser(t *testing.T) {
    cases := []struct {
        name      string
        input     service.CreateUserInput
        repoErr   error
        wantErr   bool
        wantEmail string
    }{
        {
            name:      "success",
            input:     service.CreateUserInput{Email: "user@example.com", OrgID: "org-1"},
            wantEmail: "user@example.com",
        },
        {
            name:    "duplicate email",
            input:   service.CreateUserInput{Email: "dup@example.com", OrgID: "org-1"},
            repoErr: models.ErrAlreadyExists,
            wantErr: true,
        },
        {
            name:    "empty email",
            input:   service.CreateUserInput{Email: "", OrgID: "org-1"},
            wantErr: true,
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            // Build fake dependencies
            repo := &fakeUserRepo{createErr: tc.repoErr}
            outbox := &fakeOutboxWriter{}
            svc := service.NewService(repo, outbox, discardLogger())

            user, err := svc.CreateUser(context.Background(), tc.input)

            if tc.wantErr {
                require.Error(t, err)    // require: fatal if fails — no point checking user
                assert.Nil(t, user)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tc.wantEmail, user.Email)
            // Verify outbox was written (behavioral: did the outbox receive an event?)
            assert.Len(t, outbox.written, 1, "expected one outbox record")
            assert.Equal(t, kafka.TopicAuditTrail, outbox.written[0].Topic)
        })
    }
}
```

**Rules:**
- `require` for fatal assertions (if it fails, the test cannot proceed meaningfully)
- `assert` for non-fatal assertions (collect all failures in one run)
- Table-driven for any function with more than one input/output combination
- External test package (`package foo_test`) — prevents testing internal state
- Test names use `Function_scenario` format: `TestService_CreateUser/duplicate_email`

---

## 3. Fakes (Preferred Over Generated Mocks)

Write fakes for interfaces with ≤ 5 methods. Generated mocks (`mockery`) only for
larger interfaces where writing a fake is impractical.

```go
// In the same test file or a shared testhelpers package (within the service only)

type fakeUserRepo struct {
    users     map[string]*models.User
    createErr error
    getErr    error
}

func (f *fakeUserRepo) Create(_ context.Context, input service.CreateUserInput) (*models.User, error) {
    if f.createErr != nil {
        return nil, f.createErr
    }
    user := &models.User{
        ID:    uuid.New().String(),
        OrgID: input.OrgID,
        Email: input.Email,
    }
    if f.users == nil {
        f.users = make(map[string]*models.User)
    }
    f.users[user.ID] = user
    return user, nil
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*models.User, error) {
    if f.getErr != nil {
        return nil, f.getErr
    }
    u, ok := f.users[id]
    if !ok {
        return nil, models.ErrNotFound
    }
    return u, nil
}

type fakeOutboxWriter struct {
    written []outbox.WriteCall
    writeErr error
}

type outboxWriteCall struct {
    Topic   string
    Key     string
    OrgID   string
    Envelope models.EventEnvelope
}

func (f *fakeOutboxWriter) Write(_ context.Context, _ pgx.Tx,
    topic, key, orgID string, env models.EventEnvelope) error {
    if f.writeErr != nil {
        return f.writeErr
    }
    f.written = append(f.written, outboxWriteCall{
        Topic: topic, Key: key, OrgID: orgID, Envelope: env,
    })
    return nil
}
```

---

## 4. Integration Tests (Real Containers, Mandatory for Repositories)

```go
// pkg/repository/user_integration_test.go
package repository_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/testcontainers/testcontainers-go"
    "github.com/openguard/shared/rls"
)

func TestRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    ctx := context.Background()

    // Start a real PostgreSQL container with migrations applied
    pool := startPostgres(t, ctx) // helper: starts container, runs migrations, returns OrgPool
    repo := repository.NewRepository(pool)

    t.Run("create and retrieve user", func(t *testing.T) {
        org := seedOrg(t, ctx, pool)
        // Set org_id in context — OrgPool will call SET app.org_id automatically
        ctx := rls.WithOrgID(ctx, org.ID)

        created, err := repo.Create(ctx, repository.CreateInput{
            Email:       "test@example.com",
            DisplayName: "Test User",
        })
        require.NoError(t, err)
        require.NotEmpty(t, created.ID)

        found, err := repo.GetByID(ctx, created.ID)
        require.NoError(t, err)
        assert.Equal(t, "test@example.com", found.Email)
    })

    t.Run("RLS isolation: org A cannot see org B users", func(t *testing.T) {
        orgA := seedOrg(t, ctx, pool)
        orgB := seedOrg(t, ctx, pool)

        // Create user in org A
        ctxA := rls.WithOrgID(ctx, orgA.ID)
        userA, err := repo.Create(ctxA, repository.CreateInput{Email: "a@example.com"})
        require.NoError(t, err)

        // Attempt to retrieve org A's user from org B's context
        ctxB := rls.WithOrgID(ctx, orgB.ID)
        _, err = repo.GetByID(ctxB, userA.ID)
        // RLS must return ErrNotFound — not the user
        assert.ErrorIs(t, err, models.ErrNotFound,
            "org B should not see org A's user via RLS")
    })

    t.Run("soft delete: deleted user not returned in list", func(t *testing.T) {
        org := seedOrg(t, ctx, pool)
        orgCtx := rls.WithOrgID(ctx, org.ID)

        user, err := repo.Create(orgCtx, repository.CreateInput{Email: "del@example.com"})
        require.NoError(t, err)

        require.NoError(t, repo.SoftDelete(orgCtx, user.ID))

        users, err := repo.List(orgCtx, repository.ListInput{})
        require.NoError(t, err)
        for _, u := range users {
            assert.NotEqual(t, user.ID, u.ID, "deleted user should not appear in list")
        }
    })
}

// startPostgres starts a PostgreSQL container, runs migrations, returns a *rls.OrgPool.
func startPostgres(t *testing.T, ctx context.Context) *rls.OrgPool {
    t.Helper()
    req := testcontainers.ContainerRequest{
        Image:        "postgres:16-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_USER":     "test",
            "POSTGRES_PASSWORD": "test",
            "POSTGRES_DB":       "openguard_test",
        },
        WaitingFor: wait.ForListeningPort("5432/tcp"),
    }
    container, err := testcontainers.GenericContainer(ctx,
        testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
    require.NoError(t, err)
    t.Cleanup(func() { container.Terminate(ctx) })

    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432")
    dsn := fmt.Sprintf("postgres://test:test@%s:%s/openguard_test?sslmode=disable",
        host, port.Port())

    pool, err := pgxpool.New(ctx, dsn)
    require.NoError(t, err)

    // Run migrations against the test container
    m, err := migrate.New("file://../../migrations", dsn)
    require.NoError(t, err)
    require.NoError(t, m.Up())

    return rls.NewOrgPool(pool)
}
```

---

## 5. Handler Tests (httptest, No Real Server)

```go
// pkg/handlers/users_test.go
package handlers_test

func TestHandler_CreateUser(t *testing.T) {
    cases := []struct {
        name       string
        body       string
        svcErr     error
        wantStatus int
        wantCode   string
    }{
        {
            name:       "success",
            body:       `{"email":"user@example.com","display_name":"Test"}`,
            wantStatus: http.StatusCreated,
        },
        {
            name:       "invalid JSON",
            body:       `{not json}`,
            wantStatus: http.StatusBadRequest,
            wantCode:   "INVALID_JSON",
        },
        {
            name:       "duplicate email",
            body:       `{"email":"dup@example.com","display_name":"Test"}`,
            svcErr:     models.ErrAlreadyExists,
            wantStatus: http.StatusConflict,
            wantCode:   "RESOURCE_CONFLICT",
        },
        {
            name:       "service unavailable",
            body:       `{"email":"ok@example.com","display_name":"Test"}`,
            svcErr:     models.ErrCircuitOpen,
            wantStatus: http.StatusServiceUnavailable,
            wantCode:   "UPSTREAM_UNAVAILABLE",
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            svc := &fakeUserService{createErr: tc.svcErr}
            h := handlers.NewHandler(svc, discardLogger())

            req := httptest.NewRequest(http.MethodPost, "/users",
                strings.NewReader(tc.body))
            req.Header.Set("Content-Type", "application/json")
            // Set org_id in context (simulates auth middleware)
            req = req.WithContext(rls.WithOrgID(req.Context(), "org-test"))

            w := httptest.NewRecorder()
            h.CreateUser(w, req)

            assert.Equal(t, tc.wantStatus, w.Code)
            if tc.wantCode != "" {
                var resp models.APIError
                require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
                assert.Equal(t, tc.wantCode, resp.Error.Code)
            }
        })
    }
}

func TestHandler_CreateUser_ContentType(t *testing.T) {
    h := handlers.NewHandler(&fakeUserService{}, discardLogger())

    req := httptest.NewRequest(http.MethodPost, "/users",
        strings.NewReader(`{"email":"test@example.com"}`))
    req.Header.Set("Content-Type", "text/plain") // wrong content type
    w := httptest.NewRecorder()
    h.CreateUser(w, req)

    assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
}
```

---

## 6. Contract Tests (Producer → Consumer Schema Compatibility)

```go
// shared/kafka/contract_test.go
// Verifies that what IAM produces is parseable by the audit consumer.
func TestContract_IAMEventEnvelope_ParseableByAuditConsumer(t *testing.T) {
    // Produce: build the envelope as IAM would
    produced := models.EventEnvelope{
        ID:         uuid.New().String(),
        Type:       "user.created",
        OrgID:      "org-1",
        ActorID:    "user-1",
        ActorType:  "user",
        OccurredAt: time.Now().UTC(),
        Source:     "iam",
        SchemaVer:  "1.0",
        Idempotent: uuid.New().String(),
        Payload:    json.RawMessage(`{"id":"user-1","email":"test@example.com"}`),
    }

    raw, err := json.Marshal(produced)
    require.NoError(t, err)

    // Consume: parse as the audit consumer would
    var consumed models.EventEnvelope
    require.NoError(t, json.Unmarshal(raw, &consumed))

    assert.Equal(t, produced.ID, consumed.ID)
    assert.Equal(t, produced.Type, consumed.Type)
    assert.Equal(t, produced.SchemaVer, consumed.SchemaVer)

    // Consumer validates schema version before processing
    assert.Equal(t, "1.0", consumed.SchemaVer)
    assert.NotEmpty(t, consumed.Idempotent) // required for dedup
}

func TestContract_PolicyEvaluateRequest(t *testing.T) {
    // Verify the SDK's request format matches what the policy service expects
    req := sdk.PolicyRequest{
        UserID:   "user-1",
        OrgID:    "org-1",
        Action:   "documents:read",
        Resource: "document/doc-123",
    }
    raw, err := json.Marshal(req)
    require.NoError(t, err)

    var svcReq policy.EvaluateRequest
    require.NoError(t, json.Unmarshal(raw, &svcReq))
    assert.Equal(t, req.UserID, svcReq.UserID)
    assert.Equal(t, req.Action, svcReq.Action)
}
```

---

## 7. RLS Tests (Mandatory for Every New Table)

Every table with RLS needs at least these three tests:

```go
// pkg/repository/rls_test.go
func TestRLS_<Table>(t *testing.T) {
    pool := startPostgres(t, context.Background())

    t.Run("org A cannot read org B rows", func(t *testing.T) { ... })
    t.Run("empty org_id context returns zero rows, not an error", func(t *testing.T) {
        ctx := rls.WithOrgID(context.Background(), "") // empty org_id
        repo := repository.NewRepository(pool)
        rows, err := repo.List(ctx, repository.ListInput{})
        assert.NoError(t, err, "empty org_id should return no rows, not an error")
        assert.Empty(t, rows)
    })
    t.Run("openguard_app role cannot BYPASSRLS", func(t *testing.T) {
        // Connect as openguard_app, do not set app.org_id, verify zero rows returned
        // This ensures FORCE ROW LEVEL SECURITY is in effect
        ...
    })
}
```

---

## 8. Outbox Integration Test

```go
func TestOutbox_WrittenInSameTransaction(t *testing.T) {
    if testing.Short() {
        t.Skip()
    }
    ctx := context.Background()
    pool := startPostgres(t, ctx)
    repo := repository.NewRepository(pool)
    outboxWriter := outbox.NewWriter()
    svc := service.NewService(repo, outboxWriter, discardLogger())

    orgCtx := rls.WithOrgID(ctx, "org-test-1")
    user, err := svc.CreateUser(orgCtx, service.CreateUserInput{
        Email: "outbox@example.com",
    })
    require.NoError(t, err)

    // Verify: exactly one outbox record was written
    // Query directly as openguard_outbox role (BYPASSRLS) to see the record
    var count int
    err = pool.QueryRow(orgCtx,
        `SELECT COUNT(*) FROM outbox_records WHERE status='pending'`).Scan(&count)
    require.NoError(t, err)
    assert.Equal(t, 1, count, "expected exactly one pending outbox record")

    // Verify: outbox payload contains the user ID
    var payload []byte
    err = pool.QueryRow(orgCtx,
        `SELECT payload FROM outbox_records WHERE status='pending' LIMIT 1`).Scan(&payload)
    require.NoError(t, err)

    var envelope models.EventEnvelope
    require.NoError(t, json.Unmarshal(payload, &envelope))
    assert.Equal(t, "user.created", envelope.Type)
    assert.Equal(t, user.ID, envelope.ActorID)
    _ = user
}
```

---

## 9. CI Flags (Mandatory)

Every test run in CI must include these flags:

```bash
go test ./... \
    -race \            # race detector — mandatory
    -count=1 \         # disable test caching (always re-run)
    -coverprofile=coverage.out \
    -covermode=atomic \
    -timeout 5m        # fail fast on deadlocks

# Short mode skips containers (for fast feedback during development)
go test ./... -short -race
```

**Coverage gate (70% per package):**
```bash
go tool cover -func=coverage.out | awk '
    /^total:/ { next }
    { split($3, a, "%"); if (a[1]+0 < 70) {
        print "FAIL: " $1 " coverage " $3 " < 70%"; fail=1
    }}
    END { exit fail }
'
```

---

## 10. Test Helpers and Utilities

```go
// testhelpers/logger.go — in-package test helper, not in shared/
func discardLogger() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testhelpers/seed.go
func seedOrg(t *testing.T, ctx context.Context, pool *rls.OrgPool) *models.Org {
    t.Helper()
    // Insert org directly — bypasses service layer intentionally
    // (we're testing the repository, not the service)
    var org models.Org
    err := pool.QueryRow(
        rls.WithOrgID(ctx, ""), // empty = system operation
        `INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id, name, slug`,
        "Test Org "+uuid.New().String()[:8],
        "test-"+uuid.New().String()[:8],
    ).Scan(&org.ID, &org.Name, &org.Slug)
    require.NoError(t, err)
    t.Cleanup(func() {
        // Best-effort cleanup; container is destroyed after test anyway
        pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, org.ID)
    })
    return &org
}
```

---

## 11. Testing Checklist

Before submitting any test:

- [ ] Tests are in `package foo_test` (external) — no unexported symbol access
- [ ] Table-driven for multiple inputs (min 3 cases: happy path, not found, error)
- [ ] `require` for fatal assertions, `assert` for non-fatal
- [ ] `testing.Short()` guard on all tests that start containers
- [ ] Integration tests use `testcontainers-go` — no mocking of DB behavior
- [ ] RLS isolation verified: org A cannot read org B data
- [ ] Outbox write verified: state change → exactly one outbox record
- [ ] No `time.Sleep` in tests — use `require.Eventually` with a timeout
- [ ] No `t.Parallel()` in integration tests that share containers (race on cleanup)
- [ ] Contract tests in `shared/` verify producer↔consumer schema compatibility
- [ ] Coverage ≥ 70% per package (CI enforced)
- [ ] All tests pass under `-race` flag
