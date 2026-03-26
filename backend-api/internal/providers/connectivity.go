package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"myjungle/backend-api/internal/domain"
)

var (
	ErrNilEmbeddingConfig = errors.New("nil embedding provider config")
	ErrNilLLMConfig       = errors.New("nil llm provider config")
)

// TestEmbeddingConnectivity dispatches embedding connectivity checks by provider.
func TestEmbeddingConnectivity(ctx context.Context, client *http.Client, cfg *domain.EmbeddingProviderConfig) (ConnectivityResult, error) {
	if cfg == nil {
		return ConnectivityResult{}, ErrNilEmbeddingConfig
	}
	provider := NormalizeProviderID(cfg.Provider)
	if err := ValidateProvider(CapabilityEmbedding, provider); err != nil {
		return ConnectivityResult{}, err
	}
	return testProviderConnectivity(ctx, client, CapabilityEmbedding, provider, cfg.EndpointURL, cfg.Model, true)
}

// TestLLMConnectivity dispatches LLM connectivity checks by provider.
func TestLLMConnectivity(ctx context.Context, client *http.Client, cfg *domain.LLMProviderConfig) (ConnectivityResult, error) {
	if cfg == nil {
		return ConnectivityResult{}, ErrNilLLMConfig
	}
	provider := NormalizeProviderID(cfg.Provider)
	if err := ValidateProvider(CapabilityLLM, provider); err != nil {
		return ConnectivityResult{}, err
	}
	return testProviderConnectivity(ctx, client, CapabilityLLM, provider, cfg.EndpointURL, cfg.Model, strings.TrimSpace(cfg.Model) != "")
}

func testProviderConnectivity(ctx context.Context, client *http.Client, capability Capability, provider, endpointURL, model string, requireModel bool) (ConnectivityResult, error) {
	info, ok := SupportedProviderInfo(capability, provider)
	if !ok || info.Tester == nil {
		// Defensive guard: ValidateProvider should make this unreachable unless the
		// registry is extended without wiring connectivity dispatch.
		slog.Warn("providers: missing connectivity dispatcher",
			slog.String("capability", string(capability)),
			slog.String("provider", provider),
			slog.String("endpoint", redactURL(endpointURL)),
			slog.String("model", model))
		return ConnectivityResult{}, fmt.Errorf("unsupported %s provider %q", capability, provider)
	}
	return info.Tester(ctx, client, endpointURL, model, requireModel), nil
}
