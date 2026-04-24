package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/openguard/services/alerting/pkg/handlers"
	"github.com/openguard/services/alerting/pkg/repository"
	"github.com/openguard/services/alerting/pkg/router"
	"github.com/openguard/services/alerting/pkg/saga"
	"github.com/openguard/services/alerting/pkg/webhook"
	"github.com/openguard/services/alerting/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/kafka"
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
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	kafkaBrokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	if len(kafkaBrokers) == 0 || kafkaBrokers[0] == "" {
		kafkaBrokers = []string{"localhost:9092"}
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 1. Initialize MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		logger.Error("failed to connect to mongodb", "error", err)
		os.Exit(1)
	}
	repo := repository.NewRepository(client, "alerting")

	// 2. Initialize Kafka
	publisher := kafka.NewPublisher(kafkaBrokers)
	defer publisher.Close()

	// 3. Initialize SIEM
	siem := webhook.NewSIEMDeliverer()

	// 4. Initialize Handlers
	h := handlers.NewAlertHandler(repo)

	// 5. Start Alert Saga (Kafka Consumer)
	alertSaga := saga.NewAlertSaga(
		kafkaBrokers,
		"alerting-service-group",
		"threat.alerts",
		repo,
		publisher,
		siem,
		logger,
	)
	
	go func() {
		logger.Info("starting alert saga")
		if err := alertSaga.Start(context.Background()); err != nil {
			logger.Error("alert saga failed", "error", err)
		}
	}()

	// 6. Initialize Router & Start Server
	r := router.NewRouter(h, keyring)

	logger.Info("alerting service starting", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
