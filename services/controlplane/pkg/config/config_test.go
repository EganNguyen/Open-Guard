package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Set mock environment variables
	os.Setenv("CONTROLPLANE_PORT", "9090")
	os.Setenv("CONTROLPLANE_JWT_KEYS_JSON", `[{"kid":"k1","secret":"s1","algorithm":"HS256","status":"active"}]`)
	os.Setenv("CONTROLPLANE_JWT_EXPIRY", "7200")
	os.Setenv("REDIS_ADDR", "localhost:6380")
	os.Setenv("REDIS_PASSWORD", "secret")
	os.Setenv("APP_ENV", "production")
	os.Setenv("LOG_LEVEL", "debug")

	defer func() {
		os.Unsetenv("CONTROLPLANE_PORT")
		os.Unsetenv("CONTROLPLANE_JWT_KEYS_JSON")
		os.Unsetenv("CONTROLPLANE_JWT_EXPIRY")
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("REDIS_PASSWORD")
		os.Unsetenv("APP_ENV")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg := Load()

	assert.Equal(t, "9090", cfg.Port)
	assert.Len(t, cfg.JWTKeys, 1)
	assert.Equal(t, "k1", cfg.JWTKeys[0].Kid)
	assert.Equal(t, 7200, cfg.JWTExpiry)
	assert.Equal(t, "localhost:6380", cfg.RedisAddr)
	assert.Equal(t, "secret", cfg.RedisPass)
	assert.Equal(t, "production", cfg.AppEnv)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_Defaults(t *testing.T) {
	// Ensure env is clean
	os.Unsetenv("CONTROLPLANE_PORT")
	os.Unsetenv("CONTROLPLANE_JWT_KEYS_JSON")
	os.Unsetenv("CONTROLPLANE_JWT_EXPIRY")
	os.Unsetenv("REDIS_ADDR")
	os.Unsetenv("REDIS_PASSWORD")
	os.Unsetenv("APP_ENV")
	os.Unsetenv("LOG_LEVEL")

	// Pre-setting keys JSON because MustJSON will panic/exit if missing
	os.Setenv("CONTROLPLANE_JWT_KEYS_JSON", "[]")
	defer os.Unsetenv("CONTROLPLANE_JWT_KEYS_JSON")

	cfg := Load()

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, 3600, cfg.JWTExpiry)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
	assert.Equal(t, "development", cfg.AppEnv)
	assert.Equal(t, "info", cfg.LogLevel)
}
