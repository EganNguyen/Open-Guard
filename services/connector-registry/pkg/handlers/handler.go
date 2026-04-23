package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openguard/services/connector-registry/pkg/service"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID           string   `json:"id"`
		OrgID        string   `json:"org_id"`
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	apiKey, err := h.svc.RegisterConnector(r.Context(), body.ID, body.OrgID, body.Name, body.RedirectURIs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{
		"id":      body.ID,
		"api_key": apiKey,
	})
}

func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		http.Error(w, "missing x-api-key header", http.StatusUnauthorized)
		return
	}

	connector, err := h.svc.ValidateAPIKey(r.Context(), apiKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	h.writeJSON(w, http.StatusOK, connector)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}
