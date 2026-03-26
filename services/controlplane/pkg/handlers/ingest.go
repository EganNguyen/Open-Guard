package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/openguard/controlplane/pkg/repository"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/models"
)

type IngestHandler struct {
	repo *repository.ConnectorRepository
}

func NewIngestHandler(repo *repository.ConnectorRepository) *IngestHandler {
	return &IngestHandler{repo: repo}
}

type IngestRequest struct {
	Events []IngestEvent `json:"events" validate:"required,min=1,max=500,dive"`
}

type IngestEvent struct {
	ID         string          `json:"id" validate:"required,uuid4"`
	Type       string          `json:"type" validate:"required"`
	OccurredAt time.Time       `json:"occurred_at" validate:"required"`
	ActorID    string          `json:"actor_id" validate:"required"`
	ActorType  string          `json:"actor_type" validate:"required,oneof=user service system"`
	Payload    json.RawMessage `json:"payload" validate:"required"`
}

func (h *IngestHandler) IngestEvents(w http.ResponseWriter, r *http.Request) {
	orgID, ok := r.Context().Value(middleware.TenantIDKey).(string)
	if !ok || orgID == "" {
		http.Error(w, `{"error":{"code":"unauthorized","message":"missing org id in context"}}`, http.StatusUnauthorized)
		return
	}

	// Connector ID should be in context if set by APIKeyMiddleware
	connectorID, _ := r.Context().Value(middleware.ConnectorIDKey).(string)

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB limit

	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":{"code":"invalid_json","message":"`+err.Error()+`"}}`, http.StatusBadRequest)
		return
	}

	envelopes := make([]models.EventEnvelope, len(req.Events))
	for i, e := range req.Events {
		envelopes[i] = models.EventEnvelope{
			ID:          e.ID,
			Type:        e.Type,
			OrgID:       orgID,
			ActorID:     e.ActorID,
			ActorType:   e.ActorType,
			OccurredAt:  e.OccurredAt,
			Source:      "control-plane",
			EventSource: "connector:" + connectorID,
			SchemaVer:   "2.0",
			Payload:     e.Payload,
		}
	}

	if err := h.repo.IngestEvents(r.Context(), orgID, connectorID, envelopes); err != nil {
		http.Error(w, `{"error":{"code":"internal_error","message":"`+err.Error()+`"}}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted_count": len(req.Events),
	})
}
