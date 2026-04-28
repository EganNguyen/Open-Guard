package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openguard/services/dlp/pkg/consumer"
	"github.com/openguard/services/dlp/pkg/handlers"
	"github.com/openguard/services/dlp/pkg/repository"
	"github.com/openguard/services/dlp/pkg/router"
	"github.com/openguard/services/dlp/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
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
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/openguard_dlp?sslmode=disable"
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 1. Initialize Postgres
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo, err := repository.NewRepository(ctx, databaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	if err := repo.InitSchema(ctx); err != nil {
		logger.Error("failed to init schema", "error", err)
		os.Exit(1)
	}

	// 2. Initialize Handlers
	h := handlers.NewDLPHandler(repo)

	// 3. Initialize Router & Start Server
	r := router.NewRouter(h, keyring, rdb)

	// 4. Initialize Kafka Consumer for async scanning
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}

	dlqTopic := os.Getenv("KAFKA_DLQ_TOPIC")
	if dlqTopic == "" {
		dlqTopic = "dlp.dlq"
	}

	dlqWriter := &kafka.Writer{
		Addr:     kafka.TCP(brokers),
		Topic:    dlqTopic,
		Balancer: &kafka.LeastBytes{},
	}
	defer dlqWriter.Close()

	maxFailures := 5
	if val := os.Getenv("MAX_CONSECUTIVE_FAILURES"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			maxFailures = i
		}
	}

	dlpConsumer := consumer.NewConsumer([]string{brokers}, "control.plane.events", "dlp-service", repo, logger.With("component", "consumer"), dlqWriter, maxFailures)
	go func() {
		if err := dlpConsumer.Start(context.Background()); err != nil {
			logger.Error("dlp consumer failed", "error", err)
		}
	}()

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

	logger.Info("dlp service starting", "port", port)
	go func() {
		var serverErr error
		certFile := "/certs/dlp.crt"
		keyFile := "/certs/dlp.key"
		if _, err := os.Stat(certFile); err == nil {
			serverErr = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			logger.Warn("TLS certs not found, starting in HTTP mode (DEV ONLY)")
			serverErr = srv.ListenAndServe()
		}

		if serverErr != nil && serverErr != http.ErrServerClosed {
			logger.Error("server failed", "error", serverErr)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutting down dlp service")
	srv.Shutdown(context.Background())
}
