//go:build integration

package parser_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"myjungle/backend-worker/internal/parser"
)

func BenchmarkEngine_SingleFile_TypeScript(b *testing.B) {
	eng := newTestEngine(b)
	defer eng.Close()

	td := testdataDir(b)
	content, err := os.ReadFile(filepath.Join(td, "fixtures", "typescript", "react-component.tsx"))
	if err != nil {
		b.Fatal(err)
	}
	input := []parser.FileInput{{FilePath: "react-component.tsx", Content: string(content)}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.ParseFilesBatched(context.Background(), "proj", "main", "abc", input)
	}
}

func BenchmarkEngine_BatchAll_MixedLanguages(b *testing.B) {
	eng := newTestEngine(b)
	defer eng.Close()

	td := testdataDir(b)
	fixtures := discoverFixtures(b, filepath.Join(td, "fixtures"))
	inputs := make([]parser.FileInput, len(fixtures))
	for i, fx := range fixtures {
		inputs[i] = loadFixture(b, fx)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.ParseFilesBatched(context.Background(), "proj", "main", "abc", inputs)
	}
}

func BenchmarkEngine_Batch50_TypeScript(b *testing.B) {
	eng := newTestEngine(b)
	defer eng.Close()

	td := testdataDir(b)
	content, err := os.ReadFile(filepath.Join(td, "fixtures", "typescript", "react-component.tsx"))
	if err != nil {
		b.Fatal(err)
	}

	inputs := make([]parser.FileInput, 50)
	for i := range inputs {
		inputs[i] = parser.FileInput{
			FilePath: fmt.Sprintf("copy_%d.tsx", i),
			Content:  string(content),
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.ParseFilesBatched(context.Background(), "proj", "main", "abc", inputs)
	}
}
