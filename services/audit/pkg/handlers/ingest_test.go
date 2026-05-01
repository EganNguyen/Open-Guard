package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openguard/shared/middleware"
	"github.com/stretchr/testify/assert"
)

func TestIngestHandler_DLPBlock(t *testing.T) {
	// 1. Mock DLP Service
	dlpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Content string `json:"content"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if strings.Contains(req.Content, "4222-2222-2222-2222") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"kind":"credit_card"}]`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer dlpServer.Close()

	// 2. Setup IngestHandler
	h := NewIngestHandler(nil, dlpServer.URL, "block")

	// 3. Test Blocked Event
	event := map[string]interface{}{
		"event_id": "e1",
		"data":     "my card is 4222-2222-2222-2222",
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/v1/events/ingest", strings.NewReader(string(body)))
	req = req.WithContext(context.WithValue(context.Background(), middleware.OrgIDKey, "org1"))

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), "DLP Blocked")
}

func TestIngestHandler_DLPOutageFailClosed(t *testing.T) {
	h := NewIngestHandler(nil, "http://localhost:12345", "block")

	event := map[string]interface{}{
		"event_id": "e1",
		"data":     "pii",
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/v1/events/ingest", strings.NewReader(string(body)))
	req = req.WithContext(context.WithValue(context.Background(), middleware.OrgIDKey, "org1"))

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}
