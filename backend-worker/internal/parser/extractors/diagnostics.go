package extractors

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// Diagnostic thresholds.
const (
	maxIssuesPerFile     = 50
	longFunctionLines    = 200
	longFileLines        = 1000
	deepNestingThreshold = 6
)

// ExtractDiagnostics collects parse errors, structural warnings, and
// parser-level issues from a parsed AST. Issues are deterministically sorted
// by line -> column -> code.
func ExtractDiagnostics(root *sitter.Node, content []byte, langID string, filePath string) []parser.Issue {
	if root == nil {
		return nil
	}

	langConfig, _ := registry.GetLanguageConfig(langID)

	var issues []parser.Issue

	// 1. Parse errors: ERROR and MISSING nodes.
	issues = collectParseErrors(root, issues)

	// 2. Structural warnings (require language config).
	if langConfig != nil {
		contentStr := string(content)
		lineCount := parser.CountLines(contentStr)

		// Long file.
		if lineCount > longFileLines {
			issues = append(issues, parser.Issue{
				Code:     "LONG_FILE",
				Message:  fmt.Sprintf("File has %d lines (threshold: %d)", lineCount, longFileLines),
				Line:     1,
				Column:   1,
				Severity: "warning",
			})
		}

		// Long functions.
		issues = collectLongFunctions(root, content, langConfig, issues)

		// Deep nesting.
		if len(langConfig.NestingNodeTypes) > 0 {
			nestingSet := toStringSet(langConfig.NestingNodeTypes)
			issues = collectDeepNesting(root, nestingSet, issues)
		}

		// No exports — only for languages that use the "export" keyword (JS/TS).
		// Never emitted for convention-based (Go, Python) or non-"export" keyword languages.
		if langConfig.Export.Type == "keyword" && langConfig.Export.Keyword == "export" && langConfig.Tier == registry.Tier1 {
			isTest := len(langConfig.TestFilePatterns) > 0 && filePath != "" &&
				matchesFilePatterns(filePath, langConfig.TestFilePatterns)
			if !isTest && !hasExportNodes(root) {
				issues = append(issues, parser.Issue{
					Code:     "NO_EXPORTS",
					Message:  "File has no exports",
					Line:     1,
					Column:   1,
					Severity: "info",
				})
			}
		}
	}

	// Cap at 50 issues.
	if len(issues) > maxIssuesPerFile {
		issues = issues[:maxIssuesPerFile]
	}

	parser.SortIssues(issues)
	return issues
}

// ---------------------------------------------------------------------------
// Parse error collection
// ---------------------------------------------------------------------------

// collectParseErrors walks the AST to find ERROR and MISSING nodes.
// Consecutive errors on the same line are merged into a single issue.
func collectParseErrors(node *sitter.Node, issues []parser.Issue) []parser.Issue {
	if node == nil {
		return issues
	}

	var raw []parser.Issue
	walkParseErrors(node, &raw, 0)

	// Merge consecutive same-line errors.
	return mergeParseErrors(raw, issues)
}

func walkParseErrors(node *sitter.Node, out *[]parser.Issue, depth int) {
	if node == nil || depth > 500 {
		return
	}

	if node.IsError() {
		line := int32(node.StartPoint().Row) + 1
		col := int32(node.StartPoint().Column) + 1
		*out = append(*out, parser.Issue{
			Code:     "PARSE_ERROR",
			Message:  "Parse error",
			Line:     line,
			Column:   col,
			Severity: "error",
		})
		// Don't recurse into ERROR node children — they are part of the error.
		return
	}

	if node.IsMissing() {
		line := int32(node.StartPoint().Row) + 1
		col := int32(node.StartPoint().Column) + 1
		*out = append(*out, parser.Issue{
			Code:     "MISSING_NODE",
			Message:  fmt.Sprintf("Missing expected node: %s", node.Type()),
			Line:     line,
			Column:   col,
			Severity: "error",
		})
		return
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkParseErrors(node.Child(i), out, depth+1)
	}
}

// mergeParseErrors merges consecutive errors on the same line into a single
// "Multiple parse errors on line N" issue.
func mergeParseErrors(raw []parser.Issue, into []parser.Issue) []parser.Issue {
	if len(raw) == 0 {
		return into
	}

	type lineGroup struct {
		line  int32
		first parser.Issue
		count int
	}
	var groups []lineGroup

	for _, iss := range raw {
		if len(groups) > 0 && groups[len(groups)-1].line == iss.Line {
			groups[len(groups)-1].count++
		} else {
			groups = append(groups, lineGroup{line: iss.Line, first: iss, count: 1})
		}
	}

	for _, g := range groups {
		if g.count == 1 {
			into = append(into, g.first)
		} else {
			into = append(into, parser.Issue{
				Code:     "PARSE_ERROR",
				Message:  fmt.Sprintf("Multiple parse errors on line %d", g.line),
				Line:     g.line,
				Column:   g.first.Column,
				Severity: "error",
			})
		}
	}
	return into
}

// ---------------------------------------------------------------------------
// Structural warnings
// ---------------------------------------------------------------------------

// collectLongFunctions walks the AST to find function/method nodes that
// exceed the longFunctionLines threshold.
func collectLongFunctions(node *sitter.Node, content []byte, cfg *registry.LanguageConfig, issues []parser.Issue) []parser.Issue {
	if node == nil {
		return issues
	}

	funcTypes := make(map[string]bool)
	for nodeType, kind := range cfg.SymbolNodeTypes {
		if kind == "function" || kind == "method" {
			funcTypes[nodeType] = true
		}
	}
	if len(funcTypes) == 0 {
		return issues
	}

	walkLongFunctions(node, content, funcTypes, &issues, 0)
	return issues
}

func walkLongFunctions(node *sitter.Node, content []byte, funcTypes map[string]bool, out *[]parser.Issue, depth int) {
	if node == nil || depth > 500 {
		return
	}

	if funcTypes[node.Type()] {
		startLine := nodeStartLine(node)
		endLine := nodeEndLine(node)
		lineSpan := endLine - startLine + 1
		if lineSpan > longFunctionLines {
			name := ""
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				name = nodeText(nameNode, content)
			}
			msg := fmt.Sprintf("Function has %d lines (threshold: %d)", lineSpan, longFunctionLines)
			if name != "" {
				msg = fmt.Sprintf("Function %q has %d lines (threshold: %d)", name, lineSpan, longFunctionLines)
			}
			*out = append(*out, parser.Issue{
				Code:     "LONG_FUNCTION",
				Message:  msg,
				Line:     startLine,
				Column:   1,
				Severity: "warning",
			})
		}
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkLongFunctions(node.NamedChild(i), content, funcTypes, out, depth+1)
	}
}

// collectDeepNesting walks the AST tracking nesting depth using per-language
// nesting node types. Emits a warning when depth exceeds the threshold.
func collectDeepNesting(node *sitter.Node, nestingSet map[string]bool, issues []parser.Issue) []parser.Issue {
	if node == nil {
		return issues
	}
	walkDeepNesting(node, nestingSet, 0, &issues, 0)
	return issues
}

func walkDeepNesting(node *sitter.Node, nestingSet map[string]bool, nestDepth int, out *[]parser.Issue, walkDepth int) {
	if node == nil || walkDepth > 500 {
		return
	}

	currentDepth := nestDepth
	if nestingSet[node.Type()] {
		currentDepth++
		if currentDepth > deepNestingThreshold {
			line := int32(node.StartPoint().Row) + 1
			col := int32(node.StartPoint().Column) + 1
			*out = append(*out, parser.Issue{
				Code:     "DEEP_NESTING",
				Message:  fmt.Sprintf("Nesting depth %d exceeds threshold %d", currentDepth, deepNestingThreshold),
				Line:     line,
				Column:   col,
				Severity: "warning",
			})
			// Don't recurse further — already reported for this scope.
			return
		}
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkDeepNesting(node.NamedChild(i), nestingSet, currentDepth, out, walkDepth+1)
	}
}

// hasExportNodes checks if the AST contains any export-like declaration nodes.
func hasExportNodes(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	return walkForExports(node, 0)
}

func walkForExports(node *sitter.Node, depth int) bool {
	if node == nil || depth > 1 {
		return false
	}

	t := node.Type()
	if t == "export_statement" || t == "export_default_declaration" {
		return true
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		if walkForExports(node.NamedChild(i), depth+1) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Issue factory functions (called by pipeline, not by ExtractDiagnostics)
// ---------------------------------------------------------------------------

// CreateUnsupportedLanguageIssue returns an issue for an unsupported file extension.
func CreateUnsupportedLanguageIssue(ext string) parser.Issue {
	return parser.Issue{
		Code:     "UNSUPPORTED_LANGUAGE",
		Message:  fmt.Sprintf("Unsupported file extension: %s", ext),
		Line:     0,
		Column:   0,
		Severity: "info",
	}
}

// CreateOversizedFileIssue returns an issue for a file that exceeds the size limit.
func CreateOversizedFileIssue(size, limit int64) parser.Issue {
	return parser.Issue{
		Code:     "OVERSIZED_FILE",
		Message:  fmt.Sprintf("File size %d bytes exceeds limit %d bytes", size, limit),
		Line:     0,
		Column:   0,
		Severity: "warning",
	}
}

// CreateParseTimeoutIssue returns an issue for a parse timeout.
func CreateParseTimeoutIssue(filePath string, timeoutMs int) parser.Issue {
	return parser.Issue{
		Code:     "PARSE_TIMEOUT",
		Message:  fmt.Sprintf("Parsing %s timed out after %dms", filePath, timeoutMs),
		Line:     0,
		Column:   0,
		Severity: "error",
	}
}

// CreateParseErrorIssue returns an issue for a non-timeout parse failure
// (e.g. pool shutdown, context cancellation). Uses PARSER_FAILURE to
// distinguish from the PARSE_ERROR code emitted by ExtractDiagnostics
// for AST-level syntax errors (tree-sitter ERROR nodes).
func CreateParseErrorIssue(filePath string, err error) parser.Issue {
	return parser.Issue{
		Code:     "PARSER_FAILURE",
		Message:  fmt.Sprintf("Parsing %s failed: %v", filePath, err),
		Line:     0,
		Column:   0,
		Severity: "error",
	}
}

// CreateExtractionErrorIssue returns an issue for an extraction error.
func CreateExtractionErrorIssue(extractor string, err error) parser.Issue {
	return parser.Issue{
		Code:     "EXTRACTION_ERROR",
		Message:  fmt.Sprintf("Extractor %q failed: %v", extractor, err),
		Line:     0,
		Column:   0,
		Severity: "error",
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toStringSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}
