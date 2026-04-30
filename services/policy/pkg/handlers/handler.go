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
	"github.com/openguard/shared/rls"
)

// Handler manages HTTP requests for the policy service.
type Handler struct {
	svc    *service.Service
	logger *slog.Logger
}

// NewHandler creates a new handler instance.
func NewHandler(svc *service.Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

// orgIDFromContext extracts the org_id injected by AuthJWTWithBlocklist via
// rls.WithOrgID. Returns ("", false) when the org_id is absent, which must be
// treated as an internal error (middleware misconfiguration).
func orgIDFromContext(r *http.Request) (string, bool) {
	id := rls.OrgID(r.Context())
	return id, id != ""
}

// Health returns the service health status.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "OK", "service": "policy"})
}

// Evaluate handles POST /v1/policy/evaluate
// Implements two-tier caching with singleflight per spec §11.3.
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	ctxOrgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	var req service.EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SubjectID == "" || req.Action == "" || req.Resource == "" {
		h.writeError(w, http.StatusBadRequest, "subject_id, action, and resource are required")
		return
	}

	// org_id is authoritative from the JWT — override any caller-supplied value.
	req.OrgID = ctxOrgID

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
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	policies, err := h.svc.ListPolicies(r.Context(), orgID)
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
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	var body struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Logic       json.RawMessage `json:"logic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Name == "" || len(body.Logic) == 0 {
		h.writeError(w, http.StatusBadRequest, "name and logic are required")
		return
	}

	// Validate logic JSON is valid
	var logicCheck map[string]interface{}
	if err := json.Unmarshal(body.Logic, &logicCheck); err != nil {
		h.writeError(w, http.StatusBadRequest, "logic must be valid JSON")
		return
	}

	policy, err := h.svc.CreatePolicy(r.Context(), orgID, body.Name, body.Description, body.Logic)
	if err != nil {
		h.logger.Error("create policy failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to create policy")
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%s-v%d"`, policy.ID, policy.Version))
	h.writeJSON(w, http.StatusCreated, policy)
}

// GetPolicy handles GET /v1/policies/{id}
func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	policyID := chi.URLParam(r, "id")
	if policyID == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	policy, err := h.svc.GetPolicy(r.Context(), orgID, policyID)
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
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	policyID := chi.URLParam(r, "id")
	var body struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Logic       json.RawMessage `json:"logic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if policyID == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	policy, err := h.svc.UpdatePolicy(r.Context(), orgID, policyID, body.Name, body.Description, body.Logic)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "policy not found")
			return
		}
		h.logger.Error("update policy failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to update policy")
		return
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%s-v%d"`, policy.ID, policy.Version))
	h.writeJSON(w, http.StatusOK, policy)
}

// DeletePolicy handles DELETE /v1/policies/{id}
func (h *Handler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	policyID := chi.URLParam(r, "id")
	if policyID == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
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

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListEvalLogs handles GET /v1/policy/eval-logs
const maxLimit = 500

func (h *Handler) ListEvalLogs(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	logs, err := h.svc.ListEvalLogs(r.Context(), orgID, limit)
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

// ListAssignments handles GET /v1/assignments
func (h *Handler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	assignments, err := h.svc.ListAssignments(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list assignments failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"assignments": assignments,
		"total":       len(assignments),
	})
}

// CreateAssignment handles POST /v1/assignments
func (h *Handler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	var body struct {
		PolicyID    string `json:"policy_id"`
		SubjectID   string `json:"subject_id"`
		SubjectType string `json:"subject_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.PolicyID == "" || body.SubjectID == "" {
		h.writeError(w, http.StatusBadRequest, "policy_id and subject_id are required")
		return
	}

	assignment, err := h.svc.CreateAssignment(r.Context(), orgID, body.PolicyID, body.SubjectID, body.SubjectType)
	if err != nil {
		h.logger.Error("create assignment failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to create assignment")
		return
	}

	h.writeJSON(w, http.StatusCreated, assignment)
}

// DeleteAssignment handles DELETE /v1/assignments/{id}
func (h *Handler) DeleteAssignment(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDFromContext(r)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "missing org context")
		return
	}

	assignmentID := chi.URLParam(r, "id")
	if assignmentID == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.svc.DeleteAssignment(r.Context(), orgID, assignmentID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "assignment not found")
			return
		}
		h.logger.Error("delete assignment failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "failed to delete assignment")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
