package router

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/policy/pkg/handlers"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"golang.org/x/time/rate"
)

// NewRouter wires up the chi router for the policy service.
// Route paths match exactly what the control-plane expects per spec §11.5.
func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client, stop <-chan struct{}) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestSize(512 * 1024))  // 512KB max (policy logic is small)
	r.Use(middleware.Timeout(5 * time.Second)) // 5s timeout for all policy operations
	r.Use(shared_middleware.SecurityHeaders)

	// Metrics & Health
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

		r.Route("/v1", func(r chi.Router) {
			breaker := resilience.NewBreaker(resilience.BreakerConfig{
				Name:             "policy-redis-blocklist",
				MaxRequests:      5,
				Interval:         10 * time.Second,
				FailureThreshold: 3,
				OpenDuration:     5 * time.Second,
			}, nil)
			
			rateLimiter := shared_middleware.NewRateLimiter(rdb, rate.Limit(1000), 2000, stop)

			r.Use(rateLimiter.Limit)
			r.Use(shared_middleware.AuthJWTWithBlocklist(keyring, rdb, breaker))
		idemMiddleware := shared_middleware.IdempotencyMiddleware(rdb)
		r.Route("/policies", func(r chi.Router) {
			r.Get("/", h.ListPolicies)
			r.With(idemMiddleware).Post("/", h.CreatePolicy)
			r.Get("/{id}", h.GetPolicy)
			r.With(idemMiddleware).Put("/{id}", h.UpdatePolicy)
			r.Delete("/{id}", h.DeletePolicy)
		})

		r.Route("/assignments", func(r chi.Router) {
			r.Get("/", h.ListAssignments)
			r.With(idemMiddleware).Post("/", h.CreateAssignment)
			r.Delete("/{id}", h.DeleteAssignment)
		})

		r.Post("/policy/evaluate", h.Evaluate)
		r.Get("/policy/eval-logs", h.ListEvalLogs)
	})

	return r
}
