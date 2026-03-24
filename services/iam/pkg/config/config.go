package config

import (
	"fmt"

	sharedcfg "github.com/openguard/shared/config"
	"github.com/openguard/shared/crypto"
)

type IAMConfig struct {
	Port      string
	JWTKeys   []crypto.JWTKey
	MFAKeys   []crypto.AESKey
	JWTExpiry int
	AppEnv    string
	LogLevel  string

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

	// Kafka
	KafkaBrokers string
}

func Load() *IAMConfig {
	var jwtKeys []crypto.JWTKey
	sharedcfg.MustJSON("IAM_JWT_KEYS_JSON", &jwtKeys)

	var mfaKeys []crypto.AESKey
	sharedcfg.MustJSON("IAM_MFA_ENCRYPTION_KEYS_JSON", &mfaKeys)

	return &IAMConfig{
		Port:      sharedcfg.Default("IAM_PORT", "8081"),
		JWTKeys:   jwtKeys,
		MFAKeys:   mfaKeys,
		JWTExpiry: sharedcfg.DefaultInt("IAM_JWT_EXPIRY", 3600),
		AppEnv:    sharedcfg.Default("APP_ENV", "development"),
		LogLevel:  sharedcfg.Default("LOG_LEVEL", "info"),

		TLSCertPath: sharedcfg.Must("TLS_CERT_PATH"),
		TLSKeyPath:  sharedcfg.Must("TLS_KEY_PATH"),
		CACertPath:  sharedcfg.Must("CA_CERT_PATH"),

		PostgresHost:     sharedcfg.Default("POSTGRES_HOST", "localhost"),
		PostgresPort:     sharedcfg.Default("POSTGRES_PORT", "5432"),
		PostgresUser:     sharedcfg.Default("POSTGRES_USER", "openguard"),
		PostgresPassword: sharedcfg.Default("POSTGRES_PASSWORD", "change-me"),
		PostgresDB:       sharedcfg.Default("POSTGRES_DB", "openguard"),
		PostgresSSLMode:  sharedcfg.Default("POSTGRES_SSLMODE", "disable"),

		KafkaBrokers: sharedcfg.Default("KAFKA_BROKERS", "localhost:9094"),
	}
}

func (c *IAMConfig) PostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.PostgresUser, c.PostgresPassword,
		c.PostgresHost, c.PostgresPort,
		c.PostgresDB, c.PostgresSSLMode)
}
