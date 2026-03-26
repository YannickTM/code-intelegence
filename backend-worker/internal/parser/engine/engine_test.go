package engine

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const tsContent = `import React, { useState } from 'react';
import './styles.css';

export interface User {
  name: string;
  age: number;
}

export default function App(): JSX.Element {
  const [count, setCount] = useState(0);

  async function fetchData(): Promise<void> {
    const res = await fetch('/api/data');
    const data = await res.json();
    console.log(data);
  }

  return <div onClick={() => setCount(count + 1)}>{count}</div>;
}

class UserService {
  getUser(id: string): User {
    return { name: 'test', age: 0 };
  }
}
`

const pyContent = `import os
import json
from pathlib import Path

def greet(name: str) -> str:
    """Greet someone."""
    return f"Hello, {name}!"

class Calculator:
    """A simple calculator."""
    def add(self, a: int, b: int) -> int:
        return a + b

    def subtract(self, a: int, b: int) -> int:
        return a - b

def main():
    calc = Calculator()
    print(greet("World"))
    print(calc.add(1, 2))
`

const goContent = `package main

import (
	"fmt"
	"net/http"
)

// Handler handles HTTP requests.
type Handler struct {
	name string
}

// ServeHTTP implements the http.Handler interface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, %s!", h.name)
}

func main() {
	h := &Handler{name: "World"}
	http.ListenAndServe(":8080", h)
}
`

const bashContent = `#!/bin/bash

set -euo pipefail

# Deploy the application
deploy() {
    local env="$1"
    echo "Deploying to $env..."
    docker build -t myapp .
    docker push myapp
}

cleanup() {
    echo "Cleaning up..."
    rm -rf /tmp/build
}

deploy "production"
cleanup
`

const jsonContent = `{
  "name": "my-app",
  "version": "1.0.0",
  "dependencies": {
    "react": "^18.0.0"
  }
}
`

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	eng, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer eng.Close()

	if eng.pool == nil {
		t.Fatal("pool should not be nil")
	}
	if eng.pool.Size() != runtime.NumCPU() {
		t.Errorf("pool size = %d, want %d", eng.pool.Size(), runtime.NumCPU())
	}
	if eng.timeout != defaultTimeoutFile {
		t.Errorf("timeout = %v, want %v", eng.timeout, defaultTimeoutFile)
	}
	if eng.maxFileSize != defaultMaxFileSize {
		t.Errorf("maxFileSize = %d, want %d", eng.maxFileSize, defaultMaxFileSize)
	}
}

func TestNew_CustomConfig(t *testing.T) {
	eng, err := New(Config{
		PoolSize:       4,
		TimeoutPerFile: 5 * time.Second,
		MaxFileSize:    512 * 1024,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer eng.Close()

	if eng.pool.Size() != 4 {
		t.Errorf("pool size = %d, want 4", eng.pool.Size())
	}
	if eng.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", eng.timeout)
	}
	if eng.maxFileSize != 512*1024 {
		t.Errorf("maxFileSize = %d, want %d", eng.maxFileSize, 512*1024)
	}
}

// ---------------------------------------------------------------------------
// Single-file pipeline tests
// ---------------------------------------------------------------------------

func TestParseFilesBatched_TypeScript(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "src/App.tsx", Content: tsContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	r := results[0]
	if r.FilePath != "src/App.tsx" {
		t.Errorf("FilePath = %q, want %q", r.FilePath, "src/App.tsx")
	}
	if r.Language != "tsx" {
		t.Errorf("Language = %q, want %q", r.Language, "tsx")
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
	if r.LineCount == 0 {
		t.Error("LineCount should not be 0")
	}
	if r.SizeBytes == 0 {
		t.Error("SizeBytes should not be 0")
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty for TSX file")
	}
	if len(r.Imports) == 0 {
		t.Error("Imports should not be empty for TSX file")
	}
	if len(r.Exports) == 0 {
		t.Error("Exports should not be empty for TSX file")
	}
	if len(r.References) == 0 {
		t.Error("References should not be empty for TSX file")
	}
	if len(r.JsxUsages) == 0 {
		t.Error("JsxUsages should not be empty for TSX file")
	}
	if len(r.NetworkCalls) == 0 {
		t.Error("NetworkCalls should not be empty for TSX file with fetch()")
	}
	if len(r.Chunks) == 0 {
		t.Error("Chunks should not be empty for TSX file")
	}
}

func TestParseFilesBatched_Python(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "app.py", Content: pyContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "python" {
		t.Errorf("Language = %q, want %q", r.Language, "python")
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty for Python file")
	}
	if len(r.Imports) == 0 {
		t.Error("Imports should not be empty for Python file")
	}
	if len(r.Exports) == 0 {
		t.Error("Exports should not be empty for Python file (convention-based)")
	}
	if len(r.Chunks) == 0 {
		t.Error("Chunks should not be empty for Python file")
	}
	// Python is Tier 1 — should have references
	if len(r.References) == 0 {
		t.Error("References should not be empty for Python file")
	}
	// No JSX usages or network calls expected
	if len(r.JsxUsages) != 0 {
		t.Errorf("JsxUsages = %d, want 0 for Python", len(r.JsxUsages))
	}
}

func TestParseFilesBatched_Go(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "main.go", Content: goContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "go" {
		t.Errorf("Language = %q, want %q", r.Language, "go")
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty for Go file")
	}
	if len(r.Imports) == 0 {
		t.Error("Imports should not be empty for Go file")
	}
	if len(r.Chunks) == 0 {
		t.Error("Chunks should not be empty for Go file")
	}
}

func TestParseFilesBatched_Bash(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "deploy.sh", Content: bashContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "bash" {
		t.Errorf("Language = %q, want %q", r.Language, "bash")
	}
	// Bash is Tier 2 — no exports, references, jsx, or network calls
	if len(r.Exports) != 0 {
		t.Errorf("Exports = %d, want 0 for Tier 2 bash", len(r.Exports))
	}
	if len(r.References) != 0 {
		t.Errorf("References = %d, want 0 for Tier 2 bash", len(r.References))
	}
	if len(r.JsxUsages) != 0 {
		t.Errorf("JsxUsages = %d, want 0 for Tier 2 bash", len(r.JsxUsages))
	}
	if len(r.NetworkCalls) != 0 {
		t.Errorf("NetworkCalls = %d, want 0 for Tier 2 bash", len(r.NetworkCalls))
	}
	// But should have symbols, imports, chunks
	if len(r.Chunks) == 0 {
		t.Error("Chunks should not be empty for bash file")
	}
}

func TestParseFilesBatched_JSON(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "package.json", Content: jsonContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "json" {
		t.Errorf("Language = %q, want %q", r.Language, "json")
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty for JSON")
	}
	if r.SizeBytes == 0 {
		t.Error("SizeBytes should not be 0 for JSON")
	}
	// JSON has no grammar — AST-dependent extractors return nil
	if len(r.Symbols) != 0 {
		t.Errorf("Symbols = %d, want 0 for JSON (no grammar)", len(r.Symbols))
	}
	if len(r.Exports) != 0 {
		t.Errorf("Exports = %d, want 0 for JSON", len(r.Exports))
	}
	// Chunks should still work (text-based)
	if len(r.Chunks) == 0 {
		t.Error("Chunks should not be empty for JSON (text-based chunking)")
	}
}

// ---------------------------------------------------------------------------
// Batch pipeline tests
// ---------------------------------------------------------------------------

func TestParseFilesBatched_BatchOrder(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	files := []parser.FileInput{
		{FilePath: "a.ts", Content: "const a = 1;\n"},
		{FilePath: "b.py", Content: "b = 2\n"},
		{FilePath: "c.go", Content: "package main\nvar c = 3\n"},
		{FilePath: "d.sh", Content: "#!/bin/bash\necho d\n"},
		{FilePath: "e.json", Content: `{"e": 5}`},
	}

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", files)
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}

	for i, f := range files {
		if results[i].FilePath != f.FilePath {
			t.Errorf("results[%d].FilePath = %q, want %q", i, results[i].FilePath, f.FilePath)
		}
	}
}

func TestParseFilesBatched_EmptySlice(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", nil)
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

// ---------------------------------------------------------------------------
// Edge case tests
// ---------------------------------------------------------------------------

func TestParseFilesBatched_EmptyFile(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "empty.ts", Content: ""},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.FilePath != "empty.ts" {
		t.Errorf("FilePath = %q, want %q", r.FilePath, "empty.ts")
	}
	if r.Language != "" {
		t.Errorf("Language = %q, want empty for empty file", r.Language)
	}
	if r.FileHash != "" {
		t.Errorf("FileHash = %q, want empty for empty file", r.FileHash)
	}
	if r.LineCount != 0 {
		t.Errorf("LineCount = %d, want 0", r.LineCount)
	}
}

func TestParseFilesBatched_OversizedFile(t *testing.T) {
	eng, err := New(Config{
		PoolSize:    2,
		MaxFileSize: 100, // 100 bytes
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer eng.Close()

	bigContent := strings.Repeat("x", 200)
	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "big.ts", Content: bigContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if len(r.Issues) == 0 {
		t.Fatal("Issues should contain OVERSIZED_FILE")
	}
	foundOversized := false
	for _, iss := range r.Issues {
		if iss.Code == "OVERSIZED_FILE" {
			foundOversized = true
			break
		}
	}
	if !foundOversized {
		t.Errorf("expected OVERSIZED_FILE issue, got: %+v", r.Issues)
	}
	// Should still have file metadata
	if r.FileHash == "" {
		t.Error("FileHash should be set even for oversized files")
	}
	if r.SizeBytes == 0 {
		t.Error("SizeBytes should be set even for oversized files")
	}
}

func TestParseFilesBatched_UnsupportedExtension(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "data.xyz", Content: "some content"},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	foundUnsupported := false
	for _, iss := range r.Issues {
		if iss.Code == "UNSUPPORTED_LANGUAGE" {
			foundUnsupported = true
			break
		}
	}
	if !foundUnsupported {
		t.Errorf("expected UNSUPPORTED_LANGUAGE issue, got: %+v", r.Issues)
	}
	if r.FileHash == "" {
		t.Error("FileHash should be set even for unsupported files")
	}
}

func TestParseFilesBatched_PartialFailure(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "good.py", Content: pyContent},
		{FilePath: "bad.xyz", Content: "cannot parse this"},
		{FilePath: "also_good.go", Content: goContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	// First and third should parse successfully
	if results[0].Language != "python" {
		t.Errorf("results[0].Language = %q, want python", results[0].Language)
	}
	if len(results[0].Symbols) == 0 {
		t.Error("results[0] should have symbols")
	}

	// Second should have UNSUPPORTED_LANGUAGE issue
	foundUnsupported := false
	for _, iss := range results[1].Issues {
		if iss.Code == "UNSUPPORTED_LANGUAGE" {
			foundUnsupported = true
			break
		}
	}
	if !foundUnsupported {
		t.Error("results[1] should have UNSUPPORTED_LANGUAGE issue")
	}

	// Third should parse successfully
	if results[2].Language != "go" {
		t.Errorf("results[2].Language = %q, want go", results[2].Language)
	}
	if len(results[2].Symbols) == 0 {
		t.Error("results[2] should have symbols")
	}
}

func TestParseFilesBatched_PresetLanguage(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	// File has .txt extension but language is preset to "python"
	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "script.txt", Content: pyContent, Language: "python"},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "python" {
		t.Errorf("Language = %q, want python (preset)", r.Language)
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty when language is preset")
	}
}

func TestParseFilesBatched_CRLFNormalization(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	crlfContent := "const a = 1;\r\nconst b = 2;\r\n"
	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "crlf.ts", Content: crlfContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "typescript" {
		t.Errorf("Language = %q, want typescript", r.Language)
	}
	// Should parse successfully despite CRLF
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
}

func TestParseFilesBatched_BOMNormalization(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	bomContent := "\xef\xbb\xbfconst a = 1;\nconst b = 2;\n"
	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "bom.ts", Content: bomContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Language != "typescript" {
		t.Errorf("Language = %q, want typescript", r.Language)
	}
	if r.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
	if len(r.Symbols) == 0 {
		t.Error("Symbols should not be empty after BOM stripping")
	}
}

// ---------------------------------------------------------------------------
// FileFacts tests
// ---------------------------------------------------------------------------

func TestParseFilesBatched_FileFacts(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "src/App.tsx", Content: tsContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Facts == nil {
		t.Fatal("Facts should not be nil")
	}
	if !r.Facts.HasJsx {
		t.Error("HasJsx should be true for TSX file with JSX")
	}
	if !r.Facts.HasDefaultExport {
		t.Error("HasDefaultExport should be true for file with export default")
	}
	if !r.Facts.HasClassDeclarations {
		t.Error("HasClassDeclarations should be true for file with class")
	}
	if r.Facts.JsxRuntime != "react" {
		t.Errorf("JsxRuntime = %q, want %q", r.Facts.JsxRuntime, "react")
	}
}

func TestParseFilesBatched_FileFacts_NonJsx(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "app.py", Content: pyContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Facts == nil {
		t.Fatal("Facts should not be nil")
	}
	if r.Facts.HasJsx {
		t.Error("HasJsx should be false for Python file")
	}
	if r.Facts.JsxRuntime != "" {
		t.Errorf("JsxRuntime = %q, want empty for non-JSX", r.Facts.JsxRuntime)
	}
}

func TestParseFilesBatched_FileFacts_TestFile(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	testContent := `import { render } from '@testing-library/react';
import App from './App';

test('renders', () => {
  render(<App />);
});
`
	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "src/App.test.tsx", Content: testContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Facts == nil {
		t.Fatal("Facts should not be nil")
	}
	if !r.Facts.HasTests {
		t.Error("HasTests should be true for .test.tsx file")
	}
}

func TestParseFilesBatched_FileFacts_ConfigFile(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "package.json", Content: jsonContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Facts == nil {
		t.Fatal("Facts should not be nil")
	}
	if !r.Facts.HasConfigPatterns {
		t.Error("HasConfigPatterns should be true for package.json")
	}
}

// ---------------------------------------------------------------------------
// Metadata tests
// ---------------------------------------------------------------------------

func TestParseFilesBatched_Metadata(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "src/App.tsx", Content: tsContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if r.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}
	if r.Metadata.ParserVersion != parserVersion {
		t.Errorf("ParserVersion = %q, want %q", r.Metadata.ParserVersion, parserVersion)
	}
	if r.Metadata.GrammarVersion != grammarVersion {
		t.Errorf("GrammarVersion = %q, want %q", r.Metadata.GrammarVersion, grammarVersion)
	}
	if len(r.Metadata.EnabledExtractors) == 0 {
		t.Error("EnabledExtractors should not be empty")
	}

	// TSX should enable all extractors
	expected := map[string]bool{
		"symbols": true, "imports": true, "exports": true,
		"references": true, "jsx_usages": true, "network_calls": true,
		"chunks": true, "diagnostics": true,
	}
	for _, name := range r.Metadata.EnabledExtractors {
		delete(expected, name)
	}
	if len(expected) > 0 {
		missing := make([]string, 0, len(expected))
		for name := range expected {
			missing = append(missing, name)
		}
		t.Errorf("missing extractors: %v", missing)
	}
}

func TestParseFilesBatched_ExtractorStatuses(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "src/App.tsx", Content: tsContent},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched error = %v", err)
	}

	r := results[0]
	if len(r.ExtractorStatuses) == 0 {
		t.Fatal("ExtractorStatuses should not be empty")
	}

	for _, s := range r.ExtractorStatuses {
		if s.ExtractorName == "" {
			t.Error("ExtractorName should not be empty")
		}
		if s.Status != "OK" && s.Status != "PARTIAL" && s.Status != "FAILED" {
			t.Errorf("unexpected status %q for extractor %q", s.Status, s.ExtractorName)
		}
	}
}

// ---------------------------------------------------------------------------
// runExtractor panic recovery test
// ---------------------------------------------------------------------------

func TestRunExtractor_PanicRecovery(t *testing.T) {
	result, status := runExtractor("panicky", func() []parser.Symbol {
		panic("test panic")
	})

	if result != nil {
		t.Errorf("result should be nil, got %v", result)
	}
	if status.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED", status.Status)
	}
	if status.ExtractorName != "panicky" {
		t.Errorf("ExtractorName = %q, want panicky", status.ExtractorName)
	}
	if !strings.Contains(status.Message, "test panic") {
		t.Errorf("Message = %q, should contain 'test panic'", status.Message)
	}
}

func TestRunExtractor_Success(t *testing.T) {
	result, status := runExtractor("good", func() []string {
		return []string{"a", "b"}
	})

	if len(result) != 2 {
		t.Errorf("result len = %d, want 2", len(result))
	}
	if status.Status != "OK" {
		t.Errorf("status = %q, want OK", status.Status)
	}
	if status.ExtractorName != "good" {
		t.Errorf("ExtractorName = %q, want good", status.ExtractorName)
	}
}

// ---------------------------------------------------------------------------
// Close test
// ---------------------------------------------------------------------------

func TestEngine_Close(t *testing.T) {
	eng := mustEngine(t)
	eng.Close()

	// After close, ParseFilesBatched should still return results
	// (with PARSER_FAILURE since pool is shut down — partial failure semantics).
	results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "test.ts", Content: "const x = 1;"},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched after Close should not return error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	r := results[0]
	if r.FilePath != "test.ts" {
		t.Errorf("FilePath = %q, want test.ts", r.FilePath)
	}
	// Pool shutdown produces PARSER_FAILURE (not PARSE_TIMEOUT or PARSE_ERROR).
	foundParserFailure := false
	for _, iss := range r.Issues {
		if iss.Code == "PARSER_FAILURE" {
			foundParserFailure = true
			break
		}
	}
	if !foundParserFailure {
		t.Errorf("expected PARSER_FAILURE issue after Close, got: %+v", r.Issues)
	}
}

func TestEngine_CloseNil(t *testing.T) {
	// Should not panic
	var eng *Engine
	eng.Close()
}

// ---------------------------------------------------------------------------
// Determinism test
// ---------------------------------------------------------------------------

func TestEngine_Determinism(t *testing.T) {
	// Use PoolSize=1 to eliminate non-determinism from Go map iteration
	// inside extractors when different sitter.Parser instances are used.
	eng, err := New(Config{PoolSize: 1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer eng.Close()

	files := []parser.FileInput{
		{FilePath: "src/App.tsx", Content: tsContent},
	}

	var baseline []byte
	for i := range 100 {
		results, err := eng.ParseFilesBatched(context.Background(), "proj", "main", "abc123", files)
		if err != nil {
			t.Fatalf("iteration %d: error = %v", i, err)
		}
		b, err := json.Marshal(results)
		if err != nil {
			t.Fatalf("iteration %d: json.Marshal error = %v", i, err)
		}
		if i == 0 {
			baseline = b
		} else if string(b) != string(baseline) {
			t.Fatalf("iteration %d: output differs from baseline", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Context cancellation test
// ---------------------------------------------------------------------------

func TestParseFilesBatched_ContextCancelled(t *testing.T) {
	eng := mustEngine(t)
	defer eng.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	results, err := eng.ParseFilesBatched(ctx, "proj", "main", "abc123", []parser.FileInput{
		{FilePath: "test.ts", Content: "const x = 1;"},
	})
	if err != nil {
		t.Fatalf("ParseFilesBatched should not return error, got %v", err)
	}
	// Should still return a result (with parse issues since context cancelled)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	// Parent cancellation should produce PARSER_FAILURE, not PARSE_TIMEOUT.
	r := results[0]
	foundParserFailure := false
	for _, iss := range r.Issues {
		if iss.Code == "PARSE_TIMEOUT" {
			t.Errorf("parent context cancellation should not produce PARSE_TIMEOUT, got: %+v", iss)
		}
		if iss.Code == "PARSER_FAILURE" {
			foundParserFailure = true
		}
	}
	if !foundParserFailure {
		t.Errorf("expected PARSER_FAILURE issue for cancelled context, got: %+v", r.Issues)
	}
}

// ---------------------------------------------------------------------------
// computeFileFacts unit tests
// ---------------------------------------------------------------------------

func TestHasDefaultExport(t *testing.T) {
	if hasDefaultExport(nil) {
		t.Error("nil exports should return false")
	}
	if hasDefaultExport([]parser.Export{{ExportKind: "NAMED"}}) {
		t.Error("NAMED export should return false")
	}
	if !hasDefaultExport([]parser.Export{{ExportKind: "DEFAULT"}}) {
		t.Error("DEFAULT export should return true")
	}
}

func TestHasNamedExports(t *testing.T) {
	if hasNamedExports(nil) {
		t.Error("nil exports should return false")
	}
	if !hasNamedExports([]parser.Export{{ExportKind: "NAMED"}}) {
		t.Error("NAMED export should return true")
	}
	if hasNamedExports([]parser.Export{{ExportKind: "DEFAULT"}}) {
		t.Error("DEFAULT export should return false")
	}
}

func TestHasSideEffectImports(t *testing.T) {
	if hasSideEffectImports(nil) {
		t.Error("nil imports should return false")
	}
	if !hasSideEffectImports([]parser.Import{{TargetFilePath: "./styles.css"}}) {
		t.Error("CSS import should be a side effect")
	}
	if !hasSideEffectImports([]parser.Import{{ImportName: "./theme.scss"}}) {
		t.Error("SCSS import should be a side effect")
	}
	if hasSideEffectImports([]parser.Import{{TargetFilePath: "react"}}) {
		t.Error("react import should not be a side effect")
	}
}

func TestHasHookCalls(t *testing.T) {
	if hasHookCalls(nil) {
		t.Error("nil refs should return false")
	}
	if !hasHookCalls([]parser.Reference{{ReferenceKind: "HOOK_USE"}}) {
		t.Error("HOOK_USE reference should return true")
	}
	if hasHookCalls([]parser.Reference{{ReferenceKind: "CALL"}}) {
		t.Error("CALL reference should return false")
	}
}

func TestHasClasses(t *testing.T) {
	if hasClasses(nil) {
		t.Error("nil symbols should return false")
	}
	if !hasClasses([]parser.Symbol{{Kind: "class"}}) {
		t.Error("class symbol should return true")
	}
	if hasClasses([]parser.Symbol{{Kind: "function"}}) {
		t.Error("function symbol should return false")
	}
}

func TestHasTestPatterns(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/App.test.tsx", true},
		{"src/App.spec.ts", true},
		{"main_test.go", true},
		{"src/__tests__/App.tsx", true},
		{"src/test/helper.ts", true},
		{"src/App.tsx", false},
		{"src/main.go", false},
	}
	for _, tt := range tests {
		if got := hasTestPatterns(tt.path); got != tt.want {
			t.Errorf("hasTestPatterns(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestHasConfigPatterns(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"package.json", true},
		{"tsconfig.json", true},
		{"webpack.config.js", true},
		{"Dockerfile", true},
		{".env", true},
		{"src/App.tsx", false},
		{"main.go", false},
	}
	for _, tt := range tests {
		if got := hasConfigPatterns(tt.path); got != tt.want {
			t.Errorf("hasConfigPatterns(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestDetectJsxRuntime(t *testing.T) {
	// Non-JSX language → empty
	if got := detectJsxRuntime(nil, "python"); got != "" {
		t.Errorf("non-JSX = %q, want empty", got)
	}

	// JSX with react import → "react"
	if got := detectJsxRuntime([]parser.Import{{ImportName: "react"}}, "tsx"); got != "react" {
		t.Errorf("react import = %q, want react", got)
	}

	// JSX with preact import → "preact"
	if got := detectJsxRuntime([]parser.Import{{ImportName: "preact"}}, "jsx"); got != "preact" {
		t.Errorf("preact import = %q, want preact", got)
	}

	// JSX with no recognizable runtime → "unknown"
	if got := detectJsxRuntime([]parser.Import{{ImportName: "solid-js"}}, "tsx"); got != "unknown" {
		t.Errorf("unknown runtime = %q, want unknown", got)
	}

	// JSX with no imports → "unknown"
	if got := detectJsxRuntime(nil, "tsx"); got != "unknown" {
		t.Errorf("no imports = %q, want unknown", got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustEngine(t *testing.T) *Engine {
	t.Helper()
	eng, err := New(Config{PoolSize: 2})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return eng
}
