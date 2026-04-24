package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/redis/go-redis/v9"
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
	if siemURL := os.Getenv("ALERTING_SIEM_WEBHOOK_URL"); siemURL != "" {
		if err := webhook.ValidateConfig(siemURL); err != nil {
			logger.Error("invalid ALERTING_SIEM_WEBHOOK_URL", "error", err)
			os.Exit(1)
		}
	}

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
	r := router.NewRouter(h, keyring, rdb)

	logger.Info("alerting service starting", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
