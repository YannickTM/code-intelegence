package parser

import (
	"testing"
)

func TestStableHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "hello",
			input: "hello",
			want:  "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		},
		{
			name:  "empty string",
			input: "",
			want:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:  "unicode",
			input: "こんにちは",
			want:  "125aeadf27b0459b8760c13a3d80912dfa8a81a68261906f60d87f4a0268646c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StableHash(tt.input)
			if got != tt.want {
				t.Errorf("StableHash(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestStableHashBytes(t *testing.T) {
	// StableHashBytes must produce the same result as StableHash for the same UTF-8 content.
	inputs := []string{"hello", "", "こんにちは", "func main() { }"}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			fromString := StableHash(input)
			fromBytes := StableHashBytes([]byte(input))
			if fromString != fromBytes {
				t.Errorf("StableHash(%q) = %s, StableHashBytes = %s", input, fromString, fromBytes)
			}
		})
	}
}

func TestCrossLanguageHash(t *testing.T) {
	// Node.js: crypto.createHash('sha256').update('hello', 'utf8').digest('hex')
	got := StableHash("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("cross-language hash mismatch: got %s, want %s", got, want)
	}
}

func TestSortSymbols(t *testing.T) {
	s := []Symbol{
		{Name: "beta", StartLine: 10},
		{Name: "alpha", StartLine: 5},
		{Name: "gamma", StartLine: 5},
		{Name: "delta", StartLine: 1},
	}
	SortSymbols(s)
	want := []string{"delta", "alpha", "gamma", "beta"}
	for i, w := range want {
		if s[i].Name != w {
			t.Errorf("SortSymbols[%d].Name = %s, want %s", i, s[i].Name, w)
		}
	}
}

func TestSortExports(t *testing.T) {
	s := []Export{
		{ExportedName: "C", Line: 5, Column: 10},
		{ExportedName: "A", Line: 1, Column: 1},
		{ExportedName: "B", Line: 5, Column: 1},
		{ExportedName: "D", Line: 5, Column: 10},
	}
	SortExports(s)
	want := []string{"A", "B", "C", "D"}
	for i, w := range want {
		if s[i].ExportedName != w {
			t.Errorf("SortExports[%d].ExportedName = %s, want %s", i, s[i].ExportedName, w)
		}
	}
}

func TestSortReferences(t *testing.T) {
	s := []Reference{
		{TargetName: "z", StartLine: 10, StartColumn: 1, ReferenceKind: "CALL"},
		{TargetName: "a", StartLine: 1, StartColumn: 5, ReferenceKind: "TYPE_REF"},
		{TargetName: "b", StartLine: 1, StartColumn: 5, ReferenceKind: "CALL"},
		{TargetName: "c", StartLine: 1, StartColumn: 1, ReferenceKind: "CALL"},
	}
	SortReferences(s)
	want := []string{"c", "b", "a", "z"}
	for i, w := range want {
		if s[i].TargetName != w {
			t.Errorf("SortReferences[%d].TargetName = %s, want %s", i, s[i].TargetName, w)
		}
	}
}

func TestSortJsxUsages(t *testing.T) {
	s := []JsxUsage{
		{ComponentName: "Footer", Line: 20, Column: 5},
		{ComponentName: "Header", Line: 1, Column: 1},
		{ComponentName: "Button", Line: 10, Column: 3},
		{ComponentName: "Alert", Line: 10, Column: 3},
	}
	SortJsxUsages(s)
	want := []string{"Header", "Alert", "Button", "Footer"}
	for i, w := range want {
		if s[i].ComponentName != w {
			t.Errorf("SortJsxUsages[%d].ComponentName = %s, want %s", i, s[i].ComponentName, w)
		}
	}
}

func TestSortNetworkCalls(t *testing.T) {
	s := []NetworkCall{
		{URLLiteral: "/c", StartLine: 30, StartColumn: 1},
		{URLLiteral: "/a", StartLine: 1, StartColumn: 10},
		{URLLiteral: "/b", StartLine: 1, StartColumn: 1},
	}
	SortNetworkCalls(s)
	want := []string{"/b", "/a", "/c"}
	for i, w := range want {
		if s[i].URLLiteral != w {
			t.Errorf("SortNetworkCalls[%d].URLLiteral = %s, want %s", i, s[i].URLLiteral, w)
		}
	}
}

func TestSortChunks(t *testing.T) {
	s := []Chunk{
		{ChunkType: "function", StartLine: 5, Content: "fn1"},
		{ChunkType: "class", StartLine: 20, Content: "cls2"},
		{ChunkType: "module_context", StartLine: 1, Content: "mod"},
		{ChunkType: "function", StartLine: 50, Content: "fn2"},
		{ChunkType: "class", StartLine: 10, Content: "cls1"},
		{ChunkType: "config", StartLine: 2, Content: "cfg"},
	}
	SortChunks(s)
	wantContent := []string{"mod", "cls1", "cls2", "fn1", "fn2", "cfg"}
	for i, w := range wantContent {
		if s[i].Content != w {
			t.Errorf("SortChunks[%d].Content = %s, want %s", i, s[i].Content, w)
		}
	}
}

func TestSortIssues(t *testing.T) {
	s := []Issue{
		{Code: "E002", Line: 10, Column: 1},
		{Code: "E001", Line: 1, Column: 5},
		{Code: "W001", Line: 1, Column: 5},
		{Code: "E003", Line: 1, Column: 1},
	}
	SortIssues(s)
	want := []string{"E003", "E001", "W001", "E002"}
	for i, w := range want {
		if s[i].Code != w {
			t.Errorf("SortIssues[%d].Code = %s, want %s", i, s[i].Code, w)
		}
	}
}

func TestDeduplicateImports(t *testing.T) {
	s := []Import{
		{ImportName: "A", TargetFilePath: "react"},
		{ImportName: "B", TargetFilePath: "lodash"},
		{ImportName: "C", TargetFilePath: "react"},
		{ImportName: "D", TargetFilePath: "vue"},
		{ImportName: "E", TargetFilePath: "lodash"},
	}
	got := DeduplicateImports(s)
	if len(got) != 3 {
		t.Fatalf("DeduplicateImports: got %d imports, want 3", len(got))
	}
	wantTargets := []string{"react", "lodash", "vue"}
	wantNames := []string{"A", "B", "D"}
	for i := range got {
		if got[i].TargetFilePath != wantTargets[i] {
			t.Errorf("DeduplicateImports[%d].TargetFilePath = %s, want %s", i, got[i].TargetFilePath, wantTargets[i])
		}
		if got[i].ImportName != wantNames[i] {
			t.Errorf("DeduplicateImports[%d].ImportName = %s, want %s (first occurrence preserved)", i, got[i].ImportName, wantNames[i])
		}
	}
}

func TestDeduplicateImportsEmpty(t *testing.T) {
	got := DeduplicateImports(nil)
	if len(got) != 0 {
		t.Errorf("DeduplicateImports(nil): got %d imports, want 0", len(got))
	}
}

func TestSortStability(t *testing.T) {
	// Run sort 100 times and verify identical output each time.
	base := []Symbol{
		{Name: "a", StartLine: 1},
		{Name: "b", StartLine: 1},
		{Name: "c", StartLine: 2},
		{Name: "d", StartLine: 1},
	}
	for i := 0; i < 100; i++ {
		s := make([]Symbol, len(base))
		copy(s, base)
		SortSymbols(s)
		want := []string{"a", "b", "d", "c"}
		for j, w := range want {
			if s[j].Name != w {
				t.Fatalf("iteration %d: SortSymbols[%d].Name = %s, want %s", i, j, s[j].Name, w)
			}
		}
	}
}
