package models

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

var (
	// ErrNotFound is returned when a resource is not found (§0.13).
	ErrNotFound = errors.New("not found")
	// ErrBadRequest is returned for invalid client inputs.
	ErrBadRequest = errors.New("bad request")
	// ErrUnauthorized is returned for authentication failures.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden is returned for authorization failures.
	ErrForbidden = errors.New("forbidden")
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
	// Extract trace ID from request context if available (context-aware tracing §15.2)
	traceID := ""
	if v, ok := r.Context().Value("trace_id").(string); ok {
		traceID = v
	}

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

// HandleServiceError maps service-layer errors to standard HTTP responses per §0.13.
func HandleServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil { return }

	// Common sentinel errors across services
	var status int
	var code string

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status, code = http.StatusGatewayTimeout, "TIMEOUT"
	case errors.Is(err, context.Canceled):
		status, code = 499, "CLIENT_CLOSED"
	default:
		// Attempt to detect specific error types (§0.13)
		msg := err.Error()
		switch {
		case errors.Is(err, ErrNotFound):
			status, code = http.StatusNotFound, "RESOURCE_NOT_FOUND"
		case errors.Is(err, ErrBadRequest):
			status, code = http.StatusBadRequest, "INVALID_REQUEST"
		case errors.Is(err, ErrUnauthorized):
			status, code = http.StatusUnauthorized, "UNAUTHORIZED"
		case errors.Is(err, ErrForbidden):
			status, code = http.StatusForbidden, "FORBIDDEN"
		default:
			status, code = http.StatusInternalServerError, "INTERNAL_ERROR"
		}
		WriteError(w, status, code, msg, r)
		return
	}

	WriteError(w, status, code, err.Error(), r)
}
