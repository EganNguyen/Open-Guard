package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openguard/services/audit/pkg/consumer"
	"github.com/openguard/services/audit/pkg/handlers"
	"github.com/openguard/services/audit/pkg/repository"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "audit")
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── MongoDB ──────────────────────────────────────────────────────────────
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		logger.Error("failed to connect to mongodb", "error", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	repo := repository.NewAuditRepository(client, "openguard_audit")

	// ── Kafka Consumer ────────────────────────────────────────────────────────
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = "audit-service"
	}

	c, err := consumer.NewAuditConsumer(brokers, groupID, "policy.changes", repo, logger)
	if err != nil {
		logger.Error("failed to create kafka consumer", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	go func() {
		logger.Info("starting kafka consumer")
		if err := c.Start(ctx); err != nil {
			logger.Error("consumer failed", "error", err)
		}
	}()

	// ── HTTP Server (Health + Read API) ──────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	sseH := handlers.NewSseHandler(repo)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"OK"}`)
	})

	mux.HandleFunc("/v1/events/stream", sseH.StreamEvents)
	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		events, err := repo.FindEvents(r.Context(), nil, 50, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"events": events})
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logger.Info("audit api starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down audit service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
