package parser

import (
	"strings"
	"testing"
)

func TestBatchFileInputs_EmptyInput(t *testing.T) {
	batches := BatchFileInputs(nil)
	if batches != nil {
		t.Fatalf("expected nil, got %d batches", len(batches))
	}

	batches = BatchFileInputs([]FileInput{})
	if batches != nil {
		t.Fatalf("expected nil, got %d batches", len(batches))
	}
}

func TestBatchFileInputs_SingleFile(t *testing.T) {
	files := []FileInput{{FilePath: "a.ts", Content: "x", Language: "typescript"}}
	batches := BatchFileInputs(files)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if len(batches[0]) != 1 {
		t.Fatalf("expected 1 file in batch, got %d", len(batches[0]))
	}
}

func TestBatchFileInputs_SplitsByFileCount(t *testing.T) {
	files := make([]FileInput, 120)
	for i := range files {
		files[i] = FileInput{FilePath: "f.ts", Content: "x", Language: "typescript"}
	}

	batches := BatchFileInputs(files)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 50 {
		t.Errorf("batch 0: expected 50 files, got %d", len(batches[0]))
	}
	if len(batches[1]) != 50 {
		t.Errorf("batch 1: expected 50 files, got %d", len(batches[1]))
	}
	if len(batches[2]) != 20 {
		t.Errorf("batch 2: expected 20 files, got %d", len(batches[2]))
	}
}

func TestBatchFileInputs_SplitsByContentSize(t *testing.T) {
	// Each file is 1 MB, so 5 fit per batch.
	oneMB := strings.Repeat("a", 1024*1024)
	files := make([]FileInput, 12)
	for i := range files {
		files[i] = FileInput{FilePath: "f.ts", Content: oneMB, Language: "typescript"}
	}

	batches := BatchFileInputs(files)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 5 {
		t.Errorf("batch 0: expected 5 files, got %d", len(batches[0]))
	}
	if len(batches[1]) != 5 {
		t.Errorf("batch 1: expected 5 files, got %d", len(batches[1]))
	}
	if len(batches[2]) != 2 {
		t.Errorf("batch 2: expected 2 files, got %d", len(batches[2]))
	}
}

func TestBatchFileInputs_OversizedFileGetsOwnBatch(t *testing.T) {
	big := strings.Repeat("a", 6*1024*1024) // 6 MB, exceeds limit
	files := []FileInput{
		{FilePath: "small.ts", Content: "x", Language: "typescript"},
		{FilePath: "big.ts", Content: big, Language: "typescript"},
		{FilePath: "small2.ts", Content: "y", Language: "typescript"},
	}

	batches := BatchFileInputs(files)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if batches[0][0].FilePath != "small.ts" {
		t.Errorf("batch 0: expected small.ts, got %s", batches[0][0].FilePath)
	}
	if batches[1][0].FilePath != "big.ts" {
		t.Errorf("batch 1: expected big.ts, got %s", batches[1][0].FilePath)
	}
	if batches[2][0].FilePath != "small2.ts" {
		t.Errorf("batch 2: expected small2.ts, got %s", batches[2][0].FilePath)
	}
}

func TestBatchFileInputs_ExactlyAtLimit(t *testing.T) {
	files := make([]FileInput, MaxFilesPerBatch)
	for i := range files {
		files[i] = FileInput{FilePath: "f.ts", Content: "x", Language: "typescript"}
	}

	batches := BatchFileInputs(files)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch at exact file limit, got %d", len(batches))
	}
}

func TestBatchFileInputs_PreservesOrder(t *testing.T) {
	files := make([]FileInput, 75)
	for i := range files {
		files[i] = FileInput{FilePath: string(rune('a' + i)), Content: "x", Language: "typescript"}
	}

	batches := BatchFileInputs(files)
	idx := 0
	for _, batch := range batches {
		for _, f := range batch {
			if f.FilePath != files[idx].FilePath {
				t.Fatalf("order broken at index %d: expected %s, got %s", idx, files[idx].FilePath, f.FilePath)
			}
			idx++
		}
	}
}
