package extractors

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// referenceContext carries state through reference extraction.
type referenceContext struct {
	content    []byte
	langID     string
	langConfig *registry.LanguageConfig
	symbols    []parser.Symbol
	imports    []parser.Import
	refs       []parser.Reference
	symbolIdx  map[string]string // name → symbolID
	importIdx  map[string]bool   // imported name → true
	seen       map[string]bool   // dedup key → true
}

// knownJSGlobals is the set of well-known global identifiers for JS/TS.
var knownJSGlobals = map[string]bool{
	"console": true, "window": true, "document": true, "global": true,
	"globalThis": true, "process": true, "setTimeout": true, "setInterval": true,
	"clearTimeout": true, "clearInterval": true, "Promise": true, "Math": true,
	"JSON": true, "Object": true, "Array": true, "Map": true, "Set": true,
	"WeakMap": true, "WeakSet": true, "Symbol": true, "Proxy": true,
	"Reflect": true, "Error": true, "TypeError": true, "RangeError": true,
	"Date": true, "RegExp": true, "parseInt": true, "parseFloat": true,
	"isNaN": true, "isFinite": true, "encodeURI": true, "decodeURI": true,
	"encodeURIComponent": true, "decodeURIComponent": true, "atob": true, "btoa": true,
	"fetch": true, "Request": true, "Response": true, "URL": true, "URLSearchParams": true,
	"Headers": true, "AbortController": true, "FormData": true, "Blob": true,
	"File": true, "FileReader": true, "Event": true, "CustomEvent": true,
	"EventTarget": true, "Node": true, "Element": true, "HTMLElement": true,
	"navigator": true, "location": true, "history": true, "localStorage": true,
	"sessionStorage": true, "performance": true, "crypto": true, "Buffer": true,
	"require": true, "module": true, "exports": true, "__dirname": true, "__filename": true,
	"queueMicrotask": true, "structuredClone": true, "requestAnimationFrame": true,
}

// knownPythonGlobals is the set of well-known global identifiers for Python.
var knownPythonGlobals = map[string]bool{
	"print": true, "len": true, "range": true, "enumerate": true, "zip": true,
	"map": true, "filter": true, "sorted": true, "reversed": true, "list": true,
	"dict": true, "set": true, "tuple": true, "int": true, "float": true,
	"str": true, "bool": true, "bytes": true, "type": true, "isinstance": true,
	"issubclass": true, "hasattr": true, "getattr": true, "setattr": true,
	"delattr": true, "super": true, "property": true, "staticmethod": true,
	"classmethod": true, "open": true, "input": true, "repr": true, "abs": true,
	"min": true, "max": true, "sum": true, "any": true, "all": true, "iter": true,
	"next": true, "id": true, "hash": true, "callable": true, "dir": true,
	"vars": true, "globals": true, "locals": true, "exec": true, "eval": true,
	"compile": true, "format": true, "chr": true, "ord": true, "hex": true, "oct": true,
	"bin": true, "round": true, "pow": true, "divmod": true, "complex": true,
	"memoryview": true, "bytearray": true, "frozenset": true, "slice": true,
	"object": true, "Exception": true, "ValueError": true, "TypeError": true,
	"KeyError": true, "IndexError": true, "AttributeError": true, "ImportError": true,
	"RuntimeError": true, "StopIteration": true, "NotImplementedError": true,
}

// knownGoGlobals is the set of well-known global identifiers for Go.
var knownGoGlobals = map[string]bool{
	"len": true, "cap": true, "make": true, "new": true, "append": true,
	"copy": true, "close": true, "delete": true, "panic": true, "recover": true,
	"print": true, "println": true, "complex": true, "real": true, "imag": true,
	"clear": true, "min": true, "max": true, "nil": true, "true": true, "false": true,
	"iota": true,
}

// skipCallTargets are call targets that should be skipped (import/require).
var skipCallTargets = map[string]bool{
	"require": true, "import": true,
}

// ExtractReferences extracts cross-symbol references from a parsed file.
// For Tier 1 languages, it walks the AST for call expressions, type references,
// inheritance clauses, constructors, decorators, JSX elements, and hooks.
// Tier 2/3 languages return nil.
func ExtractReferences(root *sitter.Node, content []byte, symbols []parser.Symbol, imports []parser.Import, langID string) []parser.Reference {
	if root == nil {
		return nil
	}

	langConfig, ok := registry.GetLanguageConfig(langID)
	if !ok {
		return nil
	}

	if langConfig.Tier != registry.Tier1 {
		return nil
	}

	importIdx := buildImportIndex(imports)
	enrichImportIndexFromAST(root, content, langID, importIdx)

	ctx := &referenceContext{
		content:    content,
		langID:     langID,
		langConfig: langConfig,
		symbols:    symbols,
		imports:    imports,
		refs:       nil,
		symbolIdx:  buildSymbolIndex(symbols),
		importIdx:  importIdx,
		seen:       make(map[string]bool),
	}

	ctx.walkForReferences(root, 0)

	if len(ctx.refs) == 0 {
		return nil
	}
	parser.SortReferences(ctx.refs)
	return ctx.refs
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// generateReferenceID produces a deterministic "ref_<hash16>" identifier.
func generateReferenceID(kind, targetName string, line, column int32) string {
	key := fmt.Sprintf("%s:%s:%d:%d", kind, targetName, line, column)
	hash := parser.StableHash(key)
	return "ref_" + hash[:16]
}

// buildImportIndex maps imported names to true for resolution scope.
// It uses both the stored import data and source code analysis to capture
// individual imported identifiers (e.g., from `import { readFile } from "fs"`).
func buildImportIndex(imports []parser.Import) map[string]bool {
	idx := make(map[string]bool, len(imports)*2)
	for _, imp := range imports {
		if imp.ImportName != "" {
			idx[imp.ImportName] = true
			hasSlash := strings.Contains(imp.ImportName, "/")
			// For dotted imports without slashes (e.g., Python "os.path"),
			// index both the receiver base ("os") and the leaf ("path").
			// Skip path-based imports (e.g., "github.com/foo/bar") where
			// the dot is part of a domain, not a module hierarchy.
			if !hasSlash {
				if dot := strings.Index(imp.ImportName, "."); dot >= 0 {
					idx[imp.ImportName[:dot]] = true // receiver base
				}
				if dot := strings.LastIndex(imp.ImportName, "."); dot >= 0 {
					idx[imp.ImportName[dot+1:]] = true // leaf name
				}
			}
			// For Rust :: namespace imports (e.g., "std::collections::HashMap"),
			// index the leaf name so HashMap resolves as IMPORTED.
			if sep := strings.LastIndex(imp.ImportName, "::"); sep >= 0 {
				leaf := imp.ImportName[sep+2:]
				if leaf != "" {
					idx[leaf] = true
				}
			}
			// Also add the basename for path imports like "./foo/bar".
			if hasSlash {
				if slash := strings.LastIndex(imp.ImportName, "/"); slash >= 0 {
					idx[imp.ImportName[slash+1:]] = true
				}
			}
		}
		if imp.PackageName != "" {
			idx[imp.PackageName] = true
		}
	}
	return idx
}

// enrichImportIndexFromAST adds individual imported identifiers from import statements
// to the import index. This handles cases like `import { readFile } from "fs"` where
// the import extractor stores "fs" as ImportName but we need "readFile" in the index.
// Also handles Python aliases and Rust use declarations.
func enrichImportIndexFromAST(root *sitter.Node, content []byte, langID string, idx map[string]bool) {
	if root == nil {
		return
	}

	switch langID {
	case "python":
		enrichPythonImportAliases(root, content, idx)
		return
	case "rust":
		enrichRustUseDeclarations(root, content, idx)
		return
	}

	if !jsLikeLanguages[langID] {
		return
	}
	// Walk top-level import_statements.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil || child.Type() != "import_statement" {
			continue
		}
		// Find import_clause > named_imports > import_specifier > identifier.
		clause := findChildByType(child, "import_clause")
		if clause == nil {
			continue
		}
		// Default import.
		for j := 0; j < int(clause.NamedChildCount()); j++ {
			cc := clause.NamedChild(j)
			if cc == nil {
				continue
			}
			if cc.Type() == "identifier" {
				idx[nodeText(cc, content)] = true
			}
			if cc.Type() == "named_imports" {
				for k := 0; k < int(cc.NamedChildCount()); k++ {
					spec := cc.NamedChild(k)
					if spec == nil || spec.Type() != "import_specifier" {
						continue
					}
					// May have alias: `import { foo as bar }`.
					alias := findChildByFieldName(spec, "alias")
					if alias != nil {
						idx[nodeText(alias, content)] = true
					} else {
						name := findChildByFieldName(spec, "name")
						if name != nil {
							idx[nodeText(name, content)] = true
						} else if spec.NamedChildCount() > 0 {
							if firstChild := spec.NamedChild(0); firstChild != nil {
								idx[nodeText(firstChild, content)] = true
							}
						}
					}
				}
			}
			if cc.Type() == "namespace_import" {
				// import * as ns — add the namespace name.
				for k := 0; k < int(cc.NamedChildCount()); k++ {
					id := cc.NamedChild(k)
					if id != nil && id.Type() == "identifier" {
						idx[nodeText(id, content)] = true
					}
				}
			}
		}
	}
}

// enrichPythonImportAliases walks top-level Python import statements and
// indexes aliases (e.g., `import os as operating_system` → "operating_system").
func enrichPythonImportAliases(root *sitter.Node, content []byte, idx map[string]bool) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "import_statement", "import_from_statement":
			for j := 0; j < int(child.NamedChildCount()); j++ {
				spec := child.NamedChild(j)
				if spec == nil {
					continue
				}
				if spec.Type() == "aliased_import" {
					// `import X as Y` or `from M import X as Y` — index alias Y.
					alias := findChildByFieldName(spec, "alias")
					if alias != nil {
						idx[nodeText(alias, content)] = true
					}
				} else if spec.Type() == "dotted_name" || spec.Type() == "identifier" {
					// `from M import X` — index X.
					idx[nodeText(spec, content)] = true
				}
			}
		}
	}
}

// enrichRustUseDeclarations walks top-level Rust use declarations and
// indexes the imported names (e.g., `use std::collections::HashMap` → "HashMap",
// `use std::io as stdio` → "stdio").
func enrichRustUseDeclarations(root *sitter.Node, content []byte, idx map[string]bool) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil || child.Type() != "use_declaration" {
			continue
		}
		enrichRustUseTree(child, content, idx)
	}
}

func enrichRustUseTree(node *sitter.Node, content []byte, idx map[string]bool) {
	if node == nil {
		return
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "use_as_clause":
			// `use X as Y` — index alias Y.
			alias := findChildByFieldName(child, "alias")
			if alias != nil {
				idx[nodeText(alias, content)] = true
			}
		case "use_list":
			// `use std::{A, B}` — recurse.
			enrichRustUseTree(child, content, idx)
		case "scoped_identifier":
			// `use std::collections::HashMap` — index leaf.
			nameNode := findChildByFieldName(child, "name")
			if nameNode != nil {
				idx[nodeText(nameNode, content)] = true
			}
		case "identifier":
			idx[nodeText(child, content)] = true
		case "scoped_use_list":
			// `use std::collections::{HashMap, BTreeMap}` — recurse.
			enrichRustUseTree(child, content, idx)
		}
	}
}

// findEnclosingSymbolID finds the symbol with the tightest [StartLine, EndLine]
// range that contains the given line. Returns "" if no enclosing symbol found.
func findEnclosingSymbolID(symbols []parser.Symbol, line int32) string {
	bestID := ""
	bestSpan := int32(1<<31 - 1) // max int32
	for i := range symbols {
		s := &symbols[i]
		if s.StartLine <= line && line <= s.EndLine {
			span := s.EndLine - s.StartLine
			if span < bestSpan {
				bestSpan = span
				bestID = s.SymbolID
			}
		}
	}
	return bestID
}

// resolveScope determines the resolution scope and resolved symbol ID.
func (ctx *referenceContext) resolveScope(targetName, qualifiedHint string) (scope string, resolvedSymbolID string) {
	// Check for member/qualified access first — when qualifiedHint contains
	// a dot or :: separator, this is a member/namespaced expression and
	// should not be matched against local/imported symbols of the same name.
	hasDot := strings.Contains(qualifiedHint, ".") || strings.Contains(targetName, ".")
	hasNS := strings.Contains(qualifiedHint, "::") || strings.Contains(targetName, "::")
	if hasDot || hasNS {
		// If the receiver part matches an import, classify as IMPORTED.
		if qualifiedHint != "" {
			receiver := ""
			if dot := strings.Index(qualifiedHint, "."); dot >= 0 {
				receiver = qualifiedHint[:dot]
			} else if sep := strings.Index(qualifiedHint, "::"); sep >= 0 {
				receiver = qualifiedHint[:sep]
			}
			if receiver != "" && ctx.importIdx[receiver] {
				return "IMPORTED", ""
			}
		}
		return "MEMBER", ""
	}
	// Check local symbols.
	if symID, ok := ctx.symbolIdx[targetName]; ok {
		return "LOCAL", symID
	}
	// Check imports.
	if ctx.importIdx[targetName] {
		return "IMPORTED", ""
	}
	// Check known globals.
	if ctx.isGlobal(targetName) {
		return "GLOBAL", ""
	}
	return "UNKNOWN", ""
}

// isGlobal checks whether the target name is a known global for the current language.
func (ctx *referenceContext) isGlobal(name string) bool {
	if jsLikeLanguages[ctx.langID] {
		return knownJSGlobals[name]
	}
	if ctx.langID == "python" {
		return knownPythonGlobals[name]
	}
	if ctx.langID == "go" {
		return knownGoGlobals[name]
	}
	return false
}

// confidenceForScope returns the confidence level based on resolution scope.
func confidenceForScope(scope string) string {
	switch scope {
	case "LOCAL", "IMPORTED":
		return "HIGH"
	case "MEMBER", "GLOBAL":
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// addReference deduplicates and appends a reference.
func (ctx *referenceContext) addReference(kind, targetName, qualifiedHint, rawText string, node *sitter.Node) {
	if targetName == "" {
		return
	}
	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)

	dedupKey := fmt.Sprintf("%s:%s:%d:%d", kind, targetName, line, col)
	if ctx.seen[dedupKey] {
		return
	}
	ctx.seen[dedupKey] = true

	scope, resolvedID := ctx.resolveScope(targetName, qualifiedHint)
	enclosingID := findEnclosingSymbolID(ctx.symbols, line)

	ctx.refs = append(ctx.refs, parser.Reference{
		ReferenceID:            generateReferenceID(kind, targetName, line, col),
		SourceSymbolID:         enclosingID,
		ReferenceKind:          kind,
		RawText:                rawText,
		TargetName:             targetName,
		QualifiedTargetHint:    qualifiedHint,
		StartLine:              line,
		StartColumn:            col,
		EndLine:                nodeEndLine(node),
		EndColumn:              int32(node.EndPoint().Column),
		ResolutionScope:        scope,
		ResolvedTargetSymbolID: resolvedID,
		Confidence:             confidenceForScope(scope),
	})
}

// ---------------------------------------------------------------------------
// AST walker
// ---------------------------------------------------------------------------

func (ctx *referenceContext) walkForReferences(node *sitter.Node, depth int) {
	if node == nil || depth > 50 {
		return
	}

	nodeType := node.Type()

	switch nodeType {
	// --- Call expressions ---
	case "call_expression":
		if jsLikeLanguages[ctx.langID] || ctx.langID == "go" || ctx.langID == "rust" ||
			ctx.langID == "kotlin" || ctx.langID == "c" || ctx.langID == "cpp" || ctx.langID == "swift" {
			ctx.handleCallExpression(node)
		}
	case "call":
		if ctx.langID == "python" || ctx.langID == "ruby" {
			ctx.handleCallExpression(node)
		}
	case "method_invocation":
		if ctx.langID == "java" || ctx.langID == "kotlin" {
			ctx.handleCallExpression(node)
		}
	case "method_call":
		if ctx.langID == "ruby" {
			ctx.handleCallExpression(node)
		}
	case "invocation_expression":
		if ctx.langID == "csharp" {
			ctx.handleCallExpression(node)
		}
	case "macro_invocation":
		if ctx.langID == "rust" {
			ctx.handleCallExpression(node)
		}
	case "function_call_expression", "method_call_expression":
		if ctx.langID == "php" {
			ctx.handleCallExpression(node)
		}

	// --- Type references ---
	case "type_identifier":
		ctx.handleTypeReference(node)
	case "generic_type":
		if jsLikeLanguages[ctx.langID] || ctx.langID == "rust" || ctx.langID == "java" || ctx.langID == "kotlin" {
			ctx.handleGenericType(node)
		}
	case "generic_name":
		if ctx.langID == "csharp" {
			ctx.handleTypeReference(node)
		}
	case "named_type":
		if ctx.langID == "php" {
			ctx.handleTypeReference(node)
		}
	case "template_type":
		if ctx.langID == "cpp" {
			ctx.handleGenericType(node)
		}

	// --- Inheritance ---
	case "extends_clause":
		if jsLikeLanguages[ctx.langID] {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "implements_clause":
		if jsLikeLanguages[ctx.langID] {
			ctx.handleInheritanceClause(node, "IMPLEMENTS")
		}
	case "superclass":
		if ctx.langID == "java" || ctx.langID == "ruby" {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "super_interfaces":
		if ctx.langID == "java" {
			ctx.handleInheritanceClause(node, "IMPLEMENTS")
		}
	case "delegation_specifier":
		if ctx.langID == "kotlin" {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "base_class_clause":
		if ctx.langID == "cpp" {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "base_list":
		if ctx.langID == "csharp" {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "inheritance_clause":
		if ctx.langID == "swift" {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "trait_bound":
		if ctx.langID == "rust" {
			ctx.handleInheritanceClause(node, "IMPLEMENTS")
		}
	case "class_definition":
		// Python class definition — extract bases from argument_list.
		if ctx.langID == "python" {
			argList := findChildByType(node, "argument_list")
			if argList != nil {
				ctx.handleInheritanceClause(argList, "EXTENDS")
			}
		}
	case "base_clause":
		if ctx.langID == "php" {
			ctx.handleInheritanceClause(node, "EXTENDS")
		}
	case "class_interfaces":
		if ctx.langID == "php" {
			ctx.handleInheritanceClause(node, "IMPLEMENTS")
		}

	// --- New expressions ---
	case "new_expression":
		if jsLikeLanguages[ctx.langID] || ctx.langID == "cpp" {
			ctx.handleNewExpression(node)
		}
	case "object_creation_expression":
		if ctx.langID == "java" || ctx.langID == "csharp" || ctx.langID == "php" || ctx.langID == "kotlin" {
			ctx.handleNewExpression(node)
		}
	case "struct_expression":
		if ctx.langID == "rust" {
			ctx.handleNewExpression(node)
		}
	case "composite_literal":
		if ctx.langID == "go" {
			ctx.handleNewExpression(node)
		}

	// --- JSX (JSX/TSX only) ---
	case "jsx_self_closing_element", "jsx_opening_element":
		if jsxLanguages[ctx.langID] {
			ctx.handleJsxReference(node)
		}

	// --- Decorators ---
	case "decorator":
		if jsLikeLanguages[ctx.langID] || ctx.langID == "python" {
			ctx.handleDecorator(node)
		}
	case "annotation", "marker_annotation":
		if ctx.langID == "java" || ctx.langID == "kotlin" {
			ctx.handleDecorator(node)
		}
	}

	// Recurse into all children (including anonymous ones for full coverage).
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			ctx.walkForReferences(child, depth+1)
		}
	}
}

// ---------------------------------------------------------------------------
// Call expression handler
// ---------------------------------------------------------------------------

func (ctx *referenceContext) handleCallExpression(node *sitter.Node) {
	targetName, qualifiedHint, rawText := ctx.extractCallTarget(node)
	if targetName == "" {
		return
	}

	// Skip import/require calls.
	if skipCallTargets[targetName] || skipCallTargets[qualifiedHint] {
		return
	}

	// Determine reference kind.
	kind := "CALL"
	if jsLikeLanguages[ctx.langID] {
		// Only classify as HOOK_USE/FETCH for direct calls, not member calls
		// like obj.useQuery() or api.fetch().
		isDirectCall := !strings.Contains(qualifiedHint, ".")
		if isDirectCall {
			if isHookName(targetName) {
				kind = "HOOK_USE"
			} else if targetName == "fetch" {
				kind = "FETCH"
			}
		}
	}

	ctx.addReference(kind, targetName, qualifiedHint, rawText, node)
}

// extractCallTarget extracts the function/method name from a call expression.
// Returns (targetName, qualifiedHint, rawText).
func (ctx *referenceContext) extractCallTarget(node *sitter.Node) (string, string, string) {
	var funcNode *sitter.Node
	var overrideQualified string // set by language cases that need to override qualifiedHint

	switch ctx.langID {
	case "java":
		// method_invocation: object.method(args) — field "name" for the method,
		// field "object" for the receiver.
		funcNode = findChildByFieldName(node, "name")
		if funcNode != nil {
			// Capture the full qualified hint (object.method) for scope resolution.
			if obj := findChildByFieldName(node, "object"); obj != nil {
				overrideQualified = nodeText(obj, ctx.content) + "." + nodeText(funcNode, ctx.content)
			}
		} else {
			funcNode = findChildByFieldName(node, "object")
		}
	case "python":
		// call: function(args) — field "function".
		funcNode = findChildByFieldName(node, "function")
	case "ruby":
		// call: receiver.method(args) — field "method".
		funcNode = findChildByFieldName(node, "method")
		if funcNode == nil {
			funcNode = findChildByFieldName(node, "receiver")
		}
	case "csharp":
		// C# invocation_expression: first named child is member_access_expression or identifier.
		funcNode = findChildByFieldName(node, "function")
		if funcNode == nil {
			for i := 0; i < int(node.NamedChildCount()); i++ {
				child := node.NamedChild(i)
				if child != nil && (child.Type() == "member_access_expression" || child.Type() == "identifier") {
					funcNode = child
					break
				}
			}
		}
	case "php":
		// function_call_expression or method_call_expression.
		funcNode = findChildByFieldName(node, "function")
		if funcNode == nil {
			funcNode = findChildByFieldName(node, "name")
		}
		if funcNode == nil && node.NamedChildCount() > 0 {
			funcNode = node.NamedChild(0)
		}
	default:
		// JS/TS/Go/Rust/C/C++: field "function".
		funcNode = findChildByFieldName(node, "function")
		if funcNode == nil {
			// Rust macro_invocation: field "macro".
			funcNode = findChildByFieldName(node, "macro")
		}
		if funcNode == nil {
			// Swift/Kotlin: first named child is the function identifier.
			if node.NamedChildCount() > 0 {
				first := node.NamedChild(0)
				if first != nil && (first.Type() == "simple_identifier" || first.Type() == "identifier" ||
					first.Type() == "navigation_expression" || first.Type() == "member_expression") {
					funcNode = first
				}
			}
		}
	}

	if funcNode == nil {
		return "", "", ""
	}

	rawText := nodeText(funcNode, ctx.content)
	qualifiedHint := rawText
	if overrideQualified != "" {
		qualifiedHint = overrideQualified
	}

	// Extract the final target name from member expressions.
	targetName := extractFinalName(funcNode, ctx.content)
	if targetName == "" {
		targetName = rawText
	}

	return targetName, qualifiedHint, rawText
}

// extractFinalName gets the final identifier from a potentially qualified expression.
// For "a.b.c", returns "c". For "foo", returns "foo".
func extractFinalName(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "identifier", "type_identifier", "property_identifier", "field_identifier",
		"shorthand_field_identifier", "simple_identifier", "constant":
		return nodeText(node, content)
	case "member_expression":
		// property field is the final part.
		prop := findChildByFieldName(node, "property")
		if prop != nil {
			return nodeText(prop, content)
		}
		// Fallback: last named child.
		if n := node.NamedChildCount(); n > 0 {
			return nodeText(node.NamedChild(int(n-1)), content)
		}
	case "attribute":
		// Python: obj.attr — field "attribute" is the final part.
		attr := findChildByFieldName(node, "attribute")
		if attr != nil {
			return nodeText(attr, content)
		}
		if n := node.NamedChildCount(); n > 0 {
			return nodeText(node.NamedChild(int(n-1)), content)
		}
	case "member_access_expression":
		// C#: uses "name" field for the member.
		nameNode := findChildByFieldName(node, "name")
		if nameNode != nil {
			return nodeText(nameNode, content)
		}
		prop := findChildByFieldName(node, "property")
		if prop != nil {
			return nodeText(prop, content)
		}
		if n := node.NamedChildCount(); n > 0 {
			return nodeText(node.NamedChild(int(n-1)), content)
		}
	case "scoped_identifier":
		// Rust: path::name — field "name".
		nameNode := findChildByFieldName(node, "name")
		if nameNode != nil {
			return nodeText(nameNode, content)
		}
	case "selector_expression":
		// Go: pkg.Func — field "field".
		field := findChildByFieldName(node, "field")
		if field != nil {
			return nodeText(field, content)
		}
	case "qualified_identifier":
		// C#/Java: Namespace.Type — last identifier.
		if n := node.NamedChildCount(); n > 0 {
			return nodeText(node.NamedChild(int(n-1)), content)
		}
	}
	// Default: return full text.
	return nodeText(node, content)
}

// isHookName returns true if the name matches the React hook pattern use[A-Z].
func isHookName(name string) bool {
	if len(name) < 4 {
		return false
	}
	return strings.HasPrefix(name, "use") && unicode.IsUpper(rune(name[3]))
}

// ---------------------------------------------------------------------------
// Type reference handler
// ---------------------------------------------------------------------------

func (ctx *referenceContext) handleTypeReference(node *sitter.Node) {
	name := nodeText(node, ctx.content)
	if name == "" {
		return
	}

	// Skip builtin types.
	if ctx.langConfig.BuiltinTypes[name] {
		return
	}

	// Skip type identifiers inside import/export statements.
	if hasAncestorOfType(node, "import_statement") || hasAncestorOfType(node, "import_declaration") ||
		hasAncestorOfType(node, "export_statement") {
		return
	}

	// Skip if this is part of a function/class/type declaration name.
	if ctx.isDeclarationName(node) {
		return
	}

	// Skip if inside an inheritance clause (handled separately).
	if hasAncestorOfType(node, "extends_clause") || hasAncestorOfType(node, "implements_clause") ||
		hasAncestorOfType(node, "superclass") || hasAncestorOfType(node, "super_interfaces") ||
		hasAncestorOfType(node, "base_class_clause") || hasAncestorOfType(node, "base_list") ||
		hasAncestorOfType(node, "inheritance_clause") || hasAncestorOfType(node, "base_clause") ||
		hasAncestorOfType(node, "trait_bound") || hasAncestorOfType(node, "delegation_specifier") ||
		hasAncestorOfType(node, "class_interfaces") {
		return
	}

	ctx.addReference("TYPE_REF", name, "", name, node)
}

// handleGenericType handles generic_type / template_type nodes by extracting
// the base type name.
func (ctx *referenceContext) handleGenericType(node *sitter.Node) {
	// The base type is typically the first named child (type_identifier).
	var typeName string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "type_identifier" || child.Type() == "identifier" {
			typeName = nodeText(child, ctx.content)
			break
		}
	}
	if typeName == "" {
		return
	}

	// Skip builtins.
	if ctx.langConfig.BuiltinTypes[typeName] {
		return
	}

	// Skip if in imports/exports.
	// Must match the full set in handleTypeReference.
	if hasAncestorOfType(node, "import_statement") || hasAncestorOfType(node, "import_declaration") ||
		hasAncestorOfType(node, "export_statement") {
		return
	}

	// Skip if inside inheritance clauses (handled separately).
	// Must match the full set in handleTypeReference to avoid duplicate TYPE_REF.
	if hasAncestorOfType(node, "extends_clause") || hasAncestorOfType(node, "implements_clause") ||
		hasAncestorOfType(node, "superclass") || hasAncestorOfType(node, "super_interfaces") ||
		hasAncestorOfType(node, "base_class_clause") || hasAncestorOfType(node, "base_list") ||
		hasAncestorOfType(node, "inheritance_clause") || hasAncestorOfType(node, "base_clause") ||
		hasAncestorOfType(node, "trait_bound") || hasAncestorOfType(node, "delegation_specifier") ||
		hasAncestorOfType(node, "class_interfaces") {
		return
	}

	ctx.addReference("TYPE_REF", typeName, nodeText(node, ctx.content), nodeText(node, ctx.content), node)
}

// isDeclarationName checks whether the type_identifier is the name of a declaration.
func (ctx *referenceContext) isDeclarationName(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	switch parent.Type() {
	case "function_declaration", "class_declaration", "interface_declaration",
		"type_alias_declaration", "enum_declaration", "struct_item", "enum_item",
		"type_spec", "class_definition", "function_definition",
		"method_definition", "type_declaration":
		// Check if this node is the "name" field.
		nameNode := findChildByFieldName(parent, "name")
		return nameNode != nil && nameNode.StartByte() == node.StartByte()
	}
	return false
}

// ---------------------------------------------------------------------------
// Inheritance handler
// ---------------------------------------------------------------------------

func (ctx *referenceContext) handleInheritanceClause(node *sitter.Node, kind string) {
	// Walk children to find type identifiers.
	ctx.extractInheritanceNames(node, kind, 0)
}

func (ctx *referenceContext) extractInheritanceNames(node *sitter.Node, kind string, depth int) {
	if node == nil || depth > 10 {
		return
	}
	switch node.Type() {
	case "type_identifier", "identifier", "constant", "simple_identifier":
		name := nodeText(node, ctx.content)
		if name != "" && !ctx.langConfig.BuiltinTypes[name] {
			ctx.addReference(kind, name, "", name, node)
		}
		return
	case "generic_type", "generic_name", "template_type":
		// Extract base name from generic.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "type_identifier" || child.Type() == "identifier" {
				name := nodeText(child, ctx.content)
				if name != "" && !ctx.langConfig.BuiltinTypes[name] {
					ctx.addReference(kind, name, nodeText(node, ctx.content), nodeText(node, ctx.content), node)
				}
				return
			}
		}
	case "member_expression", "scoped_type_identifier", "nested_type_identifier":
		name := nodeText(node, ctx.content)
		if name != "" {
			ctx.addReference(kind, name, "", name, node)
		}
		return
	case "keyword_argument":
		// Python: skip keyword arguments in class bases (e.g., metaclass=ABCMeta).
		return
	}

	// Recurse into children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ctx.extractInheritanceNames(node.NamedChild(i), kind, depth+1)
	}
}

// ---------------------------------------------------------------------------
// New expression handler
// ---------------------------------------------------------------------------

func (ctx *referenceContext) handleNewExpression(node *sitter.Node) {
	var name string

	switch node.Type() {
	case "new_expression":
		// JS/TS/C++: new Foo() — constructor field or first named child.
		constructor := findChildByFieldName(node, "constructor")
		if constructor != nil {
			name = extractFinalName(constructor, ctx.content)
		} else if node.NamedChildCount() > 0 {
			child := node.NamedChild(0)
			name = extractFinalName(child, ctx.content)
		}
	case "object_creation_expression":
		// Java/C#/PHP/Kotlin.
		typeNode := findChildByFieldName(node, "type")
		if typeNode != nil {
			name = extractFinalName(typeNode, ctx.content)
		} else if node.NamedChildCount() > 0 {
			name = extractFinalName(node.NamedChild(0), ctx.content)
		}
	case "struct_expression":
		// Rust: StructName { fields }.
		nameNode := findChildByFieldName(node, "name")
		if nameNode != nil {
			name = extractFinalName(nameNode, ctx.content)
		}
	case "composite_literal":
		// Go: Type{fields} — the "type" field.
		// Skip map, slice, and array literals; only struct instantiation is meaningful.
		typeNode := findChildByFieldName(node, "type")
		if typeNode != nil {
			switch typeNode.Type() {
			case "map_type", "slice_type", "array_type":
				// Not a struct instantiation — skip.
			default:
				name = extractFinalName(typeNode, ctx.content)
			}
		}
	}

	if name == "" || ctx.langConfig.BuiltinTypes[name] {
		return
	}

	ctx.addReference("NEW_EXPR", name, "", nodeText(node, ctx.content), node)
}

// ---------------------------------------------------------------------------
// JSX reference handler
// ---------------------------------------------------------------------------

func (ctx *referenceContext) handleJsxReference(node *sitter.Node) {
	name := extractJsxComponentName(node, ctx.content)
	if name == "" {
		return
	}
	// Skip intrinsic HTML elements (lowercase names like div, span, button).
	// Only user-defined components (uppercase first letter) are cross-symbol references.
	// Use utf8.DecodeRuneInString for correct non-ASCII handling.
	if firstRune, _ := utf8.DecodeRuneInString(name); unicode.IsLower(firstRune) {
		return
	}
	ctx.addReference("JSX_RENDER", name, "", nodeText(node, ctx.content), node)
}

// NOTE: extractJsxComponentName is defined in jsx.go and shared with this file.

// ---------------------------------------------------------------------------
// Decorator handler
// ---------------------------------------------------------------------------

func (ctx *referenceContext) handleDecorator(node *sitter.Node) {
	var name string
	var rawText string

	switch ctx.langID {
	case "python":
		// Python decorator: @name or @name.attr or @name(args).
		// Walk children for identifier or attribute.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			switch child.Type() {
			case "identifier":
				name = nodeText(child, ctx.content)
				rawText = name
			case "attribute":
				name = nodeText(child, ctx.content)
				rawText = name
			case "call":
				// @decorator(args) — get the function name.
				fn := findChildByFieldName(child, "function")
				if fn != nil {
					name = nodeText(fn, ctx.content)
					rawText = name
				}
			}
			if name != "" {
				break
			}
		}
	case "java", "kotlin":
		// Java annotation: @Name or @Name(args).
		nameNode := findChildByFieldName(node, "name")
		if nameNode != nil {
			name = nodeText(nameNode, ctx.content)
		} else if node.NamedChildCount() > 0 {
			name = nodeText(node.NamedChild(0), ctx.content)
		}
		rawText = nodeText(node, ctx.content)
	default:
		// JS/TS decorator.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			switch child.Type() {
			case "identifier":
				name = nodeText(child, ctx.content)
				rawText = name
			case "call_expression":
				fn := findChildByFieldName(child, "function")
				if fn != nil {
					name = extractFinalName(fn, ctx.content)
					rawText = nodeText(fn, ctx.content)
				}
			case "member_expression":
				name = nodeText(child, ctx.content)
				rawText = name
			}
			if name != "" {
				break
			}
		}
	}

	if name == "" {
		return
	}

	ctx.addReference("DECORATOR", name, "", rawText, node)
}
