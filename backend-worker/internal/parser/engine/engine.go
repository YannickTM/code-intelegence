// Package engine provides the embedded parser Engine used by workflow
// handlers. It processes files directly with tree-sitter and extractors.
package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"golang.org/x/sync/errgroup"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/extractors"
	"myjungle/backend-worker/internal/parser/registry"
)

const (
	parserVersion  = "1.0.0"
	grammarVersion = "1.0.0"

	defaultTimeoutFile = 10 * time.Second
	defaultMaxFileSize = int64(1 * 1024 * 1024) // 1 MB
)

// Config configures the parser Engine.
type Config struct {
	PoolSize       int           // default: runtime.NumCPU()
	TimeoutPerFile time.Duration // per-file parse timeout
	MaxFileSize    int64         // max bytes per file
}

// Engine processes source files using tree-sitter and extractors.
// It satisfies the fileParser interface used by workflow handlers.
type Engine struct {
	pool        *parser.Pool
	timeout     time.Duration
	maxFileSize int64
}

// New creates an Engine with the given configuration.
func New(cfg Config) (*Engine, error) {
	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = runtime.NumCPU()
	}
	timeout := cfg.TimeoutPerFile
	if timeout <= 0 {
		timeout = defaultTimeoutFile
	}
	maxFileSize := cfg.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = defaultMaxFileSize
	}
	return &Engine{
		pool:        parser.NewPool(poolSize),
		timeout:     timeout,
		maxFileSize: maxFileSize,
	}, nil
}

// ParseFilesBatched parses a batch of files concurrently using an errgroup
// bounded by the pool size. Results maintain the same order as input.
// Individual file errors are captured as Issues (partial-failure semantics).
func (e *Engine) ParseFilesBatched(ctx context.Context, projectID, branch, commitSHA string, files []parser.FileInput) ([]parser.ParsedFileResult, error) {
	results := make([]parser.ParsedFileResult, len(files))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(e.pool.Size())

	for i, f := range files {
		g.Go(func() error {
			results[i] = e.runPipeline(gCtx, f)
			return nil // never return errors — partial failure semantics
		})
	}

	_ = g.Wait()
	return results, nil
}

// Close shuts down the parser pool.
func (e *Engine) Close() {
	if e != nil && e.pool != nil {
		e.pool.Shutdown()
	}
}

// runPipeline runs the full extraction pipeline for a single file.
func (e *Engine) runPipeline(ctx context.Context, file parser.FileInput) parser.ParsedFileResult {
	result := parser.ParsedFileResult{
		FilePath: file.FilePath,
	}

	// 1. Normalize content (CRLF, BOM).
	content := parser.NormalizeNewlines(file.Content)

	// 2. Empty file — return valid empty result.
	if content == "" {
		return result
	}

	// 3. Size check.
	contentSize := int64(len(content))
	if contentSize > e.maxFileSize {
		result.Issues = append(result.Issues, extractors.CreateOversizedFileIssue(contentSize, e.maxFileSize))
		result.FileHash, result.LineCount, result.SizeBytes = extractors.ExtractFileMeta(content)
		return result
	}

	// 4. Detect language.
	langID := file.Language
	if langID == "" {
		var err error
		langID, err = registry.DetectLanguage(file.FilePath)
		if err != nil {
			ext := filepath.Ext(file.FilePath)
			result.Issues = append(result.Issues, extractors.CreateUnsupportedLanguageIssue(ext))
			result.FileHash, result.LineCount, result.SizeBytes = extractors.ExtractFileMeta(content)
			return result
		}
	}
	result.Language = langID

	// 5. File metadata.
	result.FileHash, result.LineCount, result.SizeBytes = extractors.ExtractFileMeta(content)

	// 6. Tree-sitter parse (with per-file timeout).
	contentBytes := []byte(content)
	var root *sitter.Node
	var tree *sitter.Tree
	grammar := parser.GetGrammar(langID)
	if grammar != nil {
		fileCtx, cancel := context.WithTimeout(ctx, e.timeout)
		// Defer cancel instead of calling it immediately after Parse.
		// go-tree-sitter's ParseCtx spawns a goroutine that races between
		// <-ctx.Done() and <-parseComplete. Calling cancel() right after
		// Parse returns makes ctx.Done() ready at the same instant the
		// parseComplete channel closes, so the goroutine may non-
		// deterministically pick ctx.Done() and set a stale cancellation
		// flag on the parser — causing subsequent parses to fail with
		// "operation limit was hit". Deferring cancel lets the goroutine
		// always exit via parseComplete first.
		defer cancel()
		var err error
		tree, err = e.pool.Parse(fileCtx, contentBytes, grammar)
		// Check fileCtx BEFORE cancel — cancel() sets Err() unconditionally.
		// Use DeadlineExceeded specifically: parent Canceled propagates to
		// fileCtx but should not be classified as a timeout.
		timedOut := errors.Is(fileCtx.Err(), context.DeadlineExceeded)
		if err != nil {
			if timedOut {
				// The per-file timeout fired.
				timeoutMs := int(e.timeout.Milliseconds())
				result.Issues = append(result.Issues, extractors.CreateParseTimeoutIssue(file.FilePath, timeoutMs))
			} else {
				// Pool shutdown, parent context cancellation, or other error.
				result.Issues = append(result.Issues, extractors.CreateParseErrorIssue(file.FilePath, err))
			}
			// Continue without AST — chunks/file-meta still run.
		} else {
			root = tree.RootNode()
		}
	}

	// 7. Run extractors based on tier.
	tier := registry.GetTier(langID)
	var statuses []parser.ExtractorStatus
	var enabledExtractors []string

	// --- All tiers: symbols (requires AST) ---
	symbols, symStatus := runExtractor("symbols", func() []parser.Symbol {
		if root == nil {
			return nil
		}
		return extractors.ExtractSymbols(root, contentBytes, langID)
	})
	result.Symbols = symbols
	statuses = append(statuses, symStatus)
	enabledExtractors = append(enabledExtractors, "symbols")
	if symStatus.Status == "FAILED" {
		result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("symbols", fmt.Errorf("%s", symStatus.Message)))
	}
	if len(result.Symbols) > 0 {
		parser.SortSymbols(result.Symbols)
	}

	// --- Tier 1 + Tier 2: imports (requires AST) ---
	if tier == registry.Tier1 || tier == registry.Tier2 {
		imports, impStatus := runExtractor("imports", func() []parser.Import {
			if root == nil {
				return nil
			}
			return extractors.ExtractImports(root, contentBytes, file.FilePath, langID)
		})
		result.Imports = parser.DeduplicateImports(imports)
		statuses = append(statuses, impStatus)
		enabledExtractors = append(enabledExtractors, "imports")
		if impStatus.Status == "FAILED" {
			result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("imports", fmt.Errorf("%s", impStatus.Message)))
		}
	}

	// --- Tier 1 only: exports (requires AST) ---
	if tier == registry.Tier1 {
		exports, expStatus := runExtractor("exports", func() []parser.Export {
			if root == nil {
				return nil
			}
			return extractors.ExtractExports(root, contentBytes, langID, result.Symbols)
		})
		result.Exports = exports
		statuses = append(statuses, expStatus)
		enabledExtractors = append(enabledExtractors, "exports")
		if expStatus.Status == "FAILED" {
			result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("exports", fmt.Errorf("%s", expStatus.Message)))
		}
		if len(result.Exports) > 0 {
			parser.SortExports(result.Exports)
		}
	}

	// --- Tier 1 only: references (requires AST) ---
	if tier == registry.Tier1 {
		refs, refStatus := runExtractor("references", func() []parser.Reference {
			if root == nil {
				return nil
			}
			return extractors.ExtractReferences(root, contentBytes, result.Symbols, result.Imports, langID)
		})
		result.References = refs
		statuses = append(statuses, refStatus)
		enabledExtractors = append(enabledExtractors, "references")
		if refStatus.Status == "FAILED" {
			result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("references", fmt.Errorf("%s", refStatus.Message)))
		}
		if len(result.References) > 0 {
			parser.SortReferences(result.References)
		}
	}

	// --- JSX/TSX only: JSX usages (requires AST) ---
	if langID == "jsx" || langID == "tsx" {
		jsxUsages, jsxStatus := runExtractor("jsx_usages", func() []parser.JsxUsage {
			if root == nil {
				return nil
			}
			return extractors.ExtractJsxUsages(root, contentBytes, result.Symbols, langID)
		})
		result.JsxUsages = jsxUsages
		statuses = append(statuses, jsxStatus)
		enabledExtractors = append(enabledExtractors, "jsx_usages")
		if jsxStatus.Status == "FAILED" {
			result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("jsx_usages", fmt.Errorf("%s", jsxStatus.Message)))
		}
		if len(result.JsxUsages) > 0 {
			parser.SortJsxUsages(result.JsxUsages)
		}
	}

	// --- Tier 1 only: network calls (requires AST) ---
	if tier == registry.Tier1 {
		calls, netStatus := runExtractor("network_calls", func() []parser.NetworkCall {
			if root == nil {
				return nil
			}
			return extractors.ExtractNetworkCalls(root, contentBytes, result.Symbols, langID)
		})
		result.NetworkCalls = calls
		statuses = append(statuses, netStatus)
		enabledExtractors = append(enabledExtractors, "network_calls")
		if netStatus.Status == "FAILED" {
			result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("network_calls", fmt.Errorf("%s", netStatus.Message)))
		}
		if len(result.NetworkCalls) > 0 {
			parser.SortNetworkCalls(result.NetworkCalls)
		}
	}

	// --- All tiers: chunks (text-based, no AST needed) ---
	chunks, chunkStatus := runExtractor("chunks", func() []parser.Chunk {
		return extractors.ExtractChunks(content, file.FilePath, result.Symbols, result.Imports, langID)
	})
	result.Chunks = chunks
	statuses = append(statuses, chunkStatus)
	enabledExtractors = append(enabledExtractors, "chunks")
	if chunkStatus.Status == "FAILED" {
		result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("chunks", fmt.Errorf("%s", chunkStatus.Message)))
	}
	if len(result.Chunks) > 0 {
		parser.SortChunks(result.Chunks)
	}

	// --- All tiers: diagnostics (requires AST) ---
	diagnostics, diagStatus := runExtractor("diagnostics", func() []parser.Issue {
		if root == nil {
			return nil
		}
		return extractors.ExtractDiagnostics(root, contentBytes, langID, file.FilePath)
	})
	result.Issues = append(result.Issues, diagnostics...)
	diagStatus.IssueCount = int32(len(diagnostics))
	statuses = append(statuses, diagStatus)
	enabledExtractors = append(enabledExtractors, "diagnostics")
	if diagStatus.Status == "FAILED" {
		result.Issues = append(result.Issues, extractors.CreateExtractionErrorIssue("diagnostics", fmt.Errorf("%s", diagStatus.Message)))
	}
	parser.SortIssues(result.Issues)

	// Close tree immediately after all extractors are done — nodes are no
	// longer referenced.  Without explicit Close, trees accumulate until
	// GC runs; under heavy batch load this causes tree-sitter's C runtime
	// to hit its internal operation limit ("operation limit was hit").
	if tree != nil {
		tree.Close()
	}

	// 8. File facts.
	result.Facts = computeFileFacts(&result)

	// 9. Parser metadata.
	result.Metadata = &parser.ParserMeta{
		ParserVersion:     parserVersion,
		GrammarVersion:    grammarVersion,
		EnabledExtractors: enabledExtractors,
	}
	result.ExtractorStatuses = statuses

	return result
}

// runExtractor runs an extraction function with panic recovery.
// On success: returns (result, OK status).
// On panic:   returns (zero value, FAILED status with panic message).
func runExtractor[T any](name string, fn func() T) (T, parser.ExtractorStatus) {
	var result T
	var panicErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("panic: %v", r)
			}
		}()
		result = fn()
	}()

	if panicErr != nil {
		return result, parser.ExtractorStatus{
			ExtractorName: name,
			Status:        "FAILED",
			Message:       panicErr.Error(),
		}
	}
	return result, parser.ExtractorStatus{
		ExtractorName: name,
		Status:        "OK",
	}
}

// ---------------------------------------------------------------------------
// FileFacts computation
// ---------------------------------------------------------------------------

func computeFileFacts(r *parser.ParsedFileResult) *parser.FileFacts {
	return &parser.FileFacts{
		HasJsx:                 len(r.JsxUsages) > 0,
		HasDefaultExport:       hasDefaultExport(r.Exports),
		HasNamedExports:        hasNamedExports(r.Exports),
		HasTopLevelSideEffects: hasSideEffectImports(r.Imports),
		HasReactHookCalls:      hasHookCalls(r.References),
		HasFetchCalls:          len(r.NetworkCalls) > 0,
		HasClassDeclarations:   hasClasses(r.Symbols),
		HasTests:               hasTestPatterns(r.FilePath),
		HasConfigPatterns:      hasConfigPatterns(r.FilePath),
		JsxRuntime:             detectJsxRuntime(r.Imports, r.Language),
	}
}

func hasDefaultExport(exports []parser.Export) bool {
	for _, e := range exports {
		if e.ExportKind == "DEFAULT" {
			return true
		}
	}
	return false
}

func hasNamedExports(exports []parser.Export) bool {
	for _, e := range exports {
		if e.ExportKind == "NAMED" {
			return true
		}
	}
	return false
}

// hasSideEffectImports detects side-effect-only imports by checking for
// non-JS/TS file extensions in import targets (e.g. CSS, SCSS, LESS, SASS).
func hasSideEffectImports(imports []parser.Import) bool {
	for _, imp := range imports {
		target := strings.ToLower(imp.TargetFilePath)
		if target == "" {
			target = strings.ToLower(imp.ImportName)
		}
		for _, ext := range []string{".css", ".scss", ".less", ".sass", ".styl"} {
			if strings.HasSuffix(target, ext) {
				return true
			}
		}
	}
	return false
}

func hasHookCalls(refs []parser.Reference) bool {
	for _, r := range refs {
		if r.ReferenceKind == "HOOK_USE" {
			return true
		}
	}
	return false
}

func hasClasses(symbols []parser.Symbol) bool {
	for _, s := range symbols {
		if s.Kind == "class" {
			return true
		}
	}
	return false
}

func hasTestPatterns(filePath string) bool {
	base := filepath.Base(filePath)
	lower := strings.ToLower(base)
	if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec.") || strings.Contains(lower, "_spec.") {
		return true
	}
	normalized := filepath.ToSlash(filePath)
	return strings.Contains(normalized, "/__tests__/") ||
		strings.Contains(normalized, "/test/") ||
		strings.Contains(normalized, "/tests/") ||
		strings.Contains(normalized, "/spec/")
}

func hasConfigPatterns(filePath string) bool {
	base := filepath.Base(filePath)
	lower := strings.ToLower(base)
	configNames := []string{
		"package.json", "tsconfig", "webpack.config", "vite.config",
		"babel.config", ".babelrc", ".eslintrc", "jest.config",
		"rollup.config", "next.config", "nuxt.config", "tailwind.config",
		"postcss.config", "prettier", ".prettierrc", "go.mod", "go.sum",
		"cargo.toml", "pyproject.toml", "setup.py", "setup.cfg",
		"gemfile", "rakefile", "makefile", "dockerfile",
		".env", ".gitignore", ".dockerignore",
	}
	for _, name := range configNames {
		if strings.Contains(lower, name) {
			return true
		}
	}
	return false
}

func detectJsxRuntime(imports []parser.Import, langID string) string {
	if langID != "jsx" && langID != "tsx" {
		return ""
	}
	for _, imp := range imports {
		name := strings.ToLower(imp.ImportName)
		// Check preact before react since "preact" contains "react".
		if strings.Contains(name, "preact") {
			return "preact"
		}
		if strings.Contains(name, "react") {
			return "react"
		}
	}
	return "unknown"
}
