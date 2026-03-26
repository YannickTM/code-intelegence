package health

import (
	"context"
	"time"

	"myjungle/backend-api/internal/storage/postgres"
)

// checkTimeout is the maximum time allowed for a single health check.
const checkTimeout = 2 * time.Second

// PostgresChecker checks whether the Postgres database is reachable.
type PostgresChecker struct {
	db *postgres.DB
}

// NewPostgresChecker creates a PostgresChecker. If db is nil, checks return "skipped".
func NewPostgresChecker(db *postgres.DB) *PostgresChecker {
	return &PostgresChecker{db: db}
}

func (c *PostgresChecker) Name() string { return "postgres" }

func (c *PostgresChecker) Check(ctx context.Context) CheckResult {
	if c.db == nil {
		return CheckResult{Status: StatusSkipped}
	}

	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	start := time.Now()
	err := c.db.Ping(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{Status: StatusDown, LatencyMs: latency, Error: err.Error()}
	}
	return CheckResult{Status: StatusUp, LatencyMs: latency}
}
