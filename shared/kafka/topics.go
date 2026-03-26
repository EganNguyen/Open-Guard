package kafka

// Canonical Kafka topic names. Import these constants — do not hardcode strings.

const (
	TopicAuthEvents        = "auth.events"
	TopicPolicyChanges     = "policy.changes"
	TopicDataAccess        = "data.access"
	TopicThreatAlerts      = "threat.alerts"
	TopicAuditTrail        = "audit.trail"
	TopicNotificationsOut  = "notifications.outbound"
	TopicSagaOrchestration = "saga.orchestration"
	TopicOutboxDLQ         = "outbox.dlq"
	TopicConnectorEvents   = "connector.events"
)

const (
	GroupAudit      = "openguard-audit-v1"
	GroupThreat     = "openguard-threat-v1"
	GroupAlerting   = "openguard-alerting-v1"
	GroupCompliance = "openguard-compliance-v1"
	GroupPolicy     = "openguard-policy-v1"
	GroupSaga       = "openguard-saga-v1"
)
