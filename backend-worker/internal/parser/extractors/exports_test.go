package extractors

import (
	"context"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// parseAndExtractExports parses source code and extracts exports.
func parseAndExtractExports(t *testing.T, langID, source string) []parser.Export {
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
	return ExtractExports(root, content, langID, symbols)
}

// findExport returns the first export with the given name, or nil.
func findExport(exports []parser.Export, name string) *parser.Export {
	for i := range exports {
		if exports[i].ExportedName == name {
			return &exports[i]
		}
	}
	return nil
}

// findExportByKind returns the first export with the given kind, or nil.
func findExportByKind(exports []parser.Export, kind string) *parser.Export {
	for i := range exports {
		if exports[i].ExportKind == kind {
			return &exports[i]
		}
	}
	return nil
}

// assertExportExists asserts that an export with the given name exists.
func assertExportExists(t *testing.T, exports []parser.Export, name string) *parser.Export {
	t.Helper()
	e := findExport(exports, name)
	if e == nil {
		t.Errorf("expected export %q not found; have: %v", name, exportNames(exports))
	}
	return e
}

// assertNoExport asserts that no export with the given name exists.
func assertNoExport(t *testing.T, exports []parser.Export, name string) {
	t.Helper()
	if e := findExport(exports, name); e != nil {
		t.Errorf("unexpected export %q found with kind %s", name, e.ExportKind)
	}
}

// exportNames returns a slice of all export names for debugging.
func exportNames(exports []parser.Export) []string {
	names := make([]string, len(exports))
	for i, e := range exports {
		names[i] = e.ExportedName
	}
	return names
}

// ---------------------------------------------------------------------------
// JS/TS tests
// ---------------------------------------------------------------------------

func TestDirectNamedExport(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		exports := parseAndExtractExports(t, "javascript", `export function foo() {}`)
		e := assertExportExists(t, exports, "foo")
		if e != nil && e.ExportKind != "NAMED" {
			t.Errorf("kind = %q, want NAMED", e.ExportKind)
		}
	})

	t.Run("class", func(t *testing.T) {
		exports := parseAndExtractExports(t, "javascript", `export class Bar {}`)
		e := assertExportExists(t, exports, "Bar")
		if e != nil && e.ExportKind != "NAMED" {
			t.Errorf("kind = %q, want NAMED", e.ExportKind)
		}
	})

	t.Run("const", func(t *testing.T) {
		exports := parseAndExtractExports(t, "javascript", `export const x = 1;`)
		e := assertExportExists(t, exports, "x")
		if e != nil && e.ExportKind != "NAMED" {
			t.Errorf("kind = %q, want NAMED", e.ExportKind)
		}
	})

	t.Run("multiple const", func(t *testing.T) {
		exports := parseAndExtractExports(t, "javascript", `export const a = 1, b = 2;`)
		assertExportExists(t, exports, "a")
		assertExportExists(t, exports, "b")
		if len(exports) != 2 {
			t.Errorf("got %d exports, want 2", len(exports))
		}
	})
}

func TestExportClause(t *testing.T) {
	src := `
function foo() {}
const bar = 1;
export { foo, bar };
`
	exports := parseAndExtractExports(t, "javascript", src)
	assertExportExists(t, exports, "foo")
	assertExportExists(t, exports, "bar")

	t.Run("with alias", func(t *testing.T) {
		src := `
function foo() {}
export { foo as myFoo };
`
		exports := parseAndExtractExports(t, "javascript", src)
		e := assertExportExists(t, exports, "myFoo")
		if e != nil && e.LocalName != "foo" {
			t.Errorf("LocalName = %q, want foo", e.LocalName)
		}
	})
}

func TestDefaultExport(t *testing.T) {
	t.Run("default class", func(t *testing.T) {
		exports := parseAndExtractExports(t, "javascript", `export default class Foo {}`)
		e := findExportByKind(exports, "DEFAULT")
		if e == nil {
			t.Fatal("no DEFAULT export found")
		}
		if e.LocalName != "Foo" {
			t.Errorf("LocalName = %q, want Foo", e.LocalName)
		}
	})

	t.Run("default function", func(t *testing.T) {
		exports := parseAndExtractExports(t, "javascript", `export default function greet() {}`)
		e := findExportByKind(exports, "DEFAULT")
		if e == nil {
			t.Fatal("no DEFAULT export found")
		}
		if e.LocalName != "greet" {
			t.Errorf("LocalName = %q, want greet", e.LocalName)
		}
	})

	t.Run("default expression", func(t *testing.T) {
		src := `
const val = 42;
export default val;
`
		exports := parseAndExtractExports(t, "javascript", src)
		e := findExportByKind(exports, "DEFAULT")
		if e == nil {
			t.Fatal("no DEFAULT export found")
		}
		if e.LocalName != "val" {
			t.Errorf("LocalName = %q, want val", e.LocalName)
		}
	})
}

func TestAliasedDefault(t *testing.T) {
	src := `
function foo() {}
export { foo as default };
`
	exports := parseAndExtractExports(t, "javascript", src)
	e := findExportByKind(exports, "DEFAULT")
	if e == nil {
		t.Fatal("no DEFAULT export found")
	}
	if e.LocalName != "foo" {
		t.Errorf("LocalName = %q, want foo", e.LocalName)
	}
}

func TestReExport(t *testing.T) {
	exports := parseAndExtractExports(t, "javascript", `export { foo, bar } from './mod';`)
	if len(exports) != 2 {
		t.Fatalf("got %d exports, want 2", len(exports))
	}
	for _, e := range exports {
		if e.ExportKind != "REEXPORT" {
			t.Errorf("export %q kind = %q, want REEXPORT", e.ExportedName, e.ExportKind)
		}
		if e.SourceModule != "./mod" {
			t.Errorf("export %q SourceModule = %q, want ./mod", e.ExportedName, e.SourceModule)
		}
		if e.SymbolID != "" {
			t.Errorf("export %q should have empty SymbolID for re-export", e.ExportedName)
		}
	}
}

func TestExportAll(t *testing.T) {
	exports := parseAndExtractExports(t, "javascript", `export * from './mod';`)
	if len(exports) != 1 {
		t.Fatalf("got %d exports, want 1", len(exports))
	}
	e := exports[0]
	if e.ExportKind != "EXPORT_ALL" {
		t.Errorf("kind = %q, want EXPORT_ALL", e.ExportKind)
	}
	if e.SourceModule != "./mod" {
		t.Errorf("SourceModule = %q, want ./mod", e.SourceModule)
	}
}

func TestNamespaceReExport(t *testing.T) {
	exports := parseAndExtractExports(t, "javascript", `export * as utils from './utils';`)
	if len(exports) != 1 {
		t.Fatalf("got %d exports, want 1", len(exports))
	}
	e := exports[0]
	if e.ExportKind != "EXPORT_ALL" {
		t.Errorf("kind = %q, want EXPORT_ALL", e.ExportKind)
	}
	if e.ExportedName != "utils" {
		t.Errorf("ExportedName = %q, want utils", e.ExportedName)
	}
	if e.SourceModule != "./utils" {
		t.Errorf("SourceModule = %q, want ./utils", e.SourceModule)
	}
}

func TestTypeOnlyExport(t *testing.T) {
	t.Run("type export clause", func(t *testing.T) {
		src := `
interface Foo {}
type Bar = string;
export type { Foo, Bar };
`
		exports := parseAndExtractExports(t, "typescript", src)
		if len(exports) < 2 {
			t.Fatalf("got %d exports, want at least 2", len(exports))
		}
		for _, e := range exports {
			if e.ExportKind != "TYPE_ONLY" {
				t.Errorf("export %q kind = %q, want TYPE_ONLY", e.ExportedName, e.ExportKind)
			}
		}
	})

	t.Run("type re-export", func(t *testing.T) {
		exports := parseAndExtractExports(t, "typescript", `export type { Foo } from './types';`)
		if len(exports) != 1 {
			t.Fatalf("got %d exports, want 1", len(exports))
		}
		e := exports[0]
		if e.ExportKind != "TYPE_ONLY" {
			t.Errorf("kind = %q, want TYPE_ONLY", e.ExportKind)
		}
		if e.SourceModule != "./types" {
			t.Errorf("SourceModule = %q, want ./types", e.SourceModule)
		}
	})

	t.Run("per-specifier type modifier", func(t *testing.T) {
		src := `
interface Foo {}
function bar() {}
export { type Foo, bar };
`
		exports := parseAndExtractExports(t, "typescript", src)
		foo := assertExportExists(t, exports, "Foo")
		if foo != nil && foo.ExportKind != "TYPE_ONLY" {
			t.Errorf("Foo kind = %q, want TYPE_ONLY", foo.ExportKind)
		}
		bar := assertExportExists(t, exports, "bar")
		if bar != nil && bar.ExportKind != "NAMED" {
			t.Errorf("bar kind = %q, want NAMED", bar.ExportKind)
		}
	})

	t.Run("direct interface export", func(t *testing.T) {
		exports := parseAndExtractExports(t, "typescript", `export interface Foo { name: string; }`)
		e := assertExportExists(t, exports, "Foo")
		if e != nil && e.ExportKind != "TYPE_ONLY" {
			t.Errorf("kind = %q, want TYPE_ONLY", e.ExportKind)
		}
	})

	t.Run("direct type alias export", func(t *testing.T) {
		exports := parseAndExtractExports(t, "typescript", `export type ID = string;`)
		e := assertExportExists(t, exports, "ID")
		if e != nil && e.ExportKind != "TYPE_ONLY" {
			t.Errorf("kind = %q, want TYPE_ONLY", e.ExportKind)
		}
	})
}

func TestSymbolLinking(t *testing.T) {
	src := `
function helper() { return 1; }
export { helper };
export function main() {}
`
	exports := parseAndExtractExports(t, "javascript", src)

	helper := assertExportExists(t, exports, "helper")
	if helper != nil && helper.SymbolID == "" {
		t.Error("helper export should have a linked SymbolID")
	}

	main := assertExportExists(t, exports, "main")
	if main != nil && main.SymbolID == "" {
		t.Error("main export should have a linked SymbolID")
	}

	// Verify IDs are different.
	if helper != nil && main != nil && helper.SymbolID == main.SymbolID {
		t.Error("helper and main should have different SymbolIDs")
	}
}

func TestEmptyFile(t *testing.T) {
	exports := parseAndExtractExports(t, "javascript", "")
	if exports != nil {
		t.Errorf("expected nil exports for empty file, got %d", len(exports))
	}
}

// ---------------------------------------------------------------------------
// Python tests
// ---------------------------------------------------------------------------

func TestPythonExports(t *testing.T) {
	src := `
def public_func():
    pass

def _private_func():
    pass

class PublicClass:
    pass

_private_var = 42
public_var = 1
`
	exports := parseAndExtractExports(t, "python", src)

	assertExportExists(t, exports, "public_func")
	assertExportExists(t, exports, "PublicClass")
	assertExportExists(t, exports, "public_var")
	assertNoExport(t, exports, "_private_func")
	assertNoExport(t, exports, "_private_var")
}

func TestPythonDunderAll(t *testing.T) {
	src := `
__all__ = ["foo", "bar"]

def foo():
    pass

def bar():
    pass

def baz():
    pass

def _private():
    pass
`
	exports := parseAndExtractExports(t, "python", src)

	assertExportExists(t, exports, "foo")
	assertExportExists(t, exports, "bar")
	assertNoExport(t, exports, "baz")
	assertNoExport(t, exports, "_private")

	if len(exports) != 2 {
		t.Errorf("got %d exports, want 2; have: %v", len(exports), exportNames(exports))
	}

	t.Run("tuple form", func(t *testing.T) {
		src := `
__all__ = ("foo", "bar")

def foo():
    pass

def bar():
    pass

def baz():
    pass
`
		exports := parseAndExtractExports(t, "python", src)
		assertExportExists(t, exports, "foo")
		assertExportExists(t, exports, "bar")
		assertNoExport(t, exports, "baz")
		if len(exports) != 2 {
			t.Errorf("got %d exports, want 2; have: %v", len(exports), exportNames(exports))
		}
	})

	t.Run("empty list exports nothing", func(t *testing.T) {
		src := `
__all__ = []

def public_func():
    pass
`
		exports := parseAndExtractExports(t, "python", src)
		if exports != nil {
			t.Errorf("expected nil exports for empty __all__, got %d: %v", len(exports), exportNames(exports))
		}
	})
}

// ---------------------------------------------------------------------------
// Go tests
// ---------------------------------------------------------------------------

func TestGoExports(t *testing.T) {
	src := `
package main

func Foo() {}
func bar() {}
type ExportedType struct{}
type unexportedType struct{}
`
	exports := parseAndExtractExports(t, "go", src)

	assertExportExists(t, exports, "Foo")
	assertExportExists(t, exports, "ExportedType")
	assertNoExport(t, exports, "bar")
	assertNoExport(t, exports, "unexportedType")
}

// ---------------------------------------------------------------------------
// Rust tests
// ---------------------------------------------------------------------------

func TestRustPubExports(t *testing.T) {
	src := `
pub fn public_func() {}
fn private_func() {}
pub struct PublicStruct {}
struct PrivateStruct {}
pub(crate) fn crate_visible() {}
`
	exports := parseAndExtractExports(t, "rust", src)

	assertExportExists(t, exports, "public_func")
	assertExportExists(t, exports, "PublicStruct")
	assertExportExists(t, exports, "crate_visible")
	assertNoExport(t, exports, "private_func")
	assertNoExport(t, exports, "PrivateStruct")
}

// ---------------------------------------------------------------------------
// Java tests
// ---------------------------------------------------------------------------

func TestJavaPublicExports(t *testing.T) {
	src := `
public class MyService {
    public void handleRequest() {}
    private void helper() {}
}
class PackagePrivate {}
`
	exports := parseAndExtractExports(t, "java", src)

	assertExportExists(t, exports, "MyService")
	assertNoExport(t, exports, "PackagePrivate")

	// Methods are not top-level, so not in exports.
	assertNoExport(t, exports, "handleRequest")
	assertNoExport(t, exports, "helper")
}

// ---------------------------------------------------------------------------
// C# tests
// ---------------------------------------------------------------------------

func TestCSharpPublicExports(t *testing.T) {
	src := `
public class PublicClass {}
internal class InternalClass {}
private class PrivateClass {}
class DefaultClass {}
`
	exports := parseAndExtractExports(t, "csharp", src)

	assertExportExists(t, exports, "PublicClass")
	assertNoExport(t, exports, "InternalClass")
	assertNoExport(t, exports, "PrivateClass")
	assertNoExport(t, exports, "DefaultClass")

	t.Run("public inside namespace", func(t *testing.T) {
		src := `
namespace MyApp {
    public class PublicInNs {}
    internal class InternalInNs {}
}
public class TopLevel {}
`
		exports := parseAndExtractExports(t, "csharp", src)
		assertExportExists(t, exports, "PublicInNs")
		assertExportExists(t, exports, "TopLevel")
		assertNoExport(t, exports, "InternalInNs")
	})
}

// ---------------------------------------------------------------------------
// Kotlin tests
// ---------------------------------------------------------------------------

func TestKotlinDefaultPublic(t *testing.T) {
	src := `
fun publicFunc() {}
private fun privateFunc() {}
internal fun internalFunc() {}
class PublicClass {}
private class PrivateClass {}
`
	exports := parseAndExtractExports(t, "kotlin", src)

	assertExportExists(t, exports, "publicFunc")
	assertExportExists(t, exports, "PublicClass")
	assertNoExport(t, exports, "privateFunc")
	assertNoExport(t, exports, "internalFunc")
	assertNoExport(t, exports, "PrivateClass")
}

// ---------------------------------------------------------------------------
// Swift tests
// ---------------------------------------------------------------------------

func TestSwiftAccessControl(t *testing.T) {
	src := `
public func publicFunc() {}
open class OpenClass {}
func internalFunc() {}
private func privateFunc() {}
fileprivate func filePrivateFunc() {}
`
	exports := parseAndExtractExports(t, "swift", src)

	assertExportExists(t, exports, "publicFunc")
	assertExportExists(t, exports, "OpenClass")
	assertNoExport(t, exports, "internalFunc")
	assertNoExport(t, exports, "privateFunc")
	assertNoExport(t, exports, "filePrivateFunc")
}

// ---------------------------------------------------------------------------
// Ruby tests
// ---------------------------------------------------------------------------

func TestRubyPublicDefault(t *testing.T) {
	src := `
class MyClass
  def public_method
  end

  private

  def secret_method
  end
end

def top_level_func
end
`
	exports := parseAndExtractExports(t, "ruby", src)

	assertExportExists(t, exports, "MyClass")
	assertExportExists(t, exports, "top_level_func")
	// Methods inside class are not top-level, so not exported individually.
	assertNoExport(t, exports, "public_method")
	assertNoExport(t, exports, "secret_method")
}

// ---------------------------------------------------------------------------
// C/C++ tests
// ---------------------------------------------------------------------------

func TestCppStaticNotExported(t *testing.T) {
	t.Run("C", func(t *testing.T) {
		src := `
void public_func() {}
static void file_local() {}
`
		exports := parseAndExtractExports(t, "c", src)

		assertExportExists(t, exports, "public_func")
		assertNoExport(t, exports, "file_local")
	})

	t.Run("C++", func(t *testing.T) {
		src := `
void public_func() {}
static void file_local() {}
`
		exports := parseAndExtractExports(t, "cpp", src)

		assertExportExists(t, exports, "public_func")
		assertNoExport(t, exports, "file_local")
	})
}

// ---------------------------------------------------------------------------
// PHP tests
// ---------------------------------------------------------------------------

func TestPHPPublicExports(t *testing.T) {
	src := `<?php
class MyController {
    public function handleRequest() {}
    protected function validate() {}
    private function helper() {}
}

function standalone() {}
`
	exports := parseAndExtractExports(t, "php", src)

	// Top-level symbols only.
	assertExportExists(t, exports, "MyController")
	assertExportExists(t, exports, "standalone")
	// Methods are not top-level.
	assertNoExport(t, exports, "handleRequest")
	assertNoExport(t, exports, "validate")
	assertNoExport(t, exports, "helper")
}

// ---------------------------------------------------------------------------
// Tier 2/3 tests
// ---------------------------------------------------------------------------

func TestTier2EmptyExports(t *testing.T) {
	for _, lang := range []string{"bash"} {
		t.Run(lang, func(t *testing.T) {
			var src string
			switch lang {
			case "bash":
				src = `#!/bin/bash
greet() { echo "hello"; }
`
			}
			exports := parseAndExtractExports(t, lang, src)
			if exports != nil {
				t.Errorf("%s: expected nil exports, got %d", lang, len(exports))
			}
		})
	}
}

func TestTier3EmptyExports(t *testing.T) {
	for _, lang := range []string{"html", "css"} {
		t.Run(lang, func(t *testing.T) {
			var src string
			switch lang {
			case "html":
				src = `<html><body><h1>Hello</h1></body></html>`
			case "css":
				src = `.foo { color: red; }`
			}
			exports := parseAndExtractExports(t, lang, src)
			if exports != nil {
				t.Errorf("%s: expected nil exports, got %d", lang, len(exports))
			}
		})
	}
}
