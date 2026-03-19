package models

import "time"

// User is the canonical user model shared across all services.
type User struct {
	ID           string     `json:"id" db:"id"`
	OrgID        string     `json:"org_id" db:"org_id"`
	Email        string     `json:"email" db:"email"`
	DisplayName  string     `json:"display_name" db:"display_name"`
	Status       UserStatus `json:"status" db:"status"`
	MFAEnabled   bool       `json:"mfa_enabled" db:"mfa_enabled"`
	SCIMExternal string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

// UserStatus represents the lifecycle state of a user.
type UserStatus string

const (
	UserStatusActive        UserStatus = "active"
	UserStatusSuspended     UserStatus = "suspended"
	UserStatusDeprovisioned UserStatus = "deprovisioned"
)
