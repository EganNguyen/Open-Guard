package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/compliance/pkg/consumer"
	"github.com/openguard/services/compliance/pkg/handlers"
	"github.com/openguard/services/compliance/pkg/repository"
	"github.com/openguard/services/compliance/pkg/router"
	"github.com/openguard/services/compliance/pkg/storage"
	"github.com/openguard/services/compliance/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Initialize OpenTelemetry (INFRA-04)
	tp, err := telemetry.InitTracer()
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

	// Config
	clickhouseAddr := os.Getenv("CLICKHOUSE_ADDR")
	if clickhouseAddr == "" {
		clickhouseAddr = "localhost:9000"
	}
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}

	// ── Redis (Blocklist) ────────────────────────────────────────────────────
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

	// ── PostgreSQL (Reports) ─────────────────────────────────────────────────
	pgURL := os.Getenv("DATABASE_URL")
	if pgURL == "" {
		pgURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
	}
	pgPool, err := pgxpool.New(context.Background(), pgURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pgPool.Close()

	// ── S3/MinIO Storage ─────────────────────────────────────────────────────
	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "http://localhost:9000"
	}
	s3AccessKey := os.Getenv("S3_ACCESS_KEY")
	s3SecretKey := os.Getenv("S3_SECRET_KEY")
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		s3Bucket = "compliance-reports"
	}
	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		s3Region = "us-east-1"
	}

	s3Storage, err := storage.NewS3Storage(s3Endpoint, s3AccessKey, s3SecretKey, s3Bucket, s3Region)
	if err != nil {
		logger.Error("failed to initialize s3 storage", "error", err)
		os.Exit(1)
	}

	var keyring []crypto.JWTKey
	if keysJSON := os.Getenv("JWT_KEYS"); keysJSON != "" {
		if err := json.Unmarshal([]byte(keysJSON), &keyring); err != nil {
			logger.Error("failed to parse JWT_KEYS", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Warn("JWT_KEYS not set, using default development key")
		keyring = []crypto.JWTKey{{Kid: "dev", Secret: "default-secret", Algorithm: "HS256", Status: "active"}}
	}

	concurrencyStr := os.Getenv("COMPLIANCE_REPORT_MAX_CONCURRENT")
	if concurrencyStr == "" {
		concurrencyStr = "10" // spec default
	}
	concurrency, _ := strconv.Atoi(concurrencyStr)
	bulkhead := resilience.NewBulkhead(concurrency)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 1. Initialize ClickHouse
	repo, err := repository.NewRepository(clickhouseAddr, pgPool)
	if err != nil {
		logger.Error("failed to connect to clickhouse", "error", err)
		os.Exit(1)
	}
	if err := repo.InitSchema(context.Background()); err != nil {
		logger.Error("failed to init schema", "error", err)
		os.Exit(1)
	}

	// 2. Start Kafka Consumer for Ingestion
	cw := consumer.NewClickHouseWriter(
		kafkaBrokers,
		"compliance-service-group",
		"audit.trail", // Ingest from audit trail
		repo,
		logger,
	)
	go func() {
		logger.Info("starting clickhouse writer")
		if err := cw.Start(context.Background()); err != nil {
			logger.Error("clickhouse writer failed", "error", err)
		}
	}()

	// 3. Initialize Handlers
	h := handlers.NewComplianceHandler(repo, bulkhead, s3Storage)

	// 4. Initialize Router & Start Server
	r := router.NewRouter(h, keyring, rdb)

	logger.Info("compliance service starting", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
