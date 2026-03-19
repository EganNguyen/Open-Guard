package models

import (
	"encoding/json"
	"net/http"
)

// APIError contains the error details.
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	TraceID   string `json:"trace_id"`
	Retryable bool   `json:"retryable"`
}

// APIErrorBody is the standard error response envelope.
type APIErrorBody struct {
	Error APIError `json:"error"`
}

// WriteError writes a standard JSON error response.
func WriteError(w http.ResponseWriter, status int, code, message string, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	reqID := r.Header.Get("X-Request-ID")
	// TODO: extract actual traceid from context instead of hardcoding empty
	traceID := ""

	json.NewEncoder(w).Encode(APIErrorBody{
		Error: APIError{
			Code:      code,
			Message:   message,
			RequestID: reqID,
			TraceID:   traceID,
			Retryable: status >= 500 || status == http.StatusTooManyRequests,
		},
	})
}
