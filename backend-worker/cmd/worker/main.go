// Package main is the entrypoint for the backend-worker process.
package main

import (
	"log/slog"
	"os"
	"strings"

	"myjungle/backend-worker/internal/app"
	"myjungle/backend-worker/internal/config"
	"myjungle/backend-worker/internal/logger"
)

func main() {
	// Bootstrap logger early so config.Load() benefits from structured output.
	logger.New(logger.Config{
		Level:  envOrDefault("LOG_LEVEL", "info"),
		Format: envOrDefault("LOG_FORMAT", "json"),
	})

	cfg := config.Load()

	// Re-init with validated config values.
	logger.New(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})

	application, err := app.New(cfg)
	if err != nil {
		slog.Error("app init failed", slog.Any("error", err))
		os.Exit(1)
	}
	if err := application.Run(); err != nil {
		slog.Error("worker error", slog.Any("error", err))
		os.Exit(1)
	}
}

// envOrDefault is intentionally duplicated from config.envOrDefault so the
// logger can be bootstrapped from raw env vars before config.Load() runs.
func envOrDefault(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}
