package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/openguard/services/audit/pkg/repository"
	"github.com/openguard/shared/middleware"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SseHandler struct {
	repo *repository.AuditReadRepository
}

func NewSseHandler(repo *repository.AuditReadRepository) *SseHandler {
	return &SseHandler{repo: repo}
}

func (h *SseHandler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	
	// MongoDB Change Stream
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "fullDocument.org_id", Value: orgID},
		}}},
	}
	
	coll := h.repo.DB.Collection("audit_events")
	
	watchOpts := options.ChangeStream().SetFullDocument(options.UpdateLookup)
	
	// Resume token support
	lastEventID := r.URL.Query().Get("lastEventId")
	if lastEventID != "" {
		// In a real implementation, we would decode the resume token from lastEventID
		// For now, we'll just log it. Real resume tokens are BSON documents.
	}

	cs, err := coll.Watch(ctx, pipeline, watchOpts)
	if err != nil {
		http.Error(w, "stream unavailable", http.StatusInternalServerError)
		return
	}
	defer cs.Close(ctx)

	for cs.Next(ctx) {
		var change struct {
			FullDocument map[string]interface{} `bson:"fullDocument"`
		}
		if err := cs.Decode(&change); err != nil {
			continue
		}
		
		// Use event_id as SSE ID (QUAL-05)
		if eventID, ok := change.FullDocument["event_id"].(string); ok && eventID != "" {
			fmt.Fprintf(w, "id: %s\n", eventID)
		}
		
		data, _ := json.Marshal(change.FullDocument)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}
