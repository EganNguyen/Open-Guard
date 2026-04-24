package router

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/iam/pkg/handlers"
	iam_middleware "github.com/openguard/services/iam/pkg/middleware"
	"github.com/openguard/services/iam/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"golang.org/x/time/rate"
)

// Router sets up the HTTP routes for the IAM service.
func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client, stop <-chan struct{}) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(iam_middleware.Correlation)
	r.Use(telemetry.Metrics)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestSize(1 << 20)) // 1MB limit (R-05)

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

	// Circuit breaker for Redis blocklist check
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "iam-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, iam_middleware.GetLogger(nil)) // Using default logger for now

	authMiddleware := shared_middleware.AuthJWTWithBlocklist(keyring, rdb, breaker)

	r.Route("/mgmt", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/orgs", h.CreateOrg)
		r.Post("/users", h.CreateUser)
		r.Get("/connectors", h.ListConnectors)
		r.Post("/connectors", h.CreateConnector)
		r.Put("/connectors/{id}", h.UpdateConnector)
		r.Delete("/connectors/{id}", h.DeleteConnector)
		r.Get("/users", h.ListUsers)
		r.Post("/users/{id}/reprovision", h.ReprovisionUser)
		r.Get("/users/mfa/totp/setup", h.TOTPSetup)
		r.Post("/users/mfa/totp/enable", h.TOTPEnable)
	})

	authRateLimiter := iam_middleware.NewRateLimiter(rate.Limit(1), 5, stop) // 1 req/sec, burst 5
	r.Route("/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(authRateLimiter.Limit)
			r.Post("/login", h.Login)
			r.Post("/oauth/login", h.OAuthLogin)
			r.Post("/refresh", h.Refresh)
			r.Post("/mfa/verify", h.VerifyMFA)
			r.Post("/mfa/backup-verify", h.VerifyBackupCode)
		})

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware)
			r.Post("/logout", h.Logout)
			r.Get("/me", h.Me)
		})

		r.Get("/authorize", h.Authorize)
		r.Post("/token", h.Token)

		r.Route("/scim/v2", func(r chi.Router) {
			r.Get("/Users", h.ListScimUsers)
			r.Post("/Users", h.PostScimUser)
			r.Get("/Users/{id}", h.GetScimUser)
		})
	})

	return r
}
