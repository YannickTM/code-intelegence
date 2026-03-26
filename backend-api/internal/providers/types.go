package providers

import (
	"context"
	"net/http"
)

// Capability identifies a provider capability exposed by the backend.
type Capability string

const (
	CapabilityEmbedding Capability = "embedding"
	CapabilityLLM       Capability = "llm"
)

// ProviderInfo describes an implemented provider.
type ProviderInfo struct {
	ID           string               `json:"id"`
	Label        string               `json:"label"`
	Capabilities []Capability         `json:"capabilities"`
	Tester       ConnectivityTesterFn `json:"-"`
}

// ConnectivityTesterFn probes a provider endpoint for a specific capability.
type ConnectivityTesterFn func(ctx context.Context, client *http.Client, endpoint string, capability string, secure bool) ConnectivityResult

// ConnectivityResult holds the outcome of a provider connectivity test.
type ConnectivityResult struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Message  string `json:"message"`
}
