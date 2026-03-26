// Package main is the entrypoint for the backend-api server.
package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	"myjungle/backend-api/internal/app"
	"myjungle/backend-api/internal/config"
	"myjungle/backend-api/internal/logger"
	"myjungle/backend-api/internal/storage/postgres"
)

func main() {
	// Bootstrap logger early so config.Load() benefits from structured output.
	logger.New(logger.Config{
		Level:  envOrDefault("LOG_LEVEL", "info"),
		Format: envOrDefault("LOG_FORMAT", "json"),
	})

	migrateFlag := flag.Bool("migrate", false, "run database migrations before starting the server")
	flag.Parse()

	cfg := config.Load()

	// Re-init with validated config values.
	logger.New(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})

	if *migrateFlag {
		slog.Info("running database migrations...")
		if err := postgres.Migrate(cfg.Postgres.DSN); err != nil {
			slog.Error("migration failed", slog.Any("error", err))
			os.Exit(1)
		}
	}

	application, err := app.New(cfg)
	if err != nil {
		slog.Error("app init failed", slog.Any("error", err))
		os.Exit(1)
	}
	if err := application.Run(); err != nil {
		slog.Error("server error", slog.Any("error", err))
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
