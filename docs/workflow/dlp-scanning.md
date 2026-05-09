# DLP Scanning Pipeline — Workflow

## Level 1: High-Level Architecture

```
                         ┌──────────────────────────────────────────────────────────────────────────┐
                         │                        EVENT SOURCES                                     │
                         │                                                                            │
                         │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐        │
                         │  │  SDK/External    │  │  IAM/Policy Svc  │  │  Any Producer    │        │
                         │  │  (POST /ingest)  │  │  (via outbox)    │  │  (audit.trail)   │        │
                         │  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘        │
                         │           │                     │                     │                    │
                         │           │  HTTPS (sync)       │  Kafka (async)      │  Kafka (async)    │
                         └───────────┼─────────────────────┼─────────────────────┼────────────────────┘
                                     │                     │                     │
                                     ▼                     ▼                     ▼
                         ┌──────────────────────────────────────────────────────────────────────────┐
                         │                                                                              │
                         │    ┌───────────────────────────────────────────────────────────────────┐    │
                         │    │                         AUDIT SERVICE (port 8085)                  │    │
                         │    │                                                                     │    │
                         │    │  ┌─────────────────────────────────────────────────────────────┐  │    │
                         │    │  │  INGEST ENDPOINT (POST /v1/events/ingest)                   │  │    │
                         │    │  │                                                              │  │    │
                         │    │  │  1. Receive event                                           │  │    │
                         │    │  │  2. Extract org_id from JWT                                 │  │    │
                         │    │  │  3. [IF DLP_MODE=block] Sync DLP check                     │  │    │
                         │    │  │     ├── POST {dlp_url}/v1/scan (2s timeout)                │  │    │
                         │    │  │     ├── Findings → 422 (fail-closed)                       │  │    │
                         │    │  │     ├── DLP down → 422 (fail-closed)                       │  │    │
                         │    │  │     └── Clean → continue                                   │  │    │
                         │    │  │  4. Publish to Kafka (audit.trail)                         │  │    │
                         │    │  │  5. 202 Accepted                                           │  │    │
                         │    │  └─────────────────────────────────────────────────────────────┘  │    │
                         │    └───────────────────────────────────────────────────────────────────┘    │
                         │                                                                            │
                         └────────────────────────┬───────────────────────────────────────────────────┘
                                                  │
                                                  │ Kafka: audit.trail
                                                  ▼
                         ┌──────────────────────────────────────────────────────────────────────────┐
                         │                                                                              │
                         │    ┌───────────────────────────────────────────────────────────────────┐    │
                         │    │                    DLP SERVICE (port 8089)                         │    │
                         │    │                                                                     │    │
                         │    │  ┌─────────────────────────────────────────────────────────────┐  │    │
                         │    │  │  KAFKA CONSUMER (audit.trail, group: dlp-service-group)      │  │    │
                         │    │  │                                                              │  │    │
                         │    │  │  For each message:                                           │  │    │
                         │    │  │   1. Extract content from event metadata                     │  │    │
                         │    │  │   2. Run composite scanner                                   │  │    │
                         │    │  │      ├── RegexScanner (email, SSN, CC, AWS keys, etc.)       │  │    │
                         │    │  │      └── EntropyScanner (Shannon > 4.5, len > 20)           │  │    │
                         │    │  │   3. If findings → persist to PostgreSQL                     │  │    │
                         │    │  │   4. Commit Kafka offset                                     │  │    │
                         │    │  │                                                              │  │    │
                         │    │  │  (Note: sync scan from audit ingest ALSO hits this)          │  │    │
                         │    │  └─────────────────────────────────────────────────────────────┘  │    │
                         │    │                                                                     │    │
                         │    │  ┌─────────────────────────────────────────────────────────────┐  │    │
                         │    │  │  REST API                                                     │    │
                         │    │  │  POST /v1/scan       → Scan content inline                    │    │
                         │    │  │  GET  /v1/policies   → List DLP policies                      │    │
                         │    │  │  POST /v1/policies   → Create DLP policy                      │    │
                         │    │  │  GET  /v1/findings   → List DLP findings (paginated)           │    │
                         │    │  └─────────────────────────────────────────────────────────────┘  │    │
                         │    └───────────────────────────────────────────────────────────────────┘    │
                         │                                                                            │
                         └────────────────────────┬───────────────────────────────────────────────────┘
                                                  │
                                                  ▼
                         ┌──────────────────────────────────────────────────────────────────────────┐
                         │                     DATA STORES                                           │
                         │                                                                            │
                         │  ┌────────────────────────────────────────────────────┐                   │
                         │  │  PostgreSQL (openguard_dlp)                        │                   │
                         │  │                                                    │                   │
                         │  │  dlp_findings:                                     │                   │
                         │  │    id UUID PK                                     │                   │
                         │  │    org_id UUID                                    │                   │
                         │  │    event_id UUID                                  │                   │
                         │  │    rule_name TEXT                                 │                   │
                         │  │    rule_type TEXT (regex/entropy)                 │                   │
                         │  │    matched_value TEXT (redacted)                   │                   │
                         │  │    context TEXT                                    │                   │
                         │  │    severity TEXT                                   │                   │
                         │  │    created_at TIMESTAMPTZ                          │                   │
                         │  │                                                    │                   │
                         │  │  dlp_policies:                                     │                   │
                         │  │    id UUID PK                                     │                   │
                         │  │    org_id UUID                                    │                   │
                         │  │    name TEXT                                      │                   │
                         │  │    mode TEXT (monitor/block)                      │                   │
                         │  │    rules JSONB                                    │                   │
                         │  │    enabled BOOLEAN                                │                   │
                         │  │    created_at TIMESTAMPTZ                          │                   │
                         │  └────────────────────────────────────────────────────┘                   │
                         └──────────────────────────────────────────────────────────────────────────┘
```

---

## Level 2A: Synchronous DLP (Block Mode — via Audit Ingest)

```
  SDK/App                    Audit Service                        DLP Service
    │                            │                                    │
    │  POST /v1/events/ingest    │                                    │
    │  { event payload }         │                                    │
    │───────────────────────────>│                                    │
    │                            │                                    │
    │                            │  DLP_MODE == "block"?              │
    │                            │  Yes → call DLP check              │
    │                            │                                    │
    │                            │  POST /v1/scan                     │
    │                            │  { content: extract(event) }       │
    │                            │───────────────────────────────────>│
    │                            │                                    │
    │                            │    ┌─ Step 1: Run RegexScanner     │
    │                            │    │    email pattern → match?     │
    │                            │    │    SSN pattern → match?       │
    │                            │    │    Credit Card (Luhn) →match? │
    │                            │    │    AWS Key pattern → match?   │
    │                            │    │    Private Key → match?       │
    │                            │    │    Phone → match?             │
    │                            │    │                                │
    │                            │    │  Step 2: Run EntropyScanner   │
    │                            │    │    Shannon entropy > 4.5?     │
    │                            │    │    Length > 20 chars?         │
    │                            │    │                                │
    │                            │    │  Step 3: Aggregate results    │
    │                            │    │                                │
    │                            │  <── 200 { findings: [...] } ─────│
    │                            │  <── or 200 { findings: [] } ─────│
    │                            │                                    │
    │                            │  Evaluate findings:                 │
    │                            │  ┌─ findings.length > 0 →          │
    │  <── 422 DLP Blocked ──────│──│  Block event (fail-closed)     │
    │                            │  │                                  │
    │                            │  └─ no findings →                  │
    │                            │     Publish to Kafka (audit.trail) │
    │  <── 202 Accepted ────────│────────────────────────────────────│
    │                            │                                    │
    │  ┌─ DLP service DOWN:      │                                    │
    │  │  (timeout, connection   │                                    │
    │  │   refused, 5xx)         │                                    │
    │  <── 422 DLP Unavailable ──│── (fail-closed on outage)          │
```

---

## Level 2B: Async DLP (Monitor Mode — via Kafka Consumer)

```
  Kafka (audit.trail)            DLP Consumer                        PostgreSQL
    │                                │                                  │
    │  FetchMessage()                │                                  │
    │───────────────────────────────>│                                  │
    │                                │                                  │
    │                                │  Deserialize event               │
    │                                │                                  │
    │                                │  Extract text content from       │
    │                                │  event metadata/body             │
    │                                │                                  │
    │                                │  Run composite scanner           │
    │                                │    ├── RegexScanner.Scan()       │
    │                                │    └── EntropyScanner.Scan()     │
    │                                │                                  │
    │                                │  ┌─ findings detected?           │
    │                                │  │  INSERT INTO dlp_findings     │
    │                                │  │  (redact sensitive values)    │
    │                                │  │──────────────────────────────>│
    │                                │  │                                  │
    │                                │  └─ no findings → skip            │
    │                                │                                  │
    │                                │  Commit Kafka offset             │
    │                                │                                  │
    │  (findings available via       │                                  │
    │   GET /v1/findings API)        │                                  │
```

---

## Level 3: Scanner Internals

### Regex Rules

```
  ┌─────────────────────────────────────────────────────────────────────┐
  │                        REGEX DETECTION RULES                        │
  │                                                                     │
  │  Email:       [a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}      │
  │  SSN:         \b\d{3}-\d{2}-\d{4}\b                                │
  │  Credit Card: \b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b             │
  │                 (+ Luhn algorithm validation)                       │
  │  AWS Key:     (AKIA|ASIA)[A-Z0-9]{16}                              │
  │  Private Key: -----BEGIN (RSA | OPENSSH | DSA | EC) PRIVATE KEY--- │
  │  Phone:       \b\+?\d{1,3}[-. (]?\d{1,4}[-. )]?\d{1,4}[-. ]?\d{1,9}\b  │
  │                                                                     │
  │  Each rule returns: { rule_name, matched_value (redacted),          │
  │                       context (surrounding text) }                  │
  └─────────────────────────────────────────────────────────────────────┘
```

### Shannon Entropy Detection

```
  ┌─────────────────────────────────────────────────────────────────────┐
  │                      ENTROPY DETECTION LOGIC                        │
  │                                                                     │
  │  For each text token (split on whitespace, min length 20):         │
  │                                                                     │
  │    Shannon Entropy = -Σ p(x) * log2(p(x))                          │
  │                                                                     │
  │    Where p(x) = frequency of character x in the token               │
  │                                                                     │
  │    Thresholds:                                                      │
  │      entropy > 4.5  AND  token length > 20  → FLAG                 │
  │      entropy > 5.5  AND  token length > 15  → FLAG                 │
  │      entropy > 6.0  AND  token length > 10  → FLAG                 │
  │                                                                     │
  │  Purpose: Detect high-entropy strings like API keys, tokens,       │
  │  passwords, secrets that don't match known regex patterns.         │
  │                                                                     │
  │  Example matches:                                                   │
  │    "dGhpcyBpcyBhIHRlc3QgYmFzZTY0IHN0cmluZw=="  (base64, high entropy)│
  │    "sk-9f3b8c2a1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a" (API key pattern)│
  └─────────────────────────────────────────────────────────────────────┘
```

### Composite Scanner Flow

```
  ScanContent(text)
    │
    ├── 1. Normalize input (trim, strip null bytes)
    │
    ├── 2. Run all regex rules
    │     for each rule in regexRules:
    │       matches := rule.re.FindAllString(text, -1)
    │       for each match:
    │         if rule requires validation (e.g., Luhn for CC):
    │           if !validate(match) → skip
    │         findings.append({
    │           rule_name: rule.name,
    │           matched_value: redact(match),
    │           context: extract_surrounding(text, match),
    │           rule_type: "regex"
    │         })
    │
    ├── 3. Run entropy scanner
    │     tokens := strings.Fields(text)
    │     for each token where len(token) >= minLength:
    │       entropy := shannon(token)
    │       if entropy >= threshold:
    │         findings.append({
    │           rule_name: "high_entropy",
    │           matched_value: redact(token),
    │           context: extract_surrounding(text, token),
    │           rule_type: "entropy"
    │         })
    │
    └── 4. Return findings (aggregated, no duplicates)
```

---

## Ownership Boundaries

| Layer | Responsibility |
|-------|---------------|
| **Event Producer (SDK/Service)** | Sends events that may contain sensitive data |
| **Audit Service (Ingest)** | Routes event content to DLP for synchronous scanning in block mode |
| **DLP Service (Scanner)** | Regex + entropy content analysis, finding persistence |
| **DLP Service (Consumer)** | Async scanning of all audit trail events in monitor mode |
| **PostgreSQL** | DLP findings and policy definitions |
| **Angular Dashboard** | Displays DLP findings, manages policies |

---

## Failure Scenarios

| Failure | Layer | Behavior |
|---------|-------|----------|
| **DLP service down (block mode)** | Audit | Ingest returns 422 (fail-closed: cannot verify) |
| **DLP service down (monitor mode)** | DLP | Kafka consumer blocks; no scanning until recovery |
| **Regex catastrophic backtracking** | DLP | Malicious input could cause CPU spike; mitigated by input normalization |
| **PostgreSQL unavailable (findings)** | DLP | Findings not persisted but scanning still works (in-memory only) |
| **Luhn false positive** | DLP | 16-digit numbers matching Luhn flagged as CC; mitigated by context |
| **Large payload** | DLP | Memory-bound scan of full content; no streaming split |
| **Poison pill (bad Kafka message)** | DLP | Consumer skips, logs error, commits offset (1 event lost) |
| **mTLS cert expired** | DLP | Falls back to HTTP (dev); fails to start in strict mode |

---

## Metrics & Observability

### Key Metrics (Prometheus)

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `openguard_dlp_scans_total` | Counter | `mode`, `result` | DLP Service |
| `openguard_dlp_findings_total` | Counter | `rule_type`, `rule_name` | DLP Service |
| `openguard_dlp_scan_duration_seconds` | Histogram | `scanner` | DLP Service |

### Key Traces (Jaeger)

- `dlp.sync.scan` — synchronous scan called from Audit ingest
- `dlp.async.scan` — async scan from Kafka consumer
- `dlp.regex.scan` — regex matching phase
- `dlp.entropy.scan` — entropy computation phase

### Audit Events

| Event | When | Payload |
|-------|------|---------|
| `dlp.findings.detected` | Sensitive data found | org_id, event_id, rule_names, severity |
| `dlp.event.blocked` | Block mode: event rejected | org_id, event_id, rule_names |
| `dlp.scan.completed` | Scan with no findings | org_id, event_id |

---

## Policy Configuration (Monitor vs Block)

```
  dlp_policies table:
    {
      "id": "uuid",
      "org_id": "uuid",
      "name": "PCI Data Protection",
      "mode": "block",            // "monitor" | "block"
      "enabled": true,
      "rules": [
        { "type": "regex", "pattern": "credit_card", "severity": "high" },
        { "type": "regex", "pattern": "ssn", "severity": "critical" },
        { "type": "entropy", "threshold": 4.5, "severity": "medium" }
      ]
    }

  Mode behavior:
    "monitor" → findings persisted, event always allowed
    "block"   → findings persisted, event rejected with 422
```
