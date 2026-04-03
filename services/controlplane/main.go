package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openguard/controlplane/pkg/config"
	"github.com/openguard/controlplane/pkg/db"
	"github.com/openguard/controlplane/pkg/handlers"
	"github.com/openguard/controlplane/pkg/repository"
	"github.com/openguard/controlplane/pkg/router"
	"github.com/openguard/controlplane/pkg/service"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/outbox"
	sharedcfg "github.com/openguard/shared/config"
	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Config
	cfg := config.Load()

	// Logger
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	isDev := cfg.AppEnv == "development"
	if isDev {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler).With("service", "controlplane")

	// Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis not available — rate limiting disabled", "error", err)
	} else {
		logger.Info("Redis connected", "addr", cfg.RedisAddr)
	}

	// Database
	pgPool, err := db.Connect(ctx, cfg.PostgresDSN())
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pgPool.Close()
	logger.Info("connected to PostgreSQL")

	// Load Keyring
	keyring := crypto.NewJWTKeyring(cfg.JWTKeys)

	// Load mTLS for backend services
	var tlsConfig *tls.Config
	if isDev {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else {
		caCertPath := sharedcfg.Default("CA_CERT_PATH", "/app/certs/ca.crt")
		tlsCertPath := sharedcfg.Default("TLS_CERT_PATH", "/app/certs/server.crt")
		tlsKeyPath := sharedcfg.Default("TLS_KEY_PATH", "/app/certs/server.key")

		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			logger.Error("failed to load CA cert for mtls", "error", err)
		} else {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)

			clientCert, err := tls.LoadX509KeyPair(tlsCertPath, tlsKeyPath)
			if err != nil {
				logger.Error("failed to load client keys for mtls", "error", err)
			} else {
				tlsConfig = &tls.Config{
					RootCAs:      caCertPool,
					Certificates: []tls.Certificate{clientCert},
				}
			}
		}
	}

	// Outbox
	brokers := strings.Split(sharedcfg.Default("KAFKA_BROKERS", "localhost:9092"), ",")
	producer := kafka.NewProducer(brokers, []string{kafka.TopicAuditTrail}, logger)
	defer producer.Close()

	outboxWriter := outbox.NewWriter()
	outboxWriter.TableName = "outbox_records"

	outboxRelay := outbox.NewRelay(pgPool, producer)
	outboxRelay.TableName = "outbox_records"
	go outboxRelay.Start(ctx)

	// Repositories (v2.0)
	repo := repository.New(pgPool, outboxWriter)

	// Service (v2.0)
	svc := service.New(repo, logger, isDev)

	// Handlers
	connectorHandler := handlers.NewConnectorHandler(svc)
	ingestHandler := handlers.NewIngestHandler(svc)

	// Build router
	r, err := router.New(router.Config{
		JWTKeyring:       keyring,
		APIKeyValidator:  repo, // Repository implements middleware.APIKeyValidator
		Redis:            rdb,
		ConnectorHandler: connectorHandler,
		IngestHandler:    ingestHandler,
		Logger:         logger,
		TLSConfig:      tlsConfig,
		IAMAddr:        sharedcfg.Default("IAM_TARGET", "http://localhost:8081"),
		PolicyAddr:     sharedcfg.Default("POLICY_TARGET", ""),
		ThreatAddr:     sharedcfg.Default("THREAT_TARGET", ""),
		AuditAddr:      sharedcfg.Default("AUDIT_TARGET", ""),
		AlertingAddr:   sharedcfg.Default("ALERTING_TARGET", ""),
		ComplianceAddr: sharedcfg.Default("COMPLIANCE_TARGET", ""),
		PublicBaseURL:  cfg.PublicBaseURL,
	})
	if err != nil {
		logger.Error("failed to build router", "error", err)
		os.Exit(1)
	}

	// HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		logger.Info("controlplane starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down controlplane...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	if err := rdb.Close(); err != nil {
		logger.Error("redis close error", "error", err)
	}
	logger.Info("controlplane stopped")
}
