package config

import (
	"fmt"
	"os"
	"strconv"
)

// GatewayConfig holds all configuration for the API Gateway.
type GatewayConfig struct {
	Port      string
	JWTSecret string
	JWTExpiry int // seconds
	RedisAddr string
	RedisPass string
	AppEnv    string
	LogLevel  string
}

// Load reads configuration from environment variables.
func Load() *GatewayConfig {
	return &GatewayConfig{
		Port:      Default("GATEWAY_PORT", "8080"),
		JWTSecret: Must("GATEWAY_JWT_SECRET"),
		JWTExpiry: DefaultInt("GATEWAY_JWT_EXPIRY", 3600),
		RedisAddr: Default("REDIS_ADDR", "localhost:6379"),
		RedisPass: Default("REDIS_PASSWORD", ""),
		AppEnv:    Default("APP_ENV", "development"),
		LogLevel:  Default("LOG_LEVEL", "info"),
	}
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

// MustInt parses an integer env variable or panics.
func MustInt(key string) int {
	v := Must(key)
	n, err := strconv.Atoi(v)
	if err != nil {
		panic(fmt.Sprintf("env variable %q must be an integer, got %q", key, v))
	}
	return n
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
