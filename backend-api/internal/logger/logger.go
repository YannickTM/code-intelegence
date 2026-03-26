// Package logger configures structured logging via log/slog.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Config controls log output format and minimum level.
type Config struct {
	Level  string // "debug", "info", "warn", "error" (default: "info")
	Format string // "json" or "text" (default: "json")
}

// New creates a configured *slog.Logger and sets it as the slog default.
func New(cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
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
