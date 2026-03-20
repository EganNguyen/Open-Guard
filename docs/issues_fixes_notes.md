# Issues & Fixes Log

## 📌 How to Use
- Add a new entry whenever you encounter an issue
- Keep descriptions concise but clear
- Always include a fix and a prevention tip
- Log the issue. Fix it. Update the spec to prevent it.

---

## 🧩 Issue Template

```md
### 🐞 Issue: <Short Title>
**Date:** YYYY-MM-DD  
**Tags:** #tag1 #tag2  

**Description:**  
What went wrong? Include context and example if needed.

**Cause:**  
Why did this happen?

**Fix:**  
What solved the issue?

**Prevention:**  
How to avoid this next time?

**Example (optional):**
```

---

## 📚 Issues

### 🐞 Issue: mTLS Handshake Failure (Gateway to IAM)
**Date:** 2026-03-20  
**Tags:** #security #mtls #gateway

**Description:**  
Integration tests failed with `503 Service Unavailable` during registration. Gateway could not verify the IAM service certificate.

**Cause:**  
The Gateway's Go HTTP client was not configured with the internal CA certificate (`CA_CERT_PATH`), causing it to reject backend certificates.

**Fix:**  
Plumbed `tls.Config` through the Gateway router and proxy transport, loading the CA, client cert, and key in `main.go`.

**Prevention:**  
Ensure internal client transports always trust the same CA used by backend servers in an mTLS environment.

---

### 🐞 Issue: PostgreSQL INET Scanning Regression (pgx v5)
**Date:** 2026-03-20  
**Tags:** #postgres #go #pgx #regression

**Description:**  
Transitioning a column from `TEXT` to `INET` caused `Scan` errors: `cannot scan inet (OID 869) in binary format into **string`.

**Cause:**  
`pgx` v5 in binary mode does not automatically scan the `INET` OID into a Go `string` pointer. It expects specific networking types.

**Fix:**  
Casted the column to `TEXT` in the SQL query: `SELECT ip_address::TEXT ...`.

**Prevention:**  
Be cautious when using specialized Postgres types like `INET` or `UUID` with the binary protocol; explicit casting in SQL is safer for generic string fields.

---

### 🐞 Issue: Transactional Abort Leakage
**Date:** 2026-03-20  
**Tags:** #postgres #transactions #iam

**Description:**  
Endpoints like `/auth/login` and `/users/{id}/tokens` returned 500 errors because the transaction was already aborted.

**Cause:**  
A non-critical database error (like a logged scanning failure) marked the PostgreSQL transaction as `ABORTED`. Subsequent writes or the final `Commit` then failed with `current transaction is aborted`.

**Fix:**  
Handled non-critical errors by either making them fatal (consistent rollback) or using savepoints. Ensured critical database operations correctly stop execution before reaching `Commit`.

**Prevention:**  
Assume **any** Postgres error in a transaction block invalidates the entire block. Never attempt to `Commit` after a logged DB error within the same transaction.

---

### 🐞 Issue: Null Constraint Violation on Scopes
**Date:** 2026-03-20  
**Tags:** #postgres #apitokens #iam

**Description:**  
Creating an API token failed with 500: `null value in column "scopes" ... violates not-null constraint`.

**Cause:**  
The `scopes` field was initialized as `nil` in Go, which `pgx` sent as `NULL`, violating the `TEXT[] NOT NULL` constraint.

**Fix:**  
Initialized `scopes` to an empty slice `[]string{}` if nil before calling the repository.

**Prevention:**  
Always initialize slices that map to `NOT NULL` array columns in Postgres to prevent inadvertent `NULL` inserts.

---

### 🐞 Issue: Policy Service Internal Identity Type Mismatch
**Date:** 2026-03-20  
**Tags:** #go #context #multi-tenancy #policy

**Description:**  
Policy handlers were unable to retrieve `org_id` and `user_id` from the request context, even when correctly injected by the router middleware.

**Cause:**  
The `router` package and `handlers` package defined their own `ContextKey` types. Even though both were `string` aliases with the same value, Go treats different named types as distinct keys.

**Fix:**  
Moved shared context keys and retrieval helpers to a dedicated `pkg/tenant` package reachable by both router and handlers.

**Prevention:**  
Always define context keys in a single, shared package when they need to be accessed across different packages in the same service.

---

### 🐞 Issue: Import Cycle in Service Refactoring
**Date:** 2026-03-20  
**Tags:** #go #refactoring #architecture

**Description:**  
Attempting to fix context key visibility by importing the `router` package into the `handlers` package caused a circular dependency, as the router already imported handlers to register routes.

**Cause:**  
Bidirectional dependency between two sub-packages of the same service.

**Fix:**  
Introduced a third package (`pkg/tenant`) to hold the shared data (context keys), which both `router` and `handlers` can import without depending on each other.

**Prevention:**  
When two packages need to share constants or types, move them "down" into a shared leaf package or "up" into a common parent.

---

### 🐞 Issue: Gateway Middleware Schema Mismatch
**Date:** 2026-03-20  
**Tags:** #api #gateway #json #policy

**Description:**  
Policy evaluation requests from the Gateway were rejected with `400 Bad Request` or misinterpreted as `403 Forbidden`.

**Cause:**  
The Gateway's `PolicyMiddleware` was sending a differently structured `EvalRequest` (using a `Context` map) than what the Policy Service expected (top-level JSON fields). Additionally, it looked for a `result` field in the response while the service sent `permitted`.

**Fix:**  
Aligned the `EvalRequest` and `EvalResponse` structs in the gateway to match the canonical policy engine API.

**Prevention:**  
Use shared model packages or generate client/server code from a single source of truth (like OpenAPI or Protobuf) to prevent schema drift.

---

### 🐞 Issue: Missing Mandatory Audit/User Fields (UUID Syntax Error)
**Date:** 2026-03-20  
**Tags:** #postgres #uuid #policy

**Description:**  
Policy creation failed with `500 Internal Error` and a Postgres error: `invalid input syntax for type uuid: ""`.

**Cause:**  
The `created_by` field (UUID NOT NULL) was not being populated in the `Create` handler, and the `user_id` in policy evaluation logs was also failing for anonymous requests due to a `NOT NULL` constraint.

**Fix:**  
Updated the handler to extract the Actor ID from the context and set `p.CreatedBy`. Updated the DB migration to make `user_id` nullable in evaluation logs to support anonymous access.

**Prevention:**  
Verify that all `NOT NULL` UUID columns in the database have corresponding population logic in the application layer, or use appropriate defaults/nullability for optional fields.

---

### 🐞 Issue: Policy Evaluation Cache Key Collision
**Date:** 2026-03-20  
**Tags:** #redis #caching #policy #security

**Description:**  
Policy evaluations were incorrectly returning `permitted=true` for unauthorized IP addresses after a previous successful request from an authorized IP.

**Cause:**  
The `evalCacheKey` function used to compute Redis keys did not include the `IPAddress` field in its hash. Consequently, different requests (same user/action/resource but different IPs) mapped to the same cache entry.

**Fix:**  
Updated `evalCacheKey` to include `req.IPAddress` in the SHA-256 hash calculation, ensuring distinct cache entries for different source IPs.

**Prevention:**  
Always include all fields that influence the business logic of a function (the "varying inputs") in its cache key generation to prevent data leakage and incorrect results.

---

### 🐞 Issue: Nil Logger Panic in Service Tests
**Date:** 2026-03-20  
**Tags:** #go #tests #nil-pointer #logging

**Description:**  
Unit tests for `EvaluatorService` panicked with a nil pointer dereference: `panic: runtime error: invalid memory address or nil pointer dereference` pointing to a `logger.Debug` call.

**Cause:**  
New debug logging was added to the `EvaluatorService` methods, but the corresponding unit tests initialized the service with a `nil` logger. Unlike the global `slog` functions, methods on a `nil` logger instance cause a panic.

**Fix:**  
Added safety checks (`if s.logger != nil`) or used a no-op/default logger in the constructor. Also removed temporary verbose debug logs after the root cause of the primary issue (cache collision) was resolved.

**Prevention:**  
Never assume a logger (or any dependency) provided via a constructor is non-nil in testing environments. Either enforce non-nil requirements in the constructor or wrap logging calls in safety checks.
