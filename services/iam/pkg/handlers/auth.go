package handlers

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/models"
)

type AuthHandler struct {
	iamService *service.Service
}

func NewAuthHandler(iamService *service.Service) *AuthHandler {
	return &AuthHandler{iamService: iamService}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	resp, err := h.iamService.Register(r.Context(), req)
	if err != nil {
		models.HandleServiceError(w, r, err)
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
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	ua := r.UserAgent()

	resp, err := h.iamService.Login(r.Context(), req, &ip, &ua)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_session",
		Value:    resp.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Should be true in production
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30, // 30 days or align with session expiry
	})

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

	orgID := orgIDFromCtx(r)
	userID := r.Header.Get("X-User-ID")

	sessionID := req.SessionID
	if sessionID == "" {
		if cookie, err := r.Cookie("auth_session"); err == nil {
			sessionID = cookie.Value
		}
	}

	if err := h.iamService.Logout(r.Context(), sessionID, orgID, userID); err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("auth_session")
	if err != nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing session cookie", r)
		return
	}

	orgID := orgIDFromCtx(r)

	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	ua := r.UserAgent()

	resp, err := h.iamService.Refresh(r.Context(), cookie.Value, orgID, &ip, &ua)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	// Session extended and rotated, update the cookie with the new refresh token
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_session",
		Value:    resp.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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
