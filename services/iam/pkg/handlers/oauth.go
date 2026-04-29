package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
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

		_, err = h.svc.GetConnector(r.Context(), clientID)
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

	token, err := h.svc.SignToken(orgID, userID, "oauth-jti-"+uuid.New().String(), 1*time.Hour)
	if err != nil {
		http.Error(w, "failed to sign token", http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}
