package parser

import "strings"

// NormalizeNewlines converts CRLF and CR line endings to LF and strips the
// UTF-8 BOM (U+FEFF, encoded as 0xEF 0xBB 0xBF) if present.
func NormalizeNewlines(content string) string {
	// Strip UTF-8 BOM.
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")

	// CRLF -> LF first, then remaining CR -> LF.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	return content
}

// LineRangeText extracts lines [startLine, endLine] (1-indexed, inclusive).
// Out-of-range values are clamped. Returns "" for empty content or invalid
// ranges (startLine > endLine after clamping).
func LineRangeText(content string, startLine, endLine int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	// Remove trailing empty element if content ends with \n.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}

	// Clamp to valid range.
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return ""
	}

	return strings.Join(lines[startLine-1:endLine], "\n")
}

// CountLines returns the number of lines in the content. An empty string
// returns 0. A trailing newline does not add an extra line (e.g. "a\nb\n"
// returns 2), matching common editor line-count behavior.
func CountLines(content string) int32 {
	if content == "" {
		return 0
	}
	n := int32(strings.Count(content, "\n"))
	if !strings.HasSuffix(content, "\n") {
		n++
	}
	return n
}
