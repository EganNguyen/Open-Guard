package models

import "time"

type Connector struct {
	ID          string    `json:"id" db:"id"`
	OrgID       string    `json:"org_id" db:"org_id"`
	Name        string    `json:"name" db:"name"`
	WebhookURL  string    `json:"webhook_url" db:"webhook_url"`
	APIKey      string    `json:"api_key,omitempty" db:"api_key"` // Hashed or encrypted in DB, usually not returned in list
	Status      string    `json:"status" db:"status"`             // "active", "suspended", "pending"
	CreatedBy   string    `json:"created_by" db:"created_by"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
