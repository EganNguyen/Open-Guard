package router

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openguard/policy/pkg/handlers"
	"github.com/openguard/policy/pkg/tenant"
	sharedmw "github.com/openguard/shared/middleware"
)

// Config holds the router's dependencies.
type Config struct {
	PolicyHandler *handlers.PolicyHandler
	Logger        *slog.Logger
}

// New constructs the chi router for the policy service.
// JWT authentication is done by the gateway; it injects X-Org-ID and X-User-ID headers.
func New(cfg Config) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(sharedmw.RequestID)
	r.Use(sharedmw.Logging(cfg.Logger))

	r.Get("/health", healthHandler)
	r.Get("/health/live", healthHandler)
	r.Get("/health/ready", healthHandler)

	// All policy routes require a tenant context from the gateway-injected X-Org-ID header.
	r.Group(func(r chi.Router) {
		r.Use(injectOrgContext)

		r.Post("/policies/evaluate", cfg.PolicyHandler.Evaluate)

		r.Post("/policies", cfg.PolicyHandler.Create)
		r.Get("/policies", cfg.PolicyHandler.List)
		r.Get("/policies/{id}", cfg.PolicyHandler.Get)
		r.Put("/policies/{id}", cfg.PolicyHandler.Update)
		r.Delete("/policies/{id}", cfg.PolicyHandler.Delete)
	})

	return r
}

// injectOrgContext reads X-Org-ID and X-User-ID gateway headers and puts them in context.
func injectOrgContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := r.Header.Get("X-Org-ID")
		userID := r.Header.Get("X-User-ID")
		if orgID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "MISSING_ORG_CONTEXT",
					"message": "X-Org-ID header is required",
				},
			})
			return
		}
		ctx := context.WithValue(r.Context(), tenant.OrgIDKey, orgID)
		ctx = context.WithValue(ctx, tenant.UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OrgIDFromContext retrieves the org ID stored by injectOrgContext.
func OrgIDFromContext(ctx context.Context) string {
	return tenant.OrgIDFromContext(ctx)
}

// UserIDFromContext retrieves the user ID stored by injectOrgContext.
func UserIDFromContext(ctx context.Context) string {
	return tenant.UserIDFromContext(ctx)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
