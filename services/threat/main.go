package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/openguard/services/threat/pkg/alert"
	"github.com/openguard/services/threat/pkg/detector"
	"github.com/openguard/services/threat/pkg/handlers"
	"github.com/openguard/services/threat/pkg/router"
	"github.com/openguard/services/threat/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	sharedkafka "github.com/openguard/shared/kafka"
	"github.com/openguard/shared/secrets"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Initialize OpenTelemetry
	tp, err := telemetry.InitTracer()
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://" + redisAddr + "/0"
	}
	rOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error("failed to parse redis url", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(rOptions)
	defer rdb.Close()

	kafkaBrokersStr := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokersStr == "" {
		kafkaBrokersStr = "localhost:9092"
	}
	// Split for Publisher (takes []string); detectors use the raw string internally via strings.Split
	kafkaBrokersList := strings.Split(kafkaBrokersStr, ",")

	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = "threat-detector"
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	geoDBPath := os.Getenv("GEOLITE2_DB_PATH")
	if geoDBPath == "" {
		geoDBPath = "./data/GeoLite2-City.mmdb"
	}

	// Topics
	topicAuth := os.Getenv("KAFKA_TOPIC_AUTH")
	if topicAuth == "" {
		topicAuth = "auth.events"
	}
	topicPolicy := os.Getenv("KAFKA_TOPIC_POLICY")
	if topicPolicy == "" {
		topicPolicy = "policy.changes"
	}
	topicAccess := os.Getenv("KAFKA_TOPIC_ACCESS")
	if topicAccess == "" {
		topicAccess = "data.access"
	}

	// Initialize Alert Store
	store, err := alert.NewStore(mongoURI)
	if err != nil {
		log.Fatalf("Failed to create alert store: %v", err)
	}

	// Initialize Kafka Publisher for emitting threat.alerts
	publisher := sharedkafka.NewPublisher(kafkaBrokersList)
	defer publisher.Close()

	// Initialize Detectors
	bfDetector, err := detector.NewBruteForceDetector(redisAddr, kafkaBrokersStr, groupID, topicAuth, store, publisher, logger)
	if err != nil {
		log.Fatalf("Failed to create BruteForceDetector: %v", err)
	}

	itDetector, err := detector.NewImpossibleTravelDetector(geoDBPath, redisAddr, kafkaBrokersStr, groupID, topicAuth, store, publisher, logger)
	if err != nil {
		logger.Warn("Failed to create ImpossibleTravelDetector (GeoDB missing?)", "error", err)
	}

	ohDetector := detector.NewOffHoursDetector(redisAddr, kafkaBrokersStr, groupID, topicAuth, store, publisher, logger)
	deDetector := detector.NewDataExfiltrationDetector(redisAddr, kafkaBrokersStr, groupID, topicAccess, store, publisher, logger)
	atDetector := detector.NewAccountTakeoverDetector(redisAddr, kafkaBrokersStr, groupID, topicAuth, store, publisher, logger)
	peDetector := detector.NewPrivilegeEscalationDetector(redisAddr, kafkaBrokersStr, groupID, topicAuth, topicPolicy, store, publisher, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Shutdown handling
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down...")
		cancel()
	}()

	// Start Detectors
	go bfDetector.Start(ctx)
	if itDetector != nil {
		go itDetector.Run(ctx)
	}
	go ohDetector.Run(ctx)
	go deDetector.Run(ctx)
	go atDetector.Run(ctx)
	go peDetector.Run(ctx)

	// Initialize Handlers & Router
	h := handlers.NewHandler(store)

	// Auth Configuration
	secretProvider, err := secrets.GetProvider(context.Background())
	if err != nil {
		logger.Error("failed to initialize secrets provider", "error", err)
		os.Exit(1)
	}

	keyringJSON, err := secretProvider.GetSecret(context.Background(), "IAM_JWT_KEYS")
	if err != nil {
		keyringJSON = `[{"kid":"dev-key","secret":"dev-secret-at-least-32-chars-long-!!","algorithm":"HS256","status":"active"}]`
		logger.Warn("IAM_JWT_KEYS not found in secrets provider, using default dev key", "error", err)
	}
	keyring, err := crypto.LoadKeyring(keyringJSON)
	if err != nil {
		logger.Error("failed to load JWT keyring", "error", err)
		os.Exit(1)
	}

	r := router.NewRouter(h, keyring, rdb)

	// Add metrics
	r.Handle("/metrics", promhttp.Handler())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("Threat service starting", "port", port)
	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err)
		}
	}()

	<-ctx.Done()
	server.Shutdown(context.Background())
}