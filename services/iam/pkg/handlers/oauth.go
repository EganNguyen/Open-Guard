package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/google/uuid"
	"context"
	"github.com/openguard/services/iam/pkg/service"
)

	func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request) {
		clientID := r.URL.Query().Get("client_id")
		redirectURI := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")
		codeChallenge := r.URL.Query().Get("code_challenge")
		codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

		if clientID == "" || redirectURI == "" {
			http.Error(w, "missing client_id or redirect_uri", http.StatusBadRequest)
			return
		}
		
		if codeChallenge == "" {
			http.Error(w, "code_challenge required", http.StatusBadRequest)
			return
		}
		if codeChallengeMethod != "S256" {
			http.Error(w, "only S256 code_challenge_method supported", http.StatusBadRequest)
			return
		}

	connector, err := h.svc.GetConnector(r.Context(), clientID)
	if err != nil {
		http.Error(w, "invalid client_id", http.StatusUnauthorized)
		return
	}

	// Validate redirect_uri
	validRedirect := false
	rawURIs, ok := connector["redirect_uris"]
	if !ok {
		http.Error(w, "connector misconfigured", http.StatusInternalServerError)
		return
	}

	var redirectURIs []string
	switch v := rawURIs.(type) {
	case []string:
		redirectURIs = v
	case []interface{}:
		for _, u := range v {
			if s, ok := u.(string); ok {
				redirectURIs = append(redirectURIs, s)
			}
		}
	default:
		http.Error(w, "connector misconfigured", http.StatusInternalServerError)
		return
	}

	for _, uri := range redirectURIs {
		if uri == redirectURI {
			validRedirect = true
			break
		}
	}

	if !validRedirect {
		http.Error(w, "invalid redirect_uri", http.StatusUnauthorized)
		return
	}

	// Redirect to the OpenGuard Login UI
	dashboardURL := os.Getenv("OPENGUARD_DASHBOARD_URL")
	if dashboardURL == "" {
		dashboardURL = "http://localhost:4200"
	}

		loginURL := fmt.Sprintf("%s/login?client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=%s",
			dashboardURL,
			url.QueryEscape(clientID),
			url.QueryEscape(redirectURI),
			url.QueryEscape(state),
			url.QueryEscape(codeChallenge),
			url.QueryEscape(codeChallengeMethod))
		http.Redirect(w, r, loginURL, http.StatusFound)
	}

func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	grantType := r.Form.Get("grant_type")
	if grantType == "" {
		grantType = "authorization_code" // Default for backward compatibility
	}

	switch grantType {
	case "authorization_code":
		h.handleAuthCodeGrant(w, r)
	case "refresh_token":
		h.handleRefreshTokenGrant(w, r)
	case "password":
		h.handlePasswordGrant(w, r)
	default:
		http.Error(w, "unsupported_grant_type", http.StatusBadRequest)
	}
}

func (h *Handler) handleAuthCodeGrant(w http.ResponseWriter, r *http.Request) {
	clientID := r.Form.Get("client_id")
	if clientID == "" {
		clientID, _, _ = r.BasicAuth()
	}
	code := r.Form.Get("code")
	codeVerifier := r.Form.Get("code_verifier")

	if clientID == "" || code == "" || codeVerifier == "" {
		http.Error(w, "missing client_id, code, or code_verifier", http.StatusBadRequest)
		return
	}

	_, err := h.svc.GetConnector(r.Context(), clientID)
	if err != nil {
		http.Error(w, "invalid client_id", http.StatusUnauthorized)
		return
	}

	// Verify the 'code' from Redis (R-03)
	orgID, userID, storedChallenge, err := h.svc.GetAuthCode(r.Context(), code)
	if err != nil {
		http.Error(w, "invalid or expired code", http.StatusUnauthorized)
		return
	}

	// PKCE Verification
	h256 := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h256[:])
	if subtle.ConstantTimeCompare([]byte(computed), []byte(storedChallenge)) != 1 {
		http.Error(w, "invalid code_verifier", http.StatusUnauthorized)
		return
	}

	res, err := h.svc.IssueTokens(r.Context(), orgID, userID, r.UserAgent(), r.RemoteAddr, uuid.New())
	if err != nil {
		http.Error(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  res["access_token"],
		"refresh_token": res["refresh_token"],
		"token_type":    "Bearer",
		"expires_in":    res["expires_in"],
	})
}

func (h *Handler) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.Form.Get("refresh_token")
	if refreshToken == "" {
		http.Error(w, "missing refresh_token", http.StatusBadRequest)
		return
	}

	res, err := h.svc.RefreshToken(r.Context(), refreshToken, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  res["access_token"],
		"refresh_token": res["refresh_token"],
		"token_type":    "Bearer",
		"expires_in":    res["expires_in"],
	})
}

func (h *Handler) handlePasswordGrant(w http.ResponseWriter, r *http.Request) {
	email := r.Form.Get("username")
	password := r.Form.Get("password")

	if email == "" || password == "" {
		http.Error(w, "missing username or password", http.StatusBadRequest)
		return
	}

	user, _, err := h.svc.Login(r.Context(), email, password, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if mfa, ok := user["mfa_required"].(bool); ok && mfa {
		h.writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":         "mfa_required",
			"mfa_challenge": user["mfa_challenge"],
		})
		return
	}

	// Issue full tokens (Access + Refresh)
	res, err := h.svc.IssueTokens(r.Context(), user["org_id"].(string), user["id"].(string), r.UserAgent(), r.RemoteAddr, uuid.New())
	if err != nil {
		http.Error(w, "failed to issue tokens", http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  res["access_token"],
		"refresh_token": res["refresh_token"],
		"token_type":    "Bearer",
		"expires_in":    res["expires_in"],
	})
}

func (h *Handler) JWKS(w http.ResponseWriter, r *http.Request) {
	keyring := h.svc.GetKeyring()
	var keys []map[string]interface{}

	for _, k := range keyring {
		// Only expose KID and Algorithm for public JWKS (RFC 7517)
		// Since we use HS256 (symmetric), we don't expose N/E public components.
		// For symmetric keys, JWKS usually includes 'k', but we OMIT it for security.
		// External clients validate tokens via their own knowledge of the secret
		// or by calling IAM. OIDC spec for HS256 is subtle.
		keys = append(keys, map[string]interface{}{
			"kid": k.Kid,
			"kty": "oct", // Octet sequence for symmetric keys
			"alg": k.Algorithm,
			"use": "sig",
		})
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

func (h *Handler) OIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	issuer := os.Getenv("OIDC_ISSUER_URL")
	if issuer == "" {
		issuer = "http://localhost:8081"
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/auth/authorize",
		"token_endpoint":                        issuer + "/auth/token",
		"userinfo_endpoint":                     issuer + "/auth/me",
		"jwks_uri":                              issuer + "/oauth/jwks",
		"response_types_supported":               []string{"code", "token", "id_token"},
		"grant_types_supported":                  []string{"authorization_code", "refresh_token", "password"},
		"subject_types_supported":                []string{"public"},
		"id_token_signing_alg_values_supported": []string{"HS256"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"claims_supported":                      []string{"iss", "sub", "aud", "exp", "iat", "org_id"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

// generateOAuthCode creates a random auth code, stores it in Redis with PKCE challenge,
// and returns the code string. Used by the OAuthLogin handler.
func generateOAuthCode(ctx context.Context, svc *service.Service, orgID, userID, codeChallenge string) (string, error) {
	code := base64.RawURLEncoding.EncodeToString([]byte(uuid.New().String()))
	if err := svc.StoreAuthCode(ctx, code, orgID, userID, codeChallenge); err != nil {
		return "", err
	}
	return code, nil
}
