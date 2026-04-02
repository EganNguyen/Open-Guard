package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port              string
	PostgresURL       string
	OpenGuardURL      string
	ConnectorAPIKey   string
	WebhookSecret     string
	OIDCIssuer        string
	ClientID          string
	ClientSecret      string
	FrontendURL       string
	BatchSize         int
	FlushIntervalMS   int
	PolicyCacheSize   int
}

func MustLoad() Config {
	return Config{
		Port:            getEnv("PORT", "8082"),
		PostgresURL:     getEnv("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/todoapp?sslmode=disable"),
		OpenGuardURL:    getEnv("OPENGUARD_URL", "http://localhost:8080"),
		ConnectorAPIKey: getEnv("TODO_OPENGUARD_API_KEY", "test-connector-key"),
		WebhookSecret:   getEnv("OPENGUARD_WEBHOOK_SECRET", "test-webhook-secret"),
		OIDCIssuer:      getEnv("OPENGUARD_OIDC_ISSUER", "http://localhost:8081"),
		ClientID:        getEnv("OPENGUARD_OIDC_CLIENT_ID", "todo-app"),
		ClientSecret:    getEnv("OPENGUARD_OIDC_CLIENT_SECRET", "todo-app-secret"),
		FrontendURL:     getEnv("FRONTEND_URL", "http://localhost:8082"),
		BatchSize:       getEnvInt("SDK_EVENT_BATCH_SIZE", 100),
		FlushIntervalMS: getEnvInt("SDK_EVENT_FLUSH_INTERVAL_MS", 2000),
		PolicyCacheSize: getEnvInt("SDK_POLICY_CACHE_SIZE", 1000),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}
