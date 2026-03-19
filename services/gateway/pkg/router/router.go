package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	mw "github.com/openguard/gateway/pkg/middleware"
	"github.com/openguard/gateway/pkg/proxy"
	sharedmw "github.com/openguard/shared/middleware"
	"github.com/redis/go-redis/v9"
)

// Config holds the configuration needed to build the gateway router.
type Config struct {
	JWTSecret string
	Redis     *redis.Client
	Logger    *slog.Logger

	// Upstream service addresses
	IAMAddr        string
	PolicyAddr     string
	ThreatAddr     string
	AuditAddr      string
	AlertingAddr   string
	ComplianceAddr string
}

// New creates the gateway chi router with all routes, middleware, and proxies.
func New(cfg Config) (*chi.Mux, error) {
	r := chi.NewRouter()

	// Global middleware
	r.Use(sharedmw.RequestID)
	r.Use(mw.Logger(cfg.Logger))

	// Rate limiter
	rl := mw.NewRateLimiter(cfg.Redis, cfg.Logger, 300, 1000, time.Minute)
	r.Use(rl.Middleware())

	// Health check
	r.Get("/health", healthHandler)
	r.Get("/health/live", healthHandler)
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: check Redis connectivity
		healthHandler(w, r)
	})

	// Create proxies for upstream services
	iamProxy, err := proxy.NewReverseProxy(cfg.IAMAddr, cfg.Logger)
	if err != nil {
		return nil, err
	}

	// Strip /api/v1 prefix before proxying — downstream services see e.g. /auth/login
	iamStripHandler := http.StripPrefix("/api/v1", iamProxy)

	// --- Public routes (no JWT required) ---
	r.Group(func(r chi.Router) {
		r.Handle("/api/v1/auth/*", iamStripHandler)
	})

	// --- SCIM routes (SCIM bearer token — handled by IAM itself) ---
	r.Group(func(r chi.Router) {
		r.Handle("/api/v1/scim/*", iamStripHandler)
	})

	// --- Protected routes (JWT required) ---
	r.Group(func(r chi.Router) {
		r.Use(mw.JWTAuth(cfg.JWTSecret))
		r.Handle("/api/v1/users", iamStripHandler)
		r.Handle("/api/v1/users/*", iamStripHandler)

		// Future service proxies — return 503 until implemented
		policyHandler := serviceUnavailableHandler("policy", cfg.PolicyAddr, cfg.Logger)
		threatHandler := serviceUnavailableHandler("threat", cfg.ThreatAddr, cfg.Logger)
		auditHandler := serviceUnavailableHandler("audit", cfg.AuditAddr, cfg.Logger)
		alertingHandler := serviceUnavailableHandler("alerting", cfg.AlertingAddr, cfg.Logger)
		complianceHandler := serviceUnavailableHandler("compliance", cfg.ComplianceAddr, cfg.Logger)

		r.Handle("/api/v1/policies", policyHandler)
		r.Handle("/api/v1/policies/*", policyHandler)
		r.Handle("/api/v1/threats", threatHandler)
		r.Handle("/api/v1/threats/*", threatHandler)
		r.Handle("/api/v1/audit", auditHandler)
		r.Handle("/api/v1/audit/*", auditHandler)
		r.Handle("/api/v1/alerts", alertingHandler)
		r.Handle("/api/v1/alerts/*", alertingHandler)
		r.Handle("/api/v1/compliance", complianceHandler)
		r.Handle("/api/v1/compliance/*", complianceHandler)
	})

	return r, nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// serviceUnavailableHandler returns a handler that proxies if the service address
// is configured, or returns 503 if the service is not yet available.
func serviceUnavailableHandler(name, addr string, logger *slog.Logger) http.Handler {
	if addr != "" {
		p, err := proxy.NewReverseProxy(addr, logger)
		if err == nil {
			return http.StripPrefix("/api/v1", p)
		}
		logger.Error("failed to create proxy", "service", name, "error", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"code":    "SERVICE_UNAVAILABLE",
				"message": name + " service is not available yet",
			},
		})
	})
}
