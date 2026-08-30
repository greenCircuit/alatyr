// Package logging owns the process-wide slog.Logger and Echo middleware.
// Every long-lived component (api.Server, store.Builder, policy engines,
// mesh sources) accepts *slog.Logger via constructor. Per-request handlers
// pull a child logger from echo.Context via From().
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New builds a JSON-handler logger at the given level, writing to stdout.
// Level strings map: debug, info, warn, error. Anything else falls back to info.
func New(level string) *slog.Logger {
	return NewWithWriter(os.Stdout, level)
}

// NewWithWriter is New with an injected writer — used in tests and for
// discard sinks (io.Discard).
func NewWithWriter(writer io.Writer, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: parseLevel(level),
	}))
}

// LevelFromEnv reads LOG_LEVEL; defaults to info.
func LevelFromEnv() string {
	return os.Getenv("LOG_LEVEL")
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
