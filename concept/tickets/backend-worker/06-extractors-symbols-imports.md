# 06 — Symbol & Import Extraction + Cross-Language Resolution

## Status
Done

## Goal
Implement symbol extraction across all 28 languages (functions, classes, methods, interfaces, types, enums, variables, namespaces) with qualified names, doc comments, and v2 flags. Implement import extraction with STDLIB/INTERNAL/EXTERNAL classification and per-language `resolvePath()`. Add cross-language import resolution (resolve.go) for Python, Rust, Go, Java, Kotlin, C#, and PHP with post-hoc reclassification.

## Depends On
05-parser-engine

## Scope

### Symbol Extraction (`extractors/symbols.go`)

**Main function:** `ExtractSymbols(root *sitter.Node, content []byte, langID string) []parser.Symbol`

Queries the language registry for `SymbolNodeTypes` and `DocCommentStyle`.

**Symbol kinds:** `function`, `class`, `method`, `interface`, `type_alias`, `enum`, `variable`, `namespace`. Each kind maps from language-specific AST node types via the registry.

**Per-language AST node type mappings (examples):**
- JS/TS: `function_declaration`, `class_declaration`, `method_definition`, `interface_declaration`, `type_alias_declaration`, `enum_declaration`
- Python: `function_definition`, `class_definition` (methods are nested `function_definition` in class body)
- Go: `function_declaration`, `method_declaration`, `type_spec` (struct/interface/alias)
- Rust: `function_item`, `struct_item`, `trait_item`, `enum_item`, `mod_item`
- Java: `method_declaration`, `class_declaration`, `interface_declaration`, `enum_declaration`
- C/C++: `function_definition`, `struct_specifier`, `class_specifier`, `enum_specifier`
- HCL: `block` nodes (resource, data, variable, output, module, locals, provider, terraform)

**Per-symbol extraction:**
1. `name` -- identifier node text
2. `kind` -- mapped from AST node type via registry
3. `signature` -- first line of declaration up to `{` or `=>`
4. `start_line` / `end_line` -- 1-indexed (convert from 0-indexed tree-sitter)
5. `doc_text` -- language-specific doc comment extraction
6. `symbol_hash` -- `sha256(content[node.StartByte():node.EndByte()])`
7. `parent_symbol_id` -- walk up to find enclosing class
8. `qualified_name` -- `ClassName.methodName` for methods

**Doc comment styles:** JSDoc (`/** */`), Python docstrings (`"""`), `///` (Rust, Swift), XML doc (C#), `#` (Ruby), Doxygen (`/** */` for C/C++).

**v2 Flags (SymbolFlags):**
- `is_exported`: per-language `ExportStrategy` (keyword for JS/TS/Rust/Java, convention for Go/Python, all_public for Kotlin/Ruby)
- `is_async`, `is_generator`, `is_static`, `is_abstract`, `is_readonly` (language-dependent)
- `is_arrow_function`: JS/TS only
- `is_react_component_like`: JSX/TSX only (uppercase name + JSX in body)
- `is_hook_like`: JS/TS/JSX/TSX only (`use[A-Z]` pattern)

**Traversal strategy:** Recursive depth-first walker (`walkNode`). Check each node against `SymbolNodeTypes` map. Recurse into class bodies for methods. Skip nodes inside function bodies. Unwrap export nodes. Handle export clause reconciliation.

**Tier 2 symbols:** Bash (function_definition), SQL (CREATE TABLE/FUNCTION/VIEW), GraphQL (type/interface/enum), Dockerfile (FROM/ARG/ENV), HCL (block nodes with labels).

**Tier 3 symbols:** CSS/SCSS (selectors, @rules), HTML (script/style blocks), JSON/YAML/TOML (top-level keys), Markdown (headings), XML (root element).

### Import Extraction (`extractors/imports.go`)

**Main function:** `ExtractImports(root *sitter.Node, content []byte, filePath string, langID string) []parser.Import`

**Per-language import detection:** ES6 imports, CommonJS `require()`, dynamic `import()`, Python `import`/`from`, Go `import`, Rust `use`, Java/Kotlin `import`, C/C++ `#include`, C# `using`, Swift `import`, Ruby `require`/`require_relative`, PHP `use`, Bash `source`/`.`, HCL module `source` attribute.

**Classification logic:**
- `STDLIB`: per-language stdlib modules (exact match) and prefixes (`node:`, `std::`, `java.`, `System.`, etc.)
- `INTERNAL`: per-language patterns (`./`, `../` for JS/TS; `.` for Python relative; `crate::`, `super::`, `self::` for Rust; `"..."` for C/C++)
- `EXTERNAL`: everything else

**Path resolution (`resolvePath()`):**
- JS/TS/Ruby/Bash/C++: `path.Join(dir, source)` for relative imports
- Python: dot-prefix converted to directory traversal (`.foo` -> sibling package, `..foo` -> parent)
- Rust: `crate::` -> `src/` prefix, `super::` -> parent directory, `self::` -> current directory
- Go: strip `go.mod` module path prefix when available
- Java/Kotlin: dots -> slashes (`com.myapp.Service` -> `com/myapp/Service`)

**Deduplication:** One Import per unique source value. First occurrence wins. Emit in document order.

### Cross-Language Resolution (`indexing/resolve.go`)

**`ResolveImportTargets`** runs as a post-processing step in the indexing pipeline, after parsing and before artifact persistence.

Builds lookup maps from all project file paths:
1. **Stem map:** extensionless path to full file path (`.ts` implementation files overwrite `.d.ts` declaration files)
2. **Directory index map:** directory path to index file (`index.ts`, `__init__.py`, `mod.rs`)
3. **Exact match set:** full file paths for fast lookups
4. **Go directory map:** directory path to first `.go` file

Resolution order for each `INTERNAL` import: exact match -> stem match -> directory index.

**Post-hoc reclassification:** After stem-based resolution, a second pass reclassifies `EXTERNAL` imports as `INTERNAL` when the import specifier converts to a project-relative path that matches a known file:
- Go: strip `go.mod` module path prefix, match against project directories
- Java/Kotlin: replace dots with slashes
- C#: replace dots with slashes
- PHP: trim leading `\`, replace `\` with `/`
- Python: replace dots with slashes (absolute imports)

**Go module detection:** The `go.mod` module path is extracted from the project file list during indexing and passed through the pipeline as `GoModulePath`.

**Incremental indexing:** The `AllFilePaths` field includes both changed and unchanged file paths, allowing resolution of targets pointing to unchanged files.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/parser/extractors/symbols.go` | Symbol extraction, walkNode, doc extraction, flag computation |
| `internal/parser/extractors/imports.go` | Import detection per language, resolvePath(), classification |
| `internal/parser/extractors/utils.go` | Shared utilities (findEnclosingSymbol, buildQualifiedName) |
| `internal/indexing/resolve.go` | ResolveImportTargets, stem/dir maps, post-hoc reclassification |
| `internal/indexing/pipeline.go` | GoModulePath detection and propagation |

## Acceptance Criteria
- [x] Extracts all symbol kinds from JS/TS/JSX/TSX files
- [x] Extracts symbols from all 15 Tier 1 languages using per-language node type mappings
- [x] Extracts major symbols from 5 Tier 2 languages (including HCL/Terraform)
- [x] Extracts minimal symbols from 8 Tier 3 languages
- [x] Symbols ordered by start_line -> start_column -> name
- [x] Doc comments associated using language-specific doc style
- [x] `parent_symbol_id` set for class methods; `qualified_name` as `ClassName.methodName`
- [x] v2 flags computed with language-aware logic (is_exported, is_async, etc.)
- [x] Arrow functions assigned to `const` extracted as functions (JS/TS)
- [x] Detects all ES6 import forms, CommonJS `require()`, dynamic `import()` for JS/TS
- [x] Per-language import detection correct for all 15 Tier 1 + 5 Tier 2 languages
- [x] STDLIB/INTERNAL/EXTERNAL classification correct per language
- [x] Deduplicates imports by source specifier in document order
- [x] Python relative imports resolved to file paths
- [x] Rust crate/super/self imports resolved to file paths
- [x] Go module-aware resolution (imports matching go.mod module path classified as INTERNAL)
- [x] Java/Kotlin/C#/PHP imports reclassified as INTERNAL when matching project files
- [x] No regressions on JS/TS/Ruby/C/C++/Bash resolution
- [x] Hashes are deterministic
- [x] Empty file returns empty symbol/import slices
