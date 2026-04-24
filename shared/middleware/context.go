package middleware

import "context"

type contextKey string

const (
	UserIDKey      contextKey = "user_id"
	OrgIDKey       contextKey = "org_id"
	ConnectorIDKey contextKey = "connector_id"
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
