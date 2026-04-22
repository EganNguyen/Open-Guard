package telemetry

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a new structured logger with redaction for sensitive fields.
func NewLogger(serviceName string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Redact sensitive fields
			key := strings.ToLower(a.Key)
			if key == "password" || key == "token" || key == "secret" || key == "api_key" || key == "authorization" {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(os.Stdout, opts).WithAttrs([]slog.Attr{
		slog.String("service", serviceName),
	})

	return slog.New(handler)
}

// RequestID returns the request ID from context if present.
func RequestID(ctx context.Context) string {
	// Implementation depends on how RequestID is stored in context
	// For now, returning empty
	return ""
}
