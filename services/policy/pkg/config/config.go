package config

import (
	"fmt"

	sharedcfg "github.com/openguard/shared/config"
)

// PolicyConfig holds all configuration for the Policy service.
type PolicyConfig struct {
	Port     string
	AppEnv   string
	LogLevel string

	// mTLS
	TLSCertPath string
	TLSKeyPath  string
	CACertPath  string

	// PostgreSQL
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string

	// Redis
	RedisAddr string
	RedisPass string

	// Kafka
	KafkaBrokers string

	// Cache TTL in seconds
	CacheTTLSeconds int
}

// Load reads all required and optional environment variables.
func Load() *PolicyConfig {
	return &PolicyConfig{
		Port:     sharedcfg.Default("POLICY_PORT", "8082"),
		AppEnv:   sharedcfg.Default("APP_ENV", "development"),
		LogLevel: sharedcfg.Default("LOG_LEVEL", "info"),

		TLSCertPath: sharedcfg.Default("TLS_CERT_PATH", "/certs/policy.crt"),
		TLSKeyPath:  sharedcfg.Default("TLS_KEY_PATH", "/certs/policy.key"),
		CACertPath:  sharedcfg.Default("CA_CERT_PATH", "/certs/ca.crt"),

		PostgresHost:     sharedcfg.Default("POSTGRES_HOST", "localhost"),
		PostgresPort:     sharedcfg.Default("POSTGRES_PORT", "5432"),
		PostgresUser:     sharedcfg.Default("POSTGRES_USER", "openguard"),
		PostgresPassword: sharedcfg.Default("POSTGRES_PASSWORD", "change-me"),
		PostgresDB:       sharedcfg.Default("POSTGRES_DB", "openguard"),
		PostgresSSLMode:  sharedcfg.Default("POSTGRES_SSLMODE", "disable"),

		RedisAddr: sharedcfg.Default("REDIS_ADDR", "localhost:6379"),
		RedisPass: sharedcfg.Default("REDIS_PASSWORD", ""),

		KafkaBrokers: sharedcfg.Default("KAFKA_BROKERS", "localhost:9094"),

		CacheTTLSeconds: sharedcfg.DefaultInt("POLICY_CACHE_TTL_SECONDS", 30),
	}
}

// PostgresDSN constructs the PostgreSQL Data Source Name.
func (c *PolicyConfig) PostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.PostgresUser, c.PostgresPassword,
		c.PostgresHost, c.PostgresPort,
		c.PostgresDB, c.PostgresSSLMode)
}
