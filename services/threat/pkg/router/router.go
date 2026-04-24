package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openguard/services/threat/pkg/handlers"
)

func NewRouter(h *handlers.Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/v1/threats", func(r chi.Router) {
		r.Get("/alerts", h.ListAlerts)
		r.Get("/alerts/{id}", h.GetAlert)
		r.Post("/alerts/{id}/acknowledge", h.AcknowledgeAlert)
		r.Post("/alerts/{id}/resolve", h.ResolveAlert)
		r.Get("/stats", h.GetStats)
		r.Get("/detectors", h.ListDetectors)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return r
}
