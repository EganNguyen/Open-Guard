package config

import (
	sharedcfg "github.com/openguard/shared/config"
	"github.com/openguard/shared/crypto"
)

// GatewayConfig holds all configuration for the API Gateway.
type GatewayConfig struct {
	Port      string
	JWTKeys   []crypto.JWTKey
	JWTExpiry int // seconds
	RedisAddr string
	RedisPass string
	AppEnv    string
	LogLevel  string
}

// Load reads configuration from environment variables.
func Load() *GatewayConfig {
	var keys []crypto.JWTKey
	sharedcfg.MustJSON("GATEWAY_JWT_KEYS_JSON", &keys)

	return &GatewayConfig{
		Port:      sharedcfg.Default("GATEWAY_PORT", "8080"),
		JWTKeys:   keys,
		JWTExpiry: sharedcfg.DefaultInt("GATEWAY_JWT_EXPIRY", 3600),
		RedisAddr: sharedcfg.Default("REDIS_ADDR", "localhost:6379"),
		RedisPass: sharedcfg.Default("REDIS_PASSWORD", ""),
		AppEnv:    sharedcfg.Default("APP_ENV", "development"),
		LogLevel:  sharedcfg.Default("LOG_LEVEL", "info"),
	}
}
