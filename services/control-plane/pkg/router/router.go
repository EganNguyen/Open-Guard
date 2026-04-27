package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openguard/control-plane/pkg/middleware"
	"github.com/openguard/control-plane/pkg/proxy"
	"github.com/openguard/control-plane/pkg/telemetry"
	sharedmiddleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
)

// NewRouter builds and returns the control-plane router.
// Circuit breakers use proper BreakerConfig per spec §12 and review item #11.
func NewRouter() *chi.Mux {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("component", "control-plane")

	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Correlation)
	r.Use(telemetry.Metrics)
	r.Use(chimiddleware.Recoverer)
	r.Use(sharedmiddleware.SecurityHeaders)
	
	// CORS setup
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			publicURL := os.Getenv("OPENGUARD_PUBLIC_URL")
			
			// Allow localhost, the LocalStack domain, AND the persistent InstaTunnel subdomain
			allowed := origin == "http://localhost:4200" || 
			           origin == "https://openguard.lb.localstack.cloud" || 
			           origin == "https://openguard-dev.instatunnel.io" ||
			           (publicURL != "" && origin == publicURL)



			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})


	r.Handle("/metrics", promhttp.Handler())

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "OK", "service": "control-plane"}`))
	})

	// ── Circuit Breakers ───────────────────────────────────────────────────────
	// Properly configured per review item #11.
	// FailureThreshold=5: open after 5 consecutive failures.
	// OpenDuration=30s: stay open for 30s before trying half-open.
	// MaxRequests=3: allow 3 probe requests in half-open state.
	cbPolicy := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "cb-policy",
		MaxRequests:      3,
		Interval:         10 * time.Second,
		FailureThreshold: 5,
		OpenDuration:     30 * time.Second,
	}, logger)

	cbIAM := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "cb-iam",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 5,
		OpenDuration:     30 * time.Second,
	}, logger)

	// Service URLs
	iamURL := envOrDefault("IAM_URL", "http://iam:8080")
	policyURL := envOrDefault("POLICY_URL", "http://policy:8082")
	auditURL := envOrDefault("AUDIT_URL", "http://audit:8080")

	r.Route("/v1", func(r chi.Router) {
		// Policy evaluation — proxied to policy service at /v1/policy/evaluate (path match fixed)
		r.Post("/policy/evaluate", proxy.NewProxy(policyURL, cbPolicy))

		// Eval logs proxy
		r.Get("/policy/eval-logs", proxy.NewProxy(policyURL, cbPolicy))

		// Policy CRUD proxy
		r.Route("/policies", func(r chi.Router) {
			r.Get("/", proxy.NewProxy(policyURL, cbPolicy))
			r.Post("/", proxy.NewProxy(policyURL, cbPolicy))
			r.Get("/{id}", proxy.NewProxy(policyURL, cbPolicy))
			r.Put("/{id}", proxy.NewProxy(policyURL, cbPolicy))
			r.Delete("/{id}", proxy.NewProxy(policyURL, cbPolicy))
		})

		// Event ingest (audit service — no CB per original design)
		r.Post("/events/ingest", proxy.NewProxy(auditURL, nil))

		// SCIM proxy to IAM
		r.Route("/scim/v2/Users", func(r chi.Router) {
			r.Get("/", proxy.NewProxy(iamURL, cbIAM))
			r.Post("/", proxy.NewProxy(iamURL, cbIAM))
			r.Get("/{id}", proxy.NewProxy(iamURL, cbIAM))
			r.Patch("/{id}", proxy.NewProxy(iamURL, cbIAM))
		})

		// Frontend Logging Endpoint
		r.Post("/logs", func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				Level   string                 `json:"level"`
				Message string                 `json:"message"`
				Context map[string]interface{} `json:"context"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				logger.Error("failed to decode frontend log", "error", err)
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			
			// Map level to slog levels with safety check
			logLevel := slog.LevelInfo
			switch payload.Level {
			case "error":
				logLevel = slog.LevelError
				// Safety: If the message looks like a successful HTTP request, downgrade to INFO
				if status, ok := payload.Context["status"].(float64); ok && status < 400 {
					logLevel = slog.LevelInfo
				}
			case "warn":
				logLevel = slog.LevelWarn
			case "debug":
				logLevel = slog.LevelDebug
			}

			logger.Log(r.Context(), logLevel, payload.Message, "source", "frontend", "context", payload.Context)
			w.WriteHeader(http.StatusAccepted)
		})
	})

	return r
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

