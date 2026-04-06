# §4 — Shared Contracts

All types in this section live in `github.com/openguard/shared/models`. They are **immutable across phases** — rename requires a major version bump of the shared module and migration of all consumers.

---

## 4.1 Kafka Event Envelope

```go
package models

import (
    "encoding/json"
    "time"
)

// EventEnvelope is the wire format for every Kafka message on every topic.
// Consumers MUST validate SchemaVer before processing.
type EventEnvelope struct {
    ID          string          `json:"id"`           // UUIDv4, globally unique
    Type        string          `json:"type"`         // dot-separated: "auth.login.success"
    OrgID       string          `json:"org_id"`       // tenant identifier
    ActorID     string          `json:"actor_id"`     // user ID, service name, or "system"
    ActorType   string          `json:"actor_type"`   // "user" | "service" | "system"
    OccurredAt  time.Time       `json:"occurred_at"`  // event time, not processing time
    Source      string          `json:"source"`       // originating service: "iam", "policy", etc.
    EventSource string          `json:"event_source"` // "internal" | "connector:<connector_id>"
    TraceID     string          `json:"trace_id"`     // OTel W3C trace ID
    SpanID      string          `json:"span_id"`      // OTel span ID
    SchemaVer   string          `json:"schema_ver"`   // "1.0" — increment on breaking changes
    Idempotent  string          `json:"idempotent"`   // dedup key for consumers
    Payload     json.RawMessage `json:"payload"`      // event-specific struct, JSON encoded
}
```

---

## 4.2 Outbox Record

```go
package models

import "time"

// OutboxRecord is persisted in the same transaction as the business operation.
// The relay process reads pending records and publishes to Kafka.
// IMPORTANT: The `org_id` column is explicit (UUID, not NULL) and is used
// for RLS enforcement. It must match the org_id of the business operation.
// The `key` column is the Kafka partition key (typically the same as org_id,
// but may differ). Do not use `key` in RLS policies — use `org_id`.
type OutboxRecord struct {
    ID          string     `db:"id"`           // UUIDv4
    OrgID       string     `db:"org_id"`       // Explicit org_id for RLS — NOT the Kafka key
    Topic       string     `db:"topic"`        // Kafka topic name
    Key         string     `db:"key"`          // Kafka partition key (usually org_id, may differ)
    Payload     []byte     `db:"payload"`      // JSON-encoded EventEnvelope
    Status      string     `db:"status"`       // "pending" | "published" | "dead"
    Attempts    int        `db:"attempts"`
    LastError   string     `db:"last_error"`
    CreatedAt   time.Time  `db:"created_at"`
    PublishedAt *time.Time `db:"published_at"`
    DeadAt      *time.Time `db:"dead_at"`
}
```

---

## 4.3 Saga Event

```go
package models

// SagaEvent wraps an EventEnvelope with saga orchestration metadata.
type SagaEvent struct {
    EventEnvelope
    SagaID       string `json:"saga_id"`             // UUIDv4, same across all steps
    SagaType     string `json:"saga_type"`           // "user.provision" | "user.deprovision"
    SagaStep     int    `json:"saga_step"`           // 1-based step number
    Compensation bool   `json:"compensation"`        // true = rollback event
    CausedBy     string `json:"caused_by,omitempty"` // event ID that caused this step
}
```

---

## 4.4 Kafka Topic Registry

```go
// shared/kafka/topics.go
package kafka

const (
    TopicAuthEvents        = "auth.events"
    TopicPolicyChanges     = "policy.changes"
    TopicDataAccess        = "data.access"
    TopicThreatAlerts      = "threat.alerts"
    TopicAuditTrail        = "audit.trail"
    TopicNotificationsOut  = "notifications.outbound"
    TopicSagaOrchestration = "saga.orchestration"
    TopicOutboxDLQ         = "outbox.dlq"
    TopicConnectorEvents   = "connector.events"
    TopicWebhookDelivery   = "webhook.delivery"
    TopicWebhookDLQ        = "webhook.dlq"
)

const (
    GroupAudit           = "openguard-audit-v1"
    GroupThreat          = "openguard-threat-v1"
    GroupAlerting        = "openguard-alerting-v1"
    GroupCompliance      = "openguard-compliance-v1"
    GroupPolicy          = "openguard-policy-v1"
    GroupSaga            = "openguard-saga-v1"
    GroupWebhookDelivery = "openguard-webhook-delivery-v1"
)
```

---

## 4.5 Canonical User Model

```go
package models

import "time"

type User struct {
    ID                 string     `json:"id" db:"id"`
    OrgID              string     `json:"org_id" db:"org_id"`
    Email              string     `json:"email" db:"email"`
    DisplayName        string     `json:"display_name" db:"display_name"`
    Status             UserStatus `json:"status" db:"status"`
    MFAEnabled         bool       `json:"mfa_enabled" db:"mfa_enabled"`
    MFAMethod          string     `json:"mfa_method,omitempty" db:"mfa_method"` // "totp" | "webauthn"
    SCIMExternalID     string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
    ProvisioningStatus string     `json:"provisioning_status" db:"provisioning_status"`
    TierIsolation      string     `json:"tier_isolation" db:"tier_isolation"`
    CreatedAt          time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
    DeletedAt          *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type UserStatus string

const (
    UserStatusActive             UserStatus = "active"
    UserStatusSuspended          UserStatus = "suspended"
    UserStatusDeprovisioned      UserStatus = "deprovisioned"
    UserStatusProvisioningFailed UserStatus = "provisioning_failed"
    UserStatusInitializing       UserStatus = "initializing"
)
```

---

## 4.6 Connected App Model

```go
package models

import "time"

type ConnectedApp struct {
    ID                string     `json:"id" db:"id"`
    OrgID             string     `json:"org_id" db:"org_id"`
    Name              string     `json:"name" db:"name"`
    WebhookURL        string     `json:"webhook_url" db:"webhook_url"`
    WebhookSecretHash string     `json:"-" db:"webhook_secret_hash"`
    APIKeyHash        string     `json:"-" db:"api_key_hash"` // PBKDF2-HMAC-SHA512, 600k iterations
    Scopes            []string   `json:"scopes" db:"scopes"`
    Status            string     `json:"status" db:"status"`  // "active" | "suspended" | "pending"
    CreatedAt         time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
    SuspendedAt       *time.Time `json:"suspended_at,omitempty" db:"suspended_at"`
    LastVerifiedAt    time.Time  `json:"-" db:"-"` // Ephemeral for Redis PBKDF2 grace period
}
```

---

## 4.7 Standard HTTP Contracts

**Error response:**
```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "User with id 'abc' not found",
    "request_id": "req_01j...",
    "trace_id": "4bf92f3577b34da6...",
    "retryable": false
  }
}
```

```go
package models

type APIError struct {
    Error APIErrorBody `json:"error"`
}

type APIErrorBody struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    RequestID string `json:"request_id"`
    TraceID   string `json:"trace_id"`
    Retryable bool   `json:"retryable"`
}
```

**Pagination envelope (all list endpoints):**
```json
{
  "data": [],
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 1024,
    "total_pages": 21,
    "next_cursor": "eyJpZCI6IjEyMyJ9"
  }
}
```

Cursor-based pagination for audit log and threat alert endpoints. Page-number pagination for user and policy lists.

**Cursor format per endpoint — keyset pagination semantics:**

| Endpoint family | Cursor fields | Cursor key encoding |
|---|---|---|
| `GET /audit/events` | `(occurred_at, event_id)` | `base64(json({"t":"<unix_ms>","id":"<uuid>"}))` |
| `GET /v1/threats/alerts` | `(created_at, id)` | `base64(json({"t":"<unix_ms>","id":"<uuid>"}))` |
| `GET /dlp/findings` | `(occurred_at, id)` | `base64(json({"t":"<unix_ms>","id":"<uuid>"}))` |
| `GET /users` | Offset-based (`page + per_page`) | integer page index |
| `GET /v1/policies` | Offset-based | integer page index |
| `GET /v1/scim/v2/Users` | `startIndex` + `count` (RFC 7644) | 1-indexed offset |

**Keyset WHERE clause (audit example):**
```sql
WHERE org_id = $1
  AND (occurred_at, event_id) < ($2, $3)  -- composite keyset, stable across deletes
ORDER BY occurred_at DESC, event_id DESC
LIMIT $4
```

**SCIM error responses** must follow RFC 7644 §3.12:
```json
{
  "schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
  "status": "404",
  "detail": "User not found"
}
```

The SCIM handler layer translates domain errors to SCIM error format before responding.

---

## 4.8 Canonical Sentinel Errors

```go
// shared/models/errors.go
package models

import "errors"

var (
    ErrNotFound       = errors.New("not found")
    ErrAlreadyExists  = errors.New("already exists")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrForbidden      = errors.New("forbidden")
    ErrCircuitOpen    = errors.New("circuit breaker open")
    ErrBulkheadFull   = errors.New("bulkhead full")
    ErrRetryable      = errors.New("retryable error")
    ErrSagaFailed     = errors.New("saga step failed")
    ErrRLSNotSet      = errors.New("RLS org_id context not set")
)
```
