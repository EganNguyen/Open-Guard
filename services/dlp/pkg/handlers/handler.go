package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/openguard/services/dlp/pkg/repository"
	"github.com/openguard/services/dlp/pkg/scanner"
	"github.com/openguard/shared/middleware"
)

type DLPHandler struct {
	repo *repository.Repository
}

func NewDLPHandler(repo *repository.Repository) *DLPHandler {
	return &DLPHandler{repo: repo}
}

func (h *DLPHandler) Scan(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// 1. Run scanners
	regexFindings := scanner.ScanRegex(req.Content)
	entropyFindings := scanner.ScanEntropy(req.Content)
	
	allFindings := append(regexFindings, entropyFindings...)

	// 2. Persist findings (simplified: match against all active policies)
	policies, _ := h.repo.ListPolicies(r.Context(), orgID)
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		for _, f := range allFindings {
			// Check if finding kind matches policy rules
			match := false
			for _, rule := range p.Rules {
				if rule == f.Kind || rule == "all" {
					match = true
					break
				}
			}
			if match {
				h.repo.SaveFinding(r.Context(), &repository.DLPFinding{
					OrgID:    orgID,
					PolicyID: p.ID,
					Kind:     f.Kind,
					Action:   p.Action,
				})
			}
		}
	}

	type ScanResponseFinding struct {
		Kind      string  `json:"kind"`
		Value     string  `json:"value"` // This is now masked by ScanRegex
		RiskScore float64 `json:"risk_score"`
		Location  string  `json:"location,omitempty"`
	}

	var response []ScanResponseFinding
	for _, f := range allFindings {
		response = append(response, ScanResponseFinding{
			Kind:      f.Kind,
			Value:     f.Value,
			RiskScore: f.RiskScore,
			Location:  f.Location,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *DLPHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	policies, err := h.repo.ListPolicies(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policies)
}

func (h *DLPHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	var p repository.DLPPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	p.OrgID = orgID
	if err := h.repo.CreatePolicy(r.Context(), &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func (h *DLPHandler) ListFindings(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	findings, err := h.repo.ListFindings(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(findings)
}
