package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

// TokenHandler handles API token management endpoints.
type TokenHandler struct {
	tokenRepo *repository.APITokenRepository
}

// NewTokenHandler creates a new TokenHandler.
func NewTokenHandler(tokenRepo *repository.APITokenRepository) *TokenHandler {
	return &TokenHandler{tokenRepo: tokenRepo}
}

// CreateTokenRequest is the input for creating an API token.
type CreateTokenRequest struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt *string   `json:"expires_at,omitempty"`
}

// CreateTokenResponse is the output (includes raw token — shown only once).
type CreateTokenResponse struct {
	Token    string              `json:"token"`
	Metadata *repository.APIToken `json:"metadata"`
}

// Create handles POST /users/:id/tokens
func (h *TokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	userID := chi.URLParam(r, "id")
	orgID := r.Header.Get("X-Org-ID")

	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", reqID)
		return
	}

	if req.Name == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Token name is required", reqID)
		return
	}

	// Generate raw token
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate token", reqID)
		return
	}
	rawToken := hex.EncodeToString(rawBytes)
	prefix := rawToken[:8]

	// Hash for storage
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST",
				fmt.Sprintf("Invalid expires_at format: %s", err), reqID)
			return
		}
		expiresAt = &t
	}

	scopes := req.Scopes
	if scopes == nil {
		scopes = []string{}
	}

	token, err := h.tokenRepo.Create(r.Context(), userID, orgID, req.Name, tokenHash, prefix, scopes, expiresAt)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "CREATE_TOKEN_FAILED", err.Error(), reqID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateTokenResponse{
		Token:    rawToken,
		Metadata: token,
	})
}
