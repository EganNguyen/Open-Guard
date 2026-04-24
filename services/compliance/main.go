package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/openguard/services/compliance/pkg/consumer"
	"github.com/openguard/services/compliance/pkg/handlers"
	"github.com/openguard/services/compliance/pkg/repository"
	"github.com/openguard/services/compliance/pkg/router"
	"github.com/openguard/services/compliance/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"encoding/json"
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
	
	var keyring []crypto.JWTKey
	if keysJSON := os.Getenv("JWT_KEYS"); keysJSON != "" {
		if err := json.Unmarshal([]byte(keysJSON), &keyring); err != nil {
			logger.Error("failed to parse JWT_KEYS", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Warn("JWT_KEYS not set, using default development key")
		keyring = []crypto.JWTKey{{KID: "dev", Secret: "default-secret", Algorithm: "HS256", Status: "active"}}
	}

	concurrency, _ := strconv.ParseInt(os.Getenv("COMPLIANCE_REPORT_CONCURRENCY"), 10, 64)
	if concurrency == 0 {
		concurrency = 5
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 1. Initialize ClickHouse
	repo, err := repository.NewRepository(clickhouseAddr)
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
	h := handlers.NewComplianceHandler(repo, concurrency)

	// 4. Initialize Router & Start Server
	r := router.NewRouter(h, keyring)

	logger.Info("compliance service starting", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
