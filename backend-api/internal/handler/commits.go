package handler

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"myjungle/backend-api/internal/auth"
	"myjungle/backend-api/internal/dbconv"
	"myjungle/backend-api/internal/domain"
	"myjungle/backend-api/internal/storage/postgres"
	"myjungle/backend-api/internal/validate"

	db "myjungle/datastore/postgres/sqlc"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// CommitSummary is a commit in a list response.
type CommitSummary struct {
	ID             string `json:"id"`
	CommitHash     string `json:"commit_hash"`
	ShortHash      string `json:"short_hash"`
	AuthorName     string `json:"author_name"`
	AuthorEmail    string `json:"author_email,omitempty"`
	AuthorDate     string `json:"author_date"`
	CommitterName  string `json:"committer_name"`
	CommitterEmail string `json:"committer_email,omitempty"`
	CommitterDate  string `json:"committer_date"`
	Message        string `json:"message"`
	MessageSubject string `json:"message_subject"`
}

// CommitListResponse is the paginated commit list.
type CommitListResponse struct {
	Items  []CommitSummary `json:"items"`
	Total  int64           `json:"total"`
	Limit  int32           `json:"limit"`
	Offset int32           `json:"offset"`
}

// CommitParent is a parent reference.
type CommitParent struct {
	ParentCommitID   string `json:"parent_commit_id"`
	ParentCommitHash string `json:"parent_commit_hash"`
	ParentShortHash  string `json:"parent_short_hash"`
	Ordinal          int32  `json:"ordinal"`
}

// CommitDiffStats is aggregate diff statistics.
type CommitDiffStats struct {
	FilesChanged   int64 `json:"files_changed"`
	TotalAdditions int64 `json:"total_additions"`
	TotalDeletions int64 `json:"total_deletions"`
}

// CommitDetail is the full commit with stats and parents.
type CommitDetail struct {
	CommitSummary
	Parents   []CommitParent  `json:"parents"`
	DiffStats CommitDiffStats `json:"diff_stats"`
}

// FileDiff is a per-file diff entry.
type FileDiff struct {
	ID             string  `json:"id"`
	OldFilePath    *string `json:"old_file_path"`
	NewFilePath    *string `json:"new_file_path"`
	ChangeType     string  `json:"change_type"`
	Patch          *string `json:"patch,omitempty"`
	Additions      int32   `json:"additions"`
	Deletions      int32   `json:"deletions"`
	ParentCommitID *string `json:"parent_commit_id"`
}

// CommitDiffsResponse is the paginated list of file diffs for a commit.
type CommitDiffsResponse struct {
	CommitHash string     `json:"commit_hash"`
	Diffs      []FileDiff `json:"diffs"`
	Total      int64      `json:"total"`
	Limit      int32      `json:"limit"`
	Offset     int32      `json:"offset"`
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// CommitHandler serves commit history and diff endpoints.
type CommitHandler struct {
	db *postgres.DB
}

// NewCommitHandler creates a new CommitHandler.
func NewCommitHandler(pdb *postgres.DB) *CommitHandler {
	return &CommitHandler{db: pdb}
}

func (h *CommitHandler) ensureDB(w http.ResponseWriter) bool {
	if h.db == nil || h.db.Queries == nil {
		WriteAppError(w, domain.ErrInternal)
		return false
	}
	return true
}

// parsePagination reads "limit" and "offset" query parameters, applying
// defaults and clamping limit to maxLimit. Returns an error suitable for
// WriteAppError when offset overflows int32.
func parsePagination(r *http.Request, defaultLimit, maxLimit int) (limit, offset int, err error) {
	limit = defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			offset = n
		}
	}
	if offset > math.MaxInt32 {
		return 0, 0, domain.BadRequest("offset exceeds maximum allowed value")
	}
	return limit, offset, nil
}

// callerShowEmails returns true when the caller's project role is admin or
// owner, which allows author/committer emails to be included in responses.
func callerShowEmails(r *http.Request) bool {
	m, ok := auth.MembershipFromContext(r.Context())
	if !ok {
		return false
	}
	return domain.RoleSufficient(m.Role, domain.RoleAdmin)
}

func (h *CommitHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	idStr := chi.URLParam(r, "projectID")
	errs := make(validate.Errors)
	validate.UUID(idStr, "project_id", errs)
	if errs.HasErrors() {
		WriteAppError(w, errs.ToAppError())
		return pgtype.UUID{}, false
	}
	id, err := dbconv.StringToPgUUID(idStr)
	if err != nil {
		WriteAppError(w, domain.ErrInternal)
		return pgtype.UUID{}, false
	}
	return id, true
}

// HandleList returns paginated commit history for a project.
// GET /v1/projects/{projectID}/commits?limit=20&offset=0&search=...&from_date=...&to_date=...
func (h *CommitHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	limit, offset, pgErr := parsePagination(r, 20, 100)
	if pgErr != nil {
		WriteAppError(w, pgErr)
		return
	}

	search := r.URL.Query().Get("search")
	fromDateStr := r.URL.Query().Get("from_date")
	toDateStr := r.URL.Query().Get("to_date")

	// Parse date filters
	var fromDate, toDate time.Time
	if fromDateStr != "" {
		var err error
		fromDate, err = time.Parse(time.RFC3339, fromDateStr)
		if err != nil {
			WriteAppError(w, domain.BadRequest("from_date must be a valid RFC3339 timestamp"))
			return
		}
	}
	if toDateStr != "" {
		var err error
		toDate, err = time.Parse(time.RFC3339, toDateStr)
		if err != nil {
			WriteAppError(w, domain.BadRequest("to_date must be a valid RFC3339 timestamp"))
			return
		}
	}

	hasFilters := search != "" || !fromDate.IsZero() || !toDate.IsZero()

	var items []CommitSummary
	var total int64
	showEmails := callerShowEmails(r)

	if hasFilters {
		// Dynamic query path
		qp := commitQueryParams{
			ProjectID: projectID,
			Search:    search,
			FromDate:  fromDate,
			ToDate:    toDate,
			Limit:     int32(limit),
			Offset:    int32(offset),
		}
		dataSQL, countSQL, dataArgs, countArgs := buildCommitListQuery(qp)

		rows, err := h.db.Pool.Query(r.Context(), dataSQL, dataArgs...)
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: search list failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var c db.Commit
			if scanErr := rows.Scan(
				&c.ID, &c.ProjectID, &c.CommitHash,
				&c.AuthorName, &c.AuthorEmail, &c.AuthorDate,
				&c.CommitterName, &c.CommitterEmail, &c.CommitterDate,
				&c.Message, &c.CreatedAt,
			); scanErr != nil {
				slog.ErrorContext(r.Context(), "commits: scan row", slog.Any("error", scanErr))
				WriteAppError(w, domain.ErrInternal)
				return
			}
			items = append(items, toCommitSummary(c, showEmails))
		}
		if err := rows.Err(); err != nil {
			slog.ErrorContext(r.Context(), "commits: rows error", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		if countErr := h.db.Pool.QueryRow(r.Context(), countSQL, countArgs...).Scan(&total); countErr != nil {
			slog.ErrorContext(r.Context(), "commits: search count failed", slog.Any("error", countErr))
			WriteAppError(w, domain.ErrInternal)
			return
		}
	} else {
		// Existing static query path (no filters)
		commits, err := h.db.Queries.ListProjectCommits(r.Context(), db.ListProjectCommitsParams{
			ProjectID: projectID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: list failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		total, err = h.db.Queries.CountProjectCommits(r.Context(), projectID)
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: count failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		items = make([]CommitSummary, len(commits))
		for i, c := range commits {
			items[i] = toCommitSummary(c, showEmails)
		}
	}

	if items == nil {
		items = []CommitSummary{}
	}

	WriteJSON(w, http.StatusOK, CommitListResponse{
		Items:  items,
		Total:  total,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
}

// HandleGet returns a single commit with diff stats and parents.
// GET /v1/projects/{projectID}/commits/{commitHash}
func (h *CommitHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}
	commitHash := chi.URLParam(r, "commitHash")

	commit, err := h.db.Queries.GetCommitByHash(r.Context(), db.GetCommitByHashParams{
		ProjectID:  projectID,
		CommitHash: commitHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteAppError(w, domain.ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "commits: get failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	stats, err := h.db.Queries.GetCommitDiffStats(r.Context(), db.GetCommitDiffStatsParams{
		ProjectID: projectID,
		CommitID:  commit.ID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "commits: get diff stats failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	parents, err := h.db.Queries.GetCommitParents(r.Context(), db.GetCommitParentsParams{
		ProjectID: projectID,
		CommitID:  commit.ID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "commits: get parents failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	detail := CommitDetail{
		CommitSummary: toCommitSummary(commit, callerShowEmails(r)),
		Parents:       toCommitParents(parents),
		DiffStats: CommitDiffStats{
			FilesChanged:   stats.FilesChanged,
			TotalAdditions: stats.TotalAdditions,
			TotalDeletions: stats.TotalDeletions,
		},
	}

	WriteJSON(w, http.StatusOK, detail)
}

// HandleDiffs returns per-file diffs for a commit.
// GET /v1/projects/{projectID}/commits/{commitHash}/diffs?include_patch=true
func (h *CommitHandler) HandleDiffs(w http.ResponseWriter, r *http.Request) {
	if !h.ensureDB(w) {
		return
	}
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}
	commitHash := chi.URLParam(r, "commitHash")
	includePatch := r.URL.Query().Get("include_patch") == "true"

	commit, err := h.db.Queries.GetCommitByHash(r.Context(), db.GetCommitByHashParams{
		ProjectID:  projectID,
		CommitHash: commitHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		WriteAppError(w, domain.ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "commits: get for diffs failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	// Single-file fetch — diff_id implies include_patch=true
	if diffID := r.URL.Query().Get("diff_id"); diffID != "" {
		errs := make(validate.Errors)
		validate.UUID(diffID, "diff_id", errs)
		if errs.HasErrors() {
			WriteAppError(w, errs.ToAppError())
			return
		}
		pgDiffID, err := dbconv.StringToPgUUID(diffID)
		if err != nil {
			WriteAppError(w, domain.ErrInternal)
			return
		}

		diff, err := h.db.Queries.GetCommitFileDiff(r.Context(), db.GetCommitFileDiffParams{
			ProjectID: projectID,
			CommitID:  commit.ID,
			ID:        pgDiffID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			WriteAppError(w, domain.ErrNotFound)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: get single diff failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		total, err := h.db.Queries.CountCommitFileDiffs(r.Context(), db.CountCommitFileDiffsParams{
			ProjectID: projectID,
			CommitID:  commit.ID,
		})
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: count diffs failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}

		WriteJSON(w, http.StatusOK, CommitDiffsResponse{
			CommitHash: commitHash,
			Diffs:      []FileDiff{toFileDiff(diff, true)},
			Total:      total,
			Limit:      1,
			Offset:     0,
		})
		return
	}

	limit, offset, pgErr := parsePagination(r, 200, 500)
	if pgErr != nil {
		WriteAppError(w, pgErr)
		return
	}

	var items []FileDiff
	if includePatch {
		diffs, err := h.db.Queries.ListCommitFileDiffs(r.Context(), db.ListCommitFileDiffsParams{
			ProjectID: projectID,
			CommitID:  commit.ID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: list diffs failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		items = make([]FileDiff, len(diffs))
		for i, d := range diffs {
			items[i] = toFileDiff(d, true)
		}
	} else {
		diffs, err := h.db.Queries.ListCommitFileDiffsMeta(r.Context(), db.ListCommitFileDiffsMetaParams{
			ProjectID: projectID,
			CommitID:  commit.ID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
		if err != nil {
			slog.ErrorContext(r.Context(), "commits: list diffs failed", slog.Any("error", err))
			WriteAppError(w, domain.ErrInternal)
			return
		}
		items = make([]FileDiff, len(diffs))
		for i, d := range diffs {
			items[i] = toFileDiffFromMeta(d)
		}
	}

	total, err := h.db.Queries.CountCommitFileDiffs(r.Context(), db.CountCommitFileDiffsParams{
		ProjectID: projectID,
		CommitID:  commit.ID,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "commits: count diffs failed", slog.Any("error", err))
		WriteAppError(w, domain.ErrInternal)
		return
	}

	WriteJSON(w, http.StatusOK, CommitDiffsResponse{
		CommitHash: commitHash,
		Diffs:      items,
		Total:      total,
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
}

// ---------------------------------------------------------------------------
// Mapping helpers
// ---------------------------------------------------------------------------

func toCommitSummary(c db.Commit, showEmails bool) CommitSummary {
	msg := c.Message
	subject := msg
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		subject = msg[:idx]
	}
	shortHash := c.CommitHash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	s := CommitSummary{
		ID:             dbconv.PgUUIDToString(c.ID),
		CommitHash:     c.CommitHash,
		ShortHash:      shortHash,
		AuthorName:     c.AuthorName,
		AuthorDate:     c.AuthorDate.Time.Format(time.RFC3339),
		CommitterName:  c.CommitterName,
		CommitterDate:  c.CommitterDate.Time.Format(time.RFC3339),
		Message:        msg,
		MessageSubject: subject,
	}
	if showEmails {
		s.AuthorEmail = c.AuthorEmail
		s.CommitterEmail = c.CommitterEmail
	}
	return s
}

func toCommitParents(parents []db.GetCommitParentsRow) []CommitParent {
	out := make([]CommitParent, len(parents))
	for i, p := range parents {
		shortHash := p.ParentHash
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}
		out[i] = CommitParent{
			ParentCommitID:   dbconv.PgUUIDToString(p.ParentCommitID),
			ParentCommitHash: p.ParentHash,
			ParentShortHash:  shortHash,
			Ordinal:          p.Ordinal,
		}
	}
	return out
}

func toFileDiff(d db.CommitFileDiff, includePatch bool) FileDiff {
	fd := FileDiff{
		ID:         dbconv.PgUUIDToString(d.ID),
		ChangeType: d.ChangeType,
		Additions:  d.Additions,
		Deletions:  d.Deletions,
	}
	if d.OldFilePath.Valid {
		fd.OldFilePath = &d.OldFilePath.String
	}
	if d.NewFilePath.Valid {
		fd.NewFilePath = &d.NewFilePath.String
	}
	if includePatch && d.Patch.Valid {
		fd.Patch = &d.Patch.String
	}
	if d.ParentCommitID.Valid {
		s := dbconv.PgUUIDToString(d.ParentCommitID)
		fd.ParentCommitID = &s
	}
	return fd
}

func toFileDiffFromMeta(d db.ListCommitFileDiffsMetaRow) FileDiff {
	fd := FileDiff{
		ID:         dbconv.PgUUIDToString(d.ID),
		ChangeType: d.ChangeType,
		Additions:  d.Additions,
		Deletions:  d.Deletions,
	}
	if d.OldFilePath.Valid {
		fd.OldFilePath = &d.OldFilePath.String
	}
	if d.NewFilePath.Valid {
		fd.NewFilePath = &d.NewFilePath.String
	}
	if d.ParentCommitID.Valid {
		s := dbconv.PgUUIDToString(d.ParentCommitID)
		fd.ParentCommitID = &s
	}
	return fd
}
