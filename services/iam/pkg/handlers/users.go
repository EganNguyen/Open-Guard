package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

// UserHandler handles user management HTTP endpoints.
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// List handles GET /users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		models.WriteError(w, http.StatusBadRequest, "MISSING_ORG", "Org ID is required", reqID)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	users, total, err := h.userService.ListUsers(r.Context(), orgID, page, perPage)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.PaginatedResponse{
		Data: users,
		Meta: models.NewPaginationMeta(page, perPage, total),
	})
}

// Create handles POST /users
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID := r.Header.Get("X-Org-ID")

	var req service.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", reqID)
		return
	}
	req.OrgID = orgID

	user, err := h.userService.CreateUser(r.Context(), req)
	if err != nil {
		models.WriteError(w, http.StatusBadRequest, "CREATE_USER_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// Get handles GET /users/:id
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")

	user, err := h.userService.GetUser(r.Context(), id)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "RESOURCE_NOT_FOUND",
			"User not found", reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// Update handles PATCH /users/:id
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")

	var req service.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", reqID)
		return
	}

	user, err := h.userService.UpdateUser(r.Context(), id, req)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// Delete handles DELETE /users/:id
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")
	orgID := r.Header.Get("X-Org-ID")

	if err := h.userService.DeleteUser(r.Context(), id, orgID); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error(), reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Suspend handles POST /users/:id/suspend
func (h *UserHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")

	user, err := h.userService.SuspendUser(r.Context(), id)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "SUSPEND_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// Activate handles POST /users/:id/activate
func (h *UserHandler) Activate(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")

	user, err := h.userService.ActivateUser(r.Context(), id)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "ACTIVATE_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// ListSessions handles GET /users/:id/sessions
func (h *UserHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")

	sessions, err := h.userService.ListSessions(r.Context(), id)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LIST_SESSIONS_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": sessions})
}

// RevokeSession handles DELETE /users/:id/sessions/:sid
func (h *UserHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	sid := chi.URLParam(r, "sid")

	if err := h.userService.RevokeSession(r.Context(), sid); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "REVOKE_SESSION_FAILED", err.Error(), reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListTokens handles GET /users/:id/tokens
func (h *UserHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	id := chi.URLParam(r, "id")

	tokens, err := h.userService.ListAPITokens(r.Context(), id)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LIST_TOKENS_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": tokens})
}

// RevokeToken handles DELETE /users/:id/tokens/:tid
func (h *UserHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	tid := chi.URLParam(r, "tid")

	if err := h.userService.RevokeAPIToken(r.Context(), tid); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "REVOKE_TOKEN_FAILED", err.Error(), reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
