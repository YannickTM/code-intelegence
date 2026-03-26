package extractors

import (
	"fmt"
	"strings"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func findChunk(chunks []parser.Chunk, chunkType string) *parser.Chunk {
	for i := range chunks {
		if chunks[i].ChunkType == chunkType {
			return &chunks[i]
		}
	}
	return nil
}

func findChunks(chunks []parser.Chunk, chunkType string) []parser.Chunk {
	var out []parser.Chunk
	for _, c := range chunks {
		if c.ChunkType == chunkType {
			out = append(out, c)
		}
	}
	return out
}

func sym(name, kind string, start, end int32) parser.Symbol {
	return parser.Symbol{
		SymbolID:      "sym_" + name,
		Name:          name,
		QualifiedName: name,
		Kind:          kind,
		StartLine:     start,
		EndLine:       end,
	}
}

func symWithParent(name, kind, parentID string, start, end int32) parser.Symbol {
	return parser.Symbol{
		SymbolID:       "sym_" + name,
		Name:           name,
		QualifiedName:  name,
		Kind:           kind,
		StartLine:      start,
		EndLine:        end,
		ParentSymbolID: parentID,
	}
}

func symExported(name, kind string, start, end int32) parser.Symbol {
	return parser.Symbol{
		SymbolID:      "sym_" + name,
		Name:          name,
		QualifiedName: name,
		Kind:          kind,
		StartLine:     start,
		EndLine:       end,
		Flags:         &parser.SymbolFlags{IsExported: true},
	}
}

func symWithDoc(name, kind, doc string, start, end int32) parser.Symbol {
	return parser.Symbol{
		SymbolID:      "sym_" + name,
		Name:          name,
		QualifiedName: name,
		Kind:          kind,
		StartLine:     start,
		EndLine:       end,
		DocText:       doc,
	}
}

func symReactComponent(name string, start, end int32) parser.Symbol {
	return parser.Symbol{
		SymbolID:      "sym_" + name,
		Name:          name,
		QualifiedName: name,
		Kind:          "function",
		StartLine:     start,
		EndLine:       end,
		Flags:         &parser.SymbolFlags{IsExported: true, IsReactComponentLike: true},
	}
}

func symHook(name string, start, end int32) parser.Symbol {
	return parser.Symbol{
		SymbolID:      "sym_" + name,
		Name:          name,
		QualifiedName: name,
		Kind:          "function",
		StartLine:     start,
		EndLine:       end,
		Flags:         &parser.SymbolFlags{IsExported: true, IsHookLike: true},
	}
}

// ---------------------------------------------------------------------------
// Early returns
// ---------------------------------------------------------------------------

func TestExtractChunks_EmptyContent(t *testing.T) {
	chunks := ExtractChunks("", "main.go", nil, nil, "go")
	if chunks != nil {
		t.Errorf("expected nil, got %d chunks", len(chunks))
	}
}

func TestExtractChunks_EmptyLangID(t *testing.T) {
	chunks := ExtractChunks("package main\n", "main.go", nil, nil, "")
	if chunks != nil {
		t.Errorf("expected nil, got %d chunks", len(chunks))
	}
}

func TestExtractChunks_UnknownLanguage(t *testing.T) {
	chunks := ExtractChunks("hello", "hello.xyz", nil, nil, "unknown_lang")
	if chunks != nil {
		t.Errorf("expected nil, got %d chunks", len(chunks))
	}
}

// ---------------------------------------------------------------------------
// Config files
// ---------------------------------------------------------------------------

func TestExtractChunks_ConfigFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		langID   string
		content  string
	}{
		{"go.mod", "go.mod", "go", "module example.com/foo\n\ngo 1.21\n"},
		{"Cargo.toml", "Cargo.toml", "rust", "[package]\nname = \"myapp\"\n"},
		{"pom.xml", "pom.xml", "java", "<project>...</project>\n"},
		{"pyproject.toml", "pyproject.toml", "python", "[tool.poetry]\nname = \"foo\"\n"},
		{"webpack.config.js", "webpack.config.js", "javascript", "module.exports = {};\n"},
		{"tsconfig.json", "tsconfig.json", "json", "{\"compilerOptions\": {}}\n"},
		{"Gemfile", "Gemfile", "ruby", "source 'https://rubygems.org'\n"},
		{"composer.json", "composer.json", "php", "{\"name\": \"vendor/pkg\"}\n"},
		{"Dockerfile", "Dockerfile", "dockerfile", "FROM node:18\nRUN npm install\n"},
		{"terraform.tfvars", "terraform.tfvars", "hcl", "region = \"us-east-1\"\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ExtractChunks(tt.content, tt.filePath, nil, nil, tt.langID)
			if len(chunks) != 1 {
				t.Fatalf("expected 1 chunk, got %d", len(chunks))
			}
			c := chunks[0]
			if c.ChunkType != "config" {
				t.Errorf("ChunkType = %q, want config", c.ChunkType)
			}
			if c.SemanticRole != "config" {
				t.Errorf("SemanticRole = %q, want config", c.SemanticRole)
			}
			if c.Content != tt.content {
				t.Errorf("Content mismatch")
			}
			if c.StartLine != 1 {
				t.Errorf("StartLine = %d, want 1", c.StartLine)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tier 3 full-file
// ---------------------------------------------------------------------------

func TestExtractChunks_Tier3_FullFile(t *testing.T) {
	content := "body { color: red; }\n"
	chunks := ExtractChunks(content, "style.css", nil, nil, "css")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.ChunkType != "module_context" {
		t.Errorf("ChunkType = %q, want module_context", c.ChunkType)
	}
	if c.Content != content {
		t.Errorf("Content mismatch")
	}
}

func TestExtractChunks_Tier3_ConfigOverride(t *testing.T) {
	// package.json is Tier 3 (json) but matches ConfigFilePatterns → CONFIG, not MODULE_CONTEXT.
	content := `{"name": "myapp"}` + "\n"
	chunks := ExtractChunks(content, "package.json", nil, nil, "json")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkType != "config" {
		t.Errorf("ChunkType = %q, want config", chunks[0].ChunkType)
	}
}

// ---------------------------------------------------------------------------
// Test files
// ---------------------------------------------------------------------------

func TestExtractChunks_TestFile_Go(t *testing.T) {
	content := `package foo

import "testing"

func TestAdd(t *testing.T) {
	if add(1, 2) != 3 {
		t.Fatal("wrong")
	}
}

func TestSub(t *testing.T) {
	if sub(3, 1) != 2 {
		t.Fatal("wrong")
	}
}
`
	chunks := ExtractChunks(content, "foo_test.go", nil, nil, "go")
	testChunks := findChunks(chunks, "test")
	if len(testChunks) != 2 {
		t.Fatalf("expected 2 test chunks, got %d", len(testChunks))
	}
	if !strings.Contains(testChunks[0].Content, "TestAdd") {
		t.Errorf("first test chunk should contain TestAdd")
	}
	if !strings.Contains(testChunks[1].Content, "TestSub") {
		t.Errorf("second test chunk should contain TestSub")
	}
	for _, c := range testChunks {
		if c.SemanticRole != "test" {
			t.Errorf("SemanticRole = %q, want test", c.SemanticRole)
		}
	}
}

func TestExtractChunks_TestFile_JS(t *testing.T) {
	content := `describe('math', () => {
  test('add', () => {
    expect(1 + 2).toBe(3);
  });

  test('sub', () => {
    expect(3 - 1).toBe(2);
  });
});
`
	chunks := ExtractChunks(content, "math.test.js", nil, nil, "javascript")
	testChunks := findChunks(chunks, "test")
	// describe at line 1, test at line 2, test at line 6 — 3 blocks.
	if len(testChunks) < 2 {
		t.Fatalf("expected at least 2 test chunks, got %d", len(testChunks))
	}
}

func TestExtractChunks_TestFile_Fallback(t *testing.T) {
	// A Go test file but content has no test block patterns.
	content := `package foo

var x = 1
`
	chunks := ExtractChunks(content, "foo_test.go", nil, nil, "go")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 fallback test chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkType != "test" {
		t.Errorf("ChunkType = %q, want test", chunks[0].ChunkType)
	}
}

// ---------------------------------------------------------------------------
// MODULE_CONTEXT
// ---------------------------------------------------------------------------

func TestExtractChunks_ModuleContext(t *testing.T) {
	content := `package main

import "fmt"

func hello() {
	fmt.Println("hi")
}
`
	symbols := []parser.Symbol{sym("hello", "function", 5, 7)}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	mc := findChunk(chunks, "module_context")
	if mc == nil {
		t.Fatal("expected module_context chunk")
	}
	if mc.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1", mc.StartLine)
	}
	if mc.EndLine != 4 {
		t.Errorf("EndLine = %d, want 4", mc.EndLine)
	}
}

func TestExtractChunks_ModuleContext_SkippedAtLine1(t *testing.T) {
	content := `func main() {
	println("hi")
}
`
	symbols := []parser.Symbol{sym("main", "function", 1, 3)}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	if mc := findChunk(chunks, "module_context"); mc != nil {
		t.Errorf("expected no module_context chunk when first symbol at line 1, got lines %d-%d", mc.StartLine, mc.EndLine)
	}
}

func TestExtractChunks_NoSymbols_FullModuleContext(t *testing.T) {
	content := `package utils

const Version = "1.0"
`
	chunks := ExtractChunks(content, "utils.go", nil, nil, "go")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkType != "module_context" {
		t.Errorf("ChunkType = %q, want module_context", chunks[0].ChunkType)
	}
}

// ---------------------------------------------------------------------------
// FUNCTION chunks
// ---------------------------------------------------------------------------

func TestExtractChunks_FunctionChunks(t *testing.T) {
	content := `package main

import "fmt"

func greet(name string) {
	fmt.Println("Hello", name)
}

func farewell(name string) {
	fmt.Println("Bye", name)
}
`
	symbols := []parser.Symbol{
		sym("greet", "function", 5, 7),
		sym("farewell", "function", 9, 11),
	}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	fns := findChunks(chunks, "function")
	if len(fns) != 2 {
		t.Fatalf("expected 2 function chunks, got %d", len(fns))
	}
	if !strings.Contains(fns[0].Content, "greet") {
		t.Errorf("first function chunk should contain greet")
	}
	if !strings.Contains(fns[1].Content, "farewell") {
		t.Errorf("second function chunk should contain farewell")
	}
}

func TestExtractChunks_DocCommentExtension(t *testing.T) {
	content := `package main

import "fmt"

// greet prints a greeting.
// It takes a name parameter.
func greet(name string) {
	fmt.Println("Hello", name)
}
`
	symbols := []parser.Symbol{symWithDoc("greet", "function", "greet prints a greeting.", 7, 9)}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	fn := findChunk(chunks, "function")
	if fn == nil {
		t.Fatal("expected function chunk")
	}
	// Doc comment starts at line 5, so chunk should start at 5.
	if fn.StartLine != 5 {
		t.Errorf("StartLine = %d, want 5 (with doc comment)", fn.StartLine)
	}
	if !strings.Contains(fn.Content, "// greet prints a greeting.") {
		t.Errorf("function chunk should include doc comment")
	}
}

// ---------------------------------------------------------------------------
// CLASS chunks
// ---------------------------------------------------------------------------

func TestExtractChunks_ClassChunk_Small(t *testing.T) {
	// A small class (< 200 lines) should have full text.
	lines := []string{
		`class Greeter {`,
		`  greet() { return "hello"; }`,
		`}`,
	}
	content := strings.Join(lines, "\n") + "\n"
	symbols := []parser.Symbol{
		sym("Greeter", "class", 1, 3),
		symWithParent("greet", "method", "sym_Greeter", 2, 2),
	}
	chunks := ExtractChunks(content, "greeter.js", symbols, nil, "javascript")

	cls := findChunk(chunks, "class")
	if cls == nil {
		t.Fatal("expected class chunk")
	}
	if !strings.Contains(cls.Content, "class Greeter") {
		t.Errorf("class chunk should contain full class text")
	}
}

func TestExtractChunks_ClassChunk_Large(t *testing.T) {
	// Build a class with > 200 lines.
	var sb strings.Builder
	sb.WriteString("class BigClass {\n")
	for i := 0; i < 210; i++ {
		sb.WriteString(fmt.Sprintf("  method%d() { return %d; }\n", i, i))
	}
	sb.WriteString("}\n")
	content := sb.String()

	classSym := sym("BigClass", "class", 1, 212)
	var methods []parser.Symbol
	for i := 0; i < 210; i++ {
		m := symWithParent(fmt.Sprintf("method%d", i), "method", "sym_BigClass", int32(i+2), int32(i+2))
		m.Signature = fmt.Sprintf("method%d()", i)
		methods = append(methods, m)
	}
	symbols := append([]parser.Symbol{classSym}, methods...)

	chunks := ExtractChunks(content, "big.js", symbols, nil, "javascript")

	cls := findChunk(chunks, "class")
	if cls == nil {
		t.Fatal("expected class chunk")
	}
	// The full class is 212 lines. Summarized output should be significantly
	// shorter than the full source: declaration (1) + signatures (210) + brace (1).
	// Crucially, each line must NOT contain the function body " { return N; }".
	if strings.Contains(cls.Content, "{ return 0; }") {
		t.Errorf("large class chunk should be summarized (no method bodies), but found full source")
	}
	if !strings.Contains(cls.Content, "class BigClass") {
		t.Errorf("should contain class declaration")
	}
	if !strings.Contains(cls.Content, "method0()") {
		t.Errorf("should contain method signatures")
	}
}

// ---------------------------------------------------------------------------
// No-overlap
// ---------------------------------------------------------------------------

func TestExtractChunks_NoOverlap_MethodsNotSeparateChunks(t *testing.T) {
	content := `class Foo {
  bar() { return 1; }
  baz() { return 2; }
}
`
	symbols := []parser.Symbol{
		sym("Foo", "class", 1, 4),
		symWithParent("bar", "method", "sym_Foo", 2, 2),
		symWithParent("baz", "method", "sym_Foo", 3, 3),
	}
	chunks := ExtractChunks(content, "foo.js", symbols, nil, "javascript")

	fns := findChunks(chunks, "function")
	if len(fns) != 0 {
		t.Errorf("methods inside class should not produce function chunks, got %d", len(fns))
	}
	cls := findChunks(chunks, "class")
	if len(cls) != 1 {
		t.Errorf("expected 1 class chunk, got %d", len(cls))
	}
}

// ---------------------------------------------------------------------------
// Ordering
// ---------------------------------------------------------------------------

func TestExtractChunks_SortOrder(t *testing.T) {
	content := `package main

import "fmt"

type Server struct {
	port int
}

func main() {
	fmt.Println("start")
}
`
	symbols := []parser.Symbol{
		sym("Server", "class", 5, 7),
		sym("main", "function", 9, 11),
	}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
	// Order: module_context → class → function
	if chunks[0].ChunkType != "module_context" {
		t.Errorf("chunks[0].ChunkType = %q, want module_context", chunks[0].ChunkType)
	}
	if chunks[1].ChunkType != "class" {
		t.Errorf("chunks[1].ChunkType = %q, want class", chunks[1].ChunkType)
	}
	if chunks[2].ChunkType != "function" {
		t.Errorf("chunks[2].ChunkType = %q, want function", chunks[2].ChunkType)
	}
}

// ---------------------------------------------------------------------------
// Chunk ID & hash
// ---------------------------------------------------------------------------

func TestExtractChunks_ChunkID_Format(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	symbols := []parser.Symbol{sym("main", "function", 3, 3)}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	fn := findChunk(chunks, "function")
	if fn == nil {
		t.Fatal("expected function chunk")
	}
	expected := "main.go:function:3-3"
	if fn.ChunkID != expected {
		t.Errorf("ChunkID = %q, want %q", fn.ChunkID, expected)
	}
}

func TestExtractChunks_ChunkHash_Deterministic(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	symbols := []parser.Symbol{sym("main", "function", 3, 3)}

	c1 := ExtractChunks(content, "main.go", symbols, nil, "go")
	c2 := ExtractChunks(content, "main.go", symbols, nil, "go")

	if len(c1) == 0 || len(c2) == 0 {
		t.Fatal("expected chunks")
	}
	fn1 := findChunk(c1, "function")
	fn2 := findChunk(c2, "function")
	if fn1.ChunkHash != fn2.ChunkHash {
		t.Errorf("hashes differ: %q vs %q", fn1.ChunkHash, fn2.ChunkHash)
	}
	if fn1.ChunkHash == "" {
		t.Errorf("hash should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Token estimation
// ---------------------------------------------------------------------------

func TestExtractChunks_EstimatedTokens(t *testing.T) {
	content := "1234567890\n" // 11 chars → (11+3)/4 = 3
	chunks := ExtractChunks(content, "style.css", nil, nil, "css")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].EstimatedTokens != 3 {
		t.Errorf("EstimatedTokens = %d, want 3", chunks[0].EstimatedTokens)
	}
}

// ---------------------------------------------------------------------------
// Semantic roles
// ---------------------------------------------------------------------------

func TestExtractChunks_SemanticRole_Exported(t *testing.T) {
	content := "package main\n\nfunc Hello() {}\n"
	symbols := []parser.Symbol{symExported("Hello", "function", 3, 3)}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	fn := findChunk(chunks, "function")
	if fn == nil {
		t.Fatal("expected function chunk")
	}
	if fn.SemanticRole != "api_surface" {
		t.Errorf("SemanticRole = %q, want api_surface", fn.SemanticRole)
	}
	if !fn.IsExportedContext {
		t.Errorf("IsExportedContext should be true")
	}
}

func TestExtractChunks_SemanticRole_ReactComponent(t *testing.T) {
	content := "import React from 'react';\n\nfunction App() { return <div/>; }\n"
	symbols := []parser.Symbol{symReactComponent("App", 3, 3)}
	chunks := ExtractChunks(content, "App.jsx", symbols, nil, "jsx")

	fn := findChunk(chunks, "function")
	if fn == nil {
		t.Fatal("expected function chunk")
	}
	if fn.SemanticRole != "ui_component" {
		t.Errorf("SemanticRole = %q, want ui_component", fn.SemanticRole)
	}
}

func TestExtractChunks_SemanticRole_Hook(t *testing.T) {
	content := "import { useState } from 'react';\n\nfunction useCounter() { return useState(0); }\n"
	symbols := []parser.Symbol{symHook("useCounter", 3, 3)}
	chunks := ExtractChunks(content, "hooks.js", symbols, nil, "javascript")

	fn := findChunk(chunks, "function")
	if fn == nil {
		t.Fatal("expected function chunk")
	}
	if fn.SemanticRole != "hook" {
		t.Errorf("SemanticRole = %q, want hook", fn.SemanticRole)
	}
}

// ---------------------------------------------------------------------------
// Context metadata
// ---------------------------------------------------------------------------

func TestExtractChunks_ContextBefore(t *testing.T) {
	content := "package main\n\nfunc greet() {}\n"
	symbols := []parser.Symbol{sym("greet", "function", 3, 3)}
	chunks := ExtractChunks(content, "main.go", symbols, nil, "go")

	fn := findChunk(chunks, "function")
	if fn == nil {
		t.Fatal("expected function chunk")
	}
	want := "File: main.go > function greet"
	if fn.ContextBefore != want {
		t.Errorf("ContextBefore = %q, want %q", fn.ContextBefore, want)
	}
}

// ---------------------------------------------------------------------------
// matchesFilePatterns
// ---------------------------------------------------------------------------

func TestMatchesFilePatterns(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		patterns []string
		want     bool
	}{
		{"exact match go.mod", "go.mod", []string{"go.mod"}, true},
		{"exact match deep path", "myproject/go.mod", []string{"go.mod"}, true},
		{"wildcard test Go", "foo_test.go", []string{"*_test.go"}, true},
		{"wildcard test JS", "app.test.js", []string{"*.test.*"}, true},
		{"wildcard spec JS", "app.spec.ts", []string{"*.spec.*"}, true},
		{"config pattern JS", "webpack.config.js", []string{"*.config.*"}, true},
		{"requirements glob", "requirements-dev.txt", []string{"requirements*.txt"}, true},
		{"Dockerfile.*", "Dockerfile.prod", []string{"Dockerfile.*"}, true},
		{"csproj wildcard", "MyApp.csproj", []string{"*.csproj"}, true},
		{"double star __tests__", "src/__tests__/foo.js", []string{"**/__tests__/**"}, true},
		{"double star tests py", "lib/tests/bar.py", []string{"**/tests/**/*.py"}, true},
		{"double star src/test java", "myapp/src/test/java/FooTest.java", []string{"**/src/test/**/*.java"}, true},
		{"double star wildcard dir C#", "proj/MyTestFixtures/Foo.cs", []string{"**/*Test*/**/*.cs"}, true},
		{"double star wildcard dir Swift", "proj/FooTests/BarTests.swift", []string{"**/*Tests/**/*.swift"}, true},
		{"double star wildcard dir no match", "proj/src/Foo.cs", []string{"**/*Test*/**/*.cs"}, false},
		{"double star dir glob must not match filename", "FooTest.cs", []string{"**/*Test*/**/*.cs"}, false},
		{"double star no match", "src/main.go", []string{"**/__tests__/**"}, false},
		{"double star ext mismatch", "lib/tests/bar.js", []string{"**/tests/**/*.py"}, false},
		{"no match", "main.go", []string{"*_test.go"}, false},
		{"empty patterns", "main.go", nil, false},
		{"empty patterns slice", "main.go", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilePatterns(tt.filePath, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesFilePatterns(%q, %v) = %v, want %v",
					tt.filePath, tt.patterns, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// estimateTokens
// ---------------------------------------------------------------------------

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text string
		want int32
	}{
		{"", 0},
		{"a", 1},
		{"ab", 1},
		{"abc", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"12345678", 2},
		{"123456789012", 3},
	}
	for _, tt := range tests {
		got := estimateTokens(tt.text)
		if got != tt.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// findDocCommentStart
// ---------------------------------------------------------------------------

func TestFindDocCommentStart(t *testing.T) {
	content := `package main

// greet says hello.
// It is friendly.
func greet() {}
`
	// Symbol starts at line 5.
	got := findDocCommentStart(content, 5)
	if got != 3 {
		t.Errorf("findDocCommentStart = %d, want 3", got)
	}
}

func TestFindDocCommentStart_NoComment(t *testing.T) {
	content := `package main

func greet() {}
`
	got := findDocCommentStart(content, 3)
	if got != 3 {
		t.Errorf("findDocCommentStart = %d, want 3 (no comment)", got)
	}
}

func TestFindDocCommentStart_JSDoc(t *testing.T) {
	content := `import foo from 'bar';

/**
 * Says hello.
 */
function greet() {}
`
	got := findDocCommentStart(content, 6)
	if got != 3 {
		t.Errorf("findDocCommentStart = %d, want 3", got)
	}
}

func TestFindDocCommentStart_AtLine1(t *testing.T) {
	content := `func greet() {}
`
	got := findDocCommentStart(content, 1)
	if got != 1 {
		t.Errorf("findDocCommentStart = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Python config / test patterns
// ---------------------------------------------------------------------------

func TestExtractChunks_Python_TestFile(t *testing.T) {
	content := `import pytest

def test_add():
    assert 1 + 2 == 3

def test_sub():
    assert 3 - 1 == 2
`
	chunks := ExtractChunks(content, "test_math.py", nil, nil, "python")
	testChunks := findChunks(chunks, "test")
	if len(testChunks) != 2 {
		t.Fatalf("expected 2 test chunks for Python, got %d", len(testChunks))
	}
}

func TestExtractChunks_Python_Config(t *testing.T) {
	content := `[tool.poetry]
name = "myapp"
version = "0.1.0"
`
	chunks := ExtractChunks(content, "pyproject.toml", nil, nil, "python")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ChunkType != "config" {
		t.Errorf("ChunkType = %q, want config", chunks[0].ChunkType)
	}
}

// ---------------------------------------------------------------------------
// Rust test file pattern
// ---------------------------------------------------------------------------

func TestExtractChunks_Rust_TestFile(t *testing.T) {
	content := `#[cfg(test)]
mod tests {
    #[test]
    fn test_add() {
        assert_eq!(1 + 2, 3);
    }

    #[test]
    fn test_sub() {
        assert_eq!(3 - 1, 2);
    }
}
`
	chunks := ExtractChunks(content, "src/tests/math.rs", nil, nil, "rust")
	testChunks := findChunks(chunks, "test")
	if len(testChunks) < 2 {
		t.Fatalf("expected at least 2 test chunks for Rust, got %d", len(testChunks))
	}
}

// ---------------------------------------------------------------------------
// Java config / test
// ---------------------------------------------------------------------------

func TestExtractChunks_Java_Config(t *testing.T) {
	content := `<project>
  <groupId>com.example</groupId>
</project>
`
	chunks := ExtractChunks(content, "pom.xml", nil, nil, "java")
	if len(chunks) != 1 || chunks[0].ChunkType != "config" {
		t.Errorf("expected single config chunk for pom.xml")
	}
}

func TestExtractChunks_Java_TestFile(t *testing.T) {
	content := `import org.junit.Test;

public class FooTest {
    @Test
    public void testFoo() {
        assertEquals(1, 1);
    }

    @Test
    public void testBar() {
        assertEquals(2, 2);
    }
}
`
	chunks := ExtractChunks(content, "FooTest.java", nil, nil, "java")
	testChunks := findChunks(chunks, "test")
	if len(testChunks) != 2 {
		t.Fatalf("expected 2 test chunks for Java, got %d", len(testChunks))
	}
}

// ---------------------------------------------------------------------------
// C# test patterns
// ---------------------------------------------------------------------------

func TestExtractChunks_CSharp_TestFile(t *testing.T) {
	content := `using NUnit.Framework;

[TestFixture]
public class MathTests {
    [Test]
    public void TestAdd() {
        Assert.AreEqual(3, 1 + 2);
    }

    [Test]
    public void TestSub() {
        Assert.AreEqual(2, 3 - 1);
    }
}
`
	chunks := ExtractChunks(content, "MathTests.cs", nil, nil, "csharp")
	testChunks := findChunks(chunks, "test")
	if len(testChunks) < 2 {
		t.Fatalf("expected at least 2 test chunks for C#, got %d", len(testChunks))
	}
}

// ---------------------------------------------------------------------------
// Multiple classes and functions
// ---------------------------------------------------------------------------

func TestExtractChunks_MixedClassesAndFunctions(t *testing.T) {
	content := `import { something } from 'lib';

class Dog {
  bark() { return "woof"; }
}

function run() {
  return new Dog().bark();
}

class Cat {
  meow() { return "meow"; }
}
`
	symbols := []parser.Symbol{
		sym("Dog", "class", 3, 5),
		symWithParent("bark", "method", "sym_Dog", 4, 4),
		sym("run", "function", 7, 9),
		sym("Cat", "class", 11, 13),
		symWithParent("meow", "method", "sym_Cat", 12, 12),
	}
	chunks := ExtractChunks(content, "animals.js", symbols, nil, "javascript")

	classes := findChunks(chunks, "class")
	fns := findChunks(chunks, "function")
	mc := findChunk(chunks, "module_context")

	if mc == nil {
		t.Error("expected module_context chunk")
	}
	if len(classes) != 2 {
		t.Errorf("expected 2 class chunks, got %d", len(classes))
	}
	if len(fns) != 1 {
		t.Errorf("expected 1 function chunk, got %d", len(fns))
	}
}
