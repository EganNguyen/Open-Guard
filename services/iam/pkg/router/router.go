package router

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/openguard/services/iam/pkg/handlers"
	iam_middleware "github.com/openguard/services/iam/pkg/middleware"
	"github.com/openguard/services/iam/pkg/telemetry"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
	"golang.org/x/time/rate"
	"os"
)

// Router sets up the HTTP routes for the IAM service.
func NewRouter(ctx context.Context, h *handlers.Handler, keyring []crypto.JWTKey, rdb *redis.Client, stop <-chan struct{}) *chi.Mux {
	r := chi.NewRouter()

	// Initialize WebAuthn
	rpID := os.Getenv("WEBAUTHN_RP_ID")
	if rpID == "" {
		rpID = "localhost"
	}
	rpOrigin := os.Getenv("WEBAUTHN_RP_ORIGIN")
	if rpOrigin == "" {
		rpOrigin = "http://localhost:4200"
	}

	w, _ := webauthn.New(&webauthn.Config{
		RPDisplayName: "OpenGuard",
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
	})
	h.SetServiceWebAuthn(w) // Helper needed in handler or just svc.SetWebAuthn(w)

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(iam_middleware.Correlation)
	r.Use(telemetry.Metrics)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestSize(1 << 20)) // 1MB limit (R-05)
	r.Use(shared_middleware.SecurityHeaders)

	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", h.Health)

	// Circuit breaker for Redis blocklist check
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:             "iam-redis-blocklist",
		MaxRequests:      5,
		Interval:         10 * time.Second,
		FailureThreshold: 3,
		OpenDuration:     5 * time.Second,
	}, iam_middleware.GetLogger(ctx))

	authMiddleware := shared_middleware.AuthJWTWithBlocklist(keyring, rdb, breaker)
	idemMiddleware := shared_middleware.IdempotencyMiddleware(rdb)

	r.Route("/mgmt", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/orgs", h.CreateOrg)
		r.With(idemMiddleware).Post("/users", h.CreateUser)
		r.Get("/connectors", h.ListConnectors)
		r.Post("/connectors", h.CreateConnector)
		r.Put("/connectors/{id}", h.UpdateConnector)
		r.Delete("/connectors/{id}", h.DeleteConnector)
		r.Get("/users", h.ListUsers)
		r.Post("/users/{id}/reprovision", h.ReprovisionUser)
		r.Get("/users/mfa/totp/setup", h.TOTPSetup)
		r.Post("/users/mfa/totp/enable", h.TOTPEnable)
	})

	authRateLimiter := shared_middleware.NewRateLimiter(rdb, rate.Limit(1), 5, stop) // 1 req/sec, burst 5
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
		r.With(idemMiddleware).Post("/token", h.Token)

	scimTokens := shared_middleware.LoadSCIMTokensFromEnv()
	scimMiddleware := shared_middleware.SCIMAuth(scimTokens)

	r.Route("/scim/v2", func(r chi.Router) {
			r.Use(scimMiddleware)
			r.Use(idemMiddleware)
			r.Get("/Users", h.ListScimUsers)
			r.Post("/Users", h.PostScimUser)
			r.Get("/Users/{id}", h.GetScimUser)
			r.Delete("/Users/{id}", h.DeleteScimUser)
			r.Patch("/Users/{id}", h.PatchScimUser)
		})

		r.Route("/webauthn", func(r chi.Router) {
			r.Post("/login/begin", h.WebAuthnBeginLogin)
			r.Post("/login/finish", h.WebAuthnFinishLogin)

			r.Group(func(r chi.Router) {
				r.Use(authMiddleware)
				r.Post("/register/begin", h.WebAuthnBeginRegistration)
				r.Post("/register/finish", h.WebAuthnFinishRegistration)
			})
		})

		r.Route("/saml", func(r chi.Router) {
			r.Get("/metadata", h.SAMLMetadata)
			r.Post("/acs", h.SAMLAssertionConsumerService)

			r.Group(func(r chi.Router) {
				r.Use(authMiddleware)
				r.Post("/providers", h.CreateSAMLProvider)
				r.Get("/providers", h.ListSAMLProviders)
			})
		})
	})

	return r
}
