package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openguard/services/dlp/pkg/handlers"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *handlers.DLPHandler, keyring []crypto.JWTKey) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	v1 := r.PathPrefix("/v1/dlp").Subrouter()
	v1.Use(middleware.AuthJWT(keyring))

	v1.HandleFunc("/scan", h.Scan).Methods("POST")
	v1.HandleFunc("/policies", h.ListPolicies).Methods("GET")
	v1.HandleFunc("/policies", h.CreatePolicy).Methods("POST")
	v1.HandleFunc("/findings", h.ListFindings).Methods("GET")

	return r
}
