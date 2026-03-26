package handler

import (
	"net/http"

	"myjungle/backend-api/internal/providers"
)

// ProviderHandler serves the supported-provider catalog endpoint.
type ProviderHandler struct{}

// NewProviderHandler creates a new ProviderHandler.
func NewProviderHandler() *ProviderHandler {
	return &ProviderHandler{}
}

// HandleListProviders returns the provider IDs implemented by the backend.
func (h *ProviderHandler) HandleListProviders(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, providers.SupportedProviderCatalog())
}
