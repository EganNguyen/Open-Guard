package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/services/connector-registry/pkg/handlers"
	"github.com/openguard/services/connector-registry/pkg/repository"
	"github.com/openguard/services/connector-registry/pkg/router"
	"github.com/openguard/services/connector-registry/pkg/service"
	"github.com/openguard/services/connector-registry/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/secrets"
	shared_telemetry "github.com/openguard/shared/telemetry"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(shared_telemetry.NewSafeHandler(slog.NewJSONHandler(os.Stdout, nil)))
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

	// ── Database ──────────────────────────────────────────────────────────────
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// ── Redis ─────────────────────────────────────────────────────────────────
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/2" // db 2 for connectors
	}
	rOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Error("failed to parse redis url", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(rOptions)
	defer rdb.Close()

	// ── Service + Handler + Router ────────────────────────────────────────────
	repo := repository.NewRepository(pool)
	svc := service.NewService(repo, rdb, logger)
	h := handlers.NewHandler(svc)

	// Auth Configuration
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

	// Graceful shutdown channel
	stopCh := make(chan struct{})

	r := router.NewRouter(h, keyring, rdb, stopCh)

	// ── HTTP Server ───────────────────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

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
		Handler:   r,
		TLSConfig: tlsConfig,
	}

	go func() {
		logger.Info("connector-registry starting", "port", port)
		var serverErr error
		certFile := "/certs/connector-registry.crt"
		keyFile := "/certs/connector-registry.key"
		if _, err := os.Stat(certFile); err == nil {
			serverErr = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			logger.Warn("TLS certs not found, starting in HTTP mode (DEV ONLY)")
			serverErr = srv.ListenAndServe()
		}

		if serverErr != nil && serverErr != http.ErrServerClosed {
			logger.Error("server failed", "error", serverErr)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	close(stopCh)
	logger.Info("shutting down connector-registry")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
}
