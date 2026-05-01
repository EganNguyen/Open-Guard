package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/middleware"
)

type IngestHandler struct {
	publisher *kafka.Publisher
	dlpURL    string
	dlpMode   string // "block" or "audit"
}

func NewIngestHandler(publisher *kafka.Publisher, dlpURL string, dlpMode string) *IngestHandler {
	if dlpMode == "" {
		dlpMode = "audit"
	}
	return &IngestHandler{
		publisher: publisher,
		dlpURL:    dlpURL,
		dlpMode:   dlpMode,
	}
}

func (h *IngestHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var event map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Invalid event", http.StatusBadRequest)
		return
	}

	// Ensure org_id matches authenticated tenant
	event["org_id"] = orgID
	if _, ok := event["event_id"]; !ok {
		// Generate event_id if missing (though SDK should provide it)
	}

	// ── Synchronous DLP Check (TC-NEW-DLP-001) ──────────────────────────────
	if h.dlpMode == "block" && h.dlpURL != "" {
		if blocked := h.checkDLP(r.Context(), orgID, event); blocked {
			// @AI-INTENT: [Pattern: Sync-Block DLP] (TC-NEW-DLP-001)
			// Return 422 Unprocessable Entity when PII is detected in blocking mode.
			http.Error(w, "Event contains prohibited sensitive data (DLP Blocked)", http.StatusUnprocessableEntity)
			return
		}
	}

	// ── Publish to Kafka ─────────────────────────────────────────────────────
	payload, _ := json.Marshal(event)
	topic := "audit.trail"
	if t, ok := event["topic"].(string); ok && t != "" {
		topic = t
	}

	eventID, _ := event["event_id"].(string)
	if err := h.publisher.Publish(r.Context(), topic, eventID, payload); err != nil {
		// @AI-INTENT: [Pattern: Fail-Closed] If we can't persist the audit event, 
		// we must fail the request per high-assurance requirements.
		http.Error(w, "Failed to ingest event: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "event_id": eventID})
}

func (h *IngestHandler) checkDLP(ctx context.Context, orgID string, event map[string]interface{}) bool {
	b, _ := json.Marshal(event)
	
	reqBody, _ := json.Marshal(map[string]string{"content": string(b)})
	req, _ := http.NewRequestWithContext(ctx, "POST", h.dlpURL+"/v1/scan", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OpenGuard-Org-ID", orgID)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DLP service outage in block mode: %v\n", err)
		return true // Block on outage
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return true // Block on error
	}

	var findings []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&findings); err == nil {
		return len(findings) > 0
	}

	return false
}
