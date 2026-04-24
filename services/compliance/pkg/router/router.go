package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openguard/services/compliance/pkg/handlers"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(h *handlers.ComplianceHandler, keyring []crypto.JWTKey) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/metrics", promhttp.Handler()).Methods("GET")
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	v1 := r.PathPrefix("/v1/compliance").Subrouter()
	v1.Use(middleware.AuthJWT(keyring))

	v1.HandleFunc("/posture", h.GetPosture).Methods("GET")
	v1.HandleFunc("/reports", h.ListReports).Methods("GET")
	v1.HandleFunc("/reports", h.CreateReport).Methods("POST")
	v1.HandleFunc("/reports/{id}", h.GetReportStatus).Methods("GET")
	v1.HandleFunc("/reports/{id}/download", h.DownloadReport).Methods("GET")

	return r
}
