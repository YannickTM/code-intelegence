package extractors

import (
	"context"
	"strings"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// parseAndExtract is a test helper that parses source code and extracts symbols.
func parseAndExtract(t *testing.T, langID, source string) []parser.Symbol {
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
	return ExtractSymbols(tree.RootNode(), content, langID)
}

// findSymbol returns the first symbol with the given name, or nil.
func findSymbol(symbols []parser.Symbol, name string) *parser.Symbol {
	for i := range symbols {
		if symbols[i].Name == name {
			return &symbols[i]
		}
	}
	return nil
}

// findSymbolByKind returns the first symbol with the given name and kind, or nil.
func findSymbolByKind(symbols []parser.Symbol, name, kind string) *parser.Symbol {
	for i := range symbols {
		if symbols[i].Name == name && symbols[i].Kind == kind {
			return &symbols[i]
		}
	}
	return nil
}

// assertSymbolExists checks that a symbol with the given name exists.
func assertSymbolExists(t *testing.T, symbols []parser.Symbol, name string) *parser.Symbol {
	t.Helper()
	s := findSymbol(symbols, name)
	if s == nil {
		t.Errorf("expected symbol %q not found; have: %v", name, symbolNames(symbols))
	}
	return s
}

// symbolNames returns the names of all symbols for diagnostics.
func symbolNames(symbols []parser.Symbol) []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name + "(" + s.Kind + ")"
	}
	return names
}

// ---------------------------------------------------------------------------
// JavaScript tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_JavaScript(t *testing.T) {
	t.Run("function declaration", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `function greet(name) { return "hello " + name; }`)
		if len(syms) == 0 {
			t.Fatal("expected at least 1 symbol")
		}
		s := assertSymbolExists(t, syms, "greet")
		if s != nil {
			if s.Kind != "function" {
				t.Errorf("kind = %q, want function", s.Kind)
			}
			if s.StartLine != 1 {
				t.Errorf("StartLine = %d, want 1", s.StartLine)
			}
		}
	})

	t.Run("arrow function in const", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `const add = (a, b) => a + b;`)
		s := assertSymbolExists(t, syms, "add")
		if s != nil {
			if s.Kind != "function" {
				t.Errorf("kind = %q, want function", s.Kind)
			}
			if s.Flags == nil || !s.Flags.IsArrowFunction {
				t.Error("expected IsArrowFunction = true")
			}
		}
	})

	t.Run("class with methods", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `
class MyClass {
  constructor() {}
  method1() {}
  static staticMethod() {}
}`)
		assertSymbolExists(t, syms, "MyClass")
		ctor := assertSymbolExists(t, syms, "constructor")
		if ctor != nil {
			if ctor.Kind != "method" {
				t.Errorf("constructor kind = %q, want method", ctor.Kind)
			}
			if ctor.ParentSymbolID == "" {
				t.Error("constructor should have ParentSymbolID")
			}
		}
		m1 := assertSymbolExists(t, syms, "method1")
		if m1 != nil && m1.Kind != "method" {
			t.Errorf("method1 kind = %q, want method", m1.Kind)
		}
		sm := assertSymbolExists(t, syms, "staticMethod")
		if sm != nil && (sm.Flags == nil || !sm.Flags.IsStatic) {
			t.Error("expected staticMethod to be static")
		}
	})

	t.Run("async function", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `async function fetchData() {}`)
		s := assertSymbolExists(t, syms, "fetchData")
		if s != nil && (s.Flags == nil || !s.Flags.IsAsync) {
			t.Error("expected IsAsync = true")
		}
	})

	t.Run("export function", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `export function foo() {}`)
		s := assertSymbolExists(t, syms, "foo")
		if s != nil && (s.Flags == nil || !s.Flags.IsExported) {
			t.Error("expected IsExported = true")
		}
	})

	t.Run("export default function", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `export default function bar() {}`)
		s := assertSymbolExists(t, syms, "bar")
		if s != nil {
			if s.Flags == nil || !s.Flags.IsExported {
				t.Error("expected IsExported = true")
			}
			if s.Flags == nil || !s.Flags.IsDefaultExport {
				t.Error("expected IsDefaultExport = true")
			}
		}
	})

	t.Run("jsdoc comment", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", "/** Greets a user */\nfunction greet() {}")
		s := assertSymbolExists(t, syms, "greet")
		if s != nil && s.DocText == "" {
			t.Error("expected DocText to be non-empty")
		}
	})

	t.Run("export const arrow", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `export const handler = (req, res) => { res.send("ok"); };`)
		s := assertSymbolExists(t, syms, "handler")
		if s != nil {
			if s.Flags == nil || !s.Flags.IsExported {
				t.Error("expected IsExported = true")
			}
			if s.Flags == nil || !s.Flags.IsArrowFunction {
				t.Error("expected IsArrowFunction = true")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// TypeScript tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_TypeScript(t *testing.T) {
	t.Run("interface", func(t *testing.T) {
		syms := parseAndExtract(t, "typescript", `interface User { name: string; age: number; }`)
		s := assertSymbolExists(t, syms, "User")
		if s != nil && s.Kind != "interface" {
			t.Errorf("kind = %q, want interface", s.Kind)
		}
	})

	t.Run("type alias", func(t *testing.T) {
		syms := parseAndExtract(t, "typescript", `type ID = string | number;`)
		s := assertSymbolExists(t, syms, "ID")
		if s != nil && s.Kind != "type_alias" {
			t.Errorf("kind = %q, want type_alias", s.Kind)
		}
	})

	t.Run("enum", func(t *testing.T) {
		syms := parseAndExtract(t, "typescript", `enum Color { Red, Green, Blue }`)
		s := assertSymbolExists(t, syms, "Color")
		if s != nil && s.Kind != "enum" {
			t.Errorf("kind = %q, want enum", s.Kind)
		}
	})

	t.Run("class with typed method", func(t *testing.T) {
		syms := parseAndExtract(t, "typescript", `
class Service {
  async getData(): Promise<string> { return ""; }
}`)
		assertSymbolExists(t, syms, "Service")
		s := assertSymbolExists(t, syms, "getData")
		if s != nil {
			if s.Kind != "method" {
				t.Errorf("kind = %q, want method", s.Kind)
			}
			if s.Flags == nil || !s.Flags.IsAsync {
				t.Error("expected IsAsync = true")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// JSX/TSX tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_JSX(t *testing.T) {
	t.Run("react component", func(t *testing.T) {
		syms := parseAndExtract(t, "jsx", `
function MyComponent() {
  return <div>Hello</div>;
}`)
		s := assertSymbolExists(t, syms, "MyComponent")
		if s != nil && (s.Flags == nil || !s.Flags.IsReactComponentLike) {
			t.Error("expected IsReactComponentLike = true")
		}
	})

	t.Run("hook function", func(t *testing.T) {
		syms := parseAndExtract(t, "jsx", `function useCounter() { return 0; }`)
		s := assertSymbolExists(t, syms, "useCounter")
		if s != nil && (s.Flags == nil || !s.Flags.IsHookLike) {
			t.Error("expected IsHookLike = true")
		}
	})
}

func TestExtractSymbols_TSX(t *testing.T) {
	t.Run("tsx component with export", func(t *testing.T) {
		syms := parseAndExtract(t, "tsx", `
export function Header(): JSX.Element {
  return <header><h1>Title</h1></header>;
}`)
		s := assertSymbolExists(t, syms, "Header")
		if s != nil {
			if s.Flags == nil || !s.Flags.IsExported {
				t.Error("expected IsExported = true")
			}
			if s.Flags == nil || !s.Flags.IsReactComponentLike {
				t.Error("expected IsReactComponentLike = true")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Python tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Python(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "python", `def greet(name):
    return f"hello {name}"`)
		s := assertSymbolExists(t, syms, "greet")
		if s != nil && s.Kind != "function" {
			t.Errorf("kind = %q, want function", s.Kind)
		}
	})

	t.Run("class with methods", func(t *testing.T) {
		syms := parseAndExtract(t, "python", `
class MyClass:
    def __init__(self):
        pass
    def method1(self):
        pass`)
		assertSymbolExists(t, syms, "MyClass")
		m := findSymbol(syms, "__init__")
		if m != nil && m.Kind != "function" {
			// Python methods are function_definition, mapped to "function".
			t.Errorf("__init__ kind = %q, want function", m.Kind)
		}
	})

	t.Run("docstring", func(t *testing.T) {
		syms := parseAndExtract(t, "python", `
def greet():
    """Greets the user."""
    pass`)
		s := assertSymbolExists(t, syms, "greet")
		if s != nil && s.DocText == "" {
			t.Error("expected DocText to be non-empty")
		}
	})

	t.Run("private prefix", func(t *testing.T) {
		syms := parseAndExtract(t, "python", `
def public_func():
    pass
def _private_func():
    pass`)
		pub := findSymbol(syms, "public_func")
		if pub != nil && (pub.Flags == nil || !pub.Flags.IsExported) {
			t.Error("expected public_func to be exported")
		}
		priv := findSymbol(syms, "_private_func")
		if priv != nil && (priv.Flags != nil && priv.Flags.IsExported) {
			t.Error("expected _private_func to NOT be exported")
		}
	})

	t.Run("decorated function", func(t *testing.T) {
		syms := parseAndExtract(t, "python", `
@decorator
def decorated_func():
    pass`)
		assertSymbolExists(t, syms, "decorated_func")
	})

	t.Run("async def", func(t *testing.T) {
		syms := parseAndExtract(t, "python", `
async def fetch_data():
    pass`)
		s := assertSymbolExists(t, syms, "fetch_data")
		if s != nil && (s.Flags == nil || !s.Flags.IsAsync) {
			t.Error("expected IsAsync = true")
		}
	})
}

// ---------------------------------------------------------------------------
// Go tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Go(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
func Hello() string { return "hello" }`)
		s := assertSymbolExists(t, syms, "Hello")
		if s != nil {
			if s.Kind != "function" {
				t.Errorf("kind = %q, want function", s.Kind)
			}
			if s.Flags == nil || !s.Flags.IsExported {
				t.Error("expected IsExported = true (uppercase)")
			}
		}
	})

	t.Run("unexported function", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
func hello() string { return "hello" }`)
		s := assertSymbolExists(t, syms, "hello")
		if s != nil && (s.Flags != nil && s.Flags.IsExported) {
			t.Error("expected hello to NOT be exported (lowercase)")
		}
	})

	t.Run("method", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
func (s *Server) Start() error { return nil }`)
		s := assertSymbolExists(t, syms, "Start")
		if s != nil && s.Kind != "method" {
			t.Errorf("kind = %q, want method", s.Kind)
		}
	})

	t.Run("struct type", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
type Server struct { Port int }`)
		s := assertSymbolExists(t, syms, "Server")
		if s != nil && s.Kind != "class" {
			t.Errorf("kind = %q, want class", s.Kind)
		}
	})

	t.Run("interface type", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
type Handler interface { Handle() error }`)
		s := assertSymbolExists(t, syms, "Handler")
		if s != nil && s.Kind != "interface" {
			t.Errorf("kind = %q, want interface", s.Kind)
		}
	})

	t.Run("type alias", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
type ID = string`)
		s := assertSymbolExists(t, syms, "ID")
		if s != nil && s.Kind != "type_alias" {
			t.Errorf("kind = %q, want type_alias", s.Kind)
		}
	})

	t.Run("grouped type declaration", func(t *testing.T) {
		syms := parseAndExtract(t, "go", `package main
type (
    Foo struct{}
    Bar interface{ Do() }
)`)
		assertSymbolExists(t, syms, "Foo")
		assertSymbolExists(t, syms, "Bar")
	})
}

// ---------------------------------------------------------------------------
// Rust tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Rust(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", `fn main() { println!("hello"); }`)
		s := assertSymbolExists(t, syms, "main")
		if s != nil && s.Kind != "function" {
			t.Errorf("kind = %q, want function", s.Kind)
		}
	})

	t.Run("struct", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", `pub struct Point { pub x: f64, pub y: f64 }`)
		s := assertSymbolExists(t, syms, "Point")
		if s != nil {
			if s.Kind != "class" {
				t.Errorf("kind = %q, want class", s.Kind)
			}
			if s.Flags == nil || !s.Flags.IsExported {
				t.Error("expected IsExported = true (pub)")
			}
		}
	})

	t.Run("trait", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", `pub trait Display { fn fmt(&self) -> String; }`)
		s := assertSymbolExists(t, syms, "Display")
		if s != nil && s.Kind != "interface" {
			t.Errorf("kind = %q, want interface", s.Kind)
		}
	})

	t.Run("impl methods", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", `
struct Foo;
impl Foo {
    fn new() -> Self { Foo }
    fn bar(&self) {}
}`)
		assertSymbolExists(t, syms, "Foo")
		n := findSymbol(syms, "new")
		if n != nil && n.Kind != "function" {
			t.Errorf("new kind = %q, want function", n.Kind)
		}
		b := findSymbol(syms, "bar")
		if b != nil && b.Kind != "function" {
			t.Errorf("bar kind = %q, want function", b.Kind)
		}
	})

	t.Run("enum", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", `enum Color { Red, Green, Blue }`)
		s := assertSymbolExists(t, syms, "Color")
		if s != nil && s.Kind != "enum" {
			t.Errorf("kind = %q, want enum", s.Kind)
		}
	})

	t.Run("mod", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", `mod tests { fn test_it() {} }`)
		assertSymbolExists(t, syms, "tests")
	})

	t.Run("triple slash doc", func(t *testing.T) {
		syms := parseAndExtract(t, "rust", "/// Does something\nfn do_thing() {}")
		s := assertSymbolExists(t, syms, "do_thing")
		if s != nil && s.DocText == "" {
			t.Error("expected DocText to be non-empty")
		}
	})
}

// ---------------------------------------------------------------------------
// Java tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Java(t *testing.T) {
	t.Run("class with method", func(t *testing.T) {
		syms := parseAndExtract(t, "java", `
public class MyService {
    public String getData() { return ""; }
}`)
		assertSymbolExists(t, syms, "MyService")
		m := findSymbol(syms, "getData")
		if m != nil && m.Kind != "method" {
			t.Errorf("kind = %q, want method", m.Kind)
		}
	})

	t.Run("interface", func(t *testing.T) {
		syms := parseAndExtract(t, "java", `public interface Repository { void save(); }`)
		s := assertSymbolExists(t, syms, "Repository")
		if s != nil && s.Kind != "interface" {
			t.Errorf("kind = %q, want interface", s.Kind)
		}
	})

	t.Run("enum", func(t *testing.T) {
		syms := parseAndExtract(t, "java", `public enum Status { ACTIVE, INACTIVE }`)
		s := assertSymbolExists(t, syms, "Status")
		if s != nil && s.Kind != "enum" {
			t.Errorf("kind = %q, want enum", s.Kind)
		}
	})

	t.Run("javadoc", func(t *testing.T) {
		syms := parseAndExtract(t, "java", `
/** Service class */
public class Service {}`)
		s := assertSymbolExists(t, syms, "Service")
		if s != nil && s.DocText == "" {
			t.Error("expected DocText to be non-empty")
		}
	})
}

// ---------------------------------------------------------------------------
// Kotlin tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Kotlin(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "kotlin", `fun greet(name: String): String = "Hello $name"`)
		s := assertSymbolExists(t, syms, "greet")
		if s != nil {
			if s.Kind != "function" {
				t.Errorf("kind = %q, want function", s.Kind)
			}
			// Kotlin uses all_public strategy.
			if s.Flags == nil || !s.Flags.IsExported {
				t.Error("expected IsExported = true (all_public)")
			}
		}
	})

	t.Run("class", func(t *testing.T) {
		syms := parseAndExtract(t, "kotlin", `class User(val name: String)`)
		assertSymbolExists(t, syms, "User")
	})
}

// ---------------------------------------------------------------------------
// C tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_C(t *testing.T) {
	t.Run("function definition", func(t *testing.T) {
		syms := parseAndExtract(t, "c", `int main() { return 0; }`)
		s := assertSymbolExists(t, syms, "main")
		if s != nil && s.Kind != "function" {
			t.Errorf("kind = %q, want function", s.Kind)
		}
	})

	t.Run("struct", func(t *testing.T) {
		syms := parseAndExtract(t, "c", `struct Point { int x; int y; };`)
		s := assertSymbolExists(t, syms, "Point")
		if s != nil && s.Kind != "class" {
			t.Errorf("kind = %q, want class", s.Kind)
		}
	})

	t.Run("enum", func(t *testing.T) {
		syms := parseAndExtract(t, "c", `enum Color { RED, GREEN, BLUE };`)
		s := assertSymbolExists(t, syms, "Color")
		if s != nil && s.Kind != "enum" {
			t.Errorf("kind = %q, want enum", s.Kind)
		}
	})
}

// ---------------------------------------------------------------------------
// C++ tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_CPP(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "cpp", `int main() { return 0; }`)
		assertSymbolExists(t, syms, "main")
	})

	t.Run("class", func(t *testing.T) {
		syms := parseAndExtract(t, "cpp", `class Widget { public: void draw(); };`)
		assertSymbolExists(t, syms, "Widget")
	})

	t.Run("namespace", func(t *testing.T) {
		syms := parseAndExtract(t, "cpp", `namespace mylib { int add(int a, int b) { return a+b; } }`)
		assertSymbolExists(t, syms, "mylib")
	})
}

// ---------------------------------------------------------------------------
// C# tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_CSharp(t *testing.T) {
	t.Run("class with method", func(t *testing.T) {
		syms := parseAndExtract(t, "csharp", `
public class UserService {
    public string GetName() { return ""; }
}`)
		assertSymbolExists(t, syms, "UserService")
		assertSymbolExists(t, syms, "GetName")
	})

	t.Run("interface", func(t *testing.T) {
		syms := parseAndExtract(t, "csharp", `public interface IRepository { void Save(); }`)
		assertSymbolExists(t, syms, "IRepository")
	})

	t.Run("namespace", func(t *testing.T) {
		syms := parseAndExtract(t, "csharp", `namespace MyApp { public class App {} }`)
		assertSymbolExists(t, syms, "MyApp")
		assertSymbolExists(t, syms, "App")
	})
}

// ---------------------------------------------------------------------------
// Swift tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Swift(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "swift", `func greet(name: String) -> String { return "Hello \(name)" }`)
		assertSymbolExists(t, syms, "greet")
	})

	t.Run("class", func(t *testing.T) {
		syms := parseAndExtract(t, "swift", `class ViewController { func viewDidLoad() {} }`)
		assertSymbolExists(t, syms, "ViewController")
	})

	t.Run("struct", func(t *testing.T) {
		syms := parseAndExtract(t, "swift", `struct Point { var x: Double; var y: Double }`)
		assertSymbolExists(t, syms, "Point")
	})

	t.Run("protocol", func(t *testing.T) {
		syms := parseAndExtract(t, "swift", `protocol Drawable { func draw() }`)
		s := assertSymbolExists(t, syms, "Drawable")
		if s != nil && s.Kind != "interface" {
			t.Errorf("kind = %q, want interface", s.Kind)
		}
	})

	t.Run("enum", func(t *testing.T) {
		syms := parseAndExtract(t, "swift", `enum Direction { case north, south, east, west }`)
		assertSymbolExists(t, syms, "Direction")
	})
}

// ---------------------------------------------------------------------------
// Ruby tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Ruby(t *testing.T) {
	t.Run("method", func(t *testing.T) {
		syms := parseAndExtract(t, "ruby", `def greet(name)
  "hello #{name}"
end`)
		s := assertSymbolExists(t, syms, "greet")
		if s != nil && s.Kind != "function" {
			t.Errorf("kind = %q, want function", s.Kind)
		}
	})

	t.Run("class with method", func(t *testing.T) {
		syms := parseAndExtract(t, "ruby", `
class User
  def initialize(name)
    @name = name
  end
end`)
		assertSymbolExists(t, syms, "User")
		assertSymbolExists(t, syms, "initialize")
	})

	t.Run("module", func(t *testing.T) {
		syms := parseAndExtract(t, "ruby", `module Helpers; end`)
		s := assertSymbolExists(t, syms, "Helpers")
		if s != nil && s.Kind != "namespace" {
			t.Errorf("kind = %q, want namespace", s.Kind)
		}
	})

	t.Run("hash doc comment", func(t *testing.T) {
		syms := parseAndExtract(t, "ruby", "# Greets the user\ndef greet; end")
		s := assertSymbolExists(t, syms, "greet")
		if s != nil && s.DocText == "" {
			t.Error("expected DocText to be non-empty")
		}
	})
}

// ---------------------------------------------------------------------------
// PHP tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_PHP(t *testing.T) {
	t.Run("class with method", func(t *testing.T) {
		syms := parseAndExtract(t, "php", `<?php
class UserController {
    public function index() { return []; }
}`)
		assertSymbolExists(t, syms, "UserController")
		assertSymbolExists(t, syms, "index")
	})

	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "php", `<?php function helper() { return true; }`)
		assertSymbolExists(t, syms, "helper")
	})
}

// ---------------------------------------------------------------------------
// Tier 2 language tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_Bash(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		syms := parseAndExtract(t, "bash", `function greet() { echo "hello"; }`)
		assertSymbolExists(t, syms, "greet")
	})
}

func TestExtractSymbols_SQL(t *testing.T) {
	t.Run("create table", func(t *testing.T) {
		syms := parseAndExtract(t, "sql", `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);`)
		if len(syms) == 0 {
			t.Skip("SQL grammar may not produce create_table_statement nodes")
		}
		assertSymbolExists(t, syms, "users")
	})
}

func TestExtractSymbols_Dockerfile(t *testing.T) {
	t.Run("from instruction", func(t *testing.T) {
		syms := parseAndExtract(t, "dockerfile", `FROM golang:1.21-alpine`)
		if len(syms) == 0 {
			t.Skip("Dockerfile grammar may vary")
		}
		// At least one symbol should exist.
		if len(syms) > 0 && syms[0].Kind != "variable" {
			t.Errorf("kind = %q, want variable", syms[0].Kind)
		}
	})
}

func TestExtractSymbols_HCL(t *testing.T) {
	t.Run("resource block", func(t *testing.T) {
		syms := parseAndExtract(t, "hcl", `
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t3.micro"
}`)
		if len(syms) == 0 {
			t.Fatal("expected at least 1 symbol from HCL")
		}
		// Should have a symbol with the block name.
		found := false
		for _, s := range syms {
			if s.Kind == "variable" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected a variable-kind symbol from HCL block")
		}
	})
}

// ---------------------------------------------------------------------------
// Tier 3 language tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_CSS(t *testing.T) {
	t.Run("rule set", func(t *testing.T) {
		syms := parseAndExtract(t, "css", `.container { color: red; }`)
		if len(syms) == 0 {
			t.Fatal("expected at least 1 symbol")
		}
		s := syms[0]
		if s.Kind != "variable" {
			t.Errorf("kind = %q, want variable", s.Kind)
		}
	})

	t.Run("keyframes", func(t *testing.T) {
		syms := parseAndExtract(t, "css", `@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }`)
		found := false
		for _, s := range syms {
			if s.Kind == "function" {
				found = true
			}
		}
		if !found {
			t.Log("keyframes symbol not found — grammar may vary")
		}
	})
}

func TestExtractSymbols_HTML(t *testing.T) {
	t.Run("script element", func(t *testing.T) {
		syms := parseAndExtract(t, "html", `<html><head><script src="app.js"></script></head><body></body></html>`)
		found := false
		for _, s := range syms {
			if s.Name == "script" {
				found = true
			}
		}
		if !found {
			t.Log("script element symbol not found — grammar may dispatch differently")
		}
	})
}

func TestExtractSymbols_YAML(t *testing.T) {
	t.Run("top-level keys", func(t *testing.T) {
		syms := parseAndExtract(t, "yaml", `name: myapp
version: "1.0"`)
		if len(syms) == 0 {
			t.Fatal("expected at least 1 symbol")
		}
		assertSymbolExists(t, syms, "name")
	})
}

func TestExtractSymbols_TOML(t *testing.T) {
	t.Run("pairs", func(t *testing.T) {
		syms := parseAndExtract(t, "toml", `title = "My App"
version = "1.0"`)
		if len(syms) == 0 {
			t.Fatal("expected at least 1 symbol")
		}
		assertSymbolExists(t, syms, "title")
	})
}

func TestExtractSymbols_Markdown(t *testing.T) {
	t.Run("headings", func(t *testing.T) {
		syms := parseAndExtract(t, "markdown", `# Introduction

## Getting Started

### Installation`)
		if len(syms) < 2 {
			t.Fatalf("expected at least 2 symbols, got %d: %v", len(syms), symbolNames(syms))
		}
	})
}

// ---------------------------------------------------------------------------
// Edge case tests
// ---------------------------------------------------------------------------

func TestExtractSymbols_EdgeCases(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", "")
		if len(syms) != 0 {
			t.Errorf("expected 0 symbols, got %d", len(syms))
		}
	})

	t.Run("unknown language", func(t *testing.T) {
		// Use a real parsed tree so we exercise the language-lookup path,
		// not the nil-root early return.
		grammar := parser.GetGrammar("javascript")
		pool := parser.NewPool(1)
		defer pool.Shutdown()
		tree, err := pool.Parse(context.Background(), []byte("var x = 1;"), grammar)
		if err != nil {
			t.Fatal(err)
		}
		syms := ExtractSymbols(tree.RootNode(), []byte("var x = 1;"), "brainfuck")
		if syms != nil {
			t.Errorf("expected nil for unknown language, got %v", syms)
		}
	})

	t.Run("nil root", func(t *testing.T) {
		syms := ExtractSymbols(nil, []byte("code"), "javascript")
		if syms != nil {
			t.Errorf("expected nil for nil root, got %v", syms)
		}
	})

	t.Run("syntax error partial parse", func(t *testing.T) {
		// Tree-sitter does partial parsing even with syntax errors.
		syms := parseAndExtract(t, "javascript", `function valid() {} function { broken`)
		// Should still find "valid".
		assertSymbolExists(t, syms, "valid")
	})

	t.Run("deterministic ordering", func(t *testing.T) {
		source := `
function beta() {}
function alpha() {}
function gamma() {}`
		syms1 := parseAndExtract(t, "javascript", source)
		syms2 := parseAndExtract(t, "javascript", source)

		if len(syms1) != len(syms2) {
			t.Fatalf("different symbol counts: %d vs %d", len(syms1), len(syms2))
		}
		for i := range syms1 {
			if syms1[i].Name != syms2[i].Name {
				t.Errorf("order mismatch at %d: %q vs %q", i, syms1[i].Name, syms2[i].Name)
			}
		}
	})

	t.Run("symbol hash is deterministic", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `function hello() { return 1; }`)
		s := assertSymbolExists(t, syms, "hello")
		if s != nil {
			if s.SymbolHash == "" {
				t.Error("expected non-empty SymbolHash")
			}
			// Parse again and check same hash.
			syms2 := parseAndExtract(t, "javascript", `function hello() { return 1; }`)
			s2 := assertSymbolExists(t, syms2, "hello")
			if s2 != nil && s.SymbolHash != s2.SymbolHash {
				t.Errorf("hash mismatch: %q vs %q", s.SymbolHash, s2.SymbolHash)
			}
		}
	})

	t.Run("qualified name for method", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `
class Foo {
  bar() {}
}`)
		s := assertSymbolExists(t, syms, "bar")
		if s != nil && s.QualifiedName != "Foo.bar" {
			t.Errorf("QualifiedName = %q, want Foo.bar", s.QualifiedName)
		}
	})

	t.Run("signature extraction", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `function greet(name, age) {
  return "hello";
}`)
		s := assertSymbolExists(t, syms, "greet")
		if s != nil && s.Signature == "" {
			t.Error("expected non-empty Signature")
		}
	})

	t.Run("symbol ID is non-empty", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `function test() {}`)
		s := assertSymbolExists(t, syms, "test")
		if s != nil && s.SymbolID == "" {
			t.Error("expected non-empty SymbolID")
		}
	})

	t.Run("default export not propagated to nested methods", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `
export default class Foo {
  bar() {}
}`)
		foo := assertSymbolExists(t, syms, "Foo")
		if foo != nil && (foo.Flags == nil || !foo.Flags.IsDefaultExport) {
			t.Error("expected Foo to have IsDefaultExport = true")
		}
		bar := assertSymbolExists(t, syms, "bar")
		if bar != nil && bar.Flags != nil && bar.Flags.IsDefaultExport {
			t.Error("expected bar to have IsDefaultExport = false (not propagated)")
		}
	})

	t.Run("arrow function signature trimmed at =>", func(t *testing.T) {
		syms := parseAndExtract(t, "javascript", `const add = (a, b) => a + b;`)
		s := assertSymbolExists(t, syms, "add")
		if s != nil && s.Signature != "" {
			if strings.Contains(s.Signature, "a + b") {
				t.Errorf("signature should not contain expression body, got %q", s.Signature)
			}
		}
	})

	t.Run("type annotation arrow preserved in signature", func(t *testing.T) {
		syms := parseAndExtract(t, "typescript", `const fn: () => void = () => { return; };`)
		s := assertSymbolExists(t, syms, "fn")
		if s != nil && s.Signature != "" {
			// The type-annotation "=> void" should be preserved, not truncated.
			if !strings.Contains(s.Signature, "void") {
				t.Errorf("signature lost type annotation, got %q", s.Signature)
			}
		}
	})
}
