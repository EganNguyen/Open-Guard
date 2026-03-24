package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/audit/pkg/config"
	"github.com/openguard/audit/pkg/consumer"
	"github.com/openguard/audit/pkg/handlers"
	"github.com/openguard/audit/pkg/integrity"
	"github.com/openguard/audit/pkg/repository"
	"github.com/openguard/shared/kafka"
	"strings"
	
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	// Logger
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
	logger := slog.New(handler).With("service", "audit")

	ctx, cancelGlobal := context.WithCancel(context.Background())
	defer cancelGlobal()

	// Connect MongoDB
	primaryClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURIPrimary))
	if err != nil {
		logger.Error("failed to connect primary mongo", "error", err)
		os.Exit(1)
	}
	defer primaryClient.Disconnect(context.Background())

	secondaryClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURISecondary))
	if err != nil {
		logger.Error("failed to connect secondary mongo", "error", err)
		os.Exit(1)
	}
	defer secondaryClient.Disconnect(context.Background())

	// Init Repos
	writeRepo := repository.NewWriteRepository(primaryClient)
	readRepo := repository.NewReadRepository(secondaryClient)
	
	// Ensure Indexes
	if err := writeRepo.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure indexes", "error", err)
	}

	// Init Bulk Writer
	bulkWriter := consumer.NewBulkWriter(writeRepo.GetCollection(), cfg.BulkInsertDocs, cfg.BulkInsertFlush, logger)
	go bulkWriter.Start(ctx)

	topicsToConsume := []string{
		kafka.TopicAuthEvents,
		kafka.TopicPolicyChanges,
		kafka.TopicDataAccess,
		kafka.TopicThreatAlerts,
		kafka.TopicAuditTrail,
	}

	// Init Kafka Consumer
	auditConsumer := consumer.NewConsumer(
		strings.Split(cfg.KafkaBrokers, ","),
		topicsToConsume,
		writeRepo,
		bulkWriter,
		logger,
		cfg.HashChainSecret,
	)
	go auditConsumer.Start(ctx)

	// API Handlers
	verifier := integrity.NewVerifier(readRepo, cfg.HashChainSecret)
	auditHandler := handlers.NewEventsHandler(readRepo, verifier, logger)

	// Router
	r := chi.NewRouter()
	r.Get("/audit/events", auditHandler.ListEvents)
	r.Get("/audit/events/{id}", auditHandler.GetEvent)
	r.Get("/audit/integrity", auditHandler.VerifyIntegrity)

	// Server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("audit service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down audit service...")
	cancelGlobal()
	
	bulkWriter.Stop()
	auditConsumer.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	logger.Info("audit service stopped")
}
