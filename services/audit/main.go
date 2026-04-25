package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/audit/pkg/consumer"
	"github.com/openguard/services/audit/pkg/handlers"
	"github.com/openguard/services/audit/pkg/repository"
	"github.com/openguard/services/audit/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/openguard/shared/secrets"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "audit")
	slog.SetDefault(logger)

	// Initialize OpenTelemetry (INFRA-04)
	tp, err := telemetry.InitTracer()
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "audit-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, logger)

	// ── MongoDB CQRS Split ───────────────────────────────────────────────────
	primaryURI := os.Getenv("MONGO_URI_PRIMARY")
	if primaryURI == "" {
		primaryURI = os.Getenv("MONGODB_URI") // Fallback
		if primaryURI == "" {
			primaryURI = "mongodb://localhost:27017"
		}
	}
	secondaryURI := os.Getenv("MONGO_URI_SECONDARY")
	if secondaryURI == "" {
		secondaryURI = primaryURI // Fallback
	}

	// Connect Primary (Writes)
	wc := writeconcern.Majority()
	writeOpts := options.Client().ApplyURI(primaryURI).SetWriteConcern(wc)
	writeClient, err := mongo.Connect(ctx, writeOpts)
	if err != nil {
		logger.Error("failed to connect to primary mongodb", "error", err)
		os.Exit(1)
	}
	defer writeClient.Disconnect(ctx)

	// Connect Secondary (Reads)
	rp := readpref.SecondaryPreferred()
	readOpts := options.Client().ApplyURI(secondaryURI).SetReadPreference(rp)
	readClient, err := mongo.Connect(ctx, readOpts)
	if err != nil {
		logger.Error("failed to connect to secondary mongodb", "error", err)
		os.Exit(1)
	}
	defer readClient.Disconnect(ctx)

	writeRepo := repository.NewAuditWriteRepository(writeClient, "openguard_audit")
	readRepo := repository.NewAuditReadRepository(readClient, "openguard_audit")

	// ── Kafka Consumers (Multi-Topic) ─────────────────────────────────────────
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = "audit-service"
	}

	topics := []string{
		"auth.events",
		"policy.changes",
		"data.access",
		"threat.alerts",
		"connector.events",
		"audit.trail",
		"saga.orchestration",
	}

	for _, topic := range topics {
		c, err := consumer.NewAuditConsumer(brokers, groupID+"-"+topic, topic, writeRepo, logger)
		if err != nil {
			logger.Error("failed to create kafka consumer", "topic", topic, "error", err)
			continue
		}
		
		go func(topicName string, cons *consumer.AuditConsumer) {
			logger.Info("starting kafka consumer", "topic", topicName)
			if err := cons.Start(ctx); err != nil {
				logger.Error("consumer failed", "topic", topicName, "error", err)
			}
		}(topic, c)
	}

	// ── Auth Configuration ───────────────────────────────────────────────────
	secretProvider, err := secrets.GetProvider(ctx)
	if err != nil {
		logger.Error("failed to initialize secrets provider", "error", err)
		os.Exit(1)
	}

	keyringJSON, err := secretProvider.GetSecret(ctx, "IAM_JWT_KEYS")
	if err != nil {
		keyringJSON = `[{"kid":"dev-key","secret":"dev-secret-at-least-32-chars-long-!!","algorithm":"HS256","status":"active"}]`
		logger.Warn("IAM_JWT_KEYS not found in secrets provider, using default dev key", "error", err)
	}
	keyring, err := crypto.LoadKeyring(keyringJSON)
	if err != nil {
		logger.Error("failed to load JWT keyring", "error", err)
		os.Exit(1)
	}
	authMiddleware := middleware.AuthJWTWithBlocklist(keyring, rdb, breaker)

	// ── HTTP Server (Health + Read API) ──────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	sseH := handlers.NewSseHandler(readRepo)

	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"OK"}`)
	})

	mux.HandleFunc("/v1/events/stream", func(w http.ResponseWriter, r *http.Request) {
		authMiddleware(http.HandlerFunc(sseH.StreamEvents)).ServeHTTP(w, r)
	})
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events, err := readRepo.FindEvents(r.Context(), nil, 50, 0)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"events": events})
		})).ServeHTTP(w, r)
	})

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
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	go func() {
		logger.Info("audit api starting", "port", port)
		if tlsConfig != nil {
			if err := srv.ListenAndServeTLS("/certs/audit.crt", "/certs/audit.key"); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "error", err)
			}
		} else {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "error", err)
			}
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down audit service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
