package handlers

import (
	"encoding/json"
	"net/http"
)

type DecisionRequest struct {
	SubjectID string `json:"subject_id"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
}

type DecisionResponse struct {
	Effect string `json:"effect"` // allow, deny
}

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var req DecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Fail-closed by default per spec §10
	resp := DecisionResponse{Effect: "deny"}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "OK"})
}
