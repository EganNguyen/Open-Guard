package config

import (
	"os"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Must set JSON arrays and Required path targets or Must calls will panic
	os.Setenv("IAM_JWT_KEYS_JSON", `[{"kid":"key1","alg":"EdDSA","key":"testkey12345678901234567890123456","active":true}]`)
	defer os.Unsetenv("IAM_JWT_KEYS_JSON")

	os.Setenv("IAM_MFA_ENCRYPTION_KEYS_JSON", `[{"kid":"aes1","key":"12345678901234567890123456789012","active":true}]`)
	defer os.Unsetenv("IAM_MFA_ENCRYPTION_KEYS_JSON")

	os.Setenv("TLS_CERT_PATH", "/tmp/cert")
	defer os.Unsetenv("TLS_CERT_PATH")

	os.Setenv("TLS_KEY_PATH", "/tmp/key")
	defer os.Unsetenv("TLS_KEY_PATH")

	os.Setenv("CA_CERT_PATH", "/tmp/ca")
	defer os.Unsetenv("CA_CERT_PATH")

	os.Setenv("IAM_PORT", "9999")
	defer os.Unsetenv("IAM_PORT")

	cfg := Load()
	assert.Equal(t, "9999", cfg.Port)
	assert.Equal(t, "/tmp/cert", cfg.TLSCertPath)
	assert.Len(t, cfg.JWTKeys, 1)
	assert.Equal(t, "key1", cfg.JWTKeys[0].Kid)
}

func TestPostgresDSN(t *testing.T) {
	cfg := &IAMConfig{
		PostgresUser:     "user",
		PostgresPassword: "pwd",
		PostgresHost:     "127.0.0.1",
		PostgresPort:     "5432",
		PostgresDB:       "iamdb",
		PostgresSSLMode:  "disable",
	}

	expected := "postgres://user:pwd@127.0.0.1:5432/iamdb?sslmode=disable"
	assert.Equal(t, expected, cfg.PostgresDSN())
}
