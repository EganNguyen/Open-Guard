**Role:**
Act as a senior application security engineer and threat modeling expert with deep experience in secure software design, code auditing, and vulnerability assessment across distributed systems.

---

**Objective:**
Analyze the provided codebase, architecture, configuration, or system design to:

1. **Identify security vulnerabilities**
2. **Determine root causes**
3. **Provide actionable, production-grade remediation steps**

---

**Scope of Analysis:**
You must perform a comprehensive security review covering (but not limited to):

### 1. Input & Data Validation

* Injection risks (SQL, NoSQL, OS command, LDAP, XPath, template injection)
* Improper input sanitization or encoding
* Deserialization vulnerabilities
* File upload handling flaws

### 2. Authentication & Authorization

* Broken authentication flows
* Weak password or credential handling
* Missing or incorrect access control (RBAC/ABAC)
* Privilege escalation risks
* Insecure session management (tokens, cookies, JWT misuse)

### 3. Data Protection

* Sensitive data exposure (PII, secrets, tokens)
* Weak or incorrect cryptographic usage
* Hardcoded secrets or credentials
* Missing encryption at rest or in transit

### 4. API & Service Security

* Insecure endpoints (missing auth, excessive data exposure)
* Rate limiting / abuse protection gaps
* Improper error handling leaking sensitive info
* CORS misconfiguration

### 5. Infrastructure & Configuration

* Misconfigured environments (dev settings in prod)
* Insecure headers (CSP, HSTS, etc.)
* Open ports / unnecessary services
* Dependency vulnerabilities (outdated libraries)

### 6. Concurrency & State Security

* Race conditions leading to privilege bypass
* TOCTOU (time-of-check to time-of-use) issues
* Inconsistent state under concurrent access

### 7. Logging & Observability

* Sensitive data in logs
* Missing audit trails for critical actions
* Lack of intrusion detection signals

---

**For Each Issue Identified, You MUST Provide:**

### 1. Issue Description

* Clear explanation of the vulnerability
* Where it exists (file, function, component, or flow)

### 2. Severity Assessment

* Critical / High / Medium / Low
* Justify based on exploitability and impact

### 3. Root Cause Analysis

* Why this issue exists (design flaw, missing validation, incorrect assumptions, etc.)
* Link to broader systemic weaknesses if applicable

### 4. Exploitation Scenario

* How an attacker could realistically exploit this issue
* Step-by-step attack path (no unnecessary dramatization)

### 5. Recommended Fix (Actionable)

* Concrete code-level or architectural fix
* Secure pattern to apply (e.g., input validation, least privilege, defense-in-depth)
* If applicable, provide example code or configuration

### 6. Preventive Measures

* How to avoid similar issues in the future
* Suggested practices (secure coding, reviews, tooling, policies)

---

**Output Format:**

* Prioritized list of issues (highest risk first)
* Structured, concise, and technical
* No vague advice — every recommendation must be actionable

---

**Constraints:**

* Do NOT assume security controls unless explicitly shown
* Do NOT provide generic textbook explanations
* Focus on real-world exploitability and production impact
* Be precise, critical, and practical

---

**Goal:**
Deliver a security review that a senior engineering team can directly use to harden the system for production.


---

The **OWASP API Security Top 10** highlights the most critical risks when designing and building APIs. Below is a practical breakdown with **real Go examples** (focused on insecure vs secure patterns).

---

# 🔐 OWASP API Security Top 10 (with Go examples)

---

## 1. Broken Object Level Authorization (BOLA)

**Problem:** User can access another user's resource by changing ID.

### ❌ Insecure

```go
func GetUser(w http.ResponseWriter, r *http.Request) {
    id := r.URL.Query().Get("id")

    user, _ := db.GetUserByID(id) // no ownership check
    json.NewEncoder(w).Encode(user)
}
```

### ✅ Secure

```go
func GetUser(w http.ResponseWriter, r *http.Request) {
    userID := r.Context().Value("userID").(string)
    requestedID := r.URL.Query().Get("id")

    if userID != requestedID {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    user, _ := db.GetUserByID(requestedID)
    json.NewEncoder(w).Encode(user)
}
```

---

## 2. Broken Authentication

**Problem:** Weak or missing authentication.

### ❌ Insecure (no token validation)

```go
func Profile(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode("sensitive data")
}
```

### ✅ Secure (JWT validation)

```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")

        claims, err := validateJWT(token)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        ctx := context.WithValue(r.Context(), "userID", claims.UserID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

---

## 3. Broken Object Property Level Authorization

**Problem:** Overexposing sensitive fields.

### ❌ Insecure

```go
type User struct {
    ID       string
    Email    string
    Password string // exposed!
}
```

### ✅ Secure

```go
type UserResponse struct {
    ID    string `json:"id"`
    Email string `json:"email"`
}
```

---

## 4. Unrestricted Resource Consumption

**Problem:** No rate limit → DoS risk.

### ✅ Secure (basic rate limiting)

```go
var limiter = rate.NewLimiter(10, 20)

func RateLimit(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "too many requests", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

---

## 5. Broken Function Level Authorization

**Problem:** User can call admin APIs.

### ❌ Insecure

```go
func DeleteUser(w http.ResponseWriter, r *http.Request) {
    // no role check
}
```

### ✅ Secure

```go
func DeleteUser(w http.ResponseWriter, r *http.Request) {
    role := r.Context().Value("role").(string)

    if role != "admin" {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
}
```

---

## 6. Unrestricted Access to Sensitive Business Flows

**Problem:** Abuse flows (e.g., OTP brute force).

### ✅ Secure (OTP attempt limit)

```go
func VerifyOTP(userID, otp string) error {
    attempts := cache.GetAttempts(userID)

    if attempts > 5 {
        return errors.New("too many attempts")
    }

    if !checkOTP(userID, otp) {
        cache.IncrementAttempts(userID)
        return errors.New("invalid OTP")
    }

    return nil
}
```

---

## 7. Server Side Request Forgery (SSRF)

**Problem:** API fetches arbitrary URLs.

### ❌ Insecure

```go
http.Get(userInputURL)
```

### ✅ Secure (allowlist)

```go
func isAllowedHost(url string) bool {
    allowed := []string{"api.trusted.com"}
    // validate hostname
}
```

---

## 8. Security Misconfiguration

**Problem:** Debug mode, open CORS, etc.

### ✅ Secure headers

```go
func SecureHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Content-Security-Policy", "default-src 'self'")
        next.ServeHTTP(w, r)
    })
}
```

---

## 9. Improper Inventory Management

**Problem:** Old/unused APIs still exposed.

### ✅ Solution

* Version APIs: `/v1`, `/v2`
* Maintain API registry
* Remove deprecated endpoints

---

## 10. Unsafe Consumption of APIs

**Problem:** Trusting external APIs blindly.

### ❌ Insecure

```go
resp, _ := http.Get("https://external-api.com/data")
json.NewDecoder(resp.Body).Decode(&data)
```

### ✅ Secure

```go
client := &http.Client{Timeout: 3 * time.Second}

resp, err := client.Get("https://external-api.com/data")
if err != nil {
    return err
}

if resp.StatusCode != http.StatusOK {
    return errors.New("invalid response")
}
```

---

# 🧠 Key Patterns You Should Always Apply

* **AuthN** → Who are you?
* **AuthZ** → What can you access?
* **Validation** → Never trust input
* **Rate limiting** → Protect resources
* **Least privilege**
* **Audit logging**


