package parser

import "testing"

func TestNormalizeNewlines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"LF unchanged", "a\nb\nc", "a\nb\nc"},
		{"CRLF to LF", "a\r\nb\r\nc", "a\nb\nc"},
		{"CR to LF", "a\rb\rc", "a\nb\nc"},
		{"mixed endings", "a\r\nb\rc\nd", "a\nb\nc\nd"},
		{"BOM stripped", "\xef\xbb\xbfhello", "hello"},
		{"BOM with CRLF", "\xef\xbb\xbfa\r\nb", "a\nb"},
		{"empty string", "", ""},
		{"only BOM", "\xef\xbb\xbf", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeNewlines(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeNewlines(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLineRangeText(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive"

	tests := []struct {
		name      string
		content   string
		startLine int
		endLine   int
		want      string
	}{
		{"middle range", content, 2, 4, "two\nthree\nfour"},
		{"single line", content, 3, 3, "three"},
		{"first line", content, 1, 1, "one"},
		{"last line", content, 5, 5, "five"},
		{"full range", content, 1, 5, "one\ntwo\nthree\nfour\nfive"},
		{"clamp start below 1", content, -2, 2, "one\ntwo"},
		{"clamp end beyond count", content, 4, 100, "four\nfive"},
		{"both clamped", content, 0, 100, "one\ntwo\nthree\nfour\nfive"},
		{"invalid range after clamp", content, 10, 12, ""},
		{"start > end", content, 4, 2, ""},
		{"empty content", "", 1, 3, ""},
		{"trailing newline", "a\nb\n", 1, 2, "a\nb"},
		{"single line trailing newline", "hello\n", 1, 1, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LineRangeText(tt.content, tt.startLine, tt.endLine)
			if got != tt.want {
				t.Errorf("LineRangeText(%q, %d, %d) = %q, want %q",
					tt.content, tt.startLine, tt.endLine, got, tt.want)
			}
		})
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int32
	}{
		{"empty", "", 0},
		{"single line no newline", "hello", 1},
		{"single line with newline", "hello\n", 1},
		{"two lines", "a\nb", 2},
		{"two lines trailing newline", "a\nb\n", 2},
		{"three lines", "a\nb\nc", 3},
		{"only newline", "\n", 1},
		{"two newlines", "\n\n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountLines(tt.content)
			if got != tt.want {
				t.Errorf("CountLines(%q) = %d, want %d", tt.content, got, tt.want)
			}
		})
	}
}
