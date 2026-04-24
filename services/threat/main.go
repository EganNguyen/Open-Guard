package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/openguard/services/threat/pkg/detector"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9092"
	}

	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = "threat-detector"
	}

	topic := os.Getenv("KAFKA_TOPIC")
	if topic == "" {
		topic = "iam.events"
	}

	detector, err := detector.NewBruteForceDetector(redisAddr, kafkaBrokers, groupID, topic, logger)
	if err != nil {
		log.Fatalf("Failed to create detector: %v", err)
	}
	defer detector.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down...")
		cancel()
	}()

	go func() {
		if err := detector.Start(ctx); err != nil {
			logger.Error("Detector error", "error", err)
		}
	}()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/threats", func(w http.ResponseWriter, r *http.Request) {
		threats, err := detector.GetThreats(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "%v", threats)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("Threat detector service starting", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("Server error", "error", err)
	}
}