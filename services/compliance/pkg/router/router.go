package router

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/compliance/pkg/handlers"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *handlers.ComplianceHandler, keyring []crypto.JWTKey, rdb *redis.Client) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")
	r.Use(middleware.SecurityHeaders)
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Circuit breaker for Redis blocklist check
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "compliance-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, nil)

	v1 := r.PathPrefix("/v1/compliance").Subrouter()
	v1.Use(middleware.DeprecationHeaders("Fri, 01 Jan 2027 00:00:00 GMT"))
	v1.Use(middleware.AuthJWTWithBlocklist(keyring, rdb, breaker))

	v1.HandleFunc("/posture", h.GetPosture).Methods("GET")
	v1.HandleFunc("/stats", h.GetStats).Methods("GET")
	v1.HandleFunc("/reports", h.ListReports).Methods("GET")
	v1.HandleFunc("/reports", h.CreateReport).Methods("POST")
	v1.HandleFunc("/reports/{id}", h.GetReportStatus).Methods("GET")
	v1.HandleFunc("/reports/{id}/download", h.DownloadReport).Methods("GET")

	return r
}
