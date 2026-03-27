# Changelog

All notable changes to the backend-worker service will be documented in this file.

## [Unreleased]

### Added (Task 14 — Stuck-Job Reaper)

- **Reaper core** (`internal/reaper/reaper.go`): New `Reaper` component with
  `RunOnce()` method that scans for stuck `running` jobs whose worker heartbeat
  has expired, transitions them to `failed` with `worker_crash` error category,
  and publishes `job:failed` SSE events. All operations are idempotent.
- **Orphaned snapshot cleanup**: Building snapshots with no associated running job
  are automatically deleted during the reap cycle (cascading to files, symbols,
  chunks, and dependencies).
- **Legacy job support**: Jobs without a `worker_id` (pre-tracking) are handled via
  Redis SCAN of all `worker:status:*` keys.
- **`IsWorkerAlive()` exported**: Standalone helper for single-job liveness checks,
  available for the API lazy reaper (`backend-api/17-lazy-job-reaper`).
- **Startup sweep** (`internal/app/app.go`): Worker runs a synchronous
  `Reaper.RunOnce()` on boot before accepting jobs, providing instant recovery
  after a crash.
- **Configuration**: `REAPER_STALE_THRESHOLD` env var (duration, default `5m`)
  controls the minimum age before a running job is considered stale.
- **SQL queries** (`datastore/postgres/queries/indexing.sql`):
  `ListStaleRunningJobs` and `ListOrphanedBuildingSnapshots` added.

### Added (Task 29 — Cross-Language Import Path Resolution)

- **Python relative import resolution** (`extractors/imports.go`): `resolvePath()`
  now converts `.utils`, `..models`, `.models.user` to file-system paths relative
  to the importing file. Dot count maps to directory traversal; dotted remainders
  become path segments. `ResolveImportTargets` then resolves `.py` / `__init__.py`
  via the existing stem and directory-index maps.
- **Rust crate/super/self resolution** (`extractors/imports.go`): `resolvePath()`
  converts `crate::handlers::auth` → `src/handlers/auth`, `super::models` →
  parent directory, `self::child` → current directory. Resolution picks up `.rs`
  and `mod.rs` via stem/dirIndex.
- **Post-hoc EXTERNAL→INTERNAL reclassification** (`indexing/resolve.go`): New
  `convertImportToPath` converts import specifiers to candidate file paths for
  Go, Java, Kotlin, C#, PHP, and Python absolute imports. If a candidate matches
  an existing project file (exact, stem, or directory-index), the import is
  reclassified as INTERNAL with the resolved `TargetFilePath`.
- **Go module-aware resolution** (`indexing/pipeline.go`): Extracts module path
  from `go.mod` content via `ExtractGoModulePath()` and passes it to
  `ResolveImportTargets`. Go imports matching the module prefix are stripped to
  relative paths and resolved against the project file set.

### Changed (Task 28 — Dockerfile & Deployment)

- **Dockerfile rewritten** (`backend-worker/Dockerfile`): Replaced 4-stage
  multi-process image (Go builder + Node.js sidecar deps/build + Node.js runtime)
  with a 2-stage single-binary image (Go builder + `alpine:3.21` runtime).
  `CGO_ENABLED=1` with static linking retained. Runtime contains only the Go
  binary, `tini`, `git`, and `openssh-client`. No Node.js, `node_modules`, or
  sidecar dist. Expected image size ~60-80 MB (down from ~200+ MB).
- **docker-compose.yaml**: Replaced sidecar environment variables
  (`PARSER_GRPC_ADDR`, `SIDECAR_PORT`, `SIDECAR_HEALTH_PORT`,
  `SIDECAR_LOG_LEVEL`, `SIDECAR_POOL_SIZE`) with inline parser config
  (`PARSER_POOL_SIZE`, `PARSER_TIMEOUT_PER_FILE`, `PARSER_MAX_FILE_SIZE`).
- **Makefile**: Removed `proto-gen` target (protobuf bindings deleted in Task 27).

### Removed (Task 28 — Dockerfile & Deployment)

- **Sidecar entrypoint** (`backend-worker/docker/start-with-sidecar.sh`): Deleted.
  Multi-process supervisor script no longer needed — binary runs directly via
  `tini`.
- **Sidecar Dockerfile stages**: Removed `sidecar-deps` and `sidecar-build` stages
  and all `node:22-alpine` runtime infrastructure.

### Changed (Task 27 — Config, DI & gRPC Removal)

- **ParserConfig updated** (`internal/config/config.go`): Replaced `GRPCAddr string`
  with `PoolSize int`, `TimeoutPerFile time.Duration`, and `MaxFileSize int64`.
  Existing `Timeout` field kept as overall batch timeout.
- **New environment variables**: `PARSER_POOL_SIZE` (int, default: 0 = NumCPU),
  `PARSER_TIMEOUT_PER_FILE` (duration, default: 30s), `PARSER_MAX_FILE_SIZE`
  (int64, default: 10485760 = 10 MB). `PARSER_TIMEOUT` unchanged.
- **DI wiring** (`internal/app/app.go`): `App.Parser` changed from
  `*parser.Client` to `*engine.Engine`. Handlers use the `fileParser` interface
  and required zero changes.
- **Config validation**: Added `PoolSize >= 0`, `TimeoutPerFile > 0`,
  `MaxFileSize > 0` checks. `LoadForTest()` uses `PoolSize: 2`,
  `TimeoutPerFile: 5s`, `MaxFileSize: 10 MB`.
- **Config log summary**: Parser group now logs `pool_size`, `timeout`,
  `timeout_per_file`, `max_file_size` instead of `grpc_addr`.
- **Helpers**: Added `parseInt64()` for parsing `PARSER_MAX_FILE_SIZE`.

### Removed (Task 27 — Config, DI & gRPC Removal)

- **gRPC client** (`internal/parser/client.go`, `client_test.go`): Deleted.
  Replaced by `engine.Engine` (Task 25).
- **Protobuf bindings** (`internal/parser/pb/`): Entire directory deleted.
- **Proto mapping functions** (`internal/parser/domain.go`): Removed
  `MapParsedFile`, `MapParsedFiles`, and all `map*` helpers (~280 lines).
  Domain types retained.
- **Proto mapping tests** (`internal/parser/domain_test.go`): Deleted (tested
  removed mapping functions).
- **Config defaults**: Removed `DefaultParserGRPCAddr`.
- **Environment variables**: Removed `PARSER_GRPC_ADDR` (and sidecar-related
  `SIDECAR_PORT`, `SIDECAR_HEALTH_PORT`, `SIDECAR_LOG_LEVEL`, `SIDECAR_POOL_SIZE`
  from config scope).
- **go.mod dependencies**: `google.golang.org/grpc`,
  `google.golang.org/genproto/googleapis/rpc`, `golang.org/x/net` removed via
  `go mod tidy`.

### Added (Task 26 — Parser Integration & E2E Testing)

- **Golden file test harness** (`internal/parser/golden_test.go`): Discovers all
  fixture files in `testdata/parser/fixtures/`, parses each through the engine,
  serializes to deterministic JSON, and compares against committed golden
  snapshots. `UPDATE_GOLDEN=1` regenerates golden files. Shared helpers
  (`newTestEngine`, `testdataDir`, `discoverFixtures`, `loadFixture`,
  `marshalDeterministic`, `goldenPathFor`) used by all integration test files.
- **Cross-extractor consistency tests** (`internal/parser/consistency_test.go`):
  Validates invariants across extractors — Export/Chunk/Reference/JsxUsage/
  NetworkCall SymbolID references resolve to valid symbols, chunk boundaries
  don't overlap, reference positions fall within file bounds, file metadata
  (hash/lineCount/sizeBytes) matches recomputed values, ExtractorStatuses
  correct per tier, and FileFacts fields match actual extractor output. Covers
  Tier 1 (tsx), Tier 2 (bash), and Tier 3 (json).
- **Multi-language batch tests** (`internal/parser/integration_test.go`):
  `TestBatch_MixedTiers` (10 files across all tiers, order preserved, tier-
  appropriate outputs), `TestBatch_SingleFile`, `TestBatch_EmptySlice`,
  `TestBatch_LargeBatch` (50 copies).
- **Edge case tests** (`internal/parser/integration_test.go`): 10 tests covering
  empty file, UTF-8 BOM, CRLF line endings, syntax errors, very long lines,
  deep nesting (DEEP_NESTING diagnostic), unicode identifiers, binary content,
  unsupported extension, and oversized file — all verified no panic.
- **Determinism tests** (`internal/parser/determinism_test.go`): 10 sequential
  runs and 5 concurrent independent engines produce byte-identical JSON output
  for the same input batch.
- **Benchmark tests** (`internal/parser/benchmark_test.go`): Single-file
  TypeScript (~2.2ms/op), 10-file mixed-language batch (~1.1ms/op), and 50-file
  TypeScript pool contention (~24.6ms/op) benchmarks with alloc reporting.
- **Golden JSON snapshots** (`testdata/parser/golden/`): 10 committed golden
  files for all fixture languages (typescript, python, go, rust, java, jsx,
  bash, hcl, json, yaml). CI golden comparison catches extractor regressions.
- **Makefile target**: `make integration-test` runs all integration tests with
  `CGO_ENABLED=1 go test -tags integration -v -timeout 120s ./internal/parser/...`
- All test files use `//go:build integration` tag and `package parser_test`
  (external test package to avoid circular imports with engine sub-package).

### Added (Task 25 — Parser Engine Pipeline Wiring)

- **Parser engine** (`internal/parser/engine/engine.go`): `engine.New(cfg)`
  creates a local `Engine` that replaces the gRPC `parser.Client`. Satisfies the
  `fileParser` interface used by `fullindex.Handler` and `incremental.Handler`
  with zero handler changes required.
- **Single-file pipeline** (`runPipeline`): 9-step extraction pipeline —
  normalize content, check empty/size, detect language, compute file metadata
  (SHA-256, line count, byte size), tree-sitter parse with per-file timeout, run
  extractors based on tier, compute file facts, set parser metadata.
- **Batch pipeline** (`ParseFilesBatched`): Concurrent file processing using
  `errgroup` bounded by pool size. Results maintain input order via indexed
  assignment. Partial-failure semantics — one file's error doesn't affect others.
- **Tier-based extractor routing**: Tier 1 (all extractors: symbols, imports,
  exports, references, network calls, chunks, diagnostics), Tier 2 (symbols,
  imports, chunks, diagnostics), Tier 3 (symbols, chunks, diagnostics). JSX
  usages enabled only for `jsx`/`tsx` language IDs.
- **Generic panic recovery** (`runExtractor[T]`): Each extractor wrapped in a
  closure with `defer recover()`. Panics produce `EXTRACTION_ERROR` issues and
  `FAILED` status without crashing the pipeline.
- **File facts computation** (`computeFileFacts`): Derives `FileFacts` from
  extracted data — `HasJsx`, `HasDefaultExport`, `HasNamedExports`,
  `HasTopLevelSideEffects` (CSS/SCSS/LESS/SASS import heuristic),
  `HasReactHookCalls` (`HOOK_USE` references), `HasFetchCalls` (network calls),
  `HasClassDeclarations`, `HasTests` (file path pattern matching),
  `HasConfigPatterns`, `JsxRuntime` (react/preact/unknown detection;
  "preact" matched before "react" substring check to avoid misclassification).
- **Pool.Size() getter** (`internal/parser/pool.go`): Added to expose pool size
  to the `engine` sub-package (Engine lives in `parser/engine` to break the
  `parser → extractors → parser` import cycle).
- **Promoted dependency**: `golang.org/x/sync` from indirect to direct (errgroup
  used for bounded concurrency).
- **Unit tests** (`engine_test.go`): 34 test cases covering constructor defaults
  and custom config, TypeScript/Python/Go/Bash/JSON language pipelines, batch
  ordering (5 files), empty slice, empty file, oversized file (OVERSIZED_FILE
  issue), unsupported extension (UNSUPPORTED_LANGUAGE issue), partial failure
  (3 files, middle unsupported), preset language override, CRLF normalization,
  FileFacts (JSX/non-JSX/test/config), parser metadata and enabled extractors,
  extractor statuses, panic recovery (FAILED status), Close/Close-nil, determinism
  (100 iterations byte-identical JSON), context cancellation, and 7 unit tests
  for individual FileFacts helpers.

### Added (Task 24 — Diagnostics & File Metadata)

- **Diagnostics extraction engine** (`internal/parser/extractors/diagnostics.go`):
  `ExtractDiagnostics(root, content, langID, filePath)` collects parse errors,
  structural warnings, and file-level issues from tree-sitter ASTs. Supports all
  28 registered languages across Tier 1/2/3.
- **Parse error collection**: Walks AST to find ERROR (`node.IsError()`) and
  MISSING (`node.IsMissing()`) nodes. Consecutive same-line errors merged into
  single "Multiple parse errors" issue. Capped at 50 issues per file.
- **Structural warnings**: `LONG_FILE` (>1000 lines), `LONG_FUNCTION` (>200
  lines with function name in message), `DEEP_NESTING` (>6 levels using
  per-language `NestingNodeTypes` from registry), `NO_EXPORTS` (JS/TS files
  with no export statements, excluding test files).
- **Issue factory functions**: `CreateUnsupportedLanguageIssue`,
  `CreateOversizedFileIssue`, `CreateParseTimeoutIssue`,
  `CreateExtractionErrorIssue` — called by the pipeline engine (Task 25).
- **File metadata extractor** (`internal/parser/extractors/file_meta.go`):
  `ExtractFileMeta(content)` computes SHA-256 hash, line count, and byte size
  of normalized content.
- **Deterministic sorting** via `parser.SortIssues`: line → column → code.
- **Unit tests** (`diagnostics_test.go`): 31 test cases covering nil root,
  empty file, parse errors (JS/Python/Go/Rust/Java), error merging and capping,
  long file/function warnings, deep nesting (JS/Python/Go), normal nesting
  (no false positive), NO_EXPORTS for JS (with/without exports), NO_EXPORTS
  exclusions (Python, Go, test files, Tier 2/3), deterministic sort, and all 4
  factory functions.
- **Unit tests** (`file_meta_test.go`): 6 test cases covering normal content,
  empty content, single line, trailing newline, deterministic hash, and
  Unicode byte size.

### Added (Task 23 — Chunking Strategy)

- **Chunk extraction engine** (`internal/parser/extractors/chunks.go`):
  `ExtractChunks(content, filePath, symbols, imports, langID)` generates
  embedding-ready `[]parser.Chunk` from pre-extracted symbols and text content.
  Supports all 28 registered languages across Tier 1/2/3.
- **Five chunk types**: MODULE_CONTEXT (imports and top-level declarations
  before first symbol), FUNCTION (one per top-level function with doc comment
  extension), CLASS (full text for ≤200 lines, declaration + method signatures
  summary for larger), CONFIG (entire file for per-language config file
  patterns), TEST (per test-block or full-file fallback using per-language test
  block regex patterns).
- **Per-language file pattern matching**: `matchesFilePatterns` supports simple
  globs (`*.test.*`, `*_test.go`, `go.mod`) and recursive directory patterns
  (`**/__tests__/**`, `**/tests/**/*.py`) against `ConfigFilePatterns` and
  `TestFilePatterns` from the language registry.
- **Test block splitting**: `splitTestBlocks` compiles `TestBlockPatterns`
  regexes to split test files into individual test chunks (Go `func Test*`,
  Python `def test_`, Java `@Test`, JS `describe`/`test`/`it`, etc.).
- **Precedence rules**: Config > Tier 3 full-file > Test > Symbol-based
  chunking. Tier 3 config files (e.g. `package.json`, `tsconfig.json`) emit
  CONFIG, not MODULE_CONTEXT, preventing duplicate full-file chunks.
- **No-overlap enforcement**: Methods inside classes are part of the CLASS
  chunk only; functions whose line range falls within a class are excluded from
  FUNCTION chunks. Doc-comment-aware MODULE_CONTEXT boundary prevents overlap
  with function doc comments.
- **Semantic role assignment**: config, test, ui_component (ReactComponentLike),
  hook (HookLike), api_surface (exported + Tier 1), or implementation.
- **v2 metadata**: OwnerQualifiedName, OwnerKind, IsExportedContext,
  ContextBefore (`File: path > kind name`), deterministic ChunkID
  (`filePath:type:start-end`), SHA-256 ChunkHash, token estimation
  (`(len+3)/4`).
- **Deterministic ordering** via `parser.SortChunks`: MODULE_CONTEXT → CLASS →
  FUNCTION → CONFIG/TEST, then by StartLine within each type.
- **Large class summarisation**: Classes exceeding 200 lines are condensed to
  declaration line + method signatures + closing brace.
- **Unit tests** (`chunks_test.go`): 42 test cases covering early returns,
  config file detection (10 languages), Tier 3 full-file and config override,
  test file splitting (Go/JS/Python/Rust/Java/C#), module context boundary,
  function chunks with doc comment extension, small and large class chunks,
  no-overlap enforcement, sort order, chunk ID format, hash determinism, token
  estimation, semantic roles (api_surface/ui_component/hook), context metadata,
  and `matchesFilePatterns` with 17 pattern-matching scenarios.

### Added (Task 22 — Reference, JSX & Network Extractors)

- **Reference extraction engine** (`internal/parser/extractors/references.go`):
  `ExtractReferences(root, content, symbols, imports, langID)` extracts
  `[]parser.Reference` for all cross-symbol references. Detects 9 reference
  kinds: CALL, TYPE_REF, EXTENDS, IMPLEMENTS, NEW_EXPR, HOOK_USE, JSX_RENDER,
  FETCH, and DECORATOR. Per-language AST pattern matching across all 15 Tier 1
  languages.
- **Per-language call expression detection**: JS/TS/Go/Rust/Kotlin/C/C++/Swift
  (`call_expression`), Python/Ruby (`call`, `method_call`), Java
  (`method_invocation`), C# (`invocation_expression`), PHP
  (`function_call_expression`, `method_call_expression`), Rust
  (`macro_invocation`).
- **Type reference detection** with per-language builtin type filtering from
  `registry.BuiltinTypes`. Skips type identifiers inside import/export
  statements and declaration names.
- **Inheritance detection** across all Tier 1 languages: JS/TS
  (`extends_clause`, `implements_clause`), Python (class bases via
  `argument_list` in `class_definition`), Java (`superclass`,
  `super_interfaces`), Ruby (`superclass`), C++ (`base_class_clause`), C#
  (`base_list`), Swift (`inheritance_clause`), Rust (`trait_bound`), PHP
  (`base_clause`, `class_interfaces`), Kotlin (`delegation_specifier`).
- **Constructor/new expression detection**: JS/TS/C++ (`new_expression`),
  Java/C#/PHP (`object_creation_expression`), Rust (`struct_expression`), Go
  (`composite_literal`).
- **React hook detection**: `use[A-Z]` pattern for JS/TS/JSX/TSX only
  (HOOK_USE kind).
- **JSX render references**: `jsx_self_closing_element` and
  `jsx_opening_element` for JSX/TSX only (JSX_RENDER kind).
- **Decorator detection**: Python (`decorator`), Java/Kotlin (`annotation`,
  `marker_annotation`), JS/TS (`decorator`).
- **Resolution scope classification**: LOCAL (matches local symbol), IMPORTED
  (matches import specifier via AST-enriched import index), MEMBER (qualified
  member access), GLOBAL (known JS/Python/Go globals), UNKNOWN.
- **Import index enrichment**: For JS/TS, parses import statement AST to
  capture individual named import specifiers (e.g., `readFile` from
  `import { readFile } from "fs"`).
- **Enclosing symbol lookup**: `findEnclosingSymbolID(symbols, line)` finds
  the tightest-range symbol containing a reference line. Shared across all
  three extractors.
- **JSX usage extraction engine** (`internal/parser/extractors/jsx.go`):
  `ExtractJsxUsages(root, content, symbols, langID)` extracts
  `[]parser.JsxUsage` for JSX/TSX only. Detects `jsx_self_closing_element`,
  `jsx_opening_element`, and fragment syntax (`<>...</>`). Classifies elements
  as intrinsic (lowercase) vs custom (uppercase), detects member expression
  component names (`Modal.Header`), resolves to local symbol definitions.
- **Network call detection engine** (`internal/parser/extractors/network.go`):
  `ExtractNetworkCalls(root, content, symbols, langID)` extracts
  `[]parser.NetworkCall` for all Tier 1 languages.
- **JS/TS HTTP client detection**: fetch (FETCH), axios (AXIOS), ky (KY),
  GraphQL (GRAPHQL via `gql`/`useQuery`/`useMutation`/`useSubscription`/
  `useLazyQuery`), and generic instance patterns (`api.get`, `client.post`).
- **Per-language HTTP client detection**: Python (requests, httpx, urllib,
  aiohttp), Go (http.Get/Post/NewRequest), Rust (reqwest, hyper), Java
  (HttpClient, OkHttp, RestTemplate, WebClient), Kotlin (same + ktor), C#
  (HttpClient.GetAsync/PostAsync), Swift (URLSession, Alamofire), Ruby
  (Net::HTTP, Faraday, HTTParty), PHP (curl, Guzzle, file_get_contents),
  C/C++ (curl).
- **URL extraction**: String literals extracted as `URLLiteral`, template
  literals converted to `URLTemplate` with `{expr}` substitutions, non-literal
  arguments produce `"<dynamic>"`. Relative URL detection via prefix analysis.
- **HTTP method inference**: Method name mapping (get→GET, post→POST, etc.)
  with special handling for Go cased variants, C# async variants, and Java
  RestTemplate methods. Fetch options object `method` property parsing.
- **Deterministic IDs**: `ref_<hash16>`, `jsx_<hash16>`, `net_<hash16>`
  format using SHA-256 of structured key strings.
- **Tier 2/3 filtering**: All three extractors return nil for languages below
  Tier 1.
- **Unit tests** (`references_test.go`, `jsx_test.go`, `network_test.go`):
  Comprehensive coverage including JS/TS call/hook/type/extends/implements/
  new/decorator/fetch references, multi-language smoke tests (Python, Go, Rust,
  Java, C#, Swift, Ruby, PHP, Kotlin, C, C++), resolution scope verification,
  JSX intrinsic/custom/fragment/member detection, symbol resolution, network
  fetch/axios/ky/GraphQL/instance patterns, per-language HTTP client detection,
  URL extraction variants, tier filtering, and edge cases.

### Added (Task 21 — Export Extraction across 15 Tier 1 Languages)

- **Export extraction engine** (`internal/parser/extractors/exports.go`):
  `ExtractExports(root, content, langID, symbols)` extracts `[]parser.Export`
  for all file-level export declarations. Two extraction paths: AST-based for
  JS/TS (walking `export_statement` nodes) and convention-based for all other
  Tier 1 languages (deriving exports from the symbols list using each
  language's `ExportStrategy`).
- **JS/TS AST-based extraction**: Handles all 7 export kinds — direct named
  exports (`export function foo()`), export clauses (`export { foo, bar }`),
  default exports (`export default class`), aliased defaults
  (`export { foo as default }`), re-exports (`export { foo } from './mod'`),
  barrel re-exports (`export * from './mod'`, `export * as ns from './mod'`),
  and TypeScript type-only exports (`export type { Foo }`). Each export is
  classified with the correct `ExportKind` (NAMED, DEFAULT, REEXPORT,
  EXPORT_ALL, TYPE_ONLY), with `SourceModule` set for re-exports and
  `SymbolID` linked to local symbols.
- **Convention-based extraction** for non-JS/TS Tier 1 languages: Python
  (underscore prefix filtering with `__all__` list override), Go (uppercase
  first letter convention via `Flags.IsExported`), Rust (`pub` keyword via
  `Flags.IsExported`), Java (`public` keyword), Swift (`public`/`open`
  modifiers), Kotlin (public by default, filtering out `private`/`internal`
  from `Modifiers`), Ruby (public by default with AST walk to detect
  `private`/`protected` visibility boundaries and wrapping patterns).
- **C/C++ static detection** via AST walk for `storage_class_specifier`
  containing `static`, compensating for `Flags.IsStatic` not being set
  reliably by the symbol extractor for C-family languages.
- **C# public detection** via AST walk for `modifier` nodes, compensating for
  the symbol extractor's `hasExportKeyword` expecting `accessibility_modifier`
  instead of the `modifier` node that tree-sitter-c-sharp produces.
- **PHP top-level export handling**: PHP top-level classes and functions are
  inherently accessible (no `public` keyword needed at file level), so all
  top-level symbols are exported.
- **Deterministic export IDs**: `exp_<hash16>` format using SHA-256 of
  `kind:name:line:column`, consistent with the `sym_` prefix pattern for
  symbols.
- **Tier 2/3 filtering**: Languages below Tier 1 (Bash, SQL, GraphQL,
  Dockerfile, HCL, HTML, CSS, SCSS, JSON, YAML, TOML, Markdown, XML)
  return nil — no export concept applies.
- **Unit tests** (`internal/parser/extractors/exports_test.go`): 23 test
  functions covering JS/TS (direct named, export clause, default, aliased
  default, re-export, export-all, namespace re-export, type-only, symbol
  linking, empty file), Python (prefix filtering, `__all__` override), Go
  (uppercase convention), Rust (pub keyword), Java (public keyword), C#
  (public modifier), Kotlin (default public, private/internal exclusion),
  Swift (public/open access control), Ruby (public default, private
  boundaries), C/C++ (static exclusion), PHP (top-level exports), and Tier
  2/3 empty results.

### Added (Task 20 — Import/Dependency Analysis across 28 Languages)

- **Import extraction engine** (`internal/parser/extractors/imports.go`):
  `ExtractImports(root, content, filePath, langID)` walks a tree-sitter AST
  and returns `[]parser.Import` for all import declarations. Recursive
  depth-first walker with `importContext` struct carrying language config,
  content, file path, and accumulated imports with deduplication.
- **Per-language import detection**: JS/TS/JSX/TSX (ESM `import_statement`,
  CommonJS `require()`, dynamic `import()`, re-exports via `export_statement`),
  Python (`import_statement`, `import_from_statement` with dotted-relative
  support), Go (single and grouped `import_declaration`), Rust
  (`use_declaration` with grouped `::{}` handling), Java/Kotlin
  (`import_declaration`/`import_header`), C/C++ (`preproc_include` with
  angle-bracket vs quoted-include distinction), C# (`using_directive`), Swift
  (`import_declaration`), Ruby (`require`/`require_relative` via `call` nodes),
  PHP (`namespace_use_declaration`, `include_expression`, `require_expression`),
  Bash (`source`/`.` via `command` nodes), HCL (`module` block `source`
  attribute, `terraform` → `required_providers` source detection), Dockerfile
  (`from_instruction` image name), CSS/SCSS (`@import` statements), HTML
  (`<script src>`, `<link href>`), Markdown (`inline_link` destinations).
- **Three-tier classification** using registry data: STDLIB (exact match via
  `StdlibModules` + prefix match via `StdlibPrefixes`, with Python sub-module
  support — `os.path` matches `os`), INTERNAL (prefix match via
  `InternalImportPatterns`, C/C++ quoted-include detection, Ruby
  `require_relative` forced internal), EXTERNAL (default fallback).
- **Path resolution** for filesystem-relative imports: JS/TS (`./`, `../`),
  C/C++ (quoted includes), Ruby (`require_relative`), Bash (`source ./...`)
  using `path.Join(path.Dir(filePath), source)` with POSIX semantics.
- **Deduplication** by source specifier (`ImportName`), preserving
  first-occurrence document order via `map[string]bool` tracking.
- **Registry update** (`internal/parser/registry/languages.go`): Added
  `InternalImportPatterns: []string{"./", "../"}` for HCL to classify local
  Terraform module sources as INTERNAL.
- **Unit tests** (`internal/parser/extractors/imports_test.go`): 55 test cases
  across 18 test functions covering JS/TS (named/default/namespace/side-effect
  imports, stdlib/node: prefix, internal/external classification, require,
  dynamic import, deduplication, ordering), TypeScript (import type, re-exports,
  export all), JSX, Python (stdlib, from-import, relative/parent-relative,
  external), Go (single/grouped/stdlib), Rust (std/crate/super/external), Java
  (java./javax. stdlib, external), Kotlin (kotlin. stdlib, external), C/C++
  (system includes, quoted includes with path resolution), C# (System./
  Microsoft. stdlib, external), Swift (Foundation stdlib, external), Ruby
  (require stdlib/external, require_relative with path resolution), PHP
  (namespace use), Bash (source relative, dot source), HCL (module registry/
  local source), Dockerfile (FROM), CSS (@import), and 6 edge case tests (nil
  root, unknown language, empty file, no ImportNodeTypes, source file path
  preservation, deterministic ordering).

### Added (Task 19 — Symbol Extraction across 28 Languages)

- **Symbol extraction engine** (`internal/parser/extractors/symbols.go`):
  `ExtractSymbols(root, content, langID)` walks a tree-sitter AST and returns
  `[]parser.Symbol` for all meaningful code symbols. Recursive depth-first
  walker with `extractionContext` struct carrying language config, content, and
  accumulated symbols. Supports 8 symbol kinds: function, class, method,
  interface, type_alias, enum, variable, namespace.
- **28 languages across 3 tiers**: Full extraction from 15 Tier 1 languages
  (JS, TS, JSX, TSX, Python, Go, Rust, Java, Kotlin, C, C++, C#, Swift, Ruby,
  PHP), partial extraction from 5 Tier 2 languages (Bash, SQL, Dockerfile, HCL,
  GraphQL), and structural extraction from 8 Tier 3 languages (CSS, SCSS, HTML,
  YAML, TOML, JSON, Markdown, XML).
- **Special AST marker dispatch**: Handles registry markers `_unwrap` (Python
  decorated_definition, Go type_declaration, C++ template_declaration),
  `_recurse` (Rust impl_item, Kotlin enum_class_body), `_type_spec_dispatch`
  (Go struct/interface/alias disambiguation), `_declaration_dispatch` (C/C++
  function vs variable declarations), `_element_dispatch` (HTML script/style).
- **Language-specific name extraction**: Arrow function name resolution via
  parent variable_declarator walk-up (JS/TS), CSS selector text, Markdown
  heading inline content, HCL block type + label concatenation, SQL CREATE
  object names, Dockerfile instruction names, YAML/TOML/JSON key fields,
  C/C++ function_declarator traversal, Kotlin simple_identifier support.
- **Doc comment extraction** by `DocCommentStyle`: JSDoc/Doxygen (`/** */`),
  Python docstrings (triple-quoted first expression), triple-slash (`///`),
  XML doc, hash comments (`#`).
- **Export detection** by `ExportStrategy`: keyword-based (JS `export`, Rust
  `pub`, Java/C#/PHP/Swift `public`), convention-based (Go uppercase first
  letter), prefix-based (Python `_` private), all_public (C/C++/Ruby/Kotlin),
  none (Bash/SQL). JS/TS `export_statement` unwrapping with default export
  tracking.
- **v2 flags**: `IsExported`, `IsDefaultExport`, `IsAsync`, `IsGenerator`,
  `IsStatic`, `IsAbstract`, `IsReadonly`, `IsArrowFunction`,
  `IsReactComponentLike` (JSX/TSX: uppercase name + JSX descendant),
  `IsHookLike` (JS/TS: `use[A-Z]` pattern). Modifier extraction from
  anonymous AST children.
- **Per-symbol metadata**: `SymbolHash` via `StableHashBytes(content)`,
  `SymbolID` via `sym_<hash16>`, `QualifiedName` (`ClassName.methodName`),
  `ParentSymbolID` for class methods, `Signature` (first line up to `{`),
  deterministic ordering via `SortSymbols()`.
- **Shared utilities** (`internal/parser/extractors/utils.go`): `nodeText`,
  `nodeStartLine`/`nodeEndLine` (0→1-indexed), `findChildByType`,
  `findChildByFieldName`, `findNamedChildrenByType`, `hasDescendantOfType`,
  `prevNamedSiblings`, `truncateSignature`, `generateSymbolID`.
- **Registry update** (`internal/parser/registry/languages.go`): Added Go
  `type_alias` node to `SymbolNodeTypes` for `type X = Y` alias declarations.
- **Unit tests** (`internal/parser/extractors/symbols_test.go`): 55+ test cases
  across 20 language-specific test functions covering all Tier 1 languages
  (JS functions/arrows/classes/exports/JSDoc, TS interfaces/enums/type aliases,
  JSX/TSX React components/hooks, Python classes/docstrings/decorators/async,
  Go funcs/methods/structs/interfaces/type aliases/grouped types, Rust
  fn/struct/trait/impl/enum/mod/doc, Java class/method/interface/enum/Javadoc,
  Kotlin functions/classes, C/C++ functions/structs/enums/namespaces, C#
  class/interface/namespace, Swift func/class/struct/protocol/enum, Ruby
  method/class/module/hash-doc, PHP class/method/function), Tier 2 (Bash, SQL,
  Dockerfile, HCL), Tier 3 (CSS, HTML, YAML, TOML, Markdown), and 9 edge case
  tests (empty file, unknown language, nil root, syntax errors, deterministic
  ordering, hash determinism, qualified names, signatures, symbol IDs).

### Added (Task 18 — Hashing & Determinism Utilities)

- **Stable hashing** (`internal/parser/hashing.go`): `StableHash()` and
  `StableHashBytes()` return lowercase hex SHA-256, byte-identical to Node.js
  `crypto.createHash('sha256')` for the same UTF-8 input. Used for file hashes,
  symbol hashes, and chunk hashes.
- **Deterministic sort utilities** (`internal/parser/hashing.go`):
  `SortSymbols()`, `SortExports()`, `SortReferences()`, `SortJsxUsages()`,
  `SortNetworkCalls()`, `SortChunks()`, `SortIssues()` — stable sort functions
  using `slices.SortStableFunc` that enforce deterministic output ordering for
  all extractor result types. `DeduplicateImports()` preserves first-occurrence
  order.
- **Unit tests**: 13 tests covering SHA-256 correctness (including cross-language
  Node.js validation, empty string, unicode), all sort functions, import
  deduplication, and sort stability (100-iteration determinism check).

### Added (Task 17 — Tree-Sitter Integration & Parser Pool)

- **Tree-sitter integration** via `github.com/smacker/go-tree-sitter`: Native
  CGO bindings for the tree-sitter C parsing library. Build now requires
  `CGO_ENABLED=1` with `gcc` and `musl-dev` (Alpine).
- **Grammar registry** (`internal/parser/grammars.go`): Maps 24 of 28 language
  registry entries to tree-sitter grammars (24 unique grammars across 28 IDs;
  JSX reuses TSX grammar). 4 languages (SCSS, GraphQL, XML, JSON) lack Go
  bindings and fall back to Tier 3 structural chunking. `GetGrammar()`,
  `HasGrammar()`, `GrammarLanguageIDs()` API.
- **Channel-based parser pool** (`internal/parser/pool.go`): `Pool` manages
  bounded `sitter.Parser` instances via buffered channel for goroutine-safe
  concurrent parsing. `Parse()` blocks when exhausted, respects
  `context.Context` cancellation/timeout. `Shutdown()` closes all parsers
  with `sync/atomic` idempotency guard.
- **Content normalization** (`internal/parser/normalize.go`):
  `NormalizeNewlines()` (CRLF/CR → LF, UTF-8 BOM stripping),
  `LineRangeText()` (1-indexed inclusive line extraction with clamping),
  `CountLines()`.
- **Dockerfile** (`Dockerfile`): go-builder stage updated with `gcc`,
  `musl-dev`, `CGO_ENABLED=1`, and `-extldflags '-static'` for static musl
  linking.
- **Makefile**: `build` and `test` targets now set `CGO_ENABLED=1`.
- **Unit tests**: 16 pool tests (creation, multi-language parsing, empty
  content, syntax errors, 10-goroutine concurrency, context timeout/cancel,
  nil language, shutdown idempotency, grammar registry completeness),
  21 normalization tests (newline conversion, BOM, line range extraction,
  line counting edge cases).

### Added (Task 16 — Language Registry)

- **Centralized language registry** (`internal/parser/registry/`): New package
  providing per-language configuration for all extraction tasks. Foundation for
  replacing the gRPC parser sidecar with an inline tree-sitter engine (Tasks
  17–27). Registry is read-only after initialization (package-level maps).
- **28 languages across 3 tiers**: Tier 1 (full extraction): JavaScript,
  TypeScript, JSX, TSX, Python, Go, Rust, Java, Kotlin, C, C++, C#, Swift,
  Ruby, PHP. Tier 2 (partial extraction): Bash, SQL, GraphQL, Dockerfile, HCL
  (Terraform/OpenTofu). Tier 3 (structural only): HTML, CSS, SCSS, JSON, YAML,
  TOML, Markdown, XML.
- **Per-language config** (`LanguageConfig` struct): AST node type → symbol
  kind mappings, doc comment styles, import/export patterns, stdlib modules,
  builtin types, test/config file patterns, nesting node types.
- **Extension map** (~48 entries) and **basename map** (Dockerfile, Gemfile,
  Rakefile, Makefile, shell configs). HCL extensions: `.tf`, `.tfvars`, `.tofu`.
- **`DetectLanguage()`**: Two-step detection (basename first, then extension)
  with `Dockerfile.*` prefix pattern support.
- **Registry API**: `GetLanguageConfig`, `GetLanguageByExtension`,
  `GetLanguageByBasename`, `AllLanguageIDs`, `GetTier`.
- **Unit tests**: 17 test functions covering extension detection (54 paths),
  basename detection, Dockerfile prefix pattern, unsupported extensions, tier
  classification, config completeness per tier, HasExplicitExports, doc comment
  styles, export strategies, and extension/basename map consistency.

### Added

- **Context-based logging for per-job traceability**: When multiple jobs run concurrently, all pipeline, embedding, parser, and workspace logs now carry `job_id` and `workflow` via context-injected logger. Extended to consumer, commits indexer, and copyforward so every log line in the job path is traceable.
- **Embedding progress logging**: The embedding phase now logs per-batch progress (`batch N/M, chunks: X`) and a summary with total duration, eliminating the previous silence between "artifacts persisted" and "embeddings generated" that could last 10+ minutes.
- **Split oversized sidecar chunks**: Sidecar-parsed chunks (large functions/classes) exceeding the embedding model's context window are now split on line boundaries in a post-processing step, preserving metadata and symbol references.
- **Split raw files into multiple chunks instead of truncating**: Raw files (non-JS/TS) were stored as a single chunk with content beyond 7500 chars silently dropped. Now split into multiple sequential chunks on line boundaries, each within the embedding limit.
- **`LOG_LEVEL` env var in docker-compose**: Workers respect `LOG_LEVEL` for runtime log verbosity control.

### Fixed

- **Null byte rejection in readSourceFiles**: Files containing `0x00` bytes (e.g. `favicon.ico`) pass Go's `utf8.Valid()` but are rejected by PostgreSQL TEXT columns. Added `bytes.ContainsRune(data, 0)` check.
- **Raw chunk truncation aligned with embedding limit**: Use `embedding.DefaultMaxInputChars` (7500) instead of hardcoded 8000 so raw chunks are pre-truncated at the exact limit. Added rune-safe truncation to avoid splitting multi-byte UTF-8 characters.
- **Stale retry handling**: `ClaimJob` now returns a sentinel `ErrJobNotQueued` error. Fullindex and incremental handlers detect stale retries (job already claimed/completed/failed) and return `nil` instead of failing, preventing retry storms.
- **Embedding HTTP request timeout**: Added 5-minute per-request timeout to the Ollama HTTP client to prevent a hung Ollama from blocking the entire job until the asynq task deadline.
- **gRPC max message size** (`internal/parser/client.go`): Set
  `MaxCallRecvMsgSize` and `MaxCallSendMsgSize` to 20 MB on the parser
  sidecar gRPC client. The previous default (4 MB) caused
  `ResourceExhausted` errors when indexing large repositories (e.g.
  next.js) because protobuf serialization overhead inflated 5 MB
  content batches beyond the transport limit.
- **Parser timeout too short** (`internal/config/defaults.go`): Increased
  `DefaultParserTimeout` from 30 s to 5 min to match the sidecar's
  per-batch timeout. Large batches with many files easily exceeded
  30 s, causing premature deadline-exceeded failures.
- **Batch progress logging** (`internal/parser/client.go`): Added
  per-batch log lines (batch index, total, file count, byte size) so
  sequential batch processing no longer appears stuck.

### Added (Task 12 — Commit History Indexing)

- **Commit history indexing** (`internal/workflow/commits/indexer.go`): New
  `Indexer` type persists git commit history (commits, parent relationships,
  and per-file diffs) into the database so the commit browser API and
  backoffice have data to serve. `IndexAll` indexes full history (up to 5000
  commits with `--first-parent`); `IndexSince` indexes only commits newer
  than a given base. Idempotent via ON CONFLICT upserts.
- **Git log extraction** (`internal/gitclient/gitclient.go`): `LogCommits`
  extracts commit metadata via `git log` with machine-parseable format
  (record/unit separators). `DiffStatLog` runs three git commands —
  `--name-status` (change types), `--numstat` (additions/deletions), and
  `-p` (unified diff patches) — merged by commit hash and file path into
  `FileDiffEntry` structs. Binary files are detected and skipped; patches
  exceeding 100 KB per file are discarded to prevent excessive DB storage.
  NUL-delimited parsing (`-z`) for safe handling of filenames with special
  characters.
- **Unified diff patches stored** (`commit_file_diffs.patch`): The `patch`
  column (previously always NULL) is now populated with unified diff content
  extracted from `git log -p`. The database schema, API, and frontend were
  already wired to handle patches — this change closes the data gap so the
  backoffice commit browser displays colored diffs instead of
  "No diff available — binary or omitted content."
- **Workflow integration**: Both `fullindex` and `incremental` handlers call
  the commit indexer as a non-fatal step. On success, the snapshot is linked
  to the HEAD commit via `UpdateSnapshotCommitID`.
- **Unit tests**: 13 indexer tests (happy path, edge cases, patch population,
  error propagation), 34 gitclient tests (LogCommits parsing, DiffStatLog
  merging, mergePatchLog parser, parseDiffGitPath, binary/oversized handling).

### Fixed

- **Binary files no longer crash the indexing pipeline** — `readSourceFiles()`
  now checks `utf8.Valid()` and skips non-UTF-8 file content. Binary files
  (images, compiled assets, etc.) still get a file record in the index but
  with empty content and no chunks or embeddings. Previously, inserting binary
  bytes (e.g. PNG `0x89` header) into the `file_contents.content TEXT` column
  caused a PostgreSQL `invalid byte sequence for encoding "UTF8"` error that
  failed the entire full-index job.

### Changed

- **Index all text files, not just TS/JS** — The indexing pipeline now indexes
  every file in the repository, regardless of file extension or content type.
  Files with sidecar parser support (`.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`,
  `.cjs`) are still sent to the parser sidecar for rich extraction (symbols,
  chunks, imports). All other files (including binaries, images, configs, etc.)
  are indexed with a file record and a single `"raw"` chunk containing the
  full file content. This prepares for future language sidecars while ensuring
  all repo content is tracked now.
  - `workspace.filterSourceFiles()` no longer filters by extension or content
    type; accepts all tracked regular files under 1 MB.
  - `allowedExtensions` / `IsAllowedExtension()` renamed to
    `parseableExtensions` / `HasParserSupport()` to clarify intent.
  - Both `fullindex` and `incremental` handlers split files into
    parser-supported and non-parseable groups, building synthetic
    `ParsedFileResult` entries for non-parseable files.
  - Incremental diff classification no longer gates on file extension — all
    changed files are tracked.

- **Configurable `max_tokens` for embedding input truncation** — `NewOllamaClient` now accepts a `maxTokens` parameter instead of using the hardcoded `DefaultMaxInputChars` constant. The value flows from the `embedding_provider_configs.max_tokens` column through the execution context, so each embedding config can specify its own truncation limit. Falls back to `DefaultMaxInputChars` (7500) if the value is zero or negative.

### Fixed

- **Embedding client truncates oversized inputs** (`internal/embedding/client.go`):
  `OllamaClient.Embed` now truncates chunk content that exceeds the embedding
  model's context window before sending to Ollama. The parser sidecar emits
  structurally coherent chunks without enforcing token limits (by design), so
  the backend must guard against oversized inputs. Default limit: 8000 tokens
  (~32000 characters, using the same 1 token ≈ 4 chars heuristic as the sidecar).
  Previously, large files like `app/(dashboard)/page.tsx` produced chunks that
  caused Ollama to return a 400 error ("the input length exceeds the context
  length"), failing the entire full-index job.

### Added (Task 11 — Job Event Publishing via Redis Pub/Sub)

- **Event publisher** (`internal/notify/publisher.go`): `EventPublisher` publishes
  job lifecycle events to the `myjungle:events` Redis pub/sub channel. The API's
  SSE subscriber consumes these events and broadcasts them to connected clients.
  Messages conform to `contracts/events/sse-event.v1.schema.json` with required
  fields `event`, `project_id`, `timestamp` and optional `job_id`, `snapshot_id`,
  `data`. Nil-safe: calling any method on a nil publisher is a no-op, following
  the same pattern as the API's `EventPublisher` and `Hub`.
- **Convenience helpers**: `PublishJobStarted`, `PublishJobProgress`,
  `PublishJobCompleted`, `PublishJobFailed`, and `PublishSnapshotActivated`
  construct correctly-typed events and publish with fire-and-forget semantics —
  failures are logged as warnings but never propagated to the caller. Progress
  events document a caller-side 2-second throttle expectation.
- **Unit tests** (`internal/notify/publisher_test.go`): 9 test cases using
  `alicebob/miniredis/v2` for in-process Redis with real pub/sub verification.
  Covers serialization, required fields, all five convenience helpers, nil-safety,
  and clean close.

### Added (Task 10 — Multi-Worker Execution Safety)

- **Guarded terminal state transitions**: New SQL queries
  `SetIndexingJobCompletedFromRunning` and `SetIndexingJobFailedFromRunning`
  add `WHERE status = 'running'` guards to prevent stale or retried workers
  from overwriting terminal state set by another worker. Repository methods
  `SetJobCompleted` and `SetJobFailed` now return `(bool, error)` where the
  boolean indicates whether the transition actually took effect (false = job
  no longer running, stale worker). Original unguarded queries remain for
  backend-api's legitimate use of failing queued jobs on enqueue errors.
- **Per-project advisory lock**: `TryProjectLock` acquires a PostgreSQL
  session-level advisory lock using the two-key form
  `pg_try_advisory_lock(key1, key2)` with int32 keys derived from the
  project UUID bytes, preventing conflicting indexing jobs for the same
  project from running concurrently across workers. Non-blocking: returns
  `ErrProjectLocked` immediately if another session holds the lock. Lock is
  acquired before job claim so failure allows queue retry without consuming
  the job. Session-level advisory locks auto-release on disconnect, handling
  worker crashes. If the explicit unlock fails, the connection is destroyed
  (hijacked from the pool and closed) rather than returned, preventing a
  leaked advisory lock from contaminating the pool.
- **Handler lock integration**: Both `fullindex.Handler` and
  `incremental.Handler` acquire the project advisory lock before claiming
  the job. Execution flow: load context → acquire project lock → claim job
  → execute workflow → release lock (defer). If the lock cannot be acquired,
  the task returns an error to the queue for retry. Terminal state
  transitions log a warning when 0 rows are affected (stale worker) instead
  of treating it as an error.
- **Pinned connection for advisory locks**: `TryProjectLock` acquires a
  dedicated database connection from the pool via `PinnedQueryFunc`, ensuring
  the lock and unlock execute on the same PostgreSQL session. Without this,
  pooled connections could dispatch lock/unlock to different sessions,
  silently bypassing mutual exclusion.
- **Unit tests**: Repository tests for guarded transitions (success, stale,
  DB error) and advisory lock (acquired, not acquired, DB error, connection
  acquire error, release). Handler tests for project lock failure (returns
  error for retry, no SetJobFailed called), stale completion (no error),
  and lock release guarantees on success and failure paths.

### Added (Task 09 — Incremental-Index Workflow)

- **Incremental-index workflow handler** (`internal/workflow/incremental/handler.go`):
  `Handler` implements `workflow.Handler` for the `incremental-index` workflow.
  Computes a git diff between the active snapshot's commit and the target commit,
  then re-parses and re-embeds only changed (added/modified) files. Unchanged
  files are copied forward from the previous snapshot via DB artifact duplication
  and Qdrant vector re-upsert. Deleted files are excluded from the new snapshot.
  When no valid incremental base exists (no active snapshot, embedding version
  mismatch, diff failure), the handler falls back to full-index semantics within
  the same code path. Uses the same error-handling discipline as `fullindex`:
  pre-claim errors return for queue retry, post-claim errors mark the job failed.
- **Copy-forward logic** (`internal/workflow/incremental/copyforward.go`):
  `CopyForwardFiles` duplicates file, symbol, chunk, and dependency rows from the
  old snapshot to the new one for unchanged files. Symbols are copied in
  start-line order with single-pass parent remapping. Returns an old-to-new chunk
  ID mapping used by `CopyForwardVectors` to re-upsert Qdrant vectors under new
  chunk IDs with updated snapshot metadata.
- **Git diff support** (`internal/gitclient/gitclient.go`): Added `DiffEntry` type
  and `DiffNameStatus` method running `git diff --name-status --no-renames -z` to
  compute added/modified/deleted file sets between two commits. NUL-delimited
  output parsing handles filenames with spaces.
- **VectorStore.GetPoints** (`internal/vectorstore/client.go`): Extended the
  `VectorStore` interface with `GetPoints(ctx, collection, ids, withVector)` for
  fetching existing vectors by ID. `QdrantClient` implementation uses
  `POST /collections/{name}/points` with automatic batching (100 IDs per request).
  Used by the incremental workflow to copy forward vectors for unchanged files.
- **Pipeline.RunWithoutActivation** (`internal/indexing/pipeline.go`): Extracted
  file-processing logic from `Run` into `processFiles`, added
  `RunWithoutActivation` method for the incremental workflow which activates the
  snapshot separately after copy-forward completes.
- **SQL queries**: Added `GetActiveSnapshotForBranch` (branch-scoped active
  snapshot lookup), `ListSymbolsByFileID`, `ListChunksByFileID`, and
  `ListDependenciesBySnapshotAndFile` for copy-forward artifact loading.
  Regenerated sqlc Go bindings.
- **App wiring** (`internal/app/app.go`): Replaced `StubHandler` for
  `incremental-index` with the real `incremental.Handler`. Passes the git client
  as the diff computer dependency.
- **Unit tests** (`internal/workflow/incremental/handler_test.go`): 11 test cases
  covering incremental happy path, fallback conditions (no active snapshot,
  embedding version mismatch, diff failure), parser failure, copy-forward failure,
  all-files-changed (no copy-forward), deleted files absent from new snapshot,
  workspace cleanup guarantee, invalid job ID, pre-claim failures.
  Additional tests for `DiffNameStatus` (4 cases) and `GetPoints` (3 cases).

### Added (Task 07 — Full-Index Workflow End-to-End)

- **Full-index workflow handler** (`internal/workflow/fullindex/handler.go`):
  `Handler` implements `workflow.Handler` and orchestrates the complete
  full-index pipeline end-to-end. Steps: parse job ID, load execution context
  from PostgreSQL, atomically claim the job (queued → running), resolve
  embedding version, prepare git workspace (clone/fetch + worktree), read
  source files from disk, parse via gRPC sidecar, create index snapshot,
  build per-job Ollama embedder and storage pipeline, run the pipeline
  (persist artifacts, embed chunks, upsert vectors, activate snapshot), and
  mark the job completed.
- **Structured error model**: `ErrorDetail` struct with `category`, `message`,
  and `step` fields written to the job's `error_details` JSONB column on
  failure. Error categories: `context_load`, `repo_access`, `parser`,
  `embedding`, `artifact_write`, `vector_write`, `activation`. Pipeline
  errors are categorized by inspecting error message prefixes.
- **Pre-claim vs post-claim error handling**: Errors before `ClaimJob` are
  returned to the queue framework for retry. Errors after `ClaimJob` mark
  the job as `failed` and return `nil` to prevent queue retry of a job
  already in terminal state.
- **Per-job embedder construction**: The handler creates an `OllamaClient`
  per job using the execution context's embedding config (endpoint, model,
  dimensions), ensuring correct dimension validation per project.
- **Consumer-site interfaces**: Handler defines narrow unexported interfaces
  (`jobRepo`, `workspacePreparer`, `fileParser`) for its dependencies,
  enabling isolated unit testing without modifying provider packages.
- **App wiring** (`internal/app/app.go`): Replaced `StubHandler` for
  `full-index` with the real `fullindex.Handler`. Added workspace manager,
  parser client, Qdrant client, and artifact writer initialization. Parser
  client is properly closed on shutdown.
- **Unit tests** (`internal/workflow/fullindex/handler_test.go`): 12 test
  cases covering happy path (empty files), invalid job ID, pre-claim
  failures (load context, claim), post-claim failures (resolve version,
  workspace, parser, create snapshot, pipeline errors), error
  categorization, language detection, error details JSON structure, and
  workspace cleanup guarantee.

### Added (Task 06 — Embeddings, Artifact Persistence, and Snapshot Activation)

- **Embedding version resolution** (`internal/embedding/version.go`): `VersionLabel`
  builds a deterministic label from `"{provider}-{model}-{dimensions}"`.
  `ResolveVersion` looks up an existing `embedding_versions` row by label or
  creates one, with unique-violation race handling for concurrent workers.
- **Ollama embedding client** (`internal/embedding/client.go`): `Embedder`
  interface and `OllamaClient` implementation calling `POST /api/embed`. Texts
  are batched in groups of 10 per HTTP request. Returned vector dimensions are
  validated against the configured size. Clear error messages for connection
  refused, model not found, and dimension mismatch.
- **Qdrant vector store** (`internal/vectorstore/`): `VectorStore` interface
  with `EnsureCollection` (lazy creation with Cosine distance) and
  `UpsertPoints`. REST client using `net/http` against the Qdrant HTTP API.
  `CollectionName` builds `project_{id}__emb_{version}` per `QDRANT.md`.
  Handles 409 Conflict for concurrent collection creation.
- **Artifact writer** (`internal/artifact/writer.go`): `Writer` persists a
  single `ParsedFileResult` into PostgreSQL — deduplicates file content via
  SHA-256 hash and `UpsertFileContent`, inserts `files`, `symbols` (with
  single-pass parent linking via parser-assigned IDs), `code_chunks` (linked
  to symbol DB IDs), and `dependencies`. Returns `WriteResult` with
  `ChunkForEmbed` metadata needed for the embedding step.
- **Storage pipeline** (`internal/indexing/pipeline.go`): `Pipeline`
  orchestrates the full indexing flow — ensures Qdrant collection, iterates
  parsed files to persist artifacts, embeds chunks via `Embedder`, upserts
  vectors to `VectorStore`, updates job progress counters per file, and
  activates the snapshot only after all writes succeed. Failures leave the
  previous active snapshot untouched.
- **Repository extension** (`internal/repository/job.go`): Added
  `ActivateSnapshot` method wrapping the existing `ActivateSnapshot` query
  with consistent error formatting.
- **Unit tests**: Embedding version resolution (4 tests), Ollama client (5
  tests with httptest), Qdrant client (5 tests with httptest), artifact writer
  (5 tests with mock Querier), storage pipeline (6 tests with mock Embedder,
  VectorStore, and Querier), repository ActivateSnapshot (2 tests).

### Added (Task 05 — Parser Sidecar Integration)

- **Protobuf/gRPC bindings** (`internal/parser/pb/parser/v1/`): Generated Go
  code from `contracts/proto/parser/v1/parser.proto` using `protoc` with
  `protoc-gen-go` and `protoc-gen-go-grpc`. Committed so the worker builds
  reproducibly without requiring `protoc` at build time.
- **Parser client** (`internal/parser/client.go`): `Client` struct wrapping a
  gRPC connection to the parser sidecar. `NewClient` dials with insecure
  transport (same-container sidecar). `Health(ctx)` calls the Health RPC.
  `ParseFiles(ctx, req)` calls ParseFiles and maps the response into worker
  domain types. `ParseFilesBatched(ctx, projectID, branch, commitSHA, files)`
  splits file inputs into batches, calls ParseFiles for each, and concatenates
  results. Per-call timeout applied via `context.WithTimeout`. gRPC status
  errors wrapped with `"parser: <op>:"` prefix matching codebase conventions.
- **Domain types** (`internal/parser/domain.go`): Worker-side structs decoupled
  from protobuf: `ParsedFileResult`, `Symbol`, `SymbolFlags`, `Import`,
  `Chunk`, `Issue`, `Export`, `Reference`, `JsxUsage`, `NetworkCall`,
  `FileFacts`, `ParserMeta`, `ExtractorStatus`. `MapParsedFile` and
  `MapParsedFiles` convert protobuf responses into domain types. Parser
  issues/warnings are preserved as data in `ParsedFileResult.Issues`, not
  treated as transport errors.
- **File batching** (`internal/parser/batch.go`): `FileInput` type carrying
  `(FilePath, Content, Language)`. `BatchFileInputs` splits inputs into batches
  respecting `MaxFilesPerBatch` (50) and `MaxBytesPerBatch` (5 MB). File
  ordering is preserved. Oversized single files get their own batch.
- **Makefile**: Added `proto-gen` target for regenerating Go bindings.
- **Dependencies**: Added `google.golang.org/grpc` as direct dependency.
  Promoted `google.golang.org/protobuf` from indirect to direct.
- **Unit tests**: Parser client tests (7 cases) using in-process gRPC server
  covering health, parse, RPC errors, timeout, connection refused, batched
  calls, and nil-safe close. Batch tests (7 cases) covering empty input, single
  file, count-based splits, size-based splits, oversized files, exact-limit
  boundary, and ordering. Domain mapping tests (5 cases) covering full field
  mapping, nil input, empty nested fields, multiple files, and nil symbol flags.

### Added (Task 04 — Workspace and Repo Cache Lifecycle)

- **Git client** (`internal/gitclient/gitclient.go`): `GitRunner` interface
  abstracting git CLI execution with `ExecRunner` production implementation.
  `Client` struct providing `Clone`, `Fetch`, `Checkout`, `HeadSHA`,
  `RemoteURL`, and `ListTrackedFiles` methods. `RunError` type wraps failed
  commands with truncated output for debugging.
- **SSH environment** (`internal/sshenv/sshenv.go`): `Keyscanner` interface
  abstracting `ssh-keyscan` with `ExecKeyscanner` production implementation.
  `Setup` writes decrypted SSH key to job-scoped temp file (`0600` perms),
  runs keyscan for the repo host, writes `known_hosts`, and builds
  `GIT_SSH_COMMAND` with `StrictHostKeyChecking=yes`. `ParseHostname` handles
  both scp-style (`git@host:path`) and `ssh://` URL formats. `Cleanup` is
  idempotent and nil-safe.
- **Workspace manager** (`internal/workspace/workspace.go`): `Manager` struct
  orchestrating directory layout (`projects/{id}/repo` + `jobs/{id}/tmp` +
  `jobs/{id}/worktree`), SSH setup, git clone/fetch, per-job worktree
  creation, HEAD SHA recording, and source file selection. Each job operates
  in an isolated git worktree created from the project repo cache. `Prepare`
  returns a `Result` (worktree dir, commit SHA, filtered source files) and a
  cleanup function that removes the job worktree and temp files while
  preserving the project repo cache. File selection filters to `.ts`, `.tsx`,
  `.js`, `.jsx`, `.mjs`, `.cjs` extensions and skips files over 1 MB.
  Remote URL mismatch is detected before reusing a cached clone.
- **Unit tests**: Git client mock-based tests (13 cases), SSH environment
  filesystem tests with fake keyscanner (10 cases), workspace integration
  tests with fake git runner and real filesystem for file filtering (15 cases).
  Build-tagged integration tests (8 cases) verify clone, fetch, and workspace
  preparation against a real public Git repository.

### Added (Task 03 — Job Repository and Execution Context Loading)

- **SQL queries** (`datastore/postgres/queries/`): Added `GetIndexingJob` (load job
  by ID) and `GetProjectSSHKeyForExecution` (single query joining projects, SSH key
  assignments, and SSH keys for worker execution). Regenerated sqlc bindings.
- **SSH decryption** (`internal/sshkey/decrypt.go`): Worker-local `DeriveKey` and
  `Decrypt` functions compatible with backend-api's AES-256-GCM encryption format.
  Uses SHA-256 key derivation and `nonce || sealed(ciphertext + tag)` format.
- **Execution context** (`internal/execution/context.go`): `Context` struct carrying
  all data a workflow handler needs: job ID/type, project ID, repo URL, branch,
  decrypted SSH private key, and embedding provider config (provider, endpoint,
  model, dimensions).
- **Job repository** (`internal/repository/job.go`): `JobRepository` wrapping sqlc
  `Querier` interface with `LoadExecutionContext` (loads job → validates queued
  status → loads project+SSH key → decrypts private key → loads embedding config)
  and state transition methods (`SetJobRunning`, `SetJobProgress`,
  `SetJobCompleted`, `SetJobFailed`, `CreateSnapshot`).
- **App wiring** (`internal/app/app.go`): Added `Repo` field to `App` struct,
  created in `New()` with DB queries and SSH encryption secret.
- **Unit tests**: SSH decryption roundtrip/failure tests, repository mock-based
  tests covering success path and all failure cases (job not found, wrong status,
  no SSH key, decryption failure, missing embedding config).

### Added (Task 02 — Queue Consumer and Workflow Dispatcher)

- **Workflow types** (`internal/workflow/dispatcher.go`): `WorkflowTask` struct
  matching `contracts/queue/workflow-task.v1.schema.json`, `Handler` interface,
  and `Dispatcher` for handler registration.
- **Stub handlers** (`internal/workflow/stub.go`): Placeholder handlers for
  `full-index` and `incremental-index` workflows (log and return nil).
- **Queue consumer** (`internal/queue/consumer.go`): Payload decoding with
  required-field validation (`job_id`, `workflow`, `enqueued_at`), `BuildServeMux`
  for asynq handler registration, and `wrapHandler` that decodes, validates,
  logs context (including retry count), manages registry status, and delegates
  to the workflow handler.
- **Worker registry** (`internal/registry/registry.go`): Ephemeral worker status
  in Redis via direct `go-redis/v9` client. Key pattern `worker:status:{worker_id}`
  with 30s TTL refreshed every 10s. Status transitions: `starting` → `idle` →
  `busy` → `draining`. Worker ID from `WORKER_ID` env var with `os.Hostname()`
  fallback.
- **App wiring** (`internal/app/app.go`): Replaced idle heartbeat loop with
  `Server.Start(mux)` (non-blocking). Wired dispatcher and registry into
  bootstrap. Added registry close to shutdown sequence.
- **Dependencies**: Promoted `github.com/redis/go-redis/v9` from indirect to
  direct dependency.
- **Unit tests**: Payload decode/validation, dispatcher registration, registry
  status transitions, status payload marshaling, nil-safe/idempotent close.

### Fixed (Task 01 — Code Review Feedback)

- **App.Close()** now shuts down the asynq Server to prevent resource leaks.
- **asynqLogger.Fatal()** now calls `os.Exit(1)` to satisfy the asynq
  Logger interface contract.
- **TestLogSummary_NoSecrets** properly captures and restores the original
  slog default logger instead of the already-replaced one.
- **TestNew_FailsOnBadDSN** defensively cleans up if `New()` unexpectedly
  returns a non-nil App.
- **Config.Load()** normalizes `LOG_LEVEL` and `LOG_FORMAT` to lowercase
  before validation, so mixed-case input (e.g. `INFO`) works correctly.

### Added (Task 01 — Initial Setup and Worker Bootstrap)

- **Config package** (`internal/config/`): Environment-based configuration with
  `Load()`, `LoadForTest()`, validation, and secret redaction. Required env vars:
  `POSTGRES_DSN`, `REDIS_URL`, `SSH_KEY_ENCRYPTION_SECRET`. All optional vars
  have defaults matching `docker-compose.yaml`.
- **Logger package** (`internal/logger/`): Structured `slog` setup with
  configurable level and format (JSON/text).
- **Postgres package** (`internal/storage/postgres/`): Connection pool wrapper
  using `pgxpool` with shared `sqlc` queries from `myjungle/datastore`.
  Includes nil-safe `Close()`/`Ping()` and error helpers (`IsUniqueViolation`,
  `IsDeadlock`).
- **Queue package** (`internal/queue/`): `asynq.Server` creation with Redis URL
  validation, configurable concurrency, and slog-bridged logger. Server is
  created but not started (Task 02 will register handlers).
- **App package** (`internal/app/`): Lifecycle orchestration with `New()` (wires
  all dependencies), `Run()` (signal-aware idle loop), and `Close()` (resource
  cleanup).
- **Thin entrypoint** (`cmd/worker/main.go`): Reduced from 112-line skeleton to
  ~40-line bootstrap: logger init, config load, app start.
- **Unit tests**: Config parsing, default verification, required-env panics,
  secret redaction, and app bootstrap failure paths.
- **Dependencies**: Added `pgx/v5`, `hibiken/asynq` to `go.mod`.
