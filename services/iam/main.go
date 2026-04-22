package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/openguard/services/iam/pkg/handlers"
	"github.com/openguard/services/iam/pkg/logger"
	"github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/services/iam/pkg/router"
	"github.com/openguard/services/iam/pkg/seed"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/services/iam/pkg/telemetry"
)

func main() {
	// Initialize logger
	logger.Init()
	defer logger.Log.Sync()
	log := logger.Log.With(zap.String("service", "iam"))

	// Initialize OpenTelemetry
	tp, err := telemetry.InitTracer()
	if err != nil {
		log.Fatal("failed to initialize tracer", zap.Error(err))
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Error("failed to shutdown tracer", zap.Error(err))
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// DB connection string
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatal("failed to parse db config", zap.Error(err))
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatal("failed to connect to db", zap.Error(err))
	}
	defer pool.Close()

	// Auto-seed if requested
	if os.Getenv("SEED_DB") == "true" {
		if err := seed.Seed(ctx, pool); err != nil {
			log.Error("seeding failed", zap.Error(err))
		}
	}

	// Initialize AuthWorkerPool
	authPool := service.NewAuthWorkerPool(2 * runtime.NumCPU())

	// Initialize Repository, Service, Handler
	repo := repository.NewRepository(pool)
	svc := service.NewService(repo, authPool)
	h := handlers.NewHandler(svc)

	// Setup Router
	r := router.NewRouter(h)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Info("service starting", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown failed", zap.Error(err))
	}
	
	fmt.Println("Service exited")
}
