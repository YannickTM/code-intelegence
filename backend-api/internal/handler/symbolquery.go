package handler

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"myjungle/backend-api/internal/domain"
)

// searchMode enumerates how the name filter is matched.
type searchMode int

const (
	searchModeInsensitive searchMode = iota // ILIKE (default)
	searchModeSensitive                     // LIKE
	searchModeRegex                         // ~ (POSIX regex)
)

// symbolQueryParams holds all optional filter inputs.
type symbolQueryParams struct {
	ProjectID       pgtype.UUID
	IndexSnapshotID pgtype.UUID
	NameFilter      string
	KindFilter      string
	SearchMode      searchMode
	IncludeDirs     []string // glob patterns, converted to regex
	ExcludeDirs     []string // glob patterns, converted to regex
	Limit           int32
	Offset          int32
}

// symbolQueryResult is the row shape returned by the dynamic query.
type symbolQueryResult struct {
	ID             pgtype.UUID
	Name           string
	QualifiedName  pgtype.Text
	Kind           string
	Signature      pgtype.Text
	StartLine      pgtype.Int4
	EndLine        pgtype.Int4
	DocText        pgtype.Text
	FilePath       string
	Language       pgtype.Text
	Flags          []byte
	Modifiers      []string
	ReturnType     pgtype.Text
	ParameterTypes []string
}

// buildSymbolListQuery constructs both a data query (with ORDER/LIMIT/OFFSET)
// and a COUNT query using the same WHERE clause.
func buildSymbolListQuery(p symbolQueryParams) (dataSQL, countSQL string, dataArgs, countArgs []any, err error) {
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
	where = append(where, fmt.Sprintf("s.project_id = %s", arg(p.ProjectID)))
	where = append(where, fmt.Sprintf("s.index_snapshot_id = %s", arg(p.IndexSnapshotID)))

	// Name filter with search mode
	// sortPriorityMode tracks which sort priority to build AFTER countArgs
	// snapshot, so the sort-priority arg is not included in countArgs.
	var sortPriorityMode searchMode
	var hasSortPriority bool
	if p.NameFilter != "" {
		switch p.SearchMode {
		case searchModeInsensitive:
			likePattern := "%" + p.NameFilter + "%"
			where = append(where, fmt.Sprintf("s.name ILIKE %s", arg(likePattern)))
			sortPriorityMode = searchModeInsensitive
			hasSortPriority = true
		case searchModeSensitive:
			likePattern := "%" + p.NameFilter + "%"
			where = append(where, fmt.Sprintf("s.name LIKE %s", arg(likePattern)))
			sortPriorityMode = searchModeSensitive
			hasSortPriority = true
		case searchModeRegex:
			if _, parseErr := syntax.Parse(p.NameFilter, syntax.Perl); parseErr != nil {
				return "", "", nil, nil, fmt.Errorf("invalid regex: %w", parseErr)
			}
			where = append(where, fmt.Sprintf("s.name ~ %s", arg(p.NameFilter)))
			// No exact-match ranking for regex
		}
	}

	// Kind filter (exact match)
	if p.KindFilter != "" {
		where = append(where, fmt.Sprintf("s.kind = %s", arg(p.KindFilter)))
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

	// Save count args — only WHERE-clause params, before sort/LIMIT/OFFSET
	countArgs = make([]any, len(args))
	copy(countArgs, args)

	// Build sort priority AFTER countArgs snapshot so the extra arg is
	// not included in count query binds.
	var sortPriority string
	if hasSortPriority {
		switch sortPriorityMode {
		case searchModeInsensitive:
			sortPriority = fmt.Sprintf(
				"CASE WHEN LOWER(s.name) = LOWER(%s) THEN 0 ELSE 1 END",
				arg(p.NameFilter),
			)
		case searchModeSensitive:
			sortPriority = fmt.Sprintf(
				"CASE WHEN s.name = %s THEN 0 ELSE 1 END",
				arg(p.NameFilter),
			)
		}
	}

	// ORDER BY
	var orderBy string
	if sortPriority != "" {
		orderBy = fmt.Sprintf("ORDER BY %s, s.name, s.id", sortPriority)
	} else if p.NameFilter != "" {
		orderBy = "ORDER BY s.name, s.id"
	} else {
		orderBy = "ORDER BY f.file_path, s.start_line, s.id"
	}

	// Data query
	dataSQL = fmt.Sprintf(`SELECT s.id, s.name, s.qualified_name, s.kind, s.signature,
       s.start_line, s.end_line, s.doc_text, f.file_path, f.language,
       s.flags, s.modifiers, s.return_type, s.parameter_types
FROM symbols s
JOIN files f ON f.id = s.file_id
WHERE %s
%s
LIMIT %s OFFSET %s`, whereClause, orderBy, arg(p.Limit), arg(p.Offset))

	dataArgs = args

	// Count query (same WHERE, no ORDER/LIMIT)
	countSQL = fmt.Sprintf(`SELECT COUNT(*)::bigint AS total
FROM symbols s
JOIN files f ON f.id = s.file_id
WHERE %s`, whereClause)

	return dataSQL, countSQL, dataArgs, countArgs, nil
}

// globToRegex converts a glob pattern to a PostgreSQL POSIX regex.
//
// Supported patterns:
//   - "src/api"           → "^src/api/"          (prefix match)
//   - "**/test/**"        → "(^|/)test/"         (dir anywhere in path)
//   - "*.ts"              → "[^/]*\\.ts$"        (extension match)
//   - "src/**/*.test.ts"  → "^src/.*[^/]*\\.test\\.ts$"
//   - "?"                 → "."                  (single char)
//
// Rules:
//  1. Escape regex-special chars in literal segments
//  2. "**/" → "(^|/)" at start, ".*" elsewhere
//  3. "/**" at end → "/.*" or just implied
//  4. "*" → "[^/]*" (anything except path separator)
//  5. "?" → "." (single char)
func globToRegex(glob string) (string, error) {
	glob = strings.TrimSpace(glob)
	if glob == "" {
		return "", fmt.Errorf("empty glob pattern")
	}
	if strings.Contains(glob, "..") {
		return "", fmt.Errorf("glob must not contain '..'")
	}

	// "**" alone means match everything
	trimmed := strings.TrimSuffix(glob, "/")
	if trimmed == "**" {
		return ".", nil // matches any non-empty path
	}

	// If no glob metacharacters, treat as prefix match
	if !strings.ContainsAny(glob, "*?[{") {
		dir := strings.TrimSuffix(glob, "/")
		return "^" + regexEscapeLiteral(dir) + "/", nil
	}

	var b strings.Builder
	// Anchor start if glob begins with a literal character (not a wildcard)
	if glob[0] != '*' && glob[0] != '?' && glob[0] != '[' && glob[0] != '{' {
		b.WriteString("^")
	}
	i := 0
	for i < len(glob) {
		switch {
		case glob[i] == '*' && i+1 < len(glob) && glob[i+1] == '*':
			// "**" — match any depth
			i += 2
			if i < len(glob) && glob[i] == '/' {
				i++ // consume trailing slash
			}
			if i == 2 || (i == 3 && glob[2] == '/') {
				// Leading ** — match any prefix
				b.Reset()
				b.WriteString("(^|/)")
			} else {
				b.WriteString(".*")
			}
		case glob[i] == '*':
			b.WriteString("[^/]*")
			i++
		case glob[i] == '?':
			b.WriteString(".")
			i++
		default:
			r, size := utf8.DecodeRuneInString(glob[i:])
			b.WriteString(regexEscapeLiteral(string(r)))
			i += size
		}
	}

	// If pattern doesn't end with a wildcard, anchor to end
	result := b.String()
	if !strings.HasSuffix(result, ".*") && !strings.HasSuffix(result, "/") {
		result += "$"
	}

	// Validate the result is a valid regex
	if _, parseErr := syntax.Parse(result, syntax.Perl); parseErr != nil {
		return "", fmt.Errorf("generated invalid regex from glob %q: %w", glob, parseErr)
	}

	return result, nil
}

// regexEscapeLiteral escapes regex-special characters in a literal string.
func regexEscapeLiteral(s string) string {
	special := `\.+*?^${}()|[]`
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune(special, c) {
			b.WriteRune('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// parseSearchMode converts the query param string to searchMode.
// Empty string defaults to case-insensitive (backward compatible).
func parseSearchMode(s string) (searchMode, error) {
	switch s {
	case "", "insensitive":
		return searchModeInsensitive, nil
	case "sensitive":
		return searchModeSensitive, nil
	case "regex":
		return searchModeRegex, nil
	default:
		return 0, fmt.Errorf(
			"invalid search_mode %q: must be one of insensitive, sensitive, regex", s,
		)
	}
}

// parseDirFilter splits a comma-separated dir filter param and validates each pattern.
// Returns nil slice for empty input.
func parseDirFilter(raw, paramName string) ([]string, *domain.AppError) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 10 {
		return nil, domain.BadRequest(
			fmt.Sprintf("%s exceeds maximum of 10 patterns", paramName),
		)
	}
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "..") {
			return nil, domain.BadRequest(
				fmt.Sprintf("%s pattern must not contain '..'", paramName),
			)
		}
		if strings.HasPrefix(p, "/") {
			return nil, domain.BadRequest(
				fmt.Sprintf("%s pattern must be a relative path", paramName),
			)
		}
		if len(p) > 500 {
			return nil, domain.BadRequest(
				fmt.Sprintf("%s pattern exceeds maximum length of 500", paramName),
			)
		}
		result = append(result, p)
	}
	return result, nil
}

// isInvalidRegexError checks whether a pgx error is a PostgreSQL
// invalid regular expression syntax error (SQLSTATE 2201B).
func isInvalidRegexError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "2201B"
}
