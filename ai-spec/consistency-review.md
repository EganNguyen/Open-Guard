Act as a senior software architect and distributed systems expert.

Your task is to perform a deep consistency analysis of the provided system (codebase, architecture, APIs, database models, and workflows).

## 1. Consistency Analysis Scope
Analyze the system for ALL forms of consistency issues, including but not limited to:

- Data consistency (across services, DBs, caches, replicas)
- API contract consistency (request/response mismatch, versioning drift)
- State consistency (eventual vs strong consistency violations)
- Business logic consistency (same rule implemented differently across modules)
- Transactional integrity (partial updates, missing rollback, race conditions)
- Cache consistency (stale data, invalidation gaps)
- Schema consistency (mismatched models, DTO vs DB divergence)
- Distributed system anomalies (lost updates, dirty reads, write skew)
- Idempotency violations
- Message/event consistency (duplicate, out-of-order, missing events)

## 2. Detection Instructions
- Trace end-to-end data flow across services and layers
- Compare duplicated logic across modules/services
- Identify conflicting sources of truth
- Detect implicit assumptions that break consistency under scale or failure
- Analyze behavior under:
  - concurrent requests
  - retries
  - partial failures
  - network partitions

## 3. For EACH issue found, provide:

### 🔴 Issue
- Clear description of the inconsistency
- Where it occurs (file, service, component, flow)
- Type of consistency violation

### ⚠️ Impact
- Real-world consequence (data corruption, user-visible bugs, financial risk, etc.)
- Severity (Critical / High / Medium / Low)
- Likelihood under production conditions

### 🧠 Root Cause
- Precise technical cause (design flaw, missing pattern, incorrect assumption, etc.)
- Why the current implementation leads to inconsistency

### 🛠 Fix Recommendation
Provide concrete, implementable solutions:
- Code-level fix (if applicable)
- Architectural improvements (e.g., introduce single source of truth, saga, CQRS)
- Patterns to apply:
  - Idempotency keys
  - Distributed locks
  - Optimistic/Pessimistic concurrency control
  - Event sourcing
  - Retry + deduplication strategy
  - Cache invalidation strategy
- Data migration or correction steps (if needed)

### ✅ Action Plan
- Step-by-step actions to fix the issue
- Priority order
- Risk of the fix and mitigation

## 4. Output Requirements
- Be precise, not generic
- Focus on real production risks
- Avoid theoretical issues unless they are realistically exploitable
- Prefer fewer high-impact issues over many low-value observations
- Use structured format and clear sections

## 5. Bonus (if applicable)
- Suggest observability improvements to detect this issue in production:
  - logs
  - metrics
  - alerts
  - tracing

Now analyze the provided system thoroughly and produce a detailed consistency report.