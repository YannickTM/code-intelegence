package parser

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
)

// StableHash returns the lowercase hex SHA-256 of the input string.
func StableHash(content string) string {
	return StableHashBytes([]byte(content))
}

// StableHashBytes returns the lowercase hex SHA-256 of the input bytes.
func StableHashBytes(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// SortSymbols sorts symbols by StartLine, then Name.
func SortSymbols(s []Symbol) {
	slices.SortStableFunc(s, func(a, b Symbol) int {
		if c := cmp.Compare(a.StartLine, b.StartLine); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

// SortExports sorts exports by Line, Column, then ExportedName.
func SortExports(s []Export) {
	slices.SortStableFunc(s, func(a, b Export) int {
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Column, b.Column); c != 0 {
			return c
		}
		return cmp.Compare(a.ExportedName, b.ExportedName)
	})
}

// SortReferences sorts references by StartLine, StartColumn, ReferenceKind, then TargetName.
func SortReferences(s []Reference) {
	slices.SortStableFunc(s, func(a, b Reference) int {
		if c := cmp.Compare(a.StartLine, b.StartLine); c != 0 {
			return c
		}
		if c := cmp.Compare(a.StartColumn, b.StartColumn); c != 0 {
			return c
		}
		if c := cmp.Compare(a.ReferenceKind, b.ReferenceKind); c != 0 {
			return c
		}
		return cmp.Compare(a.TargetName, b.TargetName)
	})
}

// SortJsxUsages sorts JSX usages by Line, Column, then ComponentName.
func SortJsxUsages(s []JsxUsage) {
	slices.SortStableFunc(s, func(a, b JsxUsage) int {
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Column, b.Column); c != 0 {
			return c
		}
		return cmp.Compare(a.ComponentName, b.ComponentName)
	})
}

// SortNetworkCalls sorts network calls by StartLine, then StartColumn.
func SortNetworkCalls(s []NetworkCall) {
	slices.SortStableFunc(s, func(a, b NetworkCall) int {
		if c := cmp.Compare(a.StartLine, b.StartLine); c != 0 {
			return c
		}
		return cmp.Compare(a.StartColumn, b.StartColumn)
	})
}

// chunkTypePriority returns the sort priority for a chunk type.
// MODULE_CONTEXT < CLASS < FUNCTION < others.
func chunkTypePriority(ct string) int {
	switch ct {
	case "module_context":
		return 0
	case "class":
		return 1
	case "function":
		return 2
	default:
		return 3
	}
}

// SortChunks sorts chunks by type priority (module_context → class → function → others),
// then by StartLine within each type.
func SortChunks(s []Chunk) {
	slices.SortStableFunc(s, func(a, b Chunk) int {
		if c := cmp.Compare(chunkTypePriority(a.ChunkType), chunkTypePriority(b.ChunkType)); c != 0 {
			return c
		}
		return cmp.Compare(a.StartLine, b.StartLine)
	})
}

// SortIssues sorts issues by Line, Column, then Code.
func SortIssues(s []Issue) {
	slices.SortStableFunc(s, func(a, b Issue) int {
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Column, b.Column); c != 0 {
			return c
		}
		return cmp.Compare(a.Code, b.Code)
	})
}

// DeduplicateImports returns unique imports, preserving first-occurrence order.
// For INTERNAL imports, uniqueness is by TargetFilePath (resolved path).
// For STDLIB/EXTERNAL imports, uniqueness is by ImportName (since TargetFilePath
// is empty and would collapse all entries to one).
// Keys are namespaced by ImportType to prevent collisions across categories
// (e.g. INTERNAL "./react" vs EXTERNAL "react").
func DeduplicateImports(s []Import) []Import {
	seen := make(map[string]struct{}, len(s))
	out := make([]Import, 0, len(s))
	for _, imp := range s {
		key := imp.TargetFilePath
		if key == "" {
			key = imp.ImportName
		}
		key = imp.ImportType + ":" + key
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, imp)
	}
	return out
}
