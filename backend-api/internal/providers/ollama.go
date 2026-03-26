package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxOllamaResponseBytes = 1 << 20
	connectivityTimeout    = 5 * time.Second
)

type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

type ollamaModel struct {
	Name string `json:"name"`
}

// TestOllamaConnectivity checks endpoint reachability and optionally verifies a model exists.
func TestOllamaConnectivity(ctx context.Context, client *http.Client, endpointURL, model string, requireModel bool) ConnectivityResult {
	model = strings.TrimSpace(model)
	if requireModel && model == "" {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  "model required but not provided",
		}
	}

	ctx, cancel := context.WithTimeout(ctx, connectivityTimeout)
	defer cancel()

	endpointURL, err := ValidateEndpointURL(endpointURL)
	if err != nil {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  err.Error(),
		}
	}
	if err := connectivityAllowed(endpointURL); err != nil {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  err.Error(),
		}
	}
	baseURL, err := url.Parse(endpointURL)
	if err != nil {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  "invalid endpoint URL",
		}
	}
	tagsURL := baseURL.JoinPath("api", "tags")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL.String(), nil)
	if err != nil {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  "invalid endpoint URL",
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  fmt.Sprintf("cannot reach Ollama at %s: %s", redactURL(endpointURL), sanitizeTransportError(err)),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  fmt.Sprintf("Ollama returned HTTP %d from %s/api/tags", resp.StatusCode, redactURL(endpointURL)),
		}
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOllamaResponseBytes)).Decode(&tags); err != nil {
		return ConnectivityResult{
			OK:       false,
			Provider: "ollama",
			Message:  fmt.Sprintf("failed to decode Ollama response: %v", err),
		}
	}

	if !requireModel {
		return ConnectivityResult{
			OK:       true,
			Provider: "ollama",
			Message:  "Connected to Ollama.",
		}
	}

	normalise := func(s string) string { return strings.TrimSuffix(s, ":latest") }
	available := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		name := m.Name
		available = append(available, name)
		if name == model || normalise(name) == normalise(model) {
			return ConnectivityResult{
				OK:       true,
				Provider: "ollama",
				Message:  fmt.Sprintf("Connected to Ollama. Model '%s' is available.", model),
			}
		}
	}

	return ConnectivityResult{
		OK:       false,
		Provider: "ollama",
		Message:  fmt.Sprintf("Connected to Ollama but model '%s' is not available. Available models: %s", model, strings.Join(available, ", ")),
	}
}

func sanitizeTransportError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}
	return err.Error()
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid URL>"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
