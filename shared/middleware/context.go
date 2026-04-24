package middleware

import (
	"context"
	"time"
)

type contextKey string

const (
	UserIDKey      contextKey = "user_id"
	OrgIDKey       contextKey = "org_id"
	ConnectorIDKey contextKey = "connector_id"
	JTIKey         contextKey = "jti"
	ExpiresAtKey   contextKey = "expires_at"
)

// GetOrgID retrieves the organization ID from the context.
func GetOrgID(ctx context.Context) string {
	if id, ok := ctx.Value(OrgIDKey).(string); ok {
		return id
	}
	return ""
}

// GetUserID retrieves the user ID from the context.
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(UserIDKey).(string); ok {
		return id
	}
	return ""
}

// GetConnectorID retrieves the connector ID from the context.
func GetConnectorID(ctx context.Context) string {
	if id, ok := ctx.Value(ConnectorIDKey).(string); ok {
		return id
	}
	return ""
}

// GetJTI retrieves the JWT ID (JTI) from the context.
func GetJTI(ctx context.Context) string {
	if jti, ok := ctx.Value(JTIKey).(string); ok {
		return jti
	}
	return ""
}

// GetExpiresAt retrieves the token expiry time from the context.
func GetExpiresAt(ctx context.Context) time.Time {
	if exp, ok := ctx.Value(ExpiresAtKey).(time.Time); ok {
		return exp
	}
	return time.Time{}
}
