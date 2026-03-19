package models

import (
	"encoding/json"
	"net/http"
)

// APIError is the standard error response envelope.
type APIError struct {
	Error APIErrorBody `json:"error"`
}

// APIErrorBody contains the error details.
type APIErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// WriteError writes a standard JSON error response.
func WriteError(w http.ResponseWriter, statusCode int, code, message, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(APIError{
		Error: APIErrorBody{
			Code:      code,
			Message:   message,
			RequestID: requestID,
		},
	})
}
