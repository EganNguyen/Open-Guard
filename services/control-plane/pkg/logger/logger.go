package logger

import (
	"log/slog"
	"os"
)

var Log *slog.Logger

func Init() {
	Log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(Log)
}
