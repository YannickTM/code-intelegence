package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOllamaClient_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}

		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		resp := embedResponse{
			Embeddings: make([][]float32, len(req.Input)),
		}
		for i := range req.Input {
			resp.Embeddings[i] = make([]float32, 3)
			resp.Embeddings[i][0] = float32(i)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 3, DefaultMaxInputChars)
	vecs, err := client.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vecs))
	}
	if vecs[0][0] != 0.0 || vecs[1][0] != 1.0 {
		t.Errorf("unexpected vector values: %v, %v", vecs[0], vecs[1])
	}
}

func TestOllamaClient_BatchesAtBatchSize(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		var req embedRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := embedResponse{Embeddings: make([][]float32, len(req.Input))}
		for i := range req.Input {
			resp.Embeddings[i] = make([]float32, 4)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 4, DefaultMaxInputChars)

	// 120 texts should produce 3 batches: 50 + 50 + 20
	texts := make([]string, 120)
	for i := range texts {
		texts[i] = "text"
	}

	vecs, err := client.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 120 {
		t.Fatalf("got %d vectors, want 120", len(vecs))
	}
	if requestCount.Load() != 3 {
		t.Errorf("HTTP requests = %d, want 3", requestCount.Load())
	}
}

func TestOllamaClient_DimensionMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return vectors with wrong dimensions (2 instead of 4).
		resp := embedResponse{Embeddings: [][]float32{{0.1, 0.2}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 4, DefaultMaxInputChars)
	_, err := client.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "dimension mismatch") {
		t.Errorf("error = %q, want to contain %q", err, "dimension mismatch")
	}
}

func TestOllamaClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model not found"))
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "bad-model", 4, DefaultMaxInputChars)
	_, err := client.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain %q", err, "500")
	}
}

func TestOllamaClient_TruncatesLongInput(t *testing.T) {
	var receivedInput []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedInput = req.Input

		resp := embedResponse{Embeddings: make([][]float32, len(req.Input))}
		for i := range req.Input {
			resp.Embeddings[i] = make([]float32, 3)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 3, DefaultMaxInputChars)
	// Override maxInputChars to a small value for testing.
	client.maxInputChars = 20

	longText := strings.Repeat("a", 100) // 100 chars, well over limit of 20
	shortText := "hello"

	_, err := client.Embed(context.Background(), []string{longText, shortText})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(receivedInput) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(receivedInput))
	}
	if len(receivedInput[0]) != 20 {
		t.Errorf("long text should be truncated to 20 chars, got %d", len(receivedInput[0]))
	}
	if receivedInput[1] != shortText {
		t.Errorf("short text should be unchanged, got %q", receivedInput[1])
	}
}

func TestOllamaClient_EmptyInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP request should not be made for empty input")
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 4, DefaultMaxInputChars)
	vecs, err := client.Embed(context.Background(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 0 {
		t.Errorf("got %d vectors, want 0", len(vecs))
	}
}
