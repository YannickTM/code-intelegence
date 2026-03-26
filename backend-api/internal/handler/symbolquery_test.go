package handler

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// testPgUUID returns a deterministic pgtype.UUID for testing.
func testPgUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Valid = true
	for i := range u.Bytes {
		u.Bytes[i] = b
	}
	return u
}

func baseParams() symbolQueryParams {
	return symbolQueryParams{
		ProjectID:       testPgUUID(0x01),
		IndexSnapshotID: testPgUUID(0x02),
		Limit:           50,
		Offset:          0,
	}
}

// ---------- buildSymbolListQuery ----------

func TestBuildSymbolListQuery_NoFilters(t *testing.T) {
	p := baseParams()
	dataSQL, countSQL, dataArgs, countArgs, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	// WHERE should have exactly project_id and index_snapshot_id
	if !strings.Contains(dataSQL, "s.project_id = $1") {
		t.Error("missing project_id filter")
	}
	if !strings.Contains(dataSQL, "s.index_snapshot_id = $2") {
		t.Error("missing index_snapshot_id filter")
	}
	if strings.Contains(dataSQL, "ILIKE") || strings.Contains(dataSQL, "s.kind =") {
		t.Error("unexpected filter in no-filter query")
	}

	// ORDER BY should be file_path based (no name filter)
	if !strings.Contains(dataSQL, "ORDER BY f.file_path, s.start_line, s.id") {
		t.Errorf("unexpected ORDER BY in data query: %s", dataSQL)
	}

	// Data args: project, snapshot, limit, offset
	if len(dataArgs) != 4 {
		t.Errorf("expected 4 data args, got %d", len(dataArgs))
	}
	// Count args: project, snapshot only
	if len(countArgs) != 2 {
		t.Errorf("expected 2 count args, got %d", len(countArgs))
	}

	// Count query should not have ORDER/LIMIT
	if strings.Contains(countSQL, "LIMIT") || strings.Contains(countSQL, "ORDER") {
		t.Error("count query should not have LIMIT or ORDER")
	}
	if !strings.Contains(countSQL, "COUNT(*)") {
		t.Error("count query missing COUNT(*)")
	}
}

func TestBuildSymbolListQuery_NameInsensitive(t *testing.T) {
	p := baseParams()
	p.NameFilter = "Foo"
	p.SearchMode = searchModeInsensitive

	dataSQL, _, dataArgs, countArgs, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "s.name ILIKE $3") {
		t.Error("missing ILIKE clause")
	}
	if !strings.Contains(dataSQL, "LOWER(s.name) = LOWER($4)") {
		t.Error("missing sort priority")
	}
	// Data args: project, snapshot, %Foo%, Foo (sort), limit, offset
	if len(dataArgs) != 6 {
		t.Errorf("expected 6 data args, got %d", len(dataArgs))
	}
	if dataArgs[2] != "%Foo%" {
		t.Errorf("expected %%Foo%% pattern, got %v", dataArgs[2])
	}
	if dataArgs[3] != "Foo" {
		t.Errorf("expected Foo for sort priority, got %v", dataArgs[3])
	}
	// Count args: project, snapshot, %Foo% — sort priority NOT included
	if len(countArgs) != 3 {
		t.Errorf("expected 3 count args, got %d", len(countArgs))
	}
}

func TestBuildSymbolListQuery_NameSensitive(t *testing.T) {
	p := baseParams()
	p.NameFilter = "Foo"
	p.SearchMode = searchModeSensitive

	dataSQL, _, _, _, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "s.name LIKE $3") {
		t.Error("missing LIKE clause")
	}
	if !strings.Contains(dataSQL, "CASE WHEN s.name = $4") {
		t.Error("missing case-sensitive sort priority")
	}
	if strings.Contains(dataSQL, "LOWER") {
		t.Error("should not use LOWER for case-sensitive mode")
	}
}

func TestBuildSymbolListQuery_NameRegex(t *testing.T) {
	p := baseParams()
	p.NameFilter = "^Get.*"
	p.SearchMode = searchModeRegex

	dataSQL, _, dataArgs, _, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "s.name ~ $3") {
		t.Error("missing regex operator")
	}
	if strings.Contains(dataSQL, "CASE WHEN") {
		t.Error("regex mode should not have sort priority")
	}
	// ORDER BY should be s.name, s.id for regex with name filter
	if !strings.Contains(dataSQL, "ORDER BY s.name, s.id") {
		t.Errorf("unexpected ORDER BY: %s", dataSQL)
	}
	// Regex pattern not wrapped in %...%
	if dataArgs[2] != "^Get.*" {
		t.Errorf("expected raw regex, got %v", dataArgs[2])
	}
}

func TestBuildSymbolListQuery_InvalidRegex(t *testing.T) {
	p := baseParams()
	p.NameFilter = "[a-z++"
	p.SearchMode = searchModeRegex

	_, _, _, _, err := buildSymbolListQuery(p)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildSymbolListQuery_KindFilter(t *testing.T) {
	p := baseParams()
	p.KindFilter = "function"

	dataSQL, _, dataArgs, _, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "s.kind = $3") {
		t.Error("missing kind filter")
	}
	if dataArgs[2] != "function" {
		t.Errorf("expected 'function', got %v", dataArgs[2])
	}
}

func TestBuildSymbolListQuery_IncludeDir(t *testing.T) {
	p := baseParams()
	p.IncludeDirs = []string{"src/api"}

	dataSQL, _, _, _, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "f.file_path ~ $3") {
		t.Error("missing include_dir filter")
	}
}

func TestBuildSymbolListQuery_IncludeDirMultiple(t *testing.T) {
	p := baseParams()
	p.IncludeDirs = []string{"src/api", "src/lib"}

	dataSQL, _, _, _, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "(f.file_path ~ $3 OR f.file_path ~ $4)") {
		t.Errorf("missing OR group for multiple include dirs: %s", dataSQL)
	}
}

func TestBuildSymbolListQuery_ExcludeDir(t *testing.T) {
	p := baseParams()
	p.ExcludeDirs = []string{"vendor"}

	dataSQL, _, _, _, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "f.file_path !~ $3") {
		t.Error("missing exclude_dir filter")
	}
}

func TestBuildSymbolListQuery_AllFiltersCombined(t *testing.T) {
	p := baseParams()
	p.NameFilter = "Foo"
	p.SearchMode = searchModeInsensitive
	p.KindFilter = "function"
	p.IncludeDirs = []string{"src/api"}
	p.ExcludeDirs = []string{"vendor"}

	dataSQL, countSQL, dataArgs, countArgs, err := buildSymbolListQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	// Should have all filter types
	if !strings.Contains(dataSQL, "ILIKE") {
		t.Error("missing name filter")
	}
	if !strings.Contains(dataSQL, "s.kind =") {
		t.Error("missing kind filter")
	}
	if !strings.Contains(dataSQL, "f.file_path ~") {
		t.Error("missing include_dir filter")
	}
	if !strings.Contains(dataSQL, "f.file_path !~") {
		t.Error("missing exclude_dir filter")
	}

	// Data args should have more entries than count args (limit+offset)
	if len(dataArgs) <= len(countArgs) {
		t.Errorf("data args (%d) should exceed count args (%d)", len(dataArgs), len(countArgs))
	}

	// Count query should not have LIMIT/OFFSET
	if strings.Contains(countSQL, "LIMIT") {
		t.Error("count query should not have LIMIT")
	}
}

func TestBuildSymbolListQuery_IncludeDirInvalidGlob(t *testing.T) {
	p := baseParams()
	p.IncludeDirs = []string{"../escape"}

	_, _, _, _, err := buildSymbolListQuery(p)
	if err == nil {
		t.Fatal("expected error for .. in glob")
	}
}

// ---------- globToRegex ----------

func TestGlobToRegex_PlainPrefix(t *testing.T) {
	got, err := globToRegex("src/api")
	if err != nil {
		t.Fatal(err)
	}
	if got != "^src/api/" {
		t.Errorf("expected ^src/api/, got %s", got)
	}
}

func TestGlobToRegex_DoubleStarDir(t *testing.T) {
	got, err := globToRegex("**/test/**")
	if err != nil {
		t.Fatal(err)
	}
	// (^|/) at start for **, then "test/", then .* for trailing **
	if got != "(^|/)test/.*" {
		t.Errorf("expected (^|/)test/.*, got %s", got)
	}
}

func TestGlobToRegex_ExtensionMatch(t *testing.T) {
	got, err := globToRegex("*.ts")
	if err != nil {
		t.Fatal(err)
	}
	if got != `[^/]*\.ts$` {
		t.Errorf(`expected [^/]*\.ts$, got %s`, got)
	}
}

func TestGlobToRegex_ComplexPattern(t *testing.T) {
	got, err := globToRegex("src/**/*.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	if got != `^src/.*[^/]*\.test\.ts$` {
		t.Errorf(`expected ^src/.*[^/]*\.test\.ts$, got %s`, got)
	}
}

func TestGlobToRegex_QuestionMark(t *testing.T) {
	got, err := globToRegex("?")
	if err != nil {
		t.Fatal(err)
	}
	if got != ".$" {
		t.Errorf("expected .$, got %s", got)
	}
}

func TestGlobToRegex_DoubleStarAlone(t *testing.T) {
	got, err := globToRegex("**")
	if err != nil {
		t.Fatal(err)
	}
	// ** alone should match any path
	if got != "." {
		t.Errorf("expected '.', got %s", got)
	}
}

func TestGlobToRegex_DoubleStarSlash(t *testing.T) {
	got, err := globToRegex("**/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "." {
		t.Errorf("expected '.', got %s", got)
	}
}

func TestGlobToRegex_Empty(t *testing.T) {
	_, err := globToRegex("")
	if err == nil {
		t.Fatal("expected error for empty glob")
	}
}

func TestGlobToRegex_DotDot(t *testing.T) {
	_, err := globToRegex("../escape")
	if err == nil {
		t.Fatal("expected error for .. in glob")
	}
}

func TestGlobToRegex_LiteralWithSpecialChars(t *testing.T) {
	got, err := globToRegex("src/api.v2")
	if err != nil {
		t.Fatal(err)
	}
	if got != `^src/api\.v2/` {
		t.Errorf(`expected ^src/api\.v2/, got %s`, got)
	}
}

func TestGlobToRegex_NonASCII(t *testing.T) {
	// Non-ASCII directory names must not be split across bytes
	got, err := globToRegex("données/*.ts")
	if err != nil {
		t.Fatal(err)
	}
	if got != `^données/[^/]*\.ts$` {
		t.Errorf(`expected ^données/[^/]*\.ts$, got %s`, got)
	}
}

func TestGlobToRegex_TrailingSlash(t *testing.T) {
	got, err := globToRegex("src/api/")
	if err != nil {
		t.Fatal(err)
	}
	// Trailing slash is trimmed, treated as prefix
	if got != "^src/api/" {
		t.Errorf("expected ^src/api/, got %s", got)
	}
}

// ---------- parseSearchMode ----------

func TestParseSearchMode(t *testing.T) {
	tests := []struct {
		input string
		want  searchMode
		err   bool
	}{
		{"", searchModeInsensitive, false},
		{"insensitive", searchModeInsensitive, false},
		{"sensitive", searchModeSensitive, false},
		{"regex", searchModeRegex, false},
		{"unknown", 0, true},
		{"SENSITIVE", 0, true}, // case-sensitive matching
	}
	for _, tt := range tests {
		got, err := parseSearchMode(tt.input)
		if tt.err && err == nil {
			t.Errorf("parseSearchMode(%q): expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("parseSearchMode(%q): unexpected error: %v", tt.input, err)
		}
		if !tt.err && got != tt.want {
			t.Errorf("parseSearchMode(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ---------- parseDirFilter ----------

func TestParseDirFilter_Empty(t *testing.T) {
	got, err := parseDirFilter("", "include_dir")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParseDirFilter_Single(t *testing.T) {
	got, err := parseDirFilter("src/api", "include_dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "src/api" {
		t.Errorf("expected [src/api], got %v", got)
	}
}

func TestParseDirFilter_CommaSeparated(t *testing.T) {
	got, err := parseDirFilter("src/api, src/lib", "include_dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "src/api" || got[1] != "src/lib" {
		t.Errorf("expected [src/api src/lib], got %v", got)
	}
}

func TestParseDirFilter_TrailingComma(t *testing.T) {
	got, err := parseDirFilter("src/api,", "include_dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "src/api" {
		t.Errorf("expected [src/api], got %v", got)
	}
}

func TestParseDirFilter_MaxPatterns(t *testing.T) {
	patterns := make([]string, 11)
	for i := range patterns {
		patterns[i] = "dir"
	}
	_, err := parseDirFilter(strings.Join(patterns, ","), "include_dir")
	if err == nil {
		t.Fatal("expected error for > 10 patterns")
	}
}

func TestParseDirFilter_DotDot(t *testing.T) {
	_, err := parseDirFilter("../escape", "include_dir")
	if err == nil {
		t.Fatal("expected error for .. pattern")
	}
}

func TestParseDirFilter_AbsolutePath(t *testing.T) {
	_, err := parseDirFilter("/etc/passwd", "include_dir")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestParseDirFilter_TooLong(t *testing.T) {
	long := strings.Repeat("a", 501)
	_, err := parseDirFilter(long, "include_dir")
	if err == nil {
		t.Fatal("expected error for pattern > 500 chars")
	}
}
