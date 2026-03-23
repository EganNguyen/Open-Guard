package config

import (
	"os"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Set an env var to override default
	os.Setenv("POLICY_PORT", "9999")
	defer os.Unsetenv("POLICY_PORT")

	cfg := Load()
	assert.Equal(t, "9999", cfg.Port)
	assert.Equal(t, "localhost", cfg.PostgresHost) // Default
}

func TestPostgresDSN(t *testing.T) {
	cfg := &PolicyConfig{
		PostgresUser:     "user",
		PostgresPassword: "pwd",
		PostgresHost:     "127.0.0.1",
		PostgresPort:     "5432",
		PostgresDB:       "testdb",
		PostgresSSLMode:  "disable",
	}

	expected := "postgres://user:pwd@127.0.0.1:5432/testdb?sslmode=disable"
	assert.Equal(t, expected, cfg.PostgresDSN())
}
