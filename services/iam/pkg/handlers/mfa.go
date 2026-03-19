package handlers

import (
	"net/http"

	"github.com/openguard/shared/models"
)

type MFAHandler struct{}

func NewMFAHandler() *MFAHandler {
	return &MFAHandler{}
}

func (h *MFAHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "MFA enrollment is not yet implemented", r)
}

func (h *MFAHandler) Verify(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "MFA verification is not yet implemented", r)
}

func (h *MFAHandler) Challenge(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "MFA challenge is not yet implemented", r)
}
