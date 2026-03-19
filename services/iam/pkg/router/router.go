package router

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/iam/pkg/handlers"
	sharedmw "github.com/openguard/shared/middleware"
)

// Config holds dependencies needed to build the IAM router.
type Config struct {
	AuthHandler  *handlers.AuthHandler
	UserHandler  *handlers.UserHandler
	MFAHandler   *handlers.MFAHandler
	SCIMHandler  *handlers.SCIMHandler
	TokenHandler *handlers.TokenHandler
	Logger       *slog.Logger
}

// New creates the IAM chi router with all routes.
func New(cfg Config) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(sharedmw.RequestID)
	r.Use(sharedmw.Logging(cfg.Logger))

	// Health endpoints
	r.Get("/health/live", healthHandler)
	r.Get("/health/ready", healthHandler) // TODO: check DB + Kafka

	// --- Auth routes (no JWT required — gateway handles auth) ---
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", cfg.AuthHandler.Register)
		r.Post("/login", cfg.AuthHandler.Login)
		r.Post("/logout", cfg.AuthHandler.Logout)
		r.Post("/refresh", cfg.AuthHandler.Refresh)
		r.Post("/saml/callback", cfg.AuthHandler.SAMLCallback)
		r.Get("/oidc/login", cfg.AuthHandler.OIDCLogin)
		r.Get("/oidc/callback", cfg.AuthHandler.OIDCCallback)
		r.Post("/mfa/enroll", cfg.MFAHandler.Enroll)
		r.Post("/mfa/verify", cfg.MFAHandler.Verify)
		r.Post("/mfa/challenge", cfg.MFAHandler.Challenge)
	})

	// --- User routes (gateway validates JWT and injects X-User-ID headers) ---
	r.Route("/users", func(r chi.Router) {
		r.Get("/", cfg.UserHandler.List)
		r.Post("/", cfg.UserHandler.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", cfg.UserHandler.Get)
			r.Patch("/", cfg.UserHandler.Update)
			r.Delete("/", cfg.UserHandler.Delete)
			r.Post("/suspend", cfg.UserHandler.Suspend)
			r.Post("/activate", cfg.UserHandler.Activate)
			r.Get("/sessions", cfg.UserHandler.ListSessions)
			r.Delete("/sessions/{sid}", cfg.UserHandler.RevokeSession)
			r.Get("/tokens", cfg.UserHandler.ListTokens)
			r.Post("/tokens", cfg.TokenHandler.Create)
			r.Delete("/tokens/{tid}", cfg.UserHandler.RevokeToken)
		})
	})

	// --- SCIM v2 routes ---
	r.Route("/scim/v2", func(r chi.Router) {
		r.Get("/Users", cfg.SCIMHandler.ListUsers)
		r.Post("/Users", cfg.SCIMHandler.CreateUser)
		r.Get("/Users/{id}", cfg.SCIMHandler.GetUser)
		r.Put("/Users/{id}", cfg.SCIMHandler.ReplaceUser)
		r.Patch("/Users/{id}", cfg.SCIMHandler.UpdateUser)
		r.Delete("/Users/{id}", cfg.SCIMHandler.DeleteUser)
		r.Get("/Groups", cfg.SCIMHandler.ListGroups)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
