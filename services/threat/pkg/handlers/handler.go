package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/services/threat/pkg/alert"
)

type Handler struct {
	store *alert.Store
}

func NewHandler(store *alert.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id") // Should come from middleware in production
	status := r.URL.Query().Get("status")
	severity := r.URL.Query().Get("severity")
	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")

	limit := int64(20)
	if limitStr != "" {
		if l, err := strconv.ParseInt(limitStr, 10, 64); err == nil {
			limit = l
		}
	}

	alerts, nextCursor, err := h.store.ListAlerts(r.Context(), orgID, status, severity, limit, cursor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts":      alerts,
		"next_cursor": nextCursor,
	})
}

func (h *Handler) GetAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	alert, err := h.store.GetAlert(r.Context(), id)
	if err != nil {
		http.Error(w, "Alert not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alert)
}

func (h *Handler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.store.AcknowledgeAlert(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.store.ResolveAlert(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	stats, err := h.store.GetStats(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *Handler) ListDetectors(w http.ResponseWriter, r *http.Request) {
	detectors := []map[string]interface{}{
		{"name": "Brute Force", "status": "active", "weight": 0.9},
		{"name": "Impossible Travel", "status": "active", "weight": 0.9},
		{"name": "Off-Hours Access", "status": "active", "weight": 0.5},
		{"name": "Data Exfiltration", "status": "active", "weight": 0.7},
		{"name": "Account Takeover", "status": "active", "weight": 0.7},
		{"name": "Privilege Escalation", "status": "active", "weight": 0.9},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detectors)
}
