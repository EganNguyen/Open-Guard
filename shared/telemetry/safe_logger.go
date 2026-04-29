package telemetry

import (
	"context"
	"log/slog"
	"strings"
)

// sensitiveFields defines keys that must be redacted from log output.
var sensitiveFields = map[string]bool{
	"password":      true,
	"token":         true,
	"secret":        true,
	"api_key":       true,
	"authorization": true,
	"cookie":        true,
	"access_token":  true,
	"refresh_token": true,
}

// SafeHandler wraps a slog.Handler to redact sensitive information.
type SafeHandler struct {
	slog.Handler
}

// NewSafeHandler creates a new SafeHandler wrapping the provided handler.
func NewSafeHandler(h slog.Handler) *SafeHandler {
	return &SafeHandler{Handler: h}
}

// Handle implements slog.Handler.
func (h *SafeHandler) Handle(ctx context.Context, r slog.Record) error {
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	
	r.Attrs(func(a slog.Attr) bool {
		newRecord.AddAttrs(h.redactAttr(a))
		return true
	})
	
	return h.Handler.Handle(ctx, newRecord)
}

func (h *SafeHandler) redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		newAttrs := make([]slog.Attr, len(attrs))
		for i, attr := range attrs {
			newAttrs[i] = h.redactAttr(attr)
		}
		return slog.Group(a.Key, anySliceToSlogAny(newAttrs)...)
	}

	key := strings.ToLower(a.Key)
	if sensitiveFields[key] {
		return slog.String(a.Key, "[REDACTED]")
	}
	
	return a
}

func anySliceToSlogAny(attrs []slog.Attr) []any {
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return args
}

// WithAttrs implements slog.Handler.
func (h *SafeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.redactAttr(a)
	}
	return &SafeHandler{Handler: h.Handler.WithAttrs(redacted)}
}

// WithGroup implements slog.Handler.
func (h *SafeHandler) WithGroup(name string) slog.Handler {
	return &SafeHandler{Handler: h.Handler.WithGroup(name)}
}
