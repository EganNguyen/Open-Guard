package tenant

import (
	"context"
)

// ContextKey is a typed string for context keys to avoid collisions.
type ContextKey string

const (
	// OrgIDKey is the context key for the organization ID.
	OrgIDKey ContextKey = "org_id"
	// UserIDKey is the context key for the user ID.
	UserIDKey ContextKey = "user_id"
)

// OrgIDFromContext retrieves the org ID stored in context.
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(OrgIDKey).(string)
	return v
}

// UserIDFromContext retrieves the user ID stored in context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(UserIDKey).(string)
	return v
}
