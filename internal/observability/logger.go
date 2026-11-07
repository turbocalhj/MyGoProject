// Package observability provides logging helpers shared by the HTTP layer and
// the server entrypoint.
package observability

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger builds a JSON logger writing to stdout at the given level.
// Unknown levels fall back to info.
func NewLogger(level string) *slog.Logger {
	return NewLoggerWithWriter(level, os.Stdout)
}

// NewLoggerWithWriter is the testable variant of NewLogger.
func NewLoggerWithWriter(level string, w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)}))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
