package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/openguard/services/dlp/pkg/handlers"
	"github.com/openguard/services/dlp/pkg/repository"
	"github.com/openguard/services/dlp/pkg/router"
	"github.com/openguard/services/dlp/pkg/telemetry"
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
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-secret"
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
	r := router.NewRouter(h, jwtSecret)

	logger.Info("dlp service starting", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
