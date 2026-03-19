package models

import (
	"encoding/json"
	"time"
)

// EventEnvelope is the wire format for every Kafka message on every topic.
// Consumers MUST validate SchemaVer before processing.
type EventEnvelope struct {
	ID         string          `json:"id"`          // UUIDv4, globally unique
	Type       string          `json:"type"`        // dot-separated, e.g. "auth.login.success"
	OrgID      string          `json:"org_id"`      // tenant identifier
	ActorID    string          `json:"actor_id"`    // user ID, service name, or "system"
	ActorType  string          `json:"actor_type"`  // "user" | "service" | "system"
	OccurredAt time.Time       `json:"occurred_at"` // event time, not processing time
	Source     string          `json:"source"`      // originating service: "iam", "policy", etc.
	TraceID    string          `json:"trace_id"`    // OpenTelemetry W3C trace ID
	SpanID     string          `json:"span_id"`     // OpenTelemetry span ID
	SchemaVer  string          `json:"schema_ver"`  // "1.0" — increment on breaking changes
	Idempotent string          `json:"idempotent"`  // dedup key for consumers
	Payload    json.RawMessage `json:"payload"`     // event-specific struct, JSON encoded
}
