package models

import (
	"time"
)

// AuditEvent represents the event as stored in MongoDB.
type AuditEvent struct {
	EventID       string    `bson:"event_id" json:"event_id"`
	OrgID         string    `bson:"org_id" json:"org_id"`
	Type          string    `bson:"type" json:"type"`
	OccurredAt    time.Time `bson:"occurred_at" json:"occurred_at"`
	ActorID       string    `bson:"actor_id,omitempty" json:"actor_id,omitempty"`
	TargetID      string    `bson:"target_id,omitempty" json:"target_id,omitempty"`
	Payload       any       `bson:"payload,omitempty" json:"payload,omitempty"`
	ClientIP      string    `bson:"client_ip,omitempty" json:"client_ip,omitempty"`
	UserAgent     string    `bson:"user_agent,omitempty" json:"user_agent,omitempty"`
	TraceID       string    `bson:"trace_id,omitempty" json:"trace_id,omitempty"`

	// Integrity fields
	ChainHash     string    `bson:"chain_hash" json:"chain_hash"`
	PrevChainHash string    `bson:"prev_chain_hash" json:"prev_chain_hash"`
	ChainSeq      int64     `bson:"chain_seq" json:"chain_seq"`
}
