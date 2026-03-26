package config

import (
	sharedcfg "github.com/openguard/shared/config"
	"github.com/openguard/shared/crypto"
)

// ControlPlaneConfig holds all configuration for the Control Plane.
type ControlPlaneConfig struct {
	Port      string
	JWTKeys   []crypto.JWTKey
	JWTExpiry int // seconds
	RedisAddr    string
	RedisPass    string
	PostgresHost string
	PostgresPort string
	PostgresUser string
	PostgresPass string
	PostgresDB   string
	AppEnv       string
	LogLevel     string
}

func (c *ControlPlaneConfig) PostgresDSN() string {
	return "postgres://" + c.PostgresUser + ":" + c.PostgresPass + "@" + c.PostgresHost + ":" + c.PostgresPort + "/" + c.PostgresDB + "?sslmode=disable"
}

// Load reads configuration from environment variables.
func Load() *ControlPlaneConfig {
	var keys []crypto.JWTKey
	sharedcfg.MustJSON("CONTROLPLANE_JWT_KEYS_JSON", &keys)

	return &ControlPlaneConfig{
		Port:      sharedcfg.Default("CONTROLPLANE_PORT", "8080"),
		JWTKeys:   keys,
		JWTExpiry: sharedcfg.DefaultInt("CONTROLPLANE_JWT_EXPIRY", 3600),
		RedisAddr:    sharedcfg.Default("REDIS_ADDR", "localhost:6379"),
		RedisPass:    sharedcfg.Default("REDIS_PASSWORD", ""),
		PostgresHost: sharedcfg.Default("POSTGRES_HOST", "localhost"),
		PostgresPort: sharedcfg.Default("POSTGRES_PORT", "5432"),
		PostgresUser: sharedcfg.Default("POSTGRES_USER", "openguard"),
		PostgresPass: sharedcfg.Default("POSTGRES_PASSWORD", "change-me"),
		PostgresDB:   sharedcfg.Default("POSTGRES_DB", "openguard"),
		AppEnv:       sharedcfg.Default("APP_ENV", "development"),
		LogLevel:     sharedcfg.Default("LOG_LEVEL", "info"),
	}
}
