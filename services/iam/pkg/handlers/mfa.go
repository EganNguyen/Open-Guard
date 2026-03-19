package handlers

import (
	"net/http"

	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

// MFAHandler handles MFA endpoints — stub for Phase 1.
type MFAHandler struct{}

// NewMFAHandler creates a new MFAHandler.
func NewMFAHandler() *MFAHandler {
	return &MFAHandler{}
}

// Enroll handles POST /auth/mfa/enroll — stub.
func (h *MFAHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"MFA enrollment is not yet implemented", reqID)
}

// Verify handles POST /auth/mfa/verify — stub.
func (h *MFAHandler) Verify(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"MFA verification is not yet implemented", reqID)
}

// Challenge handles POST /auth/mfa/challenge — stub.
func (h *MFAHandler) Challenge(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
		"MFA challenge is not yet implemented", reqID)
}
