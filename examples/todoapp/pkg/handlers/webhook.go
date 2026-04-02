package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

type WebhookHandler struct {
	secret string
	audit  AuditClient
}

func NewWebhookHandler(secret string, audit AuditClient) *WebhookHandler {
	return &WebhookHandler{secret: secret, audit: audit}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Extract headers for verification
	sig := r.Header.Get("X-OpenGuard-Signature")   // "sha256=<hex>"
	tsStr := r.Header.Get("X-OpenGuard-Timestamp") // unix seconds
	deliveryID := r.Header.Get("X-OpenGuard-Delivery")

	if sig == "" || tsStr == "" || deliveryID == "" {
		http.Error(w, "missing_headers", http.StatusUnauthorized)
		return
	}

	// 2. Replay protection: check timestamp (|now - ts| < 300s)
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid_timestamp", http.StatusUnauthorized)
		return
	}

	now := time.Now().Unix()
	if math.Abs(float64(now-ts)) > 300 {
		http.Error(w, "stale_request", http.StatusUnauthorized)
		return
	}

	// 3. Verify HMAC-SHA256 signature
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed_to_read_body", http.StatusInternalServerError)
		return
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write([]byte(tsStr + "."))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		http.Error(w, "invalid_signature", http.StatusUnauthorized)
		return
	}

	// 4. Handle events
	var event struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid_json", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "saga.completed":
		// Activate new user, etc.
	case "threat.alert.created":
		// Handle high/critical threats
	default:
		// Ignore unknown events but return 200 OK as per spec
	}

	w.WriteHeader(http.StatusOK)
}
