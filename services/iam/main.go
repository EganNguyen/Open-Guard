package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/openguard/services/iam/pkg/handlers"
	"github.com/openguard/services/iam/pkg/logger"
	"github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/services/iam/pkg/router"
	"github.com/openguard/services/iam/pkg/seed"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/services/iam/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/kafka/outbox"
)

func main() {
	// Initialize logger
	logger.Init()
	defer logger.Log.Sync()
	log := logger.Log.With(zap.String("service", "iam"))

	// Initialize OpenTelemetry
	tp, err := telemetry.InitTracer()
	if err != nil {
		log.Fatal("failed to initialize tracer", zap.Error(err))
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Error("failed to shutdown tracer", zap.Error(err))
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
		log.Fatal("failed to parse db config", zap.Error(err))
	}

	// Set connection pool limits per spec §2.10
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatal("failed to connect to db", zap.Error(err))
	}
	defer pool.Close()

	// Initialize Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	rOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal("failed to parse redis url", zap.Error(err))
	}
	rdb := redis.NewClient(rOptions)
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Warn("redis connection check failed", zap.Error(err))
	}

	// Auto-seed if requested
	if os.Getenv("SEED_DB") == "true" {
		if err := seed.Seed(ctx, pool); err != nil {
			log.Error("seeding failed", zap.Error(err))
		}
	}

	// Initialize AuthWorkerPool
	authPool := service.NewAuthWorkerPool(2 * runtime.NumCPU())

	// Load JWT Keyring
	keyringJSON := os.Getenv("IAM_JWT_KEYS")
	if keyringJSON == "" {
		// Default dev key if not provided - NOT FOR PRODUCTION
		keyringJSON = `[{"kid":"dev-key","secret":"dev-secret-at-least-32-chars-long-!!","algorithm":"HS256","status":"active"}]`
		log.Warn("IAM_JWT_KEYS not set, using default dev key")
	}
	keyring, err := crypto.LoadKeyring(keyringJSON)
	if err != nil {
		log.Fatal("failed to load JWT keyring", zap.Error(err))
	}

	// Load AES Keyring for MFA
	aesKeyringJSON := os.Getenv("IAM_AES_KEYS")
	if aesKeyringJSON == "" {
		// Default dev key - 32 bytes base64 encoded
		aesKeyringJSON = `[{"kid":"dev-aes","key":"YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE=","status":"active"}]`
		log.Warn("IAM_AES_KEYS not set, using default dev key")
	}
	aesKeyring, err := crypto.LoadAESKeyring(aesKeyringJSON)
	if err != nil {
		log.Fatal("failed to load AES keyring", zap.Error(err))
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

	relay := outbox.NewRelay(pool, kp, "outbox_records", 5*time.Second, slog.Default())
	
	// Start Outbox Relay in background
	go relay.Run(ctx)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Info("service starting", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown failed", zap.Error(err))
	}
	
	fmt.Println("Service exited")
}
