package telemetry

import (
	"log/slog"
)

// SafeAttr masks sensitive values for logging per §15.3.
// If the environment is not development, it masks the value.
func SafeAttr(key string, value string, isDev bool) slog.Attr {
	if isDev {
		return slog.String(key, value)
	}
	return slog.String(key, "***MASKED***")
}

// SensitiveAttrs returns a list of attributes where sensitive ones are masked in non-dev.
func SensitiveAttrs(isDev bool, attrs ...slog.Attr) []any {
	args := make([]any, len(attrs))
	for i, a := range attrs {
		if !isDev && isSensitiveKey(a.Key) {
			args[i] = slog.String(a.Key, "***MASKED***")
		} else {
			args[i] = a
		}
	}
	return args
}

func isSensitiveKey(key string) bool {
	switch key {
	case "password", "token", "secret", "api_key", "refresh_token":
		return true
	}
	return false
}
