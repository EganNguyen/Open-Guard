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
	"golang.org/x/time/rate"
)

func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client, stop <-chan struct{}) *chi.Mux {
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

	// Ingest endpoints need 20,000 req/s. We will use a reasonably high limit per IP for public endpoints.
	rateLimiter := shared_middleware.NewRateLimiter(rdb, rate.Limit(1000), 2000, stop)

	r.Route("/v1/connectors", func(r chi.Router) {
		r.Use(rateLimiter.Limit)
		r.Use(shared_middleware.AuthJWTWithBlocklist(keyring, rdb, breaker))
		idemMiddleware := shared_middleware.IdempotencyMiddleware(rdb)
		r.With(idemMiddleware).Post("/", h.Register)
		r.Post("/validate", h.Validate)
	})

	return r
}
