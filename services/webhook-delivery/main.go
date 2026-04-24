package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/openguard/services/webhook-delivery/pkg/consumer"
	"github.com/openguard/services/webhook-delivery/pkg/deliverer"
	"github.com/openguard/services/webhook-delivery/pkg/router"
	"github.com/openguard/services/webhook-delivery/pkg/telemetry"
	"github.com/openguard/shared/kafka"
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

	// 2. Initialize Deliverer
	d := deliverer.NewDeliverer(logger)

	// 3. Start Webhook Consumer
	wc := consumer.NewWebhookConsumer(
		strings.Join(kafkaBrokers, ","),
		"webhook-delivery-group",
		"webhook.delivery",
		d,
		publisher,
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

	logger.Info("webhook-delivery service starting", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
