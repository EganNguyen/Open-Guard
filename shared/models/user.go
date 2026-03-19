package models

import "time"

type User struct {
	ID                 string     `json:"id" db:"id"`
	OrgID              string     `json:"org_id" db:"org_id"`
	Email              string     `json:"email" db:"email"`
	DisplayName        string     `json:"display_name" db:"display_name"`
	Status             UserStatus `json:"status" db:"status"`
	MFAEnabled         bool       `json:"mfa_enabled" db:"mfa_enabled"`
	MFAMethod          string     `json:"mfa_method,omitempty" db:"mfa_method"` // "totp" | "webauthn"
	SCIMExternalID     string     `json:"scim_external_id,omitempty" db:"scim_external_id"`
	ProvisioningStatus string     `json:"provisioning_status" db:"provisioning_status"` // "complete" | "pending" | "failed"
	TierIsolation      string     `json:"tier_isolation" db:"tier_isolation"`           // "shared" | "schema" | "shard"
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}

type UserStatus string

const (
	UserStatusActive             UserStatus = "active"
	UserStatusSuspended          UserStatus = "suspended"
	UserStatusDeprovisioned      UserStatus = "deprovisioned"
	UserStatusProvisioningFailed UserStatus = "provisioning_failed"
)
