package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"myjungle/backend-api/internal/domain"
)

func TestTestEmbeddingConnectivity_NilConfig(t *testing.T) {
	_, err := TestEmbeddingConnectivity(context.Background(), nil, nil)
	if !errors.Is(err, ErrNilEmbeddingConfig) {
		t.Fatalf("error = %v, want errors.Is(_, ErrNilEmbeddingConfig)", err)
	}
}

func TestTestLLMConnectivity_NilConfig(t *testing.T) {
	_, err := TestLLMConnectivity(context.Background(), nil, nil)
	if !errors.Is(err, ErrNilLLMConfig) {
		t.Fatalf("error = %v, want errors.Is(_, ErrNilLLMConfig)", err)
	}
}

func TestConnectivityValidationAndExecution(t *testing.T) {
	t.Setenv(providerConnectivityAllowedHostsEnv, "127.0.0.1")
	client := &http.Client{Timeout: 200 * time.Millisecond}

	tests := []struct {
		name            string
		call            func() (ConnectivityResult, error)
		wantErrContains string
		wantOK          bool
		wantMsgContains string
	}{
		{
			name: "embedding invalid provider",
			call: func() (ConnectivityResult, error) {
				return TestEmbeddingConnectivity(context.Background(), client, &domain.EmbeddingProviderConfig{
					Provider:    "invalid",
					EndpointURL: "jina/jina-embeddings-v2-base-en:11434",
					Model:       "jina/jina-embeddings-v2-base-en",
					Dimensions:  768,
				})
			},
			wantErrContains: `unsupported embedding provider "invalid"`,
		},
		{
			name: "llm invalid provider",
			call: func() (ConnectivityResult, error) {
				return TestLLMConnectivity(context.Background(), client, &domain.LLMProviderConfig{
					Provider:    "invalid",
					EndpointURL: "http://localhost:11434",
					Model:       "llama3",
				})
			},
			wantErrContains: `unsupported llm provider "invalid"`,
		},
		{
			name: "embedding unreachable endpoint",
			call: func() (ConnectivityResult, error) {
				return TestEmbeddingConnectivity(context.Background(), client, &domain.EmbeddingProviderConfig{
					Provider:    "ollama",
					EndpointURL: "http://127.0.0.1:1",
					Model:       "jina/jina-embeddings-v2-base-en",
					Dimensions:  768,
				})
			},
			wantOK:          false,
			wantMsgContains: "cannot reach Ollama",
		},
		{
			name: "llm unreachable endpoint",
			call: func() (ConnectivityResult, error) {
				return TestLLMConnectivity(context.Background(), client, &domain.LLMProviderConfig{
					Provider:    "ollama",
					EndpointURL: "http://127.0.0.1:1",
					Model:       "llama3",
				})
			},
			wantOK:          false,
			wantMsgContains: "cannot reach Ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.call()
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.OK != tt.wantOK {
				t.Fatalf("result.OK = %v, want %v", result.OK, tt.wantOK)
			}
			if !strings.Contains(result.Message, tt.wantMsgContains) {
				t.Fatalf("result.Message = %q, want substring %q", result.Message, tt.wantMsgContains)
			}
		})
	}
}

func TestLLMConnectivity_NoModel(t *testing.T) {
	t.Setenv(providerConnectivityAllowedHostsEnv, "127.0.0.1")
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "llama3.1:latest"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockOllama.Close()

	client := &http.Client{Timeout: 2 * time.Second}

	result, err := TestLLMConnectivity(context.Background(), client, &domain.LLMProviderConfig{
		Provider:    "ollama",
		EndpointURL: mockOllama.URL,
		Model:       "   ",
	})
	if err != nil {
		t.Fatalf("TestLLMConnectivity returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true (message=%q)", result.Message)
	}
	if result.Message != "Connected to Ollama." {
		t.Fatalf("result.Message = %q, want %q", result.Message, "Connected to Ollama.")
	}
}

func TestOllamaConnectivity_PreservesBasePath(t *testing.T) {
	t.Setenv(providerConnectivityAllowedHostsEnv, "127.0.0.1")
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/base/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "llama3.1:latest"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockOllama.Close()

	client := &http.Client{Timeout: 2 * time.Second}

	result := TestOllamaConnectivity(context.Background(), client, mockOllama.URL+"/base", "llama3.1", true)
	if !result.OK {
		t.Fatalf("result.OK = false, want true (message=%q)", result.Message)
	}
	if !strings.Contains(result.Message, "Model 'llama3.1' is available.") {
		t.Fatalf("result.Message = %q, want model success", result.Message)
	}
}

func TestOllamaConnectivity_DeniesEndpointOutsideAllowlist(t *testing.T) {
	t.Setenv(providerConnectivityAllowedHostsEnv, "host.docker.internal:11434")

	result := TestOllamaConnectivity(context.Background(), &http.Client{Timeout: time.Second}, "http://127.0.0.1:11434", "llama3.1", true)
	if result.OK {
		t.Fatal("result.OK = true, want false")
	}
	if !strings.Contains(result.Message, "configured provider test hosts") {
		t.Fatalf("result.Message = %q, want allowlist denial", result.Message)
	}
}

func TestConnectivityAllowed_DefaultLoopback(t *testing.T) {
	// Unset the env var to test default loopback behavior.
	t.Setenv(providerConnectivityAllowedHostsEnv, "")
	os.Unsetenv(providerConnectivityAllowedHostsEnv)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"localhost no port", "http://localhost/api", false},
		{"localhost with port", "http://localhost:11434/api", false},
		{"127.0.0.1 with port", "http://127.0.0.1:11434/api", false},
		{"ipv6 loopback", "http://[::1]:11434/api", false},
		{"host.docker.internal", "http://host.docker.internal:11434/api", false},
		{"external host denied", "http://evil.example.com:11434/api", true},
		{"public IP denied", "http://203.0.113.1:11434/api", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := connectivityAllowed(tt.url)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestConnectivityAllowed_ExplicitEnvOverridesDefaults(t *testing.T) {
	t.Setenv(providerConnectivityAllowedHostsEnv, "custom-host:8080")

	if err := connectivityAllowed("http://localhost:11434/api"); err == nil {
		t.Fatal("localhost should be denied when env var is set to different host")
	}
	if err := connectivityAllowed("http://custom-host:8080/api"); err != nil {
		t.Fatalf("custom-host:8080 should be allowed: %v", err)
	}
}

func TestConnectivityAllowed_EmptyEnvDeniesAll(t *testing.T) {
	t.Setenv(providerConnectivityAllowedHostsEnv, "")

	if err := connectivityAllowed("http://localhost:11434/api"); err == nil {
		t.Fatal("expected error when env var is explicitly empty")
	}
}
