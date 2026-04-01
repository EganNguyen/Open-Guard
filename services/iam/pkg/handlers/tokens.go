package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/models"
	"time"
)

type TokenHandler struct {
	iamService *service.Service
	logger      *slog.Logger
}

func NewTokenHandler(iamService *service.Service, logger *slog.Logger) *TokenHandler {
	return &TokenHandler{
		iamService: iamService,
		logger:      logger,
	}
}

type CreateTokenRequest struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt *string   `json:"expires_at,omitempty"`
}

type CreateTokenResponse struct {
	Token    string              `json:"token"`
	Metadata *repository.APIToken `json:"metadata"`
}

func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	if req.Name == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Token name is required", r)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			expiresAt = &t
		}
	}

	userID := chi.URLParam(r, "id")
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		models.WriteError(w, http.StatusBadRequest, "MISSING_ORG", "Org ID is required", r)
		return
	}

	token, rawToken, err := h.iamService.CreateAPIToken(r.Context(), orgID, userID, req.Name, req.Scopes, expiresAt)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateTokenResponse{
		Token:    rawToken,
		Metadata: token,
	})
}
