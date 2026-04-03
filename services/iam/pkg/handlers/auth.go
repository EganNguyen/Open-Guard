package handlers

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/models"
)

type AuthHandler struct {
	iamService   *service.Service
	publicWebURL string
}

func NewAuthHandler(iamService *service.Service, publicWebURL string) *AuthHandler {
	return &AuthHandler{iamService: iamService, publicWebURL: publicWebURL}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterInput
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
	var req service.LoginInput
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
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	scope := r.URL.Query().Get("scope")

	if clientID == "" || redirectURI == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "client_id and redirect_uri are required", r)
		return
	}

	// Redirect to the dashboard login page with OIDC parameters.
	// The dashboard will handle authentication and then call /auth/oidc/authorize to get a real code.
	loginURL := h.publicWebURL + "/login?oidc_client_id=" + clientID + "&redirect_uri=" + redirectURI + "&state=" + state + "&scope=" + scope
	http.Redirect(w, r, loginURL, http.StatusFound)
}

func (h *AuthHandler) OIDCToken(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")

	if code == "" || clientID == "" {
		// Try to decode from JSON if not in Form
		var req struct {
			Code     string `json:"code"`
			ClientID string `json:"client_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			code = req.Code
			clientID = req.ClientID
		}
	}

	if code == "" || clientID == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "code and client_id are required", r)
		return
	}

	resp, err := h.iamService.ExchangeAuthCode(r.Context(), code, clientID)
	if err != nil {
		models.WriteError(w, http.StatusUnauthorized, "INVALID_CODE", err.Error(), r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  resp.Token,
		"id_token":      resp.Token, // In this specimen, we use the same signed JWT for both
		"refresh_token": resp.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    resp.ExpiresIn,
	})
}

func (h *AuthHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	// Internal/Authenticated endpoint called by the dashboard after user login
	var req struct {
		ClientID    string `json:"client_id"`
		RedirectURI string `json:"redirect_uri"`
		UserID      string `json:"user_id"`
		OrgID       string `json:"org_id"`
		Scope       string `json:"scope"`
		State       string `json:"state"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	code, err := h.iamService.CreateAuthCode(r.Context(), req.UserID, req.OrgID, req.ClientID, req.RedirectURI, req.Scope, req.State)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}

func (h *AuthHandler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "OIDC SSO is not yet implemented", r)
}

func (h *AuthHandler) OIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// For local testing on localhost, allow overriding to localhost:8081
	host := r.Host
	if strings.Contains(host, "openguard-iam") {
		host = "localhost:8081"
	}
	issuer := scheme + "://" + host

	resp := map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/auth/oidc/login",
		"token_endpoint":                        issuer + "/auth/oidc/token",
		"jwks_uri":                              issuer + "/auth/jwks",
		"response_types_supported":              []string{"code", "token", "id_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "org"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"claims_supported":                      []string{"sub", "iss", "aud", "exp", "iat", "org_id"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) JWKS(w http.ResponseWriter, r *http.Request) {
	// Derive public keys from our configured JWT keyring
	keys := h.iamService.GetJWTKeys()
	var jwks []map[string]interface{}

	for _, key := range keys {
		if strings.HasPrefix(key.Algorithm, "RS") {
			// Parse private key to get public key
			priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(key.Secret))
			if err != nil {
				continue
			}

			// Format as JWK
			n := priv.N.Bytes()
			e := priv.E

			jwk := map[string]interface{}{
				"kty": "RSA",
				"use": "sig",
				"kid": key.Kid,
				"alg": key.Algorithm,
				"n":   base64.RawURLEncoding.EncodeToString(n),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(e)).Bytes()),
			}
			jwks = append(jwks, jwk)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"keys": jwks,
	})
}
