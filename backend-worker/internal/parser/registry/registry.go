package registry

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Tier represents the extraction depth for a language.
type Tier int

const (
	Tier1 Tier = 1 // Full extraction: symbols + imports + exports + references + chunks + diagnostics
	Tier2 Tier = 2 // Partial extraction: symbols + imports + chunks + diagnostics
	Tier3 Tier = 3 // Structural only: chunks + diagnostics, minimal symbols
)

// DocCommentStyle identifies the documentation comment convention for a language.
type DocCommentStyle string

const (
	DocJSDoc       DocCommentStyle = "jsdoc"           // /** ... */
	DocDocstring   DocCommentStyle = "docstring"       // """ ... """
	DocTripleSlash DocCommentStyle = "slashslashslash" // /// ...
	DocXMLDoc      DocCommentStyle = "xmldoc"          // /// <summary>
	DocHash        DocCommentStyle = "hash"            // # ...
	DocDoxygen     DocCommentStyle = "doxygen"         // /** ... */ (C/C++ style)
	DocNone        DocCommentStyle = "none"
)

// ExportStrategy describes how a language determines symbol visibility.
type ExportStrategy struct {
	Type          string // "keyword", "convention", "prefix", "all_public", "none"
	Keyword       string // for "keyword" type: "pub", "public", "export"
	Convention    string // for "convention" type: "uppercase_first_letter"
	PrivatePrefix string // for "prefix" type: "_"
}

// LanguageConfig holds all per-language configuration that extractors need.
type LanguageConfig struct {
	ID        string
	Tier      Tier
	Extensions []string
	Basenames  []string

	// Symbol extraction
	SymbolNodeTypes map[string]string // AST node type -> symbol kind
	DocCommentStyle DocCommentStyle

	// Import extraction
	ImportNodeTypes        []string
	StdlibModules          map[string]bool
	StdlibPrefixes         []string
	InternalImportPatterns []string

	// Export detection
	Export ExportStrategy

	// Reference extraction
	BuiltinTypes map[string]bool

	// Chunking
	TestFilePatterns   []string
	ConfigFilePatterns []string
	TestBlockPatterns  []string

	// Diagnostics
	NestingNodeTypes   []string
	HasExplicitExports bool
}

// extensionMap maps file extensions (lowercase, with dot) to language IDs.
var extensionMap = map[string]string{
	".ts":         "typescript",
	".tsx":        "tsx",
	".js":         "javascript",
	".mjs":        "javascript",
	".cjs":        "javascript",
	".jsx":        "jsx",
	".py":         "python",
	".pyw":        "python",
	".pyi":        "python",
	".go":         "go",
	".rs":         "rust",
	".java":       "java",
	".kt":         "kotlin",
	".kts":        "kotlin",
	".c":          "c",
	".h":          "cpp",
	".cpp":        "cpp",
	".cc":         "cpp",
	".cxx":        "cpp",
	".hpp":        "cpp",
	".hxx":        "cpp",
	".cs":         "csharp",
	".swift":      "swift",
	".rb":         "ruby",
	".rake":       "ruby",
	".gemspec":    "ruby",
	".php":        "php",
	".html":       "html",
	".htm":        "html",
	".css":        "css",
	".scss":       "scss",
	".json":       "json",
	".jsonc":      "json",
	".yaml":       "yaml",
	".yml":        "yaml",
	".toml":       "toml",
	".md":         "markdown",
	".markdown":   "markdown",
	".mdx":        "markdown",
	".xml":        "xml",
	".svg":        "xml",
	".xsl":        "xml",
	".xsd":        "xml",
	".plist":      "xml",
	".sh":         "bash",
	".bash":       "bash",
	".zsh":        "bash",
	".dockerfile": "dockerfile",
	".sql":        "sql",
	".graphql":    "graphql",
	".gql":        "graphql",
	".tf":         "hcl",
	".tfvars":     "hcl",
	".tofu":       "hcl",
}

// basenameMap maps exact filenames to language IDs.
var basenameMap = map[string]string{
	"Dockerfile":    "dockerfile",
	"Gemfile":       "ruby",
	"Rakefile":      "ruby",
	"Makefile":      "bash",
	".bashrc":       "bash",
	".zshrc":        "bash",
	".bash_profile": "bash",
	".profile":      "bash",
}

// GetLanguageConfig returns the configuration for the given language ID.
func GetLanguageConfig(langID string) (*LanguageConfig, bool) {
	cfg, ok := languages[langID]
	if !ok {
		return nil, false
	}
	return &cfg, true
}

// GetLanguageByExtension returns the language ID for the given file extension.
// The extension should include the leading dot (e.g. ".go").
func GetLanguageByExtension(ext string) (string, bool) {
	langID, ok := extensionMap[strings.ToLower(ext)]
	return langID, ok
}

// GetLanguageByBasename returns the language ID for the given filename.
func GetLanguageByBasename(basename string) (string, bool) {
	langID, ok := basenameMap[basename]
	return langID, ok
}

// AllLanguageIDs returns a sorted list of all registered language IDs.
func AllLanguageIDs() []string {
	ids := make([]string, 0, len(languages))
	for id := range languages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GetTier returns the extraction tier for the given language ID.
// Returns 0 if the language is not found.
func GetTier(langID string) Tier {
	cfg, ok := languages[langID]
	if !ok {
		return 0
	}
	return cfg.Tier
}

// DetectLanguage determines the language ID for a file path.
// It checks basenames first, then the Dockerfile.* prefix pattern, then extensions.
func DetectLanguage(filePath string) (string, error) {
	basename := filepath.Base(filePath)

	// 1. Check exact basename match.
	if langID, ok := basenameMap[basename]; ok {
		return langID, nil
	}

	// 2. Check Dockerfile.* prefix pattern.
	if strings.HasPrefix(basename, "Dockerfile.") || strings.HasPrefix(basename, "dockerfile.") {
		return "dockerfile", nil
	}

	// 3. Check extension map.
	ext := strings.ToLower(filepath.Ext(filePath))
	if langID, ok := extensionMap[ext]; ok {
		return langID, nil
	}

	return "", fmt.Errorf("unsupported language for file: %s", basename)
}
