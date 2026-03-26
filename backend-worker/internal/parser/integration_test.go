//go:build integration

package parser_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/engine"
)

// ---------------------------------------------------------------------------
// Batch tests
// ---------------------------------------------------------------------------

func TestBatch_MixedTiers(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	td := testdataDir(t)
	fixtures := discoverFixtures(t, filepath.Join(td, "fixtures"))
	if len(fixtures) < 10 {
		t.Fatalf("expected at least 10 fixtures, got %d", len(fixtures))
	}

	inputs := make([]parser.FileInput, len(fixtures))
	for i, fx := range fixtures {
		inputs[i] = loadFixture(t, fx)
	}

	results, err := eng.ParseFilesBatched(
		context.Background(), "test-proj", "main", "abc123", inputs,
	)
	if err != nil {
		t.Fatalf("ParseFilesBatched: %v", err)
	}
	if len(results) != len(inputs) {
		t.Fatalf("result count = %d, want %d", len(results), len(inputs))
	}

	// Results must be in input order.
	for i, fx := range fixtures {
		if results[i].FilePath != fx.RelPath {
			t.Errorf("results[%d].FilePath = %q, want %q", i, results[i].FilePath, fx.RelPath)
		}
	}

	// Tier-appropriate assertions.
	tier1Langs := map[string]bool{
		"typescript": true, "python": true, "go": true,
		"rust": true, "java": true, "jsx": true,
	}
	tier2Langs := map[string]bool{"bash": true, "hcl": true}
	tier3Langs := map[string]bool{"json": true, "yaml": true}

	for i, r := range results {
		fx := fixtures[i]
		t.Run(fx.RelPath, func(t *testing.T) {
			if tier1Langs[fx.Language] {
				if len(r.Chunks) == 0 {
					t.Error("Tier 1: expected chunks")
				}
				if len(r.Imports) == 0 {
					t.Error("Tier 1: expected imports")
				}
			}
			if tier2Langs[fx.Language] {
				if len(r.Chunks) == 0 {
					t.Error("Tier 2: expected chunks")
				}
				if len(r.Exports) != 0 {
					t.Errorf("Tier 2: expected no exports, got %d", len(r.Exports))
				}
				if len(r.References) != 0 {
					t.Errorf("Tier 2: expected no references, got %d", len(r.References))
				}
				if len(r.JsxUsages) != 0 {
					t.Errorf("Tier 2: expected no jsx usages, got %d", len(r.JsxUsages))
				}
				if len(r.NetworkCalls) != 0 {
					t.Errorf("Tier 2: expected no network calls, got %d", len(r.NetworkCalls))
				}
			}
			if tier3Langs[fx.Language] {
				if len(r.Chunks) == 0 {
					t.Error("Tier 3: expected chunks")
				}
				if len(r.Exports) != 0 {
					t.Errorf("Tier 3: expected no exports, got %d", len(r.Exports))
				}
				if len(r.References) != 0 {
					t.Errorf("Tier 3: expected no references, got %d", len(r.References))
				}
			}
		})
	}
}

func TestBatch_SingleFile(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	td := testdataDir(t)
	content, err := os.ReadFile(filepath.Join(td, "fixtures", "typescript", "react-component.tsx"))
	if err != nil {
		t.Fatal(err)
	}

	results, err := eng.ParseFilesBatched(
		context.Background(), "test-proj", "main", "abc123",
		[]parser.FileInput{{FilePath: "typescript/react-component.tsx", Content: string(content)}},
	)
	if err != nil {
		t.Fatalf("ParseFilesBatched: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Language != "tsx" {
		t.Errorf("Language = %q, want tsx", results[0].Language)
	}
}

func TestBatch_EmptySlice(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(
		context.Background(), "test-proj", "main", "abc123", nil,
	)
	if err != nil {
		t.Fatalf("ParseFilesBatched: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestBatch_LargeBatch(t *testing.T) {
	// Use a dedicated engine with PoolSize=1 and longer timeout to avoid
	// contention when parsing 50 files through a shared pool.
	eng, err := engine.New(engine.Config{
		PoolSize:       1,
		TimeoutPerFile: 10 * time.Second,
		MaxFileSize:    1 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	td := testdataDir(t)
	content, err := os.ReadFile(filepath.Join(td, "fixtures", "typescript", "react-component.tsx"))
	if err != nil {
		t.Fatal(err)
	}

	const n = 50
	inputs := make([]parser.FileInput, n)
	for i := range inputs {
		inputs[i] = parser.FileInput{
			FilePath: fmt.Sprintf("copy_%d.tsx", i),
			Content:  string(content),
		}
	}

	results, err := eng.ParseFilesBatched(
		context.Background(), "test-proj", "main", "abc123", inputs,
	)
	if err != nil {
		t.Fatalf("ParseFilesBatched: %v", err)
	}
	if len(results) != n {
		t.Fatalf("expected %d results, got %d", n, len(results))
	}
	for i, r := range results {
		if r.FilePath != inputs[i].FilePath {
			t.Errorf("results[%d].FilePath = %q, want %q", i, r.FilePath, inputs[i].FilePath)
		}
		if len(r.Symbols) == 0 {
			t.Errorf("results[%d]: expected symbols", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge case tests
// ---------------------------------------------------------------------------

func readEdgeCase(t *testing.T, name string) string {
	t.Helper()
	td := testdataDir(t)
	data, err := os.ReadFile(filepath.Join(td, "edge-cases", name))
	if err != nil {
		t.Fatalf("ReadFile edge-case %s: %v", name, err)
	}
	return string(data)
}

func parseOne(t *testing.T, eng *engine.Engine, filePath, content string) parser.ParsedFileResult {
	t.Helper()
	results, err := eng.ParseFilesBatched(
		context.Background(), "test-proj", "main", "abc123",
		[]parser.FileInput{{FilePath: filePath, Content: content}},
	)
	if err != nil {
		t.Fatalf("ParseFilesBatched: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	return results[0]
}

func hasIssueCode(issues []parser.Issue, code string) bool {
	for _, iss := range issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}

func TestEdge_EmptyFile(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "empty.ts")
	r := parseOne(t, eng, "edge-cases/empty.ts", content)

	if r.Language != "" {
		t.Errorf("Language = %q, want empty for empty file", r.Language)
	}
	if r.FileHash != "" {
		t.Errorf("FileHash = %q, want empty", r.FileHash)
	}
	if r.LineCount != 0 {
		t.Errorf("LineCount = %d, want 0", r.LineCount)
	}
}

func TestEdge_BOMFile(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "bom-utf8.ts")
	r := parseOne(t, eng, "edge-cases/bom-utf8.ts", content)

	if r.Language != "typescript" {
		t.Errorf("Language = %q, want typescript", r.Language)
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty after BOM stripping")
	}
	// Verify hash matches normalized content.
	normalized := parser.NormalizeNewlines(content)
	expectedHash := parser.StableHash(normalized)
	if r.FileHash != expectedHash {
		t.Errorf("FileHash = %q, want %q (hash of normalized content)", r.FileHash, expectedHash)
	}
}

func TestEdge_CRLFFile(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "crlf-line-endings.ts")
	r := parseOne(t, eng, "edge-cases/crlf-line-endings.ts", content)

	if r.Language != "typescript" {
		t.Errorf("Language = %q, want typescript", r.Language)
	}
	normalized := parser.NormalizeNewlines(content)
	expectedLines := parser.CountLines(normalized)
	if r.LineCount != expectedLines {
		t.Errorf("LineCount = %d, want %d (after normalization)", r.LineCount, expectedLines)
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty")
	}
}

func TestEdge_SyntaxErrorFile(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "syntax-error.ts")
	r := parseOne(t, eng, "edge-cases/syntax-error.ts", content)

	if r.Language != "typescript" {
		t.Errorf("Language = %q, want typescript", r.Language)
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
	// Partial parse should still produce some results — the test completing is the assertion.
}

func TestEdge_VeryLongLines(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "very-long-lines.py")
	r := parseOne(t, eng, "edge-cases/very-long-lines.py", content)

	if r.Language != "python" {
		t.Errorf("Language = %q, want python", r.Language)
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
	// No panic or timeout is the main assertion.
}

func TestEdge_DeeplyNestedCode(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "deeply-nested.js")
	r := parseOne(t, eng, "edge-cases/deeply-nested.js", content)

	if r.Language != "javascript" {
		t.Errorf("Language = %q, want javascript", r.Language)
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty")
	}
	if !hasIssueCode(r.Issues, "DEEP_NESTING") {
		t.Errorf("expected DEEP_NESTING diagnostic, got issues: %+v", r.Issues)
	}
}

func TestEdge_UnicodeIdentifiers(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "unicode-identifiers.py")
	r := parseOne(t, eng, "edge-cases/unicode-identifiers.py", content)

	if r.Language != "python" {
		t.Errorf("Language = %q, want python", r.Language)
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty")
	}
	// Check that at least one symbol has non-ASCII characters.
	hasNonASCII := false
	for _, s := range r.Symbols {
		for _, c := range s.Name {
			if c > 127 {
				hasNonASCII = true
				break
			}
		}
		if hasNonASCII {
			break
		}
	}
	if !hasNonASCII {
		names := make([]string, len(r.Symbols))
		for i, s := range r.Symbols {
			names[i] = s.Name
		}
		t.Errorf("expected at least one symbol with non-ASCII name, got: %v", names)
	}
}

func TestEdge_BinaryContent(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	content := readEdgeCase(t, "binary-content.dat")
	r := parseOne(t, eng, "edge-cases/binary-content.dat", content)

	if !hasIssueCode(r.Issues, "UNSUPPORTED_LANGUAGE") {
		t.Errorf("expected UNSUPPORTED_LANGUAGE issue for .dat file, got: %+v", r.Issues)
	}
	if r.FileHash == "" {
		t.Error("FileHash should be set even for unsupported files")
	}
}

func TestEdge_UnsupportedExtension(t *testing.T) {
	eng := newTestEngine(t)
	defer eng.Close()

	r := parseOne(t, eng, "data.xyz", "some random content")

	if !hasIssueCode(r.Issues, "UNSUPPORTED_LANGUAGE") {
		t.Errorf("expected UNSUPPORTED_LANGUAGE issue, got: %+v", r.Issues)
	}
	if r.FileHash == "" {
		t.Error("FileHash should be set even for unsupported files")
	}
}

func TestEdge_OversizedFile(t *testing.T) {
	eng, err := engine.New(engine.Config{
		PoolSize:    2,
		MaxFileSize: 100, // 100 bytes
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	bigContent := strings.Repeat("x", 200)
	r := parseOne(t, eng, "big.ts", bigContent)

	if !hasIssueCode(r.Issues, "OVERSIZED_FILE") {
		t.Errorf("expected OVERSIZED_FILE issue, got: %+v", r.Issues)
	}
	if r.FileHash == "" {
		t.Error("FileHash should be set even for oversized files")
	}
	if r.SizeBytes == 0 {
		t.Error("SizeBytes should be set even for oversized files")
	}
}
