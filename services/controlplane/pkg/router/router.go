package router

import (
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	mw "github.com/openguard/controlplane/pkg/middleware"
	"github.com/openguard/controlplane/pkg/proxy"
	"github.com/openguard/shared/crypto"
	sharedmw "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	JWTKeyring      *crypto.JWTKeyring
	APIKeyValidator sharedmw.APIKeyValidator
	Redis           redis.UniversalClient
	Logger     *slog.Logger
	TLSConfig  *tls.Config

	IAMAddr        string
	PolicyAddr     string
	ThreatAddr     string
	AuditAddr      string
	AlertingAddr   string
	ComplianceAddr string
}

func New(cfg Config) (*chi.Mux, error) {
	r := chi.NewRouter()

	r.Use(sharedmw.RequestID)
	r.Use(mw.Logger(cfg.Logger))

	rl := mw.NewRateLimiter(cfg.Redis, cfg.Logger, 300, 1000, time.Minute)
	r.Use(rl.Middleware())

	r.Get("/health", healthHandler)
	r.Get("/health/live", healthHandler)
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		healthHandler(w, r)
	})

	iamBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "iam-breaker",
		Timeout:          5 * time.Second,
		MaxRequests:      2,
		Interval:         10 * time.Second,
		FailureThreshold: 5,
		OpenDuration:     30 * time.Second,
	})

	iamProxy, err := proxy.NewReverseProxy(cfg.IAMAddr, cfg.Logger, iamBreaker, cfg.TLSConfig)
	if err != nil {
		return nil, err
	}

	iamStripHandler := http.StripPrefix("/api/v1", iamProxy)

	r.Group(func(r chi.Router) {
		r.Handle("/api/v1/auth/*", iamStripHandler) // Public IAM routes
	})

	// Connector-authenticated routes (Bearer API Key)
	r.Group(func(r chi.Router) {
		r.Use(sharedmw.APIKeyAuth(cfg.APIKeyValidator))
		r.Use(injectOrgIDHeader)

		policyHandler := serviceUnavailableHandler("policy", cfg.PolicyAddr, cfg.Logger, cfg.TLSConfig)
		r.Handle("/v1/policy/*", policyHandler)

		auditHandler := serviceUnavailableHandler("audit", cfg.AuditAddr, cfg.Logger, cfg.TLSConfig)
		r.Handle("/v1/events/ingest", auditHandler) // Control plane handles ingestion, or proxies for now
	})

	// Admin-JWT-authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(mw.JWTAuth(cfg.JWTKeyring, cfg.Logger))

		r.Handle("/v1/scim/v2/*", iamStripHandler)
		r.Handle("/api/v1/scim/*", iamStripHandler)
		r.Handle("/api/v1/users", iamStripHandler)
		r.Handle("/api/v1/users/*", iamStripHandler)
		
		policyHandler := serviceUnavailableHandler("policy", cfg.PolicyAddr, cfg.Logger, cfg.TLSConfig)
		r.Handle("/api/v1/policies", policyHandler)
		r.Handle("/api/v1/policies/*", policyHandler)

		threatHandler := serviceUnavailableHandler("threat", cfg.ThreatAddr, cfg.Logger, cfg.TLSConfig)
		auditAPIHandler := serviceUnavailableHandler("audit", cfg.AuditAddr, cfg.Logger, cfg.TLSConfig)
		alertingHandler := serviceUnavailableHandler("alerting", cfg.AlertingAddr, cfg.Logger, cfg.TLSConfig)
		complianceHandler := serviceUnavailableHandler("compliance", cfg.ComplianceAddr, cfg.Logger, cfg.TLSConfig)

		r.Handle("/api/v1/threats", threatHandler)
		r.Handle("/api/v1/threats/*", threatHandler)
		r.Handle("/api/v1/audit", auditAPIHandler)
		r.Handle("/api/v1/audit/*", auditAPIHandler)
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

func injectOrgIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if orgID, ok := r.Context().Value(sharedmw.TenantIDKey).(string); ok {
			r.Header.Set("X-Org-ID", orgID)
		}
		next.ServeHTTP(w, r)
	})
}

func serviceUnavailableHandler(name, addr string, logger *slog.Logger, tlsCfg *tls.Config) http.Handler {
	if addr != "" {
		cb := resilience.NewBreaker(resilience.BreakerConfig{
			Name:             name + "-breaker",
			Timeout:          5 * time.Second,
			MaxRequests:      2,
			Interval:         10 * time.Second,
			FailureThreshold: 5,
			OpenDuration:     30 * time.Second,
		})
		p, err := proxy.NewReverseProxy(addr, logger, cb, tlsCfg)
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
