// Package postgres provides the database connection pool and query layer.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"myjungle/backend-worker/internal/config"
	db "myjungle/datastore/postgres/sqlc"
)

// DB wraps a pgxpool connection pool and the sqlc-generated Queries.
type DB struct {
	Pool    *pgxpool.Pool
	Queries *db.Queries
}

// DefaultConnectTimeout is the maximum time to wait for an initial connection.
const DefaultConnectTimeout = 5 * time.Second

// New creates a new DB connection pool from the given PostgresConfig.
// It parses the DSN, applies pool settings, connects, pings, and returns
// a ready-to-use DB or an error.
func New(ctx context.Context, cfg config.PostgresConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLife

	connectCtx, cancel := context.WithTimeout(ctx, DefaultConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	stat := pool.Stat()
	slog.Info("database connected",
		slog.Int("acquired", int(stat.AcquiredConns())),
		slog.Int("idle", int(stat.IdleConns())),
		slog.Int("total", int(stat.TotalConns())),
		slog.Int("max", int(stat.MaxConns())))

	return &DB{
		Pool:    pool,
		Queries: db.New(pool),
	}, nil
}

// Close drains the connection pool. It is a no-op if d or d.Pool is nil.
func (d *DB) Close() {
	if d == nil || d.Pool == nil {
		return
	}
	d.Pool.Close()
	slog.Info("database connection closed")
}

// Ping verifies the database connection is alive. It returns nil if d or d.Pool is nil.
func (d *DB) Ping(ctx context.Context) error {
	if d == nil || d.Pool == nil {
		return nil
	}
	return d.Pool.Ping(ctx)
}

// IsUniqueViolation returns true if the error is a PostgreSQL unique constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsDeadlock returns true if the error is a PostgreSQL deadlock detection
// (SQLSTATE 40P01).
func IsDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40P01"
}
