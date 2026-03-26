package extractors

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/registry"
)

// networkContext carries state through network call extraction.
type networkContext struct {
	content    []byte
	langID     string
	langConfig *registry.LanguageConfig
	symbols    []parser.Symbol
	calls      []parser.NetworkCall
	seen       map[string]bool // dedup key → true
}

// httpMethodNames maps lowercase method names to uppercase HTTP methods.
var httpMethodNames = map[string]string{
	"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH",
	"delete": "DELETE", "head": "HEAD", "options": "OPTIONS",
	// Go cased variants.
	"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH",
	"Delete": "DELETE", "Head": "HEAD", "PostForm": "POST",
	// C# async variants.
	"GetAsync": "GET", "PostAsync": "POST", "PutAsync": "PUT",
	"PatchAsync": "PATCH", "DeleteAsync": "DELETE", "SendAsync": "UNKNOWN",
	// Go specific.
	"NewRequest": "UNKNOWN", "Do": "UNKNOWN",
	// Java specific.
	"send": "UNKNOWN", "execute": "UNKNOWN", "exchange": "UNKNOWN",
	"getForObject": "GET", "getForEntity": "GET",
	"postForObject": "POST", "postForEntity": "POST", "postForLocation": "POST",
}

// jsHTTPVerbMethods are standard HTTP verb method names used in JS instance
// fallback detection. Excludes generic names (send, execute, exchange) that
// are common in non-HTTP contexts.
var jsHTTPVerbMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true,
	"delete": true, "head": true, "options": true,
	"request": true,
}

// jsHTTPClients maps direct function calls to client kinds for JS/TS.
var jsHTTPClients = map[string]string{
	"fetch": "FETCH",
	"axios": "AXIOS",
	"ky":    "KY",
}

// jsHTTPReceivers maps receiver names to client kinds for member calls.
var jsHTTPReceivers = map[string]string{
	"axios": "AXIOS",
	"ky":    "KY",
}

// jsGraphQLFunctions detects GraphQL-related function calls.
var jsGraphQLFunctions = map[string]bool{
	"gql":              true,
	"useQuery":         true,
	"useMutation":      true,
	"useLazyQuery":     true,
	"useSubscription":  true,
}

// jsGraphQLMethods detects GraphQL client methods that are only classified
// as GraphQL when the receiver is in jsGraphQLReceivers. These names are
// too generic on their own (e.g. db.query(), el.mutate(), http.request()).
var jsGraphQLMethods = map[string]bool{
	"query":   true,
	"mutate":  true,
	"request": true,
}

// jsGraphQLReceivers maps receiver names known to be GraphQL clients.
// Methods like "query", "mutate", "request" are only classified as GraphQL
// when the receiver is in this map.
var jsGraphQLReceivers = map[string]bool{
	"graphqlClient": true, "apolloClient": true,
	"graphql": true, "gqlClient": true, "urqlClient": true,
}

// isGraphQLReceiver checks whether the receiver (which may be a dotted
// expression like "this.apolloClient") matches a known GraphQL client.
// It checks the full receiver first, then falls back to the last segment.
func isGraphQLReceiver(receiver string) bool {
	if jsGraphQLReceivers[receiver] {
		return true
	}
	if dot := strings.LastIndexByte(receiver, '.'); dot >= 0 {
		return jsGraphQLReceivers[receiver[dot+1:]]
	}
	return false
}

// pythonHTTPReceivers maps Python HTTP client receiver names.
var pythonHTTPReceivers = map[string]bool{
	"requests": true, "httpx": true, "urllib": true,
	"aiohttp": true, "session": true, "client": true,
}

// goHTTPReceivers maps Go HTTP receiver/package names.
var goHTTPReceivers = map[string]bool{
	"http": true, "client": true,
}

// goHTTPMethods are Go net/http and http.Client methods that perform requests.
// Excludes server-side methods (ListenAndServe, Handle, HandleFunc, etc.),
// request constructors (NewRequest, NewRequestWithContext), and connection
// management (CloseIdleConnections).
var goHTTPMethods = map[string]bool{
	"Get": true, "Head": true, "Post": true, "PostForm": true,
	"Do": true,
}

// httpClientReceivers maps receiver names to true for non-JS languages.
// These are used to detect instance.method() HTTP patterns.
var httpClientReceivers = map[string]bool{
	// Rust
	"reqwest": true, "hyper": true,
	// Java/Kotlin
	"restTemplate": true, "webClient": true, "HttpClient": true, "httpClient": true,
	"OkHttpClient": true, "okHttpClient": true,
	// Swift
	"URLSession": true, "AF": true, "Alamofire": true,
	// Ruby
	"Net::HTTP": true, "Faraday": true, "HTTParty": true, "RestClient": true,
	// PHP
	"Guzzle": true,
}

// csharpHTTPReceivers maps C# HTTP client receiver names.
// "client" is included because C# conventionally uses DI-injected HttpClient
// as "client"; the method check (GetAsync, PostAsync, etc.) prevents false positives.
var csharpHTTPReceivers = map[string]bool{
	"HttpClient": true, "httpClient": true, "_httpClient": true,
	"client": true, "_client": true,
	"WebClient": true, "webClient": true,
}

// csharpHTTPAsyncMethods are C# HttpClient async methods that perform HTTP requests.
// Only includes methods that actually send network requests, not generic Async methods.
var csharpHTTPAsyncMethods = map[string]bool{
	"GetAsync": true, "PostAsync": true, "PutAsync": true,
	"PatchAsync": true, "DeleteAsync": true, "SendAsync": true,
	"GetStringAsync": true, "GetByteArrayAsync": true, "GetStreamAsync": true,
}

// curlFunctions detects curl-like function calls (C/C++/PHP).
var curlFunctions = map[string]bool{
	"curl_init": true, "curl_exec": true, "curl_setopt": true,
	"curl_easy_init": true, "curl_easy_perform": true,
}

// ExtractNetworkCalls detects HTTP client calls from a parsed file.
// For Tier 1 languages, it walks the AST for fetch, axios, ky, GraphQL,
// and per-language HTTP client patterns.
// Tier 2/3 languages return nil.
func ExtractNetworkCalls(root *sitter.Node, content []byte, symbols []parser.Symbol, langID string) []parser.NetworkCall {
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

	ctx := &networkContext{
		content:    content,
		langID:     langID,
		langConfig: langConfig,
		symbols:    symbols,
		seen:       make(map[string]bool),
	}

	ctx.walkForNetwork(root, 0)

	if len(ctx.calls) == 0 {
		return nil
	}
	parser.SortNetworkCalls(ctx.calls)
	return ctx.calls
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// generateNetworkCallID produces a deterministic "net_<hash16>" identifier.
func generateNetworkCallID(clientKind, method string, line, column int32) string {
	key := fmt.Sprintf("%s:%s:%d:%d", clientKind, method, line, column)
	hash := parser.StableHash(key)
	return "net_" + hash[:16]
}

// inferHTTPMethod maps a method name to an uppercase HTTP method.
func inferHTTPMethod(methodName string) string {
	if m, ok := httpMethodNames[methodName]; ok {
		return m
	}
	lower := strings.ToLower(methodName)
	if m, ok := httpMethodNames[lower]; ok {
		return m
	}
	return "UNKNOWN"
}

// isRelativeURL checks whether a URL is relative.
func isRelativeURL(url string) bool {
	if url == "" || url == "<dynamic>" {
		return false
	}
	// Protocol-relative URLs (//host/path) are absolute.
	if strings.HasPrefix(url, "//") {
		return false
	}
	return strings.HasPrefix(url, "/") ||
		strings.HasPrefix(url, "./") ||
		strings.HasPrefix(url, "../") ||
		!strings.Contains(url, "://")
}

// ---------------------------------------------------------------------------
// URL extraction
// ---------------------------------------------------------------------------

// extractURLFromArgs extracts URL information from the first argument of a call.
func (ctx *networkContext) extractURLFromArgs(node *sitter.Node) (literal, template string, relative bool) {
	// Find the arguments node.
	args := findChildByFieldName(node, "arguments")
	if args == nil {
		args = findChildByType(node, "arguments")
	}
	if args == nil {
		args = findChildByType(node, "argument_list")
	}
	if args == nil {
		return "<dynamic>", "", false
	}

	// Get the first argument.
	var firstArg *sitter.Node
	for i := 0; i < int(args.NamedChildCount()); i++ {
		child := args.NamedChild(i)
		if child != nil {
			firstArg = child
			break
		}
	}
	if firstArg == nil {
		return "<dynamic>", "", false
	}

	return ctx.extractURLFromNode(firstArg)
}

// extractURLFromNode extracts URL information from a single AST node.
func (ctx *networkContext) extractURLFromNode(node *sitter.Node) (literal, template string, relative bool) {
	if node == nil {
		return "<dynamic>", "", false
	}

	switch node.Type() {
	case "string", "string_literal", "interpreted_string_literal", "raw_string_literal":
		// String literal — strip quotes.
		text := nodeText(node, ctx.content)
		text = stripQuotes(text)
		return text, "", isRelativeURL(text)
	case "string_fragment":
		text := nodeText(node, ctx.content)
		return text, "", isRelativeURL(text)
	case "template_string":
		// JS/TS template literal: `url/${expr}`.
		tpl := ctx.buildTemplateLiteral(node)
		return "", tpl, isRelativeURL(tpl)
	case "concatenated_string":
		// Python f"..." or "..." + "...".
		return "<dynamic>", "", false
	}

	// Check if it's a string-containing expression.
	// Look for a string child inside.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil {
			switch child.Type() {
			case "string", "string_literal", "string_fragment", "interpreted_string_literal":
				text := nodeText(child, ctx.content)
				text = stripQuotes(text)
				return text, "", isRelativeURL(text)
			case "string_content":
				text := nodeText(child, ctx.content)
				return text, "", isRelativeURL(text)
			}
		}
	}

	return "<dynamic>", "", false
}

// buildTemplateLiteral converts a template_string AST node to a template
// with {expr} substitutions.
func (ctx *networkContext) buildTemplateLiteral(node *sitter.Node) string {
	var parts []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "string_fragment":
			parts = append(parts, nodeText(child, ctx.content))
		case "template_substitution":
			parts = append(parts, "{expr}")
		case "`":
			// Skip backticks.
		default:
			text := nodeText(child, ctx.content)
			if text != "`" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

// Note: stripQuotes is defined in imports.go and reused here.

// ---------------------------------------------------------------------------
// AST walker
// ---------------------------------------------------------------------------

func (ctx *networkContext) walkForNetwork(node *sitter.Node, depth int) {
	if node == nil || depth > 50 {
		return
	}

	nodeType := node.Type()

	switch nodeType {
	case "call_expression":
		if jsLikeLanguages[ctx.langID] || ctx.langID == "go" || ctx.langID == "rust" ||
			ctx.langID == "kotlin" || ctx.langID == "c" || ctx.langID == "cpp" || ctx.langID == "swift" {
			ctx.handleNetworkCall(node)
		}
	case "call":
		if ctx.langID == "python" || ctx.langID == "ruby" {
			ctx.handleNetworkCall(node)
		}
	case "method_invocation":
		if ctx.langID == "java" || ctx.langID == "kotlin" {
			ctx.handleNetworkCall(node)
		}
	case "method_call":
		if ctx.langID == "ruby" {
			ctx.handleNetworkCall(node)
		}
	case "invocation_expression":
		if ctx.langID == "csharp" {
			ctx.handleNetworkCall(node)
		}
	case "function_call_expression", "method_call_expression":
		if ctx.langID == "php" {
			ctx.handleNetworkCall(node)
		}
	}

	// Recurse.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			ctx.walkForNetwork(child, depth+1)
		}
	}
}

// ---------------------------------------------------------------------------
// Network call handler
// ---------------------------------------------------------------------------

func (ctx *networkContext) handleNetworkCall(node *sitter.Node) {
	targetName, receiverName, qualifiedName := ctx.extractCallParts(node)
	if targetName == "" && receiverName == "" {
		return
	}

	clientKind, method, confidence, isMatch := ctx.classifyNetworkCall(targetName, receiverName, qualifiedName, node)
	if !isMatch {
		return
	}

	line := nodeStartLine(node)
	col := int32(node.StartPoint().Column)

	// Dedup.
	dedupKey := fmt.Sprintf("%s:%s:%d:%d", clientKind, method, line, col)
	if ctx.seen[dedupKey] {
		return
	}
	ctx.seen[dedupKey] = true

	// Extract URL.
	urlLiteral, urlTemplate, isRelative := ctx.extractURLFromArgs(node)

	enclosingID := findEnclosingSymbolID(ctx.symbols, line)

	ctx.calls = append(ctx.calls, parser.NetworkCall{
		NetworkCallID:  generateNetworkCallID(clientKind, method, line, col),
		SourceSymbolID: enclosingID,
		ClientKind:     clientKind,
		Method:         method,
		URLLiteral:     urlLiteral,
		URLTemplate:    urlTemplate,
		IsRelative:     isRelative,
		StartLine:      line,
		StartColumn:    col,
		Confidence:     confidence,
	})
}

// extractCallParts extracts the target method name, receiver name, and qualified name.
func (ctx *networkContext) extractCallParts(node *sitter.Node) (target, receiver, qualified string) {
	var funcNode *sitter.Node

	switch ctx.langID {
	case "java":
		funcNode = findChildByFieldName(node, "name")
		objNode := findChildByFieldName(node, "object")
		if objNode != nil {
			receiver = nodeText(objNode, ctx.content)
		}
		if funcNode != nil {
			target = nodeText(funcNode, ctx.content)
		}
		if receiver != "" && target != "" {
			qualified = receiver + "." + target
		} else {
			qualified = target
		}
		return
	case "python":
		funcNode = findChildByFieldName(node, "function")
	case "ruby":
		funcNode = findChildByFieldName(node, "method")
		recvNode := findChildByFieldName(node, "receiver")
		if recvNode != nil {
			receiver = nodeText(recvNode, ctx.content)
		}
		if funcNode != nil {
			target = nodeText(funcNode, ctx.content)
		}
		if receiver != "" && target != "" {
			qualified = receiver + "." + target
		} else {
			qualified = target
		}
		return
	case "csharp":
		// C# invocation_expression: first named child is the member_access_expression.
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
		funcNode = findChildByFieldName(node, "function")
		if funcNode == nil {
			funcNode = findChildByFieldName(node, "name")
		}
		if funcNode == nil && node.NamedChildCount() > 0 {
			funcNode = node.NamedChild(0)
		}
	default:
		funcNode = findChildByFieldName(node, "function")
	}

	if funcNode == nil {
		return "", "", ""
	}

	qualified = nodeText(funcNode, ctx.content)

	switch funcNode.Type() {
	case "member_expression":
		objNode := findChildByFieldName(funcNode, "object")
		propNode := findChildByFieldName(funcNode, "property")
		if objNode != nil {
			receiver = nodeText(objNode, ctx.content)
		}
		if propNode != nil {
			target = nodeText(propNode, ctx.content)
		}
	case "member_access_expression":
		// C#: uses "expression" for object and "name" for member.
		objNode := findChildByFieldName(funcNode, "expression")
		if objNode == nil {
			objNode = findChildByFieldName(funcNode, "object")
		}
		nameNode := findChildByFieldName(funcNode, "name")
		if nameNode == nil {
			nameNode = findChildByFieldName(funcNode, "property")
		}
		if objNode != nil {
			receiver = nodeText(objNode, ctx.content)
		}
		if nameNode != nil {
			target = nodeText(nameNode, ctx.content)
		}
		// Fallback: extract from named children.
		if receiver == "" && target == "" && funcNode.NamedChildCount() >= 2 {
			receiver = nodeText(funcNode.NamedChild(0), ctx.content)
			target = nodeText(funcNode.NamedChild(int(funcNode.NamedChildCount()-1)), ctx.content)
		}
	case "selector_expression":
		// Go: pkg.Func.
		operandNode := findChildByFieldName(funcNode, "operand")
		fieldNode := findChildByFieldName(funcNode, "field")
		if operandNode != nil {
			receiver = nodeText(operandNode, ctx.content)
		}
		if fieldNode != nil {
			target = nodeText(fieldNode, ctx.content)
		}
	case "scoped_identifier":
		// Rust: path::name.
		pathNode := findChildByFieldName(funcNode, "path")
		nameNode := findChildByFieldName(funcNode, "name")
		if pathNode != nil {
			receiver = nodeText(pathNode, ctx.content)
		}
		if nameNode != nil {
			target = nodeText(nameNode, ctx.content)
		}
	case "attribute":
		// Python: obj.method.
		objNode := findChildByFieldName(funcNode, "object")
		attrNode := findChildByFieldName(funcNode, "attribute")
		if objNode != nil {
			receiver = nodeText(objNode, ctx.content)
		}
		if attrNode != nil {
			target = nodeText(attrNode, ctx.content)
		}
	case "identifier":
		target = nodeText(funcNode, ctx.content)
	default:
		target = nodeText(funcNode, ctx.content)
	}

	return
}

// classifyNetworkCall determines whether a call is a network call and returns
// the client kind, HTTP method, confidence, and whether it matched.
func (ctx *networkContext) classifyNetworkCall(target, receiver, qualified string, node *sitter.Node) (clientKind, method, confidence string, isMatch bool) {
	if jsLikeLanguages[ctx.langID] {
		return ctx.classifyJSNetworkCall(target, receiver, qualified, node)
	}
	return ctx.classifyGenericNetworkCall(target, receiver, qualified)
}

// classifyJSNetworkCall handles JS/TS-specific HTTP client detection.
func (ctx *networkContext) classifyJSNetworkCall(target, receiver, qualified string, node *sitter.Node) (string, string, string, bool) {
	// Direct client calls: fetch(), axios(), ky().
	if receiver == "" {
		if kind, ok := jsHTTPClients[target]; ok {
			method := "GET"
			if target == "fetch" {
				method = ctx.extractFetchMethod(node)
			}
			return kind, method, "HIGH", true
		}
		// GraphQL functions.
		if jsGraphQLFunctions[target] {
			method := "POST"
			if target == "useQuery" || target == "useLazyQuery" || target == "useSubscription" {
				method = "GET"
			}
			return "GRAPHQL", method, "HIGH", true
		}
		return "", "", "", false
	}

	// Receiver.method() patterns.
	if kind, ok := jsHTTPReceivers[receiver]; ok {
		method := inferHTTPMethod(target)
		return kind, method, "HIGH", true
	}

	// GraphQL client methods — only when receiver is a known GraphQL client.
	// Check both the full receiver and the last segment for qualified
	// receivers like this.apolloClient or app.gqlClient.
	if jsGraphQLMethods[target] && isGraphQLReceiver(receiver) {
		return "GRAPHQL", "POST", "HIGH", true
	}

	// Instance pattern: api.get(), client.post(), etc.
	// MEDIUM confidence — the receiver is not a known HTTP client.
	// Only match standard HTTP verb names to avoid false positives from
	// generic method names like send(), execute(), exchange().
	if jsHTTPVerbMethods[strings.ToLower(target)] {
		return "UNKNOWN", inferHTTPMethod(target), "MEDIUM", true
	}

	return "", "", "", false
}

// classifyGenericNetworkCall handles non-JS language HTTP client detection.
func (ctx *networkContext) classifyGenericNetworkCall(target, receiver, qualified string) (string, string, string, bool) {
	switch ctx.langID {
	case "python":
		if pythonHTTPReceivers[receiver] && isHTTPMethodName(target) {
			return "UNKNOWN", inferHTTPMethod(target), "HIGH", true
		}
		// urllib.request.urlopen / urlopen pattern.
		if target == "urlopen" || qualified == "urllib.request.urlopen" {
			return "UNKNOWN", "GET", "HIGH", true
		}
	case "go":
		if goHTTPReceivers[receiver] && goHTTPMethods[target] {
			m := inferHTTPMethod(target)
			return "UNKNOWN", m, "HIGH", true
		}
	case "rust":
		if receiver == "reqwest" || receiver == "hyper" || httpClientReceivers[receiver] {
			m := inferHTTPMethod(target)
			return "UNKNOWN", m, "HIGH", true
		}
	case "java", "kotlin":
		if httpClientReceivers[receiver] {
			m := inferHTTPMethod(target)
			return "UNKNOWN", m, "HIGH", true
		}
	case "csharp":
		if csharpHTTPReceivers[receiver] {
			m := inferHTTPMethod(target)
			if m != "UNKNOWN" {
				return "UNKNOWN", m, "HIGH", true
			}
			// Only match known HTTP async methods, not arbitrary *Async() calls.
			if csharpHTTPAsyncMethods[target] {
				return "UNKNOWN", "UNKNOWN", "HIGH", true
			}
		}
	case "swift":
		if receiver == "URLSession" || strings.Contains(qualified, "URLSession") ||
			receiver == "AF" || receiver == "Alamofire" {
			return "UNKNOWN", "UNKNOWN", "HIGH", true
		}
	case "ruby":
		if httpClientReceivers[receiver] && isHTTPMethodName(target) {
			return "UNKNOWN", inferHTTPMethod(target), "HIGH", true
		}
		// Net::HTTP special.
		if strings.Contains(receiver, "Net::HTTP") || strings.Contains(qualified, "Net::HTTP") {
			return "UNKNOWN", inferHTTPMethod(target), "HIGH", true
		}
	case "php":
		if target == "file_get_contents" {
			// file_get_contents can read URLs or local files — we can't
			// distinguish statically, so use MEDIUM confidence.
			return "UNKNOWN", "GET", "MEDIUM", true
		}
		if curlFunctions[target] || curlFunctions[qualified] {
			return "UNKNOWN", "UNKNOWN", "HIGH", true
		}
		if httpClientReceivers[receiver] && isHTTPMethodName(target) {
			return "UNKNOWN", inferHTTPMethod(target), "HIGH", true
		}
	case "c", "cpp":
		if curlFunctions[target] || curlFunctions[qualified] {
			return "UNKNOWN", "UNKNOWN", "HIGH", true
		}
	}

	return "", "", "", false
}

// extractFetchMethod looks for the method in a fetch() options argument.
func (ctx *networkContext) extractFetchMethod(node *sitter.Node) string {
	args := findChildByFieldName(node, "arguments")
	if args == nil {
		args = findChildByType(node, "arguments")
	}
	if args == nil {
		return "GET"
	}

	// Look for the second argument (options object).
	argCount := 0
	for i := 0; i < int(args.NamedChildCount()); i++ {
		child := args.NamedChild(i)
		if child == nil {
			continue
		}
		argCount++
		if argCount == 2 && child.Type() == "object" {
			return ctx.extractMethodFromObject(child)
		}
	}
	return "GET"
}

// extractMethodFromObject extracts the "method" property from an object literal.
func (ctx *networkContext) extractMethodFromObject(obj *sitter.Node) string {
	for i := 0; i < int(obj.NamedChildCount()); i++ {
		child := obj.NamedChild(i)
		if child == nil || child.Type() != "pair" {
			continue
		}
		key := findChildByFieldName(child, "key")
		value := findChildByFieldName(child, "value")
		if key == nil || value == nil {
			continue
		}
		// Strip quotes from key — handles both `method:` and `"method":`.
		keyText := stripQuotes(nodeText(key, ctx.content))
		if keyText != "method" {
			continue
		}
		method := stripQuotes(nodeText(value, ctx.content))
		// Only emit valid HTTP methods; skip dynamic values like variable names.
		if m, ok := httpMethodNames[strings.ToLower(method)]; ok {
			return m
		}
		// Dynamic value — can't determine method statically.
		return "UNKNOWN"
	}
	return "GET"
}

// isHTTPMethodName checks whether a name is a known HTTP method name.
func isHTTPMethodName(name string) bool {
	_, ok := httpMethodNames[name]
	if ok {
		return true
	}
	_, ok = httpMethodNames[strings.ToLower(name)]
	return ok
}
