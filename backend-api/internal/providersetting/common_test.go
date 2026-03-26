package providersetting

import (
	"encoding/json"
	"strings"
	"testing"

	"myjungle/backend-api/internal/providers"
	"myjungle/backend-api/internal/secrets"
)

func TestResolveCredentials_SetFalse_CarriesForward(t *testing.T) {
	key := secrets.DeriveKey("test-secret")
	current := []byte("existing-ciphertext")

	got, err := ResolveCredentials(CredentialsUpdate{}, current, true, key)
	if err != nil {
		t.Fatalf("ResolveCredentials returned error: %v", err)
	}
	if string(got) != string(current) {
		t.Fatalf("ResolveCredentials returned %v, want %v", got, current)
	}
}

func TestResolveCredentials_SetTrueClearTrue_Clears(t *testing.T) {
	key := secrets.DeriveKey("test-secret")

	got, err := ResolveCredentials(CredentialsUpdate{
		Set:   true,
		Clear: true,
	}, []byte("existing-ciphertext"), true, key)
	if err != nil {
		t.Fatalf("ResolveCredentials returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("ResolveCredentials returned ciphertext for clear: %v", got)
	}
}

func TestResolveCredentials_SetTrueWithData_Encrypts(t *testing.T) {
	key := secrets.DeriveKey("test-secret")
	want := map[string]any{"api_key": "secret"}

	got, err := ResolveCredentials(CredentialsUpdate{
		Set:  true,
		Data: want,
	}, nil, false, key)
	if err != nil {
		t.Fatalf("ResolveCredentials returned error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("ResolveCredentials returned empty ciphertext")
	}
	plaintext, err := secrets.Decrypt(got, key)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(plaintext, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(decoded) != len(want) || decoded["api_key"] != want["api_key"] {
		t.Fatalf("decrypted credentials = %#v, want %#v", decoded, want)
	}
}

func TestResolveCredentials_EmptyObjectStoresNoCiphertext(t *testing.T) {
	key := secrets.DeriveKey("test-secret")

	got, err := ResolveCredentials(CredentialsUpdate{
		Set:  true,
		Data: map[string]any{},
	}, nil, false, key)
	if err != nil {
		t.Fatalf("ResolveCredentials returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("ResolveCredentials returned ciphertext for empty object: %v", got)
	}
}

func TestValidateBaseFields_NilPointers(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "nil name",
			call: func() error {
				providerID := "ollama"
				endpointURL := "http://localhost:11434"
				settings := map[string]any{}
				return ValidateBaseFields(nil, &providerID, &endpointURL, &settings, providers.CapabilityEmbedding)
			},
			want: "name is required",
		},
		{
			name: "nil provider",
			call: func() error {
				name := "Config"
				endpointURL := "http://localhost:11434"
				settings := map[string]any{}
				return ValidateBaseFields(&name, nil, &endpointURL, &settings, providers.CapabilityEmbedding)
			},
			want: "provider is required",
		},
		{
			name: "nil endpoint",
			call: func() error {
				name := "Config"
				providerID := "ollama"
				settings := map[string]any{}
				return ValidateBaseFields(&name, &providerID, nil, &settings, providers.CapabilityEmbedding)
			},
			want: "endpoint_url is required",
		},
		{
			name: "nil settings pointer",
			call: func() error {
				name := "Config"
				providerID := "ollama"
				endpointURL := "http://localhost:11434"
				return ValidateBaseFields(&name, &providerID, &endpointURL, nil, providers.CapabilityEmbedding)
			},
			want: "settings must be provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("ValidateBaseFields returned nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("ValidateBaseFields error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidateBaseFields_InvalidValues(t *testing.T) {
	tests := []struct {
		name         string
		providerID   string
		endpointURL  string
		configName   string
		want         string
		wantContains string
	}{
		{
			name:        "whitespace name",
			configName:  "   ",
			providerID:  "ollama",
			endpointURL: "http://localhost:11434",
			want:        "name is required",
		},
		{
			name:         "invalid provider",
			configName:   "Config",
			providerID:   "invalid-provider",
			endpointURL:  "http://localhost:11434",
			wantContains: `unsupported embedding provider "invalid-provider"`,
		},
		{
			name:        "invalid url",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://",
			want:        "endpoint_url must include a host",
		},
		{
			name:        "hostless url",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://:11434",
			want:        "endpoint_url must include a host",
		},
		{
			name:        "unsupported scheme",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "ftp://localhost:11434",
			want:        "endpoint_url must use http or https scheme",
		},
		{
			name:        "embedded credentials",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://user:pass@localhost:11434",
			want:        "endpoint_url must not contain user credentials",
		},
		{
			name:        "malformed url",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://%zz",
			want:        "endpoint_url must be a valid URL",
		},
		{
			name:        "query string",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://localhost:11434?token=secret",
			want:        "endpoint_url must not contain query or fragment",
		},
		{
			name:        "invalid port zero",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://localhost:0",
			want:        "endpoint_url must include a valid port (1-65535)",
		},
		{
			name:        "invalid port too large",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://localhost:99999",
			want:        "endpoint_url must include a valid port (1-65535)",
		},
		{
			name:        "fragment",
			configName:  "Config",
			providerID:  "ollama",
			endpointURL: "http://localhost:11434#secret",
			want:        "endpoint_url must not contain query or fragment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := map[string]any{}
			err := ValidateBaseFields(&tt.configName, &tt.providerID, &tt.endpointURL, &settings, providers.CapabilityEmbedding)
			if err == nil {
				t.Fatal("ValidateBaseFields returned nil, want error")
			}
			if tt.wantContains != "" {
				if !strings.Contains(err.Error(), tt.wantContains) {
					t.Fatalf("ValidateBaseFields error = %q, want substring %q", err.Error(), tt.wantContains)
				}
				return
			}
			if err.Error() != tt.want {
				t.Fatalf("ValidateBaseFields error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidateBaseFields_InvalidProvider_LLM(t *testing.T) {
	name := "Config"
	providerID := "invalid-provider"
	endpointURL := "http://localhost:11434"
	settings := map[string]any{}

	err := ValidateBaseFields(&name, &providerID, &endpointURL, &settings, providers.CapabilityLLM)
	if err == nil {
		t.Fatal("ValidateBaseFields returned nil, want error")
	}
	if !strings.Contains(err.Error(), `unsupported llm provider "invalid-provider"`) {
		t.Fatalf("ValidateBaseFields error = %q, want llm provider substring", err.Error())
	}
}
