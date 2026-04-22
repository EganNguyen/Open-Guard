package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openguard/services/iam/pkg/handlers"
	iam_middleware "github.com/openguard/services/iam/pkg/middleware"
	"github.com/openguard/services/iam/pkg/telemetry"
)

// Router sets up the HTTP routes for the IAM service.
func NewRouter(h *handlers.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(iam_middleware.Correlation)
	r.Use(telemetry.Metrics)
	r.Use(middleware.Recoverer)

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)
	
	r.Route("/mgmt", func(r chi.Router) {
		r.Post("/orgs", h.CreateOrg)
		r.Post("/users", h.CreateUser)
		r.Get("/connectors", h.ListConnectors)
		r.Post("/connectors", h.CreateConnector)
		r.Put("/connectors/{id}", h.UpdateConnector)
		r.Delete("/connectors/{id}", h.DeleteConnector)
		r.Get("/users", h.ListUsers)
	})

	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", h.Login)
		r.Post("/logout", h.Logout)
		r.Get("/authorize", h.Authorize)
		r.Post("/token", h.Token)
	})

	return r
}
