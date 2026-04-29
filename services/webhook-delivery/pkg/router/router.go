package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openguard/shared/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")
	r.Use(middleware.SecurityHeaders)
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Placeholder for delivery history/stats if needed
	v1 := r.PathPrefix("/v1").Subrouter()
	v1.Use(middleware.DeprecationHeaders("Fri, 01 Jan 2027 00:00:00 GMT"))
	v1.HandleFunc("/webhook/deliveries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}).Methods("GET")

	return r
}
