package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	mw "github.com/openguard/gateway/pkg/middleware"
	"github.com/openguard/gateway/pkg/proxy"
	"github.com/openguard/shared/crypto"
	sharedmw "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	JWTKeyring *crypto.JWTKeyring
	Redis      *redis.Client
	Logger     *slog.Logger

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

	iamProxy, err := proxy.NewReverseProxy(cfg.IAMAddr, cfg.Logger, iamBreaker)
	if err != nil {
		return nil, err
	}

	iamStripHandler := http.StripPrefix("/api/v1", iamProxy)

	r.Group(func(r chi.Router) {
		r.Handle("/api/v1/auth/*", iamStripHandler)
	})

	r.Group(func(r chi.Router) {
		r.Handle("/api/v1/scim/*", iamStripHandler)
	})

	r.Group(func(r chi.Router) {
		r.Use(mw.JWTAuth(cfg.JWTKeyring))
		
		pc := mw.NewPolicyClient(cfg.PolicyAddr, cfg.Logger)
		r.Use(pc.Middleware())

		r.Handle("/api/v1/users", iamStripHandler)
		r.Handle("/api/v1/users/*", iamStripHandler)

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

func serviceUnavailableHandler(name, addr string, logger *slog.Logger) http.Handler {
	if addr != "" {
		cb := resilience.NewBreaker(resilience.BreakerConfig{
			Name:             name + "-breaker",
			Timeout:          5 * time.Second,
			MaxRequests:      2,
			Interval:         10 * time.Second,
			FailureThreshold: 5,
			OpenDuration:     30 * time.Second,
		})
		p, err := proxy.NewReverseProxy(addr, logger, cb)
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
