package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/openguard/services/alerting/pkg/repository"
	"github.com/openguard/shared/middleware"
)

type AlertHandler struct {
	repo *repository.Repository
}

func NewAlertHandler(repo *repository.Repository) *AlertHandler {
	return &AlertHandler{repo: repo}
}

func (h *AlertHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	status := repository.AlertStatus(r.URL.Query().Get("status"))
	severity := repository.AlertSeverity(r.URL.Query().Get("severity"))
	cursor := r.URL.Query().Get("cursor")
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if limit == 0 {
		limit = 50
	}

	alerts, nextCursor, err := h.repo.List(r.Context(), orgID, status, severity, cursor, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("X-Next-Cursor", nextCursor)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func (h *AlertHandler) GetAlert(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id := mux.Vars(r)["id"]

	alert, err := h.repo.GetByID(r.Context(), orgID, id)
	if err != nil {
		http.Error(w, "Alert not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alert)
}

func (h *AlertHandler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id := mux.Vars(r)["id"]

	if err := h.repo.Acknowledge(r.Context(), orgID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id := mux.Vars(r)["id"]

	if err := h.repo.Resolve(r.Context(), orgID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AlertHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())

	stats, err := h.repo.GetStats(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *AlertHandler) GetDetectors(w http.ResponseWriter, r *http.Request) {
	// Mock detectors for now as per spec §13 requirement for list and weights
	detectors := []map[string]interface{}{
		{"id": "brute-force", "name": "Brute Force Detector", "weight": 0.8, "status": "active"},
		{"id": "impossible-travel", "name": "Impossible Travel", "weight": 0.9, "status": "active"},
		{"id": "dlp-pii", "name": "DLP PII Scanner", "weight": 0.7, "status": "active"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detectors)
}
