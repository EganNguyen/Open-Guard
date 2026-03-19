package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/models"
	// pgx transaction logic pushed to service layer, repo only expects tx
	"github.com/jackc/pgx/v5/pgxpool"
)

type TokenHandler struct {
	tokenRepo *repository.APITokenRepository
	pool *pgxpool.Pool
}

func NewTokenHandler(tokenRepo *repository.APITokenRepository) *TokenHandler {
	return &TokenHandler{tokenRepo: tokenRepo}
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
	_ = chi.URLParam(r, "id")
	_ = r.Header.Get("X-Org-ID")

	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	if req.Name == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Token name is required", r)
		return
	}

	// NOTE: In Phase 1 token.go handled repository interaction directly. 
	// The repository requires tx now! Wait, token generation logic belongs in the service layer.
	// For API stubbing purposes during Phase 2 compile, we just defer handling.
	models.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "Tokens stub currently uncoupled from tx handling", r)
}
