package router

import (
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/policy/pkg/handlers"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
)

// NewRouter wires up the chi router for the policy service.
// Route paths match exactly what the control-plane expects per spec §11.5.
func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestSize(512 * 1024))  // 512KB max (policy logic is small)
	r.Use(middleware.Timeout(5 * time.Second)) // 5s timeout for all policy operations

	// Metrics & Health
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

	r.Route("/v1", func(r chi.Router) {
		r.Use(shared_middleware.APIKeyAuth(os.Getenv("INTERNAL_API_KEY")))
		r.Route("/policies", func(r chi.Router) {
			r.Get("/", h.ListPolicies)
			r.Post("/", h.CreatePolicy)
			r.Get("/{id}", h.GetPolicy)
			r.Put("/{id}", h.UpdatePolicy)
			r.Delete("/{id}", h.DeletePolicy)
		})

		r.Route("/assignments", func(r chi.Router) {
			r.Get("/", h.ListAssignments)
			r.Post("/", h.CreateAssignment)
			r.Delete("/{id}", h.DeleteAssignment)
		})

		r.Post("/policy/evaluate", h.Evaluate)
		r.Get("/policy/eval-logs", h.ListEvalLogs)
	})

	return r
}
