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

	"github.com/openguard/policy/pkg/config"
	"github.com/openguard/policy/pkg/db"
	"github.com/openguard/policy/pkg/handlers"
	"github.com/openguard/policy/pkg/repository"
	"github.com/openguard/policy/pkg/router"
	"github.com/openguard/policy/pkg/service"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/outbox"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	// ── Logger ──────────────────────────────────────────
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
	logger := slog.New(handler).With("service", "policy")

	ctx, cancelGlobal := context.WithCancel(context.Background())
	defer cancelGlobal()

	// ── PostgreSQL ──────────────────────────────────────
	pool, err := db.Connect(ctx, cfg.PostgresDSN())
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("connected to PostgreSQL")

	// ── Redis ───────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		logger.Warn("Redis not available — cache disabled", "error", err)
	} else {
		logger.Info("connected to Redis", "addr", cfg.RedisAddr)
	}
	defer rdb.Close()

	// ── Kafka Producer + Outbox Relay ───────────────────
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := kafka.NewProducer(brokers, []string{kafka.TopicPolicyChanges, kafka.TopicAuditTrail}, logger)
	defer producer.Close()

	outboxWriter := outbox.NewWriter()
	outboxWriter.TableName = "policy_outbox_records"

	outboxRelay := outbox.NewRelay(pool, producer)
	outboxRelay.TableName = "policy_outbox_records"
	go outboxRelay.Start(ctx)

	// ── Repositories ────────────────────────────────────
	policyRepo := repository.NewPolicyRepository(pool, outboxWriter)

	// ── Services ────────────────────────────────────────
	policySvc := service.NewPolicyService(policyRepo)
	evaluatorSvc := service.NewEvaluatorService(policyRepo, rdb, cfg.CacheTTLSeconds, logger)

	// ── Cache Invalidation Consumer ─────────────────────
	cacheInvalidator := service.NewCacheInvalidator(evaluatorSvc, brokers, logger)
	go cacheInvalidator.Start(ctx)

	// ── Handlers + Router ───────────────────────────────
	policyHandler := handlers.NewPolicyHandler(policySvc, evaluatorSvc, logger)
	r := router.New(router.Config{
		PolicyHandler: policyHandler,
		Logger:        logger,
	})

	// ── HTTP Server ─────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("policy service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// ── Graceful Shutdown ───────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down policy service...")
	cancelGlobal()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	logger.Info("policy service stopped")
}
