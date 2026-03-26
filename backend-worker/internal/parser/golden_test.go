//go:build integration

package parser_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/engine"
)

// ---------------------------------------------------------------------------
// Shared helpers (used by all integration test files)
// ---------------------------------------------------------------------------

func newTestEngine(t testing.TB) *engine.Engine {
	t.Helper()
	eng, err := engine.New(engine.Config{
		PoolSize:       4,
		TimeoutPerFile: 5 * time.Second,
		MaxFileSize:    1 * 1024 * 1024, // 1 MB
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return eng
}

// testdataDir returns the absolute path to backend-worker/testdata/parser/.
func testdataDir(t testing.TB) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../backend-worker/internal/parser/golden_test.go
	// Navigate up to backend-worker/ then into testdata/parser/
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "parser")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("testdata dir not found: %s", abs)
	}
	return abs
}

type fixture struct {
	RelPath  string // e.g. "typescript/react-component.tsx"
	Language string // directory name, e.g. "typescript"
	Name     string // stem without extension, e.g. "react-component"
	AbsPath  string
}

func discoverFixtures(t testing.TB, fixturesDir string) []fixture {
	t.Helper()
	var fixtures []fixture
	err := filepath.WalkDir(fixturesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == ".gitkeep" {
			return nil
		}
		rel, err := filepath.Rel(fixturesDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		parts := strings.SplitN(relSlash, "/", 2)
		lang := ""
		if len(parts) == 2 {
			lang = parts[0] // e.g. "typescript" from "typescript/react-component.tsx"
		}
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		fixtures = append(fixtures, fixture{
			RelPath:  relSlash,
			Language: lang,
			Name:     name,
			AbsPath:  path,
		})
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	slices.SortFunc(fixtures, func(a, b fixture) int {
		return strings.Compare(a.RelPath, b.RelPath)
	})
	return fixtures
}

func loadFixture(t testing.TB, fx fixture) parser.FileInput {
	t.Helper()
	data, err := os.ReadFile(fx.AbsPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", fx.AbsPath, err)
	}
	return parser.FileInput{
		FilePath: fx.RelPath,
		Content:  string(data),
	}
}

func marshalDeterministic(t testing.TB, result parser.ParsedFileResult) []byte {
	t.Helper()
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}
	data = append(data, '\n')
	return data
}

// goldenPathFor maps a fixture to its golden JSON path.
// For "fixtures" layout:   "typescript/react-component.tsx" → golden/typescript/react-component.json
// For "edge-cases" layout: "empty.ts" → golden/edge-cases/empty.json
func goldenPathFor(td string, fx fixture) string {
	base := filepath.Base(fx.RelPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	// For non-source extensions (json, yml, tf, dat, rb), incorporate the
	// extension into the golden name to avoid ambiguity.
	sourceExts := map[string]bool{
		".tsx": true, ".ts": true, ".py": true, ".go": true,
		".rs": true, ".java": true, ".jsx": true, ".sh": true,
	}
	goldenName := stem
	if !sourceExts[ext] {
		goldenName = stem + "-" + strings.TrimPrefix(ext, ".")
	}

	// Use the fixture's directory structure for the golden path.
	// "typescript/react-component.tsx" → golden/typescript/react-component.json
	// "empty.ts" (flat, Language="") → golden/edge-cases/empty.json
	subdir := fx.Language
	if subdir == "" {
		subdir = "edge-cases"
	}
	return filepath.Join(td, "golden", subdir, goldenName+".json")
}

func readGolden(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadFile golden %s: %v", path, err)
	}
	return data
}

func lineDiff(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	var buf strings.Builder
	maxLen := len(wantLines)
	if len(gotLines) > maxLen {
		maxLen = len(gotLines)
	}
	diffs := 0
	for i := 0; i < maxLen; i++ {
		var wl, gl string
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if wl != gl {
			diffs++
			if diffs > 50 {
				fmt.Fprintf(&buf, "... (%d more differences)\n", maxLen-i)
				break
			}
			fmt.Fprintf(&buf, "line %d:\n  - %s\n  + %s\n", i+1, wl, gl)
		}
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// Golden file tests
// ---------------------------------------------------------------------------

func TestGolden_AllFixtures(t *testing.T) {
	runGoldenSuite(t, "fixtures")
}

func TestGolden_EdgeCases(t *testing.T) {
	runGoldenSuite(t, "edge-cases")
}

// runGoldenSuite discovers all files in testdata/parser/<subdir>, parses each
// through the engine, and compares against golden/<subdir>/<name>.json.
func runGoldenSuite(t *testing.T, subdir string) {
	t.Helper()
	eng := newTestEngine(t)
	defer eng.Close()

	td := testdataDir(t)
	fixtures := discoverFixtures(t, filepath.Join(td, subdir))
	if len(fixtures) == 0 {
		t.Fatalf("no fixtures discovered in %s", subdir)
	}

	for _, fx := range fixtures {
		t.Run(fx.RelPath, func(t *testing.T) {
			input := loadFixture(t, fx)
			results, err := eng.ParseFilesBatched(
				context.Background(), "test-proj", "main", "abc123",
				[]parser.FileInput{input},
			)
			if err != nil {
				t.Fatalf("ParseFilesBatched: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}

			got := marshalDeterministic(t, results[0])
			gp := goldenPathFor(td, fx)

			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(gp), 0755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(gp, got, 0644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				t.Logf("updated golden: %s", gp)
				return
			}

			want := readGolden(t, gp)
			if want == nil {
				t.Fatalf("golden file missing: %s (run with UPDATE_GOLDEN=1)", gp)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("golden mismatch for %s:\n%s", fx.RelPath, lineDiff(want, got))
			}
		})
	}
}
