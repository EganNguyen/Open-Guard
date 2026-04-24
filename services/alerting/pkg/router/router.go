package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openguard/services/alerting/pkg/handlers"
	"github.com/openguard/shared/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *handlers.AlertHandler, jwtSecret string) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")
	// Health check (unauthenticated)
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}).Methods("GET")

	v1 := r.PathPrefix("/v1/threats").Subrouter()
	
	// Apply JWT authentication to all v1 routes
	v1.Use(middleware.JWTAuth(jwtSecret))

	v1.HandleFunc("/alerts", h.ListAlerts).Methods("GET")
	v1.HandleFunc("/alerts/{id}", h.GetAlert).Methods("GET")
	v1.HandleFunc("/alerts/{id}/acknowledge", h.AcknowledgeAlert).Methods("POST")
	v1.HandleFunc("/alerts/{id}/resolve", h.ResolveAlert).Methods("POST")
	v1.HandleFunc("/stats", h.GetStats).Methods("GET")
	v1.HandleFunc("/detectors", h.GetDetectors).Methods("GET")

	return r
}
