// Package postgres provides database migration helpers.
package postgres

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending up-migrations against the database at dsn.
// The dsn must be a URL-style connection string (e.g. "postgres://user:pass@host/db").
// Key=value DSNs (e.g. "host=localhost dbname=mydb") are not supported by golang-migrate.
// It returns nil if migrations succeed or if there are no new migrations to apply.
func RunMigrations(dsn string) error {
	if err := validateDSN(dsn); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrations: open source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, "pgx5://"+stripScheme(dsn))
	if err != nil {
		return fmt.Errorf("migrations: init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Println("migrations: already up to date")
			return nil
		}
		return fmt.Errorf("migrations: up: %w", err)
	}

	log.Println("migrations: applied successfully")
	return nil
}

// validateDSN checks that dsn is a non-empty URL with a postgres:// or postgresql:// scheme.
// golang-migrate only accepts URL-style DSNs.
func validateDSN(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("empty DSN")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("DSN must be a URL (postgres://...), got key=value format")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("unsupported DSN scheme %q, expected postgres:// or postgresql://", u.Scheme)
	}
	return nil
}

// stripScheme removes the "postgres://" or "postgresql://" prefix from a DSN
// so the caller can prepend the "pgx5://" scheme that golang-migrate expects.
func stripScheme(dsn string) string {
	for _, prefix := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(dsn, prefix) {
			return dsn[len(prefix):]
		}
	}
	return dsn
}
