# 07 — Export, Reference, JSX & Network Extractors

## Status
Done

## Goal
Implement four Tier 1 extractors: export extraction with per-language conventions and symbol linking, reference extraction for call sites and type refs with confidence levels, JSX component usage detection for JSX/TSX files, and network call detection for HTTP client patterns across languages.

## Depends On
05-parser-engine, 06-extractors-symbols-imports

## Scope

### Export Extraction (`extractors/exports.go`)

**Main function:** `ExtractExports(root *sitter.Node, content []byte, symbols []parser.Symbol, langID string) []parser.Export`

**JS/TS export patterns (AST-based):**

| Pattern | Kind | Example |
|---|---|---|
| `export function foo()` | NAMED | Direct named export |
| `export { foo, bar }` | NAMED | Export clause |
| `export default function()` | DEFAULT | Default export |
| `export { foo as default }` | DEFAULT | Aliased default |
| `export { foo } from './mod'` | REEXPORT | Re-export |
| `export * from './mod'` | EXPORT_ALL | Barrel re-export |
| `export type { Foo }` | TYPE_ONLY | TypeScript type-only |

**Non-JS/TS languages (convention-based):** Examines the already-extracted symbols list and applies the language's export convention from the registry:
- Python: non-underscore top-level symbols; `__all__` list
- Go: uppercase first letter
- Rust: `pub` keyword
- Java/C#/PHP: `public` keyword
- Kotlin: public by default (unless `private`/`internal`)
- Swift: `public` or `open` modifier
- Ruby: public by default (unless after `private`/`protected`)
- C/C++: non-`static` file-scope symbols

**Symbol linking:** For each export, finds the corresponding local symbol and sets `linked_symbol_id`.

**Output:** `ExportKind`, `ExportedName`, `LocalName`, `SymbolID`, `SourceModule`, `Line`, `Column`.

Tier 2/3 languages return empty exports slice.

### Reference Extraction (`extractors/references.go`)

**Main function:** `ExtractReferences(root *sitter.Node, content []byte, symbols []parser.Symbol, imports []parser.Import, langID string) []parser.Reference`

**Reference kinds:** CALL, JSX_RENDER, TYPE_REF, VALUE_REF, EXTENDS, IMPLEMENTS, NEW_EXPR, HOOK_USE, FETCH, DECORATOR, EXPORTS.

**Per-language AST patterns:**
- Call expressions: `call_expression` (JS/TS/Go/Rust/Kotlin), `call` (Python/Ruby), `method_invocation` (Java), `invocation_expression` (C#)
- Type references: `type_identifier`, `generic_type` -- skip per-language builtin types from registry
- Inheritance: `extends_clause`, `implements_clause`, `superclass`, `base_class_clause`
- Constructor: `new_expression`, `object_creation_expression`
- Language-specific: HOOK_USE (JS/TS/JSX/TSX only), JSX_RENDER (JSX/TSX only), DECORATOR (Python/Java/TS)

**Resolution scope:** LOCAL (matches local symbol), IMPORTED (matches import), MEMBER (contains "."), GLOBAL (known globals), UNKNOWN.

**Confidence levels:** HIGH, MEDIUM, LOW -- based on resolution scope and match quality.

**Deduplication:** By `"{kind}:{symbol_name}:{line}:{column}"`, emit in document order.

Tier 2/3 languages return empty slice.

### JSX Usage Extraction (`extractors/jsx.go`)

**Main function:** `ExtractJsxUsages(root *sitter.Node, content []byte, symbols []parser.Symbol) []parser.JsxUsage`

Only runs for JSX and TSX language IDs. All other languages return empty slice.

**Detection:** `jsx_self_closing_element`, `jsx_opening_element`, `jsx_fragment`.

**Classification:**
- Lowercase first char = intrinsic HTML element (`IsIntrinsic: true`)
- Uppercase first char = custom component
- Fragment: `Fragment`, `React.Fragment`, or `<>` syntax (`IsFragment: true`)

**Member expression names:** Extracted for compound components like `Modal.Header`.

**Symbol resolution:** Links to local component definitions via `ResolvedTargetSymbolID`.

### Network Call Detection (`extractors/network.go`)

**Main function:** `ExtractNetworkCalls(root *sitter.Node, content []byte, symbols []parser.Symbol, langID string) []parser.NetworkCall`

**JS/TS patterns:** `fetch()`, `axios.get/post/...`, `ky.get/post/...`, GraphQL (`gql`, `useQuery`, `useMutation`), instance patterns (`api.get`, `client.post`).

**Per-language HTTP client patterns:**
- Python: `requests`, `httpx`, `urllib`, `aiohttp`
- Go: `http.Get`, `http.Post`, `http.NewRequest`, `client.Do`
- Rust: `reqwest`, `hyper`
- Java: `HttpClient`, `OkHttp`, `RestTemplate`, `WebClient`
- C#: `HttpClient.GetAsync/PostAsync`
- Swift: `URLSession`, `Alamofire`
- Ruby: `Net::HTTP`, `Faraday`, `HTTParty`
- PHP: `curl`, `Guzzle`

**URL extraction:** String literal -> literal URL, template literal -> template with `{expr}`, non-literal -> `"<dynamic>"`.

**Output:** `ClientKind` (FETCH, AXIOS, KY, GRAPHQL, UNKNOWN), `Method` (GET, POST, etc.), `URLLiteral`, `URLTemplate`, `IsRelative`, confidence level.

Tier 2/3 languages return empty slice.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/parser/extractors/exports.go` | Export extraction with per-language conventions |
| `internal/parser/extractors/references.go` | Reference detection (calls, types, inheritance) |
| `internal/parser/extractors/jsx.go` | JSX/TSX component usage detection |
| `internal/parser/extractors/network.go` | HTTP client call detection |

## Acceptance Criteria
- [x] JS/TS: all export kinds detected (NAMED, DEFAULT, REEXPORT, EXPORT_ALL, TYPE_ONLY)
- [x] `source_module` set for re-exports; `linked_symbol_id` set for local exports
- [x] Python `_` prefix symbols not exported; Go uppercase detected; Rust `pub` detected
- [x] Java/C#/PHP `public` keyword detected; Kotlin all public unless private/internal
- [x] C/C++ non-`static` symbols detected as exported
- [x] Tier 2/3 return empty exports slice
- [x] Detects all reference kinds for JS/TS; call expressions across all Tier 1 languages
- [x] Type references detected (skip per-language builtins)
- [x] HOOK_USE only for JS/TS/JSX/TSX; DECORATOR for Python/Java/TS
- [x] Resolution scope classified correctly; confidence levels assigned
- [x] JSX: detects self-closing, opening elements, and fragments
- [x] JSX: intrinsic vs custom classification correct; member expression names extracted
- [x] JSX: only runs for JSX/TSX language IDs
- [x] Network: detects fetch, axios, ky, GraphQL patterns (JS/TS)
- [x] Network: detects per-language HTTP clients (Python requests, Go http, etc.)
- [x] Network: URL extracted correctly from literals, templates, dynamics
- [x] Deterministic sorting: line -> column -> name/kind
