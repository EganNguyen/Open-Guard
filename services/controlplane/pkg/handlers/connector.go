package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/controlplane/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/models"
)

type ConnectorHandler struct {
	svc *service.Service
}

func NewConnectorHandler(svc *service.Service) *ConnectorHandler {
	return &ConnectorHandler{svc: svc}
}

func (h *ConnectorHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing organization context", r)
		return
	}

	connectors, err := h.svc.ListConnectors(r.Context(), orgID)
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": connectors,
	})
}

func (h *ConnectorHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing organization context", r)
		return
	}
	userID := userIDFromCtx(r)

	var req struct {
		Name       string `json:"name"`
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", r)
		return
	}

	plaintextKey, err := crypto.GenerateRandomKey()
	if err != nil {
		models.HandleServiceError(w, r, err)
		return
	}

	hasher := &crypto.PBKDF2Hasher{}
	keyHash := hasher.Hash(plaintextKey)

	c := &models.Connector{
		ID:         uuid.New().String(),
		OrgID:      orgID,
		Name:       req.Name,
		WebhookURL: req.WebhookURL,
		APIKey:     keyHash,
		Status:     "active",
		CreatedBy:  userID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := h.svc.CreateConnector(r.Context(), c); err != nil {
		models.HandleServiceError(w, r, err)
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
