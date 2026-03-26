package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/redisclient"
)

// WorkerHandler serves worker status endpoints.
type WorkerHandler struct {
	redis *redisclient.Reader
}

// NewWorkerHandler creates a WorkerHandler backed by the given Redis reader.
func NewWorkerHandler(r *redisclient.Reader) *WorkerHandler {
	return &WorkerHandler{redis: r}
}

// HandleListWorkers returns all active workers read from Redis.
//
//	GET /v1/platform-management/workers
func (h *WorkerHandler) HandleListWorkers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check Redis connectivity.
	if err := h.redis.Ping(ctx); err != nil {
		slog.ErrorContext(ctx, "workers: redis ping failed", slog.Any("error", err))
		WriteError(w, http.StatusBadGateway, "worker status unavailable: redis connection failed")
		return
	}

	// Scan for all worker status keys.
	keys, err := h.redis.ScanKeys(ctx, "worker:status:*")
	if err != nil {
		slog.ErrorContext(ctx, "workers: scan keys failed", slog.Any("error", err))
		WriteError(w, http.StatusBadGateway, "worker status unavailable: failed to read worker keys")
		return
	}

	// No workers online.
	if len(keys) == 0 {
		WriteJSON(w, http.StatusOK, map[string]any{
			"items": []domain.WorkerStatus{},
			"count": 0,
		})
		return
	}

	// Batch-fetch all worker status values.
	rawValues, err := h.redis.MGetJSON(ctx, keys)
	if err != nil {
		slog.ErrorContext(ctx, "workers: mget failed", slog.Any("error", err))
		WriteError(w, http.StatusBadGateway, "worker status unavailable: failed to read worker keys")
		return
	}

	// Unmarshal each entry; skip malformed ones.
	items := make([]domain.WorkerStatus, 0, len(rawValues))
	for _, raw := range rawValues {
		var ws domain.WorkerStatus
		if err := json.Unmarshal([]byte(raw), &ws); err != nil {
			slog.WarnContext(ctx, "workers: skipping malformed entry", slog.String("raw", raw), slog.Any("error", err))
			continue
		}
		items = append(items, ws)
	}

	// Deterministic ordering by worker_id.
	sort.Slice(items, func(i, j int) bool {
		return items[i].WorkerID < items[j].WorkerID
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}
