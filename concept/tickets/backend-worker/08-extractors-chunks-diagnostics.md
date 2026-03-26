# 08 — Chunking, Diagnostics & File Metadata

## Status
Done

## Goal
Implement the chunk extractor (symbol-aligned boundaries with semantic roles), diagnostics extractor (parse errors, structural warnings, deep nesting detection), file metadata computation (SHA-256 hash, line count, byte size), and file facts derivation. Includes stable content/chunk hashing for deterministic incremental indexing.

## Depends On
05-parser-engine, 06-extractors-symbols-imports

## Scope

### Chunk Extraction (`extractors/chunks.go`)

**Main function:** `ExtractChunks(content string, filePath string, symbols []parser.Symbol, imports []parser.Import, langID string) []parser.Chunk`

Unlike other extractors, chunks are text-based and do not require an AST -- they use symbol and import data when available but fall back to raw content.

**Chunk types:**

| Type | Description |
|---|---|
| `module_context` | Line 1 to the line before the first major declaration. Includes imports, top-level type aliases, constants, comments. Skipped if first declaration starts at line 1. |
| `function` | Full function source text including leading doc comment. One chunk per top-level function symbol. |
| `class` | Full class source for classes <= 200 lines. For larger classes: class declaration line + all method signatures + closing brace. |
| `config` | Entire file content. Triggered by per-language `ConfigFilePatterns` from registry. |
| `test` | Per test block or full file fallback. Triggered by per-language `TestFilePatterns`. |

**Config file patterns:** JS/TS `*.config.*`, Python `pyproject.toml`/`setup.py`, Go `go.mod`, Rust `Cargo.toml`, Java `pom.xml`/`build.gradle`, Docker `Dockerfile`/`docker-compose*.yml`, HCL `*.tf`/`*.tfvars`/`*.tofu`, Pulumi `Pulumi.yaml`/`Pulumi.*.yaml`.

**Test file patterns:** JS/TS `*.test.*`/`*.spec.*`/`__tests__/*`, Python `test_*.py`/`*_test.py`, Go `*_test.go`, Java `*Test.java`, Rust `tests/*.rs`.

**Tier 3 chunking:** Single full-file chunk. If the file matches a config pattern, emit CONFIG; otherwise emit MODULE_CONTEXT. No FUNCTION or CLASS chunks. No duplicates.

**Token estimation:** `estimateTokens(text) = (len(text) + 3) / 4` (approximately 1 token per 4 characters).

**Chunk identity and hashing:**
- `chunk_id = fmt.Sprintf("%s:%s:%d-%d", filePath, chunkType, startLine, endLine)`
- `chunk_hash = sha256(chunkContent)` -- stable content hash for dedup/upsert decisions
- `context = fmt.Sprintf("File: %s > %s %s", filePath, ownerKind, ownerName)` for function/class chunks

**v2 fields:**
- `OwnerQualifiedName`, `OwnerKind`: owning symbol context
- `SemanticRole`: `implementation`, `api_surface`, `config`, `test`, `ui_component`, `hook`
- `IsExportedContext`: whether the owning symbol is exported

**Ordering:** Deterministic: MODULE_CONTEXT -> CLASS (by start_line) -> FUNCTION (by start_line) -> CONFIG -> TEST.

**No overlap:** Chunks must not contain overlapping content. Methods inside classes are part of the CLASS chunk, not separate FUNCTION chunks.

### Diagnostics (`extractors/diagnostics.go`)

**Main function:** `ExtractDiagnostics(root *sitter.Node, content []byte, langID string) []parser.Issue`

**Parse error collection:** Walk AST for ERROR (`node.IsError()`) and MISSING (`node.IsMissing()`) nodes. Cap at 50 issues per file. Merge consecutive same-line errors into a single "Multiple parse errors" issue.

**Structural warnings:**

| Warning | Threshold | Code | Severity |
|---|---|---|---|
| Long function | > 200 lines | `LONG_FUNCTION` | WARNING |
| Long file | > 1000 lines | `LONG_FILE` | WARNING |
| Deep nesting | > 6 levels | `DEEP_NESTING` | WARNING |
| No exports | 0 exports in non-test file | `NO_EXPORTS` | INFO |

**Deep nesting detection:** Uses per-language `NestingNodeTypes` from registry:
- JS/TS: if, for, for_in, while, do, try, switch
- Python: if, for, while, try, with
- Go: if, for, select, switch
- Rust: if_expression, for_expression, while_expression, loop_expression, match_expression

**`NO_EXPORTS` warning:** Only emitted when `HasExplicitExports = true` (primarily JS/TS). Never emitted for Python, Go, or Tier 2/3 languages.

**Issue factory functions (called by the engine, not the diagnostics extractor):**
- `CreateUnsupportedLanguageIssue(ext)`
- `CreateOversizedFileIssue(size, limit)`
- `CreateParseTimeoutIssue(filePath, timeoutMs)`
- `CreateExtractionErrorIssue(extractor, err)`

### File Metadata (`extractors/file_meta.go`)

**Main function:** `ExtractFileMeta(content string) (fileHash string, lineCount int32, sizeBytes int64)`

- `fileHash`: SHA-256 of normalized content
- `lineCount`: number of lines
- `sizeBytes`: byte length of normalized content

Always runs, even for oversized or unparseable files.

### Stable Hashing for Determinism

Given identical `(file_path, language, content)`, the parser produces stable output: same symbol ordering, same chunk boundaries, same hash values. Determinism is critical for incremental indexing so unchanged files do not churn vectors or metadata rows.

## Key Files

| File/Package | Purpose |
|---|---|
| `internal/parser/extractors/chunks.go` | Chunk extraction with symbol alignment and semantic roles |
| `internal/parser/extractors/diagnostics.go` | Parse errors, structural warnings, issue factories |
| `internal/parser/extractors/file_meta.go` | File hash (SHA-256), line count, byte size |

## Acceptance Criteria
- [x] MODULE_CONTEXT chunk captures imports and top-level declarations
- [x] FUNCTION chunks generated per top-level function with doc comments
- [x] CLASS chunks: full for <= 200 lines, summarized for larger
- [x] CONFIG chunks for per-language config file patterns
- [x] TEST chunks for per-language test file patterns with per-block detection
- [x] Tier 3 languages produce single MODULE_CONTEXT or CONFIG chunk (no duplicates)
- [x] Token estimation uses `(len + 3) / 4`
- [x] Chunk IDs unique and deterministic; hash is SHA-256 of chunk content
- [x] v2 fields (owner, semantic_role, is_exported_context) populated correctly
- [x] Deterministic ordering; no overlapping content between chunks
- [x] ERROR and MISSING nodes collected from AST for all 28 languages
- [x] Cap at 50 parse errors per file; consecutive same-line errors merged
- [x] `LONG_FUNCTION`, `LONG_FILE`, `DEEP_NESTING` warnings with correct thresholds
- [x] `NO_EXPORTS` only emitted when `HasExplicitExports` is true
- [x] Factory functions produce correctly structured issues
- [x] File hash, line count, and byte size computed correctly
- [x] Empty file returns empty chunks slice
