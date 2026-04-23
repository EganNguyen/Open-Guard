package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/openguard/services/iam/pkg/handlers"
	iam_middleware "github.com/openguard/services/iam/pkg/middleware"
	"github.com/openguard/services/iam/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	"golang.org/x/time/rate"
)

// Router sets up the HTTP routes for the IAM service.
func NewRouter(h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(iam_middleware.Correlation)
	r.Use(telemetry.Metrics)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestSize(1 << 20)) // 1MB limit (R-05)

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)
	
	r.Route("/mgmt", func(r chi.Router) {
		r.Use(iam_middleware.Auth(keyring, rdb))
		r.Post("/orgs", h.CreateOrg)
		r.Post("/users", h.CreateUser)
		r.Get("/connectors", h.ListConnectors)
		r.Post("/connectors", h.CreateConnector)
		r.Put("/connectors/{id}", h.UpdateConnector)
		r.Delete("/connectors/{id}", h.DeleteConnector)
		r.Get("/users", h.ListUsers)
		r.Get("/users/mfa/totp/setup", h.TOTPSetup)
		r.Post("/users/mfa/totp/enable", h.TOTPEnable)
	})
	
	authRateLimiter := iam_middleware.NewRateLimiter(rate.Limit(1), 5) // 1 req/sec, burst 5
	r.Route("/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(authRateLimiter.Limit)
			r.Post("/login", h.Login)
			r.Post("/oauth/login", h.OAuthLogin)
			r.Post("/refresh", h.Refresh)
			r.Post("/mfa/verify", h.VerifyMFA)
		})
		r.Post("/logout", h.Logout)
		r.Get("/authorize", h.Authorize)
		r.Post("/token", h.Token)
		
		r.Route("/scim/v2", func(r chi.Router) {
			r.Get("/Users", h.ListScimUsers)
			r.Get("/Users/{id}", h.GetScimUser)
		})

		r.Group(func(r chi.Router) {
			r.Use(iam_middleware.Auth(keyring, rdb))
			r.Get("/me", h.Me)
		})
	})

	return r
}
