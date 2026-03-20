# Issues & Fixes Log

## 📌 How to Use
- Add a new entry whenever you encounter an issue
- Keep descriptions concise but clear
- Always include a fix and a prevention tip

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
