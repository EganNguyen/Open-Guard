package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/policy/pkg/service"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

// orgIDFromCtx reads the org ID set by the router's injectOrgContext middleware.
func orgIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(middleware.TenantIDKey).(string); ok {
		return v
	}
	return ""
}

func userIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value("user_id").(string); ok {
		return v
	}
	return ""
}

// PolicyHandler handles CRUD and evaluation of policies.
type PolicyHandler struct {
	svc    *service.Service
	logger *slog.Logger
}

func NewPolicyHandler(svc *service.Service, logger *slog.Logger) *PolicyHandler {
	return &PolicyHandler{svc: svc, logger: logger}
}

// Evaluate handles POST /policies/evaluate
// Checks whether a request should be permitted based on the active policies for an org.
func (h *PolicyHandler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var req service.EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	// Fall back to org from control plane context if not provided in body
	if req.OrgID == "" {
		req.OrgID = orgIDFromCtx(r.Context())
	}
	if req.OrgID == "" {
		writeError(w, r, http.StatusBadRequest, "MISSING_ORG_ID", "org_id is required")
		return
	}

	resp, err := h.svc.Evaluate(r.Context(), req)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
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

	if err := h.svc.Create(r.Context(), &p); err != nil {
		models.HandleServiceError(w, r, err)
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

	p, err := h.svc.Get(r.Context(), orgID, policyID)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// List handles GET /policies
func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())

	policies, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		models.HandleServiceError(w, r, err)
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

	if err := h.svc.Update(r.Context(), &p); err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// Delete handles DELETE /policies/{id}
func (h *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r.Context())
	policyID := chi.URLParam(r, "id")

	if err := h.svc.Delete(r.Context(), orgID, policyID); err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ErrNotFound is the sentinel for missing records.
var ErrNotFound = errors.New("not found")

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	models.WriteError(w, status, code, message, r)
}
