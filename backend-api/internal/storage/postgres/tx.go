package postgres

import (
	"context"
	"fmt"
	"time"

	db "myjungle/datastore/postgres/sqlc"
)

// maxDeadlockRetries is the number of times WithTx retries after a PostgreSQL
// deadlock (SQLSTATE 40P01) before giving up and returning the error.
const maxDeadlockRetries = 3

// WithTx executes fn within a database transaction. If fn returns an error,
// the transaction is rolled back; otherwise it is committed.
//
// If the transaction fails due to a PostgreSQL deadlock (SQLSTATE 40P01),
// WithTx retries the entire transaction up to maxDeadlockRetries times with
// a short linear backoff (10ms per attempt). The callback fn must be safe to
// re-execute (i.e. no side effects outside the transaction).
func (d *DB) WithTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if d == nil || d.Pool == nil || d.Queries == nil {
		return fmt.Errorf("db not initialized")
	}
	if fn == nil {
		return fmt.Errorf("nil tx callback")
	}

	var lastErr error
	for attempt := 0; attempt <= maxDeadlockRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 10 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		lastErr = d.execTx(ctx, fn)
		if lastErr == nil {
			return nil
		}

		// Only retry on deadlock; all other errors are returned immediately.
		if !IsDeadlock(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

// execTx runs fn inside a single transaction attempt.
func (d *DB) execTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck // rollback after commit is a no-op

	if err := fn(d.Queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
