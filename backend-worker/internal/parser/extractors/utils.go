// Package extractors provides AST-based extraction of code symbols, imports,
// exports, references, and chunks from tree-sitter parse trees.
package extractors

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
)

const maxSignatureLen = 200

// nodeText returns the source text for a node, or "" if the node is nil.
func nodeText(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	if start >= end || int(end) > len(content) {
		return ""
	}
	return string(content[start:end])
}

// nodeStartLine returns the 1-indexed start line of a node.
func nodeStartLine(node *sitter.Node) int32 {
	return int32(node.StartPoint().Row) + 1
}

// nodeEndLine returns the 1-indexed end line of a node.
// When the endpoint column is 0, the node ends at the start of the next row,
// so the actual last line of content is the previous row.
func nodeEndLine(node *sitter.Node) int32 {
	ep := node.EndPoint()
	if ep.Column == 0 && ep.Row > 0 {
		return int32(ep.Row) // last content line is previous row (1-indexed)
	}
	return int32(ep.Row) + 1
}

// findChildByType searches direct children for the first child with the given node type.
func findChildByType(node *sitter.Node, nodeType string) *sitter.Node {
	if node == nil {
		return nil
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil && child.Type() == nodeType {
			return child
		}
	}
	return nil
}

// findChildByFieldName returns the child for a named field, or nil.
func findChildByFieldName(node *sitter.Node, fieldName string) *sitter.Node {
	if node == nil {
		return nil
	}
	return node.ChildByFieldName(fieldName)
}

// findNamedChildrenByType returns all named children of the given type.
func findNamedChildrenByType(node *sitter.Node, nodeType string) []*sitter.Node {
	if node == nil {
		return nil
	}
	var result []*sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Type() == nodeType {
			result = append(result, child)
		}
	}
	return result
}

// hasChildWithType checks whether any direct child (named or anonymous) has the given type.
func hasChildWithType(node *sitter.Node, nodeType string) bool {
	return findChildByType(node, nodeType) != nil
}

// hasDescendantOfType checks whether any descendant within maxDepth has the given type.
func hasDescendantOfType(node *sitter.Node, nodeType string, maxDepth int) bool {
	if node == nil || maxDepth <= 0 {
		return false
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == nodeType {
			return true
		}
		if hasDescendantOfType(child, nodeType, maxDepth-1) {
			return true
		}
	}
	return false
}

// isDirectExportChild checks if the node is a direct child of an export_statement.
func isDirectExportChild(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	parent := node.Parent()
	return parent != nil && parent.Type() == "export_statement"
}

// hasAncestorOfType checks if any ancestor (up to depth 10) has the given type.
// 10 levels handles deeply nested generics in inheritance clauses, e.g.
// class Foo implements Comparable<List<Map<K, V>>> { ... }
func hasAncestorOfType(node *sitter.Node, nodeType string) bool {
	if node == nil {
		return false
	}
	cur := node.Parent()
	for i := 0; i < 10 && cur != nil; i++ {
		if cur.Type() == nodeType {
			return true
		}
		cur = cur.Parent()
	}
	return false
}

// prevNamedSiblings collects preceding named siblings walking backward.
// Stops at maxCount or when pred returns false.
func prevNamedSiblings(node *sitter.Node, maxCount int, pred func(*sitter.Node) bool) []*sitter.Node {
	if node == nil {
		return nil
	}
	var siblings []*sitter.Node
	cur := node.PrevNamedSibling()
	for cur != nil && len(siblings) < maxCount {
		if !pred(cur) {
			break
		}
		siblings = append(siblings, cur)
		cur = cur.PrevNamedSibling()
	}
	// Reverse to get them in source order.
	for i, j := 0, len(siblings)-1; i < j; i, j = i+1, j-1 {
		siblings[i], siblings[j] = siblings[j], siblings[i]
	}
	return siblings
}

// truncateSignature extracts the first line of source text for a node,
// trimmed up to '{' or '=>', with a hard limit of maxSignatureLen characters.
func truncateSignature(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	text := nodeText(node, content)
	if text == "" {
		return ""
	}

	// Take the first line.
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}

	// Trim at '{' first — it always marks a block body.
	// Only fall back to '=>' (using LastIndex) for expression-body arrows,
	// so that '=>' inside type annotations like (x: A) => B is preserved.
	if idx := strings.Index(text, "{"); idx >= 0 {
		text = text[:idx]
	} else if idx := strings.LastIndex(text, "=>"); idx >= 0 {
		text = text[:idx]
	}

	text = strings.TrimSpace(text)

	if len(text) > maxSignatureLen {
		text = text[:maxSignatureLen] + "..."
	}
	return text
}

// generateSymbolID produces a deterministic "sym_<hash16>" identifier.
func generateSymbolID(name string, startLine, endLine int32) string {
	key := fmt.Sprintf("%s:%d:%d", name, startLine, endLine)
	hash := parser.StableHash(key)
	return "sym_" + hash[:16]
}
