package extractors

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// ExtractChunks generates embedding-ready code chunks from parsed files.
// It splits by logical boundaries (functions, classes, module context) with
// metadata for semantic search. Unlike other extractors this operates on
// pre-extracted symbols and text content, not the AST directly.
func ExtractChunks(content string, filePath string, symbols []parser.Symbol, _ []parser.Import, langID string) []parser.Chunk {
	if content == "" || langID == "" {
		return nil
	}

	langConfig, ok := registry.GetLanguageConfig(langID)
	if !ok {
		return nil
	}

	totalLines := parser.CountLines(content)

	// Config files → single CONFIG chunk (highest priority).
	if matchesFilePatterns(filePath, langConfig.ConfigFilePatterns) {
		return []parser.Chunk{makeChunk(
			filePath, "config", content, 1, totalLines,
			"", "", "", "config", "", "", false,
		)}
	}

	// Tier 3 → single MODULE_CONTEXT chunk.
	if langConfig.Tier == registry.Tier3 {
		return []parser.Chunk{makeChunk(
			filePath, "module_context", content, 1, totalLines,
			"", "", "", "implementation", "", "", false,
		)}
	}

	// Test files → per-block or full-file TEST chunk.
	if matchesFilePatterns(filePath, langConfig.TestFilePatterns) {
		return extractTestChunks(content, filePath, langConfig.TestBlockPatterns, totalLines)
	}

	// Tier 1/2 symbol-based chunking.
	return extractSymbolChunks(content, filePath, symbols, langConfig.Tier, totalLines)
}

// extractTestChunks splits test files by test block patterns, falling back to
// a single full-file TEST chunk when no patterns match.
func extractTestChunks(content, filePath string, blockPatterns []string, totalLines int32) []parser.Chunk {
	if len(blockPatterns) > 0 {
		chunks := splitTestBlocks(content, blockPatterns, filePath)
		if len(chunks) > 0 {
			parser.SortChunks(chunks)
			return chunks
		}
	}
	// Fallback: single full-file TEST chunk.
	return []parser.Chunk{makeChunk(
		filePath, "test", content, 1, totalLines,
		"", "", "", "test", "", "", false,
	)}
}

// effectiveFunc holds a top-level function symbol with its effective start
// line (extended upward for doc comments).
type effectiveFunc struct {
	sym       parser.Symbol
	startLine int32
}

// extractSymbolChunks produces MODULE_CONTEXT, CLASS, and FUNCTION chunks
// for Tier 1/2 non-config, non-test files.
func extractSymbolChunks(content, filePath string, symbols []parser.Symbol, tier registry.Tier, totalLines int32) []parser.Chunk {
	// Classify top-level classes and build a set of their IDs.
	var topClasses []parser.Symbol
	classIDs := make(map[string]bool)
	for _, sym := range symbols {
		if sym.ParentSymbolID == "" && isClassLike(sym.Kind) {
			topClasses = append(topClasses, sym)
			classIDs[sym.SymbolID] = true
		}
	}

	// Collect methods per class for large-class summarisation.
	classMethods := make(map[string][]parser.Symbol)
	for _, sym := range symbols {
		if sym.ParentSymbolID != "" && classIDs[sym.ParentSymbolID] {
			classMethods[sym.ParentSymbolID] = append(classMethods[sym.ParentSymbolID], sym)
		}
	}

	// Top-level functions (not methods, not inside any class range).
	var topFunctions []parser.Symbol
	for _, sym := range symbols {
		if sym.ParentSymbolID != "" || sym.Kind != "function" {
			continue
		}
		if insideAnyClass(sym, topClasses) {
			continue
		}
		topFunctions = append(topFunctions, sym)
	}

	// Orphan methods: functions/methods with a parent but defined outside any
	// class body. This covers Go receiver methods (Kind="method") and Rust
	// impl block methods (Kind="function") whose code isn't inside the
	// struct/enum line range.
	for _, sym := range symbols {
		if sym.ParentSymbolID == "" {
			continue
		}
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		if insideAnyClass(sym, topClasses) {
			continue
		}
		topFunctions = append(topFunctions, sym)
	}

	// Pre-compute effective start lines for functions (doc comment extension).
	effFunctions := make([]effectiveFunc, len(topFunctions))
	for i, fn := range topFunctions {
		start := fn.StartLine
		if fn.DocText != "" {
			if ds := findDocCommentStart(content, fn.StartLine); ds < start {
				start = ds
			}
		}
		effFunctions[i] = effectiveFunc{sym: fn, startLine: start}
	}

	// Determine first effective symbol line (classes use their StartLine directly).
	firstLine := int32(0)
	for _, cls := range topClasses {
		if firstLine == 0 || cls.StartLine < firstLine {
			firstLine = cls.StartLine
		}
	}
	for _, ef := range effFunctions {
		if firstLine == 0 || ef.startLine < firstLine {
			firstLine = ef.startLine
		}
	}

	var chunks []parser.Chunk

	// MODULE_CONTEXT: line 1 to line before first symbol.
	if firstLine > 1 {
		mcEnd := firstLine - 1
		mcContent := parser.LineRangeText(content, 1, int(mcEnd))
		if strings.TrimSpace(mcContent) != "" {
			chunks = append(chunks, makeChunk(
				filePath, "module_context", mcContent, 1, mcEnd,
				"", "", "", "implementation", "", "", false,
			))
		}
	} else if firstLine == 0 && totalLines > 0 {
		// No top-level symbols → entire file is module context.
		chunks = append(chunks, makeChunk(
			filePath, "module_context", content, 1, totalLines,
			"", "", "", "implementation", "", "", false,
		))
	}

	// CLASS chunks.
	for _, cls := range topClasses {
		classLines := cls.EndLine - cls.StartLine + 1
		var chunkContent string
		if classLines <= 200 {
			chunkContent = parser.LineRangeText(content, int(cls.StartLine), int(cls.EndLine))
		} else {
			chunkContent = summarizeLargeClass(content, cls, classMethods[cls.SymbolID])
		}
		isExp := cls.Flags != nil && cls.Flags.IsExported
		role := determineSemanticRole(&cls, false, false, tier)
		ctx := fmt.Sprintf("File: %s > %s %s", filePath, cls.Kind, cls.Name)
		chunks = append(chunks, makeChunk(
			filePath, "class", chunkContent, cls.StartLine, cls.EndLine,
			cls.SymbolID, cls.QualifiedName, cls.Kind, role, ctx, "", isExp,
		))
	}

	// FUNCTION chunks.
	for _, ef := range effFunctions {
		fn := ef.sym
		fnContent := parser.LineRangeText(content, int(ef.startLine), int(fn.EndLine))
		isExp := fn.Flags != nil && fn.Flags.IsExported
		role := determineSemanticRole(&fn, false, false, tier)
		ctx := fmt.Sprintf("File: %s > %s %s", filePath, fn.Kind, fn.Name)
		chunks = append(chunks, makeChunk(
			filePath, "function", fnContent, ef.startLine, fn.EndLine,
			fn.SymbolID, fn.QualifiedName, fn.Kind, role, ctx, "", isExp,
		))
	}

	parser.SortChunks(chunks)
	return chunks
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// matchesFilePatterns checks whether filePath matches any of the provided
// glob patterns. Patterns may be simple globs matched against the basename
// (e.g. "*.test.*", "go.mod") or recursive directory patterns using "**/"
// (e.g. "**/__tests__/**", "**/tests/**/*.py").
func matchesFilePatterns(filePath string, patterns []string) bool {
	base := filepath.Base(filePath)
	normalized := filepath.ToSlash(filePath)

	for _, pattern := range patterns {
		if strings.Contains(pattern, "**/") {
			if matchesDoubleStarPattern(normalized, base, pattern) {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}
	return false
}

// matchesDoubleStarPattern handles patterns containing "**/" such as
// "**/__tests__/**", "**/tests/**/*.py", or "**/*Test*/**/*.cs".
// The directory segment may itself be a glob (e.g. "*Test*").
func matchesDoubleStarPattern(normalizedPath, base, pattern string) bool {
	trimmed := strings.TrimPrefix(pattern, "**/")
	parts := strings.SplitN(trimmed, "/**", 2)

	dirGlob := parts[0] // e.g. "__tests__", "src/test", "*Test*"

	// Check that dirGlob matches one or more consecutive path components.
	if !matchesDirGlob(normalizedPath, dirGlob) {
		return false
	}

	// If there is a trailing file glob after /**, match it against the basename.
	if len(parts) > 1 {
		fileGlob := strings.TrimPrefix(parts[1], "/")
		if fileGlob != "" {
			if matched, _ := filepath.Match(fileGlob, base); !matched {
				return false
			}
		}
	}
	return true
}

// matchesDirGlob checks whether dirGlob (which may contain slashes for
// multi-segment patterns like "src/test" and globs like "*Test*") matches
// a contiguous run of path components in normalizedPath.
func matchesDirGlob(normalizedPath, dirGlob string) bool {
	pathSegments := strings.Split(normalizedPath, "/")
	globSegments := strings.Split(dirGlob, "/")

	// Exclude the last segment (filename) — we only match directories.
	dirSegments := pathSegments[:len(pathSegments)-1]

	if len(globSegments) > len(dirSegments) {
		return false
	}

	// Slide a window of len(globSegments) across dirSegments.
	for i := 0; i <= len(dirSegments)-len(globSegments); i++ {
		allMatch := true
		for j, gs := range globSegments {
			if matched, _ := filepath.Match(gs, dirSegments[i+j]); !matched {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

// estimateTokens returns a rough token count (~1 token per 4 characters).
func estimateTokens(text string) int32 {
	return int32((len(text) + 3) / 4)
}

// determineSemanticRole assigns a semantic role string based on file/symbol context.
func determineSemanticRole(sym *parser.Symbol, isConfig, isTest bool, tier registry.Tier) string {
	if isConfig {
		return "config"
	}
	if isTest {
		return "test"
	}
	if sym != nil && sym.Flags != nil {
		if sym.Flags.IsReactComponentLike {
			return "ui_component"
		}
		if sym.Flags.IsHookLike {
			return "hook"
		}
		if sym.Flags.IsExported && tier == registry.Tier1 {
			return "api_surface"
		}
	}
	return "implementation"
}

// makeChunk builds a parser.Chunk with computed ID, hash, and token estimate.
func makeChunk(
	filePath, chunkType, content string,
	startLine, endLine int32,
	symbolID, ownerQN, ownerKind, role, ctxBefore, ctxAfter string,
	isExported bool,
) parser.Chunk {
	return parser.Chunk{
		ChunkID:            fmt.Sprintf("%s:%s:%d-%d", filePath, chunkType, startLine, endLine),
		SymbolID:           symbolID,
		ChunkType:          chunkType,
		ChunkHash:          parser.StableHash(content),
		Content:            content,
		ContextBefore:      ctxBefore,
		ContextAfter:       ctxAfter,
		StartLine:          startLine,
		EndLine:            endLine,
		EstimatedTokens:    estimateTokens(content),
		OwnerQualifiedName: ownerQN,
		OwnerKind:          ownerKind,
		IsExportedContext:  isExported,
		SemanticRole:       role,
	}
}

// findDocCommentStart scans backward from the line before symbolStartLine
// to find the first line of a leading doc comment block. Returns the
// 1-indexed start line (may equal symbolStartLine if no comment found).
func findDocCommentStart(content string, symbolStartLine int32) int32 {
	if symbolStartLine <= 1 {
		return symbolStartLine
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start := symbolStartLine
	for i := int(symbolStartLine) - 2; i >= 0; i-- { // 0-indexed
		line := strings.TrimSpace(lines[i])
		if line == "" {
			break
		}
		if isCommentLine(line) {
			start = int32(i + 1) // back to 1-indexed
		} else {
			break
		}
	}
	return start
}

// isCommentLine returns true if the trimmed line looks like a comment or
// annotation across common languages.
func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "*/") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, `"""`) ||
		strings.HasPrefix(line, `'''`) ||
		strings.HasPrefix(line, "@")
}

// splitTestBlocks splits a test file into individual test-block chunks
// using the provided regex patterns. Returns nil when no blocks are found.
func splitTestBlocks(content string, patterns []string, filePath string) []parser.Chunk {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	var regexps []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			regexps = append(regexps, re)
		}
	}
	if len(regexps) == 0 {
		return nil
	}

	var blockStarts []int32
	for i, line := range lines {
		for _, re := range regexps {
			if re.MatchString(line) {
				blockStarts = append(blockStarts, int32(i+1))
				break
			}
		}
	}
	if len(blockStarts) == 0 {
		return nil
	}

	totalLines := int32(len(lines))
	chunks := make([]parser.Chunk, 0, len(blockStarts))
	for i, startLine := range blockStarts {
		endLine := totalLines
		if i+1 < len(blockStarts) {
			endLine = blockStarts[i+1] - 1
		}
		blockContent := parser.LineRangeText(content, int(startLine), int(endLine))
		chunks = append(chunks, makeChunk(
			filePath, "test", blockContent, startLine, endLine,
			"", "", "", "test", "", "", false,
		))
	}
	return chunks
}

// summarizeLargeClass produces a summary for classes exceeding 200 lines:
// the declaration line, each method's signature, and the closing brace.
func summarizeLargeClass(content string, cls parser.Symbol, methods []parser.Symbol) string {
	var sb strings.Builder
	sb.WriteString(parser.LineRangeText(content, int(cls.StartLine), int(cls.StartLine)))
	sb.WriteByte('\n')
	for _, m := range methods {
		if m.Signature != "" {
			sb.WriteString("  ")
			sb.WriteString(m.Signature)
			sb.WriteByte('\n')
		} else {
			line := parser.LineRangeText(content, int(m.StartLine), int(m.StartLine))
			sb.WriteString("  ")
			sb.WriteString(strings.TrimSpace(line))
			sb.WriteByte('\n')
		}
	}
	sb.WriteString(parser.LineRangeText(content, int(cls.EndLine), int(cls.EndLine)))
	return sb.String()
}

// isClassLike returns true for symbol kinds that represent class-like containers.
func isClassLike(kind string) bool {
	return kind == "class" || kind == "interface" || kind == "enum"
}

// insideAnyClass checks whether sym's line range falls entirely within
// any of the given class symbols.
func insideAnyClass(sym parser.Symbol, classes []parser.Symbol) bool {
	for _, cls := range classes {
		if sym.StartLine >= cls.StartLine && sym.EndLine <= cls.EndLine {
			return true
		}
	}
	return false
}
