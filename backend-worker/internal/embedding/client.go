package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"myjungle/backend-worker/internal/logger"
)

// DefaultEmbedBatchSize is the number of texts sent per Ollama HTTP request.
// With pre-truncated inputs (max 8K chars each), a batch of 50 texts is
// ~400 KB — well within HTTP payload limits while reducing round-trips 5x.
const DefaultEmbedBatchSize = 50

// DefaultEmbedRequestTimeout is the per-HTTP-request timeout for Ollama calls.
// A batch of 50 texts at ~7.5K chars each typically completes in 1-3 seconds,
// but under load (GPU contention, cold model load) can take longer.
// 5 minutes is generous enough for worst-case batches without letting a
// hung Ollama block the entire job until the asynq task deadline.
const DefaultEmbedRequestTimeout = 5 * time.Minute

// DefaultMaxInputChars is the hard character limit per embedding input.
// jina/jina-embeddings-v2-base-en has an 8192 token context window. Because BERT-style
// tokenizers can map single characters to individual tokens (operators,
// brackets, short identifiers), the worst-case ratio approaches 1 char
// per token. We set the character limit equal to the token budget (with
// a safety margin) so truncation is guaranteed to prevent context-length
// errors regardless of input content.
const DefaultMaxInputChars = 7500

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// OllamaClient calls the Ollama /api/embed endpoint.
type OllamaClient struct {
	httpClient    *http.Client
	endpointURL   string
	model         string
	dimensions    int32
	batchSize     int
	maxInputChars int
}

// NewOllamaClient creates an embedding client for the given Ollama endpoint.
// maxTokens controls the character-level truncation limit per input text;
// if <= 0 it falls back to DefaultMaxInputChars.
func NewOllamaClient(endpointURL, model string, dimensions int32, maxTokens int) *OllamaClient {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxInputChars
	}
	return &OllamaClient{
		httpClient:    &http.Client{Timeout: DefaultEmbedRequestTimeout},
		endpointURL:   endpointURL,
		model:         model,
		dimensions:    dimensions,
		batchSize:     DefaultEmbedBatchSize,
		maxInputChars: maxTokens,
	}
}

// embedRequest is the Ollama /api/embed request body.
type embedRequest struct {
	Model    string   `json:"model"`
	Input    []string `json:"input"`
	Truncate bool     `json:"truncate"`
}

// embedResponse is the Ollama /api/embed response body.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed generates embeddings for the given texts, batching them in groups
// of batchSize. Returns one vector per input text in the same order.
// Texts exceeding the model's context window are truncated with a warning.
func (c *OllamaClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	log := logger.FromContext(ctx)

	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Truncate any texts that exceed the model's context window.
	for i, t := range texts {
		if len(t) > c.maxInputChars {
			log.Warn("embedding: truncating oversized input",
				slog.Int("index", i),
				slog.Int("original_chars", len(t)),
				slog.Int("max_chars", c.maxInputChars))
			texts[i] = t[:c.maxInputChars]
		}
	}

	totalBatches := (len(texts) + c.batchSize - 1) / c.batchSize
	result := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.batchSize {
		end := start + c.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		batchNum := start/c.batchSize + 1

		log.Debug("embedding: batch start",
			slog.Int("batch", batchNum),
			slog.Int("total", totalBatches),
			slog.Int("texts", len(batch)))

		vectors, err := c.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		result = append(result, vectors...)

		if batchNum%10 == 0 || batchNum == totalBatches {
			log.Info("embedding: progress",
				slog.Int("batch", batchNum),
				slog.Int("total", totalBatches),
				slog.Int("embedded", len(result)),
				slog.Int("remaining", len(texts)-len(result)))
		}
	}
	return result, nil
}

func (c *OllamaClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{
		Model:    c.model,
		Input:    texts,
		Truncate: true,
	})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	url := c.endpointURL + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("embedding: %s returned %d: %s", url, resp.StatusCode, respBody)
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding: expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}

	for i, vec := range result.Embeddings {
		if int32(len(vec)) != c.dimensions {
			return nil, fmt.Errorf("embedding: dimension mismatch at index %d: got %d, want %d", i, len(vec), c.dimensions)
		}
	}

	return result.Embeddings, nil
}
