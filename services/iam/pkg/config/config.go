package config

import (
	"fmt"
	"os"
	"strconv"
)

// IAMConfig holds all configuration for the IAM service.
type IAMConfig struct {
	Port         string
	JWTSecret    string
	JWTExpiry    int // seconds
	AppEnv       string
	LogLevel     string

	// PostgreSQL
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string
	PostgresSSLMode  string

	// Kafka
	KafkaBrokers string
}

// Load reads IAM configuration from environment variables.
func Load() *IAMConfig {
	return &IAMConfig{
		Port:      Default("IAM_PORT", "8081"),
		JWTSecret: Must("GATEWAY_JWT_SECRET"),
		JWTExpiry: DefaultInt("GATEWAY_JWT_EXPIRY", 3600),
		AppEnv:    Default("APP_ENV", "development"),
		LogLevel:  Default("LOG_LEVEL", "info"),

		PostgresHost:     Default("POSTGRES_HOST", "localhost"),
		PostgresPort:     Default("POSTGRES_PORT", "5432"),
		PostgresUser:     Default("POSTGRES_USER", "openguard"),
		PostgresPassword: Default("POSTGRES_PASSWORD", "change-me"),
		PostgresDB:       Default("POSTGRES_DB", "openguard"),
		PostgresSSLMode:  Default("POSTGRES_SSLMODE", "disable"),

		KafkaBrokers: Default("KAFKA_BROKERS", "localhost:9094"),
	}
}

// PostgresDSN returns the PostgreSQL connection string.
func (c *IAMConfig) PostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.PostgresUser, c.PostgresPassword,
		c.PostgresHost, c.PostgresPort,
		c.PostgresDB, c.PostgresSSLMode)
}

// Must returns the value of an environment variable or panics.
func Must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return v
}

// Default returns the value of an environment variable or a fallback.
func Default(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// DefaultInt returns an integer env variable or a fallback.
func DefaultInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
