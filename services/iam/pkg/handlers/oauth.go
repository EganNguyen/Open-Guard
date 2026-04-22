package handlers

import (
	"fmt"
	"net/http"
	"net/url"
)

func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")

	if clientID == "" || redirectURI == "" {
		http.Error(w, "missing client_id or redirect_uri", http.StatusBadRequest)
		return
	}

	connector, err := h.svc.GetConnector(r.Context(), clientID)
	if err != nil {
		http.Error(w, "invalid client_id", http.StatusUnauthorized)
		return
	}

	// Validate redirect_uri
	validRedirect := false
	for _, uri := range connector["redirect_uris"].([]string) {
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
	// In a real app, we'd store the OAuth params in a session/cookie
	loginURL := fmt.Sprintf("http://localhost:4200/login?client_id=%s&redirect_uri=%s&state=%s", 
		url.QueryEscape(clientID), 
		url.QueryEscape(redirectURI), 
		url.QueryEscape(state))
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

	if clientID == "" {
		http.Error(w, "missing client_id", http.StatusBadRequest)
		return
	}

	_, err = h.svc.GetConnector(r.Context(), clientID)
	if err != nil {
		http.Error(w, "invalid client_id", http.StatusUnauthorized)
		return
	}

	// Skeleton for token exchange
	h.writeJSON(w, http.StatusOK, map[string]string{
		"access_token": "skeleton-token",
		"token_type":   "Bearer",
		"expires_in":   "3600",
	})
}
