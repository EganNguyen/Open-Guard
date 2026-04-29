package router

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/alerting/pkg/handlers"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *handlers.AlertHandler, keyring []crypto.JWTKey, rdb *redis.Client) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")
	r.Use(middleware.SecurityHeaders)
	// Health check (unauthenticated)
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Circuit breaker for Redis blocklist check
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "alerting-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, nil)

	v1 := r.PathPrefix("/v1/threats").Subrouter()

	// Apply deprecation headers to v1 routes
	v1.Use(middleware.DeprecationHeaders("Fri, 01 Jan 2027 00:00:00 GMT"))

	// Apply JWT authentication with blocklist to all v1 routes
	v1.Use(middleware.AuthJWTWithBlocklist(keyring, rdb, breaker))

	v1.HandleFunc("/alerts", h.ListAlerts).Methods("GET")
	v1.HandleFunc("/alerts/{id}", h.GetAlert).Methods("GET")
	v1.HandleFunc("/alerts/{id}/acknowledge", h.AcknowledgeAlert).Methods("POST")
	v1.HandleFunc("/alerts/{id}/resolve", h.ResolveAlert).Methods("POST")
	v1.HandleFunc("/stats", h.GetStats).Methods("GET")
	v1.HandleFunc("/detectors", h.GetDetectors).Methods("GET")

	return r
}
