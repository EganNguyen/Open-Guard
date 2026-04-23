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

	r.Route("/v1", func(r chi.Router) {
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
