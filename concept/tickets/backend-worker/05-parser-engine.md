# 05 — Embedded Tree-Sitter Parser Engine

## Status
Done

## Goal
Build the in-process tree-sitter parser engine that replaces the former gRPC parser integration. Includes go-tree-sitter bindings, the language registry (28 languages, 3 tiers with per-language config), the bounded parser pool, content normalization, and the `Engine` struct that satisfies the `fileParser` interface used by workflow handlers.

## Depends On
—

## Scope

### Language Registry (`internal/parser/registry/`)

Centralized configuration that all extractors query for per-language settings.

**Registry types:**

```go
type LanguageConfig struct {
    ID, Tier, Extensions, Basenames
    SymbolNodeTypes        map[string]string  // AST node type -> symbol kind
    DocCommentStyle        DocCommentStyle     // jsdoc, docstring, slashslashslash, xmldoc, hash, doxygen, none
    ImportNodeTypes        []string
    StdlibModules          map[string]bool
    StdlibPrefixes         []string
    InternalImportPatterns []string
    Export                 ExportStrategy      // keyword, convention, prefix, all_public, none
    BuiltinTypes           map[string]bool
    TestFilePatterns, ConfigFilePatterns, TestBlockPatterns []string
    NestingNodeTypes       []string
    HasExplicitExports     bool
}
```

**Registry API:** `GetLanguageConfig(langID)`, `GetLanguageByExtension(ext)`, `GetLanguageByBasename(basename)`, `AllLanguageIDs()`, `GetTier(langID)`, `DetectLanguage(filename)`.

**Language detection:** Three-step: exact basename -> prefix pattern (`Dockerfile.*`) -> extension map (50+ entries).

**Tier definitions:**
- Tier 1 (15 languages): JS, TS, JSX, TSX, Python, Go, Rust, Java, Kotlin, C, C++, C#, Swift, Ruby, PHP
- Tier 2 (5 languages): Bash, SQL, GraphQL, Dockerfile, HCL
- Tier 3 (8 languages): HTML, CSS, SCSS, JSON, YAML, TOML, Markdown, XML

Each Tier 1 language has complete config (all fields populated). Tier 2 has partial config (SymbolNodeTypes, ImportNodeTypes, NestingNodeTypes). Tier 3 has minimal config.

### Tree-Sitter Integration (`internal/parser/grammars.go`)

Grammar registry maps language IDs to `*sitter.Language` objects loaded from Go bindings:
- 15 languages from `smacker/go-tree-sitter` (JS, TS, TSX, Python, Go, Rust, Java, C, C++, C#, Ruby, PHP, HTML, CSS, Bash)
- Community packages for remaining languages (Kotlin, Swift, YAML, TOML, Markdown, Dockerfile, SQL, HCL)
- JSX uses the TSX grammar
- 4 languages without Go bindings (SCSS, GraphQL, XML, JSON) fall back to text-only processing

CGO required: `CGO_ENABLED=1` for all builds. Docker build stage includes `gcc` and `musl-dev`.

### Parser Pool (`internal/parser/pool.go`)

`sitter.Parser` is not goroutine-safe. The pool provides exclusive access via a buffered channel:

```go
type Pool struct {
    parsers chan *sitter.Parser
    size    int
}
func NewPool(size int) *Pool
func (p *Pool) Parse(ctx context.Context, content []byte, lang *sitter.Language) (*sitter.Tree, error)
func (p *Pool) Shutdown()
func (p *Pool) Size() int
```

- Pool size: `PARSER_POOL_SIZE` (default: `runtime.NumCPU()`)
- Parsers are created at startup and reused across requests
- Acquire respects context cancellation and pool shutdown
- `parser.Reset()` is called after each parse to clear incremental-parse state
- Parse timeout enforced via `context.WithTimeout` + `sitter.Parser.SetCancellationFlag()`

### Content Normalization (`internal/parser/normalize.go`)

- `NormalizeNewlines(content)`: CRLF/CR -> LF, strips UTF-8 BOM
- `LineRangeText(content, startLine, endLine)`: extracts 1-indexed line ranges
- `CountLines(content)`: returns line count

### Parser Engine (`internal/parser/engine/engine.go`)

The `Engine` struct satisfies the existing `fileParser` interface:

```go
type fileParser interface {
    ParseFilesBatched(ctx context.Context, projectID, branch, commitSHA string, files []parser.FileInput) ([]parser.ParsedFileResult, error)
}
```

**EngineConfig:** `PoolSize`, `Timeout` (batch), `TimeoutPerFile`, `MaxFileSize`.

**Single-file pipeline (`runPipeline`):**
1. Normalize content (CRLF, BOM)
2. Check empty -> return valid empty result
3. Check file size -> add `OVERSIZED_FILE` issue if exceeded
4. Detect language -> add `UNSUPPORTED_LANGUAGE` issue if unknown
5. Compute file metadata (SHA-256, line count, byte size)
6. Parse with tree-sitter (per-file timeout) -> add `PARSE_TIMEOUT` on failure
7. Run extractors based on tier (all: symbols, chunks, diagnostics; Tier 1+2: imports; Tier 1: exports, references, network; JSX/TSX: JSX usages)
8. Compute file facts from extracted data
9. Set parser metadata (version, enabled extractors)

Each extractor is wrapped in `runExtractor[T any]()` for panic recovery, timing, and error isolation. A panic in one extractor produces an `EXTRACTION_ERROR` issue but does not crash the pipeline.

**Batch pipeline (`ParseFilesBatched`):** Uses `errgroup` bounded by pool size. Results maintain input order. Partial failure: one file's error does not affect others.

**File facts computation:** `HasJsx`, `HasDefaultExport`, `HasNamedExports`, `HasTopLevelSideEffects`, `HasReactHookCalls`, `HasFetchCalls`, `HasClassDeclarations`, `HasTests`, `HasConfigPatterns`.

### Timeouts and Limits

| Setting | Env Variable | Default |
|---|---|---|
| Per-file parse timeout | `PARSER_TIMEOUT_PER_FILE` | 30s |
| Batch timeout | `PARSER_TIMEOUT` | 5m |
| Max file size | `PARSER_MAX_FILE_SIZE` | 10 MB |

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/parser/registry/registry.go` | Registry types, API functions, extension/basename maps, DetectLanguage |
| `internal/parser/registry/languages.go` | 28 language config definitions |
| `internal/parser/grammars.go` | Grammar loading (language ID -> `*sitter.Language`) |
| `internal/parser/pool.go` | Bounded channel-based parser pool |
| `internal/parser/normalize.go` | Content normalization utilities |
| `internal/parser/engine/engine.go` | Engine struct, pipeline orchestration, file facts |
| `internal/parser/domain.go` | Domain types (`ParsedFileResult`, `Symbol`, `Import`, `Chunk`, etc.) |

## Acceptance Criteria
- [x] All 28 language IDs registered with correct tiers (15 Tier 1, 5 Tier 2, 8 Tier 3)
- [x] Extension map covers 50+ file extensions including `.tf`, `.tfvars`, `.tofu`
- [x] Basename detection works for Dockerfile, Gemfile, Rakefile, Makefile, shell configs
- [x] `DetectLanguage()` handles `Dockerfile.*` prefix pattern
- [x] Grammar registry contains 24+ language entries (languages without Go bindings use text-only fallback)
- [x] Parser pool initializes N parsers based on config with exclusive access via channel
- [x] Concurrent parse calls do not crash (pool serializes access)
- [x] Parse timeout enforced via context cancellation
- [x] `Engine` satisfies the `fileParser` interface (same method signature)
- [x] Single-file pipeline runs all extractors in correct tier order
- [x] Per-extractor error isolation (panic in one does not crash pipeline)
- [x] Batch pipeline processes files concurrently with bounded goroutines
- [x] Results in input order for batch requests
- [x] Partial failure: one file's error does not affect others
- [x] Oversized, unsupported, and timeout files produce appropriate issues
- [x] `Engine.Close()` shuts down the parser pool
