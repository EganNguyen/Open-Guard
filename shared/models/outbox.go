package models

import "time"

// OutboxRecord is persisted in the same transaction as the business operation.
// The relay process reads pending records and publishes to Kafka.
type OutboxRecord struct {
	ID          string     `db:"id"`           // UUIDv4
	Topic       string     `db:"topic"`        // Kafka topic name
	Key         string     `db:"key"`          // Kafka partition key (usually org_id)
	Payload     []byte     `db:"payload"`      // JSON-encoded EventEnvelope
	Status      string     `db:"status"`       // "pending" | "published" | "dead"
	Attempts    int        `db:"attempts"`     // number of publish attempts
	LastError   string     `db:"last_error"`   // last error message
	CreatedAt   time.Time  `db:"created_at"`
	PublishedAt *time.Time `db:"published_at"`
	DeadAt      *time.Time `db:"dead_at"`
}
