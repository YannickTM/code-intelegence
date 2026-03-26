package handler

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"myjungle/backend-api/internal/domain"

	"github.com/jackc/pgx/v5/pgtype"
)

// codeSearchParams holds all filter inputs for code content search.
type codeSearchParams struct {
	ProjectID       pgtype.UUID
	IndexSnapshotID pgtype.UUID
	Query           string     // required — the text/pattern to find in code_chunks.content
	SearchMode      searchMode // reused from symbolquery.go
	Language        string     // exact match on f.language
	FilePatterns    []string   // glob patterns on f.file_path (e.g. "*.go")
	IncludeDirs     []string   // glob patterns, converted to regex (reuse globToRegex)
	ExcludeDirs     []string   // glob patterns, converted to regex
	Limit           int32
	Offset          int32
}

// codeSearchResult is the row shape returned by the dynamic query.
type codeSearchResult struct {
	ID        pgtype.UUID
	FilePath  string
	Language  pgtype.Text
	StartLine int32
	EndLine   int32
	Content   string
}

// buildCodeSearchQuery constructs both a data query (with ORDER/LIMIT/OFFSET)
// and a COUNT query using the same WHERE clause.
// Returns (dataSQL, countSQL, dataArgs, countArgs, error).
func buildCodeSearchQuery(p codeSearchParams) (dataSQL, countSQL string, dataArgs, countArgs []any, err error) {
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

	// Always-present filters
	where = append(where, fmt.Sprintf("c.project_id = %s", arg(p.ProjectID)))
	where = append(where, fmt.Sprintf("c.index_snapshot_id = %s", arg(p.IndexSnapshotID)))

	// Content query filter (required)
	if p.Query == "" {
		return "", "", nil, nil, fmt.Errorf("query is required")
	}
	switch p.SearchMode {
	case searchModeInsensitive:
		likePattern := "%" + escapeLike(p.Query) + "%"
		where = append(where, fmt.Sprintf("c.content ILIKE %s ESCAPE '\\'", arg(likePattern)))
	case searchModeSensitive:
		likePattern := "%" + escapeLike(p.Query) + "%"
		where = append(where, fmt.Sprintf("c.content LIKE %s ESCAPE '\\'", arg(likePattern)))
	case searchModeRegex:
		if _, parseErr := syntax.Parse(p.Query, syntax.Perl); parseErr != nil {
			return "", "", nil, nil, fmt.Errorf("invalid regex: %w", parseErr)
		}
		where = append(where, fmt.Sprintf("c.content ~ %s", arg(p.Query)))
	}

	// Language filter (exact match)
	if p.Language != "" {
		where = append(where, fmt.Sprintf("f.language = %s", arg(p.Language)))
	}

	// File pattern filter (glob on f.file_path — OR logic, like include_dir)
	if len(p.FilePatterns) > 0 {
		var clauses []string
		for _, glob := range p.FilePatterns {
			re, convErr := globToRegex(glob)
			if convErr != nil {
				return "", "", nil, nil, fmt.Errorf("invalid file_pattern %q: %w", glob, convErr)
			}
			clauses = append(clauses, fmt.Sprintf("f.file_path ~ %s", arg(re)))
		}
		where = append(where, "("+strings.Join(clauses, " OR ")+")")
	}

	// Include directory filters (OR — match at least one pattern)
	if len(p.IncludeDirs) > 0 {
		var clauses []string
		for _, glob := range p.IncludeDirs {
			re, convErr := globToRegex(glob)
			if convErr != nil {
				return "", "", nil, nil, fmt.Errorf("invalid include_dir pattern %q: %w", glob, convErr)
			}
			clauses = append(clauses, fmt.Sprintf("f.file_path ~ %s", arg(re)))
		}
		where = append(where, "("+strings.Join(clauses, " OR ")+")")
	}

	// Exclude directory filters (AND NOT — reject all patterns)
	for _, glob := range p.ExcludeDirs {
		re, convErr := globToRegex(glob)
		if convErr != nil {
			return "", "", nil, nil, fmt.Errorf("invalid exclude_dir pattern %q: %w", glob, convErr)
		}
		where = append(where, fmt.Sprintf("f.file_path !~ %s", arg(re)))
	}

	whereClause := strings.Join(where, "\n  AND ")

	// Save count args before LIMIT/OFFSET
	countArgs = make([]any, len(args))
	copy(countArgs, args)

	// ORDER BY: file path, then line position within file
	orderBy := "ORDER BY f.file_path, c.start_line, c.id"

	// Data query
	dataSQL = fmt.Sprintf(`SELECT c.id, f.file_path, f.language,
       c.start_line, c.end_line, c.content
FROM code_chunks c
JOIN files f ON f.id = c.file_id
WHERE %s
%s
LIMIT %s OFFSET %s`, whereClause, orderBy, arg(p.Limit), arg(p.Offset))

	dataArgs = args

	// Count query (same WHERE, no ORDER/LIMIT)
	countSQL = fmt.Sprintf(`SELECT COUNT(*)::bigint AS total
FROM code_chunks c
JOIN files f ON f.id = c.file_id
WHERE %s`, whereClause)

	return dataSQL, countSQL, dataArgs, countArgs, nil
}

// escapeLike escapes LIKE/ILIKE wildcard characters so they are matched literally.
// Uses backslash as the escape character (paired with ESCAPE '\' in the SQL clause).
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// parseFilePattern splits a comma-separated file pattern param and validates each glob.
// Returns nil slice for empty input. Reuses parseDirFilter validation logic.
func parseFilePattern(raw string) ([]string, *domain.AppError) {
	return parseDirFilter(raw, "file_pattern")
}

// countMatches counts occurrences of the query pattern within content.
// This is computed Go-side for response enrichment, not in SQL.
func countMatches(content, query string, mode searchMode) int {
	switch mode {
	case searchModeInsensitive:
		return strings.Count(strings.ToLower(content), strings.ToLower(query))
	case searchModeSensitive:
		return strings.Count(content, query)
	case searchModeRegex:
		re, err := regexp.Compile(query)
		if err != nil {
			return 0
		}
		return len(re.FindAllStringIndex(content, -1))
	default:
		return 0
	}
}
