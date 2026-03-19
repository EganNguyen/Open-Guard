package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/models"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	resp, err := h.authService.Register(r.Context(), req)
	if err != nil {
		models.WriteError(w, http.StatusBadRequest, "REGISTRATION_FAILED", err.Error(), r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()

	resp, err := h.authService.Login(r.Context(), req, &ip, &ua)
	if err != nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid credentials", r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	orgID := r.Header.Get("X-Org-ID")
	userID := r.Header.Get("X-User-ID")

	if err := h.authService.Logout(r.Context(), req.SessionID, orgID, userID); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LOGOUT_FAILED", err.Error(), r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Token refresh is not yet implemented", r)
}

func (h *AuthHandler) SAMLCallback(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "SAML SSO is not yet implemented", r)
}

func (h *AuthHandler) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "OIDC SSO is not yet implemented", r)
}

func (h *AuthHandler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "OIDC SSO is not yet implemented", r)
}
