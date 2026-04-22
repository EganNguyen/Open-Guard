package kafka

import "time"

// EventEnvelope is the canonical wire format for all Kafka events.
type EventEnvelope struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	OrgID       string    `json:"org_id"`
	ActorID     string    `json:"actor_id"`
	ActorType   string    `json:"actor_type"`
	OccurredAt  time.Time `json:"occurred_at"`
	Source      string    `json:"source"`
	EventSource string    `json:"event_source"`
	SchemaVer   string    `json:"schema_ver"`
	Payload     []byte    `json:"payload"`
}
