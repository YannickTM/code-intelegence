package indexing

import (
	"path"
	"sort"
	"strings"

	"myjungle/backend-worker/internal/parser"
)

// Well-known directory-index filenames across languages, mapping each
// basename to the set of extensions it is valid with.
//
//   - JS/TS:  index.ts, index.tsx, index.js, index.jsx, index.mjs, index.cjs
//   - Python: __init__.py
//   - Rust:   mod.rs
var indexFileNames = map[string]map[string]struct{}{
	"index": {
		".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {},
		".mjs": {}, ".cjs": {},
	},
	"__init__": {".py": {}},
	"mod":      {".rs": {}},
}

// ResolveImportTargets resolves extensionless internal import targets
// (e.g. "src/utils/foo") to actual file paths (e.g. "src/utils/foo.ts")
// by matching against the known file set.
//
// The resolution is language-agnostic: it builds a stem-based lookup from
// all known file paths, so it works for .ts, .py, .rs, .go, .java, etc.
// without hardcoding extension lists.
//
// allPaths optionally provides the full set of file paths in the project
// (including files not in the files slice, e.g. unchanged files during
// incremental indexing). When nil, the lookup is built from files only.
//
// Paths are sorted before registration so that stem collisions are
// resolved deterministically regardless of input order.
//
// goModulePath is the Go module path from go.mod (e.g. "github.com/user/repo").
// It is used to recognise Go-internal imports whose specifier starts with the
// module path. Pass "" when the project is not a Go module or go.mod is
// unavailable.
//
// This mutates the Imports slice of each ParsedFileResult in place.
func ResolveImportTargets(files []parser.ParsedFileResult, allPaths []string, goModulePath string) {
	// Collect all paths to register, deduplicating.
	seen := make(map[string]struct{}, len(files)+len(allPaths))
	paths := make([]string, 0, len(files)+len(allPaths))
	for _, fp := range allPaths {
		if _, ok := seen[fp]; !ok {
			seen[fp] = struct{}{}
			paths = append(paths, fp)
		}
	}
	for _, f := range files {
		if _, ok := seen[f.FilePath]; !ok {
			seen[f.FilePath] = struct{}{}
			paths = append(paths, f.FilePath)
		}
	}

	// Sort so that stem/index collisions resolve deterministically.
	sort.Strings(paths)

	// stemMap: extensionless path → full file path.
	stemMap := make(map[string]string, len(paths))

	// dirIndex: directory path → full file path for index-like files.
	dirIndex := make(map[string]string, len(paths)/4)

	// exact: full file path set for fast exact-match checks.
	exact := make(map[string]struct{}, len(paths))

	// goDir: directory path → first .go file in that directory.
	// Used to resolve Go package imports which reference directories,
	// not individual files (e.g. "internal/handler" → directory containing
	// handler.go and routes.go).
	goDir := make(map[string]string, len(paths)/4)

	for _, fp := range paths {
		registerPath(fp, exact, stemMap, dirIndex, goDir)
	}

	// Resolve each INTERNAL import target.
	for i := range files {
		for j := range files[i].Imports {
			imp := &files[i].Imports[j]

			if imp.ImportType != "INTERNAL" || imp.TargetFilePath == "" {
				continue
			}

			// Already an exact match — no resolution needed.
			if _, ok := exact[imp.TargetFilePath]; ok {
				continue
			}

			// Try stem match: "src/foo" → "src/foo.ts"
			if resolved, ok := stemMap[imp.TargetFilePath]; ok {
				imp.TargetFilePath = resolved
				continue
			}

			// Try directory index: "src/lib" → "src/lib/index.ts"
			if resolved, ok := dirIndex[imp.TargetFilePath]; ok {
				imp.TargetFilePath = resolved
			}
		}
	}

	// Post-hoc: try to reclassify EXTERNAL imports as INTERNAL by matching
	// against the project file set using language-specific path conversion.
	// This handles Go, Java, Kotlin, C#, PHP, and Python absolute imports
	// which lack InternalImportPatterns.
	for i := range files {
		lang := files[i].Language
		for j := range files[i].Imports {
			imp := &files[i].Imports[j]
			if imp.ImportType != "EXTERNAL" || imp.TargetFilePath != "" {
				continue
			}

			candidate := convertImportToPath(imp.ImportName, lang, goModulePath)
			if candidate == "" {
				continue
			}

			if _, ok := exact[candidate]; ok {
				imp.TargetFilePath = candidate
				imp.ImportType = "INTERNAL"
				continue
			}
			if resolved, ok := stemMap[candidate]; ok {
				imp.TargetFilePath = resolved
				imp.ImportType = "INTERNAL"
				continue
			}
			if resolved, ok := dirIndex[candidate]; ok {
				imp.TargetFilePath = resolved
				imp.ImportType = "INTERNAL"
				continue
			}

			// Go imports reference packages (directories), not files.
			// If the candidate is a directory containing .go files, resolve
			// to the first .go file in that directory.
			if lang == "go" {
				if resolved, ok := goDir[candidate]; ok {
					imp.TargetFilePath = resolved
					imp.ImportType = "INTERNAL"
					continue
				}
			}
		}
	}
}

// convertImportToPath converts a language-specific import specifier to a
// candidate file path that can be matched against the project file set.
// Returns "" if conversion is not applicable for the language or import.
func convertImportToPath(importName, language, goModulePath string) string {
	switch language {
	case "go":
		if goModulePath == "" {
			return ""
		}
		// Require exact prefix + "/" to avoid false positives
		// (e.g. "myapp" must not match "myapp-fork/pkg").
		if !strings.HasPrefix(importName, goModulePath+"/") {
			return ""
		}
		return strings.TrimPrefix(importName, goModulePath+"/")

	case "java", "kotlin":
		return strings.ReplaceAll(importName, ".", "/")

	case "csharp":
		return strings.ReplaceAll(importName, ".", "/")

	case "php":
		// Trim leading \ from fully-qualified names (e.g. \App\Models\User).
		name := strings.TrimPrefix(importName, `\`)
		return strings.ReplaceAll(name, `\`, "/")

	case "python":
		if strings.HasPrefix(importName, ".") {
			return "" // relative imports handled in resolvePath
		}
		return strings.ReplaceAll(importName, ".", "/")
	}
	return ""
}

// registerPath adds a file path to the lookup maps used for import resolution.
func registerPath(fp string, exact map[string]struct{}, stemMap map[string]string, dirIndex map[string]string, goDir map[string]string) {
	exact[fp] = struct{}{}

	// Primary stem: strip the final extension.
	// "src/foo.ts" → "src/foo", "src/types.d.ts" → "src/types.d"
	primaryStem := stripExt(fp)
	if primaryStem != fp {
		// Implementation files overwrite declaration files;
		// otherwise first-wins avoids ambiguity (e.g. foo.ts vs foo.js).
		if existing, exists := stemMap[primaryStem]; !exists || isDTS(existing) {
			stemMap[primaryStem] = fp
		}
	}

	// Secondary stem for .d.ts files only: strip the full compound
	// extension so "src/types.d.ts" also registers stem "src/types".
	// Implementation files (.ts, .js, etc.) always take priority:
	// we only register the secondary stem if no entry exists yet.
	if isDTS(fp) {
		secondaryStem := stripExt(primaryStem)
		if secondaryStem != primaryStem {
			if _, exists := stemMap[secondaryStem]; !exists {
				stemMap[secondaryStem] = fp
			}
		}
	}

	// Track directories containing .go files for Go package resolution.
	// Go imports reference packages (directories), so we need to know which
	// directories contain Go source files. First file wins (paths are sorted).
	// Must run before the index-file check below, which may return early.
	if strings.HasSuffix(fp, ".go") {
		dir := path.Dir(fp)
		if _, exists := goDir[dir]; !exists {
			goDir[dir] = fp
		}
	}

	// Check if this is a directory index file.
	// Use single-stripped basename: "index.ts" → "index", "mod.rs" → "mod".
	// For .d.ts: "index.d.ts" → stripExt → "index.d" → stripExt → "index".
	baseStem := stripExt(path.Base(fp))
	if isDTS(fp) {
		baseStem = stripExt(baseStem)
	}
	if allowedExts, isIndex := indexFileNames[baseStem]; isIndex {
		ext := path.Ext(fp)
		if _, ok := allowedExts[ext]; !ok {
			return
		}
		dir := path.Dir(fp)
		// Implementation index files overwrite declaration index files.
		if existing, exists := dirIndex[dir]; !exists || isDTS(existing) {
			dirIndex[dir] = fp
		}
	}

	// Track directories containing .go files for Go package resolution.
	// Go imports reference packages (directories), so we need to know which
	// directories contain Go source files. First file wins (paths are sorted).
	if strings.HasSuffix(fp, ".go") {
		dir := path.Dir(fp)
		if _, exists := goDir[dir]; !exists {
			goDir[dir] = fp
		}
	}
}

// isDTS reports whether the file path ends with ".d.ts".
func isDTS(fp string) bool {
	return strings.HasSuffix(fp, ".d.ts")
}

// stripExt removes the final file extension from a path.
// "src/foo.ts" → "src/foo", "src/foo" → "src/foo" (unchanged).
func stripExt(p string) string {
	ext := path.Ext(p)
	if ext == "" {
		return p
	}
	return p[:len(p)-len(ext)]
}
