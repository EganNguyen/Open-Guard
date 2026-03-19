package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

// AuthHandler handles authentication HTTP endpoints.
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register handles POST /auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req service.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid JSON body", reqID)
		return
	}

	resp, err := h.authService.Register(r.Context(), req)
	if err != nil {
		models.WriteError(w, http.StatusBadRequest, "REGISTRATION_FAILED",
			err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// Login handles POST /auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid JSON body", reqID)
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()

	resp, err := h.authService.Login(r.Context(), req, &ip, &ua)
	if err != nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED",
			"Invalid credentials", reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Logout handles POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid JSON body", reqID)
		return
	}

	orgID := r.Header.Get("X-Org-ID")
	userID := r.Header.Get("X-User-ID")

	if err := h.authService.Logout(r.Context(), req.SessionID, orgID, userID); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LOGOUT_FAILED",
			err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Refresh handles POST /auth/refresh — stub for Phase 1.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"Token refresh is not yet implemented", reqID)
}

// SAMLCallback handles POST /auth/saml/callback — stub.
func (h *AuthHandler) SAMLCallback(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"SAML SSO is not yet implemented", reqID)
}

// OIDCLogin handles GET /auth/oidc/login — stub.
func (h *AuthHandler) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"OIDC SSO is not yet implemented", reqID)
}

// OIDCCallback handles GET /auth/oidc/callback — stub.
func (h *AuthHandler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"OIDC SSO is not yet implemented", reqID)
}
