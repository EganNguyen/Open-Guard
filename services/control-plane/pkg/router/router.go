package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sony/gobreaker"

	"github.com/openguard/control-plane/pkg/middleware"
	"github.com/openguard/control-plane/pkg/telemetry"
	"github.com/openguard/control-plane/pkg/proxy"
)

func NewRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Correlation)
	r.Use(telemetry.Metrics)
	r.Use(chimiddleware.Recoverer)

	r.Handle("/metrics", promhttp.Handler())
	
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status": "OK"}`))
	})

	// Circuit Breakers
	cbPolicy := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "cb-policy"})
	cbIAM := gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "cb-iam"})

	iamURL := "http://iam:8080"
	policyURL := "http://policy:8080"
	auditURL := "http://audit:8080"

	r.Route("/v1", func(r chi.Router) {
		r.Post("/policy/evaluate", proxy.NewProxy(policyURL, cbPolicy))
		r.Post("/events/ingest", proxy.NewProxy(auditURL, nil)) // no cb for audit as per table
		
		r.Route("/scim/v2/Users", func(r chi.Router) {
			r.Get("/", proxy.NewProxy(iamURL, cbIAM))
			r.Post("/", proxy.NewProxy(iamURL, cbIAM))
			r.Get("/{id}", proxy.NewProxy(iamURL, cbIAM))
			r.Patch("/{id}", proxy.NewProxy(iamURL, cbIAM))
		})
	})

	return r
}
