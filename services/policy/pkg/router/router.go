package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openguard/services/policy/pkg/handlers"
)

// NewRouter wires up the chi router for the policy service.
// Route paths match exactly what the control-plane expects per spec §11.5.
func NewRouter(h *handlers.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Metrics & Health
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

	// Policy evaluation — path must be /v1/policy/evaluate per spec §11.5
	// (control-plane proxies POST /v1/policy/evaluate → policy-service /v1/policy/evaluate)
	r.Post("/v1/policy/evaluate", h.Evaluate)

	// Policy CRUD
	r.Route("/v1/policies", func(r chi.Router) {
		r.Get("/", h.ListPolicies)
		r.Post("/", h.CreatePolicy)
		r.Get("/{id}", h.GetPolicy)
		r.Put("/{id}", h.UpdatePolicy)
		r.Delete("/{id}", h.DeletePolicy)
	})

	// Eval log
	r.Get("/v1/policy/eval-logs", h.ListEvalLogs)

	return r
}
