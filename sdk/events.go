package sdk

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AuditEvent struct {
	EventID    string         `json:"event_id"`
	EventType  string         `json:"event_type"`
	UserID     string         `json:"user_id"`
	OrgID      string         `json:"org_id"`
	ResourceID string         `json:"resource_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// IngestEvent sends a security event to the OpenGuard event pipeline.
// IngestEvent pushes a security audit event to the OpenGuard control plane.
// This uses a unique EventID (UUID) for deduplication at the edge.
func (c *Client) IngestEvent(ctx context.Context, event AuditEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return c.do(ctx, "POST", "/v1/events/ingest", event, nil)
}
