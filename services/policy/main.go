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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/services/policy/pkg/handlers"
	"github.com/openguard/services/policy/pkg/repository"
	"github.com/openguard/services/policy/pkg/router"
	"github.com/openguard/services/policy/pkg/service"
	"github.com/openguard/shared/database"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/kafka/outbox"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", "policy")
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Database ──────────────────────────────────────────────────────────────
	dbURL := os.Getenv("POLICY_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
		logger.Warn("POLICY_DATABASE_URL not set, using default")
	}

	pgConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		logger.Error("failed to parse db config", "error", err)
		os.Exit(1)
	}
	// Connection pool per spec §2.10 (policy service is lighter than IAM)
	pgConfig.MaxConns = 10
	pgConfig.MinConns = 2
	pgConfig.MaxConnLifetime = 30 * time.Minute
	pgConfig.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to database")

	// Run Migrations
	if err := database.Migrate(ctx, pool, "migrations"); err != nil {
		logger.Error("migrations failed", "error", err)
	}

	// ── Redis ─────────────────────────────────────────────────────────────────
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/1" // db 1 for policy, db 0 for IAM
		logger.Warn("REDIS_URL not set, using default")
	}

	rOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error("failed to parse redis url", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(rOptions)
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("redis connection check failed — policy caching disabled", "error", err)
		rdb = nil
	} else {
		logger.Info("connected to redis")
	}

	// ── Kafka + Outbox ────────────────────────────────────────────────────────
	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		brokersEnv = "localhost:9092"
	}
	brokers := strings.Split(brokersEnv, ",")
	kp := kafka.NewPublisher(brokers)
	defer kp.Close()

	outboxWriter := outbox.NewWriter("outbox_records")
	relay := outbox.NewRelay(pool, kp, "outbox_records", 5*time.Second, logger)
	go relay.Run(ctx)

	// ── Service + Handler + Router ────────────────────────────────────────────
	repo := repository.NewRepository(pool)
	svc := service.NewService(repo, rdb, outboxWriter, logger)
	h := handlers.NewHandler(svc, repo, logger)
	r := router.NewRouter(h)

	// ── HTTP Server ───────────────────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	var tlsConfig *tls.Config
	if _, err := os.Stat("/certs/ca.crt"); err == nil {
		caCert, err := os.ReadFile("/certs/ca.crt")
		if err == nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig = &tls.Config{
				ClientCAs:  caCertPool,
				ClientAuth: tls.VerifyClientCertIfGiven,
			}
			logger.Info("mTLS configured")
		}
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		TLSConfig:    tlsConfig,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 35 * time.Second, // > 30ms SLO + buffer
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("policy service starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down policy service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
	logger.Info("policy service exited")
}
