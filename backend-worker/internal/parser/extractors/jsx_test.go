package extractors

import (
	"context"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func parseAndExtractJsx(t *testing.T, langID, source string) []parser.JsxUsage {
	t.Helper()
	grammar := parser.GetGrammar(langID)
	if grammar == nil {
		t.Fatalf("no grammar for %s", langID)
	}
	pool := parser.NewPool(1)
	defer pool.Shutdown()

	content := []byte(source)
	tree, err := pool.Parse(context.Background(), content, grammar)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	root := tree.RootNode()
	symbols := ExtractSymbols(root, content, langID)
	return ExtractJsxUsages(root, content, symbols, langID)
}

func findJsxUsage(usages []parser.JsxUsage, componentName string) *parser.JsxUsage {
	for i := range usages {
		if usages[i].ComponentName == componentName {
			return &usages[i]
		}
	}
	return nil
}

func assertJsxUsageExists(t *testing.T, usages []parser.JsxUsage, componentName string) *parser.JsxUsage {
	t.Helper()
	u := findJsxUsage(usages, componentName)
	if u == nil {
		names := make([]string, len(usages))
		for i, u := range usages {
			names[i] = u.ComponentName
		}
		t.Errorf("expected JSX usage %q not found; have: %v", componentName, names)
	}
	return u
}

// ---------------------------------------------------------------------------
// Custom component tests
// ---------------------------------------------------------------------------

func TestJsxCustomComponent(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function MyComponent() {
  return <Button onClick={() => {}}>Click</Button>;
}
`)
	u := assertJsxUsageExists(t, usages, "Button")
	if u != nil {
		if u.IsIntrinsic {
			t.Error("Button should not be intrinsic")
		}
		if u.IsFragment {
			t.Error("Button should not be fragment")
		}
	}
}

func TestJsxSelfClosing(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function App() {
  return <Input placeholder="type here" />;
}
`)
	u := assertJsxUsageExists(t, usages, "Input")
	if u != nil && u.IsIntrinsic {
		t.Error("Input (uppercase) should not be intrinsic")
	}
}

// ---------------------------------------------------------------------------
// Intrinsic element tests
// ---------------------------------------------------------------------------

func TestJsxIntrinsicElement(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function App() {
  return <div><span>hello</span></div>;
}
`)
	divUsage := findJsxUsage(usages, "div")
	if divUsage == nil {
		t.Fatal("expected intrinsic div")
	}
	if !divUsage.IsIntrinsic {
		t.Error("div should be intrinsic")
	}

	spanUsage := findJsxUsage(usages, "span")
	if spanUsage == nil {
		t.Fatal("expected intrinsic span")
	}
	if !spanUsage.IsIntrinsic {
		t.Error("span should be intrinsic")
	}
}

func TestJsxSelfClosingIntrinsic(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function Form() {
  return <input type="text" />;
}
`)
	u := findJsxUsage(usages, "input")
	if u == nil {
		t.Fatal("expected intrinsic input")
	}
	if !u.IsIntrinsic {
		t.Error("input should be intrinsic")
	}
}

// ---------------------------------------------------------------------------
// Fragment tests
// ---------------------------------------------------------------------------

func TestJsxShorthandFragment(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function App() {
  return <><div>a</div><div>b</div></>;
}
`)
	var fragment *parser.JsxUsage
	for i := range usages {
		if usages[i].IsFragment {
			fragment = &usages[i]
			break
		}
	}
	if fragment == nil {
		t.Fatal("expected fragment usage for <>...</>")
	}
}

func TestJsxNamedFragment(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
import React from "react";
function App() {
  return <React.Fragment><div>a</div></React.Fragment>;
}
`)
	u := findJsxUsage(usages, "React.Fragment")
	if u == nil {
		names := make([]string, len(usages))
		for i, u := range usages {
			names[i] = u.ComponentName
		}
		t.Fatalf("expected React.Fragment usage; have: %v", names)
	}
	if !u.IsFragment {
		t.Error("React.Fragment should be marked as fragment")
	}
}

// ---------------------------------------------------------------------------
// Member expression tests
// ---------------------------------------------------------------------------

func TestJsxMemberExpression(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function App() {
  return <Modal.Header title="test" />;
}
`)
	u := findJsxUsage(usages, "Modal.Header")
	if u == nil {
		names := make([]string, len(usages))
		for i, u := range usages {
			names[i] = u.ComponentName
		}
		t.Fatalf("expected Modal.Header usage; have: %v", names)
	}
	if u.IsIntrinsic {
		t.Error("Modal.Header should not be intrinsic")
	}
}

func TestJsxMemberExpressionConfidence(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function Modal() { return <div />; }

function App() {
  return <Modal.Header title="test" />;
}
`)
	u := findJsxUsage(usages, "Modal.Header")
	if u == nil {
		t.Fatal("expected Modal.Header usage")
	}
	// Base "Modal" resolves locally, but the member path is unverified.
	if u.Confidence != "MEDIUM" {
		t.Errorf("expected MEDIUM confidence for dotted component, got %s", u.Confidence)
	}
	if u.ResolvedTargetSymbolID == "" {
		t.Error("expected ResolvedTargetSymbolID to point to Modal's symbol")
	}
}

func TestJsxDirectComponentHighConfidence(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function Button() { return <button />; }

function App() {
  return <Button />;
}
`)
	u := findJsxUsage(usages, "Button")
	if u == nil {
		t.Fatal("expected Button usage")
	}
	if u.Confidence != "HIGH" {
		t.Errorf("expected HIGH confidence for direct local component, got %s", u.Confidence)
	}
}

// ---------------------------------------------------------------------------
// Symbol resolution tests
// ---------------------------------------------------------------------------

func TestJsxSymbolResolution(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function Button() {
  return <button>click</button>;
}

function App() {
  return <Button />;
}
`)
	u := findJsxUsage(usages, "Button")
	if u == nil {
		t.Fatal("expected Button usage")
	}
	if u.ResolvedTargetSymbolID == "" {
		t.Error("expected ResolvedTargetSymbolID for locally defined Button")
	}
}

func TestJsxEnclosingSymbol(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function MyComponent() {
  return <div>hello</div>;
}
`)
	u := findJsxUsage(usages, "div")
	if u == nil {
		t.Fatal("expected div usage")
	}
	if u.SourceSymbolID == "" {
		t.Error("expected SourceSymbolID (enclosing MyComponent)")
	}
}

// ---------------------------------------------------------------------------
// Language gate tests
// ---------------------------------------------------------------------------

func TestJsxNonJsxLanguageReturnsNil(t *testing.T) {
	// Parse JSX-bearing source with a JSX grammar so the tree contains
	// actual JSX nodes. The language guard should be the only thing
	// preventing results — if removed, the walk would find usages.
	grammar := parser.GetGrammar("jsx")
	if grammar == nil {
		t.Skip("jsx grammar unavailable")
	}
	pool := parser.NewPool(1)
	content := []byte("function App() { return <div>hello</div>; }")
	tree, err := pool.Parse(context.Background(), content, grammar)
	pool.Shutdown()
	if err != nil {
		t.Skip("parse failed")
	}
	root := tree.RootNode()

	// Verify the tree actually produces usages when called with "jsx".
	sanity := ExtractJsxUsages(root, content, nil, "jsx")
	if len(sanity) == 0 {
		t.Fatal("sanity check: expected JSX usages from jsx grammar")
	}

	// Non-JSX languages must return nil even with a JSX-bearing tree.
	for _, lang := range []string{"javascript", "typescript", "python", "go"} {
		usages := ExtractJsxUsages(root, content, nil, lang)
		if usages != nil {
			t.Errorf("expected nil for %s", lang)
		}
	}
}

func TestJsxBothDialects(t *testing.T) {
	source := `
function App() {
  return <div>hello</div>;
}
`
	for _, lang := range []string{"jsx", "tsx"} {
		usages := parseAndExtractJsx(t, lang, source)
		if usages == nil {
			t.Errorf("expected usages for %s", lang)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestJsxEmptyFile(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", "")
	if usages != nil {
		t.Errorf("expected nil, got %d usages", len(usages))
	}
}

func TestJsxNilRoot(t *testing.T) {
	usages := ExtractJsxUsages(nil, nil, nil, "tsx")
	if usages != nil {
		t.Error("expected nil for nil root")
	}
}

func TestJsxNestedElements(t *testing.T) {
	usages := parseAndExtractJsx(t, "tsx", `
function App() {
  return (
    <Container>
      <Header />
      <Content>
        <Button>Click</Button>
      </Content>
    </Container>
  );
}
`)
	for _, name := range []string{"Container", "Header", "Content", "Button"} {
		if u := findJsxUsage(usages, name); u == nil {
			t.Errorf("expected usage for %s", name)
		}
	}
}

func TestJsxIDDeterministic(t *testing.T) {
	source := `
function App() {
  return <Button />;
}
`
	u1 := parseAndExtractJsx(t, "tsx", source)
	u2 := parseAndExtractJsx(t, "tsx", source)
	if len(u1) == 0 || len(u2) == 0 {
		t.Fatal("expected usages")
	}
	b1 := findJsxUsage(u1, "Button")
	b2 := findJsxUsage(u2, "Button")
	if b1 == nil || b2 == nil {
		t.Fatal("expected Button usage")
	}
	if b1.UsageID != b2.UsageID {
		t.Errorf("expected deterministic IDs: %s != %s", b1.UsageID, b2.UsageID)
	}
}
