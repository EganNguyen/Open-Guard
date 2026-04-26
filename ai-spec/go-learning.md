# Go Mastery Guide

A progressive, production-grounded reference for mastering Go — structured around real-world patterns from a security microservices codebase.

---

## Table of Contents

1. [Pointers](#1-pointers)
2. [Channels](#2-channels)
3. [The `context` Package](#3-the-context-package)
4. [`make` vs `new`](#4-make-vs-new)
5. [Slices](#5-slices)
6. [Loops](#6-loops)
7. [Maps](#7-maps)
8. [Strings](#8-strings)
9. [Structs](#9-structs)
10. [Functions & Methods](#10-functions--methods)
11. [Pointer Receivers (Deep Dive)](#11-pointer-receivers-deep-dive)
12. [Interfaces](#12-interfaces)
13. [Error Handling](#13-error-handling)
14. [`defer`, `panic`, `recover`](#14-defer-panic-recover)
15. [Database Access](#15-database-access)
16. [Logging with `log/slog`](#16-logging-with-logslog)
17. [Real-Time Communication](#17-real-time-communication)
18. [Concurrency Patterns](#18-concurrency-patterns)
19. [Goroutine Management](#19-goroutine-management)
20. [Testing](#20-testing)
21. [Performance & Profiling](#21-performance--profiling)
22. [Packages, Modules & Project Structure](#22-packages-modules--project-structure)
23. [Go Memory Model](#23-go-memory-model)

---

## 1. Pointers

### 1.1 Basics

A pointer stores the **memory address** of a value rather than the value itself.

| Syntax | Meaning |
|--------|---------|
| `*T` | Type: a pointer to a value of type `T` |
| `&x` | Address-of: produces a `*T` pointing to `x` |
| `*p` | Dereference: reads/writes the value at address `p` |

```go
name := "Go"
ptr := &name       // ptr is *string
fmt.Println(*ptr)  // "Go"
*ptr = "Golang"
fmt.Println(name)  // "Golang" — original variable mutated
```

### 1.2 Three Reasons to Use Pointers

**A. Pointer Receivers — mutating a struct in a method**

```go
// Pointer receiver: all methods share the same pool instance
func (r *Repository) ListPolicies(ctx context.Context, orgID string) ([]Policy, error) {
    // r.pool is the shared connection pool — no copy made
}
```

**B. Optional / nullable values**

Pointers are the only way to express "this value may be absent" for structs in Go.

```go
func (r *Repository) GetPolicy(ctx context.Context, id string) (*Policy, error) {
    var p Policy
    if err := conn.QueryRow(ctx, query, id).Scan(&p); err != nil {
        return nil, err // caller can check for nil
    }
    return &p, nil
}
```

**C. Avoiding copies of large structs**

```go
type Repository struct {
    pool *pgxpool.Pool // pointer to the shared DB pool — not copied
}
```

### 1.3 Differences from C/C++

- **No pointer arithmetic** — you cannot do `ptr++`.
- **Escape analysis** — the compiler automatically promotes stack variables to the heap when their address is returned or captured, keeping the pointer valid.
- **Nil panics** — dereferencing a `nil` pointer causes a runtime panic. Always guard: `if p != nil { ... }`.

### 1.4 Value vs. Pointer Receivers — Summary

| | Value Receiver `(t T)` | Pointer Receiver `(t *T)` |
|---|---|---|
| **Mutates original?** | ❌ No | ✅ Yes |
| **Copy cost** | Full struct copy | 8-byte address copy |
| **Nil safe?** | ✅ Always | ⚠️ Must check for nil |
| **Best for** | Small, immutable data | Large structs, stateful types |

> ⚠️ **Common Pitfall:** If any method on a type uses a pointer receiver, all methods should use pointer receivers for consistency, especially when implementing interfaces.

---

## 2. Channels

Go's concurrency mantra: *"Do not communicate by sharing memory; share memory by communicating."*

### 2.1 Core Use Cases

**Worker Pool / Task Distribution**

A fixed number of goroutines pulls jobs from a shared channel, preventing unbounded goroutine spawning.

```go
type bcryptCompareJob struct {
    hash   []byte
    plain  []byte
    result chan error // one-shot result channel (buffered!)
}

func (p *AuthWorkerPool) worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case job, ok := <-p.compareJobs:
            if !ok { return }
            job.result <- bcrypt.CompareHashAndPassword(job.hash, []byte(job.plain))
        }
    }
}
```

**Bounded Concurrency (Semaphore)**

A buffered channel limits how many goroutines perform an operation simultaneously.

```go
refreshSem := make(chan struct{}, 100) // max 100 concurrent refreshes

// Acquire
refreshSem <- struct{}{}
go func() {
    defer func() { <-refreshSem }() // Release
    doRefresh(ctx)
}()
```

**Graceful Shutdown (Signal Handling)**

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
<-sigCh
cancel() // propagate shutdown to all goroutines
```

**Async Result (Future Pattern)**

Embed a reply channel inside the job struct so the caller blocks only on its own result.

```go
job := bcryptCompareJob{
    hash:   storedHash,
    plain:  password,
    result: make(chan error, 1), // buffered: worker never blocks
}
p.compareJobs <- job
err := <-job.result
```

**Wakeup Signaling (Event Multiplexing)**

Combine a ticker with a notification channel for low-latency, non-polling event handling.

```go
for {
    select {
    case <-ctx.Done():
        return
    case <-ticker.C:
        relay.publishPending(ctx)
    case <-notifications: // triggered by Postgres NOTIFY
        relay.publishPending(ctx)
    }
}
```

### 2.2 Buffered vs. Unbuffered

| | Unbuffered `make(chan T)` | Buffered `make(chan T, n)` |
|---|---|---|
| **Synchronization** | Tight — sender and receiver rendezvous | Loose — sender outpaces receiver by up to `n` |
| **Blocks on send** | Immediately | Only when full |
| **Blocks on receive** | Immediately | Only when empty |
| **Typical use** | Stop/quit signals, lifecycle control | Worker pools, semaphores, async logging |

> 🧠 **Mental Model:** An unbuffered channel is a handshake. A buffered channel is a mailbox.

> ⚠️ **Common Pitfall — Goroutine Leak:** In a worker pool, the result channel for a one-shot job **must** be buffered with capacity 1. If the caller cancels before reading, an unbuffered result channel causes the worker goroutine to block forever.

```go
result := make(chan error, 1) // ✅ buffer of 1 prevents leak
// NOT: make(chan error)       // ❌ leaks worker if caller times out
```

### 2.3 Best Practices

- The goroutine (or component) that **creates** a channel should be the one to **close** it.
- Always use `select` with `ctx.Done()` on any potentially blocking channel operation.
- Use `select { case ch <- v: default: }` (non-blocking send with drop) for telemetry/logging channels where dropping is preferable to blocking.

---

## 3. The `context` Package

`context.Context` carries three things across API and goroutine boundaries:

1. **Cancellation signals** — tell goroutines to stop work.
2. **Deadlines / Timeouts** — auto-cancel if an operation takes too long.
3. **Request-scoped values** — metadata like `OrgID`, `RequestID`, `UserID`.

### 3.1 Passing Values with `WithValue`

Use typed, private keys to avoid collisions between packages.

```go
// shared/rls/context.go
type contextKey struct{} // unexported type — collision-safe key

func WithOrgID(ctx context.Context, orgID string) context.Context {
    return context.WithValue(ctx, contextKey{}, orgID)
}

func OrgIDFromCtx(ctx context.Context) (string, bool) {
    id, ok := ctx.Value(contextKey{}).(string)
    return id, ok
}
```

**Applied to Row-Level Security (RLS):**

```go
// Middleware injects org ID; repository enforces it at the DB level
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
    return err
}
```

### 3.2 Timeouts and Deadlines

```go
// Every HTTP handler downstream gets a 5-second budget
r.Use(middleware.Timeout(5 * time.Second))

// Explicit timeout for a DB operation
ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
defer cancel()
rows, err := conn.Query(ctx, query)
```

When the deadline expires, any context-aware library (`pgx`, `net/http`, `redis`) immediately aborts and returns an error.

### 3.3 Background Contexts

Use `context.Background()` for tasks that must **not** be cancelled when the parent request finishes.

```go
// Cache update runs independently of the HTTP request lifecycle
go lookup.CacheSet(context.Background(), keyHash, result, ttl)
```

### 3.4 Context API Reference

| Function | Returns | Use Case |
|---|---|---|
| `context.Background()` | Root context | Service entry point, background goroutines |
| `context.WithValue(ctx, k, v)` | Child context | Inject request-scoped metadata |
| `context.WithTimeout(ctx, d)` | Child context + cancel | DB queries, external API calls |
| `context.WithDeadline(ctx, t)` | Child context + cancel | Absolute expiry times |
| `context.WithCancel(ctx)` | Child context + cancel | Manual cancellation (graceful shutdown) |
| `context.TODO()` | Placeholder | Refactoring in progress |

### 3.5 Best Practices

- `ctx context.Context` is always the **first parameter** of any function that performs I/O.
- Never store a context inside a struct; pass it as a parameter.
- In long-running loops, check `ctx.Err()` at the top of each iteration.

---

## 4. `make` vs `new`

### 4.1 Comparison

| | `new(T)` | `make(T, ...)` |
|---|---|---|
| **Returns** | `*T` (pointer to zero value) | `T` (initialized value) |
| **Applies to** | Any type | Slices, Maps, Channels **only** |
| **Initialization** | Zero-value memory only | Allocates + initializes internal structure |

### 4.2 `new(T)`

```go
u := new(User)     // equivalent to &User{}
// u.ID == ""      // zero value
```

In practice, prefer **composite literals** over `new`:

```go
u := &User{ID: "abc", Name: "Alice"} // more readable, allows field initialization
```

### 4.3 `make(T, ...)`

Required for slices, maps, and channels because they have internal structure that must be initialized.

```go
salt := make([]byte, 32)           // pre-allocated byte buffer
cache := make(map[string]int)      // usable map
jobs := make(chan Job, 10)         // buffered channel
```

### 4.4 The Map Gotcha

```go
// ❌ Panics: new returns a pointer to a nil map
m := new(map[string]int)
(*m)["key"] = 10 // panic: assignment to entry in nil map

// ✅ Works: make initializes the map's hash table
m := make(map[string]int)
m["key"] = 10
```

> 🧠 **Rule of thumb:** Use `make` for slices, maps, and channels. Use `&T{}` composite literals instead of `new`.

---

## 5. Slices

Slices are the primary collection type in Go — a window into an underlying array with a length and capacity.

### 5.1 Domain Object Collections

```go
func (s *Service) evaluate(req EvaluateRequest, policies []Policy) (string, []string) {
    var matchedIDs []string // nil slice — valid starting point
    for _, p := range policies {
        if matches(req, p) {
            matchedIDs = append(matchedIDs, p.ID) // grows dynamically
        }
    }
    return decision, matchedIDs
}
```

### 5.2 Pre-allocated Byte Buffers (Crypto)

When size is known in advance, pre-allocate to avoid repeated reallocations.

```go
salt := make([]byte, 32)
if _, err := rand.Read(salt); err != nil {
    return nil, err
}
```

### 5.3 Batch Processing

Reset a slice without releasing its underlying memory by re-slicing to zero length.

```go
var batch []ComplianceEvent
for {
    select {
    case msg := <-c.input:
        batch = append(batch, msg)
        if len(batch) >= c.batchSize {
            c.flush(batch)
            batch = batch[:0] // ✅ clears length, keeps capacity
        }
    }
}
```

### 5.4 Mapping Between Types

```go
var scimUsers []ScimUser
for _, u := range dbUsers {
    scimUsers = append(scimUsers, mapToScim(u))
}
```

### 5.5 Slice Idioms Summary

| Idiom | Purpose |
|---|---|
| `var s []T` | Nil slice — safe to append, marshals as `null` |
| `s := []T{}` | Empty slice — marshals as `[]` |
| `make([]T, n)` | Pre-allocated, all zero values |
| `make([]T, 0, cap)` | Known capacity, no initial elements |
| `s = append(s, v)` | Dynamic growth |
| `s = s[:0]` | Reset length, keep capacity (batch reuse) |
| `copy(dst, src)` | Copy between slices |

> ⚠️ **Common Pitfall:** Slices share underlying arrays. Modifying a sub-slice (`s[1:3]`) modifies the original. Use `copy` when isolation is needed.

---

## 6. Loops

### 6.1 `for range` Slices

```go
for _, network := range blockedCIDRs {
    if network.Contains(ip) {
        return true
    }
}
```

### 6.2 `for range` Maps

```go
for k, v := range c.data {
    if time.Now().After(v.expiresAt) {
        delete(c.data, k)
    }
}
```

> ⚠️ Map iteration order is randomized in Go. Never rely on order.

### 6.3 Infinite Loops (Daemons)

```go
for {
    if ctx.Err() != nil { return } // always check context first
    conn, err := pool.Acquire(ctx)
    if err != nil {
        time.Sleep(5 * time.Second)
        continue
    }
    // ... work ...
}
```

### 6.4 Concurrency Event Loops

The `for { select { ... } }` pattern is the standard idiom for goroutines that handle multiple event sources.

```go
func (p *AuthWorkerPool) worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case job, ok := <-p.compareJobs:
            if !ok { return }
            job.result <- bcrypt.CompareHashAndPassword(job.hash, []byte(job.plain))
        }
    }
}
```

### 6.5 Database Iterators

```go
rows, err := conn.Query(ctx, query, args...)
if err != nil {
    return nil, err
}
defer rows.Close()

for rows.Next() {
    var rec Record
    if err := rows.Scan(&rec.ID, &rec.Name); err != nil {
        continue
    }
    records = append(records, rec)
}
return records, rows.Err()
```

> ⚠️ Always check `rows.Err()` after the loop — it surfaces errors that occurred during iteration.

### 6.6 Counter Loops

```go
for i := 0; i < 30; i++ {
    if err := db.Ping(); err == nil {
        break
    }
    time.Sleep(1 * time.Second)
}
```

---

## 7. Maps

### 7.1 Dynamic JSON Payloads

```go
payload, _ := json.Marshal(map[string]string{
    "org_id":     orgID,
    "event_type": "policy.updated",
})
```

### 7.2 Thread-Safe In-Memory State

Go maps are **not** goroutine-safe. Always protect with a mutex.

```go
type RateLimiter struct {
    ips map[string]*entry
    mu  sync.Mutex
}

func (rl *RateLimiter) check(ip string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    // ... read/write rl.ips ...
}
```

For read-heavy workloads, prefer `sync.RWMutex`:

```go
type LocalCache struct {
    data map[string]cacheEntry
    mu   sync.RWMutex
}

func (c *LocalCache) Get(key string) (cacheEntry, bool) {
    c.mu.RLock()         // multiple concurrent readers allowed
    defer c.mu.RUnlock()
    v, ok := c.data[key]
    return v, ok
}

func (c *LocalCache) Set(key string, val cacheEntry) {
    c.mu.Lock()          // exclusive write lock
    defer c.mu.Unlock()
    c.data[key] = val
}
```

### 7.3 Existence Check ("comma ok")

```go
val, exists := m[key]
if !exists {
    // key is absent
}
```

### 7.4 Best Practices

| Use Case | Pattern | Note |
|---|---|---|
| Dynamic JSON | `map[string]string` or `map[string]interface{}` | Avoid for typed data with known shape — use a struct instead |
| Shared state | `map` + `sync.Mutex` | Maps are never goroutine-safe |
| Read-heavy cache | `map` + `sync.RWMutex` | Allows concurrent reads |
| Memory hygiene | `delete(m, key)` in background goroutine | Prevents long-running services from leaking memory |

---

## 8. Strings

### 8.1 Byte/String Conversions

```go
// []byte → string (for storage)
userID, _ := s.repo.CreateUser(ctx, string(hash))

// string → []byte (for cryptographic processing)
return totp.Validate(code, string(secretBytes)), nil
```

### 8.2 Slugification

```go
slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + uuid.New().String()[:8]
```

### 8.3 Composite String Parsing

Encode structured data into a delimited string; split it back out on the other side.

```go
// Encode: "<kid>:<base64ciphertext>"
encoded := fmt.Sprintf("%s:%s", activeKey.Kid, base64.StdEncoding.EncodeToString(ciphertext))

// Decode:
parts := strings.Split(encoded, ":")
kid, payload := parts[0], parts[1]
```

### 8.4 UTF-8 Rune Iteration

When you need to iterate over **characters** (not bytes), use `for range` over a string:

```go
for i, r := range "héllo" {
    fmt.Printf("index=%d rune=%c\n", i, r)
}
```

### 8.5 Essential `strings` Package Functions

| Function | Purpose |
|---|---|
| `strings.ToLower(s)` | Normalize for comparison |
| `strings.TrimSpace(s)` | Strip leading/trailing whitespace |
| `strings.Contains(s, sub)` | Substring check |
| `strings.HasPrefix(s, p)` | Prefix check |
| `strings.Split(s, sep)` | Tokenize |
| `strings.ReplaceAll(s, old, new)` | Global substitution |
| `strings.Builder` | Efficient string concatenation in loops |

---

## 9. Structs

### 9.1 Domain Entities

Flat structs with exported fields and struct tags for JSON and DB mapping.

```go
type Policy struct {
    ID          string          `json:"id"`
    OrgID       string          `json:"org_id"`
    Name        string          `json:"name"`
    Logic       json.RawMessage `json:"logic"` // flexible: defer parsing
    Version     int             `json:"version"`
    CreatedAt   time.Time       `json:"created_at"`
    UpdatedAt   time.Time       `json:"updated_at"`
}
```

### 9.2 Service / Repository Aggregators

Dependencies are unexported; construction is done via a factory function.

```go
type Service struct {
    repo      *repository.Repository
    rdb       *redis.Client
    logger    *slog.Logger
    logCh     chan evalLogEntry
    sfGroup   singleflight.Group
}

func NewService(repo *repository.Repository, rdb *redis.Client, logger *slog.Logger) *Service {
    s := &Service{
        repo:   repo,
        rdb:    rdb,
        logger: logger,
        logCh:  make(chan evalLogEntry, 1000),
    }
    go s.logWorker()
    return s
}
```

### 9.3 Request / Response DTOs

```go
type EvaluateRequest struct {
    OrgID      string   `json:"org_id"`
    SubjectID  string   `json:"subject_id"`
    UserGroups []string `json:"user_groups"`
    Action     string   `json:"action"`
    Resource   string   `json:"resource"`
}

type EvaluateResponse struct {
    Effect           string   `json:"effect"`
    MatchedPolicyIDs []string `json:"matched_policy_ids"`
    LatencyMs        int      `json:"latency_ms"`
}
```

### 9.4 Anonymous / Nested Structs

Use anonymous structs for one-off, single-use decoding to avoid polluting the package namespace.

```go
var body struct {
    Schemas    []string          `json:"schemas"`
    Operations []service.PatchOp `json:"Operations"`
}
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
    http.Error(w, "bad request", http.StatusBadRequest)
    return
}
```

### 9.5 Concurrency Structs

Bundle request and response together for channel-based async processing.

```go
type evalLogEntry struct {
    req  EvaluateRequest
    resp *EvaluateResponse
}

// Non-blocking send with drop on full buffer
select {
case s.logCh <- evalLogEntry{req, resp}:
default:
    s.logger.Warn("eval log channel full, dropping entry")
}
```

### 9.6 Best Practices Summary

| Concern | Pattern |
|---|---|
| Exported fields | JSON/DB mapping |
| Unexported fields | Internal service state |
| Pointer receivers | All service and repository methods |
| Struct tags | `json:"..."` for API contracts; `db:"..."` for DB scanning |
| Construction | `NewXxx(deps...) *Xxx` factory functions |
| Polymorphic data | `json.RawMessage` for fields with variable schemas |

---

## 10. Functions & Methods

### 10.1 Functions vs. Methods

The only syntactic difference is the **receiver** — a method has one, a function does not.

```go
// Function — stateless, no receiver
func NewRepository(pool *pgxpool.Pool) *Repository {
    return &Repository{pool: pool}
}

// Method — stateful, has receiver
func (r *Repository) ListPolicies(ctx context.Context, orgID string) ([]Policy, error) {
    // r.pool is accessible via the receiver
}
```

**Use a function when:** the logic is a standalone utility or a constructor (`New...`).
**Use a method when:** the logic needs struct fields, or you need to satisfy an interface.

### 10.2 Higher-Order Functions

Functions passed as arguments enable the "execute-around" pattern — setup, execute callback, tear down.

```go
// withOrgContext wraps any DB operation in RLS setup/teardown
func (r *Repository) withOrgContext(ctx context.Context, fn func(ctx context.Context, conn *pgxpool.Conn) error) error {
    conn, err := r.pool.Acquire(ctx)
    if err != nil { return err }
    defer conn.Release()

    if err := rls.SetSessionVar(ctx, conn, orgIDFromCtx(ctx)); err != nil {
        return err
    }
    return fn(ctx, conn)
}

// Usage: anonymous closure captures outer variables
func (r *Repository) GetPolicy(ctx context.Context, id string) (*Policy, error) {
    var p Policy
    err := r.withOrgContext(ctx, func(ctx context.Context, conn *pgxpool.Conn) error {
        return conn.QueryRow(ctx, `SELECT ... FROM policies WHERE id=$1`, id).Scan(&p.ID, &p.Name)
    })
    return &p, err
}
```

### 10.3 Closures & Goroutines

Anonymous functions capture variables from their enclosing scope.

```go
go func() {
    if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        logger.Error("server failed", "error", err)
    }
}()
```

> ⚠️ **Common Pitfall — Loop Variable Capture:** Before Go 1.22, closures in loops captured the loop variable by reference. The fix was to shadow it: `i := i` or pass it as a parameter.

### 10.4 Variadic Functions

```go
func (r *Repository) getUser(ctx context.Context, where string, args ...interface{}) (map[string]interface{}, error) {
    query := `SELECT * FROM users WHERE ` + where
    return conn.QueryRow(ctx, query, args...).Scan(...)
}

// Call sites:
r.getUser(ctx, "email = $1", "alice@example.com")
r.getUser(ctx, "id = $1 AND status = $2", userID, "active")
```

### 10.5 Function Fields (Strategy Pattern)

Store a function in a struct field for pluggable behavior.

```go
type RegexDetector struct {
    Name     string
    Pattern  *regexp.Regexp
    Validate func(string) bool // swappable validation logic
}

cc := RegexDetector{
    Name:     "CreditCard",
    Pattern:  regexp.MustCompile(`\d{16}`),
    Validate: func(match string) bool { return luhnCheck(match) },
}
```

### 10.6 HTTP Middleware (Function Adapter)

```go
func TracingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // pre-handler logic
        next.ServeHTTP(w, r)
        // post-handler logic
    })
}
```

---

## 11. Pointer Receivers (Deep Dive)

### 11.1 When Pointer Receivers Are Mandatory

**Concurrency primitives** — copying a `sync.Mutex`, `sync.WaitGroup`, or channel breaks them. Always use pointer receivers for types containing these.

```go
// ✅ Pointer receiver — sfGroup and logCh are shared
func (s *Service) Evaluate(ctx context.Context, req EvaluateRequest) (*EvaluateResponse, error) {
    ch := s.sfGroup.DoChan(cacheKey(req), func() (interface{}, error) {
        return s.evaluateFromDB(ctx, req)
    })
    // ...
}
```

**State mutation** — when a method updates internal buffers or counters.

```go
func (w *ClickHouseWriter) flush(batch []ComplianceEvent) {
    w.buffer = w.buffer[:0] // modifies the receiver's field
    // ...
}
```

### 11.2 Architectural Consistency Rule

If one method on a type requires a pointer receiver, all methods should use pointer receivers. This prevents subtle bugs when types are used through interfaces.

### 11.3 When Value Receivers Are Appropriate

Value receivers are reserved for small, immutable value objects where copying is safe and intentional — typically simple helper methods on config structs with no side effects.

### 11.4 Usage Pattern Summary

| Component | Receiver | Reason |
|---|---|---|
| `Handler` | `*Handler` | Shared service + logger dependencies |
| `Service` | `*Service` | Contains channels, singleflight, mutexes |
| `Repository` | `*Repository` | Shared DB connection pool |
| `ClickHouseWriter` | `*ClickHouseWriter` | Mutable internal buffer |
| Data DTOs (e.g., `Policy`) | Value | Pure data, no behavior |

---

## 12. Interfaces

Go interfaces are satisfied **implicitly** — no `implements` keyword. A type satisfies an interface by having all the required methods.

### 12.1 Repository (Persistence Layer)

Decouples service logic from storage implementations, enabling easy mock substitution in tests.

```go
type Repository interface {
    CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role, status string) (string, error)
    GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error)
    CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error
    UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secretEncrypted string) error
}
```

### 12.2 Security / Auth

```go
type APIKeyLookup interface {
    FindByPrefix(ctx context.Context, prefix string) (connectorID, orgID, pbkdf2Hash string, err error)
    CacheGet(ctx context.Context, keyHash string) (*APIKeyAuthResult, error)
    CacheSet(ctx context.Context, keyHash string, result *APIKeyAuthResult, ttl time.Duration) error
}

type SecretsProvider interface {
    GetSecret(ctx context.Context, secretID string) (string, error)
}
```

### 12.3 Messaging

```go
type KafkaPublisher interface {
    Publish(ctx context.Context, topic, key string, payload []byte) error
}
```

### 12.4 Saga Orchestration

```go
type UserStatusUpdater interface {
    UpdateUserStatus(ctx context.Context, userID, status string) error
}
```

### 12.5 Third-Party Integration (webauthn)

```go
// Implements the go-webauthn library's required interface
type WebAuthnUser struct {
    id          []byte
    displayName string
    credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte              { return u.id }
func (u *WebAuthnUser) WebAuthnName() string            { return u.displayName }
func (u *WebAuthnUser) WebAuthnDisplayName() string     { return u.displayName }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }
```

### 12.6 Key Design Principles

- Define interfaces in the **consumer** package (the package that uses the dependency), not the producer package.
- Keep interfaces small — the `io.Reader` philosophy. One or two methods is ideal.
- Interface satisfaction is checked at compile time when you assign: `var _ Repository = (*PostgresRepository)(nil)`.

---

## 13. Error Handling

### 13.1 Errors as Values

Go has no exceptions. Every fallible function returns an `error` as its last return value.

```go
user, err := s.repo.GetUserByID(ctx, userID)
if err != nil {
    return nil, err
}
```

### 13.2 Error Wrapping with `%w`

Add context to errors as they bubble up through layers. This creates an **error chain** that can be inspected later.

```go
if err := s.repo.StoreBackupCodes(ctx, userID, hashes); err != nil {
    return nil, fmt.Errorf("store backup codes: %w", err)
}
// Error message: "store backup codes: connection refused"
```

### 13.3 Sentinel Errors and `errors.Is` / `errors.As`

Define package-level sentinel errors for conditions callers must handle.

```go
var ErrCircuitOpen = errors.New("circuit breaker open")

// Wrap a sentinel error with context
if errors.Is(err, gobreaker.ErrOpenState) {
    return zero, fmt.Errorf("%w: %s", ErrCircuitOpen, cb.Name())
}

// Caller checks:
if errors.Is(err, ErrCircuitOpen) {
    // handle specifically
}
```

### 13.4 Fail-Closed for Security

In security systems, when a critical check fails, **deny by default**.

```go
policies, err := resilience.Call(ctx, func() ([]Policy, error) {
    return s.repo.ListPolicies(ctx, req.OrgID)
})
if err != nil {
    s.logger.Error("policy fetch failed, denying request", "error", err)
    return &EvaluateResponse{Effect: "deny"}, nil // safe default
}
```

### 13.5 HTTP Error Mapping

Translate internal errors into HTTP status codes at the handler boundary.

```go
user, err := h.svc.Login(ctx, email, password)
if err != nil {
    if errors.Is(err, service.ErrProvisioningInProgress) {
        h.writeJSON(w, http.StatusForbidden, map[string]string{"error": "provisioning in progress"})
        return
    }
    http.Error(w, "invalid credentials", http.StatusUnauthorized)
    return
}
```

### 13.6 Async Error Handling

Background goroutines are detached from the request lifecycle — log errors rather than returning them.

```go
func (s *Service) processWriteEvalLog(entry evalLogEntry) {
    if err := s.repo.WriteEvalLog(ctx, entry); err != nil {
        s.logger.Error("failed to write eval log", "error", err)
        // no return value — caller is a goroutine
    }
}
```

### 13.7 Best Practices Summary

| Pattern | Implementation | Benefit |
|---|---|---|
| Wrapping | `fmt.Errorf("context: %w", err)` | Preserves error chain; adds call-site context |
| Sentinel errors | `var ErrNotFound = errors.New(...)` | Programmatic error type checking |
| Type checking | `errors.Is`, `errors.As` | Works correctly through wrapped errors |
| Fail-closed | Return safe default on security check failure | Denies access when uncertain |
| Async errors | Log with `slog.Error(...)` | Observability without propagation |

---

## 14. `defer`, `panic`, `recover`

### 14.1 `defer` — Resource Safety

`defer` runs a statement when the surrounding function returns, regardless of whether it returns normally or via a panic. It is the standard tool for cleanup.

**Database connections and rows:**

```go
rows, err := conn.Query(ctx, query, args...)
if err != nil { return nil, err }
defer rows.Close() // always closed, even if the loop errors
```

**Transaction rollback safety net:**

```go
tx, err := s.repo.Begin(ctx)
if err != nil { return err }
defer tx.Rollback(ctx) // no-op if tx.Commit() already succeeded

if err := s.repo.UpdateUser(ctx, tx, ...); err != nil {
    return err // rollback fires here
}
return tx.Commit(ctx)
```

**Restoring a database role after elevation:**

```go
_, err = conn.Exec(ctx, "SET ROLE openguard_login")
defer func() { _, _ = conn.Exec(ctx, "RESET ROLE") }()
```

> 🧠 `defer` is LIFO — multiple defers execute in reverse order of declaration.

### 14.2 `panic` — Fail-Fast Initialization

Use `panic` **only during startup** for unrecoverable configuration errors. Never panic in request handlers or business logic.

```go
// Missing required configuration — service cannot function
if dbURL == "" {
    panic("required env var DATABASE_URL not set")
}

// Nil dependency injected — programming error
if targetURL == "" {
    panic("proxy: target URL is required")
}
```

### 14.3 `recover` — Worker Resilience

`recover` stops a panic from crashing the process. It must be called inside a `defer` to work.

```go
// In a Kafka consumer goroutine:
go func(msg kafka.Message) {
    defer wg.Done()
    defer func() {
        if r := recover(); r != nil {
            c.logger.Error("panic in message processing", "recovered", r, "key", string(msg.Key))
            // consumer loop continues — message is not re-queued (or routed to DLQ)
        }
    }()

    if err := c.processMessage(ctx, msg); err != nil {
        c.logger.Error("processing failed", "error", err)
    }
}(m)
```

### 14.4 Usage Summary

| Feature | Primary Purpose | Frequency |
|---|---|---|
| `defer` | Resource cleanup, transaction safety, role reset | High — standard practice everywhere |
| `panic` | Fatal config/init errors only | Medium — startup phase only |
| `recover` | Prevent worker goroutines from crashing the service | Low — specific resilience points |

---

## 15. Database Access

### 15.1 PostgreSQL with `pgx/v5`

The `pgx` driver is used directly (no ORM) for high performance and fine-grained control.

**Multi-tenancy via Row-Level Security (RLS):**

```go
// shared/rls/context.go
func SetSessionVar(ctx context.Context, conn *pgxpool.Conn, orgID string) error {
    _, err := conn.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", orgID)
    return err
}

// Repository wrapper — every query goes through this
func (r *Repository) withOrgContext(ctx context.Context, fn func(*pgxpool.Conn) error) error {
    conn, err := r.pool.Acquire(ctx)
    if err != nil { return err }
    defer conn.Release()

    if err := rls.SetSessionVar(ctx, conn, orgIDFromCtx(ctx)); err != nil {
        return err
    }
    return fn(conn)
}
```

**Bypassing RLS for system-level operations:**

```go
// Login path needs cross-tenant user lookup
_, err = conn.Exec(ctx, "SET ROLE openguard_login")
defer func() { _, _ = conn.Exec(ctx, "RESET ROLE") }()
// ... query without RLS filter ...
```

### 15.2 MongoDB (Audit Storage)

Used for high-volume append-heavy audit logs where schema flexibility and bulk writes are valuable.

```go
// Read and write concerns are separated into distinct repository types
type AuditWriteRepository struct{ coll *mongo.Collection }
type AuditReadRepository  struct{ coll *mongo.Collection }

func (r *AuditReadRepository) FindEvents(ctx context.Context, orgID string) ([]map[string]interface{}, error) {
    // mongo-driver returns flexible documents
}
```

A **hash chain** pattern ensures audit records cannot be tampered with — each record contains a hash of the previous one.

### 15.3 Redis (Caching & Rate Limiting)

```go
// API key cache (tiered: Redis → DB → PBKDF2 verification)
result, err := rdb.Get(ctx, "apikey:"+keyHash).Result()
if errors.Is(err, redis.Nil) {
    // cache miss — fall through to DB lookup
}

// Session JTI storage for fast revocation
rdb.SAdd(ctx, "jtis:"+userID, jti)
rdb.Expire(ctx, "jtis:"+userID, sessionTTL)
```

### 15.4 Transactional Outbox Pattern

Atomically write both the domain change and the event to the DB in the same transaction, preventing message loss.

```go
func (r *Repository) CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error {
    _, err := tx.Exec(ctx,
        `INSERT INTO outbox_records (org_id, topic, key, payload) VALUES ($1, $2, $3, $4)`,
        orgID, topic, key, payload,
    )
    return err
}
```

### 15.5 Driver Summary

| Storage | Driver | Primary Use |
|---|---|---|
| PostgreSQL | `github.com/jackc/pgx/v5` | Core data, RLS, transactions |
| MongoDB | `go.mongodb.org/mongo-driver` | Audit logs, hash-chained records |
| Redis | `github.com/redis/go-redis/v9` | Cache, sessions, rate limits, Lua scripts |
| Kafka | `github.com/segmentio/kafka-go` | Async event delivery |

---

## 16. Logging with `log/slog`

`log/slog` (Go 1.21+) is the standard library's structured logging package. It produces machine-readable JSON logs enriched with context.

### 16.1 Centralized Logger Factory

```go
// shared/telemetry/logger.go
func NewLogger(serviceName string) *slog.Logger {
    opts := &slog.HandlerOptions{
        Level: slog.LevelInfo,
        ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
            key := strings.ToLower(a.Key)
            if key == "password" || key == "token" || key == "secret" || key == "api_key" {
                return slog.String(a.Key, "[REDACTED]")
            }
            return a
        },
    }
    handler := slog.NewJSONHandler(os.Stdout, opts).WithAttrs([]slog.Attr{
        slog.String("service", serviceName),
    })
    return slog.New(handler)
}
```

### 16.2 Service Initialization

```go
func main() {
    logger := telemetry.NewLogger("iam")
    slog.SetDefault(logger)
    logger.Info("service starting", "port", 8080)
}
```

### 16.3 Component Context

Use `.With()` to enrich a logger with component-specific metadata.

```go
sagaWatcher := saga.NewWatcher(rdb, kp, logger.With("component", "saga-watcher"))
```

### 16.4 Log Levels

| Level | When to Use |
|---|---|
| `Info` | Normal operational events — startup, migrations, requests |
| `Warn` | Recoverable issues — Redis check failed, using default config |
| `Error` | Critical failures requiring attention — DB connection lost |
| `Debug` | Verbose detail for development (disabled in production) |

---

## 17. Real-Time Communication

### 17.1 Server-Sent Events (SSE) — Frontend

```go
// audit/pkg/handlers/sse.go
func (h *Handler) StreamEvents(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")

    flusher := w.(http.Flusher)
    cs, _ := h.collection.Watch(r.Context(), pipeline)

    for cs.Next(r.Context()) {
        fmt.Fprintf(w, "data: %s\n\n", cs.Current.String())
        flusher.Flush() // push immediately — don't buffer
    }
}
```

### 17.2 Kafka (Inter-Service Events)

Services run continuous consumer loops that process messages as they arrive.

```go
for {
    msg, err := reader.FetchMessage(ctx) // blocks until message arrives
    if err != nil {
        if ctx.Err() != nil { return } // shutdown
        continue
    }
    if err := processMessage(ctx, msg); err != nil {
        routeToDLQ(ctx, msg, err)
        continue
    }
    reader.CommitMessages(ctx, msg) // commit only after success
}
```

### 17.3 PostgreSQL LISTEN/NOTIFY (Outbox Relay)

The outbox relay avoids polling by waking up instantly when new events are inserted.

```go
// relay.go
conn.Exec(ctx, "LISTEN outbox_new")

for {
    select {
    case <-ctx.Done(): return
    case <-ticker.C:          // fallback: poll every N seconds
        relay.publishPending(ctx)
    case <-notifications:     // instant: triggered by DB NOTIFY
        relay.publishPending(ctx)
    }
}
```

### 17.4 Real-Time Architecture Summary

| Layer | Technology | Latency | Component |
|---|---|---|---|
| Frontend | Server-Sent Events | Low | `audit` service |
| Inter-service | Kafka | Medium | `alerting`, `webhook-delivery` |
| DB → Kafka | PG LISTEN/NOTIFY | Sub-millisecond | `shared/kafka/outbox` |
| External | Webhooks (HTTP POST) | Variable | `webhook-delivery` |

---

## 18. Concurrency Patterns

### 18.1 Core In-Process Patterns

**Worker Pool** — fixed goroutines for CPU-bound work

```go
// Prevents goroutine explosion during peak login traffic
type AuthWorkerPool struct {
    compareJobs chan bcryptCompareJob
    generateJobs chan bcryptGenerateJob
}

func (p *AuthWorkerPool) Start(ctx context.Context, numWorkers int) {
    for i := 0; i < numWorkers; i++ {
        go p.worker(ctx)
    }
}
```

**Singleflight** — deduplicate concurrent identical requests

```go
// Only one DB query fires for a given policy key, even under heavy load
ch := s.sfGroup.DoChan(cacheKey, func() (interface{}, error) {
    return s.evaluateFromDB(ctx, req)
})

select {
case res := <-ch:
    return res.Val.(*sfResult), res.Err
case <-ctx.Done():
    return nil, ctx.Err()
}
```

**Producer-Consumer** — decouple ingestion from processing

```go
// Producer: non-blocking send; drops on full buffer
select {
case s.logCh <- evalLogEntry{req, resp}:
default:
    s.logger.Warn("log channel full, dropping")
}

// Consumer: background goroutine drains the channel
func (s *Service) logWorker() {
    for entry := range s.logCh {
        s.repo.WriteEvalLog(context.Background(), entry)
    }
}
```

**Fork-Join / Parallel Map** — process a batch concurrently

```go
var wg sync.WaitGroup
sem := make(chan struct{}, 50) // max 50 concurrent deliveries

for _, msg := range batch {
    wg.Add(1)
    sem <- struct{}{}
    go func(m kafka.Message) {
        defer wg.Done()
        defer func() { <-sem }()
        deliver(ctx, m)
    }(msg)
}
wg.Wait()
```

### 18.2 Distributed Concurrency Patterns

| Pattern | Mechanism | Purpose |
|---|---|---|
| **Saga Orchestration** | Kafka message passing | Multi-step distributed transactions |
| **Transactional Outbox** | DB + Kafka in same transaction | Guaranteed event delivery |
| **Optimistic Locking (CAS)** | `WHERE hash = $prev` SQL | Prevent concurrent audit chain corruption |
| **Pessimistic Claim** | `DELETE...RETURNING` / `GETDEL` | One-time token consumption |
| **Sliding Window** | Redis Sorted Sets | Brute-force detection, rate limiting |
| **Idempotency Gate** | Redis `SETNX` | Prevent duplicate event processing |
| **Distributed Lua Atomic** | Redis Lua script | Atomic read-check-update across replicas |

### 18.3 Resilience Patterns

| Pattern | Mechanism | Purpose |
|---|---|---|
| **Circuit Breaker** | `gobreaker` | Stop cascading failures when Redis/DB is unhealthy |
| **Retry + Exponential Backoff** | `select` + `time.After` | Handle transient errors for webhook delivery |
| **Dead Letter Queue (DLQ)** | Separate Kafka topic | Park permanently failing messages for inspection |
| **Fail-Closed** | Default deny on error | Security-critical decisions default to safe state |

---

## 19. Goroutine Management

Preventing goroutine leaks is as important as preventing memory leaks.

### 19.1 Root Context + OS Signal

Tie the root cancellation context to OS signals so the entire service can shut down cleanly.

```go
ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
defer cancel()

// All goroutines receive ctx; when SIGTERM fires, ctx.Done() closes
```

### 19.2 `sync.WaitGroup` for Orchestration

```go
var wg sync.WaitGroup

wg.Add(1)
go func() {
    defer wg.Done()
    runConsumer(ctx)
}()

<-ctx.Done()       // wait for shutdown signal
wg.Wait()          // wait for all goroutines to finish
```

### 19.3 Bounded Concurrency (Semaphore)

```go
sem := make(chan struct{}, 50)

go func() {
    sem <- struct{}{}          // Acquire — blocks when 50 are active
    defer func() { <-sem }()  // Release — runs when done
    doWork(ctx)
}()
```

### 19.4 Graceful HTTP Shutdown with Timeout

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := server.Shutdown(shutdownCtx); err != nil {
    logger.Error("server shutdown failed", "error", err)
}
```

### 19.5 Panic Recovery in Workers

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            logger.Error("worker panic recovered", "error", r)
        }
    }()
    processMessage(ctx, msg)
}()
```

### 19.6 Goroutine Lifecycle Summary

| Strategy | Tool | Benefit |
|---|---|---|
| Global shutdown signal | `context.WithCancel` + OS signals | Unified cancellation propagation |
| Worker synchronization | `sync.WaitGroup` | Clean exit of all workers |
| Resource throttling | `chan struct{}` semaphore | CPU/memory protection |
| Timeout on shutdown | `context.WithTimeout` | Prevent zombie processes in containers |
| Per-worker resilience | `defer recover()` | Single bad message doesn't crash the service |

---

## 20. Testing

### 20.1 Table-Driven Tests

The idiomatic Go testing pattern for validating multiple scenarios with a single test function.

```go
var globTests = []struct {
    patterns []string
    value    string
    want     bool
}{
    {[]string{"read:*"}, "read:documents", true},
    {[]string{"read:*"}, "write:documents", false},
    {[]string{"*"}, "anything", true},
    {[]string{}, "read:docs", false},
}

func TestMatchesGlob(t *testing.T) {
    for _, tt := range globTests {
        t.Run(fmt.Sprintf("%v/%s", tt.patterns, tt.value), func(t *testing.T) {
            got := matchesGlob(tt.patterns, tt.value)
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 20.2 Interface Mocks

Implement the interface manually with in-memory state for hermetic, database-free tests.

```go
type MockRepository struct {
    service.Repository // embed to auto-satisfy unimplemented methods
    Users        map[string]map[string]interface{}
    FailedLogins map[string]int
}

func (m *MockRepository) GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error) {
    for _, u := range m.Users {
        if u["email"] == email {
            return u, nil
        }
    }
    return nil, fmt.Errorf("user not found: %s", email)
}
```

### 20.3 Redis Mocking with `miniredis`

`miniredis` runs a real Redis-compatible in-memory server. Its `FastForward` capability is essential for testing time-window rate limits.

```go
func TestBruteForce_ResetsAfterWindow(t *testing.T) {
    mr, _ := miniredis.Run()
    defer mr.Close()

    rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    detector := NewBruteForceDetector(rdb)

    detector.trackFailedAttempt(ctx, "user:alice")
    mr.FastForward(6 * time.Minute) // simulate time passing

    ok, count := detector.CheckRateLimit(ctx, "user:alice")
    if !ok || count != 1 {
        t.Errorf("expected count 1 after window reset, got %d", count)
    }
}
```

### 20.4 Testing Tools Summary

| Tool | Purpose |
|---|---|
| `testing` | Standard library test runner |
| `t.Run(name, func)` | Sub-tests for table-driven cases |
| `miniredis` | In-memory Redis for integration-level tests |
| Interface embedding | Satisfy large interfaces with only the methods under test |
| Real crypto (`bcrypt`, `aes`) | Validate security logic end-to-end |

---

## 21. Performance & Profiling

### 21.1 Escape Analysis & Stack Allocations

The Go compiler performs **escape analysis** to decide whether a variable lives on the stack (fast, no GC) or heap (GC-managed). Variables that don't escape to other goroutines or are not returned as pointers stay on the stack.

```bash
# View escape analysis decisions
go build -gcflags="-m" ./...
```

**Prefer stack allocations in hot paths** by avoiding unnecessary pointers and keeping struct sizes small.

### 21.2 Pre-allocation

When you know the size of a slice or map in advance, allocate it upfront to avoid repeated reallocations.

```go
// ✅ Pre-allocated — no reallocation during loop
codes := make([]string, 8)
hashes := make([]string, 8)

// ✅ Pre-allocated map
result := make(map[string]int, len(input))
```

### 21.3 `sync.Pool` for Buffer Reuse

Reuse expensive allocations (e.g., byte buffers) across requests to reduce GC pressure.

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 4096) // 4KB reusable buffer
    },
}

func processPayload(data []byte) {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf[:0]) // reset length, return to pool

    buf = append(buf, data...)
    // ... process buf ...
}
```

### 21.4 Profiling with `pprof`

```go
// Add to main.go for on-demand profiling
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

| Profile Type | Endpoint | Finds |
|---|---|---|
| CPU | `/debug/pprof/profile?seconds=30` | Hot functions consuming execution time |
| Heap | `/debug/pprof/heap` | High-allocation areas and memory leaks |
| Goroutine | `/debug/pprof/goroutine` | Goroutine leaks (workers that never exit) |

```bash
go tool pprof http://localhost:6060/debug/pprof/heap
```

### 21.5 Performance Patterns Summary

| Technique | Implementation | Benefit |
|---|---|---|
| Worker Pool (CPU-bound) | `AuthWorkerPool` with fixed goroutines | Prevents CPU/memory exhaustion |
| Semaphore (IO-bound) | Buffered channel | Caps concurrent outbound requests |
| Singleflight | `golang.org/x/sync/singleflight` | Eliminates duplicate DB queries under load |
| Pre-allocation | `make([]T, n, cap)` | Reduces heap allocations in hot paths |
| Buffer reuse | `sync.Pool` | Reduces GC pressure for byte buffers |

---

## 22. Packages, Modules & Project Structure

### 22.1 Go Workspaces (`go.work`)

A monorepo containing multiple interdependent modules uses Go Workspaces to link them locally without publishing.

```
go.work          ← workspace root: lists all modules
services/
  iam/           ← go.mod: module github.com/openguard/services/iam
  policy/        ← go.mod: module github.com/openguard/services/policy
  ...
shared/          ← go.mod: module github.com/openguard/shared
sdk/             ← go.mod: module github.com/openguard/sdk
```

```go
// go.work
go 1.22

use (
    ./sdk
    ./shared
    ./services/iam
    ./services/policy
)
```

### 22.2 Service Structure (`pkg/` Layout)

Each service follows a consistent internal layout:

```
services/<name>/
  main.go              ← entry point: wires dependencies, starts server
  cmd/                 ← optional: secondary binaries (seeds, migrations)
  pkg/
    handlers/          ← HTTP: decode request, call service, encode response
    service/           ← business logic
    repository/        ← data access (DB queries)
    middleware/        ← service-specific HTTP middleware
    router/            ← route definitions and middleware chain
```

### 22.3 Key Standard Library Packages

| Package | Usage |
|---|---|
| `net/http` | HTTP server, handlers, middleware, cookies |
| `encoding/json` | Request/response serialization, Kafka payloads |
| `context` | Request lifecycle, cancellation, timeout propagation |
| `sync` | `Mutex`, `RWMutex`, `WaitGroup`, `Once`, `Pool` |
| `io` | `ReadFull` for cryptographic nonce generation |
| `log/slog` | Structured JSON logging |
| `errors` | `errors.Is`, `errors.As`, `errors.New`, `fmt.Errorf("%w", ...)` |
| `crypto/rand` | Cryptographically secure random bytes |
| `time` | Timeouts, expiry, ticker loops |

---

## 23. Go Memory Model

The Go Memory Model defines when a goroutine's write to a variable is guaranteed to be visible to another goroutine's read of the same variable. It is based on the **happens-before** relationship.

### 23.1 Core Rules

**Mutex:** `mu.Unlock()` happens-before the next `mu.Lock()` completes. Any writes made before unlocking are visible after locking.

**Channel send:** A send on a channel happens-before the corresponding receive completes. Closing a channel happens-before a receive that returns the zero value.

**`sync.WaitGroup`:** `wg.Done()` happens-before the return from `wg.Wait()` in another goroutine.

**`sync/atomic`:** Atomic operations provide sequentially consistent ordering for the specific variable.

### 23.2 Applied Synchronization Patterns

| Primitive | Guarantee | Use Case |
|---|---|---|
| `sync.Mutex` | `Unlock → Lock` visibility | Rate limiter visitor map |
| `sync.RWMutex` | `Unlock → RLock` visibility | SDK local cache (many readers, one writer) |
| Buffered channel | `Send → Receive` visibility | Logging queue (`logCh`), task dispatch |
| `chan struct{}` | `Send/Close → Receive` visibility | Semaphore (`refreshSem`), stop signals |
| `singleflight.Group` | Atomic execution + result sharing | DB query deduplication |
| `atomic` operations | Sequential consistency | Low-level counters and flags |

### 23.3 Best Practices

- **Fail-closed concurrency:** If a background refresh fails or a semaphore is full, serve cached data or deny the request — never operate on stale or absent state.
- **Bounded goroutines:** Combine semaphores and `WaitGroup`s to prevent resource exhaustion while ensuring clean shutdown.
- **Context propagation:** Background goroutines must handle context cancellation to avoid memory leaks at shutdown.

```go
// Correct pattern: every blocking goroutine is context-aware
func (s *Service) backgroundRefresh(ctx context.Context, key string) {
    select {
    case s.refreshSem <- struct{}{}: // acquire semaphore
    default:
        return // skip if at capacity
    }
    defer func() { <-s.refreshSem }() // release semaphore

    if ctx.Err() != nil { return } // check before doing work

    if err := s.repo.RefreshCache(ctx, key); err != nil {
        s.logger.Error("cache refresh failed", "key", key, "error", err)
    }
}
```

---

## ✅ Key Takeaways

- **Pointer receivers are the norm** for service, handler, and repository types — they share state, avoid copies, and correctly interact with concurrency primitives.
- **Channels are not just queues** — they are synchronization primitives, semaphores, and futures.
- **`context.Context` is the backbone** of every concurrent, I/O-bound operation. Pass it everywhere; never store it.
- **Errors are values** — wrap them with `%w` for traceability; use sentinel errors for caller-inspectable conditions.
- **`defer` + transactions + connections** is a triad: always defer `rows.Close()`, `conn.Release()`, and `tx.Rollback()` immediately after acquisition.
- **Fail-closed by default** — in security systems, unknown state equals denied access.
- **Test with interfaces** — designing against interfaces makes mocking trivial and keeps tests hermetic.

## ⚠️ Common Pitfalls

- Storing a `context.Context` in a struct field rather than passing it as an argument.
- Closing a channel from the receiver side — senders should always own the channel lifecycle.
- Using an unbuffered result channel in a worker pool, causing the worker to leak when the caller times out.
- Writing to a map concurrently without a mutex — Go maps are not goroutine-safe.
- Ignoring `rows.Err()` after iterating over SQL results.
- Using `new(map[K]V)` — produces a pointer to a nil map that panics on write.
- Forgetting `defer cancel()` after `context.WithTimeout` or `context.WithCancel` — leaks the context's resources.

## 🧠 Mental Models

- **Goroutines + Channels = CSP (Communicating Sequential Processes):** Design goroutines as independent agents that communicate through well-defined channel contracts.
- **Interface = Behaviour Contract:** Define interfaces at the point of consumption, not production. Small interfaces compose better.
- **`defer` is a stack:** In a function with multiple defers, they run LIFO. Use this to pair setup and teardown in adjacent lines.
- **Buffered channel as a token bucket:** The buffer capacity is the maximum number of simultaneous actors; sending acquires a token, receiving releases one.
- **Escape analysis = GC pressure indicator:** Variables that stay on the stack cost nothing to the garbage collector. Reduce escapes in hot paths.