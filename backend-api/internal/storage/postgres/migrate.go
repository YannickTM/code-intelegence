package postgres

import (
	dspg "myjungle/datastore/postgres"
)

// Migrate applies all pending database migrations.
func Migrate(dsn string) error {
	return dspg.RunMigrations(dsn)
}
