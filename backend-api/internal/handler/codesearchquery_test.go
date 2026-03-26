package handler

import (
	"strings"
	"testing"
)

func baseCodeSearchParams() codeSearchParams {
	return codeSearchParams{
		ProjectID:       testPgUUID(0x01),
		IndexSnapshotID: testPgUUID(0x02),
		Query:           "handleSearch",
		SearchMode:      searchModeInsensitive,
		Limit:           20,
		Offset:          0,
	}
}

// ---------- buildCodeSearchQuery ----------

func TestBuildCodeSearchQuery_NoFilters(t *testing.T) {
	p := baseCodeSearchParams()
	dataSQL, countSQL, dataArgs, countArgs, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	// WHERE should have project_id, index_snapshot_id, and ILIKE on content
	if !strings.Contains(dataSQL, "c.project_id = $1") {
		t.Error("missing project_id filter")
	}
	if !strings.Contains(dataSQL, "c.index_snapshot_id = $2") {
		t.Error("missing index_snapshot_id filter")
	}
	if !strings.Contains(dataSQL, "c.content ILIKE $3") {
		t.Error("missing ILIKE content filter")
	}

	// Should not contain extra filters
	if strings.Contains(dataSQL, "f.language =") {
		t.Error("unexpected language filter")
	}

	// ORDER BY file path then line
	if !strings.Contains(dataSQL, "ORDER BY f.file_path, c.start_line, c.id") {
		t.Errorf("unexpected ORDER BY in data query: %s", dataSQL)
	}

	// Data args: project, snapshot, like_pattern, limit, offset
	if len(dataArgs) != 5 {
		t.Errorf("expected 5 data args, got %d", len(dataArgs))
	}
	// Count args: project, snapshot, like_pattern
	if len(countArgs) != 3 {
		t.Errorf("expected 3 count args, got %d", len(countArgs))
	}

	// Like pattern should wrap query
	if dataArgs[2] != "%handleSearch%" {
		t.Errorf("expected ILIKE pattern %%handleSearch%%, got %v", dataArgs[2])
	}

	// Count SQL should have same WHERE without ORDER/LIMIT
	if !strings.Contains(countSQL, "c.content ILIKE $3") {
		t.Error("count SQL missing content filter")
	}
	if strings.Contains(countSQL, "LIMIT") {
		t.Error("count SQL should not have LIMIT")
	}
}

func TestBuildCodeSearchQuery_Sensitive(t *testing.T) {
	p := baseCodeSearchParams()
	p.SearchMode = searchModeSensitive

	dataSQL, _, _, _, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "c.content LIKE $3") {
		t.Error("expected LIKE (case-sensitive) on content")
	}
	if strings.Contains(dataSQL, "ILIKE") {
		t.Error("should not contain ILIKE for sensitive mode")
	}
}

func TestBuildCodeSearchQuery_Regex(t *testing.T) {
	p := baseCodeSearchParams()
	p.SearchMode = searchModeRegex
	p.Query = "handle[A-Z]+"

	dataSQL, _, dataArgs, _, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "c.content ~ $3") {
		t.Error("expected regex operator ~ on content")
	}
	// Regex mode passes raw pattern, not wrapped in %
	if dataArgs[2] != "handle[A-Z]+" {
		t.Errorf("expected raw regex pattern, got %v", dataArgs[2])
	}
}

func TestBuildCodeSearchQuery_Language(t *testing.T) {
	p := baseCodeSearchParams()
	p.Language = "go"

	dataSQL, _, dataArgs, _, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "f.language = $4") {
		t.Error("missing language filter")
	}
	if dataArgs[3] != "go" {
		t.Errorf("expected language arg 'go', got %v", dataArgs[3])
	}
}

func TestBuildCodeSearchQuery_FilePattern(t *testing.T) {
	p := baseCodeSearchParams()
	p.FilePatterns = []string{"*.go"}

	dataSQL, _, _, _, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dataSQL, "f.file_path ~ $4") {
		t.Error("missing file_path regex filter")
	}
}

func TestBuildCodeSearchQuery_IncludeExcludeDir(t *testing.T) {
	p := baseCodeSearchParams()
	p.IncludeDirs = []string{"src/api"}
	p.ExcludeDirs = []string{"vendor"}

	dataSQL, _, _, _, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	// Include dir: OR clause with f.file_path ~
	if !strings.Contains(dataSQL, "f.file_path ~ $4") {
		t.Error("missing include_dir filter")
	}
	// Exclude dir: AND NOT with f.file_path !~
	if !strings.Contains(dataSQL, "f.file_path !~ $5") {
		t.Error("missing exclude_dir filter")
	}
}

func TestBuildCodeSearchQuery_AllFilters(t *testing.T) {
	p := baseCodeSearchParams()
	p.Language = "typescript"
	p.FilePatterns = []string{"*.ts"}
	p.IncludeDirs = []string{"src"}
	p.ExcludeDirs = []string{"node_modules"}

	dataSQL, countSQL, dataArgs, countArgs, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	// All filters present
	if !strings.Contains(dataSQL, "c.content ILIKE") {
		t.Error("missing content filter")
	}
	if !strings.Contains(dataSQL, "f.language =") {
		t.Error("missing language filter")
	}
	// file_path ~ for file pattern, include dir, and !~ for exclude dir
	pathCount := strings.Count(dataSQL, "f.file_path ~")
	if pathCount < 2 {
		t.Errorf("expected at least 2 file_path regex clauses, got %d", pathCount)
	}
	if !strings.Contains(dataSQL, "f.file_path !~") {
		t.Error("missing exclude_dir filter")
	}

	// Count args should not include LIMIT/OFFSET args
	if len(countArgs) != len(dataArgs)-2 {
		t.Errorf("count args (%d) should be data args (%d) minus 2", len(countArgs), len(dataArgs))
	}

	// Count SQL should have same WHERE
	if !strings.Contains(countSQL, "f.language =") {
		t.Error("count SQL missing language filter")
	}
}

func TestBuildCodeSearchQuery_EmptyQuery(t *testing.T) {
	p := baseCodeSearchParams()
	p.Query = ""

	_, _, _, _, err := buildCodeSearchQuery(p)
	if err == nil {
		t.Error("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestBuildCodeSearchQuery_InvalidRegex(t *testing.T) {
	p := baseCodeSearchParams()
	p.SearchMode = searchModeRegex
	p.Query = "[a-z++"

	_, _, _, _, err := buildCodeSearchQuery(p)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "invalid regex") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

// ---------- countMatches ----------

func TestCountMatches_Insensitive(t *testing.T) {
	content := "Hello hello HELLO world"
	count := countMatches(content, "hello", searchModeInsensitive)
	if count != 3 {
		t.Errorf("expected 3 insensitive matches, got %d", count)
	}
}

func TestCountMatches_Sensitive(t *testing.T) {
	content := "Hello hello HELLO world"
	count := countMatches(content, "hello", searchModeSensitive)
	if count != 1 {
		t.Errorf("expected 1 sensitive match, got %d", count)
	}
}

func TestCountMatches_Regex(t *testing.T) {
	content := "func foo() {}\nfunc bar() {}"
	count := countMatches(content, `func \w+\(\)`, searchModeRegex)
	if count != 2 {
		t.Errorf("expected 2 regex matches, got %d", count)
	}
}

func TestCountMatches_InvalidRegex(t *testing.T) {
	content := "some content"
	count := countMatches(content, "[invalid++", searchModeRegex)
	if count != 0 {
		t.Errorf("expected 0 for invalid regex, got %d", count)
	}
}

func TestCountMatches_NoMatch(t *testing.T) {
	content := "some content"
	count := countMatches(content, "notfound", searchModeInsensitive)
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

// ---------- parseFilePattern ----------

func TestParseFilePattern_Empty(t *testing.T) {
	result, err := parseFilePattern("")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestParseFilePattern_CommaSeparated(t *testing.T) {
	result, err := parseFilePattern("*.go, *.ts")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(result))
	}
}

// ---------- escapeLike ----------

func TestEscapeLike_Wildcards(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"100%", `100\%`},
		{"foo_bar", `foo\_bar`},
		{`a\b`, `a\\b`},
		{`%_\`, `\%\_\\`},
		{`fmt.Sprintf("%s")`, `fmt.Sprintf("\%s")`},
	}
	for _, tt := range tests {
		got := escapeLike(tt.input)
		if got != tt.want {
			t.Errorf("escapeLike(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildCodeSearchQuery_EscapesLikeWildcards(t *testing.T) {
	p := baseCodeSearchParams()
	p.Query = "100%"

	dataSQL, _, dataArgs, _, err := buildCodeSearchQuery(p)
	if err != nil {
		t.Fatal(err)
	}

	// Should use ESCAPE clause
	if !strings.Contains(dataSQL, "ESCAPE") {
		t.Error("missing ESCAPE clause in LIKE expression")
	}
	// The % in the query should be escaped in the pattern arg
	if dataArgs[2] != `%100\%%` {
		t.Errorf("expected escaped LIKE pattern, got %v", dataArgs[2])
	}
}

func TestParseFilePattern_RejectsDotDot(t *testing.T) {
	_, err := parseFilePattern("../etc/passwd")
	if err == nil {
		t.Error("expected error for '..' in pattern")
	}
}
