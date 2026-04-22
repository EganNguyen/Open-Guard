package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"github.com/openguard/control-plane/pkg/logger"
)

type contextKey string

const loggerKey contextKey = "logger"

// Correlation injects a correlation ID and a Zap logger into the request context.
func Correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		ctx := r.Context()
		
		// Create a request-scoped logger
		reqLogger := logger.Log.With(zap.String("correlationId", correlationID))
		ctx = context.WithValue(ctx, loggerKey, reqLogger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetLogger retrieves the request-scoped logger from context, falling back to global logger.
func GetLogger(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return l
	}
	return logger.Log
}
