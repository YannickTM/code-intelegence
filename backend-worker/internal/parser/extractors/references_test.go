package extractors

import (
	"context"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func parseAndExtractReferences(t *testing.T, langID, source string) []parser.Reference {
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
	imports := ExtractImports(root, content, "test."+langID, langID)
	return ExtractReferences(root, content, symbols, imports, langID)
}

func findReference(refs []parser.Reference, targetName string) *parser.Reference {
	for i := range refs {
		if refs[i].TargetName == targetName {
			return &refs[i]
		}
	}
	return nil
}

func findReferenceByKind(refs []parser.Reference, kind string) *parser.Reference {
	for i := range refs {
		if refs[i].ReferenceKind == kind {
			return &refs[i]
		}
	}
	return nil
}

func findReferenceByKindAndName(refs []parser.Reference, kind, targetName string) *parser.Reference {
	for i := range refs {
		if refs[i].ReferenceKind == kind && refs[i].TargetName == targetName {
			return &refs[i]
		}
	}
	return nil
}

func assertReferenceExists(t *testing.T, refs []parser.Reference, targetName string) *parser.Reference {
	t.Helper()
	r := findReference(refs, targetName)
	if r == nil {
		t.Errorf("expected reference %q not found; have: %v", targetName, referenceNames(refs))
	}
	return r
}

func referenceNames(refs []parser.Reference) []string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.ReferenceKind + ":" + r.TargetName
	}
	return names
}

// ---------------------------------------------------------------------------
// JS/TS call expression tests
// ---------------------------------------------------------------------------

func TestJSCallReference(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
function foo() {}
function bar() {
  foo();
}
`)
	r := findReferenceByKindAndName(refs, "CALL", "foo")
	if r == nil {
		t.Fatal("expected CALL reference to foo")
	}
	if r.ResolutionScope != "LOCAL" {
		t.Errorf("expected LOCAL scope, got %s", r.ResolutionScope)
	}
}

func TestJSMethodCall(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
const result = console.log("hello");
`)
	r := findReferenceByKindAndName(refs, "CALL", "log")
	if r == nil {
		t.Fatal("expected CALL reference to log")
	}
	if r.QualifiedTargetHint != "console.log" {
		t.Errorf("expected qualified hint console.log, got %s", r.QualifiedTargetHint)
	}
}

func TestJSSkipRequireImport(t *testing.T) {
	refs := parseAndExtractReferences(t, "javascript", `
const fs = require("fs");
`)
	for _, r := range refs {
		if r.TargetName == "require" && r.ReferenceKind == "CALL" {
			t.Error("require should be skipped")
		}
	}
}

// ---------------------------------------------------------------------------
// Hook detection tests (JS/TS/JSX/TSX only)
// ---------------------------------------------------------------------------

func TestHookUseState(t *testing.T) {
	refs := parseAndExtractReferences(t, "tsx", `
function MyComponent() {
  const [count, setCount] = useState(0);
  useEffect(() => {}, []);
  const memo = useMemo(() => count * 2, [count]);
  return <div>{count}</div>;
}
`)
	hookNames := []string{"useState", "useEffect", "useMemo"}
	for _, name := range hookNames {
		r := findReferenceByKindAndName(refs, "HOOK_USE", name)
		if r == nil {
			t.Errorf("expected HOOK_USE reference to %s", name)
		}
	}
}

func TestHookOnlyForReact(t *testing.T) {
	// Python function starting with "use" should NOT be HOOK_USE.
	refs := parseAndExtractReferences(t, "python", `
def useData():
    pass

useData()
`)
	for _, r := range refs {
		if r.ReferenceKind == "HOOK_USE" {
			t.Errorf("HOOK_USE should not appear in Python; found: %s", r.TargetName)
		}
	}
}

// ---------------------------------------------------------------------------
// JSX_RENDER tests (JSX/TSX only)
// ---------------------------------------------------------------------------

func TestJsxRenderReference(t *testing.T) {
	refs := parseAndExtractReferences(t, "tsx", `
function App() {
  return <Button onClick={() => {}}>Click</Button>;
}
`)
	r := findReferenceByKindAndName(refs, "JSX_RENDER", "Button")
	if r == nil {
		t.Fatal("expected JSX_RENDER reference to Button")
	}
}

func TestJsxRenderSkipsIntrinsicElements(t *testing.T) {
	refs := parseAndExtractReferences(t, "tsx", `
function App() {
  return <div><span><Button>Click</Button></span></div>;
}
`)
	// Button (uppercase) should have JSX_RENDER.
	if r := findReferenceByKindAndName(refs, "JSX_RENDER", "Button"); r == nil {
		t.Error("expected JSX_RENDER reference to Button")
	}
	// div and span (lowercase) should NOT have JSX_RENDER.
	for _, r := range refs {
		if r.ReferenceKind == "JSX_RENDER" && (r.TargetName == "div" || r.TargetName == "span") {
			t.Errorf("intrinsic element %q should not emit JSX_RENDER", r.TargetName)
		}
	}
}

func TestJsxRenderOnlyForJsx(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
const x = "no JSX here";
`)
	for _, r := range refs {
		if r.ReferenceKind == "JSX_RENDER" {
			t.Error("JSX_RENDER should not appear in plain TypeScript")
		}
	}
}

// ---------------------------------------------------------------------------
// TYPE_REF tests
// ---------------------------------------------------------------------------

func TestTypeReference(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
interface User {
  name: string;
}
function greet(user: User): void {}
`)
	r := findReferenceByKindAndName(refs, "TYPE_REF", "User")
	if r == nil {
		t.Fatal("expected TYPE_REF to User")
	}
}

func TestBuiltinTypeSkip(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
const x: string = "hello";
const y: number = 42;
const z: boolean = true;
`)
	builtins := []string{"string", "number", "boolean"}
	for _, b := range builtins {
		if r := findReferenceByKindAndName(refs, "TYPE_REF", b); r != nil {
			t.Errorf("builtin type %q should be skipped", b)
		}
	}
}

func TestGoTypeRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "go", `
package main

type MyStruct struct{}

func foo() MyStruct {
	var x MyStruct
	return x
}
`)
	found := false
	for _, r := range refs {
		if r.ReferenceKind == "TYPE_REF" && r.TargetName == "MyStruct" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected TYPE_REF to MyStruct in Go")
	}
}

func TestGoBuiltinSkip(t *testing.T) {
	refs := parseAndExtractReferences(t, "go", `
package main

func foo() {
	var x int
	var y string
	var z bool
	_ = x
	_ = y
	_ = z
}
`)
	for _, r := range refs {
		if r.ReferenceKind == "TYPE_REF" {
			switch r.TargetName {
			case "int", "string", "bool":
				t.Errorf("builtin type %q should be skipped in Go", r.TargetName)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// EXTENDS / IMPLEMENTS tests
// ---------------------------------------------------------------------------

func TestExtendsClause(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
class Animal {}
class Dog extends Animal {}
`)
	r := findReferenceByKindAndName(refs, "EXTENDS", "Animal")
	if r == nil {
		t.Fatal("expected EXTENDS reference to Animal")
	}
}

func TestImplementsClause(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
interface Serializable {}
class User implements Serializable {}
`)
	r := findReferenceByKindAndName(refs, "IMPLEMENTS", "Serializable")
	if r == nil {
		t.Fatal("expected IMPLEMENTS reference to Serializable")
	}
}

func TestJavaInheritance(t *testing.T) {
	refs := parseAndExtractReferences(t, "java", `
class Animal {}
class Dog extends Animal {}
`)
	r := findReferenceByKindAndName(refs, "EXTENDS", "Animal")
	if r == nil {
		t.Fatal("expected EXTENDS reference to Animal in Java")
	}
}

func TestPythonInheritance(t *testing.T) {
	refs := parseAndExtractReferences(t, "python", `
class Base:
    pass

class Child(Base):
    pass
`)
	// Python inheritance should emit EXTENDS, not CALL.
	found := false
	for _, r := range refs {
		if r.TargetName == "Base" && r.ReferenceKind == "EXTENDS" {
			found = true
		}
		if r.TargetName == "Base" && r.ReferenceKind == "CALL" {
			t.Errorf("Python inheritance should emit EXTENDS, not CALL")
		}
	}
	if !found {
		t.Errorf("expected EXTENDS reference to Base in Python class inheritance; have: %v", referenceNames(refs))
	}
}

func TestRubyInheritance(t *testing.T) {
	refs := parseAndExtractReferences(t, "ruby", `
class Animal
end

class Dog < Animal
end
`)
	r := findReferenceByKindAndName(refs, "EXTENDS", "Animal")
	if r == nil {
		t.Errorf("expected EXTENDS reference to Animal in Ruby; have: %v", referenceNames(refs))
	}
}

// ---------------------------------------------------------------------------
// NEW_EXPR tests
// ---------------------------------------------------------------------------

func TestNewExpression(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
class Foo {}
const x = new Foo();
`)
	r := findReferenceByKindAndName(refs, "NEW_EXPR", "Foo")
	if r == nil {
		t.Fatal("expected NEW_EXPR reference to Foo")
	}
}

func TestJavaNewExpression(t *testing.T) {
	refs := parseAndExtractReferences(t, "java", `
class Main {
  void test() {
    ArrayList list = new ArrayList();
  }
}
`)
	r := findReferenceByKindAndName(refs, "NEW_EXPR", "ArrayList")
	if r == nil {
		t.Errorf("expected NEW_EXPR reference to ArrayList in Java; have: %v", referenceNames(refs))
	}
}

func TestGoCompositeLiteral(t *testing.T) {
	refs := parseAndExtractReferences(t, "go", `
package main

type Config struct {
	Name string
}

func main() {
	c := Config{Name: "test"}
	_ = c
}
`)
	r := findReferenceByKindAndName(refs, "NEW_EXPR", "Config")
	if r == nil {
		t.Errorf("expected NEW_EXPR reference to Config in Go; have: %v", referenceNames(refs))
	}
}

func TestGoMapSliceLiteralSkipped(t *testing.T) {
	refs := parseAndExtractReferences(t, "go", `
package main

func main() {
	m := map[string]int{"a": 1}
	s := []int{1, 2, 3}
	_ = m
	_ = s
}
`)
	for _, r := range refs {
		if r.ReferenceKind == "NEW_EXPR" {
			t.Errorf("map/slice literal should not emit NEW_EXPR; got %s:%s", r.ReferenceKind, r.TargetName)
		}
	}
}

// ---------------------------------------------------------------------------
// DECORATOR tests
// ---------------------------------------------------------------------------

func TestPythonDecorator(t *testing.T) {
	refs := parseAndExtractReferences(t, "python", `
def my_decorator(func):
    return func

@my_decorator
def hello():
    pass
`)
	r := findReferenceByKindAndName(refs, "DECORATOR", "my_decorator")
	if r == nil {
		t.Errorf("expected DECORATOR reference to my_decorator; have: %v", referenceNames(refs))
	}
}

func TestJavaAnnotation(t *testing.T) {
	refs := parseAndExtractReferences(t, "java", `
class Main {
  @Override
  public String toString() {
    return "main";
  }
}
`)
	r := findReferenceByKindAndName(refs, "DECORATOR", "Override")
	if r == nil {
		t.Errorf("expected DECORATOR reference to Override in Java; have: %v", referenceNames(refs))
	}
}

// ---------------------------------------------------------------------------
// FETCH reference tests
// ---------------------------------------------------------------------------

func TestFetchReference(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
async function loadData() {
  const res = await fetch("/api/users");
}
`)
	r := findReferenceByKindAndName(refs, "FETCH", "fetch")
	if r == nil {
		t.Fatal("expected FETCH reference")
	}
}

// ---------------------------------------------------------------------------
// Resolution scope tests
// ---------------------------------------------------------------------------

func TestResolutionScopeLocal(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
function helper() {}
function main() {
  helper();
}
`)
	r := findReferenceByKindAndName(refs, "CALL", "helper")
	if r == nil {
		t.Fatal("expected CALL reference to helper")
	}
	if r.ResolutionScope != "LOCAL" {
		t.Errorf("expected LOCAL, got %s", r.ResolutionScope)
	}
	if r.Confidence != "HIGH" {
		t.Errorf("expected HIGH confidence, got %s", r.Confidence)
	}
}

func TestResolutionScopeImported(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
import { readFile } from "fs";
readFile("test.txt");
`)
	r := findReferenceByKindAndName(refs, "CALL", "readFile")
	if r == nil {
		t.Fatal("expected CALL reference to readFile")
	}
	if r.ResolutionScope != "IMPORTED" {
		t.Errorf("expected IMPORTED, got %s", r.ResolutionScope)
	}
}

func TestResolutionScopeMember(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
const result = obj.method();
`)
	r := findReferenceByKindAndName(refs, "CALL", "method")
	if r == nil {
		t.Fatal("expected CALL reference to method")
	}
	if r.ResolutionScope != "MEMBER" {
		t.Errorf("expected MEMBER, got %s", r.ResolutionScope)
	}
}

// ---------------------------------------------------------------------------
// Dedup / edge cases
// ---------------------------------------------------------------------------

func TestDeduplication(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
function foo() {}
function bar() {
  foo();
  foo();
}
`)
	count := 0
	for _, r := range refs {
		if r.TargetName == "foo" && r.ReferenceKind == "CALL" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 CALL refs to foo (different lines), got %d", count)
	}
}

func TestEnclosingSymbol(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", `
function outer() {
  console.log("test");
}
`)
	r := findReferenceByKindAndName(refs, "CALL", "log")
	if r == nil {
		t.Fatal("expected CALL reference to log")
	}
	if r.SourceSymbolID == "" {
		t.Error("expected SourceSymbolID to be set (enclosing outer function)")
	}
}

func TestRefEmptyFile(t *testing.T) {
	refs := parseAndExtractReferences(t, "typescript", "")
	if refs != nil {
		t.Errorf("expected nil, got %d refs", len(refs))
	}
}

func TestRefNilRoot(t *testing.T) {
	refs := ExtractReferences(nil, nil, nil, nil, "typescript")
	if refs != nil {
		t.Error("expected nil for nil root")
	}
}

// ---------------------------------------------------------------------------
// Tier 2/3 tests
// ---------------------------------------------------------------------------

func TestTier2EmptyRefs(t *testing.T) {
	for _, lang := range []string{"bash", "sql"} {
		grammar := parser.GetGrammar(lang)
		if grammar == nil {
			continue
		}
		pool := parser.NewPool(1)
		content := []byte("echo hello")
		tree, err := pool.Parse(context.Background(), content, grammar)
		pool.Shutdown()
		if err != nil {
			continue
		}
		refs := ExtractReferences(tree.RootNode(), content, nil, nil, lang)
		if refs != nil {
			t.Errorf("expected nil refs for Tier 2 language %s, got %d", lang, len(refs))
		}
	}
}

func TestTier3EmptyRefs(t *testing.T) {
	// Use a Tier 2 grammar to get a real root node, then pass it with a
	// Tier 3 language ID so the tier guard is actually exercised (not just
	// the nil-root early return).
	grammar := parser.GetGrammar("bash")
	if grammar == nil {
		t.Skip("bash grammar unavailable")
	}
	pool := parser.NewPool(1)
	content := []byte("echo hello")
	tree, err := pool.Parse(context.Background(), content, grammar)
	pool.Shutdown()
	if err != nil {
		t.Skip("parse failed")
	}
	refs := ExtractReferences(tree.RootNode(), content, nil, nil, "json")
	if refs != nil {
		t.Error("expected nil for Tier 3")
	}
}

// ---------------------------------------------------------------------------
// Multi-language smoke tests
// ---------------------------------------------------------------------------

func TestPythonCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "python", `
def greet(name):
    print(name)

greet("world")
`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in Python; have: %v", referenceNames(refs))
	}
}

func TestGoCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "go", `
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)
	if r := findReferenceByKindAndName(refs, "CALL", "Println"); r == nil {
		t.Errorf("expected CALL reference to Println in Go; have: %v", referenceNames(refs))
	}
}

func TestRustCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "rust", `
fn greet(name: &str) {
    println!("Hello, {}", name);
}

fn main() {
    greet("world");
}
`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in Rust; have: %v", referenceNames(refs))
	}
}

func TestCSharpCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "csharp", `
class Program {
  static void Main() {
    Console.WriteLine("hello");
  }
}
`)
	if r := findReferenceByKindAndName(refs, "CALL", "WriteLine"); r == nil {
		t.Errorf("expected CALL reference to WriteLine in C#; have: %v", referenceNames(refs))
	}
}

func TestSwiftCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "swift", `
func greet(_ name: String) {
    print(name)
}

greet("world")
`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in Swift; have: %v", referenceNames(refs))
	}
}

func TestRubyCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "ruby", `
def greet(name)
  puts name
end

greet("world")
`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in Ruby; have: %v", referenceNames(refs))
	}
}

func TestPHPCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "php", `<?php
function greet($name) {
  echo $name;
}

greet("world");
?>`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in PHP; have: %v", referenceNames(refs))
	}
}

func TestKotlinCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "kotlin", `
fun greet(name: String) {
    println(name)
}

fun main() {
    greet("world")
}
`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in Kotlin; have: %v", referenceNames(refs))
	}
}

func TestCCallRef(t *testing.T) {
	refs := parseAndExtractReferences(t, "c", `
void greet(const char* name) {
    printf("%s\n", name);
}

int main() {
    greet("world");
    return 0;
}
`)
	if r := findReferenceByKindAndName(refs, "CALL", "greet"); r == nil {
		t.Errorf("expected CALL reference to greet in C; have: %v", referenceNames(refs))
	}
}

func TestCppInheritance(t *testing.T) {
	refs := parseAndExtractReferences(t, "cpp", `
class Animal {
public:
    virtual void speak() = 0;
};

class Dog : public Animal {
public:
    void speak() override {}
};
`)
	r := findReferenceByKindAndName(refs, "EXTENDS", "Animal")
	if r == nil {
		t.Errorf("expected EXTENDS reference to Animal in C++; have: %v", referenceNames(refs))
	}
}

// ---------------------------------------------------------------------------
// Reference ID determinism
// ---------------------------------------------------------------------------

func TestReferenceIDDeterministic(t *testing.T) {
	refs1 := parseAndExtractReferences(t, "typescript", `
function foo() {}
foo();
`)
	refs2 := parseAndExtractReferences(t, "typescript", `
function foo() {}
foo();
`)
	if len(refs1) == 0 || len(refs2) == 0 {
		t.Fatal("expected at least one ref")
	}
	r1 := findReferenceByKindAndName(refs1, "CALL", "foo")
	r2 := findReferenceByKindAndName(refs2, "CALL", "foo")
	if r1 == nil || r2 == nil {
		t.Fatal("expected CALL ref to foo")
	}
	if r1.ReferenceID != r2.ReferenceID {
		t.Errorf("expected deterministic IDs: %s != %s", r1.ReferenceID, r2.ReferenceID)
	}
}
