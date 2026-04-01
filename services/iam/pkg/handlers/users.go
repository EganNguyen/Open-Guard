package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/models"
)

type UserHandler struct {
	iamService *service.Service
}

func NewUserHandler(iamService *service.Service) *UserHandler {
	return &UserHandler{iamService: iamService}
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		models.WriteError(w, http.StatusBadRequest, "MISSING_ORG", "Org ID is required", r)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 { page = 1 }
	if perPage < 1 || perPage > 100 { perPage = 50 }

	users, total, err := h.iamService.ListUsers(r.Context(), orgID, page, perPage)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	
	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": users,
		"meta": map[string]int{
			"page":        page,
			"per_page":    perPage,
			"total_items": total,
			"total_pages": totalPages,
		},
	})
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")

	var req service.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}
	req.OrgID = orgID

	user, err := h.iamService.CreateUser(r.Context(), req)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := r.Header.Get("X-Org-ID")

	user, err := h.iamService.GetUser(r.Context(), orgID, id)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := r.Header.Get("X-Org-ID")

	var req service.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	user, err := h.iamService.UpdateUser(r.Context(), orgID, id, req)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := orgIDFromCtx(r)

	if err := h.iamService.DeleteUser(r.Context(), orgID, id); err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := orgIDFromCtx(r)

	user, err := h.iamService.SuspendUser(r.Context(), orgID, id)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := orgIDFromCtx(r)

	user, err := h.iamService.ActivateUser(r.Context(), orgID, id)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := orgIDFromCtx(r)

	sessions, err := h.iamService.ListSessions(r.Context(), orgID, id)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": sessions})
}

func (h *UserHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sid")
	orgID := orgIDFromCtx(r)

	if err := h.iamService.RevokeSession(r.Context(), orgID, sid); err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := orgIDFromCtx(r)

	tokens, err := h.iamService.ListAPITokens(r.Context(), orgID, id)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": tokens})
}

func (h *UserHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	tid := chi.URLParam(r, "tid")
	orgID := orgIDFromCtx(r)

	if err := h.iamService.RevokeAPIToken(r.Context(), orgID, tid); err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
