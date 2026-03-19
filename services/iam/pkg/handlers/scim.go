package handlers

import (
	"net/http"

	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

// SCIMHandler handles SCIM v2 endpoints — stub for Phase 1.
type SCIMHandler struct{}

// NewSCIMHandler creates a new SCIMHandler.
func NewSCIMHandler() *SCIMHandler {
	return &SCIMHandler{}
}

func scimStub(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"SCIM provisioning is not yet implemented", reqID)
}

// ListUsers handles GET /scim/v2/Users — stub.
func (h *SCIMHandler) ListUsers(w http.ResponseWriter, r *http.Request)   { scimStub(w, r) }

// CreateUser handles POST /scim/v2/Users — stub.
func (h *SCIMHandler) CreateUser(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }

// GetUser handles GET /scim/v2/Users/:id — stub.
func (h *SCIMHandler) GetUser(w http.ResponseWriter, r *http.Request)     { scimStub(w, r) }

// ReplaceUser handles PUT /scim/v2/Users/:id — stub.
func (h *SCIMHandler) ReplaceUser(w http.ResponseWriter, r *http.Request) { scimStub(w, r) }

// UpdateUser handles PATCH /scim/v2/Users/:id — stub.
func (h *SCIMHandler) UpdateUser(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }

// DeleteUser handles DELETE /scim/v2/Users/:id — stub.
func (h *SCIMHandler) DeleteUser(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }

// ListGroups handles GET /scim/v2/Groups — stub.
func (h *SCIMHandler) ListGroups(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }
