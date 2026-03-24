package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// In-memory data store for the Todo app
var (
	todosStore = make(map[string][]Todo) // Keyed by UserID
	storeMutex sync.RWMutex
	nextID     = 1
)

type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	CreatedAt time.Time `json:"created_at"`
}

// RequireOpenGuardHeaders is a middleware that enforces the zero-trust
// requirement: every request MUST have the X-User-ID header injected
// by the OpenGuard Gateway. If it's missing, the request bypassed the Gateway.
func RequireOpenGuardHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-ID")
		orgID := r.Header.Get("X-Org-ID")

		if userID == "" {
			http.Error(w, `{"error": "Missing OpenGuard identity headers. Bypassing the gateway is strictly prohibited."}`, http.StatusUnauthorized)
			return
		}

		// Inject the authenticated identity into the request context (or just pass headers)
		r.Header.Set("Authenticated-User", userID)
		r.Header.Set("Authenticated-Org", orgID)

		next.ServeHTTP(w, r)
	}
}

// writeJSON is a small helper for standardizing responses
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
