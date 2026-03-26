package extractors

import (
	"path"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// importContext carries state through the recursive AST walk for import extraction.
type importContext struct {
	content       []byte
	filePath      string
	langID        string
	langConfig    *registry.LanguageConfig
	imports       []parser.Import
	seen          map[string]bool // dedup by source specifier (ImportName)
	rustLocalMods map[string]bool // Rust: mod names declared in this file (for bare use path classification)
}

// ExtractImports walks the tree-sitter AST and returns all import declarations found.
// It uses the language registry to determine which node types represent imports
// and how to classify them (STDLIB, INTERNAL, EXTERNAL).
// One Import is emitted per unique source specifier, in document order.
func ExtractImports(root *sitter.Node, content []byte, filePath string, langID string) []parser.Import {
	if root == nil {
		return nil
	}

	langConfig, ok := registry.GetLanguageConfig(langID)
	if !ok {
		return nil
	}

	if len(langConfig.ImportNodeTypes) == 0 {
		return nil
	}

	ctx := &importContext{
		content:    content,
		filePath:   filePath,
		langID:     langID,
		langConfig: langConfig,
		seen:       make(map[string]bool),
	}

	// Rust: pre-scan top-level mod declarations (mod foo;) so bare use paths
	// like `use foo::Bar` can be classified as INTERNAL when foo is a local module.
	if langID == "rust" {
		ctx.rustLocalMods = make(map[string]bool)
		for i := 0; i < int(root.NamedChildCount()); i++ {
			child := root.NamedChild(i)
			if child.Type() == "mod_item" && findChildByType(child, "declaration_list") == nil {
				if nameNode := findChildByFieldName(child, "name"); nameNode != nil {
					ctx.rustLocalMods[nodeText(nameNode, content)] = true
				}
			}
		}
	}

	ctx.walkNode(root, 0)
	return ctx.imports
}

// walkNode recursively visits the AST looking for import-related nodes.
func (ctx *importContext) walkNode(node *sitter.Node, depth int) {
	if node == nil || depth > 50 {
		return
	}

	nodeType := node.Type()

	// JS/TS: also handle export_statement with a source (re-exports).
	if jsLikeLanguages[ctx.langID] && nodeType == "export_statement" {
		ctx.extractJSReexport(node)
		// Don't return — still recurse children for nested call_expressions.
	}

	// Rust: handle mod declarations (mod foo;) as internal imports.
	if ctx.langID == "rust" && nodeType == "mod_item" {
		ctx.extractRustModImport(node)
		return
	}

	// Check if this node type matches any ImportNodeTypes.
	if ctx.isImportNodeType(nodeType) {
		ctx.extractImportFromNode(node)
		// For JS/TS call_expressions, still recurse because non-import calls
		// (e.g. Promise.all) may contain nested import() calls as arguments.
		if !(jsLikeLanguages[ctx.langID] && nodeType == "call_expression") {
			return
		}
	}

	// Recurse into children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ctx.walkNode(node.NamedChild(i), depth+1)
	}
}

// isImportNodeType checks whether the node type is in ImportNodeTypes.
func (ctx *importContext) isImportNodeType(nodeType string) bool {
	for _, t := range ctx.langConfig.ImportNodeTypes {
		if t == nodeType {
			return true
		}
	}
	return false
}

// extractImportFromNode dispatches to language-specific extraction logic.
func (ctx *importContext) extractImportFromNode(node *sitter.Node) {
	switch {
	case jsLikeLanguages[ctx.langID]:
		ctx.extractJSImport(node)
	case ctx.langID == "python":
		ctx.extractPythonImport(node)
	case ctx.langID == "go":
		ctx.extractGoImport(node)
	case ctx.langID == "rust":
		ctx.extractRustImport(node)
	case ctx.langID == "java":
		ctx.extractJavaImport(node)
	case ctx.langID == "kotlin":
		ctx.extractKotlinImport(node)
	case ctx.langID == "c" || ctx.langID == "cpp":
		ctx.extractCInclude(node)
	case ctx.langID == "csharp":
		ctx.extractCSharpUsing(node)
	case ctx.langID == "swift":
		ctx.extractSwiftImport(node)
	case ctx.langID == "ruby":
		ctx.extractRubyRequire(node)
	case ctx.langID == "php":
		ctx.extractPHPImport(node)
	case ctx.langID == "bash":
		ctx.extractBashSource(node)
	case ctx.langID == "hcl":
		ctx.extractHCLImport(node)
	case ctx.langID == "dockerfile":
		ctx.extractDockerfileFrom(node)
	case ctx.langID == "css" || ctx.langID == "scss":
		ctx.extractCSSImport(node)
	case ctx.langID == "html":
		ctx.extractHTMLImport(node)
	case ctx.langID == "markdown":
		ctx.extractMarkdownLink(node)
	}
}

// addImport adds an import if its source has not been seen before (dedup).
func (ctx *importContext) addImport(source string, isInternal bool) {
	if source == "" {
		return
	}
	if ctx.seen[source] {
		return
	}
	ctx.seen[source] = true

	importType := ctx.classifyImport(source, isInternal)
	targetPath := ctx.resolvePath(source, importType)

	ctx.imports = append(ctx.imports, parser.Import{
		SourceFilePath: ctx.filePath,
		ImportName:     source,
		ImportType:     importType,
		TargetFilePath: targetPath,
		PackageName:    source,
	})
}

// classifyImport determines the import type: STDLIB, INTERNAL, or EXTERNAL.
func (ctx *importContext) classifyImport(source string, forceInternal bool) string {
	if forceInternal {
		return "INTERNAL"
	}

	// Check exact stdlib match.
	if ctx.langConfig.StdlibModules[source] {
		return "STDLIB"
	}

	// For Python, check if the top-level module (before first dot) is stdlib.
	// e.g., "os.path" → check "os".
	if ctx.langID == "python" {
		if idx := strings.IndexByte(source, '.'); idx > 0 {
			if ctx.langConfig.StdlibModules[source[:idx]] {
				return "STDLIB"
			}
		}
	}

	// Check stdlib prefixes (e.g., "node:" for JS, "std::" for Rust).
	// Any import matching a stdlib prefix is classified as STDLIB by convention.
	for _, prefix := range ctx.langConfig.StdlibPrefixes {
		if strings.HasPrefix(source, prefix) {
			return "STDLIB"
		}
	}

	// Check internal import patterns.
	for _, pattern := range ctx.langConfig.InternalImportPatterns {
		if strings.HasPrefix(source, pattern) {
			return "INTERNAL"
		}
	}

	// Rust: bare use paths whose first segment matches a local mod declaration
	// are internal (e.g., `use error::Foo` when `mod error;` is in the same file).
	if ctx.langID == "rust" && ctx.rustLocalMods != nil {
		firstSeg := source
		if idx := strings.Index(source, "::"); idx >= 0 {
			firstSeg = source[:idx]
		}
		if ctx.rustLocalMods[firstSeg] {
			return "INTERNAL"
		}
	}

	return "EXTERNAL"
}

// resolvePath computes the resolved file path for internal imports.
// Only applies to languages with filesystem-relative import specifiers.
func (ctx *importContext) resolvePath(source, importType string) string {
	if importType != "INTERNAL" {
		return ""
	}

	switch {
	case jsLikeLanguages[ctx.langID]:
		if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
			return path.Join(path.Dir(ctx.filePath), source)
		}
	case ctx.langID == "c" || ctx.langID == "cpp":
		// Quoted includes get path resolution.
		return path.Join(path.Dir(ctx.filePath), source)
	case ctx.langID == "ruby":
		if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
			return path.Join(path.Dir(ctx.filePath), source)
		}
	case ctx.langID == "bash":
		if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
			return path.Join(path.Dir(ctx.filePath), source)
		}
	case ctx.langID == "python":
		// Python relative imports: ".utils" → sibling package, "..models" → parent.
		if len(source) == 0 || source[0] != '.' {
			return ""
		}
		dots := 0
		for dots < len(source) && source[dots] == '.' {
			dots++
		}
		remainder := source[dots:]
		if remainder != "" {
			remainder = strings.ReplaceAll(remainder, ".", "/")
		}
		// First dot = same package dir; each extra dot = one level up.
		dir := path.Dir(ctx.filePath)
		for i := 1; i < dots; i++ {
			dir = path.Dir(dir)
		}
		if remainder == "" {
			return dir
		}
		return path.Join(dir, remainder)
	case ctx.langID == "rust":
		switch {
		case strings.HasPrefix(source, "crate::"):
			modPath := strings.TrimPrefix(source, "crate::")
			modPath = strings.ReplaceAll(modPath, "::", "/")
			return "src/" + modPath
		case strings.HasPrefix(source, "super::"):
			// Count and consume all leading super:: segments.
			// super::super::foo walks up two parent modules.
			rest := source
			supers := 0
			for strings.HasPrefix(rest, "super::") {
				rest = strings.TrimPrefix(rest, "super::")
				supers++
			}
			modPath := strings.ReplaceAll(rest, "::", "/")
			dir := rustModuleDir(ctx.filePath)
			for i := 0; i < supers; i++ {
				dir = path.Dir(dir)
			}
			if modPath == "" {
				return dir
			}
			return path.Join(dir, modPath)
		case strings.HasPrefix(source, "self::"):
			modPath := strings.TrimPrefix(source, "self::")
			modPath = strings.ReplaceAll(modPath, "::", "/")
			modDir := rustModuleDir(ctx.filePath)
			return path.Join(modDir, modPath)
		default:
			// Bare path (e.g., "error" from mod declaration or "error::Foo" from use).
			modPath := strings.ReplaceAll(source, "::", "/")
			return path.Join(rustModuleDir(ctx.filePath), modPath)
		}
	}

	return ""
}

// rustModuleDir returns the directory that represents the current module scope
// for a Rust source file.
//
//   - mod.rs always represents its containing directory (directory module).
//   - lib.rs and main.rs represent their containing directory only at the
//     crate root (parent dir named "src"). Nested lib.rs/main.rs (e.g.
//     src/handlers/main.rs) are ordinary file modules.
//   - All other .rs files represent a virtual directory named after the
//     file stem (e.g. src/handlers/auth.rs → src/handlers/auth).
func rustModuleDir(filePath string) string {
	base := path.Base(filePath)
	stem := strings.TrimSuffix(base, ".rs")
	if stem == "mod" {
		return path.Dir(filePath)
	}
	if (stem == "lib" || stem == "main") && path.Base(path.Dir(filePath)) == "src" {
		return path.Dir(filePath)
	}
	return path.Join(path.Dir(filePath), stem)
}

// ---------------------------------------------------------------------------
// JS/TS/JSX/TSX
// ---------------------------------------------------------------------------

// extractJSImport handles import_statement and call_expression (require/import()).
func (ctx *importContext) extractJSImport(node *sitter.Node) {
	switch node.Type() {
	case "import_statement":
		ctx.extractJSImportStatement(node)
	case "call_expression":
		ctx.extractJSCallExpression(node)
	}
}

// extractJSImportStatement extracts the source from an ESM import statement.
// Covers: named, default, namespace, side-effect, and import type.
func (ctx *importContext) extractJSImportStatement(node *sitter.Node) {
	source := findChildByFieldName(node, "source")
	if source == nil {
		source = findChildByType(node, "string")
	}
	if source == nil {
		return
	}
	ctx.addImport(stripQuotes(nodeText(source, ctx.content)), false)
}

// extractJSReexport handles export statements that re-export from another module.
// e.g., export { x } from './mod'
func (ctx *importContext) extractJSReexport(node *sitter.Node) {
	source := findChildByFieldName(node, "source")
	if source == nil {
		source = findChildByType(node, "string")
	}
	if source == nil {
		return
	}
	ctx.addImport(stripQuotes(nodeText(source, ctx.content)), false)
}

// extractJSCallExpression handles require() and dynamic import() calls.
func (ctx *importContext) extractJSCallExpression(node *sitter.Node) {
	fn := findChildByFieldName(node, "function")
	if fn == nil {
		return
	}

	fnText := nodeText(fn, ctx.content)

	switch {
	case fnText == "require":
		// CommonJS require('module')
		args := findChildByFieldName(node, "arguments")
		if args == nil {
			return
		}
		firstArg := firstNamedChild(args)
		if firstArg == nil {
			return
		}
		if firstArg.Type() == "string" {
			ctx.addImport(stripQuotes(nodeText(firstArg, ctx.content)), false)
		} else if firstArg.Type() == "template_string" && !hasTemplateSubstitution(firstArg) {
			ctx.addImport(stripQuotes(nodeText(firstArg, ctx.content)), false)
		}

	case fnText == "import":
		// Dynamic import('module')
		args := findChildByFieldName(node, "arguments")
		if args == nil {
			return
		}
		firstArg := firstNamedChild(args)
		if firstArg == nil {
			ctx.addImport("<dynamic>", false)
			return
		}
		if firstArg.Type() == "string" {
			ctx.addImport(stripQuotes(nodeText(firstArg, ctx.content)), false)
		} else if firstArg.Type() == "template_string" && !hasTemplateSubstitution(firstArg) {
			ctx.addImport(stripQuotes(nodeText(firstArg, ctx.content)), false)
		} else {
			ctx.addImport("<dynamic>", false)
		}
	}
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

func (ctx *importContext) extractPythonImport(node *sitter.Node) {
	switch node.Type() {
	case "import_statement":
		ctx.extractPythonImportStatement(node)
	case "import_from_statement":
		ctx.extractPythonFromImport(node)
	}
}

// extractPythonImportStatement handles `import os` and `import os, sys`.
func (ctx *importContext) extractPythonImportStatement(node *sitter.Node) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Type() == "dotted_name" || child.Type() == "aliased_import" {
			name := child
			if child.Type() == "aliased_import" {
				name = findChildByFieldName(child, "name")
				if name == nil {
					name = child.NamedChild(0)
				}
			}
			if name != nil {
				ctx.addImport(nodeText(name, ctx.content), false)
			}
		}
	}
}

// extractPythonFromImport handles `from os.path import join` and `from .utils import foo`.
func (ctx *importContext) extractPythonFromImport(node *sitter.Node) {
	moduleName := findChildByFieldName(node, "module_name")
	if moduleName != nil {
		ctx.addImport(nodeText(moduleName, ctx.content), false)
		return
	}

	// For `from . import something` — the module is just dots.
	// Walk children to find the relative import prefix.
	text := nodeText(node, ctx.content)
	if strings.HasPrefix(text, "from") {
		// Extract the module path between "from" and "import".
		// Use regex to match the import keyword surrounded by whitespace,
		// handling tabs and multi-space formatting.
		if idx := findPythonImportKeyword(text); idx >= 0 {
			mod := strings.TrimSpace(text[4:idx]) // skip "from"
			if mod != "" {
				ctx.addImport(mod, false)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Go
// ---------------------------------------------------------------------------

func (ctx *importContext) extractGoImport(node *sitter.Node) {
	// import_declaration can contain import_spec or import_spec_list.
	specs := findNamedChildrenByType(node, "import_spec")
	if len(specs) > 0 {
		for _, spec := range specs {
			ctx.extractGoImportSpec(spec)
		}
		return
	}

	// Single import: import "fmt"
	// The path is a direct string child.
	pathNode := findChildByFieldName(node, "path")
	if pathNode != nil {
		ctx.addImport(stripQuotes(nodeText(pathNode, ctx.content)), false)
		return
	}

	// Try finding interpreted_string_literal children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Type() == "import_spec" {
			ctx.extractGoImportSpec(child)
		} else if child.Type() == "import_spec_list" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				spec := child.NamedChild(j)
				if spec != nil && spec.Type() == "import_spec" {
					ctx.extractGoImportSpec(spec)
				}
			}
		}
	}
}

func (ctx *importContext) extractGoImportSpec(spec *sitter.Node) {
	pathNode := findChildByFieldName(spec, "path")
	if pathNode == nil {
		// Fallback: look for interpreted_string_literal or raw_string_literal.
		pathNode = findChildByType(spec, "interpreted_string_literal")
		if pathNode == nil {
			pathNode = findChildByType(spec, "raw_string_literal")
		}
	}
	if pathNode != nil {
		ctx.addImport(stripQuotes(nodeText(pathNode, ctx.content)), false)
	}
}

// ---------------------------------------------------------------------------
// Rust
// ---------------------------------------------------------------------------

func (ctx *importContext) extractRustImport(node *sitter.Node) {
	// use_declaration contains a use path/tree.
	// Get the full text, strip "use " prefix and trailing ";".
	text := strings.TrimSpace(nodeText(node, ctx.content))
	text = strings.TrimPrefix(text, "use ")
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)

	if text == "" {
		return
	}

	// For grouped uses like `use std::{io, fs}`, take the base path.
	if idx := strings.Index(text, "::{"); idx >= 0 {
		text = text[:idx]
	}

	// For `use std::io::Read as _`, strip alias.
	if idx := strings.Index(text, " as "); idx >= 0 {
		text = text[:idx]
	}

	ctx.addImport(text, false)
}

// extractRustModImport handles mod declarations (mod foo;) as internal file imports.
// Inline mod blocks (mod foo { ... }) are skipped since they don't reference another file.
func (ctx *importContext) extractRustModImport(node *sitter.Node) {
	if findChildByType(node, "declaration_list") != nil {
		return
	}
	nameNode := findChildByFieldName(node, "name")
	if nameNode == nil {
		return
	}
	modName := nodeText(nameNode, ctx.content)
	if modName == "" {
		return
	}
	ctx.addImport(modName, true)
}

// ---------------------------------------------------------------------------
// Java
// ---------------------------------------------------------------------------

func (ctx *importContext) extractJavaImport(node *sitter.Node) {
	// import_declaration contains a scoped_identifier or identifier.
	text := strings.TrimSpace(nodeText(node, ctx.content))
	text = strings.TrimPrefix(text, "import ")
	text = strings.TrimPrefix(text, "static ")
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)

	if text != "" {
		ctx.addImport(text, false)
	}
}

// ---------------------------------------------------------------------------
// Kotlin
// ---------------------------------------------------------------------------

func (ctx *importContext) extractKotlinImport(node *sitter.Node) {
	// import_header contains an identifier.
	text := strings.TrimSpace(nodeText(node, ctx.content))
	text = strings.TrimPrefix(text, "import ")
	text = strings.TrimSpace(text)

	// Strip alias: `import foo.bar as Baz`
	if idx := strings.Index(text, " as "); idx >= 0 {
		text = text[:idx]
	}

	if text != "" {
		ctx.addImport(text, false)
	}
}

// ---------------------------------------------------------------------------
// C/C++
// ---------------------------------------------------------------------------

func (ctx *importContext) extractCInclude(node *sitter.Node) {
	// preproc_include has a path child that is either system_lib_string or string_literal.
	pathNode := findChildByFieldName(node, "path")
	if pathNode == nil {
		pathNode = findChildByType(node, "system_lib_string")
		if pathNode == nil {
			pathNode = findChildByType(node, "string_literal")
		}
	}
	if pathNode == nil {
		return
	}

	raw := nodeText(pathNode, ctx.content)
	isQuoted := strings.HasPrefix(raw, "\"")

	source := stripAngleBrackets(stripQuotes(raw))
	if source == "" {
		return
	}

	ctx.addImport(source, isQuoted)
}

// ---------------------------------------------------------------------------
// C#
// ---------------------------------------------------------------------------

func (ctx *importContext) extractCSharpUsing(node *sitter.Node) {
	text := strings.TrimSpace(nodeText(node, ctx.content))
	// Strip qualifiers in the order they appear: global using static ...
	text = strings.TrimPrefix(text, "global ")
	text = strings.TrimPrefix(text, "using ")
	text = strings.TrimPrefix(text, "static ")
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)

	// Skip using aliases like `using X = System.IO.Path;`
	if strings.Contains(text, "=") {
		parts := strings.SplitN(text, "=", 2)
		if len(parts) == 2 {
			text = strings.TrimSpace(parts[1])
		}
	}

	if text != "" {
		ctx.addImport(text, false)
	}
}

// ---------------------------------------------------------------------------
// Swift
// ---------------------------------------------------------------------------

func (ctx *importContext) extractSwiftImport(node *sitter.Node) {
	text := strings.TrimSpace(nodeText(node, ctx.content))
	// Remove @testable attribute before stripping the import keyword.
	text = strings.TrimPrefix(text, "@testable ")
	text = strings.TrimPrefix(text, "import ")
	// Remove kind qualifiers (e.g., `import class Foundation.NSObject`).
	text = strings.TrimPrefix(text, "class ")
	text = strings.TrimPrefix(text, "struct ")
	text = strings.TrimPrefix(text, "enum ")
	text = strings.TrimPrefix(text, "protocol ")
	text = strings.TrimPrefix(text, "func ")
	text = strings.TrimPrefix(text, "var ")
	text = strings.TrimPrefix(text, "let ")
	text = strings.TrimPrefix(text, "typealias ")
	text = strings.TrimSpace(text)

	if text != "" {
		ctx.addImport(text, false)
	}
}

// ---------------------------------------------------------------------------
// Ruby
// ---------------------------------------------------------------------------

func (ctx *importContext) extractRubyRequire(node *sitter.Node) {
	// call node: method name + arguments.
	methodNode := findChildByFieldName(node, "method")
	if methodNode == nil {
		return
	}
	method := nodeText(methodNode, ctx.content)

	if method != "require" && method != "require_relative" {
		return
	}

	args := findChildByFieldName(node, "arguments")
	if args == nil {
		return
	}
	firstArg := firstNamedChild(args)
	if firstArg == nil {
		return
	}

	source := stripQuotes(nodeText(firstArg, ctx.content))
	if source == "" {
		return
	}

	if method == "require_relative" {
		// require_relative is always relative to the current file.
		// Ensure bare names get a "./" prefix for correct path resolution.
		if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
			source = "./" + source
		}
		ctx.addImport(source, true)
	} else {
		ctx.addImport(source, false)
	}
}

// ---------------------------------------------------------------------------
// PHP
// ---------------------------------------------------------------------------

func (ctx *importContext) extractPHPImport(node *sitter.Node) {
	switch node.Type() {
	case "namespace_use_declaration":
		ctx.extractPHPNamespaceUse(node)
	case "include_expression", "include_once_expression", "require_expression", "require_once_expression":
		ctx.extractPHPInclude(node)
	}
}

func (ctx *importContext) extractPHPNamespaceUse(node *sitter.Node) {
	// Extract the qualified name.
	text := strings.TrimSpace(nodeText(node, ctx.content))
	text = strings.TrimPrefix(text, "use ")
	text = strings.TrimPrefix(text, "function ")
	text = strings.TrimPrefix(text, "const ")
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)

	// Handle grouped uses: `use App\{Models\User, Services\Auth}`
	if idx := strings.Index(text, "\\{"); idx >= 0 {
		base := text[:idx]
		endIdx := strings.LastIndex(text, "}")
		if endIdx <= idx+2 {
			// Malformed grouped use — skip gracefully.
			return
		}
		group := text[idx+2 : endIdx]
		for _, part := range strings.Split(group, ",") {
			part = stripPHPAlias(strings.TrimSpace(part))
			if part != "" {
				ctx.addImport(base+"\\"+part, false)
			}
		}
		return
	}

	// Handle comma-separated uses: `use Foo\Bar, Baz\Qux`
	if strings.Contains(text, ",") {
		for _, part := range strings.Split(text, ",") {
			part = stripPHPAlias(strings.TrimSpace(part))
			if part != "" {
				ctx.addImport(part, false)
			}
		}
		return
	}

	text = stripPHPAlias(text)
	if text != "" {
		ctx.addImport(text, false)
	}
}

func (ctx *importContext) extractPHPInclude(node *sitter.Node) {
	// include/require followed by a string, optionally wrapped in
	// parentheses: `include 'f.php'` or `include('f.php')`.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		// Unwrap parenthesized_expression: include('file.php')
		if child.Type() == "parenthesized_expression" {
			child = firstNamedChild(child)
			if child == nil {
				continue
			}
		}
		if child.Type() == "string" || child.Type() == "encapsed_string" {
			source := stripQuotes(nodeText(child, ctx.content))
			if source != "" {
				ctx.addImport(source, false)
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Bash
// ---------------------------------------------------------------------------

func (ctx *importContext) extractBashSource(node *sitter.Node) {
	// command node: first word is the command name.
	nameNode := findChildByFieldName(node, "name")
	if nameNode == nil {
		// Try first child.
		if node.NamedChildCount() > 0 {
			nameNode = node.NamedChild(0)
		}
	}
	if nameNode == nil {
		return
	}

	name := nodeText(nameNode, ctx.content)
	if name != "source" && name != "." {
		return
	}

	// The argument is the second word/named child.
	argNode := findChildByFieldName(node, "argument")
	if argNode == nil {
		// Find the first named child after the command name.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child != nameNode {
				argNode = child
				break
			}
		}
	}
	if argNode == nil {
		return
	}

	source := stripQuotes(nodeText(argNode, ctx.content))
	if source != "" {
		ctx.addImport(source, false)
	}
}

// ---------------------------------------------------------------------------
// HCL (Terraform)
// ---------------------------------------------------------------------------

func (ctx *importContext) extractHCLImport(node *sitter.Node) {
	// block node: the first identifier child gives the block type.
	typeNode := findChildByType(node, "identifier")
	if typeNode == nil {
		return
	}
	blockType := nodeText(typeNode, ctx.content)

	switch blockType {
	case "module":
		ctx.extractHCLModuleSource(node)
	case "terraform":
		ctx.extractHCLRequiredProviders(node)
	}
}

func (ctx *importContext) extractHCLModuleSource(node *sitter.Node) {
	// Look for source = "..." attribute inside the block body.
	source := ctx.findHCLAttributeValue(node, "source")
	if source != "" {
		ctx.addImport(source, false)
	}
}

func (ctx *importContext) extractHCLRequiredProviders(node *sitter.Node) {
	// Look for required_providers block inside terraform block body.
	body := findChildByType(node, "body")
	if body == nil {
		return
	}

	// Walk body children for blocks named "required_providers".
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child == nil || child.Type() != "block" {
			continue
		}
		typeNode := findChildByType(child, "identifier")
		if typeNode == nil || nodeText(typeNode, ctx.content) != "required_providers" {
			continue
		}
		ctx.extractHCLProviderSources(child)
	}
}

func (ctx *importContext) extractHCLProviderSources(node *sitter.Node) {
	body := findChildByType(node, "body")
	if body == nil {
		return
	}
	// Each provider is an attribute whose value is an object containing "source".
	// HCL AST: attribute → expression → object → object_elem (attribute with "source").
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child == nil || child.Type() != "attribute" {
			continue
		}
		// The value expression should contain a source field.
		// Try to find source in the nested object.
		expr := findChildByType(child, "expression")
		if expr == nil {
			continue
		}
		obj := findChildByType(expr, "object")
		if obj == nil {
			continue
		}
		// Look for source attribute inside the object.
		for j := 0; j < int(obj.ChildCount()); j++ {
			elem := obj.Child(j)
			if elem == nil {
				continue
			}
			if elem.Type() == "object_elem" {
				keyNode := findChildByType(elem, "identifier")
				if keyNode != nil && nodeText(keyNode, ctx.content) == "source" {
					val := ctx.extractHCLStringValue(elem)
					if val != "" {
						ctx.addImport(val, false)
					}
				}
			}
		}
	}
}

// findHCLAttributeValue looks for `key = "value"` in a block's body.
// HCL AST: attribute → identifier + expression → literal_value → string_lit → template_literal.
func (ctx *importContext) findHCLAttributeValue(node *sitter.Node, key string) string {
	if node == nil {
		return ""
	}

	body := findChildByType(node, "body")
	if body == nil {
		body = node
	}

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "attribute" {
			nameNode := findChildByType(child, "identifier")
			if nameNode != nil && nodeText(nameNode, ctx.content) == key {
				return ctx.extractHCLStringValue(child)
			}
		}
	}
	return ""
}

// extractHCLStringValue extracts the string value from an HCL attribute.
// Traverses: attribute → expression → literal_value → string_lit → template_literal.
func (ctx *importContext) extractHCLStringValue(attrNode *sitter.Node) string {
	expr := findChildByType(attrNode, "expression")
	if expr == nil {
		// Fallback: look for string_lit directly.
		return ctx.findHCLTemplateLiteral(attrNode)
	}
	litVal := findChildByType(expr, "literal_value")
	if litVal == nil {
		return ctx.findHCLTemplateLiteral(expr)
	}
	return ctx.findHCLTemplateLiteral(litVal)
}

// findHCLTemplateLiteral finds a template_literal descendant within a string_lit node.
func (ctx *importContext) findHCLTemplateLiteral(node *sitter.Node) string {
	strLit := findChildByType(node, "string_lit")
	if strLit == nil {
		strLit = node
	}
	tmpl := findChildByType(strLit, "template_literal")
	if tmpl != nil {
		return nodeText(tmpl, ctx.content)
	}
	// Fallback: strip quotes from the string_lit text.
	if strLit.Type() == "string_lit" {
		return stripQuotes(nodeText(strLit, ctx.content))
	}
	return ""
}

// ---------------------------------------------------------------------------
// Dockerfile
// ---------------------------------------------------------------------------

func (ctx *importContext) extractDockerfileFrom(node *sitter.Node) {
	// from_instruction: FROM <image>
	// Get image name from the image_spec child.
	imageSpec := findChildByType(node, "image_spec")
	if imageSpec != nil {
		// Use the full image_spec text rather than the AST "name" field,
		// because the grammar misparses registry:port as name:tag.
		text := stripDockerTag(nodeText(imageSpec, ctx.content))
		if text != "" {
			ctx.addImport(text, false)
		}
		return
	}

	// Fallback: parse from text.
	text := strings.TrimSpace(nodeText(node, ctx.content))
	text = strings.TrimPrefix(text, "FROM ")
	text = strings.TrimPrefix(text, "from ")
	fields := strings.Fields(text)
	if len(fields) > 0 {
		image := stripDockerTag(fields[0])
		ctx.addImport(image, false)
	}
}

// ---------------------------------------------------------------------------
// CSS/SCSS
// ---------------------------------------------------------------------------

func (ctx *importContext) extractCSSImport(node *sitter.Node) {
	// @import url("...") or @import "..."
	// May include trailing media queries: @import "foo.css" screen and (...);
	text := strings.TrimSpace(nodeText(node, ctx.content))
	text = strings.TrimPrefix(text, "@import ")
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)

	var source string
	if strings.HasPrefix(text, "url(") {
		// Handle url(...) — content may be quoted or unquoted.
		inner := strings.TrimSpace(strings.TrimPrefix(text, "url("))
		if len(inner) > 1 && (inner[0] == '"' || inner[0] == '\'' || inner[0] == '`') {
			// Quoted URL: extract by matching quote so parentheses in
			// the URL (e.g. url("file(1).css")) don't corrupt the path.
			quote := inner[0]
			if end := strings.IndexByte(inner[1:], quote); end >= 0 {
				source = inner[1 : end+1]
			}
		} else if idx := strings.Index(inner, ")"); idx >= 0 {
			// Unquoted URL: cut at first ) (unquoted URLs cannot
			// contain parentheses per the CSS spec).
			source = strings.TrimSpace(inner[:idx])
		}
	} else if len(text) > 1 && (text[0] == '"' || text[0] == '\'' || text[0] == '`') {
		// Quoted path, possibly followed by media query — extract just the
		// quoted portion and ignore everything after the closing quote.
		quote := text[0]
		if end := strings.IndexByte(text[1:], quote); end >= 0 {
			source = text[1 : end+1]
		}
	} else {
		source = text
	}

	if source != "" {
		ctx.addImport(source, false)
	}
}

// ---------------------------------------------------------------------------
// HTML
// ---------------------------------------------------------------------------

func (ctx *importContext) extractHTMLImport(node *sitter.Node) {
	// element node: check if it's a <script> or <link> tag.
	tagNode := findChildByType(node, "start_tag")
	if tagNode == nil {
		tagNode = findChildByType(node, "self_closing_tag")
	}
	if tagNode == nil {
		return
	}

	tagName := ""
	nameNode := findChildByType(tagNode, "tag_name")
	if nameNode != nil {
		tagName = nodeText(nameNode, ctx.content)
	}

	switch tagName {
	case "script":
		src := ctx.findHTMLAttribute(tagNode, "src")
		if src != "" {
			ctx.addImport(src, false)
		}
	case "link":
		// Only capture stylesheet and modulepreload links as imports.
		// Skip favicon, canonical, preconnect, etc.
		// HTML rel attribute is case-insensitive per spec and can contain
		// multiple space-separated values (e.g. rel="stylesheet preload").
		rel := strings.ToLower(ctx.findHTMLAttribute(tagNode, "rel"))
		if !hasRelToken(rel, "stylesheet") && !hasRelToken(rel, "modulepreload") && !hasRelToken(rel, "preload") {
			return
		}
		href := ctx.findHTMLAttribute(tagNode, "href")
		if href != "" {
			ctx.addImport(href, false)
		}
	}
}

// findHTMLAttribute finds the value of an attribute in a tag node.
func (ctx *importContext) findHTMLAttribute(tagNode *sitter.Node, attrName string) string {
	for i := 0; i < int(tagNode.NamedChildCount()); i++ {
		child := tagNode.NamedChild(i)
		if child == nil || child.Type() != "attribute" {
			continue
		}
		nameNode := findChildByType(child, "attribute_name")
		if nameNode != nil && nodeText(nameNode, ctx.content) == attrName {
			valNode := findChildByType(child, "attribute_value")
			if valNode == nil {
				valNode = findChildByType(child, "quoted_attribute_value")
			}
			if valNode != nil {
				return stripQuotes(nodeText(valNode, ctx.content))
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Markdown
// ---------------------------------------------------------------------------

func (ctx *importContext) extractMarkdownLink(node *sitter.Node) {
	// inline_link: [text](url)
	dest := findChildByType(node, "link_destination")
	if dest == nil {
		return
	}
	url := nodeText(dest, ctx.content)
	if url == "" {
		return
	}
	// Skip external URLs, anchors, and mailto links — only capture
	// local file references (relative paths) as imports.
	// Use case-insensitive check for protocol prefixes (HTTP://, HTTPS://, etc.).
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(url, "#") || strings.HasPrefix(lower, "mailto:") {
		return
	}
	ctx.addImport(url, false)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hasRelToken checks whether a lowercased HTML rel attribute (possibly
// containing multiple space-separated tokens) includes the given token.
func hasRelToken(rel, token string) bool {
	for _, t := range strings.Fields(rel) {
		if t == token {
			return true
		}
	}
	return false
}

// stripQuotes removes surrounding single or double quotes from a string.
func stripQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '`' && s[len(s)-1] == '`') {
		return s[1 : len(s)-1]
	}
	return s
}

// stripDockerTag strips the tag or digest suffix from a Docker image reference,
// handling registry ports correctly (e.g., "registry:5000/image:tag" → "registry:5000/image").
func stripDockerTag(s string) string {
	// Strip digest first: image@sha256:...
	if idx := strings.Index(s, "@"); idx > 0 {
		s = s[:idx]
	}
	// Strip tag: only the colon after the last slash is a tag separator.
	// "registry:5000/image:tag" → last slash at position 14, last colon at 20 → strip.
	// "registry:5000/image" → last slash at 14, last colon at 8 → no strip (port, not tag).
	lastSlash := strings.LastIndex(s, "/")
	lastColon := strings.LastIndex(s, ":")
	if lastColon > lastSlash {
		s = s[:lastColon]
	}
	return s
}

// stripAngleBrackets removes surrounding < > from a string.
func stripAngleBrackets(s string) string {
	if len(s) >= 2 && s[0] == '<' && s[len(s)-1] == '>' {
		return s[1 : len(s)-1]
	}
	return s
}

// stripPHPAlias removes a trailing ` as Alias` from a PHP use clause.
// Case-insensitive because PHP keywords are case-insensitive.
func stripPHPAlias(s string) string {
	lower := strings.ToLower(s)
	if idx := strings.Index(lower, " as "); idx >= 0 {
		return s[:idx]
	}
	return s
}

// hasTemplateSubstitution reports whether a template_string node contains
// interpolation expressions (e.g. `./path/${var}`).
func hasTemplateSubstitution(node *sitter.Node) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if node.NamedChild(i).Type() == "template_substitution" {
			return true
		}
	}
	return false
}

// findPythonImportKeyword finds the position of the `import` keyword in a
// Python from-import statement, handling tabs and multi-space whitespace.
// Returns the byte index of the `import` keyword, or -1 if not found.
func findPythonImportKeyword(text string) int {
	// Search for "import" preceded by whitespace and followed by whitespace,
	// EOL, or valid Python tokens like *, (.
	// This avoids matching "import" inside identifiers like "reimport_utils".
	searchFrom := 4 // skip "from"
	for {
		idx := strings.Index(text[searchFrom:], "import")
		if idx < 0 {
			return -1
		}
		pos := searchFrom + idx
		// Check that it's preceded by whitespace (word boundary).
		if pos > 0 && (text[pos-1] == ' ' || text[pos-1] == '\t') {
			// Check that it's followed by a non-identifier character.
			end := pos + 6
			if end >= len(text) {
				return pos
			}
			ch := text[end]
			if ch == ' ' || ch == '\t' || ch == '(' || ch == '*' || ch == '\\' || ch == '\n' {
				return pos
			}
		}
		searchFrom = pos + 6
	}
}

// firstNamedChild returns the first named child, or nil.
func firstNamedChild(node *sitter.Node) *sitter.Node {
	if node == nil || node.NamedChildCount() == 0 {
		return nil
	}
	return node.NamedChild(0)
}
