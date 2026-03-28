# 15 — LLM Client Package

## Status
Pending

## Goal
Create a provider-agnostic LLM completion client (`internal/llmclient/`) with a `Completer` interface and an Ollama `/api/chat` implementation, providing the foundational LLM integration for the description pipeline. Add description-specific configuration to the worker config.

## Depends On
—

## Scope

### Interface Definition (`internal/llmclient/client.go`)

Define the completion interface and shared types. This follows the same pattern as `embedding.Embedder` in `internal/embedding/client.go`: a small interface with one provider implementation.

```go
package llmclient

import "context"

// Completer generates text completions from a sequence of messages.
type Completer interface {
    Complete(ctx context.Context, messages []Message) (string, CompletionMeta, error)
}

// Message represents a single message in the conversation.
type Message struct {
    Role    string // "system", "user", "assistant"
    Content string
}

// CompletionMeta captures per-call telemetry for cost tracking and debugging.
type CompletionMeta struct {
    InputTokens  int
    OutputTokens int
    LatencyMs    int64
    Model        string
}
```

### Ollama Implementation (`internal/llmclient/ollama.go`)

HTTP client for the Ollama `/api/chat` endpoint. Follows the pattern established in `internal/embedding/client.go` (constructor, HTTP request building, response decoding, error handling).

```go
type OllamaCompleter struct {
    httpClient  *http.Client
    endpointURL string
    model       string
}

func NewOllamaCompleter(endpointURL, model string, perCallTimeout time.Duration) *OllamaCompleter
```

Ollama `/api/chat` request body:

```json
{
  "model": "llama3.1",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "stream": false,
  "format": "json"
}
```

The response includes `message.content`, `eval_count` (output tokens), `prompt_eval_count` (input tokens), and `total_duration` (nanoseconds). Map these to `CompletionMeta`.

Error classification — inspect HTTP status codes and map to descriptive errors:
- 404: model not found / not loaded
- 429: rate limit (handle for proxy setups)
- 5xx: server error
- Connection refused / timeout: endpoint unreachable

### Configuration Extension (`internal/config/config.go`)

Add a `Describe` section to `Config`:

```go
type DescribeConfig struct {
    Concurrency     int
    RateLimitMs     int
    PerFileTimeout  time.Duration
    MaxContentChars int
    MaxSymbols      int
    JobTimeout      time.Duration
}
```

| Variable | Default | Description |
|---|---|---|
| `DESCRIBE_CONCURRENCY` | `4` | Concurrent LLM calls per description job |
| `DESCRIBE_RATE_LIMIT_MS` | `0` | Minimum delay between LLM calls (ms) |
| `DESCRIBE_PER_FILE_TIMEOUT` | `60s` | Timeout for a single LLM call |
| `DESCRIBE_MAX_CONTENT_CHARS` | `12000` | Max file content characters in prompt |
| `DESCRIBE_MAX_SYMBOLS` | `30` | Max symbols included in prompt context |
| `DESCRIBE_JOB_TIMEOUT` | `60m` | Overall job timeout (asynq) |

## Key Files

| File/Package | Purpose |
|---|---|
| `backend-worker/internal/llmclient/client.go` | `Completer` interface, `Message`, `CompletionMeta` types |
| `backend-worker/internal/llmclient/ollama.go` | Ollama `/api/chat` HTTP implementation |
| `backend-worker/internal/llmclient/ollama_test.go` | Unit tests with httptest server |
| `backend-worker/internal/config/config.go` | Add `Describe DescribeConfig` field, env loading |
| `backend-worker/internal/embedding/client.go` | Reference pattern for HTTP client structure |

## Acceptance Criteria
- [ ] `Completer` interface defined with `Complete(ctx, []Message) (string, CompletionMeta, error)`
- [ ] `OllamaCompleter` sends well-formed `/api/chat` requests with `stream: false` and `format: "json"`
- [ ] `CompletionMeta` populated from Ollama response fields (`prompt_eval_count`, `eval_count`, `total_duration`)
- [ ] HTTP errors mapped to descriptive Go errors (404, 429, 5xx, connection refused)
- [ ] Context cancellation and per-call timeout respected
- [ ] `DescribeConfig` loaded from environment variables with documented defaults
- [ ] Unit tests cover: successful completion, malformed response, HTTP error codes, context cancellation, timeout
