package logger

import (
	"log/slog"
	"os"

	"github.com/openguard/shared/telemetry"
)

var Log *slog.Logger

func Init() {
	Log = slog.New(telemetry.NewSafeHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(Log)
}
