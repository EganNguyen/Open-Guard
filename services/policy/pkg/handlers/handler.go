package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/services/policy/pkg/repository"
	"github.com/openguard/services/policy/pkg/service"
)

// Handler manages HTTP requests for the policy service.
type Handler struct {
	svc    *service.Service
	repo   *repository.Repository
	logger *slog.Logger
}

// NewHandler creates a new handler instance.
func NewHandler(svc *service.Service, repo *repository.Repository, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, repo: repo, logger: logger}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

// Health returns the service health status.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "OK", "service": "policy"})
}

// Evaluate handles POST /v1/policy/evaluate
// Implements two-tier caching with singleflight per spec §11.3.
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var req service.EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OrgID == "" || req.SubjectID == "" || req.Action == "" || req.Resource == "" {
		h.writeError(w, http.StatusBadRequest, "org_id, subject_id, action, and resource are required")
		return
	}

	resp, err := h.svc.Evaluate(r.Context(), req)
	if err != nil {
		h.logger.Error("evaluate failed", "error", err, "org_id", req.OrgID)
		h.writeError(w, http.StatusInternalServerError, "evaluation failed")
		return
	}

	// Set ETag based on response content for client-side caching
	etag := fmt.Sprintf(`"%s-v%d"`, req.OrgID, resp.MaxVersion)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store") // Per spec: decisions must not be cached by intermediaries

	h.writeJSON(w, http.StatusOK, resp)
}

// ListPolicies handles GET /v1/policies
func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		h.writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	policies, err := h.repo.ListPolicies(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list policies failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to list policies")
		return
	}

	if policies == nil {
		policies = []repository.Policy{}
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"policies": policies,
		"total":    len(policies),
	})
}

// CreatePolicy handles POST /v1/policies
func (h *Handler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID       string          `json:"org_id"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Logic       json.RawMessage `json:"logic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.OrgID == "" || body.Name == "" || len(body.Logic) == 0 {
		h.writeError(w, http.StatusBadRequest, "org_id, name, and logic are required")
		return
	}

	// Validate logic JSON is valid
	var logicCheck map[string]interface{}
	if err := json.Unmarshal(body.Logic, &logicCheck); err != nil {
		h.writeError(w, http.StatusBadRequest, "logic must be valid JSON")
		return
	}

	policy, err := h.svc.CreatePolicy(r.Context(), body.OrgID, body.Name, body.Description, body.Logic)
	if err != nil {
		h.logger.Error("create policy failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to create policy")
		return
	}

	// Invalidate cache for the org after mutation
	go h.svc.InvalidateOrgCache(r.Context(), body.OrgID)

	w.Header().Set("ETag", fmt.Sprintf(`"%s-v%d"`, policy.ID, policy.Version))
	h.writeJSON(w, http.StatusCreated, policy)
}

// GetPolicy handles GET /v1/policies/{id}
func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	orgID := r.URL.Query().Get("org_id")

	if policyID == "" || orgID == "" {
		h.writeError(w, http.StatusBadRequest, "id and org_id are required")
		return
	}

	policy, err := h.repo.GetPolicy(r.Context(), orgID, policyID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "policy not found")
			return
		}
		h.logger.Error("get policy failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to get policy")
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%s-v%d"`, policy.ID, policy.Version))
	h.writeJSON(w, http.StatusOK, policy)
}

// UpdatePolicy handles PUT /v1/policies/{id}
func (h *Handler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	var body struct {
		OrgID       string          `json:"org_id"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Logic       json.RawMessage `json:"logic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if policyID == "" || body.OrgID == "" {
		h.writeError(w, http.StatusBadRequest, "id and org_id are required")
		return
	}

	policy, err := h.svc.UpdatePolicy(r.Context(), body.OrgID, policyID, body.Name, body.Description, body.Logic)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "policy not found")
			return
		}
		h.logger.Error("update policy failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to update policy")
		return
	}

	// Invalidate cache for the org after mutation
	go h.svc.InvalidateOrgCache(r.Context(), body.OrgID)

	w.Header().Set("ETag", fmt.Sprintf(`"%s-v%d"`, policy.ID, policy.Version))
	h.writeJSON(w, http.StatusOK, policy)
}

// DeletePolicy handles DELETE /v1/policies/{id}
func (h *Handler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	orgID := r.URL.Query().Get("org_id")

	if policyID == "" || orgID == "" {
		h.writeError(w, http.StatusBadRequest, "id and org_id are required")
		return
	}

	if err := h.svc.DeletePolicy(r.Context(), orgID, policyID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "policy not found")
			return
		}
		h.logger.Error("delete policy failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to delete policy")
		return
	}

	// Invalidate cache for the org after mutation
	go h.svc.InvalidateOrgCache(r.Context(), orgID)

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListEvalLogs handles GET /v1/policy/eval-logs
func (h *Handler) ListEvalLogs(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		h.writeError(w, http.StatusBadRequest, "org_id is required")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	logs, err := h.repo.ListEvalLogs(r.Context(), orgID, limit)
	if err != nil {
		h.logger.Error("list eval logs failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to list eval logs")
		return
	}

	if logs == nil {
		logs = []repository.EvalLog{}
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": len(logs),
	})
}
