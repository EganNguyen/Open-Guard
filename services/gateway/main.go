package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openguard/gateway/pkg/config"
	"github.com/openguard/gateway/pkg/router"
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
	if cfg.AppEnv == "development" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(handler).With("service", "gateway")

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

	// Load Keyring
	keyring := crypto.NewJWTKeyring(cfg.JWTKeys)

	// Build router
	r, err := router.New(router.Config{
		JWTKeyring:     keyring,
		Redis:          rdb,
		Logger:         logger,
		IAMAddr:        sharedcfg.Default("IAM_TARGET", "http://localhost:8081"),
		PolicyAddr:     sharedcfg.Default("POLICY_TARGET", ""),
		ThreatAddr:     sharedcfg.Default("THREAT_TARGET", ""),
		AuditAddr:      sharedcfg.Default("AUDIT_TARGET", ""),
		AlertingAddr:   sharedcfg.Default("ALERTING_TARGET", ""),
		ComplianceAddr: sharedcfg.Default("COMPLIANCE_TARGET", ""),
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
		logger.Info("gateway starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down gateway...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	if err := rdb.Close(); err != nil {
		logger.Error("redis close error", "error", err)
	}
	logger.Info("gateway stopped")
}
