package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openguard/services/threat/pkg/handlers"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/redis/go-redis/v9"
	"time"
)

func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(shared_middleware.SecurityHeaders)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "threat-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, nil)

	r.Route("/v1/threats", func(r chi.Router) {
		r.Use(shared_middleware.AuthJWTWithBlocklist(keyring, rdb, breaker))
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
