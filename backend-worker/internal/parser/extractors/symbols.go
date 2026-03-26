package extractors

import (
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// jsLikeLanguages is the set of JS/TS family languages.
var jsLikeLanguages = map[string]bool{
	"javascript": true,
	"typescript": true,
	"jsx":        true,
	"tsx":        true,
}

// jsxLanguages is the set of JSX/TSX languages (for React-specific flags).
var jsxLanguages = map[string]bool{
	"jsx": true,
	"tsx": true,
}

// extractionContext carries state through the recursive AST walk.
type extractionContext struct {
	content    []byte
	langID     string
	langConfig *registry.LanguageConfig
	symbols    []parser.Symbol
	// inExport tracks whether we're inside an export statement (JS/TS).
	inExport bool
	// inDefaultExport tracks whether we're inside a default export.
	inDefaultExport bool
	// pendingMethodReceivers stores Go method symbol indices with their
	// unresolved receiver type names for post-walk resolution.
	pendingMethodReceivers []pendingReceiver
}

// pendingReceiver records a Go method that couldn't resolve its receiver
// type during the walk (e.g. struct defined later in the file).
type pendingReceiver struct {
	symbolIdx    int
	receiverType string
}

// ExtractSymbols walks the tree-sitter AST and returns all symbols found.
// It uses the language registry to determine which node types map to symbols
// and how to handle language-specific features like exports and documentation.
func ExtractSymbols(root *sitter.Node, content []byte, langID string) []parser.Symbol {
	if root == nil {
		return nil
	}

	langConfig, ok := registry.GetLanguageConfig(langID)
	if !ok {
		return nil
	}

	ctx := &extractionContext{
		content:    content,
		langID:     langID,
		langConfig: langConfig,
	}

	ctx.walkNode(root, nil, 0)

	// Post-walk: resolve Go methods whose receiver struct was defined later.
	for _, pr := range ctx.pendingMethodReceivers {
		if parent := ctx.findSymbolByName(pr.receiverType, ""); parent != nil {
			sym := &ctx.symbols[pr.symbolIdx]
			sym.ParentSymbolID = parent.SymbolID
			sym.QualifiedName = parent.QualifiedName + "." + sym.Name
		}
	}

	parser.SortSymbols(ctx.symbols)
	return ctx.symbols
}

// walkNode recursively visits the AST. For each node whose type appears in
// SymbolNodeTypes, it creates a Symbol.
func (ctx *extractionContext) walkNode(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	if node == nil || depth > 50 {
		return
	}

	nodeType := node.Type()

	// JS/TS: Handle export_statement by unwrapping inner declaration.
	if jsLikeLanguages[ctx.langID] && nodeType == "export_statement" {
		ctx.handleExportStatement(node, parentSymbol, depth)
		return
	}

	// Check if this node type is in SymbolNodeTypes.
	if kind, ok := ctx.langConfig.SymbolNodeTypes[nodeType]; ok {
		switch kind {
		case "_unwrap":
			ctx.handleUnwrap(node, parentSymbol, depth)
			return
		case "_recurse":
			ctx.handleRecurse(node, parentSymbol, depth)
			return
		case "_type_spec_dispatch":
			ctx.handleTypeSpecDispatch(node, parentSymbol, depth)
			return
		case "_declaration_dispatch":
			ctx.handleDeclarationDispatch(node, parentSymbol, depth)
			return
		case "_element_dispatch":
			ctx.handleElementDispatch(node, parentSymbol, depth)
			return
		default:
			// For JS/TS arrow_function: only extract if it's assigned to a variable.
			if nodeType == "arrow_function" {
				parent := node.Parent()
				if parent == nil || (parent.Type() != "variable_declarator" && parent.Type() != "assignment_expression" && parent.Type() != "pair") {
					// Arrow function not assigned to a named variable — skip as a top-level symbol.
					// Still recurse into children.
					ctx.walkChildren(node, parentSymbol, depth)
					return
				}
			}

			// For lexical_declaration/variable_declaration: check if it wraps an arrow function.
			if nodeType == "lexical_declaration" || nodeType == "variable_declaration" {
				ctx.handleVariableDeclaration(node, kind, parentSymbol, depth)
				return
			}

			// Go method_declaration: resolve receiver type as parent.
			if nodeType == "method_declaration" && parentSymbol == nil {
				if recv := ctx.goReceiverType(node); recv != "" {
					if found := ctx.findSymbolByName(recv, ""); found != nil {
						parentSymbol = found
					} else {
						// Struct may be defined later in the file; defer resolution.
						sym := ctx.extractSymbol(node, kind, parentSymbol)
						if sym != nil {
							ctx.pendingMethodReceivers = append(ctx.pendingMethodReceivers, pendingReceiver{
								symbolIdx:    len(ctx.symbols) - 1,
								receiverType: recv,
							})
							if isContainerKind(kind) {
								ctx.recurseBody(node, sym, depth)
							}
						}
						return
					}
				}
			}

			sym := ctx.extractSymbol(node, kind, parentSymbol)
			if sym != nil && isContainerKind(kind) {
				ctx.recurseBody(node, sym, depth)
			}
			return
		}
	}

	// Not a symbol node — recurse into children.
	ctx.walkChildren(node, parentSymbol, depth)
}

// walkChildren recurses into all named children.
func (ctx *extractionContext) walkChildren(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ctx.walkNode(node.NamedChild(i), parentSymbol, depth+1)
	}
}

// recurseBody recurses into the body of a container (class, interface, namespace, enum)
// to find nested symbols like methods and fields.
func (ctx *extractionContext) recurseBody(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	// Look for a body child first.
	bodyNames := []string{"body", "class_body", "members", "declaration_list", "block"}
	for _, name := range bodyNames {
		if body := node.ChildByFieldName(name); body != nil {
			ctx.walkChildren(body, parentSymbol, depth+1)
			return
		}
	}
	// Fallback: recurse into all named children.
	ctx.walkChildren(node, parentSymbol, depth+1)
}

// isContainerKind returns true for symbol kinds that may contain nested symbols.
func isContainerKind(kind string) bool {
	switch kind {
	case "class", "interface", "namespace", "enum":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Special marker handlers
// ---------------------------------------------------------------------------

// handleUnwrap processes _unwrap nodes: skip the wrapper, process inner declarations.
// Used for: Python decorated_definition, Go type_declaration, C++ template_declaration.
func (ctx *extractionContext) handleUnwrap(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ctx.walkNode(node.NamedChild(i), parentSymbol, depth)
	}
}

// handleRecurse processes _recurse nodes: recurse into children looking for symbols.
// The container itself does NOT emit a symbol.
// Used for: Rust impl_item (find methods), Kotlin enum_class_body.
func (ctx *extractionContext) handleRecurse(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	// For Rust impl_item, find the actual struct/enum symbol to use as parent.
	if node.Type() == "impl_item" {
		typeName := ctx.implTypeName(node)
		if typeName != "" {
			// Look up the struct/enum, scoped to the enclosing module if any.
			scopeID := ""
			if parentSymbol != nil {
				scopeID = parentSymbol.SymbolID
			}
			parent := ctx.findSymbolByName(typeName, scopeID)
			if parent == nil {
				// Fallback: create a synthetic parent (struct defined later or in another file).
				parent = &parser.Symbol{
					SymbolID:      generateSymbolID(typeName, nodeStartLine(node), nodeEndLine(node)),
					Name:          typeName,
					QualifiedName: typeName,
				}
			}
			for i := 0; i < int(node.NamedChildCount()); i++ {
				ctx.walkNode(node.NamedChild(i), parent, depth+1)
			}
			return
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ctx.walkNode(node.NamedChild(i), parentSymbol, depth+1)
	}
}

// implTypeName extracts the type name from a Rust impl_item.
func (ctx *extractionContext) implTypeName(node *sitter.Node) string {
	// impl Type { ... } or impl Trait for Type { ... }
	// The type is in the "type" field.
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		return nodeText(typeNode, ctx.content)
	}
	// Fallback: look for a type_identifier child.
	if id := findChildByType(node, "type_identifier"); id != nil {
		return nodeText(id, ctx.content)
	}
	return ""
}

// goReceiverType extracts the receiver type name from a Go method_declaration.
// e.g. "func (h *UserHandler) ServeHTTP(...)" → "UserHandler"
func (ctx *extractionContext) goReceiverType(node *sitter.Node) string {
	params := node.ChildByFieldName("receiver")
	if params == nil {
		return ""
	}
	// The receiver is a parameter_list containing a parameter_declaration.
	// The type is either a type_identifier or a pointer_type wrapping one.
	for i := 0; i < int(params.NamedChildCount()); i++ {
		param := params.NamedChild(i)
		// Look for type_identifier directly or inside pointer_type.
		if id := findChildByType(param, "type_identifier"); id != nil {
			return nodeText(id, ctx.content)
		}
		if ptr := findChildByType(param, "pointer_type"); ptr != nil {
			if id := findChildByType(ptr, "type_identifier"); id != nil {
				return nodeText(id, ctx.content)
			}
		}
	}
	return ""
}

// findSymbolByName searches for a symbol by name, optionally scoped to a
// parent. When scopeID is non-empty, it first searches within that scope
// (e.g. Rust nested module), then falls back to top-level. This prevents
// incorrectly binding an impl block to a same-named type in a different scope.
func (ctx *extractionContext) findSymbolByName(name string, scopeID string) *parser.Symbol {
	// If scoped, search within that scope first.
	if scopeID != "" {
		for i := range ctx.symbols {
			if ctx.symbols[i].Name == name && ctx.symbols[i].ParentSymbolID == scopeID {
				return &ctx.symbols[i]
			}
		}
	}
	// Fall back to top-level (ParentSymbolID == "").
	for i := range ctx.symbols {
		if ctx.symbols[i].Name == name && ctx.symbols[i].ParentSymbolID == "" {
			return &ctx.symbols[i]
		}
	}
	return nil
}

// handleTypeSpecDispatch processes Go's type_spec node.
// Dispatches: struct_type → class, interface_type → interface, else → type_alias.
func (ctx *extractionContext) handleTypeSpecDispatch(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	kind := "type_alias"
	if hasChildWithType(node, "struct_type") {
		kind = "class"
	} else if hasChildWithType(node, "interface_type") {
		kind = "interface"
	}
	sym := ctx.extractSymbol(node, kind, parentSymbol)
	if sym != nil && isContainerKind(kind) {
		ctx.recurseBody(node, sym, depth)
	}
}

// handleDeclarationDispatch processes C/C++ declaration nodes.
// If it has a function_declarator descendant → function, else → variable.
func (ctx *extractionContext) handleDeclarationDispatch(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	kind := "variable"
	if hasDescendantOfType(node, "function_declarator", 3) {
		kind = "function"
	}
	ctx.extractSymbol(node, kind, parentSymbol)
}

// handleElementDispatch processes HTML elements: only extract script/style.
func (ctx *extractionContext) handleElementDispatch(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	startTag := findChildByType(node, "start_tag")
	if startTag == nil {
		return
	}
	tagName := findChildByType(startTag, "tag_name")
	if tagName == nil {
		return
	}
	name := nodeText(tagName, ctx.content)
	if name == "script" || name == "style" {
		ctx.extractSymbol(node, "variable", parentSymbol)
	}
}

// handleExportStatement handles JS/TS export_statement by unwrapping the inner declaration.
func (ctx *extractionContext) handleExportStatement(node *sitter.Node, parentSymbol *parser.Symbol, depth int) {
	// Check for "default" keyword.
	hasDefault := hasChildWithType(node, "default")

	prevExport := ctx.inExport
	prevDefault := ctx.inDefaultExport
	ctx.inExport = true
	ctx.inDefaultExport = hasDefault
	defer func() {
		ctx.inExport = prevExport
		ctx.inDefaultExport = prevDefault
	}()

	// Recurse into the inner declaration(s).
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		childType := child.Type()
		// Skip export_clause ({ foo, bar }) — handled by exports extractor.
		if childType == "export_clause" {
			continue
		}
		ctx.walkNode(child, parentSymbol, depth+1)
	}
}

// handleVariableDeclaration handles lexical_declaration/variable_declaration nodes.
// If it wraps an arrow function, extract as function. Otherwise extract as variable.
func (ctx *extractionContext) handleVariableDeclaration(node *sitter.Node, kind string, parentSymbol *parser.Symbol, depth int) {
	declarators := findNamedChildrenByType(node, "variable_declarator")
	if len(declarators) == 0 {
		// No declarators — extract as is.
		ctx.extractSymbol(node, kind, parentSymbol)
		return
	}

	for _, decl := range declarators {
		value := decl.ChildByFieldName("value")
		if value != nil && (value.Type() == "arrow_function" || value.Type() == "function") {
			// Arrow function / function expression assigned to variable → extract as function.
			sym := ctx.extractSymbolFromDeclarator(decl, value, "function", parentSymbol)
			if sym != nil && isContainerKind("function") {
				ctx.recurseBody(value, sym, depth)
			}
		} else {
			// Regular variable.
			ctx.extractSymbolFromDeclarator(decl, nil, "variable", parentSymbol)
		}
	}
}

// ---------------------------------------------------------------------------
// Symbol extraction
// ---------------------------------------------------------------------------

// extractSymbol creates a Symbol from a node and appends it to ctx.symbols.
func (ctx *extractionContext) extractSymbol(node *sitter.Node, kind string, parentSymbol *parser.Symbol) *parser.Symbol {
	name := ctx.extractName(node, kind)
	if name == "" {
		return nil
	}

	startLine := nodeStartLine(node)
	endLine := nodeEndLine(node)
	symbolID := generateSymbolID(name, startLine, endLine)

	qualifiedName := name
	parentSymbolID := ""
	if parentSymbol != nil {
		qualifiedName = parentSymbol.QualifiedName + "." + name
		parentSymbolID = parentSymbol.SymbolID
	}

	sym := parser.Symbol{
		SymbolID:       symbolID,
		Name:           name,
		QualifiedName:  qualifiedName,
		Kind:           kind,
		Signature:      truncateSignature(node, ctx.content),
		StartLine:      startLine,
		EndLine:        endLine,
		DocText:        ctx.extractDocComment(node),
		SymbolHash:     parser.StableHashBytes(ctx.content[node.StartByte():node.EndByte()]),
		ParentSymbolID: parentSymbolID,
		Flags:          ctx.extractFlags(node, name, kind),
		Modifiers:      ctx.extractModifiers(node),
	}

	ctx.symbols = append(ctx.symbols, sym)
	return &ctx.symbols[len(ctx.symbols)-1]
}

// extractSymbolFromDeclarator creates a Symbol from a variable_declarator node,
// using the optional valueNode for flags/hash.
func (ctx *extractionContext) extractSymbolFromDeclarator(decl *sitter.Node, valueNode *sitter.Node, kind string, parentSymbol *parser.Symbol) *parser.Symbol {
	nameNode := decl.ChildByFieldName("name")
	if nameNode == nil {
		nameNode = findChildByType(decl, "identifier")
	}
	if nameNode == nil {
		return nil
	}
	name := nodeText(nameNode, ctx.content)
	if name == "" {
		return nil
	}

	// Use the parent declaration node for line range (covers const/let keyword).
	declParent := decl.Parent()
	var startLine, endLine int32
	if declParent != nil && (declParent.Type() == "lexical_declaration" || declParent.Type() == "variable_declaration") {
		startLine = nodeStartLine(declParent)
		endLine = nodeEndLine(declParent)
	} else {
		startLine = nodeStartLine(decl)
		endLine = nodeEndLine(decl)
	}

	symbolID := generateSymbolID(name, startLine, endLine)

	qualifiedName := name
	parentSymbolID := ""
	if parentSymbol != nil {
		qualifiedName = parentSymbol.QualifiedName + "." + name
		parentSymbolID = parentSymbol.SymbolID
	}

	// Use the value node for hash/signature if available, otherwise use the declarator.
	hashNode := decl
	sigNode := decl
	if valueNode != nil {
		hashNode = valueNode
	}
	if declParent != nil && (declParent.Type() == "lexical_declaration" || declParent.Type() == "variable_declaration") {
		sigNode = declParent
	}

	// Extract doc comment from the parent declaration or its predecessor.
	docNode := decl
	if declParent != nil && (declParent.Type() == "lexical_declaration" || declParent.Type() == "variable_declaration") {
		docNode = declParent
	}

	flagNode := valueNode
	if flagNode == nil {
		flagNode = decl
	}

	sym := parser.Symbol{
		SymbolID:       symbolID,
		Name:           name,
		QualifiedName:  qualifiedName,
		Kind:           kind,
		Signature:      truncateSignature(sigNode, ctx.content),
		StartLine:      startLine,
		EndLine:        endLine,
		DocText:        ctx.extractDocComment(docNode),
		SymbolHash:     parser.StableHashBytes(ctx.content[hashNode.StartByte():hashNode.EndByte()]),
		ParentSymbolID: parentSymbolID,
		Flags:          ctx.extractDeclaratorFlags(flagNode, name, kind),
		Modifiers:      ctx.extractModifiers(decl),
	}

	ctx.symbols = append(ctx.symbols, sym)
	return &ctx.symbols[len(ctx.symbols)-1]
}

// ---------------------------------------------------------------------------
// Name extraction
// ---------------------------------------------------------------------------

// extractName extracts the symbol name from a node, handling language-specific patterns.
func (ctx *extractionContext) extractName(node *sitter.Node, kind string) string {
	nodeType := node.Type()

	// CSS rule_set: extract selector text.
	if nodeType == "rule_set" {
		if sel := findChildByType(node, "selectors"); sel != nil {
			return strings.TrimSpace(nodeText(sel, ctx.content))
		}
		// Fallback: first child text.
		if node.NamedChildCount() > 0 {
			return strings.TrimSpace(nodeText(node.NamedChild(0), ctx.content))
		}
		return ""
	}

	// CSS keyframes_statement / media_statement.
	if nodeType == "keyframes_statement" {
		if name := findChildByFieldName(node, "name"); name != nil {
			return nodeText(name, ctx.content)
		}
		// Keyframes name is after @keyframes keyword.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "keyframes_name" {
				return nodeText(child, ctx.content)
			}
		}
		return ""
	}

	if nodeType == "media_statement" {
		return "@media"
	}

	// Markdown headings.
	if nodeType == "atx_heading" || nodeType == "setext_heading" {
		if inline := findChildByType(node, "inline"); inline != nil {
			return strings.TrimSpace(nodeText(inline, ctx.content))
		}
		// Fallback: extract text from heading content (skip the # markers).
		text := nodeText(node, ctx.content)
		text = strings.TrimLeft(text, "# ")
		text = strings.TrimSpace(text)
		if idx := strings.IndexByte(text, '\n'); idx >= 0 {
			text = text[:idx]
		}
		return text
	}

	// HCL blocks: concatenate block type + labels.
	if nodeType == "block" && ctx.langID == "hcl" {
		return ctx.hclBlockName(node)
	}

	// YAML block_mapping_pair: key field.
	if nodeType == "block_mapping_pair" {
		if key := findChildByFieldName(node, "key"); key != nil {
			return nodeText(key, ctx.content)
		}
		return ""
	}

	// TOML pair: key field. TOML table: name from first child.
	if nodeType == "pair" {
		if key := findChildByFieldName(node, "key"); key != nil {
			return nodeText(key, ctx.content)
		}
		// JSON pair: key is a string.
		if node.NamedChildCount() > 0 {
			keyNode := node.NamedChild(0)
			text := nodeText(keyNode, ctx.content)
			return strings.Trim(text, `"'`)
		}
		return ""
	}

	if nodeType == "table" {
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "bare_key" {
				return nodeText(child, ctx.content)
			}
			if child != nil && child.Type() == "dotted_key" {
				return nodeText(child, ctx.content)
			}
		}
		return ""
	}

	// HTML element: use tag name.
	if nodeType == "element" {
		startTag := findChildByType(node, "start_tag")
		if startTag != nil {
			tagName := findChildByType(startTag, "tag_name")
			if tagName != nil {
				return nodeText(tagName, ctx.content)
			}
		}
		return ""
	}

	// SQL CREATE statements: extract the object name.
	if strings.HasPrefix(nodeType, "create_") {
		return ctx.sqlObjectName(node)
	}

	// Dockerfile instructions.
	if strings.HasSuffix(nodeType, "_instruction") {
		return ctx.dockerfileName(node)
	}

	// Go type_spec and type_alias: name is the type_identifier child.
	if nodeType == "type_spec" || nodeType == "type_alias" {
		if id := findChildByFieldName(node, "name"); id != nil {
			return nodeText(id, ctx.content)
		}
		if id := findChildByType(node, "type_identifier"); id != nil {
			return nodeText(id, ctx.content)
		}
		return ""
	}

	// Ruby method / singleton_method: name field.
	if nodeType == "method" || nodeType == "singleton_method" {
		if name := findChildByFieldName(node, "name"); name != nil {
			return nodeText(name, ctx.content)
		}
	}

	// Ruby class / module: name field.
	if nodeType == "class" || nodeType == "module" {
		if name := findChildByFieldName(node, "name"); name != nil {
			return nodeText(name, ctx.content)
		}
	}

	// Python/Ruby assignment.
	if nodeType == "assignment" {
		if left := findChildByFieldName(node, "left"); left != nil {
			return nodeText(left, ctx.content)
		}
		return ""
	}

	// C/C++ function_definition: name is inside function_declarator child.
	if nodeType == "function_definition" {
		if decltor := findChildByType(node, "function_declarator"); decltor != nil {
			if id := findChildByType(decltor, "identifier"); id != nil {
				return nodeText(id, ctx.content)
			}
			if id := findChildByType(decltor, "field_identifier"); id != nil {
				return nodeText(id, ctx.content)
			}
		}
	}

	// Java/C#/C++ field_declaration: the identifier is inside variable_declarator,
	// not a direct child. Without this, the generic fallback finds type_identifier
	// (the field type) instead of the actual field name.
	if nodeType == "field_declaration" {
		if decltor := findChildByType(node, "variable_declarator"); decltor != nil {
			if id := findChildByType(decltor, "identifier"); id != nil {
				return nodeText(id, ctx.content)
			}
		}
	}

	// General case: try ChildByFieldName("name"), then look for identifier children.
	if name := node.ChildByFieldName("name"); name != nil {
		return nodeText(name, ctx.content)
	}

	// Look for common identifier child types.
	for _, idType := range []string{"identifier", "type_identifier", "simple_identifier", "name", "property_identifier"} {
		if id := findChildByType(node, idType); id != nil {
			return nodeText(id, ctx.content)
		}
	}

	return ""
}

// hclBlockName extracts the name from an HCL block node.
// e.g., resource "aws_instance" "web" → "aws_instance.web"
func (ctx *extractionContext) hclBlockName(node *sitter.Node) string {
	var parts []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "identifier":
			if len(parts) == 0 {
				// First identifier is the block type — use as a prefix but skip in name for now.
				parts = append(parts, nodeText(child, ctx.content))
			}
		case "string_lit":
			text := nodeText(child, ctx.content)
			text = strings.Trim(text, `"`)
			parts = append(parts, text)
		case "body", "{", "}":
			// Stop at body.
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ".")
}

// sqlObjectName extracts the object name from a SQL CREATE statement.
func (ctx *extractionContext) sqlObjectName(node *sitter.Node) string {
	// Look for identifier or object_reference child.
	if name := node.ChildByFieldName("name"); name != nil {
		return nodeText(name, ctx.content)
	}
	for _, idType := range []string{"identifier", "object_reference", "table_reference"} {
		if id := findChildByType(node, idType); id != nil {
			return nodeText(id, ctx.content)
		}
	}
	// Fallback: extract name from source text after CREATE keyword.
	text := nodeText(node, ctx.content)
	text = strings.ToUpper(text)
	for _, kw := range []string{"TABLE", "FUNCTION", "PROCEDURE", "VIEW", "INDEX", "TYPE"} {
		idx := strings.Index(text, kw)
		if idx >= 0 {
			rest := strings.TrimSpace(string(ctx.content[node.StartByte()+uint32(idx)+uint32(len(kw)):node.EndByte()]))
			if kwIdx := strings.IndexByte(rest, '('); kwIdx >= 0 {
				rest = rest[:kwIdx]
			}
			if kwIdx := strings.IndexByte(rest, ' '); kwIdx >= 0 {
				rest = rest[:kwIdx]
			}
			rest = strings.TrimSpace(rest)
			if rest != "" {
				return rest
			}
		}
	}
	return ""
}

// dockerfileName extracts a name from a Dockerfile instruction node.
func (ctx *extractionContext) dockerfileName(node *sitter.Node) string {
	nodeType := node.Type()
	switch nodeType {
	case "from_instruction":
		// FROM image:tag AS stage
		if img := findChildByType(node, "image_spec"); img != nil {
			return nodeText(img, ctx.content)
		}
		return ""
	case "arg_instruction":
		if name := findChildByType(node, "unquoted_string"); name != nil {
			text := nodeText(name, ctx.content)
			if idx := strings.IndexByte(text, '='); idx >= 0 {
				text = text[:idx]
			}
			return strings.TrimSpace(text)
		}
		return ""
	case "env_instruction":
		if pair := findChildByType(node, "env_pair"); pair != nil {
			if name := pair.ChildByFieldName("name"); name != nil {
				return nodeText(name, ctx.content)
			}
			// First child is often the var name.
			for i := 0; i < int(pair.ChildCount()); i++ {
				child := pair.Child(i)
				if child != nil && child.Type() == "unquoted_string" {
					return nodeText(child, ctx.content)
				}
			}
		}
		return ""
	case "label_instruction":
		if pair := findChildByType(node, "label_pair"); pair != nil {
			if key := pair.ChildByFieldName("key"); key != nil {
				return nodeText(key, ctx.content)
			}
		}
		return ""
	}
	return ""
}

// ---------------------------------------------------------------------------
// Doc comment extraction
// ---------------------------------------------------------------------------

// extractDocComment extracts documentation comments based on the language's DocCommentStyle.
func (ctx *extractionContext) extractDocComment(node *sitter.Node) string {
	style := ctx.langConfig.DocCommentStyle

	switch style {
	case registry.DocJSDoc, registry.DocDoxygen:
		return ctx.extractJSDocComment(node)
	case registry.DocDocstring:
		return ctx.extractDocstring(node)
	case registry.DocTripleSlash, registry.DocXMLDoc:
		return ctx.extractTripleSlashComment(node)
	case registry.DocHash:
		return ctx.extractHashComment(node)
	default:
		return ""
	}
}

// extractJSDocComment extracts /** ... */ style doc comments.
func (ctx *extractionContext) extractJSDocComment(node *sitter.Node) string {
	isJSDoc := func(s *sitter.Node) bool {
		st := s.Type()
		return (st == "comment" || st == "block_comment") && strings.HasPrefix(nodeText(s, ctx.content), "/**")
	}
	siblings := prevNamedSiblings(node, 1, isJSDoc)
	if len(siblings) == 0 {
		// For exported declarations (e.g. "export interface User"), the node is
		// the inner declaration inside export_statement. The JSDoc comment is a
		// sibling of the export_statement, not the inner node. Check the parent.
		if p := node.Parent(); p != nil && p.Type() == "export_statement" {
			siblings = prevNamedSiblings(p, 1, isJSDoc)
		}
	}
	if len(siblings) == 0 {
		return ""
	}
	return nodeText(siblings[0], ctx.content)
}

// extractDocstring extracts Python-style docstrings from the first expression in the body.
func (ctx *extractionContext) extractDocstring(node *sitter.Node) string {
	// Find the body/block child.
	body := node.ChildByFieldName("body")
	if body == nil {
		body = findChildByType(node, "block")
	}
	if body == nil {
		return ""
	}

	// The docstring is the first expression_statement containing a string.
	if body.NamedChildCount() == 0 {
		return ""
	}
	first := body.NamedChild(0)
	if first == nil || first.Type() != "expression_statement" {
		return ""
	}
	if first.NamedChildCount() == 0 {
		return ""
	}
	strNode := first.NamedChild(0)
	if strNode == nil || strNode.Type() != "string" {
		return ""
	}
	text := nodeText(strNode, ctx.content)
	// Strip triple quotes.
	text = strings.TrimPrefix(text, `"""`)
	text = strings.TrimPrefix(text, `'''`)
	text = strings.TrimSuffix(text, `"""`)
	text = strings.TrimSuffix(text, `'''`)
	return strings.TrimSpace(text)
}

// extractTripleSlashComment extracts /// style doc comments (Rust, Swift, C# XML doc).
// It skips over attribute_item nodes (e.g. #[derive(...)]) that sit between
// the doc comment and the declaration.
func (ctx *extractionContext) extractTripleSlashComment(node *sitter.Node) string {
	var comments []*sitter.Node
	cur := node.PrevNamedSibling()
	for cur != nil && len(comments) < 50 {
		// Skip Rust attributes like #[derive(...)].
		if cur.Type() == "attribute_item" {
			cur = cur.PrevNamedSibling()
			continue
		}
		if cur.Type() != "line_comment" && cur.Type() != "comment" {
			break
		}
		text := nodeText(cur, ctx.content)
		if !strings.HasPrefix(text, "///") {
			break
		}
		comments = append(comments, cur)
		cur = cur.PrevNamedSibling()
	}
	if len(comments) == 0 {
		return ""
	}
	// Reverse to source order.
	for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
		comments[i], comments[j] = comments[j], comments[i]
	}
	var lines []string
	for _, s := range comments {
		text := nodeText(s, ctx.content)
		text = strings.TrimPrefix(text, "///")
		text = strings.TrimPrefix(text, " ")
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

// extractHashComment extracts # style doc comments (Ruby, Bash).
func (ctx *extractionContext) extractHashComment(node *sitter.Node) string {
	siblings := prevNamedSiblings(node, 50, func(s *sitter.Node) bool {
		if s.Type() != "comment" {
			return false
		}
		text := nodeText(s, ctx.content)
		return strings.HasPrefix(text, "#")
	})
	if len(siblings) == 0 {
		return ""
	}
	var lines []string
	for _, s := range siblings {
		text := nodeText(s, ctx.content)
		text = strings.TrimPrefix(text, "#")
		text = strings.TrimPrefix(text, " ")
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Export detection and flag computation
// ---------------------------------------------------------------------------

// extractFlags computes all SymbolFlags for a symbol.
func (ctx *extractionContext) extractFlags(node *sitter.Node, name string, kind string) *parser.SymbolFlags {
	flags := &parser.SymbolFlags{}

	// Export detection.
	flags.IsExported = ctx.isExported(node, name)
	// IsDefaultExport only applies to the direct export target, not nested members.
	flags.IsDefaultExport = ctx.inDefaultExport && isDirectExportChild(node)

	// Async.
	flags.IsAsync = ctx.isAsync(node)

	// Generator (JS/TS only).
	if jsLikeLanguages[ctx.langID] {
		flags.IsGenerator = ctx.isGenerator(node)
	}

	// Static.
	flags.IsStatic = ctx.hasModifierKeyword(node, "static")

	// Abstract.
	flags.IsAbstract = ctx.hasModifierKeyword(node, "abstract")

	// Readonly.
	flags.IsReadonly = ctx.hasModifierKeyword(node, "readonly")

	// Arrow function.
	flags.IsArrowFunction = node.Type() == "arrow_function"

	// React component like (JSX/TSX only).
	if jsxLanguages[ctx.langID] || jsLikeLanguages[ctx.langID] {
		flags.IsReactComponentLike = ctx.isReactComponentLike(node, name)
	}

	// Hook like (JS/TS/JSX/TSX only).
	if jsLikeLanguages[ctx.langID] {
		flags.IsHookLike = isHookLike(name)
	}

	return flags
}

// extractDeclaratorFlags computes flags for a symbol extracted from a variable declarator.
func (ctx *extractionContext) extractDeclaratorFlags(node *sitter.Node, name string, kind string) *parser.SymbolFlags {
	flags := &parser.SymbolFlags{}

	flags.IsExported = ctx.isExported(node, name)
	// IsDefaultExport only applies to the direct export target, not nested members.
	// For declarator nodes the export_statement is a grandparent/great-grandparent.
	flags.IsDefaultExport = ctx.inDefaultExport && hasAncestorOfType(node, "export_statement")

	if node != nil {
		flags.IsAsync = ctx.isAsync(node)
		flags.IsGenerator = ctx.isGenerator(node)
		flags.IsArrowFunction = node.Type() == "arrow_function"

		if jsxLanguages[ctx.langID] || jsLikeLanguages[ctx.langID] {
			flags.IsReactComponentLike = ctx.isReactComponentLike(node, name)
		}
	}

	if jsLikeLanguages[ctx.langID] {
		flags.IsHookLike = isHookLike(name)
	}

	return flags
}

// isExported determines if a symbol is exported based on the language's ExportStrategy.
func (ctx *extractionContext) isExported(node *sitter.Node, name string) bool {
	// If we're already inside an export statement (JS/TS), it's exported.
	if ctx.inExport {
		return true
	}

	strategy := ctx.langConfig.Export
	switch strategy.Type {
	case "keyword":
		return ctx.hasExportKeyword(node, strategy.Keyword)
	case "convention":
		if strategy.Convention == "uppercase_first_letter" && len(name) > 0 {
			return unicode.IsUpper(rune(name[0]))
		}
		return false
	case "prefix":
		if strategy.PrivatePrefix != "" && len(name) > 0 {
			return !strings.HasPrefix(name, strategy.PrivatePrefix)
		}
		return true
	case "all_public":
		return true
	case "none":
		return false
	}
	return false
}

// hasExportKeyword checks if the node or an ancestor contains the export keyword.
func (ctx *extractionContext) hasExportKeyword(node *sitter.Node, keyword string) bool {
	if node == nil {
		return false
	}

	// JS/TS: Check parent for export_statement.
	if keyword == "export" {
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "export_statement" {
				return true
			}
			parent = parent.Parent()
		}
		return false
	}

	// Rust: Check for visibility_modifier child.
	if keyword == "pub" {
		if vis := findChildByType(node, "visibility_modifier"); vis != nil {
			text := nodeText(vis, ctx.content)
			return strings.HasPrefix(text, "pub")
		}
		return false
	}

	// Java/C#/PHP/Swift: Check for keyword in modifiers/children.
	if ctx.hasModifierKeyword(node, keyword) {
		return true
	}

	// Check accessibility_modifier for C#.
	if mod := findChildByType(node, "accessibility_modifier"); mod != nil {
		return nodeText(mod, ctx.content) == keyword
	}
	// Check modifiers node (Java, Kotlin).
	if mods := findChildByType(node, "modifiers"); mods != nil {
		for i := 0; i < int(mods.ChildCount()); i++ {
			child := mods.Child(i)
			if child != nil && nodeText(child, ctx.content) == keyword {
				return true
			}
		}
	}

	return false
}

// isAsync checks if a node represents an async function/method.
func (ctx *extractionContext) isAsync(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	// Check for "async" keyword as a direct child.
	if hasChildWithType(node, "async") {
		return true
	}
	// Check node text starts with "async ".
	text := nodeText(node, ctx.content)
	if strings.HasPrefix(text, "async ") {
		return true
	}
	// Python: Check parent decorated_definition or the function itself.
	if ctx.langID == "python" {
		parent := node.Parent()
		if parent != nil && parent.Type() == "decorated_definition" {
			if hasChildWithType(parent, "async") {
				return true
			}
		}
	}
	return false
}

// isGenerator checks if a node represents a generator function (JS/TS).
func (ctx *extractionContext) isGenerator(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	nodeType := node.Type()
	if nodeType == "generator_function" || nodeType == "generator_function_declaration" {
		return true
	}
	// Check for "*" child adjacent to function keyword.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil && nodeText(child, ctx.content) == "*" {
			return true
		}
	}
	return false
}

// hasModifierKeyword checks if a node has the specified keyword as an anonymous child.
func (ctx *extractionContext) hasModifierKeyword(node *sitter.Node, keyword string) bool {
	if node == nil {
		return false
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil && !child.IsNamed() && nodeText(child, ctx.content) == keyword {
			return true
		}
	}
	return false
}

// isReactComponentLike checks if a function looks like a React component.
// Requirements: uppercase first letter + JSX in body.
func (ctx *extractionContext) isReactComponentLike(node *sitter.Node, name string) bool {
	if len(name) == 0 || !unicode.IsUpper(rune(name[0])) {
		return false
	}
	// Check for JSX elements in the function body.
	return hasDescendantOfType(node, "jsx_element", 10) ||
		hasDescendantOfType(node, "jsx_self_closing_element", 10) ||
		hasDescendantOfType(node, "jsx_fragment", 10)
}

// isHookLike checks if a name matches the React hook pattern (use[A-Z]).
func isHookLike(name string) bool {
	if len(name) < 4 {
		return false
	}
	return strings.HasPrefix(name, "use") && unicode.IsUpper(rune(name[3]))
}

// ---------------------------------------------------------------------------
// Modifier extraction
// ---------------------------------------------------------------------------

// knownModifiers is the set of modifier keywords we extract.
var knownModifiers = map[string]bool{
	"public": true, "private": true, "protected": true,
	"static": true, "abstract": true, "readonly": true,
	"async": true, "override": true, "final": true,
	"const": true, "export": true, "default": true,
	"pub": true, "mut": true, "virtual": true,
	"inline": true, "extern": true, "internal": true,
	"open": true, "fileprivate": true,
}

// extractModifiers extracts modifier keywords from a node's anonymous children.
func (ctx *extractionContext) extractModifiers(node *sitter.Node) []string {
	if node == nil {
		return nil
	}
	var mods []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil || child.IsNamed() {
			continue
		}
		text := nodeText(child, ctx.content)
		if knownModifiers[text] {
			mods = append(mods, text)
		}
	}
	// Also check for dedicated modifier nodes.
	if modNode := findChildByType(node, "modifiers"); modNode != nil {
		for i := 0; i < int(modNode.ChildCount()); i++ {
			child := modNode.Child(i)
			if child != nil {
				text := nodeText(child, ctx.content)
				if knownModifiers[text] {
					mods = append(mods, text)
				}
			}
		}
	}
	if len(mods) == 0 {
		return nil
	}
	return mods
}
