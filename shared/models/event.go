package models

import (
	"encoding/json"
	"time"
)

// EventEnvelope is the standard wrapper for all Kafka messages.
// Every message on every topic must be a JSON-serialized EventEnvelope.
type EventEnvelope struct {
	ID         string          `json:"id"`          // UUIDv4
	Type       string          `json:"type"`        // e.g. "auth.login.success"
	OrgID      string          `json:"org_id"`
	ActorID    string          `json:"actor_id"`    // user or system ID
	ActorType  string          `json:"actor_type"`  // "user" | "service" | "system"
	OccurredAt time.Time       `json:"occurred_at"`
	Source     string          `json:"source"`      // originating service name
	TraceID    string          `json:"trace_id"`    // OpenTelemetry trace ID
	SchemaVer  string          `json:"schema_ver"`  // "1.0"
	Payload    json.RawMessage `json:"payload"`     // event-specific data
}
