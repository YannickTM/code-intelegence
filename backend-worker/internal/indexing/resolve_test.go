package indexing

import (
	"testing"

	"myjungle/backend-worker/internal/parser"
)

func TestResolveImportTargets(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/index.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/utils", ImportName: "./utils"},
			{ImportType: "INTERNAL", TargetFilePath: "src/components/Button", ImportName: "./components/Button"},
			{ImportType: "INTERNAL", TargetFilePath: "src/lib", ImportName: "./lib"},
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "express", PackageName: "express"},
			{ImportType: "STDLIB", TargetFilePath: "", ImportName: "node:path"},
			{ImportType: "INTERNAL", TargetFilePath: "src/already.ts", ImportName: "./already.ts"},
		}},
		{FilePath: "src/utils.ts"},
		{FilePath: "src/components/Button.tsx"},
		{FilePath: "src/lib/index.ts"},
		{FilePath: "src/already.ts"},
	}

	ResolveImportTargets(files, nil, "")

	imports := files[0].Imports

	tests := []struct {
		name     string
		idx      int
		wantPath string
	}{
		{"resolve .ts extension", 0, "src/utils.ts"},
		{"resolve .tsx extension", 1, "src/components/Button.tsx"},
		{"resolve index file", 2, "src/lib/index.ts"},
		{"skip external", 3, ""},
		{"skip stdlib", 4, ""},
		{"already has extension", 5, "src/already.ts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imports[tt.idx].TargetFilePath
			if got != tt.wantPath {
				t.Errorf("import[%d] TargetFilePath = %q, want %q", tt.idx, got, tt.wantPath)
			}
		})
	}
}

func TestResolveImportTargets_Python(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "app/main.py", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "app/utils", ImportName: "app.utils"},
			{ImportType: "INTERNAL", TargetFilePath: "app/models", ImportName: "app.models"},
		}},
		{FilePath: "app/utils.py"},
		{FilePath: "app/models/__init__.py"},
	}

	ResolveImportTargets(files, nil, "")

	imports := files[0].Imports

	if got := imports[0].TargetFilePath; got != "app/utils.py" {
		t.Errorf("Python .py resolution: got %q, want %q", got, "app/utils.py")
	}
	if got := imports[1].TargetFilePath; got != "app/models/__init__.py" {
		t.Errorf("Python __init__.py resolution: got %q, want %q", got, "app/models/__init__.py")
	}
}

func TestResolveImportTargets_Rust(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/main.rs", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/utils", ImportName: "crate::utils"},
			{ImportType: "INTERNAL", TargetFilePath: "src/handlers", ImportName: "crate::handlers"},
		}},
		{FilePath: "src/utils.rs"},
		{FilePath: "src/handlers/mod.rs"},
	}

	ResolveImportTargets(files, nil, "")

	imports := files[0].Imports

	if got := imports[0].TargetFilePath; got != "src/utils.rs" {
		t.Errorf("Rust .rs resolution: got %q, want %q", got, "src/utils.rs")
	}
	if got := imports[1].TargetFilePath; got != "src/handlers/mod.rs" {
		t.Errorf("Rust mod.rs resolution: got %q, want %q", got, "src/handlers/mod.rs")
	}
}

func TestResolveImportTargets_Go(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "cmd/server/main.go", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "internal/handler", ImportName: "myapp/internal/handler"},
		}},
		{FilePath: "internal/handler.go"},
	}

	ResolveImportTargets(files, nil, "")

	if got := files[0].Imports[0].TargetFilePath; got != "internal/handler.go" {
		t.Errorf("Go .go resolution: got %q, want %q", got, "internal/handler.go")
	}
}

func TestResolveImportTargets_NoMatchKeepsOriginal(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/index.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/nonexistent", ImportName: "./nonexistent"},
		}},
	}

	ResolveImportTargets(files, nil, "")

	got := files[0].Imports[0].TargetFilePath
	if got != "src/nonexistent" {
		t.Errorf("unresolvable import should keep original path, got %q", got)
	}
}

func TestResolveImportTargets_ExactMatchNoChange(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/index.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/utils.ts", ImportName: "./utils.ts"},
		}},
		{FilePath: "src/utils.ts"},
	}

	ResolveImportTargets(files, nil, "")

	got := files[0].Imports[0].TargetFilePath
	if got != "src/utils.ts" {
		t.Errorf("exact match should stay unchanged, got %q", got)
	}
}

func TestResolveImportTargets_DTS(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/app.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/types", ImportName: "./types"},
			{ImportType: "INTERNAL", TargetFilePath: "src/lib", ImportName: "./lib"},
		}},
		{FilePath: "src/types.d.ts"},
		{FilePath: "src/lib/index.d.ts"},
	}

	ResolveImportTargets(files, nil, "")

	imports := files[0].Imports

	if got := imports[0].TargetFilePath; got != "src/types.d.ts" {
		t.Errorf(".d.ts stem resolution: got %q, want %q", got, "src/types.d.ts")
	}
	if got := imports[1].TargetFilePath; got != "src/lib/index.d.ts" {
		t.Errorf("index.d.ts directory resolution: got %q, want %q", got, "src/lib/index.d.ts")
	}
}

func TestResolveImportTargets_DTS_PrefersTSOverDTS(t *testing.T) {
	// When both foo.ts and foo.d.ts exist, foo.ts should win
	// regardless of file order.
	t.Run("ts first", func(t *testing.T) {
		files := []parser.ParsedFileResult{
			{FilePath: "src/app.ts", Imports: []parser.Import{
				{ImportType: "INTERNAL", TargetFilePath: "src/utils", ImportName: "./utils"},
			}},
			{FilePath: "src/utils.ts"},
			{FilePath: "src/utils.d.ts"},
		}
		ResolveImportTargets(files, nil, "")
		if got := files[0].Imports[0].TargetFilePath; got != "src/utils.ts" {
			t.Errorf("should prefer .ts over .d.ts: got %q, want %q", got, "src/utils.ts")
		}
	})

	t.Run("d.ts first", func(t *testing.T) {
		files := []parser.ParsedFileResult{
			{FilePath: "src/app.ts", Imports: []parser.Import{
				{ImportType: "INTERNAL", TargetFilePath: "src/utils", ImportName: "./utils"},
			}},
			{FilePath: "src/utils.d.ts"},
			{FilePath: "src/utils.ts"},
		}
		ResolveImportTargets(files, nil, "")
		if got := files[0].Imports[0].TargetFilePath; got != "src/utils.ts" {
			t.Errorf("should prefer .ts over .d.ts even when d.ts appears first: got %q, want %q", got, "src/utils.ts")
		}
	})
}

func TestResolveImportTargets_IndexTestNotTreatedAsIndex(t *testing.T) {
	// index.test.ts should NOT be treated as a directory index file.
	files := []parser.ParsedFileResult{
		{FilePath: "src/app.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/lib", ImportName: "./lib"},
		}},
		{FilePath: "src/lib/index.test.ts"},
	}

	ResolveImportTargets(files, nil, "")

	if got := files[0].Imports[0].TargetFilePath; got != "src/lib" {
		t.Errorf("index.test.ts should not be a directory index: got %q, want %q", got, "src/lib")
	}
}

func TestResolveImportTargets_ModTSNotDirIndex(t *testing.T) {
	// mod.ts should NOT be treated as a directory index — mod is Rust-only.
	files := []parser.ParsedFileResult{
		{FilePath: "src/app.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/lib", ImportName: "./lib"},
		}},
		{FilePath: "src/lib/mod.ts"},
	}

	ResolveImportTargets(files, nil, "")

	if got := files[0].Imports[0].TargetFilePath; got != "src/lib" {
		t.Errorf("mod.ts should not be a directory index: got %q, want %q", got, "src/lib")
	}
}

func TestResolveImportTargets_IndexDTSUsedAsDirIndex(t *testing.T) {
	// index.d.ts IS a valid directory index, and implementation
	// index.ts should win over index.d.ts when both exist.
	t.Run("only index.d.ts", func(t *testing.T) {
		files := []parser.ParsedFileResult{
			{FilePath: "src/app.ts", Imports: []parser.Import{
				{ImportType: "INTERNAL", TargetFilePath: "src/types", ImportName: "./types"},
			}},
			{FilePath: "src/types/index.d.ts"},
		}
		ResolveImportTargets(files, nil, "")
		if got := files[0].Imports[0].TargetFilePath; got != "src/types/index.d.ts" {
			t.Errorf("should resolve to index.d.ts: got %q", got)
		}
	})

	t.Run("index.d.ts before index.ts", func(t *testing.T) {
		files := []parser.ParsedFileResult{
			{FilePath: "src/app.ts", Imports: []parser.Import{
				{ImportType: "INTERNAL", TargetFilePath: "src/types", ImportName: "./types"},
			}},
			{FilePath: "src/types/index.d.ts"},
			{FilePath: "src/types/index.ts"},
		}
		ResolveImportTargets(files, nil, "")
		if got := files[0].Imports[0].TargetFilePath; got != "src/types/index.ts" {
			t.Errorf("should prefer index.ts over index.d.ts: got %q", got)
		}
	})
}

func TestResolveImportTargets_IndexNonSourceNotDirIndex(t *testing.T) {
	// Non-source index files (index.css, index.html, index.json, index.png)
	// must NOT be treated as directory index files.
	for _, ext := range []string{".css", ".html", ".json", ".png", ".svg", ".md"} {
		t.Run("index"+ext, func(t *testing.T) {
			files := []parser.ParsedFileResult{
				{FilePath: "src/app.ts", Imports: []parser.Import{
					{ImportType: "INTERNAL", TargetFilePath: "src/components", ImportName: "./components"},
				}},
				{FilePath: "src/components/index" + ext},
			}
			ResolveImportTargets(files, nil, "")
			if got := files[0].Imports[0].TargetFilePath; got != "src/components" {
				t.Errorf("index%s should not be a directory index: got %q", ext, got)
			}
		})
	}
}

func TestResolveImportTargets_DeterministicWithMapOrder(t *testing.T) {
	// Simulate incremental indexing where allPaths comes from map iteration.
	// Include competing candidates (utils.ts vs utils.js) that share the
	// same stem, so non-deterministic registration order would produce
	// different results without the sort.
	var firstResult string
	for run := 0; run < 50; run++ {
		files := []parser.ParsedFileResult{
			{FilePath: "src/app.ts", Imports: []parser.Import{
				{ImportType: "INTERNAL", TargetFilePath: "src/utils", ImportName: "./utils"},
				{ImportType: "INTERNAL", TargetFilePath: "src/lib", ImportName: "./lib"},
			}},
		}

		// Build allPaths from a map (non-deterministic iteration order).
		m := map[string]bool{
			"src/app.ts":       true,
			"src/utils.ts":     true,
			"src/utils.js":     true, // competing stem for "src/utils"
			"src/lib/index.ts": true,
			"src/lib/index.js": true, // competing dir index for "src/lib"
		}
		var allPaths []string
		for k := range m {
			allPaths = append(allPaths, k)
		}

		ResolveImportTargets(files, allPaths, "")

		gotUtils := files[0].Imports[0].TargetFilePath
		gotLib := files[0].Imports[1].TargetFilePath

		if run == 0 {
			firstResult = gotUtils + "|" + gotLib
		} else if got := gotUtils + "|" + gotLib; got != firstResult {
			t.Fatalf("run %d: non-deterministic resolution: got %q, first was %q", run, got, firstResult)
		}
	}
}

func TestResolveImportTargets_IncrementalWithAllPaths(t *testing.T) {
	// Simulate incremental indexing: only changed files are in the ParsedFiles
	// slice, but unchanged files are provided via allPaths so imports to them
	// can be resolved.
	changedFiles := []parser.ParsedFileResult{
		{FilePath: "src/app.ts", Imports: []parser.Import{
			{ImportType: "INTERNAL", TargetFilePath: "src/utils", ImportName: "./utils"},
			{ImportType: "INTERNAL", TargetFilePath: "src/lib", ImportName: "./lib"},
		}},
	}

	// All project files (changed + unchanged).
	allPaths := []string{
		"src/app.ts",
		"src/utils.ts",     // unchanged
		"src/lib/index.ts", // unchanged
	}

	ResolveImportTargets(changedFiles, allPaths, "")

	imports := changedFiles[0].Imports
	if got := imports[0].TargetFilePath; got != "src/utils.ts" {
		t.Errorf("should resolve against allPaths: got %q, want %q", got, "src/utils.ts")
	}
	if got := imports[1].TargetFilePath; got != "src/lib/index.ts" {
		t.Errorf("should resolve index via allPaths: got %q, want %q", got, "src/lib/index.ts")
	}
}

// ---------------------------------------------------------------------------
// Post-hoc EXTERNAL → INTERNAL reclassification tests
// ---------------------------------------------------------------------------

func TestResolveImportTargets_GoModuleReclassification(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "cmd/server/main.go", Language: "go", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "myapp/internal/handler"},
			{ImportType: "STDLIB", TargetFilePath: "", ImportName: "fmt"},
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "github.com/other/lib"},
		}},
		{FilePath: "internal/handler.go", Language: "go"},
	}

	ResolveImportTargets(files, nil, "myapp")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("Go internal import: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "internal/handler.go" {
		t.Errorf("Go internal import: TargetFilePath = %q, want %q", imp.TargetFilePath, "internal/handler.go")
	}
	if files[0].Imports[1].ImportType != "STDLIB" {
		t.Errorf("stdlib should remain STDLIB")
	}
	if files[0].Imports[2].ImportType != "EXTERNAL" {
		t.Errorf("other-module import should remain EXTERNAL")
	}
}

func TestResolveImportTargets_JavaReclassification(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/main/java/App.java", Language: "java", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "com.example.MyClass"},
			{ImportType: "STDLIB", TargetFilePath: "", ImportName: "java.util.List"},
		}},
		{FilePath: "com/example/MyClass.java", Language: "java"},
	}

	ResolveImportTargets(files, nil, "")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("Java internal import: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "com/example/MyClass.java" {
		t.Errorf("Java internal import: TargetFilePath = %q, want %q", imp.TargetFilePath, "com/example/MyClass.java")
	}
	if files[0].Imports[1].ImportType != "STDLIB" {
		t.Errorf("Java stdlib should remain STDLIB")
	}
}

func TestResolveImportTargets_KotlinReclassification(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/main/kotlin/App.kt", Language: "kotlin", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "com.example.Service"},
		}},
		{FilePath: "com/example/Service.kt", Language: "kotlin"},
	}

	ResolveImportTargets(files, nil, "")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("Kotlin internal import: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "com/example/Service.kt" {
		t.Errorf("Kotlin: TargetFilePath = %q, want %q", imp.TargetFilePath, "com/example/Service.kt")
	}
}

func TestResolveImportTargets_PythonAbsoluteReclassification(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "main.py", Language: "python", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "myapp.models"},
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "myapp.utils"},
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "requests"},
		}},
		{FilePath: "myapp/models.py", Language: "python"},
		{FilePath: "myapp/utils/__init__.py", Language: "python"},
	}

	ResolveImportTargets(files, nil, "")

	if imp := files[0].Imports[0]; imp.ImportType != "INTERNAL" || imp.TargetFilePath != "myapp/models.py" {
		t.Errorf("Python absolute (stem): type=%q path=%q, want INTERNAL %q", imp.ImportType, imp.TargetFilePath, "myapp/models.py")
	}
	if imp := files[0].Imports[1]; imp.ImportType != "INTERNAL" || imp.TargetFilePath != "myapp/utils/__init__.py" {
		t.Errorf("Python absolute (dir): type=%q path=%q, want INTERNAL %q", imp.ImportType, imp.TargetFilePath, "myapp/utils/__init__.py")
	}
	if files[0].Imports[2].ImportType != "EXTERNAL" {
		t.Errorf("unresolvable import should remain EXTERNAL")
	}
}

func TestResolveImportTargets_CSharpReclassification(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/Program.cs", Language: "csharp", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "MyApp.Services.AuthService"},
		}},
		{FilePath: "MyApp/Services/AuthService.cs", Language: "csharp"},
	}

	ResolveImportTargets(files, nil, "")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("C# internal import: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "MyApp/Services/AuthService.cs" {
		t.Errorf("C#: TargetFilePath = %q, want %q", imp.TargetFilePath, "MyApp/Services/AuthService.cs")
	}
}

func TestResolveImportTargets_PHPReclassification(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "src/index.php", Language: "php", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: `App\Models\User`},
		}},
		{FilePath: "App/Models/User.php", Language: "php"},
	}

	ResolveImportTargets(files, nil, "")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("PHP internal import: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "App/Models/User.php" {
		t.Errorf("PHP: TargetFilePath = %q, want %q", imp.TargetFilePath, "App/Models/User.php")
	}
}

func TestResolveImportTargets_GoDirectoryPackage(t *testing.T) {
	// Go imports reference packages (directories), not files.
	// When a package directory contains multiple .go files, the import
	// should resolve to the directory's first .go file (sorted).
	files := []parser.ParsedFileResult{
		{FilePath: "cmd/server/main.go", Language: "go", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "myapp/internal/handler"},
		}},
		{FilePath: "internal/handler/handler.go", Language: "go"},
		{FilePath: "internal/handler/routes.go", Language: "go"},
	}

	ResolveImportTargets(files, nil, "myapp")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("Go directory package: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "internal/handler/handler.go" {
		t.Errorf("Go directory package: TargetFilePath = %q, want %q", imp.TargetFilePath, "internal/handler/handler.go")
	}
}

func TestResolveImportTargets_GoDirectoryPackage_ModGo(t *testing.T) {
	// A file named "mod.go" has baseStem "mod" which matches indexFileNames.
	// The early return in the index-file check must not skip goDir registration.
	files := []parser.ParsedFileResult{
		{FilePath: "cmd/main.go", Language: "go", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "myapp/internal/mods"},
		}},
		{FilePath: "internal/mods/mod.go", Language: "go"},
	}

	ResolveImportTargets(files, nil, "myapp")

	imp := files[0].Imports[0]
	if imp.ImportType != "INTERNAL" {
		t.Errorf("Go mod.go package: type = %q, want INTERNAL", imp.ImportType)
	}
	if imp.TargetFilePath != "internal/mods/mod.go" {
		t.Errorf("Go mod.go package: TargetFilePath = %q, want %q", imp.TargetFilePath, "internal/mods/mod.go")
	}
}

func TestResolveImportTargets_ExternalStaysExternal(t *testing.T) {
	files := []parser.ParsedFileResult{
		{FilePath: "main.py", Language: "python", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "requests"},
		}},
		{FilePath: "cmd/main.go", Language: "go", Imports: []parser.Import{
			{ImportType: "EXTERNAL", TargetFilePath: "", ImportName: "github.com/gin-gonic/gin"},
		}},
	}

	ResolveImportTargets(files, nil, "myapp")

	if files[0].Imports[0].ImportType != "EXTERNAL" {
		t.Errorf("Python: unresolvable import should remain EXTERNAL")
	}
	if files[1].Imports[0].ImportType != "EXTERNAL" {
		t.Errorf("Go: external module should remain EXTERNAL")
	}
}

func TestExtractGoModulePath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"simple", "module myapp\n\ngo 1.21\n", "myapp"},
		{"domain path", "module github.com/user/repo\n\ngo 1.21\n", "github.com/user/repo"},
		{"with extra spaces", "module   myapp  \n", "myapp"},
		{"tab separated", "module\tmyapp\n", "myapp"},
		{"quoted path", `module "example.com/foo"` + "\n", "example.com/foo"},
		{"inline comment", "module myapp // v2\n", "myapp"},
		{"no module line", "go 1.21\n", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractGoModulePath(tt.content)
			if got != tt.want {
				t.Errorf("ExtractGoModulePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertImportToPath(t *testing.T) {
	tests := []struct {
		name         string
		importName   string
		language     string
		goModulePath string
		want         string
	}{
		{"go internal", "myapp/internal/handler", "go", "myapp", "internal/handler"},
		{"go external", "github.com/other/lib", "go", "myapp", ""},
		{"go no module", "myapp/pkg", "go", "", ""},
		{"go prefix collision", "myapp-fork/pkg", "go", "myapp", ""},
		{"java", "com.example.MyClass", "java", "", "com/example/MyClass"},
		{"kotlin", "com.example.Service", "kotlin", "", "com/example/Service"},
		{"csharp", "MyApp.Services.Auth", "csharp", "", "MyApp/Services/Auth"},
		{"php", `App\Models\User`, "php", "", "App/Models/User"},
		{"php leading backslash", `\App\Models\User`, "php", "", "App/Models/User"},
		{"python absolute", "myapp.utils", "python", "", "myapp/utils"},
		{"python relative skip", ".utils", "python", "", ""},
		{"unknown lang", "foo.bar", "ruby", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertImportToPath(tt.importName, tt.language, tt.goModulePath)
			if got != tt.want {
				t.Errorf("convertImportToPath(%q, %q, %q) = %q, want %q",
					tt.importName, tt.language, tt.goModulePath, got, tt.want)
			}
		})
	}
}

func TestStripExt(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"src/foo.ts", "src/foo"},
		{"src/foo.d.ts", "src/foo.d"},
		{"src/foo", "src/foo"},
		{"__init__.py", "__init__"},
		{"mod.rs", "mod"},
		{"handler.go", "handler"},
		{"Makefile", "Makefile"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripExt(tt.input); got != tt.want {
				t.Errorf("stripExt(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
