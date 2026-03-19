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

	"github.com/openguard/iam/pkg/config"
	"github.com/openguard/iam/pkg/db"
	"github.com/openguard/iam/pkg/handlers"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/iam/pkg/router"
	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/outbox"
)

func main() {
	cfg := config.Load()

	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug": logLevel = slog.LevelDebug
	case "warn":  logLevel = slog.LevelWarn
	case "error": logLevel = slog.LevelError
	default:      logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	if cfg.AppEnv == "development" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler).With("service", "iam")

	// Global Context
	ctx, cancelGlobal := context.WithCancel(context.Background())
	defer cancelGlobal()

	// Database
	pool, err := db.Connect(ctx, cfg.PostgresDSN())
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("connected to PostgreSQL")

	// Kafka Producer
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := kafka.NewProducer(brokers, []string{
		kafka.TopicAuthEvents,
		kafka.TopicAuditTrail,
	}, logger)
	defer producer.Close()

	// Distributed Transaction Outbox
	outboxWriter := outbox.NewWriter()
	outboxRelay := outbox.NewRelay(pool, producer)
	
	// Start daemon loop in background
	go outboxRelay.Start(ctx)

	// Keyrings
	jwtKeyring := crypto.NewJWTKeyring(cfg.JWTKeys)
	aesKeyring := crypto.NewAESKeyring(cfg.MFAKeys)

	// Repositories
	orgRepo := repository.NewOrgRepository()
	userRepo := repository.NewUserRepository()
	sessionRepo := repository.NewSessionRepository()
	tokenRepo := repository.NewAPITokenRepository()
	mfaRepo := repository.NewMFARepository()

	// Services
	authService := service.NewAuthService(pool, userRepo, orgRepo, sessionRepo, mfaRepo, outboxWriter, logger, jwtKeyring, aesKeyring, cfg.JWTExpiry)
	userService := service.NewUserService(pool, userRepo, sessionRepo, tokenRepo, outboxWriter, logger)

	// Handlers
	authHandler := handlers.NewAuthHandler(authService)
	userHandler := handlers.NewUserHandler(userService)
	mfaHandler := handlers.NewMFAHandler()
	scimHandler := handlers.NewSCIMHandler()
	tokenHandler := handlers.NewTokenHandler(tokenRepo)

	// Router
	r := router.New(router.Config{
		AuthHandler:  authHandler,
		UserHandler:  userHandler,
		MFAHandler:   mfaHandler,
		SCIMHandler:  scimHandler,
		TokenHandler: tokenHandler,
		Logger:       logger,
	})

	// Setup mTLS Server
	caCert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		logger.Error("failed to load CA cert for mtls", "error", err)
		os.Exit(1)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		TLSConfig:    tlsConfig,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		logger.Info("IAM service mTLS server starting", "port", cfg.Port)
		if err := srv.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down IAM service...")
	// Cancel the root context to unblock the Relay daemon
	cancelGlobal()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	
	logger.Info("IAM service stopped")
}
