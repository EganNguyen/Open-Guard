package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/openguard/services/audit/pkg/repository"
)

type SseHandler struct {
	repo *repository.AuditRepository
}

func NewSseHandler(repo *repository.AuditRepository) *SseHandler {
	return &SseHandler{repo: repo}
}

func (h *SseHandler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		http.Error(w, "org_id is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastID := ""

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// In a real implementation, we'd use a change stream or a pub/sub mechanism.
			// For now, we poll for the latest events for simplicity.
			events, err := h.repo.FindEvents(ctx, nil, 5, 0)
			if err != nil {
				continue
			}

			for _, event := range events {
				id := event["_id"].(interface{}). (fmt.Stringer).String()
				if id == lastID {
					break
				}
				
				data, _ := json.Marshal(event)
				fmt.Fprintf(w, "data: %s\n\n", data)
				lastID = id
				break // only send the latest one for the poll demo
			}
			flusher.Flush()
		}
	}
}
