package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/audit/pkg/integrity"
	"github.com/openguard/audit/pkg/models"
)

type AuditReader interface {
	FindEvents(ctx context.Context, filter bson.M, limit int64, skip int64) ([]models.AuditEvent, error)
	GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error)
}

type EventsHandler struct {
	repo     AuditReader
	verifier *integrity.Verifier
	logger   *slog.Logger
}

func NewEventsHandler(repo AuditReader, verifier *integrity.Verifier, logger *slog.Logger) *EventsHandler {
	return &EventsHandler{
		repo:     repo,
		verifier: verifier,
		logger:   logger,
	}
}

func (h *EventsHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		http.Error(w, `{"error":"X-Org-ID header required"}`, http.StatusBadRequest)
		return
	}

	filter := bson.M{"org_id": orgID}
	
	if actorID := r.URL.Query().Get("actor_id"); actorID != "" {
		filter["actor_id"] = actorID
	}
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		filter["type"] = eventType
	}

	limit := int64(50)
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil {
			limit = parsed
		}
	}

	skip := int64(0)
	if s := r.URL.Query().Get("skip"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			skip = parsed
		}
	}

	events, err := h.repo.FindEvents(r.Context(), filter, limit, skip)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to fetch events", "error", err)
		http.Error(w, `{"error":"failed to fetch events"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if events == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(events)
}

func (h *EventsHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		http.Error(w, `{"error":"X-Org-ID header required"}`, http.StatusBadRequest)
		return
	}
	
	id := chi.URLParam(r, "id")
	
	events, err := h.repo.FindEvents(r.Context(), bson.M{"event_id": id, "org_id": orgID}, 1, 0)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to fetch event", "error", err, "event_id", id)
		http.Error(w, `{"error":"failed to fetch event"}`, http.StatusInternalServerError)
		return
	}
	if len(events) == 0 {
		http.Error(w, `{"error":"event not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events[0])
}

func (h *EventsHandler) VerifyIntegrity(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		http.Error(w, `{"error":"X-Org-ID header required"}`, http.StatusBadRequest)
		return
	}

	result, err := h.verifier.VerifyChain(r.Context(), orgID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "verification failed", "error", err, "org_id", orgID)
		http.Error(w, `{"error":"verification failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

