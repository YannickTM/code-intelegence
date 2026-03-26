package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// commitQueryParams holds optional filter inputs for commit listing.
type commitQueryParams struct {
	ProjectID pgtype.UUID
	Search    string    // ILIKE '%search%' on message
	FromDate  time.Time // committer_date >= from_date (zero = no filter)
	ToDate    time.Time // committer_date <= to_date (zero = no filter)
	Limit     int32
	Offset    int32
}

// buildCommitListQuery constructs parameterized data and count queries.
func buildCommitListQuery(p commitQueryParams) (dataSQL, countSQL string, dataArgs, countArgs []any) {
	var (
		where []string
		args  []any
		argN  int
	)

	arg := func(v any) string {
		args = append(args, v)
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	// Always-present filter
	where = append(where, fmt.Sprintf("project_id = %s", arg(p.ProjectID)))

	// Message search (case-insensitive)
	if p.Search != "" {
		likePattern := "%" + p.Search + "%"
		where = append(where, fmt.Sprintf("message ILIKE %s", arg(likePattern)))
	}

	// Date range
	if !p.FromDate.IsZero() {
		where = append(where, fmt.Sprintf("committer_date >= %s", arg(p.FromDate)))
	}
	if !p.ToDate.IsZero() {
		where = append(where, fmt.Sprintf("committer_date <= %s", arg(p.ToDate)))
	}

	whereClause := strings.Join(where, " AND ")

	// Save count args before LIMIT/OFFSET are appended
	countArgs = make([]any, len(args))
	copy(countArgs, args)

	// Data query
	dataSQL = fmt.Sprintf(`SELECT *
FROM commits
WHERE %s
ORDER BY committer_date DESC, id DESC
LIMIT %s OFFSET %s`, whereClause, arg(p.Limit), arg(p.Offset))

	dataArgs = args

	// Count query
	countSQL = fmt.Sprintf(`SELECT COUNT(*)::bigint AS total
FROM commits
WHERE %s`, whereClause)

	return dataSQL, countSQL, dataArgs, countArgs
}
