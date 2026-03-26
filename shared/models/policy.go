package models

import (
	"encoding/json"
	"time"
)

// Policy is the canonical policy model shared across all services.
type Policy struct {
	ID          string          `json:"id" db:"id"`
	OrgID       string          `json:"org_id" db:"org_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	Type        PolicyType      `json:"type" db:"type"`
	Rules       json.RawMessage `json:"rules" db:"rules"`
	Enabled     bool            `json:"enabled" db:"enabled"`
	CreatedBy   string          `json:"created_by" db:"created_by"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// PolicyType represents the kind of security policy.
type PolicyType string

const (
	PolicyTypeDataExport   PolicyType = "data_export"
	PolicyTypeAnonAccess   PolicyType = "anon_access"
	PolicyTypeIPAllowlist  PolicyType = "ip_allowlist"
	PolicyTypeSessionLimit PolicyType = "session_limit"
	PolicyTypeRBAC         PolicyType = "rbac"
)
