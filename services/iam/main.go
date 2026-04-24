package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/iam/pkg/handlers"
	"github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/services/iam/pkg/router"
	"github.com/openguard/services/iam/pkg/seed"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/services/iam/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/database"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/kafka/outbox"
)

func main() {
	// Initialize slog (R-13)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "iam")
	slog.SetDefault(logger)

	// Initialize OpenTelemetry
	tp, err := telemetry.InitTracer()
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			logger.Error("failed to shutdown tracer", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// DB connection string
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		logger.Error("failed to parse db config", "error", err)
		os.Exit(1)
	}

	// Set connection pool limits per spec §2.10
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		logger.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	rOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error("failed to parse redis url", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(rOptions)
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("redis connection check failed", "error", err)
	}

	// Run Migrations
	if err := database.Migrate(ctx, pool, "migrations"); err != nil {
		logger.Error("migrations failed", "error", err)
	}

	// Auto-seed if requested
	if os.Getenv("SEED_DB") == "true" {
		if err := seed.Seed(ctx, pool); err != nil {
			logger.Error("seeding failed", "error", err)
		}
	}

	// Initialize AuthWorkerPool
	authPool := service.NewAuthWorkerPool(2 * runtime.NumCPU())

	// Load JWT Keyring
	keyringJSON := os.Getenv("IAM_JWT_KEYS")
	if keyringJSON == "" {
		// Default dev key if not provided - NOT FOR PRODUCTION
		keyringJSON = `[{"kid":"dev-key","secret":"dev-secret-at-least-32-chars-long-!!","algorithm":"HS256","status":"active"}]`
		logger.Warn("IAM_JWT_KEYS not set, using default dev key")
	}
	keyring, err := crypto.LoadKeyring(keyringJSON)
	if err != nil {
		logger.Error("failed to load JWT keyring", "error", err)
		os.Exit(1)
	}

	// Load AES Keyring for MFA
	aesKeyringJSON := os.Getenv("IAM_AES_KEYS")
	if aesKeyringJSON == "" {
		// Default dev key - 32 bytes base64 encoded
		aesKeyringJSON = `[{"kid":"dev-aes","key":"YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=","status":"active"}]`
		logger.Warn("IAM_AES_KEYS not set, using default dev key")
	}
	aesKeyring, err := crypto.LoadAESKeyring(aesKeyringJSON)
	if err != nil {
		logger.Error("failed to load AES keyring", "error", err)
		os.Exit(1)
	}

	// Initialize Repository, Service, Handler
	repo := repository.NewRepository(pool)
	svc := service.NewService(repo, authPool, keyring, aesKeyring, rdb)
	h := handlers.NewHandler(svc)

	// Setup Router
	r := router.NewRouter(h, keyring, rdb)

	// Initialize Kafka and Outbox Relay
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	kp := kafka.NewPublisher([]string{brokers})
	defer kp.Close()

	relayLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("component", "outbox-relay")
	relay := outbox.NewRelay(pool, kp, "outbox_records", 5*time.Second, relayLogger)
	
	// Start Outbox Relay in background
	go relay.Run(ctx)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
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
		Addr:      ":" + port,
		Handler:   r,
		TLSConfig: tlsConfig,
	}

	go func() {
		logger.Info("service starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
	
	fmt.Println("Service exited")
}
