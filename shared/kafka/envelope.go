package kafka

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

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
	Traceparent string    `json:"traceparent,omitempty"` // W3C Trace Context
	Tracestate  string    `json:"tracestate,omitempty"`
	Payload     []byte    `json:"payload"`
}

// InjectTraceContext populates Traceparent and Tracestate from the given context.
func (e *EventEnvelope) InjectTraceContext(ctx context.Context) {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	e.Traceparent = carrier["traceparent"]
	e.Tracestate = carrier["tracestate"]
}
