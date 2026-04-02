package sdk

import (
	"encoding/json"
	"time"
)

// PolicyRequest matches the OpenGuard control plane evaluation request.
type PolicyRequest struct {
	UserID   string `json:"user_id"`
	OrgID    string `json:"org_id"`
	Action   string `json:"action"`
	Resource string `json:"resource"`
}

// PolicyResponse matches the OpenGuard control plane evaluation response.
type PolicyResponse struct {
	Permitted bool   `json:"permitted"`
	Reason    string `json:"reason,omitempty"`
}

// AuditEvent is the payload sent to /v1/events/ingest.
type AuditEvent struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	OrgID       string          `json:"org_id"`
	ActorID     string          `json:"actor_id"`
	ActorType   string          `json:"actor_type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Source      string          `json:"source"`
	EventSource string          `json:"event_source"`
	Payload     json.RawMessage `json:"payload"`
}
