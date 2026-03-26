//go:build integration

package parser_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"myjungle/backend-worker/internal/parser"
	"myjungle/backend-worker/internal/parser/engine"
)

func TestDeterminism_FullPipeline(t *testing.T) {
	// Use PoolSize=1 to eliminate non-determinism from concurrent parser
	// instances inside the pool (matches engine_test.go's determinism test).
	eng, err := engine.New(engine.Config{
		PoolSize:       1,
		TimeoutPerFile: 10 * time.Second,
		MaxFileSize:    1 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	td := testdataDir(t)
	fixtures := discoverFixtures(t, filepath.Join(td, "fixtures"))
	if len(fixtures) == 0 {
		t.Fatal("no fixtures discovered — determinism test requires at least 1 fixture")
	}

	// Use first 5 fixtures for a multi-language batch.
	limit := 5
	if len(fixtures) < limit {
		limit = len(fixtures)
	}
	inputs := make([]parser.FileInput, limit)
	for i := 0; i < limit; i++ {
		inputs[i] = loadFixture(t, fixtures[i])
	}

	const N = 10
	outputs := make([][]byte, N)

	for i := 0; i < N; i++ {
		results, err := eng.ParseFilesBatched(
			context.Background(), "test-proj", "main", "abc123", inputs,
		)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatalf("run %d marshal: %v", i, err)
		}
		outputs[i] = data
	}

	for i := 1; i < N; i++ {
		if !bytes.Equal(outputs[0], outputs[i]) {
			t.Errorf("run 0 and run %d produced different JSON output (len %d vs %d)",
				i, len(outputs[0]), len(outputs[i]))
		}
	}
}

func TestDeterminism_ConcurrentBatches(t *testing.T) {
	td := testdataDir(t)
	fixtures := discoverFixtures(t, filepath.Join(td, "fixtures"))
	if len(fixtures) == 0 {
		t.Fatal("no fixtures discovered — determinism test requires at least 1 fixture")
	}

	limit := 5
	if len(fixtures) < limit {
		limit = len(fixtures)
	}
	inputs := make([]parser.FileInput, limit)
	for i := 0; i < limit; i++ {
		inputs[i] = loadFixture(t, fixtures[i])
	}

	// Each goroutine gets its own engine with PoolSize=1 to avoid
	// non-determinism from concurrent parser instances within a pool.
	const G = 5
	outputs := make([][]byte, G)
	errs := make([]error, G)

	var wg sync.WaitGroup
	for g := 0; g < G; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			eng, err := engine.New(engine.Config{
				PoolSize:       1,
				TimeoutPerFile: 10 * time.Second,
				MaxFileSize:    1 * 1024 * 1024,
			})
			if err != nil {
				errs[idx] = err
				return
			}
			defer eng.Close()
			results, err := eng.ParseFilesBatched(
				context.Background(), "test-proj", "main", "abc123", inputs,
			)
			if err != nil {
				errs[idx] = err
				return
			}
			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				errs[idx] = fmt.Errorf("marshal: %w", err)
				return
			}
			outputs[idx] = data
		}(g)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	for i := 1; i < G; i++ {
		if !bytes.Equal(outputs[0], outputs[i]) {
			t.Errorf("goroutine 0 and goroutine %d produced different JSON (len %d vs %d)",
				i, len(outputs[0]), len(outputs[i]))
		}
	}
}
