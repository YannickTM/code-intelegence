package extractors

import (
	"testing"

	"myjungle/backend-worker/internal/parser"
)

func TestExtractFileMeta_Normal(t *testing.T) {
	content := "line1\nline2\nline3\n"
	hash, lines, size := ExtractFileMeta(content)

	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if lines != 3 {
		t.Errorf("lineCount = %d, want 3", lines)
	}
	if size != int64(len(content)) {
		t.Errorf("sizeBytes = %d, want %d", size, len(content))
	}
}

func TestExtractFileMeta_Empty(t *testing.T) {
	hash, lines, size := ExtractFileMeta("")

	if hash == "" {
		t.Error("expected non-empty hash for empty string")
	}
	if lines != 0 {
		t.Errorf("lineCount = %d, want 0", lines)
	}
	if size != 0 {
		t.Errorf("sizeBytes = %d, want 0", size)
	}
}

func TestExtractFileMeta_SingleLineNoNewline(t *testing.T) {
	content := "hello world"
	_, lines, _ := ExtractFileMeta(content)
	if lines != 1 {
		t.Errorf("lineCount = %d, want 1", lines)
	}
}

func TestExtractFileMeta_TrailingNewline(t *testing.T) {
	content := "a\nb\n"
	_, lines, _ := ExtractFileMeta(content)
	if lines != 2 {
		t.Errorf("lineCount = %d, want 2 (trailing newline shouldn't add extra line)", lines)
	}
}

func TestExtractFileMeta_DeterministicHash(t *testing.T) {
	content := "const x = 42;\n"
	hash1, _, _ := ExtractFileMeta(content)
	hash2, _, _ := ExtractFileMeta(content)
	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash1, hash2)
	}

	// Also verify it matches parser.StableHash directly.
	want := parser.StableHash(content)
	if hash1 != want {
		t.Errorf("hash = %q, want %q (from StableHash)", hash1, want)
	}
}

func TestExtractFileMeta_SizeBytes(t *testing.T) {
	// Unicode content: each emoji is multi-byte.
	content := "hello 🌍\n"
	_, _, size := ExtractFileMeta(content)
	if size != int64(len(content)) {
		t.Errorf("sizeBytes = %d, want %d (byte length, not rune count)", size, len(content))
	}
}
