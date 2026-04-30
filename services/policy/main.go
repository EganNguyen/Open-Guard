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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/services/policy/pkg/handlers"
	"github.com/openguard/services/policy/pkg/repository"
	"github.com/openguard/services/policy/pkg/router"
	"github.com/openguard/services/policy/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/database"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/kafka/outbox"
	"github.com/openguard/shared/secrets"
	"github.com/openguard/shared/telemetry"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(telemetry.NewSafeHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))).With("service", "policy")
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

	pgConfig.AfterRelease = func(conn *pgx.Conn) bool {
		ctx := context.Background()
		_, _ = conn.Exec(ctx, "SELECT set_config('app.org_id', '', false)")
		return true
	}

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
	defer func() { _ = rdb.Close() }()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("redis connection check failed — migrations cannot run", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to redis")

	// Run Migrations with distributed lock (INFRA-02)
	lockKey := "migrate:lock:policy"
	err = database.RunWithLock(ctx, rdb, lockKey, logger, func(ctx context.Context) error {
		return database.Migrate(ctx, pool, "migrations")
	})
	if err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	// ── Kafka + Outbox ────────────────────────────────────────────────────────
	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		brokersEnv = "localhost:9092"
	}
	brokers := strings.Split(brokersEnv, ",")
	kp := kafka.NewPublisher(brokers)
	defer func() { _ = kp.Close() }()

	outboxWriter := outbox.NewWriter("outbox_records")
	relay := outbox.NewRelay(pool, kp, "outbox_records", 5*time.Second, logger)
	go relay.Run(ctx)

	repo := repository.NewRepository(pool)
	svc := service.NewService(repo, rdb, outboxWriter, logger)
	h := handlers.NewHandler(svc, logger)

	// ── Auth Configuration ───────────────────────────────────────────────────
	secretProvider, err := secrets.GetProvider(ctx)
	if err != nil {
		logger.Error("failed to initialize secrets provider", "error", err)
		os.Exit(1)
	}

	keyringJSON, err := secretProvider.GetSecret(ctx, "IAM_JWT_KEYS")
	if err != nil {
		keyringJSON = `[{"kid":"dev-key","secret":"dev-secret-at-least-32-chars-long-!!","algorithm":"HS256","status":"active"}]`
		logger.Warn("IAM_JWT_KEYS not found in secrets provider, using default dev key", "error", err)
	}
	keyring, err := crypto.LoadKeyring(keyringJSON)
	if err != nil {
		logger.Error("failed to load JWT keyring", "error", err)
		os.Exit(1)
	}

	stopCh := make(chan struct{})

	r := router.NewRouter(h, keyring, rdb, stopCh)

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
		if tlsConfig != nil {
			if err := srv.ListenAndServeTLS("/certs/policy.crt", "/certs/policy.key"); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "error", err)
				os.Exit(1)
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "error", err)
				os.Exit(1)
			}
		}
	}()

	<-ctx.Done()
	close(stopCh)
	logger.Info("shutting down policy service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
	logger.Info("policy service exited")
}
