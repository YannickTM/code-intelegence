package providers

import (
	"fmt"
	"sort"
	"strings"
)

var supportedProviders = []ProviderInfo{
	{
		ID:           "ollama",
		Label:        "Ollama",
		Capabilities: []Capability{CapabilityEmbedding, CapabilityLLM},
		Tester:       TestOllamaConnectivity,
	},
}

var supportedProviderCatalog map[string][]string

func init() {
	supportedProviderCatalog = map[string][]string{
		string(CapabilityEmbedding): SupportedProviderIDs(CapabilityEmbedding),
		string(CapabilityLLM):       SupportedProviderIDs(CapabilityLLM),
	}
}

// NormalizeProviderID trims whitespace and lowercases a provider identifier.
func NormalizeProviderID(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// SupportedProviderIDs returns implemented provider IDs for a capability.
func SupportedProviderIDs(capability Capability) []string {
	ids := make([]string, 0, len(supportedProviders))
	for _, provider := range supportedProviders {
		for _, cap := range provider.Capabilities {
			if cap == capability {
				ids = append(ids, provider.ID)
				break
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// SupportedProviderCatalog returns the provider IDs exposed by the backend.
func SupportedProviderCatalog() map[string][]string {
	catalog := make(map[string][]string, len(supportedProviderCatalog))
	for capability, ids := range supportedProviderCatalog {
		catalog[capability] = append([]string(nil), ids...)
	}
	return catalog
}

// ValidateProvider returns an error if the provider is not implemented for the capability.
func ValidateProvider(capability Capability, provider string) error {
	provider = NormalizeProviderID(provider)
	ids := SupportedProviderIDs(capability)
	for _, supported := range ids {
		if provider == supported {
			return nil
		}
	}
	return fmt.Errorf("unsupported %s provider %q; supported: %s", capability, provider, strings.Join(ids, ", "))
}

func SupportedProviderInfo(capability Capability, provider string) (ProviderInfo, bool) {
	normalizedProvider := NormalizeProviderID(provider)
	for _, info := range supportedProviders {
		if NormalizeProviderID(info.ID) != normalizedProvider {
			continue
		}
		for _, supportedCapability := range info.Capabilities {
			if supportedCapability == capability {
				return info, true
			}
		}
	}
	return ProviderInfo{}, false
}
