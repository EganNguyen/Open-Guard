package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const loggerKey contextKey = "logger"

// Correlation injects a correlation ID into the request context and sets up a scoped slog logger.
func Correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Create a request-scoped logger
		reqLogger := slog.Default().With("correlationId", correlationID)
		ctx := context.WithValue(r.Context(), loggerKey, reqLogger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetLogger retrieves the request-scoped logger from context, falling back to default logger.
func GetLogger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
