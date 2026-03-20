package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/policy/pkg/service"
	"github.com/openguard/policy/pkg/tenant"
	"github.com/openguard/shared/models"
)

// orgIDFromCtx reads the org ID set by the router's injectOrgContext middleware.
func orgIDFromCtx(ctx context.Context) string {
	return tenant.OrgIDFromContext(ctx)
}

func userIDFromCtx(ctx context.Context) string {
	return tenant.UserIDFromContext(ctx)
}

// PolicyHandler handles CRUD and evaluation of policies.
type PolicyHandler struct {
	policySvc    *service.PolicyService
	evaluatorSvc *service.EvaluatorService
	logger       *slog.Logger
}

func NewPolicyHandler(policySvc *service.PolicyService, evaluatorSvc *service.EvaluatorService, logger *slog.Logger) *PolicyHandler {
	return &PolicyHandler{policySvc: policySvc, evaluatorSvc: evaluatorSvc, logger: logger}
}

// Evaluate handles POST /policies/evaluate
// Checks whether a request should be permitted based on the active policies for an org.
func (h *PolicyHandler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var req service.EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	// Fall back to org from gateway context if not provided in body
	if req.OrgID == "" {
		req.OrgID = orgIDFromCtx(r.Context())
	}
	if req.OrgID == "" {
		writeError(w, r, http.StatusBadRequest, "MISSING_ORG_ID", "org_id is required")
		return
	}

	resp, err := h.evaluatorSvc.Evaluate(r.Context(), req)
	if err != nil {
		h.logger.Error("evaluate error", "error", err)
		// Fail closed: deny on errors
		resp = &service.EvalResponse{Permitted: false, Reason: "evaluation error — fail closed"}
	}

	w.Header().Set("Content-Type", "application/json")
	if !resp.Permitted {
		w.WriteHeader(http.StatusForbidden)
	}
	json.NewEncoder(w).Encode(resp)
}

// Create handles POST /policies
func (h *PolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())
	userID := userIDFromCtx(r.Context())
	if orgID == "" {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing org context")
		return
	}

	var p models.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	p.OrgID = orgID
	p.CreatedBy = userID

	if err := h.policySvc.Create(r.Context(), &p); err != nil {
		h.logger.Error("create policy error", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create policy")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

// Get handles GET /policies/{id}
func (h *PolicyHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())
	policyID := chi.URLParam(r, "id")

	p, err := h.policySvc.Get(r.Context(), orgID, policyID)
	if errors.Is(err, errNotFound) {
		writeError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "policy not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch policy")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// List handles GET /policies
func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())

	policies, err := h.policySvc.List(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list policies error", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list policies")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": policies,
		"meta": map[string]int{"total": len(policies)},
	})
}

// Update handles PUT /policies/{id}
func (h *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())
	policyID := chi.URLParam(r, "id")

	var p models.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	p.ID = policyID
	p.OrgID = orgID

	if err := h.policySvc.Update(r.Context(), &p); err != nil {
		if errors.Is(err, errNotFound) {
			writeError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "policy not found")
			return
		}
		h.logger.Error("update policy error", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update policy")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// Delete handles DELETE /policies/{id}
func (h *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())
	policyID := chi.URLParam(r, "id")

	if err := h.policySvc.Delete(r.Context(), orgID, policyID); err != nil {
		if errors.Is(err, errNotFound) {
			writeError(w, r, http.StatusNotFound, "RESOURCE_NOT_FOUND", "policy not found")
			return
		}
		h.logger.Error("delete policy error", "error", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete policy")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// errNotFound is the sentinel for missing records.
var errNotFound = errors.New("not found")

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	models.WriteError(w, status, code, message, r)
}
