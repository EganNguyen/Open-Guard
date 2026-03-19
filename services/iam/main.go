package main

import (
	"context"
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
	"github.com/openguard/shared/kafka"
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
	logger := slog.New(handler).With("service", "iam")

	// Database
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.PostgresDSN())
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("connected to PostgreSQL")

	// Kafka producer (best-effort — service starts even if Kafka is down)
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := kafka.NewProducer(brokers, []string{
		kafka.TopicAuthEvents,
		kafka.TopicAuditTrail,
	}, logger)
	defer producer.Close()

	// Repositories
	orgRepo := repository.NewOrgRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	tokenRepo := repository.NewAPITokenRepository(pool)

	// Services
	authService := service.NewAuthService(userRepo, orgRepo, sessionRepo, producer, logger, cfg.JWTSecret, cfg.JWTExpiry)
	userService := service.NewUserService(userRepo, sessionRepo, tokenRepo, producer, logger)

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
		logger.Info("IAM service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down IAM service...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	pool.Close()
	producer.Close()
	logger.Info("IAM service stopped")
}
