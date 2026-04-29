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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/services/webhook-delivery/pkg/consumer"
	"github.com/openguard/services/webhook-delivery/pkg/deliverer"
	"github.com/openguard/services/webhook-delivery/pkg/repository"
	"github.com/openguard/services/webhook-delivery/pkg/router"
	"github.com/openguard/services/webhook-delivery/pkg/telemetry"
	"github.com/openguard/shared/kafka"
	shared_telemetry "github.com/openguard/shared/telemetry"
)

func main() {
	logger := slog.New(shared_telemetry.NewSafeHandler(slog.NewJSONHandler(os.Stdout, nil)))

	// Initialize OpenTelemetry (INFRA-04)
	tp, err := telemetry.InitTracer()
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

	// Config
	kafkaBrokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	if len(kafkaBrokers) == 0 || kafkaBrokers[0] == "" {
		kafkaBrokers = []string{"localhost:9092"}
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 1. Initialize Kafka Publisher for DLQ
	publisher := kafka.NewPublisher(kafkaBrokers)
	defer publisher.Close()

	// Initialize DB
	dbURL := os.Getenv("DATABASE_URL")
	var repo *repository.Repository
	if dbURL != "" {
		pool, err := pgxpool.New(context.Background(), dbURL)
		if err != nil {
			logger.Error("failed to connect to database", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
		repo = repository.NewRepository(pool)
		logger.Info("connected to database")
	} else {
		logger.Warn("DATABASE_URL not set, webhook delivery state will not be persisted")
	}

	// 2. Initialize Deliverer
	d := deliverer.NewDeliverer(logger)

	// 3. Start Webhook Consumer
	wc := consumer.NewWebhookConsumer(
		strings.Join(kafkaBrokers, ","),
		"webhook-delivery-group",
		"webhook.delivery",
		d,
		publisher,
		repo,
		logger,
	)
	go func() {
		logger.Info("starting webhook consumer")
		if err := wc.Start(context.Background()); err != nil {
			logger.Error("webhook consumer failed", "error", err)
		}
	}()

	// 4. Initialize Router & Start Server
	r := router.NewRouter()

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

	logger.Info("webhook-delivery service starting", "port", port)
	go func() {
		var serverErr error
		certFile := "/certs/webhook-delivery.crt"
		keyFile := "/certs/webhook-delivery.key"
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
	logger.Info("shutting down webhook-delivery service")
	srv.Shutdown(context.Background())
}
