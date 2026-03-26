package extractors

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// exportContext carries state through export extraction.
type exportContext struct {
	content    []byte
	langID     string
	langConfig *registry.LanguageConfig
	symbols    []parser.Symbol
	exports    []parser.Export
	symbolIdx  map[string]string // name → symbolID for linking
}

// ExtractExports extracts export declarations from a parsed file.
// For JS/TS, it walks the AST for export_statement nodes.
// For other Tier 1 languages, it derives exports from the symbols list
// using the language's export strategy.
// Tier 2/3 languages return nil.
func ExtractExports(root *sitter.Node, content []byte, langID string, symbols []parser.Symbol) []parser.Export {
	if root == nil {
		return nil
	}

	langConfig, ok := registry.GetLanguageConfig(langID)
	if !ok {
		return nil
	}

	// Tier 2/3: no exports.
	if langConfig.Tier != registry.Tier1 {
		return nil
	}

	ctx := &exportContext{
		content:    content,
		langID:     langID,
		langConfig: langConfig,
		symbols:    symbols,
		symbolIdx:  buildSymbolIndex(symbols),
	}

	if jsLikeLanguages[langID] {
		ctx.extractJSExports(root)
	} else {
		ctx.extractConventionExports(root)
	}

	if len(ctx.exports) == 0 {
		return nil
	}
	parser.SortExports(ctx.exports)
	return ctx.exports
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// generateExportID produces a deterministic "exp_<hash16>" identifier.
func generateExportID(kind, name string, line, column int32) string {
	key := fmt.Sprintf("%s:%s:%d:%d", kind, name, line, column)
	hash := parser.StableHash(key)
	return "exp_" + hash[:16]
}

// buildSymbolIndex maps symbol Name → SymbolID, preferring top-level symbols.
// First top-level symbol wins; nested symbols only fill gaps.
func buildSymbolIndex(symbols []parser.Symbol) map[string]string {
	idx := make(map[string]string, len(symbols))
	topLevel := make(map[string]bool, len(symbols))
	for i := range symbols {
		s := &symbols[i]
		if s.ParentSymbolID == "" {
			// Top-level: only set if not already claimed by another top-level.
			if !topLevel[s.Name] {
				idx[s.Name] = s.SymbolID
				topLevel[s.Name] = true
			}
		} else if _, exists := idx[s.Name]; !exists {
			// Nested: fill in only if no entry yet.
			idx[s.Name] = s.SymbolID
		}
	}
	return idx
}

// findLinkedSymbolID looks up a symbol ID by name.
func (ctx *exportContext) findLinkedSymbolID(name string) string {
	return ctx.symbolIdx[name]
}

// addExport appends an export entry.
func (ctx *exportContext) addExport(kind, exportedName, localName, symbolID, sourceModule string, line, column int32) {
	ctx.exports = append(ctx.exports, parser.Export{
		ExportID:     generateExportID(kind, exportedName, line, column),
		ExportKind:   kind,
		ExportedName: exportedName,
		LocalName:    localName,
		SymbolID:     symbolID,
		SourceModule: sourceModule,
		Line:         line,
		Column:       column,
	})
}

// ---------------------------------------------------------------------------
// JS/TS AST-based extraction
// ---------------------------------------------------------------------------

// extractJSExports walks the AST for export_statement nodes.
// Also recurses into ambient module declarations (declare module "foo" { ... })
// and namespace blocks (namespace Foo { ... }).
func (ctx *exportContext) extractJSExports(root *sitter.Node) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "export_statement":
			ctx.handleExportStatement(child)
		case "module", "internal_module":
			// Recurse into declare module / namespace blocks.
			if body := findChildByType(child, "statement_block"); body != nil {
				ctx.extractJSExports(body)
			}
		case "ambient_declaration":
			// TS: `declare module "foo" { ... }` wraps module in ambient_declaration.
			ctx.extractJSExports(child)
		}
	}
}

// handleExportStatement dispatches an export_statement to the appropriate handler.
func (ctx *exportContext) handleExportStatement(node *sitter.Node) {
	isTypeOnly := ctx.isTypeOnlyExport(node)
	sourceModule := ctx.getSourceModule(node)

	// export_clause: export { foo, bar } or export { foo } from './mod'
	if clause := findChildByType(node, "export_clause"); clause != nil {
		kind := "NAMED"
		if sourceModule != "" {
			kind = "REEXPORT"
		}
		if isTypeOnly {
			kind = "TYPE_ONLY"
		}
		ctx.handleExportClause(clause, kind, sourceModule, node)
		return
	}

	// export * from './mod' or export * as ns from './mod'
	if hasChildWithType(node, "*") || findChildByType(node, "namespace_export") != nil {
		ctx.handleExportAll(node, sourceModule)
		return
	}

	// export default ...
	if hasChildWithType(node, "default") {
		ctx.handleDefaultExport(node)
		return
	}

	// Direct named export: export function foo(), export const x, etc.
	ctx.handleDirectNamedExport(node)
}

// isTypeOnlyExport checks for the "type" keyword in an export statement.
func (ctx *exportContext) isTypeOnlyExport(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		// "type" appears as an anonymous child between "export" and the clause.
		if !child.IsNamed() && nodeText(child, ctx.content) == "type" {
			return true
		}
	}
	return false
}

// getSourceModule extracts the module path from a re-export (from '...').
func (ctx *exportContext) getSourceModule(node *sitter.Node) string {
	// Look for source field.
	if src := node.ChildByFieldName("source"); src != nil {
		return stripQuotes(nodeText(src, ctx.content))
	}
	// Fallback: look for a string child.
	if str := findChildByType(node, "string"); str != nil {
		return stripQuotes(nodeText(str, ctx.content))
	}
	return ""
}

// handleExportClause processes export { foo, bar } / export { foo } from './mod'.
func (ctx *exportContext) handleExportClause(clause *sitter.Node, kind, sourceModule string, stmt *sitter.Node) {
	specifiers := findNamedChildrenByType(clause, "export_specifier")
	for _, spec := range specifiers {
		exportedName, localName := ctx.extractSpecifierNames(spec)
		if exportedName == "" {
			continue
		}

		specKind := kind
		// Per-specifier "type" modifier (TS 4.5+): export { type Foo, Bar }
		if specKind != "TYPE_ONLY" && hasSpecifierTypeModifier(spec, ctx.content) {
			specKind = "TYPE_ONLY"
		}
		if exportedName == "default" && specKind != "TYPE_ONLY" {
			specKind = "DEFAULT"
		}

		var symbolID string
		if sourceModule == "" {
			symbolID = ctx.findLinkedSymbolID(localName)
		}

		line := nodeStartLine(spec)
		col := int32(spec.StartPoint().Column)
		ctx.addExport(specKind, exportedName, localName, symbolID, sourceModule, line, col)
	}
}

// extractSpecifierNames extracts the exported and local names from an export_specifier.
// export { foo }         → exported=foo, local=foo
// export { foo as bar }  → exported=bar, local=foo
func (ctx *exportContext) extractSpecifierNames(spec *sitter.Node) (exported, local string) {
	nameNode := spec.ChildByFieldName("name")
	aliasNode := spec.ChildByFieldName("alias")

	if nameNode != nil {
		local = nodeText(nameNode, ctx.content)
	}

	if aliasNode != nil {
		exported = nodeText(aliasNode, ctx.content)
	} else {
		exported = local
	}
	return exported, local
}

// hasSpecifierTypeModifier checks if an export_specifier has a per-specifier
// "type" modifier (TS 4.5+): export { type Foo, Bar }.
func hasSpecifierTypeModifier(spec *sitter.Node, content []byte) bool {
	for i := 0; i < int(spec.ChildCount()); i++ {
		child := spec.Child(i)
		if child != nil && !child.IsNamed() && nodeText(child, content) == "type" {
			return true
		}
	}
	return false
}

// handleExportAll processes export * from './mod' and export * as ns from './mod'.
func (ctx *exportContext) handleExportAll(node *sitter.Node, sourceModule string) {
	exportedName := "*"
	if nsExport := findChildByType(node, "namespace_export"); nsExport != nil {
		// export * as ns from './mod'
		for i := 0; i < int(nsExport.NamedChildCount()); i++ {
			child := nsExport.NamedChild(i)
			if child != nil && child.Type() == "identifier" {
				exportedName = nodeText(child, ctx.content)
				break
			}
		}
	}

	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)
	ctx.addExport("EXPORT_ALL", exportedName, "", "", sourceModule, line, col)
}

// handleDefaultExport processes export default ... statements.
func (ctx *exportContext) handleDefaultExport(node *sitter.Node) {
	name := "default"
	localName := ""

	// Try to find the declared name from the exported value.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "function_declaration", "generator_function_declaration":
			if n := child.ChildByFieldName("name"); n != nil {
				localName = nodeText(n, ctx.content)
			}
		case "class_declaration":
			if n := child.ChildByFieldName("name"); n != nil {
				localName = nodeText(n, ctx.content)
			}
		case "identifier":
			localName = nodeText(child, ctx.content)
		case "assignment_expression":
			// export default foo = bar
			if left := child.ChildByFieldName("left"); left != nil && left.Type() == "identifier" {
				localName = nodeText(left, ctx.content)
			}
		}
	}

	symbolID := ""
	if localName != "" {
		symbolID = ctx.findLinkedSymbolID(localName)
	}

	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)
	ctx.addExport("DEFAULT", name, localName, symbolID, "", line, col)
}

// handleDirectNamedExport processes export function foo(), export const x, etc.
func (ctx *exportContext) handleDirectNamedExport(node *sitter.Node) {
	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "lexical_declaration", "variable_declaration":
			ctx.handleVariableExport(child, node)

		// Type-only declarations.
		case "interface_declaration", "type_alias_declaration":
			ctx.emitNamedDeclExport(child, "TYPE_ONLY", line, col)

		// Value declarations.
		case "function_declaration", "generator_function_declaration",
			"class_declaration", "enum_declaration", "internal_module":
			ctx.emitNamedDeclExport(child, "NAMED", line, col)
		}
	}
}

// emitNamedDeclExport emits an export for a declaration node that has a "name" field.
func (ctx *exportContext) emitNamedDeclExport(decl *sitter.Node, kind string, line, col int32) {
	n := decl.ChildByFieldName("name")
	if n == nil {
		return
	}
	name := nodeText(n, ctx.content)
	symbolID := ctx.findLinkedSymbolID(name)
	ctx.addExport(kind, name, name, symbolID, "", line, col)
}

// handleVariableExport processes export const x = 1, export let y = 2,
// and destructured exports like export const { a, b } = obj.
func (ctx *exportContext) handleVariableExport(decl *sitter.Node, exportStmt *sitter.Node) {
	declarators := findNamedChildrenByType(decl, "variable_declarator")
	for _, d := range declarators {
		nameNode := d.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		line := nodeStartLine(exportStmt)
		col := int32(exportStmt.StartPoint().Column)

		switch nameNode.Type() {
		case "object_pattern":
			// export const { a, b } = obj → emit each binding.
			ctx.extractDestructuredNames(nameNode, line, col)
		case "array_pattern":
			// export const [a, b] = arr → emit each binding.
			ctx.extractDestructuredNames(nameNode, line, col)
		default:
			// Simple identifier binding.
			name := nodeText(nameNode, ctx.content)
			symbolID := ctx.findLinkedSymbolID(name)
			ctx.addExport("NAMED", name, name, symbolID, "", line, col)
		}
	}
}

// extractDestructuredNames emits exports for each identifier in a destructuring pattern.
func (ctx *exportContext) extractDestructuredNames(pattern *sitter.Node, line, col int32) {
	for i := 0; i < int(pattern.NamedChildCount()); i++ {
		child := pattern.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "identifier", "shorthand_property_identifier_pattern":
			name := nodeText(child, ctx.content)
			symbolID := ctx.findLinkedSymbolID(name)
			ctx.addExport("NAMED", name, name, symbolID, "", line, col)
		case "pair_pattern":
			// { key: alias } → export the alias (value side).
			// { key: { nested } } → recurse into nested destructuring.
			if val := child.ChildByFieldName("value"); val != nil {
				switch val.Type() {
				case "identifier":
					name := nodeText(val, ctx.content)
					symbolID := ctx.findLinkedSymbolID(name)
					ctx.addExport("NAMED", name, name, symbolID, "", line, col)
				case "object_pattern", "array_pattern":
					ctx.extractDestructuredNames(val, line, col)
				}
			}
		case "assignment_pattern":
			// { a = 1 } → export the left-hand identifier (default value).
			if left := child.ChildByFieldName("left"); left != nil {
				switch left.Type() {
				case "identifier", "shorthand_property_identifier_pattern":
					name := nodeText(left, ctx.content)
					symbolID := ctx.findLinkedSymbolID(name)
					ctx.addExport("NAMED", name, name, symbolID, "", line, col)
				case "object_pattern", "array_pattern":
					ctx.extractDestructuredNames(left, line, col)
				}
			}
		case "rest_pattern":
			// { ...rest } or [...rest] → export the rest identifier.
			if id := findChildByType(child, "identifier"); id != nil {
				name := nodeText(id, ctx.content)
				symbolID := ctx.findLinkedSymbolID(name)
				ctx.addExport("NAMED", name, name, symbolID, "", line, col)
			}
		case "object_pattern", "array_pattern":
			// Nested destructuring — recurse.
			ctx.extractDestructuredNames(child, line, col)
		}
	}
}

// ---------------------------------------------------------------------------
// Convention-based extraction (non-JS/TS Tier 1)
// ---------------------------------------------------------------------------

// extractConventionExports derives exports from the symbols list.
func (ctx *exportContext) extractConventionExports(root *sitter.Node) {
	strategy := ctx.langConfig.Export

	// Python special case: check for __all__.
	if ctx.langID == "python" {
		if allNames := extractPythonDunderAll(root, ctx.content); allNames != nil {
			ctx.buildPythonExportsFromAll(allNames)
			return
		}
	}

	// Ruby special case: detect private/protected methods.
	var rubyPrivate map[string]bool
	if ctx.langID == "ruby" {
		rubyPrivate = detectRubyPrivateMethods(root, ctx.content)
	}

	// C/C++ special case: detect static declarations via AST
	// (Flags.IsStatic is unreliable because `static` is in a named
	// storage_class_specifier node that hasModifierKeyword doesn't find).
	var cStaticNames map[string]bool
	if ctx.langID == "c" || ctx.langID == "cpp" {
		cStaticNames = detectCStaticDeclarations(root, ctx.content)
	}

	// C# special case: detect public declarations via AST
	// (Flags.IsExported is unreliable because C# uses `modifier` named node
	// not `accessibility_modifier` that hasExportKeyword expects).
	var csharpPublic map[string]bool
	if ctx.langID == "csharp" {
		csharpPublic = detectCSharpPublicDeclarations(root, ctx.content)
	}

	// For C#, build a set of namespace symbol IDs so we can allow symbols
	// inside namespaces through the top-level filter. Namespaces are
	// organizational containers, not access boundaries.
	var namespaceIDs map[string]bool
	if ctx.langID == "csharp" {
		namespaceIDs = make(map[string]bool)
		for i := range ctx.symbols {
			if ctx.symbols[i].Kind == "namespace" {
				namespaceIDs[ctx.symbols[i].SymbolID] = true
			}
		}
	}

	for i := range ctx.symbols {
		sym := &ctx.symbols[i]

		// Only top-level symbols (or C# symbols whose parent is a namespace).
		if sym.ParentSymbolID != "" {
			if !namespaceIDs[sym.ParentSymbolID] {
				continue
			}
		}

		if !ctx.shouldExportSymbol(sym, strategy, rubyPrivate, cStaticNames, csharpPublic) {
			continue
		}

		ctx.addExport("NAMED", sym.Name, sym.Name, sym.SymbolID, "", sym.StartLine, 0)
	}
}

// shouldExportSymbol determines if a symbol should be exported based on the language's strategy.
func (ctx *exportContext) shouldExportSymbol(sym *parser.Symbol, strategy registry.ExportStrategy, rubyPrivate, cStaticNames, csharpPublic map[string]bool) bool {
	switch strategy.Type {
	case "keyword":
		// C#: use AST-based detection (symbol extractor misses modifier node).
		if ctx.langID == "csharp" && csharpPublic != nil {
			return csharpPublic[sym.Name]
		}
		// PHP: top-level classes and functions are always accessible.
		if ctx.langID == "php" {
			return true
		}
		// Rust pub, Java public, Swift public — require Flags.
		if sym.Flags == nil {
			return false
		}
		if sym.Flags.IsExported {
			return true
		}
		// Swift: also check for "open" modifier.
		if ctx.langID == "swift" {
			return containsModifier(sym.Modifiers, "open")
		}
		return false

	case "convention":
		// Go: uppercase first letter — requires Flags.
		return sym.Flags != nil && sym.Flags.IsExported

	case "prefix":
		// Python: non-underscore prefix — requires Flags.
		return sym.Flags != nil && sym.Flags.IsExported

	case "all_public":
		// C/C++: exclude static symbols (detected via AST).
		if (ctx.langID == "c" || ctx.langID == "cpp") && cStaticNames != nil {
			return !cStaticNames[sym.Name]
		}
		// Kotlin: exclude private/internal.
		if ctx.langID == "kotlin" {
			return !containsModifier(sym.Modifiers, "private") &&
				!containsModifier(sym.Modifiers, "internal")
		}
		// Ruby: exclude private/protected methods.
		if ctx.langID == "ruby" && rubyPrivate != nil {
			return !rubyPrivate[sym.Name]
		}
		return true

	case "none":
		return false
	}
	return false
}

// containsModifier checks if a modifier list contains a specific keyword.
func containsModifier(modifiers []string, keyword string) bool {
	for _, m := range modifiers {
		if m == keyword {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Python __all__ detection
// ---------------------------------------------------------------------------

// extractPythonDunderAll walks top-level statements looking for __all__ = [...],
// __all__ += [...], and __all__.extend([...]).
// Returns the list of exported names, or nil if __all__ is not found.
func extractPythonDunderAll(root *sitter.Node, content []byte) []string {
	var names []string
	found := false

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil || child.Type() != "expression_statement" {
			continue
		}
		for j := 0; j < int(child.NamedChildCount()); j++ {
			expr := child.NamedChild(j)
			if expr == nil {
				continue
			}

			switch expr.Type() {
			case "assignment":
				// __all__ = ["a", "b"] — replaces any prior value.
				left := expr.ChildByFieldName("left")
				if left == nil || nodeText(left, content) != "__all__" {
					continue
				}
				right := expr.ChildByFieldName("right")
				if right == nil {
					continue
				}
				if right.Type() != "list" && right.Type() != "tuple" {
					continue
				}
				found = true
				names = extractCollectionStrings(right, content)

			case "augmented_assignment":
				// __all__ += ["c"]
				left := expr.ChildByFieldName("left")
				if left == nil || nodeText(left, content) != "__all__" {
					continue
				}
				right := expr.ChildByFieldName("right")
				if right == nil {
					continue
				}
				if right.Type() != "list" && right.Type() != "tuple" {
					continue
				}
				found = true
				names = append(names, extractCollectionStrings(right, content)...)

			case "call":
				// __all__.extend(["d"])
				fn := expr.ChildByFieldName("function")
				if fn == nil || fn.Type() != "attribute" {
					continue
				}
				obj := fn.ChildByFieldName("object")
				attr := fn.ChildByFieldName("attribute")
				if obj == nil || attr == nil {
					continue
				}
				if nodeText(obj, content) != "__all__" || nodeText(attr, content) != "extend" {
					continue
				}
				args := expr.ChildByFieldName("arguments")
				if args == nil {
					continue
				}
				firstArg := firstNamedChild(args)
				if firstArg == nil {
					continue
				}
				if firstArg.Type() != "list" && firstArg.Type() != "tuple" {
					continue
				}
				found = true
				names = append(names, extractCollectionStrings(firstArg, content)...)
			}
		}
	}

	if !found {
		return nil
	}
	// Return non-nil empty slice if __all__ was found but empty.
	if names == nil {
		return make([]string, 0)
	}
	return names
}

// extractCollectionStrings extracts string values from a Python list or tuple literal.
// Returns a non-nil empty slice for empty collections so callers can distinguish
// "empty __all__" (export nothing) from "absent __all__" (nil).
func extractCollectionStrings(node *sitter.Node, content []byte) []string {
	names := make([]string, 0)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil || child.Type() != "string" {
			continue
		}
		val := stripQuotes(nodeText(child, content))
		if val != "" {
			names = append(names, val)
		}
	}
	return names
}

// buildPythonExportsFromAll creates exports for symbols matching __all__ names.
func (ctx *exportContext) buildPythonExportsFromAll(allNames []string) {
	allSet := make(map[string]bool, len(allNames))
	for _, n := range allNames {
		allSet[n] = true
	}

	for i := range ctx.symbols {
		sym := &ctx.symbols[i]
		if sym.ParentSymbolID != "" {
			continue
		}
		if !allSet[sym.Name] {
			continue
		}
		ctx.addExport("NAMED", sym.Name, sym.Name, sym.SymbolID, "", sym.StartLine, 0)
	}
}

// ---------------------------------------------------------------------------
// Ruby private/protected detection
// ---------------------------------------------------------------------------

// detectRubyPrivateMethods walks the AST to find private/protected method names.
// Returns a set of method names that are private or protected.
func detectRubyPrivateMethods(root *sitter.Node, content []byte) map[string]bool {
	private := make(map[string]bool)
	walkRubyBody(root, content, private)
	return private
}

// walkRubyBody walks a Ruby body (class/module/program) tracking visibility state.
func walkRubyBody(node *sitter.Node, content []byte, private map[string]bool) {
	if node == nil {
		return
	}

	// For class/module bodies, track visibility state.
	body := node
	if node.Type() == "class" || node.Type() == "module" {
		body = findChildByType(node, "body_statement")
		if body == nil {
			return
		}
	}

	visibility := "public"

	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "call":
			methodName := ""
			if m := child.ChildByFieldName("method"); m != nil {
				methodName = nodeText(m, content)
			}

			if methodName == "private" || methodName == "protected" {
				// Check if this wraps a method definition (private def foo).
				if wrappedMethod := findMethodInCall(child, content); wrappedMethod != "" {
					private[wrappedMethod] = true
				} else if args := findChildByType(child, "argument_list"); args != nil {
					// private :foo, :bar style.
					markSymbolArgs(args, content, private)
				} else {
					// Bare private/protected — affects subsequent methods.
					visibility = methodName
				}
			}

		case "method", "singleton_method":
			if visibility == "private" || visibility == "protected" {
				if nameNode := child.ChildByFieldName("name"); nameNode != nil {
					private[nodeText(nameNode, content)] = true
				}
			}

		case "class", "module":
			// Recurse into nested class/module bodies.
			walkRubyBody(child, content, private)
		}
	}
}

// findMethodInCall checks if a call node wraps a method definition (private def foo).
func findMethodInCall(call *sitter.Node, content []byte) string {
	// Look for a method/singleton_method as an argument or child.
	if args := findChildByType(call, "argument_list"); args != nil {
		for i := 0; i < int(args.NamedChildCount()); i++ {
			child := args.NamedChild(i)
			if child != nil && (child.Type() == "method" || child.Type() == "singleton_method") {
				if nameNode := child.ChildByFieldName("name"); nameNode != nil {
					return nodeText(nameNode, content)
				}
			}
		}
	}
	// Also check direct named children.
	for i := 0; i < int(call.NamedChildCount()); i++ {
		child := call.NamedChild(i)
		if child != nil && (child.Type() == "method" || child.Type() == "singleton_method") {
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				return nodeText(nameNode, content)
			}
		}
	}
	return ""
}

// markSymbolArgs marks symbol arguments (private :foo, :bar) as private.
func markSymbolArgs(args *sitter.Node, content []byte, private map[string]bool) {
	for i := 0; i < int(args.NamedChildCount()); i++ {
		child := args.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Type() == "simple_symbol" {
			// :foo → "foo"
			text := nodeText(child, content)
			text = strings.TrimPrefix(text, ":")
			if text != "" {
				private[text] = true
			}
		}
	}
}

// ---------------------------------------------------------------------------
// C/C++ static detection
// ---------------------------------------------------------------------------

// detectCStaticDeclarations walks the AST to find declarations with the
// `static` storage class specifier. Returns a set of symbol names that are
// file-local (static).
// The symbol extractor's Flags.IsStatic is unreliable for C/C++ because
// `static` is inside a named `storage_class_specifier` node that
// hasModifierKeyword (which only checks anonymous children) does not find.
func detectCStaticDeclarations(root *sitter.Node, content []byte) map[string]bool {
	static := make(map[string]bool)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		if !hasStorageClassStatic(child, content) {
			continue
		}
		name := extractCDeclarationName(child, content)
		if name != "" {
			static[name] = true
		}
	}
	return static
}

// hasStorageClassStatic checks if a node has a storage_class_specifier child
// containing "static".
func hasStorageClassStatic(node *sitter.Node, content []byte) bool {
	spec := findChildByType(node, "storage_class_specifier")
	if spec == nil {
		return false
	}
	return strings.Contains(nodeText(spec, content), "static")
}

// extractCDeclarationName extracts the identifier name from a C/C++ declaration.
func extractCDeclarationName(node *sitter.Node, content []byte) string {
	switch node.Type() {
	case "function_definition":
		if decl := findChildByType(node, "function_declarator"); decl != nil {
			if id := findChildByType(decl, "identifier"); id != nil {
				return nodeText(id, content)
			}
		}
	case "declaration":
		// Variable declarations: look for init_declarator or declarator.
		if initDecl := findChildByType(node, "init_declarator"); initDecl != nil {
			if id := findChildByType(initDecl, "identifier"); id != nil {
				return nodeText(id, content)
			}
		}
		if id := findChildByType(node, "identifier"); id != nil {
			return nodeText(id, content)
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// C# public modifier detection
// ---------------------------------------------------------------------------

// detectCSharpPublicDeclarations walks the AST to find declarations with the
// `public` modifier. Returns a set of symbol names that are public.
// The symbol extractor's Flags.IsExported is unreliable for C# because the
// grammar uses a named `modifier` node (not `accessibility_modifier` that
// the hasExportKeyword function expects).
func detectCSharpPublicDeclarations(root *sitter.Node, content []byte) map[string]bool {
	public := make(map[string]bool)
	walkCSharpDeclarations(root, content, public)
	return public
}

// walkCSharpDeclarations recursively finds C# declarations with `public` modifier.
func walkCSharpDeclarations(node *sitter.Node, content []byte, public map[string]bool) {
	if node == nil {
		return
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "class_declaration", "struct_declaration", "interface_declaration",
			"enum_declaration", "record_declaration", "delegate_declaration",
			"namespace_declaration":
			if hasCSharpModifier(child, content, "public") {
				if name := extractCSharpName(child, content); name != "" {
					public[name] = true
				}
			}
			// Recurse into namespaces.
			if child.Type() == "namespace_declaration" {
				if body := findChildByType(child, "declaration_list"); body != nil {
					walkCSharpDeclarations(body, content, public)
				}
			}
		}
	}
}

// hasCSharpModifier checks if a C# declaration has a specific modifier.
func hasCSharpModifier(node *sitter.Node, content []byte, modifier string) bool {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Type() == "modifier" {
			if nodeText(child, content) == modifier {
				return true
			}
		}
	}
	return false
}

// extractCSharpName extracts the name identifier from a C# declaration.
func extractCSharpName(node *sitter.Node, content []byte) string {
	if n := node.ChildByFieldName("name"); n != nil {
		return nodeText(n, content)
	}
	if n := findChildByType(node, "identifier"); n != nil {
		return nodeText(n, content)
	}
	return ""
}
