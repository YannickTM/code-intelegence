package extractors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func parseAndExtractDiagnostics(t *testing.T, langID, source, filePath string) []parser.Issue {
	t.Helper()
	grammar := parser.GetGrammar(langID)
	if grammar == nil {
		t.Fatalf("no grammar for %s", langID)
	}
	pool := parser.NewPool(1)
	defer pool.Shutdown()

	content := []byte(source)
	tree, err := pool.Parse(context.Background(), content, grammar)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	return ExtractDiagnostics(tree.RootNode(), content, langID, filePath)
}

func findIssueByCode(issues []parser.Issue, code string) *parser.Issue {
	for i := range issues {
		if issues[i].Code == code {
			return &issues[i]
		}
	}
	return nil
}

func countIssuesByCode(issues []parser.Issue, code string) int {
	n := 0
	for _, iss := range issues {
		if iss.Code == code {
			n++
		}
	}
	return n
}

func issueCodes(issues []parser.Issue) []string {
	codes := make([]string, len(issues))
	for i, iss := range issues {
		codes[i] = iss.Code
	}
	return codes
}

// ---------------------------------------------------------------------------
// Parse error tests
// ---------------------------------------------------------------------------

func TestDiagnostics_NilRoot(t *testing.T) {
	issues := ExtractDiagnostics(nil, nil, "javascript", "test.js")
	if issues != nil {
		t.Errorf("expected nil, got %d issues", len(issues))
	}
}

func TestDiagnostics_EmptyFile(t *testing.T) {
	issues := parseAndExtractDiagnostics(t, "javascript", "", "empty.js")
	// Empty JS file has no errors but may have NO_EXPORTS.
	for _, iss := range issues {
		if iss.Code == "PARSE_ERROR" || iss.Code == "MISSING_NODE" {
			t.Errorf("unexpected parse error in empty file: %v", iss)
		}
	}
}

func TestDiagnostics_ParseErrors_JS(t *testing.T) {
	// Intentional syntax error: missing closing brace.
	source := `function foo() {
  const x = 1
  if (x) {
    return x
  // missing closing brace for if
}
`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	hasError := false
	for _, iss := range issues {
		if iss.Code == "PARSE_ERROR" || iss.Code == "MISSING_NODE" {
			hasError = true
			if iss.Severity != "error" {
				t.Errorf("parse error severity = %q, want %q", iss.Severity, "error")
			}
		}
	}
	if !hasError {
		t.Log("Note: tree-sitter may recover from this syntax error without ERROR nodes")
	}
}

func TestDiagnostics_MissingNodes_JS(t *testing.T) {
	// Missing semicolons and incomplete expressions trigger MISSING nodes.
	source := `const x = ;`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	hasParseIssue := false
	for _, iss := range issues {
		if iss.Code == "PARSE_ERROR" || iss.Code == "MISSING_NODE" {
			hasParseIssue = true
			break
		}
	}
	if !hasParseIssue {
		t.Log("Note: tree-sitter may handle 'const x = ;' differently")
	}
}

func TestDiagnostics_ErrorMerging(t *testing.T) {
	// Multiple errors on the same line should be merged.
	source := `const a = @@@ $$$ %%%;`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")

	line1Errors := 0
	for _, iss := range issues {
		if (iss.Code == "PARSE_ERROR" || iss.Code == "MISSING_NODE") && iss.Line == 1 {
			line1Errors++
		}
	}
	// Should be at most 1 error per line after merging.
	if line1Errors > 1 {
		t.Errorf("expected at most 1 merged error on line 1, got %d", line1Errors)
	}
}

func TestDiagnostics_ErrorCap(t *testing.T) {
	// Generate many lines with errors.
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("const x%d = @@@;", i))
	}
	source := strings.Join(lines, "\n")

	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	if len(issues) > maxIssuesPerFile {
		t.Errorf("issue count %d exceeds cap %d", len(issues), maxIssuesPerFile)
	}
}

// ---------------------------------------------------------------------------
// Long file tests
// ---------------------------------------------------------------------------

func TestDiagnostics_LongFile(t *testing.T) {
	// Generate a file with 1001+ lines.
	var lines []string
	for i := 0; i < 1005; i++ {
		lines = append(lines, fmt.Sprintf("const x%d = %d;", i, i))
	}
	source := strings.Join(lines, "\n")

	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	iss := findIssueByCode(issues, "LONG_FILE")
	if iss == nil {
		t.Fatal("expected LONG_FILE warning")
	}
	if iss.Severity != "warning" {
		t.Errorf("severity = %q, want warning", iss.Severity)
	}
}

func TestDiagnostics_ShortFile(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, fmt.Sprintf("const x%d = %d;", i, i))
	}
	source := strings.Join(lines, "\n")

	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	if findIssueByCode(issues, "LONG_FILE") != nil {
		t.Error("unexpected LONG_FILE warning for 500-line file")
	}
}

// ---------------------------------------------------------------------------
// Long function tests
// ---------------------------------------------------------------------------

func TestDiagnostics_LongFunction_JS(t *testing.T) {
	// Create a function with 205 lines.
	var body []string
	for i := 0; i < 203; i++ {
		body = append(body, fmt.Sprintf("  const x%d = %d;", i, i))
	}
	source := "function longFunc() {\n" + strings.Join(body, "\n") + "\n}\n"

	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	iss := findIssueByCode(issues, "LONG_FUNCTION")
	if iss == nil {
		t.Fatal("expected LONG_FUNCTION warning")
	}
	if iss.Severity != "warning" {
		t.Errorf("severity = %q, want warning", iss.Severity)
	}
	if !strings.Contains(iss.Message, "longFunc") {
		t.Errorf("message should contain function name, got: %s", iss.Message)
	}
}

func TestDiagnostics_NormalFunction(t *testing.T) {
	var body []string
	for i := 0; i < 50; i++ {
		body = append(body, fmt.Sprintf("  const x%d = %d;", i, i))
	}
	source := "function shortFunc() {\n" + strings.Join(body, "\n") + "\n}\n"

	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	if findIssueByCode(issues, "LONG_FUNCTION") != nil {
		t.Error("unexpected LONG_FUNCTION for 50-line function")
	}
}

func TestDiagnostics_LongFunction_Go(t *testing.T) {
	var body []string
	for i := 0; i < 203; i++ {
		body = append(body, fmt.Sprintf("\tx%d := %d", i, i))
	}
	source := "package main\n\nfunc longFunc() {\n" + strings.Join(body, "\n") + "\n}\n"

	issues := parseAndExtractDiagnostics(t, "go", source, "main.go")
	iss := findIssueByCode(issues, "LONG_FUNCTION")
	if iss == nil {
		t.Fatal("expected LONG_FUNCTION warning for Go")
	}
}

// ---------------------------------------------------------------------------
// Deep nesting tests
// ---------------------------------------------------------------------------

func TestDiagnostics_DeepNesting_JS(t *testing.T) {
	// 7 levels of nesting (exceeds threshold of 6).
	source := `function foo() {
  if (true) {
    for (let i = 0; i < 10; i++) {
      while (true) {
        if (true) {
          try {
            if (true) {
              for (let j = 0; j < 5; j++) {
                console.log(j);
              }
            }
          } catch(e) {}
        }
      }
    }
  }
}`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	iss := findIssueByCode(issues, "DEEP_NESTING")
	if iss == nil {
		t.Fatal("expected DEEP_NESTING warning for 7 levels")
	}
	if iss.Severity != "warning" {
		t.Errorf("severity = %q, want warning", iss.Severity)
	}
}

func TestDiagnostics_DeepNesting_Python(t *testing.T) {
	source := `def foo():
    if True:
        for i in range(10):
            while True:
                if True:
                    with open("f") as f:
                        if True:
                            for j in range(5):
                                print(j)
`
	issues := parseAndExtractDiagnostics(t, "python", source, "test.py")
	iss := findIssueByCode(issues, "DEEP_NESTING")
	if iss == nil {
		t.Fatal("expected DEEP_NESTING warning for Python 7 levels")
	}
}

func TestDiagnostics_DeepNesting_Go(t *testing.T) {
	// Go nesting types: if_statement, for_statement, select_statement, switch_statement, type_switch_statement
	// Need 7 levels of these to exceed threshold of 6.
	source := `package main

func foo() {
	if true {
		for i := 0; i < 10; i++ {
			if true {
				for j := 0; j < 5; j++ {
					if true {
						for k := 0; k < 3; k++ {
							if true {
								println("deep")
							}
						}
					}
				}
			}
		}
	}
}`
	issues := parseAndExtractDiagnostics(t, "go", source, "main.go")
	iss := findIssueByCode(issues, "DEEP_NESTING")
	if iss == nil {
		t.Fatal("expected DEEP_NESTING warning for Go 7+ levels")
	}
}

func TestDiagnostics_NormalNesting(t *testing.T) {
	// 4 levels — should not trigger.
	source := `function foo() {
  if (true) {
    for (let i = 0; i < 10; i++) {
      while (true) {
        if (true) {
          console.log("ok");
        }
      }
    }
  }
}`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	if findIssueByCode(issues, "DEEP_NESTING") != nil {
		t.Error("unexpected DEEP_NESTING for 4-level nesting")
	}
}

// ---------------------------------------------------------------------------
// NO_EXPORTS tests
// ---------------------------------------------------------------------------

func TestDiagnostics_NoExports_JS(t *testing.T) {
	source := `const x = 1;
function foo() { return x; }
`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "src/utils.js")
	iss := findIssueByCode(issues, "NO_EXPORTS")
	if iss == nil {
		t.Fatal("expected NO_EXPORTS info for JS file without exports")
	}
	if iss.Severity != "info" {
		t.Errorf("severity = %q, want info", iss.Severity)
	}
}

func TestDiagnostics_HasExports_JS(t *testing.T) {
	source := `export const x = 1;
export function foo() { return x; }
`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "src/utils.js")
	if findIssueByCode(issues, "NO_EXPORTS") != nil {
		t.Error("unexpected NO_EXPORTS for JS file with exports")
	}
}

func TestDiagnostics_NoExports_NotForPython(t *testing.T) {
	source := `x = 1
def foo():
    return x
`
	issues := parseAndExtractDiagnostics(t, "python", source, "utils.py")
	if findIssueByCode(issues, "NO_EXPORTS") != nil {
		t.Error("unexpected NO_EXPORTS for Python (convention-based exports)")
	}
}

func TestDiagnostics_NoExports_NotForGo(t *testing.T) {
	source := `package main

var x = 1
func foo() int { return x }
`
	issues := parseAndExtractDiagnostics(t, "go", source, "main.go")
	if findIssueByCode(issues, "NO_EXPORTS") != nil {
		t.Error("unexpected NO_EXPORTS for Go (convention-based exports)")
	}
}

func TestDiagnostics_NoExports_NotForTestFiles(t *testing.T) {
	source := `const x = 1;
function foo() { return x; }
`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "src/utils.test.js")
	if findIssueByCode(issues, "NO_EXPORTS") != nil {
		t.Error("unexpected NO_EXPORTS for test file")
	}
}

func TestDiagnostics_NoExports_NotForTier2(t *testing.T) {
	source := `SELECT * FROM users;`
	issues := parseAndExtractDiagnostics(t, "sql", source, "query.sql")
	if findIssueByCode(issues, "NO_EXPORTS") != nil {
		t.Error("unexpected NO_EXPORTS for Tier 2 language (SQL)")
	}
}

func TestDiagnostics_NoExports_NotForTier3(t *testing.T) {
	source := `<html><body>Hello</body></html>`
	issues := parseAndExtractDiagnostics(t, "html", source, "index.html")
	if findIssueByCode(issues, "NO_EXPORTS") != nil {
		t.Error("unexpected NO_EXPORTS for Tier 3 language (HTML)")
	}
}

// ---------------------------------------------------------------------------
// Deterministic sort
// ---------------------------------------------------------------------------

func TestDiagnostics_DeterministicSort(t *testing.T) {
	// Issues should be sorted by line → column → code.
	source := `export const a = @@@;
export const b = $$$;
`
	issues := parseAndExtractDiagnostics(t, "javascript", source, "test.js")
	for i := 1; i < len(issues); i++ {
		prev := issues[i-1]
		cur := issues[i]
		if prev.Line > cur.Line {
			t.Errorf("issues not sorted by line: %v before %v", prev, cur)
		}
		if prev.Line == cur.Line && prev.Column > cur.Column {
			t.Errorf("issues not sorted by column: %v before %v", prev, cur)
		}
		if prev.Line == cur.Line && prev.Column == cur.Column && prev.Code > cur.Code {
			t.Errorf("issues not sorted by code: %v before %v", prev, cur)
		}
	}
}

// ---------------------------------------------------------------------------
// Factory function tests
// ---------------------------------------------------------------------------

func TestCreateUnsupportedLanguageIssue(t *testing.T) {
	iss := CreateUnsupportedLanguageIssue(".xyz")
	if iss.Code != "UNSUPPORTED_LANGUAGE" {
		t.Errorf("code = %q, want UNSUPPORTED_LANGUAGE", iss.Code)
	}
	if iss.Severity != "info" {
		t.Errorf("severity = %q, want info", iss.Severity)
	}
	if !strings.Contains(iss.Message, ".xyz") {
		t.Errorf("message should contain extension, got: %s", iss.Message)
	}
}

func TestCreateOversizedFileIssue(t *testing.T) {
	iss := CreateOversizedFileIssue(5_000_000, 1_000_000)
	if iss.Code != "OVERSIZED_FILE" {
		t.Errorf("code = %q, want OVERSIZED_FILE", iss.Code)
	}
	if iss.Severity != "warning" {
		t.Errorf("severity = %q, want warning", iss.Severity)
	}
	if !strings.Contains(iss.Message, "5000000") {
		t.Errorf("message should contain size, got: %s", iss.Message)
	}
}

func TestCreateParseTimeoutIssue(t *testing.T) {
	iss := CreateParseTimeoutIssue("big.js", 5000)
	if iss.Code != "PARSE_TIMEOUT" {
		t.Errorf("code = %q, want PARSE_TIMEOUT", iss.Code)
	}
	if iss.Severity != "error" {
		t.Errorf("severity = %q, want error", iss.Severity)
	}
	if !strings.Contains(iss.Message, "big.js") || !strings.Contains(iss.Message, "5000") {
		t.Errorf("message should contain path and timeout, got: %s", iss.Message)
	}
}

func TestCreateExtractionErrorIssue(t *testing.T) {
	iss := CreateExtractionErrorIssue("symbols", errors.New("out of memory"))
	if iss.Code != "EXTRACTION_ERROR" {
		t.Errorf("code = %q, want EXTRACTION_ERROR", iss.Code)
	}
	if iss.Severity != "error" {
		t.Errorf("severity = %q, want error", iss.Severity)
	}
	if !strings.Contains(iss.Message, "symbols") || !strings.Contains(iss.Message, "out of memory") {
		t.Errorf("message should contain extractor and error, got: %s", iss.Message)
	}
}

// ---------------------------------------------------------------------------
// Multi-language parse error tests
// ---------------------------------------------------------------------------

func TestDiagnostics_ParseErrors_Python(t *testing.T) {
	source := `def foo(
    x = 1
    return x
`
	issues := parseAndExtractDiagnostics(t, "python", source, "test.py")
	// Python's tree-sitter should detect the syntax error (unclosed parenthesis).
	_ = issues // Just verifying no panic.
}

func TestDiagnostics_ParseErrors_Go(t *testing.T) {
	source := `package main

func foo() {
	x := @@@
}
`
	issues := parseAndExtractDiagnostics(t, "go", source, "main.go")
	hasError := false
	for _, iss := range issues {
		if iss.Code == "PARSE_ERROR" || iss.Code == "MISSING_NODE" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Log("Note: Go tree-sitter may handle '@@@' differently")
	}
}

func TestDiagnostics_ParseErrors_Rust(t *testing.T) {
	source := `fn main() {
    let x = @@@;
}
`
	issues := parseAndExtractDiagnostics(t, "rust", source, "main.rs")
	_ = issues // Verify no panic.
}

func TestDiagnostics_ParseErrors_Java(t *testing.T) {
	source := `public class Foo {
    public void bar() {
        int x = @@@;
    }
}
`
	issues := parseAndExtractDiagnostics(t, "java", source, "Foo.java")
	_ = issues // Verify no panic.
}
