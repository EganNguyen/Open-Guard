package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/openguard/todoapp/pkg/config"
	"github.com/openguard/todoapp/pkg/db"
	"github.com/openguard/todoapp/pkg/handlers"
	"github.com/openguard/todoapp/pkg/middleware"
	"github.com/openguard/todoapp/pkg/repository"
	"github.com/openguard/todoapp/pkg/sdk"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.MustLoad()

	// 1. Infrastructure
	database, err := db.Connect(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("db_connect_failed", "error", err)
		os.Exit(1)
	}

	// 2. OpenGuard SDK Components
	authClient, err := sdk.NewAuthClient(ctx, cfg.OIDCIssuer, cfg.ClientID, cfg.ClientSecret, cfg.FrontendURL, nil)
	if err != nil {
		logger.Error("auth_sdk_failed", "error", err)
		os.Exit(1)
	}

	policyClient, err := sdk.NewPolicyClient(cfg.OpenGuardURL, cfg.ConnectorAPIKey, cfg.PolicyCacheSize)
	if err != nil {
		logger.Error("policy_sdk_failed", "error", err)
		os.Exit(1)
	}

	auditClient := sdk.NewAuditClient(cfg.OpenGuardURL, cfg.ConnectorAPIKey, cfg.BatchSize, time.Duration(cfg.FlushIntervalMS)*time.Millisecond, logger)
	go auditClient.Start(ctx)
	defer auditClient.Stop()

	// 3. Business Layer
	repo := repository.NewRepository(database)
	todoHandler := handlers.NewTodoHandler(repo, policyClient, auditClient)
	authHandler := handlers.NewAuthHandler(authClient, cfg.FrontendURL)
	webhooksHandler := handlers.NewWebhookHandler(cfg.WebhookSecret, auditClient)

	// 4. Routing
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", authHandler.Login)
		r.Get("/callback", authHandler.Callback)
	})
	r.Handle("/webhooks/openguard", webhooksHandler)

	// Protected API routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(authClient))
		
		r.Route("/api/v1/todos", func(r chi.Router) {
			r.Get("/", todoHandler.List)
			r.Post("/", todoHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Patch("/", todoHandler.Update)
				r.Delete("/", todoHandler.Delete)
			})
		})
	})

	// 5. Server Start
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		logger.Info("server_starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_failed", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("server_shutting_down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server_shutdown_failed", "error", err)
	}
}
