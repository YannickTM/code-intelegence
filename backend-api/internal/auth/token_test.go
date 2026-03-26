package auth

import (
	"net/http/httptest"
	"testing"
)

func TestGenerateSessionToken(t *testing.T) {
	raw, hash, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw) != 64 {
		t.Errorf("raw token length = %d, want 64", len(raw))
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
	if raw == hash {
		t.Error("raw and hash should differ")
	}
}

func TestGenerateSessionToken_Unique(t *testing.T) {
	raw1, _, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("unexpected error generating token 1: %v", err)
	}
	raw2, _, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("unexpected error generating token 2: %v", err)
	}
	if raw1 == raw2 {
		t.Error("two generated tokens should not be equal")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("abc123")
	h2 := HashToken("abc123")
	if h1 != h2 {
		t.Errorf("HashToken not deterministic: %q != %q", h1, h2)
	}
}

func TestHashToken_MatchesGenerate(t *testing.T) {
	raw, hash, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if HashToken(raw) != hash {
		t.Error("HashToken(raw) does not match hash from GenerateSessionToken")
	}
}

func TestHasBearerScheme(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"valid with token", "Bearer abc123", true},
		{"bearer only", "Bearer", true},
		{"case insensitive", "bearer abc123", true},
		{"mixed case", "BEARER abc123", true},
		{"empty header", "", false},
		{"basic scheme", "Basic abc123", false},
		{"bearer-like prefix", "BearerX abc123", false},
		{"bearers", "Bearers abc123", false},
		{"tab delimited", "Bearer\tabc123", false},
		{"scheme trailing space", "Bearer ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.value != "" {
				r.Header.Set("Authorization", tt.value)
			}
			if got := HasBearerScheme(r); got != tt.want {
				t.Errorf("HasBearerScheme() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"valid", "Bearer abc123", "abc123"},
		{"case insensitive", "bearer abc123", "abc123"},
		{"mixed case", "BEARER abc123", "abc123"},
		{"with extra spaces", "Bearer  abc123 ", "abc123"},
		{"empty", "", ""},
		{"no bearer prefix", "Basic abc123", ""},
		{"bearer only", "Bearer", ""},
		{"bearer space only", "Bearer ", ""},
		{"tab separator", "Bearer\tabc123", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.value != "" {
				r.Header.Set("Authorization", tt.value)
			}
			got := ExtractBearerToken(r)
			if got != tt.want {
				t.Errorf("ExtractBearerToken() = %q, want %q", got, tt.want)
			}
		})
	}
}
