package logger

import (
	"log/slog"
	"os"
	"strings"
)

var Log *slog.Logger

func init() {
	// Default: discard
	Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// Init sets up the logger based on LOG_LEVEL and LOG_FILE environment variables.
func Init() {
	level := slog.LevelInfo
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}

	var w *os.File
	if path := os.Getenv("LOG_FILE"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			w = f
		}
	}
	if w == nil {
		w = os.Stderr
	}

	Log = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
