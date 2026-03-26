package extractors

import (
	"myjungle/backend-worker/internal/parser"
)

// ExtractFileMeta computes the SHA-256 hash, line count, and byte size
// of the (already-normalized) content string.
func ExtractFileMeta(content string) (fileHash string, lineCount int32, sizeBytes int64) {
	fileHash = parser.StableHash(content)
	lineCount = parser.CountLines(content)
	sizeBytes = int64(len(content))
	return
}
