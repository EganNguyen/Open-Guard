package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/audit/pkg/service"
	sharedmodels "github.com/openguard/shared/models"
)

type EventsHandler struct {
	svc    *service.Service
	logger *slog.Logger
}

func NewEventsHandler(svc *service.Service, logger *slog.Logger) *EventsHandler {
	return &EventsHandler{
		svc:    svc,
		logger: logger,
	}
}

func (h *EventsHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		sharedmodels.WriteError(w, http.StatusBadRequest, "MISSING_ORG", "Organization ID is required", r)
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

	events, err := h.svc.FindEvents(r.Context(), filter, limit, skip)
	if err != nil {
		sharedmodels.HandleServiceError(w, r, err)
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
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		sharedmodels.WriteError(w, http.StatusBadRequest, "MISSING_ORG", "Organization ID is required", r)
		return
	}
	
	id := chi.URLParam(r, "id")
	
	events, err := h.svc.FindEvents(r.Context(), bson.M{"event_id": id, "org_id": orgID}, 1, 0)
	if err != nil {
		sharedmodels.HandleServiceError(w, r, err)
		return
	}
	if len(events) == 0 {
		sharedmodels.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Event not found", r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events[0])
}

func (h *EventsHandler) VerifyIntegrity(w http.ResponseWriter, r *http.Request) {
	orgID := orgIDFromCtx(r)
	if orgID == "" {
		sharedmodels.WriteError(w, http.StatusBadRequest, "MISSING_ORG", "Organization ID is required", r)
		return
	}

	result, err := h.svc.VerifyIntegrity(r.Context(), orgID)
	if err != nil {
		sharedmodels.HandleServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
