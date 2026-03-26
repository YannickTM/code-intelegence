package handler

import (
	"log/slog"
	"net/http"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"
)

// DashboardHandler serves cross-project dashboard endpoints.
type DashboardHandler struct {
	db *postgres.DB
}

// NewDashboardHandler creates a new DashboardHandler backed by the given database.
func NewDashboardHandler(pdb *postgres.DB) *DashboardHandler {
	return &DashboardHandler{db: pdb}
}

func (h *DashboardHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// HandleSummary returns the dashboard summary (GET /v1/dashboard/summary).
func (h *DashboardHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		WriteAppError(w, domain.ErrUnauthorized)
		return
	}

	if !h.ensureDB(w) {
		return
	}

	pgID, err := dbconv.StringToPgUUID(u.ID)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return
	}

	ctx := r.Context()

	projectsTotal, err := h.db.Queries.CountProjectsByMember(ctx, pgID)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboard: count projects failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	jobsActive, err := h.db.Queries.CountActiveJobsForUser(ctx, pgID)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboard: count active jobs failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	jobsFailed, err := h.db.Queries.CountFailedJobsForUser24h(ctx, pgID)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboard: count failed jobs failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	queryStats, err := h.db.Queries.GetQueryStats24hForUser(ctx, pgID)
	if err != nil {
		slog.ErrorContext(r.Context(), "dashboard: query stats failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"projects_total":     projectsTotal,
		"jobs_active":        jobsActive,
		"jobs_failed_24h":    jobsFailed,
		"query_count_24h":    queryStats.QueryCount,
		"p95_latency_ms_24h": queryStats.P95LatencyMs,
	})
}
