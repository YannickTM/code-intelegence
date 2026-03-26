// Package redisclient provides a general-purpose Redis client for key reads.
// It is intentionally separate from the SSE subscriber and queue publisher,
// which serve specific concerns (pub/sub and job enqueuing).
package redisclient

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Reader is a thin wrapper around a Redis client for key reads (SCAN, MGET).
type Reader struct {
	rdb *redis.Client
}

// NewReader creates a Reader backed by a Redis connection pool.
// poolSize should be small (2–4) since this is used for infrequent admin reads.
func NewReader(redisURL string, poolSize int) (*Reader, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redisclient: parse redis url: %w", err)
	}
	opts.PoolSize = poolSize
	return &Reader{rdb: redis.NewClient(opts)}, nil
}

// ScanKeys returns all keys matching pattern using SCAN (non-blocking iteration).
// Returns nil if no keys match.
func (r *Reader) ScanKeys(ctx context.Context, pattern string) ([]string, error) {
	seen := make(map[string]struct{})
	var cursor uint64
	for {
		keys, next, err := r.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			seen[k] = struct{}{}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	return result, nil
}

// MGetJSON fetches multiple keys via MGET and returns raw JSON strings.
// Missing or expired keys are silently skipped.
func (r *Reader) MGetJSON(ctx context.Context, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	vals, err := r.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result, nil
}

// GetJSON fetches a single key and returns its raw string value.
// Returns ("", nil) if the key does not exist or has expired.
func (r *Reader) GetJSON(ctx context.Context, key string) (string, error) {
	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// Ping checks Redis connectivity.
func (r *Reader) Ping(ctx context.Context) error {
	return r.rdb.Ping(ctx).Err()
}

// Close closes the underlying Redis client. Nil-safe.
func (r *Reader) Close() error {
	if r == nil || r.rdb == nil {
		return nil
	}
	return r.rdb.Close()
}
