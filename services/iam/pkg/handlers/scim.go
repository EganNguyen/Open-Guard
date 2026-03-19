package handlers

import (
	"net/http"
	"github.com/openguard/shared/models"
)

type SCIMHandler struct{}

func NewSCIMHandler() *SCIMHandler {
	return &SCIMHandler{}
}

func scimStub(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "SCIM provisioning is not yet implemented", r)
}

func (h *SCIMHandler) ListUsers(w http.ResponseWriter, r *http.Request)   { scimStub(w, r) }
func (h *SCIMHandler) CreateUser(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }
func (h *SCIMHandler) GetUser(w http.ResponseWriter, r *http.Request)     { scimStub(w, r) }
func (h *SCIMHandler) ReplaceUser(w http.ResponseWriter, r *http.Request) { scimStub(w, r) }
func (h *SCIMHandler) UpdateUser(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }
func (h *SCIMHandler) DeleteUser(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }
func (h *SCIMHandler) ListGroups(w http.ResponseWriter, r *http.Request)  { scimStub(w, r) }
