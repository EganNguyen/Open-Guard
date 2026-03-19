package kafka

// Canonical Kafka topic names. Import these constants — do not hardcode strings.
const (
	TopicAuthEvents       = "auth.events"
	TopicPolicyChanges    = "policy.changes"
	TopicDataAccess       = "data.access"
	TopicThreatAlerts     = "threat.alerts"
	TopicAuditTrail       = "audit.trail"
	TopicNotificationsOut = "notifications.outbound"
)
