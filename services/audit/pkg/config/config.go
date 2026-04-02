package config

import (
	sharedcfg "github.com/openguard/shared/config"
	"time"
)

type AuditConfig struct {
	Port     string
	AppEnv   string
	LogLevel string

	// Kafka
	KafkaBrokers string

	// MongoDB
	MongoURIPrimary   string
	MongoURISecondary string

	// Audit specific
	HashChainSecret string
	BulkInsertDocs  int
	BulkInsertFlush time.Duration
}

func Load() *AuditConfig {
	return &AuditConfig{
		Port:     sharedcfg.Default("AUDIT_PORT", "8083"),
		AppEnv:   sharedcfg.Default("APP_ENV", "development"),
		LogLevel: sharedcfg.Default("LOG_LEVEL", "info"),

		KafkaBrokers: sharedcfg.Default("KAFKA_BROKERS", "localhost:9094"),

		MongoURIPrimary:   sharedcfg.Must("MONGO_URI_PRIMARY"),
		MongoURISecondary: sharedcfg.Must("MONGO_URI_SECONDARY"),

		HashChainSecret: sharedcfg.Must("AUDIT_HASH_CHAIN_SECRET"),
		BulkInsertDocs:  sharedcfg.DefaultInt("AUDIT_BULK_INSERT_MAX_DOCS", 500),
		BulkInsertFlush: time.Duration(sharedcfg.DefaultInt("AUDIT_BULK_INSERT_FLUSH_MS", 1000)) * time.Millisecond,
	}
}
