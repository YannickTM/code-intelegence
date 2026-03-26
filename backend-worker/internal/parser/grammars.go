package parser

import (
	"sort"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	cGrammar "github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	tree_sitter_markdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

// grammarRegistry maps language IDs (from the registry package) to tree-sitter
// grammars. Languages without a Go binding (scss, graphql, xml, json) are
// absent and fall back to Tier 3 structural chunking with no tree-sitter parse.
var grammarRegistry = map[string]*sitter.Language{
	"typescript":  typescript.GetLanguage(),
	"tsx":         tsx.GetLanguage(),
	"javascript":  javascript.GetLanguage(),
	"jsx":         tsx.GetLanguage(), // JSX uses the TSX grammar (superset)
	"python":      python.GetLanguage(),
	"go":          golang.GetLanguage(),
	"rust":        rust.GetLanguage(),
	"java":        java.GetLanguage(),
	"kotlin":      kotlin.GetLanguage(),
	"c":           cGrammar.GetLanguage(),
	"cpp":         cpp.GetLanguage(),
	"csharp":      csharp.GetLanguage(),
	"swift":       swift.GetLanguage(),
	"ruby":        ruby.GetLanguage(),
	"php":         php.GetLanguage(),
	"bash":        bash.GetLanguage(),
	"sql":         sql.GetLanguage(),
	"dockerfile":  dockerfile.GetLanguage(),
	"hcl":         hcl.GetLanguage(),
	"html":        html.GetLanguage(),
	"css":         css.GetLanguage(),
	"yaml":        yaml.GetLanguage(),
	"toml":        toml.GetLanguage(),
	"markdown":    tree_sitter_markdown.GetLanguage(),
}

// GetGrammar returns the tree-sitter grammar for the given language ID.
// Returns nil if no grammar is available (language uses Tier 3 fallback).
func GetGrammar(langID string) *sitter.Language {
	return grammarRegistry[langID]
}

// HasGrammar reports whether a tree-sitter grammar is available for the language.
func HasGrammar(langID string) bool {
	_, ok := grammarRegistry[langID]
	return ok
}

// GrammarLanguageIDs returns a sorted list of language IDs that have grammars.
func GrammarLanguageIDs() []string {
	ids := make([]string, 0, len(grammarRegistry))
	for id := range grammarRegistry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
