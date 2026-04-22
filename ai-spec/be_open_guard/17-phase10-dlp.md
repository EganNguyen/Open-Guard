# §18 — Phase 10: Content Scanning & DLP

**Goal:** Detect and mitigate sensitive data leakage in real-time. Scan latency p99 < 50ms for sync mode (per-org opt-in). Default mode is async.

---

## 18.1 Database Schema

**001_create_dlp_policies.up.sql**
```sql
CREATE TABLE dlp_policies (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    name         TEXT NOT NULL,
    rules        JSONB NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    mode         TEXT NOT NULL DEFAULT 'monitor',  -- 'monitor' | 'block'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Index on (org_id) WHERE enabled = TRUE; RLS policy; GRANT to openguard_app
```

**002_create_dlp_findings.up.sql**
```sql
CREATE TABLE dlp_findings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL,
    event_id      UUID NOT NULL,
    rule_id       UUID REFERENCES dlp_policies(id),
    finding_type  TEXT NOT NULL,    -- 'pii' | 'credential' | 'financial'
    finding_kind  TEXT NOT NULL,    -- 'email' | 'ssn' | 'credit_card' | 'api_key' | 'high_entropy'
    json_path     TEXT NOT NULL,    -- JSONPath to the matched field (for masking)
    action_taken  TEXT NOT NULL,    -- 'monitor' | 'mask' | 'block'
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Indexes on event_id, (org_id, occurred_at DESC); RLS policy
```

---

## 18.2 Scanning Engine

**Tier 1 — Regex (PII and Financial):**

| Finding kind | Pattern | Validation |
|---|---|---|
| `email` | RFC 5322 simplified | None |
| `ssn` | `\b\d{3}-\d{2}-\d{4}\b` | None |
| `credit_card` | Visa/MC/Amex patterns | Luhn check |
| `phone_us` | `\b\+?1?[-.\s]?\(?\d{3}\)?[-.\s]\d{3}[-.\s]\d{4}\b` | None |

**Tier 2 — Entropy (Credentials):**

```go
func shannonEntropy(s string) float64 {
    if len(s) == 0 { return 0 }
    freq := make(map[rune]int)
    for _, c := range s { freq[c]++ }
    entropy := 0.0
    for _, count := range freq {
        p := float64(count) / float64(len(s))
        entropy -= p * math.Log2(p)
    }
    return entropy
}

// Flagged as credential if:
//   len(s) >= DLP_MIN_CREDENTIAL_LENGTH (24) AND
//   shannonEntropy(s) >= DLP_ENTROPY_THRESHOLD (4.5) AND
//   not in common false-positive list (UUIDs, base64 of low-entropy data)
```

**Known prefixes (immediate credential flag):** `sk_live_`, `sk_test_`, `AIza`, `AKIA`, `ghp_`, `github_pat_`, `xoxb-`, `xoxp-`.

---

## 18.3 Integration Flow

**Default (`dlp_mode=monitor`):**
```
Connected app → POST /v1/events/ingest → accepted immediately
→ Outbox relay → Kafka (connector.events, audit.trail)
→ DLP service consumes connector.events ASYNC
→ Finds PII → dlp.finding.created event → audit service masks field in MongoDB
```

**Opt-in (`dlp_mode=block`):**
```
Connected app → POST /v1/events/ingest
→ Control Plane: org has dlp_mode=block? YES
→ Sync call to DLP service (mTLS, cb-dlp, DLP_SYNC_BLOCK_TIMEOUT_MS=30ms)
→ DLP: finds credit card → returns Block decision
→ Control Plane: 422 DLP_POLICY_VIOLATION, event NOT written to outbox
→ DLP service unavailable: reject event (fail closed)
```

**Masking flow (monitor mode):**
```
DLP service finds SSN at json_path "$.payload.form_data.social_security"
→ Writes dlp_finding record (PostgreSQL, RLS-scoped)
→ Publishes dlp.finding.created (via outbox) to audit.trail
→ Audit service consumes dlp.finding.created
→ Updates audit_events document: replaces matched value with "[REDACTED:ssn]"
```

> **Compliance Notice:** In monitor mode, there is an async window (typically < 2 seconds) between ingestion and masking. Cleartext PII exists in MongoDB during this window. For strict HIPAA or GDPR deployments, organizations MUST configure `dlp_mode=block`.

---

## 18.4 DLP API

| Method | Path | Description |
|---|---|---|
| `GET` | `/v1/dlp/policies` | List DLP policies |
| `POST` | `/v1/dlp/policies` | Create DLP policy |
| `GET` | `/v1/dlp/policies/:id` | Get policy |
| `PUT` | `/v1/dlp/policies/:id` | Update policy |
| `DELETE` | `/v1/dlp/policies/:id` | Delete policy |
| `GET` | `/v1/dlp/findings` | List findings (cursor paginated) |
| `GET` | `/v1/dlp/findings/:id` | Finding detail + json_path |
| `GET` | `/v1/dlp/stats` | Finding counts by type |

---

## 18.5 Phase 10 Acceptance Criteria

- [ ] Regex scanner identifies email and SSN in JSON payloads.
- [ ] Luhn scanner identifies valid Visa credit card numbers; ignores random digit strings.
- [ ] Entropy scanner detects `AKIAIOSFODNN7EXAMPLE` (AWS access key) correctly.
- [ ] Sync block (`dlp_mode=block`): `POST /v1/events/ingest` with cleartext credit card → `422 DLP_POLICY_VIOLATION`.
- [ ] Sync block with DLP service down → request rejected (`503 DLP_UNAVAILABLE` for blocking orgs).
- [ ] Monitor mode: event accepted → SSN detected → audit log field masked within 5s.
- [ ] DLP finding auto-creates HIGH threat alert for `credential` finding type.
- [ ] `openguard_dlp_findings_total` metric incremented per finding.
