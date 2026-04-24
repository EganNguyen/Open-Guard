package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openguard/services/connector-registry/pkg/handlers"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"time"
)

func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(shared_middleware.SecurityHeaders)

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "connector-registry-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, nil)

	r.Route("/v1/connectors", func(r chi.Router) {
		r.Use(shared_middleware.AuthJWTWithBlocklist(keyring, rdb, breaker))
		idemMiddleware := shared_middleware.IdempotencyMiddleware(rdb)
		r.With(idemMiddleware).Post("/", h.Register)
		r.Post("/validate", h.Validate)
	})

	return r
}
