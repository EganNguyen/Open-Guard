package models

// SagaEvent wraps an EventEnvelope with saga orchestration metadata.
type SagaEvent struct {
	EventEnvelope
	SagaID       string `json:"saga_id"`             // UUIDv4, same across all steps
	SagaType     string `json:"saga_type"`           // "user.provision", "user.deprovision"
	SagaStep     int    `json:"saga_step"`           // 1-based step number
	Compensation bool   `json:"compensation"`        // true = this is a rollback event
	CausedBy     string `json:"caused_by,omitempty"` // event ID that caused this step
}
