//go:build integration

package parser_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func symbolIDSet(symbols []parser.Symbol) map[string]struct{} {
	m := make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		if s.SymbolID != "" {
			m[s.SymbolID] = struct{}{}
		}
	}
	return m
}

func extractorStatusMap(statuses []parser.ExtractorStatus) map[string]string {
	m := make(map[string]string, len(statuses))
	for _, s := range statuses {
		m[s.ExtractorName] = s.Status
	}
	return m
}

// parseFixtureFile loads and parses a fixture file through the engine.
func parseFixtureFile(t *testing.T, fixtureRelPath string) parser.ParsedFileResult {
	t.Helper()
	eng := newTestEngine(t)
	defer eng.Close()

	td := testdataDir(t)
	absPath := filepath.Join(td, "fixtures", fixtureRelPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", absPath, err)
	}

	results, err := eng.ParseFilesBatched(
		context.Background(), "test-proj", "main", "abc123",
		[]parser.FileInput{{FilePath: fixtureRelPath, Content: string(data)}},
	)
	if err != nil {
		t.Fatalf("ParseFilesBatched: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	return results[0]
}

// ---------------------------------------------------------------------------
// Tier 1 consistency: TypeScript/React (tsx)
// ---------------------------------------------------------------------------

func TestConsistency_ReactComponent(t *testing.T) {
	r := parseFixtureFile(t, "typescript/react-component.tsx")

	symIDs := symbolIDSet(r.Symbols)

	t.Run("ExportSymbolIDs", func(t *testing.T) {
		for _, exp := range r.Exports {
			if exp.SymbolID != "" {
				if _, ok := symIDs[exp.SymbolID]; !ok {
					t.Errorf("Export %q references unknown SymbolID %q", exp.ExportedName, exp.SymbolID)
				}
			}
		}
	})

	t.Run("ChunkBoundaries", func(t *testing.T) {
		if len(r.Chunks) == 0 {
			t.Fatal("no chunks")
		}
		// Sort by StartLine for overlap check.
		sorted := make([]parser.Chunk, len(r.Chunks))
		copy(sorted, r.Chunks)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].StartLine < sorted[j].StartLine
		})
		for i := 1; i < len(sorted); i++ {
			prev := sorted[i-1]
			curr := sorted[i]
			if curr.StartLine < prev.EndLine {
				t.Errorf("chunk overlap: %q (lines %d-%d) and %q (lines %d-%d)",
					prev.ChunkType, prev.StartLine, prev.EndLine,
					curr.ChunkType, curr.StartLine, curr.EndLine)
			}
		}
	})

	t.Run("ChunkSymbolIDs", func(t *testing.T) {
		for _, ch := range r.Chunks {
			if ch.SymbolID != "" {
				if _, ok := symIDs[ch.SymbolID]; !ok {
					t.Errorf("Chunk %q (lines %d-%d) references unknown SymbolID %q",
						ch.ChunkType, ch.StartLine, ch.EndLine, ch.SymbolID)
				}
			}
		}
	})

	t.Run("ModuleContextChunk", func(t *testing.T) {
		for _, ch := range r.Chunks {
			if ch.ChunkType == "module_context" {
				if ch.StartLine != 1 {
					t.Errorf("module_context chunk starts at line %d, want 1", ch.StartLine)
				}
				return
			}
		}
		// module_context chunk is optional — not an error if missing.
	})

	t.Run("ReferencePositions", func(t *testing.T) {
		for _, ref := range r.References {
			if ref.StartLine < 1 || ref.StartLine > r.LineCount {
				t.Errorf("Reference %q StartLine=%d out of range [1, %d]",
					ref.TargetName, ref.StartLine, r.LineCount)
			}
			if ref.EndLine < ref.StartLine || ref.EndLine > r.LineCount {
				t.Errorf("Reference %q EndLine=%d invalid (StartLine=%d, LineCount=%d)",
					ref.TargetName, ref.EndLine, ref.StartLine, r.LineCount)
			}
		}
	})

	t.Run("ReferenceSourceSymbolIDs", func(t *testing.T) {
		for _, ref := range r.References {
			if ref.SourceSymbolID != "" {
				if _, ok := symIDs[ref.SourceSymbolID]; !ok {
					t.Errorf("Reference %q has unknown SourceSymbolID %q",
						ref.TargetName, ref.SourceSymbolID)
				}
			}
		}
	})

	t.Run("JsxSourceSymbolIDs", func(t *testing.T) {
		if len(r.JsxUsages) == 0 {
			t.Fatal("expected JSX usages for tsx file")
		}
		for _, jsx := range r.JsxUsages {
			if jsx.SourceSymbolID != "" {
				if _, ok := symIDs[jsx.SourceSymbolID]; !ok {
					t.Errorf("JsxUsage %q has unknown SourceSymbolID %q",
						jsx.ComponentName, jsx.SourceSymbolID)
				}
			}
		}
	})

	t.Run("NetworkCallSourceSymbolIDs", func(t *testing.T) {
		if len(r.NetworkCalls) == 0 {
			t.Fatal("expected network calls for tsx file with fetch()")
		}
		for _, nc := range r.NetworkCalls {
			if nc.SourceSymbolID != "" {
				if _, ok := symIDs[nc.SourceSymbolID]; !ok {
					t.Errorf("NetworkCall has unknown SourceSymbolID %q", nc.SourceSymbolID)
				}
			}
		}
	})

	t.Run("FileMetadata", func(t *testing.T) {
		td := testdataDir(t)
		raw, err := os.ReadFile(filepath.Join(td, "fixtures", "typescript", "react-component.tsx"))
		if err != nil {
			t.Fatal(err)
		}
		normalized := parser.NormalizeNewlines(string(raw))
		expectedHash := parser.StableHash(normalized)
		expectedLines := parser.CountLines(normalized)
		expectedSize := int64(len(normalized))

		if r.FileHash != expectedHash {
			t.Errorf("FileHash = %q, want %q", r.FileHash, expectedHash)
		}
		if r.LineCount != expectedLines {
			t.Errorf("LineCount = %d, want %d", r.LineCount, expectedLines)
		}
		if r.SizeBytes != expectedSize {
			t.Errorf("SizeBytes = %d, want %d", r.SizeBytes, expectedSize)
		}
	})

	t.Run("ExtractorStatuses", func(t *testing.T) {
		sm := extractorStatusMap(r.ExtractorStatuses)
		// tsx is Tier 1 + JSX → all extractors enabled.
		expected := []string{"symbols", "imports", "exports", "references", "jsx_usages", "network_calls", "chunks", "diagnostics"}
		for _, name := range expected {
			status, ok := sm[name]
			if !ok {
				t.Errorf("missing extractor status for %q", name)
				continue
			}
			if status != "OK" {
				t.Errorf("extractor %q status = %q, want OK", name, status)
			}
		}
	})

	t.Run("FactsConsistency", func(t *testing.T) {
		if r.Facts == nil {
			t.Fatal("Facts should not be nil")
		}
		// HasJsx ↔ len(JsxUsages) > 0
		if r.Facts.HasJsx != (len(r.JsxUsages) > 0) {
			t.Errorf("HasJsx = %v, but len(JsxUsages) = %d", r.Facts.HasJsx, len(r.JsxUsages))
		}
		// HasFetchCalls ↔ len(NetworkCalls) > 0
		if r.Facts.HasFetchCalls != (len(r.NetworkCalls) > 0) {
			t.Errorf("HasFetchCalls = %v, but len(NetworkCalls) = %d", r.Facts.HasFetchCalls, len(r.NetworkCalls))
		}
		// HasClassDeclarations ↔ any symbol Kind == "class"
		hasClass := false
		for _, s := range r.Symbols {
			if s.Kind == "class" {
				hasClass = true
				break
			}
		}
		if r.Facts.HasClassDeclarations != hasClass {
			t.Errorf("HasClassDeclarations = %v, but hasClass = %v", r.Facts.HasClassDeclarations, hasClass)
		}
		// HasDefaultExport ↔ any export ExportKind == "DEFAULT"
		hasDefault := false
		for _, e := range r.Exports {
			if e.ExportKind == "DEFAULT" {
				hasDefault = true
				break
			}
		}
		if r.Facts.HasDefaultExport != hasDefault {
			t.Errorf("HasDefaultExport = %v, but hasDefault = %v", r.Facts.HasDefaultExport, hasDefault)
		}
		// HasNamedExports ↔ any export ExportKind == "NAMED"
		hasNamed := false
		for _, e := range r.Exports {
			if e.ExportKind == "NAMED" {
				hasNamed = true
				break
			}
		}
		if r.Facts.HasNamedExports != hasNamed {
			t.Errorf("HasNamedExports = %v, but hasNamed = %v", r.Facts.HasNamedExports, hasNamed)
		}
	})
}

// ---------------------------------------------------------------------------
// Tier 2 consistency: Bash
// ---------------------------------------------------------------------------

func TestConsistency_BashTier2(t *testing.T) {
	r := parseFixtureFile(t, "bash/deploy-script.sh")

	t.Run("TierAppropriateOutput", func(t *testing.T) {
		if len(r.Chunks) == 0 {
			t.Error("expected chunks for bash file")
		}
		if len(r.Exports) != 0 {
			t.Errorf("expected no exports for Tier 2, got %d", len(r.Exports))
		}
		if len(r.References) != 0 {
			t.Errorf("expected no references for Tier 2, got %d", len(r.References))
		}
		if len(r.JsxUsages) != 0 {
			t.Errorf("expected no jsx usages for Tier 2, got %d", len(r.JsxUsages))
		}
		if len(r.NetworkCalls) != 0 {
			t.Errorf("expected no network calls for Tier 2, got %d", len(r.NetworkCalls))
		}
	})

	t.Run("ExtractorStatuses", func(t *testing.T) {
		sm := extractorStatusMap(r.ExtractorStatuses)
		expected := []string{"symbols", "imports", "chunks", "diagnostics"}
		for _, name := range expected {
			if _, ok := sm[name]; !ok {
				t.Errorf("missing extractor status for %q", name)
			}
		}
		notExpected := []string{"exports", "references", "jsx_usages", "network_calls"}
		for _, name := range notExpected {
			if _, ok := sm[name]; ok {
				t.Errorf("unexpected extractor status for %q (Tier 2 should not have it)", name)
			}
		}
	})

	t.Run("FileMetadata", func(t *testing.T) {
		td := testdataDir(t)
		raw, err := os.ReadFile(filepath.Join(td, "fixtures", "bash", "deploy-script.sh"))
		if err != nil {
			t.Fatal(err)
		}
		normalized := parser.NormalizeNewlines(string(raw))
		if r.FileHash != parser.StableHash(normalized) {
			t.Error("FileHash mismatch")
		}
		if r.LineCount != parser.CountLines(normalized) {
			t.Error("LineCount mismatch")
		}
		if r.SizeBytes != int64(len(normalized)) {
			t.Error("SizeBytes mismatch")
		}
	})
}

// ---------------------------------------------------------------------------
// Tier 3 consistency: JSON
// ---------------------------------------------------------------------------

func TestConsistency_JSONTier3(t *testing.T) {
	r := parseFixtureFile(t, "json/package.json")

	t.Run("TierAppropriateOutput", func(t *testing.T) {
		if len(r.Chunks) == 0 {
			t.Error("expected chunks for JSON file (text-based chunking)")
		}
		// JSON has no tree-sitter grammar → symbols should be nil/empty.
		if len(r.Symbols) != 0 {
			t.Errorf("expected no symbols for JSON (no grammar), got %d", len(r.Symbols))
		}
		if len(r.Exports) != 0 {
			t.Errorf("expected no exports for Tier 3, got %d", len(r.Exports))
		}
		if len(r.References) != 0 {
			t.Errorf("expected no references for Tier 3, got %d", len(r.References))
		}
	})

	t.Run("ExtractorStatuses", func(t *testing.T) {
		sm := extractorStatusMap(r.ExtractorStatuses)
		expected := []string{"symbols", "chunks", "diagnostics"}
		for _, name := range expected {
			if _, ok := sm[name]; !ok {
				t.Errorf("missing extractor status for %q", name)
			}
		}
	})

	t.Run("FileMetadata", func(t *testing.T) {
		td := testdataDir(t)
		raw, err := os.ReadFile(filepath.Join(td, "fixtures", "json", "package.json"))
		if err != nil {
			t.Fatal(err)
		}
		normalized := parser.NormalizeNewlines(string(raw))
		if r.FileHash != parser.StableHash(normalized) {
			t.Error("FileHash mismatch")
		}
		if r.LineCount != parser.CountLines(normalized) {
			t.Error("LineCount mismatch")
		}
		if r.SizeBytes != int64(len(normalized)) {
			t.Error("SizeBytes mismatch")
		}
	})
}
