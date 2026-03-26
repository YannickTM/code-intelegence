package extractors

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
)

// jsxContext carries state through JSX usage extraction.
type jsxContext struct {
	content   []byte
	langID    string
	symbols   []parser.Symbol
	usages    []parser.JsxUsage
	symbolIdx map[string]string // name → symbolID
	seen      map[string]bool   // dedup key → true
}

// ExtractJsxUsages extracts JSX component usages from a parsed file.
// Only runs for JSX and TSX language IDs. All other languages return nil.
func ExtractJsxUsages(root *sitter.Node, content []byte, symbols []parser.Symbol, langID string) []parser.JsxUsage {
	if root == nil {
		return nil
	}

	// Only JSX/TSX have JSX syntax.
	if !jsxLanguages[langID] {
		return nil
	}

	ctx := &jsxContext{
		content:   content,
		langID:    langID,
		symbols:   symbols,
		symbolIdx: buildSymbolIndex(symbols),
		seen:      make(map[string]bool),
	}

	ctx.walkForJsx(root, 0)

	if len(ctx.usages) == 0 {
		return nil
	}
	parser.SortJsxUsages(ctx.usages)
	return ctx.usages
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// generateJsxUsageID produces a deterministic "jsx_<hash16>" identifier.
func generateJsxUsageID(componentName string, line, column int32) string {
	key := fmt.Sprintf("jsx:%s:%d:%d", componentName, line, column)
	hash := parser.StableHash(key)
	return "jsx_" + hash[:16]
}

// ---------------------------------------------------------------------------
// AST walker
// ---------------------------------------------------------------------------

func (ctx *jsxContext) walkForJsx(node *sitter.Node, depth int) {
	if node == nil || depth > 50 {
		return
	}

	switch node.Type() {
	case "jsx_self_closing_element":
		ctx.handleJsxElement(node)
	case "jsx_opening_element":
		ctx.handleJsxElement(node)
	case "jsx_fragment":
		ctx.handleJsxFragment(node)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			ctx.walkForJsx(child, depth+1)
		}
	}
}

// handleJsxElement processes jsx_self_closing_element and jsx_opening_element.
func (ctx *jsxContext) handleJsxElement(node *sitter.Node) {
	name := extractJsxComponentName(node, ctx.content)
	if name == "" {
		// A jsx_opening_element with no identifier is a fragment shorthand (<>).
		// Some tree-sitter grammars emit this instead of a dedicated jsx_fragment node.
		if node.Type() == "jsx_opening_element" && node.NamedChildCount() == 0 {
			ctx.handleJsxFragment(node)
		}
		return
	}

	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)

	// Dedup.
	dedupKey := fmt.Sprintf("%s:%d:%d", name, line, col)
	if ctx.seen[dedupKey] {
		return
	}
	ctx.seen[dedupKey] = true

	isIntrinsic := false
	isFragment := false

	if len(name) > 0 {
		firstRune, _ := utf8.DecodeRuneInString(name)
		// Member expressions like foo.Bar are not intrinsic even if the
		// base starts lowercase — they are dotted component references.
		isIntrinsic = unicode.IsLower(firstRune) && !strings.Contains(name, ".")
	}

	// Check for Fragment.
	if name == "Fragment" || name == "React.Fragment" {
		isFragment = true
		isIntrinsic = false
	}

	// Resolve to local symbol.
	resolvedID := ""
	confidence := "LOW"
	isDotted := strings.Contains(name, ".")
	if !isIntrinsic && !isFragment {
		// For member expressions like Modal.Header, try the base name.
		lookupName := name
		if dot := strings.IndexByte(name, '.'); dot >= 0 {
			lookupName = name[:dot]
		}
		if id, ok := ctx.symbolIdx[lookupName]; ok {
			resolvedID = id
			if isDotted {
				// Only the base name was resolved; the member path is unverified.
				confidence = "MEDIUM"
			} else {
				confidence = "HIGH"
			}
		} else {
			confidence = "MEDIUM"
		}
	} else if isFragment {
		confidence = "HIGH" // fragments are always known
	} else if isIntrinsic {
		confidence = "HIGH" // intrinsics are always known
	}

	enclosingID := findEnclosingSymbolID(ctx.symbols, line)

	ctx.usages = append(ctx.usages, parser.JsxUsage{
		UsageID:                generateJsxUsageID(name, line, col),
		SourceSymbolID:         enclosingID,
		ComponentName:          name,
		IsIntrinsic:            isIntrinsic,
		IsFragment:             isFragment,
		Line:                   line,
		Column:                 col,
		ResolvedTargetSymbolID: resolvedID,
		Confidence:             confidence,
	})
}

// handleJsxFragment processes jsx_fragment nodes (<>...</>).
func (ctx *jsxContext) handleJsxFragment(node *sitter.Node) {
	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)

	dedupKey := fmt.Sprintf("Fragment:%d:%d", line, col)
	if ctx.seen[dedupKey] {
		return
	}
	ctx.seen[dedupKey] = true

	enclosingID := findEnclosingSymbolID(ctx.symbols, line)

	ctx.usages = append(ctx.usages, parser.JsxUsage{
		UsageID:        generateJsxUsageID("Fragment", line, col),
		SourceSymbolID: enclosingID,
		ComponentName:  "",
		IsIntrinsic:    false,
		IsFragment:     true,
		Line:           line,
		Column:         col,
		Confidence:     "HIGH",
	})
}

// extractJsxComponentName extracts the component name from a JSX element.
// Used by both jsx.go (JsxUsage extraction) and references.go (JSX_RENDER references).
func extractJsxComponentName(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "identifier":
			return nodeText(child, content)
		case "member_expression", "nested_identifier":
			return nodeText(child, content)
		case "jsx_namespace_name":
			return nodeText(child, content)
		}
	}
	return ""
}
