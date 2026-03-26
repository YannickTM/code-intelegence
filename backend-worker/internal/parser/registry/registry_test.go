package registry

import (
	"testing"
)

func TestDetectLanguage_Extensions(t *testing.T) {
	tests := []struct {
		path   string
		wantID string
	}{
		// TypeScript / JavaScript family
		{"src/app.ts", "typescript"},
		{"components/Button.tsx", "tsx"},
		{"index.js", "javascript"},
		{"lib/utils.mjs", "javascript"},
		{"lib/utils.cjs", "javascript"},
		{"components/App.jsx", "jsx"},

		// Python
		{"main.py", "python"},
		{"script.pyw", "python"},
		{"types.pyi", "python"},

		// Go
		{"main.go", "go"},

		// Rust
		{"lib.rs", "rust"},

		// Java / Kotlin
		{"Main.java", "java"},
		{"Main.kt", "kotlin"},
		{"build.gradle.kts", "kotlin"},

		// C / C++
		{"main.c", "c"},
		{"utils.h", "cpp"},
		{"main.cpp", "cpp"},
		{"main.cc", "cpp"},
		{"main.cxx", "cpp"},
		{"utils.hpp", "cpp"},
		{"utils.hxx", "cpp"},

		// C#
		{"Program.cs", "csharp"},

		// Swift
		{"App.swift", "swift"},

		// Ruby
		{"app.rb", "ruby"},
		{"deploy.rake", "ruby"},
		{"my_gem.gemspec", "ruby"},

		// PHP
		{"index.php", "php"},

		// Tier 2
		{"script.sh", "bash"},
		{"script.bash", "bash"},
		{"script.zsh", "bash"},
		{"migrations/001.sql", "sql"},
		{"schema.graphql", "graphql"},
		{"schema.gql", "graphql"},
		{"app.dockerfile", "dockerfile"},

		// HCL
		{"main.tf", "hcl"},
		{"vars.tfvars", "hcl"},
		{"main.tofu", "hcl"},

		// Tier 3
		{"index.html", "html"},
		{"index.htm", "html"},
		{"style.css", "css"},
		{"style.scss", "scss"},
		{"data.json", "json"},
		{"tsconfig.jsonc", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"Cargo.toml", "toml"},
		{"README.md", "markdown"},
		{"notes.markdown", "markdown"},
		{"page.mdx", "markdown"},
		{"layout.xml", "xml"},
		{"icon.svg", "xml"},
		{"transform.xsl", "xml"},
		{"schema.xsd", "xml"},
		{"Info.plist", "xml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := DetectLanguage(tt.path)
			if err != nil {
				t.Fatalf("DetectLanguage(%q) error: %v", tt.path, err)
			}
			if got != tt.wantID {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.wantID)
			}
		})
	}
}

func TestDetectLanguage_Basenames(t *testing.T) {
	tests := []struct {
		path   string
		wantID string
	}{
		{"Dockerfile", "dockerfile"},
		{"/app/Dockerfile", "dockerfile"},
		{"Gemfile", "ruby"},
		{"Rakefile", "ruby"},
		{"Makefile", "bash"},
		{".bashrc", "bash"},
		{".zshrc", "bash"},
		{".bash_profile", "bash"},
		{".profile", "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := DetectLanguage(tt.path)
			if err != nil {
				t.Fatalf("DetectLanguage(%q) error: %v", tt.path, err)
			}
			if got != tt.wantID {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.wantID)
			}
		})
	}
}

func TestDetectLanguage_DockerfilePrefix(t *testing.T) {
	tests := []string{
		"Dockerfile.dev",
		"Dockerfile.prod",
		"Dockerfile.staging",
		"dockerfile.test",
		"/deploy/Dockerfile.worker",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			got, err := DetectLanguage(path)
			if err != nil {
				t.Fatalf("DetectLanguage(%q) error: %v", path, err)
			}
			if got != "dockerfile" {
				t.Errorf("DetectLanguage(%q) = %q, want %q", path, got, "dockerfile")
			}
		})
	}
}

func TestDetectLanguage_UnsupportedExtension(t *testing.T) {
	unsupported := []string{
		"file.xyz",
		"binary.exe",
		"image.png",
		"archive.tar.gz",
	}

	for _, path := range unsupported {
		t.Run(path, func(t *testing.T) {
			_, err := DetectLanguage(path)
			if err == nil {
				t.Errorf("DetectLanguage(%q) expected error for unsupported extension", path)
			}
		})
	}
}

func TestGetLanguageConfig_AllLanguages(t *testing.T) {
	allIDs := AllLanguageIDs()
	for _, id := range allIDs {
		t.Run(id, func(t *testing.T) {
			cfg, ok := GetLanguageConfig(id)
			if !ok {
				t.Fatalf("GetLanguageConfig(%q) returned false", id)
			}
			if cfg.ID != id {
				t.Errorf("config.ID = %q, want %q", cfg.ID, id)
			}
			if cfg.Tier < Tier1 || cfg.Tier > Tier3 {
				t.Errorf("config.Tier = %d, want 1-3", cfg.Tier)
			}
			if len(cfg.SymbolNodeTypes) == 0 {
				t.Errorf("%s: SymbolNodeTypes should not be empty", id)
			}
		})
	}
}

func TestGetLanguageConfig_NotFound(t *testing.T) {
	_, ok := GetLanguageConfig("nonexistent")
	if ok {
		t.Error("GetLanguageConfig(\"nonexistent\") should return false")
	}
}

func TestGetTier_Classification(t *testing.T) {
	tier1 := []string{
		"javascript", "typescript", "jsx", "tsx", "python", "go", "rust",
		"java", "kotlin", "c", "cpp", "csharp", "swift", "ruby", "php",
	}
	tier2 := []string{"bash", "sql", "graphql", "dockerfile", "hcl"}
	tier3 := []string{"html", "css", "scss", "json", "yaml", "toml", "markdown", "xml"}

	for _, id := range tier1 {
		if got := GetTier(id); got != Tier1 {
			t.Errorf("GetTier(%q) = %d, want %d (Tier1)", id, got, Tier1)
		}
	}
	for _, id := range tier2 {
		if got := GetTier(id); got != Tier2 {
			t.Errorf("GetTier(%q) = %d, want %d (Tier2)", id, got, Tier2)
		}
	}
	for _, id := range tier3 {
		if got := GetTier(id); got != Tier3 {
			t.Errorf("GetTier(%q) = %d, want %d (Tier3)", id, got, Tier3)
		}
	}
}

func TestGetTier_NotFound(t *testing.T) {
	if got := GetTier("nonexistent"); got != 0 {
		t.Errorf("GetTier(\"nonexistent\") = %d, want 0", got)
	}
}

func TestAllLanguageIDs_Count(t *testing.T) {
	ids := AllLanguageIDs()
	// 15 Tier 1 + 5 Tier 2 (incl. HCL) + 8 Tier 3 = 28
	if len(ids) != 28 {
		t.Errorf("AllLanguageIDs() returned %d languages, want 28: %v", len(ids), ids)
	}
}

func TestAllLanguageIDs_Sorted(t *testing.T) {
	ids := AllLanguageIDs()
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[i-1] {
			t.Fatalf("AllLanguageIDs() not sorted: %q comes after %q", ids[i], ids[i-1])
		}
	}
}

func TestTier1_CompleteConfig(t *testing.T) {
	tier1 := []string{
		"javascript", "typescript", "jsx", "tsx", "python", "go", "rust",
		"java", "kotlin", "c", "cpp", "csharp", "swift", "ruby", "php",
	}

	for _, id := range tier1 {
		t.Run(id, func(t *testing.T) {
			cfg, ok := GetLanguageConfig(id)
			if !ok {
				t.Fatalf("language %q not found", id)
			}
			if len(cfg.SymbolNodeTypes) == 0 {
				t.Errorf("%s: SymbolNodeTypes is empty", id)
			}
			if len(cfg.ImportNodeTypes) == 0 {
				t.Errorf("%s: ImportNodeTypes is empty", id)
			}
			if len(cfg.BuiltinTypes) == 0 {
				t.Errorf("%s: BuiltinTypes is empty", id)
			}
			if cfg.Export.Type == "" {
				t.Errorf("%s: Export.Type is empty", id)
			}
			if len(cfg.NestingNodeTypes) == 0 {
				t.Errorf("%s: NestingNodeTypes is empty", id)
			}
		})
	}
}

func TestTier2_PartialConfig(t *testing.T) {
	tier2 := []string{"bash", "sql", "graphql", "dockerfile", "hcl"}

	for _, id := range tier2 {
		t.Run(id, func(t *testing.T) {
			cfg, ok := GetLanguageConfig(id)
			if !ok {
				t.Fatalf("language %q not found", id)
			}
			if len(cfg.SymbolNodeTypes) == 0 {
				t.Errorf("%s: SymbolNodeTypes is empty", id)
			}
		})
	}
}

func TestHasExplicitExports(t *testing.T) {
	tests := []struct {
		langID string
		want   bool
	}{
		{"javascript", true},
		{"typescript", true},
		{"jsx", true},
		{"tsx", true},
		{"go", true},
		{"rust", true},
		{"java", true},
		{"csharp", true},
		{"swift", true},
		{"php", true},
		{"python", false},
		{"kotlin", false},
		{"c", false},
		{"cpp", false},
		{"ruby", false},
		{"bash", false},
		{"sql", false},
		{"hcl", false},
	}

	for _, tt := range tests {
		t.Run(tt.langID, func(t *testing.T) {
			cfg, ok := GetLanguageConfig(tt.langID)
			if !ok {
				t.Fatalf("language %q not found", tt.langID)
			}
			if cfg.HasExplicitExports != tt.want {
				t.Errorf("%s: HasExplicitExports = %v, want %v", tt.langID, cfg.HasExplicitExports, tt.want)
			}
		})
	}
}

func TestGetLanguageByExtension(t *testing.T) {
	langID, ok := GetLanguageByExtension(".go")
	if !ok || langID != "go" {
		t.Errorf("GetLanguageByExtension(\".go\") = (%q, %v), want (\"go\", true)", langID, ok)
	}

	// Case-insensitive.
	langID, ok = GetLanguageByExtension(".GO")
	if !ok || langID != "go" {
		t.Errorf("GetLanguageByExtension(\".GO\") = (%q, %v), want (\"go\", true)", langID, ok)
	}

	_, ok = GetLanguageByExtension(".xyz")
	if ok {
		t.Error("GetLanguageByExtension(\".xyz\") should return false")
	}
}

func TestGetLanguageByBasename(t *testing.T) {
	langID, ok := GetLanguageByBasename("Dockerfile")
	if !ok || langID != "dockerfile" {
		t.Errorf("GetLanguageByBasename(\"Dockerfile\") = (%q, %v), want (\"dockerfile\", true)", langID, ok)
	}

	_, ok = GetLanguageByBasename("unknown")
	if ok {
		t.Error("GetLanguageByBasename(\"unknown\") should return false")
	}
}

func TestExtensionMapCoverage(t *testing.T) {
	// Verify all extensions listed in each language config are in the extensionMap.
	for _, cfg := range languages {
		for _, ext := range cfg.Extensions {
			got, ok := extensionMap[ext]
			if !ok {
				t.Errorf("extension %q (from %s) not in extensionMap", ext, cfg.ID)
				continue
			}
			if got != cfg.ID {
				t.Errorf("extensionMap[%q] = %q, want %q", ext, got, cfg.ID)
			}
		}
	}
}

func TestBasenameMapCoverage(t *testing.T) {
	// Verify all basenames listed in each language config are in the basenameMap.
	for _, cfg := range languages {
		for _, bn := range cfg.Basenames {
			got, ok := basenameMap[bn]
			if !ok {
				t.Errorf("basename %q (from %s) not in basenameMap", bn, cfg.ID)
				continue
			}
			if got != cfg.ID {
				t.Errorf("basenameMap[%q] = %q, want %q", bn, got, cfg.ID)
			}
		}
	}
}

func TestDocCommentStyles(t *testing.T) {
	tests := []struct {
		langID string
		want   DocCommentStyle
	}{
		{"typescript", DocJSDoc},
		{"python", DocDocstring},
		{"go", DocTripleSlash},
		{"rust", DocTripleSlash},
		{"csharp", DocXMLDoc},
		{"c", DocDoxygen},
		{"cpp", DocDoxygen},
		{"ruby", DocHash},
		{"sql", DocNone},
	}

	for _, tt := range tests {
		t.Run(tt.langID, func(t *testing.T) {
			cfg, ok := GetLanguageConfig(tt.langID)
			if !ok {
				t.Fatalf("language %q not found", tt.langID)
			}
			if cfg.DocCommentStyle != tt.want {
				t.Errorf("%s: DocCommentStyle = %q, want %q", tt.langID, cfg.DocCommentStyle, tt.want)
			}
		})
	}
}

func TestExportStrategies(t *testing.T) {
	tests := []struct {
		langID   string
		wantType string
	}{
		{"typescript", "keyword"},
		{"python", "prefix"},
		{"go", "convention"},
		{"kotlin", "all_public"},
		{"bash", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.langID, func(t *testing.T) {
			cfg, ok := GetLanguageConfig(tt.langID)
			if !ok {
				t.Fatalf("language %q not found", tt.langID)
			}
			if cfg.Export.Type != tt.wantType {
				t.Errorf("%s: Export.Type = %q, want %q", tt.langID, cfg.Export.Type, tt.wantType)
			}
		})
	}
}
