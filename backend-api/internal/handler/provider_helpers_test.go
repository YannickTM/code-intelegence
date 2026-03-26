package handler

import (
	"encoding/json"
	"testing"

	"myjungle/backend-api/internal/validate"
)

func TestValidateProviderEndpointURL(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      string
		wantValue string
	}{
		{name: "empty", value: "", want: "", wantValue: ""},
		{name: "whitespace-only", value: "   ", want: "endpoint_url is required", wantValue: "   "},
		{name: "valid", value: "http://localhost:11434", want: "", wantValue: "http://localhost:11434"},
		{name: "trimmed", value: "  http://localhost:11434/  ", want: "", wantValue: "http://localhost:11434/"},
		{name: "scheme-less", value: "localhost:11434", want: "endpoint_url must include a host", wantValue: "localhost:11434"},
		{name: "hostless", value: "http://:11434", want: "endpoint_url must include a host", wantValue: "http://:11434"},
		{name: "ipv6", value: "http://[::1]:11434", want: "", wantValue: "http://[::1]:11434"},
		{name: "no-port", value: "http://localhost", want: "", wantValue: "http://localhost"},
		{name: "port zero", value: "http://localhost:0", want: "endpoint_url must include a valid port (1-65535)", wantValue: "http://localhost:0"},
		{name: "port too large", value: "http://localhost:99999", want: "endpoint_url must include a valid port (1-65535)", wantValue: "http://localhost:99999"},
		{name: "query", value: "http://localhost:11434?token=secret", want: "endpoint_url must not contain query or fragment", wantValue: "http://localhost:11434?token=secret"},
		{name: "fragment", value: "http://localhost:11434#secret", want: "endpoint_url must not contain query or fragment", wantValue: "http://localhost:11434#secret"},
		{name: "userinfo", value: "http://user:pass@localhost:11434", want: "endpoint_url must not contain user credentials", wantValue: "http://user:pass@localhost:11434"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validate.Errors{}
			gotValue := validateProviderEndpointURL(tt.value, "endpoint_url", errs)
			got := errs["endpoint_url"]
			if got != tt.want {
				t.Fatalf("validateProviderEndpointURL(%q) error = %q, want %q", tt.value, got, tt.want)
			}
			if gotValue != tt.wantValue {
				t.Fatalf("validateProviderEndpointURL(%q) value = %q, want %q", tt.value, gotValue, tt.wantValue)
			}
		})
	}
}

func TestParseCredentialsUpdate(t *testing.T) {
	tests := []struct {
		name      string
		raw       json.RawMessage
		wantSet   bool
		wantClear bool
		wantData  map[string]any
		wantErr   bool
	}{
		{name: "omitted", raw: nil, wantSet: false, wantClear: false, wantData: nil},
		{name: "null", raw: json.RawMessage("null"), wantSet: true, wantClear: true, wantData: nil},
		{name: "empty object", raw: json.RawMessage("{}"), wantSet: true, wantClear: true, wantData: nil},
		{name: "non empty", raw: json.RawMessage(`{"api_key":"secret"}`), wantSet: true, wantClear: false, wantData: map[string]any{"api_key": "secret"}},
		{name: "array", raw: json.RawMessage("[]"), wantErr: true},
		{name: "invalid json", raw: json.RawMessage("}{"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCredentialsUpdate(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseCredentialsUpdate returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCredentialsUpdate returned error: %v", err)
			}
			if got.set != tt.wantSet || got.clear != tt.wantClear {
				t.Fatalf("parseCredentialsUpdate flags = {set:%v clear:%v}, want {set:%v clear:%v}", got.set, got.clear, tt.wantSet, tt.wantClear)
			}
			if len(got.data) != len(tt.wantData) {
				t.Fatalf("parseCredentialsUpdate data len = %d, want %d", len(got.data), len(tt.wantData))
			}
			for k, want := range tt.wantData {
				if got.data[k] != want {
					t.Fatalf("parseCredentialsUpdate data[%q] = %#v, want %#v", k, got.data[k], want)
				}
			}
		})
	}
}
