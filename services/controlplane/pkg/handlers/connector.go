package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/controlplane/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

type ConnectorHandler struct {
	repo *repository.ConnectorRepository
	pool *pgxpool.Pool
}

func NewConnectorHandler(repo *repository.ConnectorRepository, pool *pgxpool.Pool) *ConnectorHandler {
	return &ConnectorHandler{repo: repo, pool: pool}
}

func (h *ConnectorHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := r.Context().Value(middleware.TenantIDKey).(string)
	if !ok || orgID == "" {
		http.Error(w, `{"error":{"code":"unauthorized","message":"missing org id in context"}}`, http.StatusUnauthorized)
		return
	}

	connectors, err := h.repo.List(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": connectors,
	})
}

func (h *ConnectorHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := r.Context().Value(middleware.TenantIDKey).(string)
	if !ok || orgID == "" {
		http.Error(w, `{"error":{"code":"unauthorized","message":"missing org id in context"}}`, http.StatusUnauthorized)
		return
	}
	userID := r.Header.Get("X-User-ID") // User ID can still be a header or derived from JWT if available

	var req struct {
		Name       string `json:"name"`
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	plaintextKey, err := crypto.GenerateRandomKey()
	if err != nil {
		http.Error(w, "failed to generate api key", http.StatusInternalServerError)
		return
	}

	hasher := &crypto.PBKDF2Hasher{}
	keyHash := hasher.Hash(plaintextKey)

	c := &models.Connector{
		ID:         uuid.New().String(),
		OrgID:      orgID,
		Name:       req.Name,
		WebhookURL: req.WebhookURL,
		APIKey:     keyHash, // Store the hash
		Status:     "active",
		CreatedBy:  userID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	if err := h.repo.Create(r.Context(), tx, c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// For the response, we return the plaintext key ONCE
	response := struct {
		*models.Connector
		PlaintextKey string `json:"api_key"`
	}{
		Connector:    c,
		PlaintextKey: plaintextKey,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
