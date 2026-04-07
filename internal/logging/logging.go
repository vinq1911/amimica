// Package logging provides a thin wrapper around Go's structured logging package
// (log/slog). It configures a slog.Logger with the appropriate level and output
// format based on configuration.
//
// Usage:
//
//	log := logging.Setup("info", "text")
//	log.Info("analysis started", "files", 42)
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup creates and returns a configured slog.Logger.
//
// The level parameter controls the minimum log severity. Accepted values are
// "debug", "info", "warn", and "error" (case-insensitive). Any unrecognized
// value defaults to "info".
//
// The format parameter controls the output encoding. "json" emits structured
// JSON lines; any other value (including "text") emits human-readable text.
// Output is always written to os.Stderr.
func Setup(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		// Unrecognized level — default to info for safety.
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
