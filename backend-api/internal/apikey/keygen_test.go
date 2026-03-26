package apikey

import (
	"strings"
	"testing"

	"myjungle/backend-api/internal/domain"
)

func TestGenerateAPIKey_Project(t *testing.T) {
	plaintext, prefix, hash, err := GenerateAPIKey(domain.KeyTypeProject)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check length first so subsequent slicing is safe.
	if len(plaintext) != 40 {
		t.Fatalf("plaintext len = %d, want 40 (value=%q)", len(plaintext), plaintext)
	}
	if !strings.HasPrefix(plaintext, "mj_proj_") {
		t.Errorf("plaintext = %q, want mj_proj_ prefix", plaintext)
	}
	expectedPrefix := plaintext[:12]
	if prefix != expectedPrefix {
		t.Errorf("prefix = %q, want %q", prefix, expectedPrefix)
	}
	if len(hash) != 64 { // SHA-256 hex = 64 chars
		t.Errorf("hash len = %d, want 64", len(hash))
	}
}

func TestGenerateAPIKey_Personal(t *testing.T) {
	plaintext, prefix, hash, err := GenerateAPIKey(domain.KeyTypePersonal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plaintext) != 40 {
		t.Fatalf("plaintext len = %d, want 40 (value=%q)", len(plaintext), plaintext)
	}
	if !strings.HasPrefix(plaintext, "mj_pers_") {
		t.Errorf("plaintext = %q, want mj_pers_ prefix", plaintext)
	}
	expectedPrefix := plaintext[:12]
	if prefix != expectedPrefix {
		t.Errorf("prefix = %q, want %q", prefix, expectedPrefix)
	}
	if len(hash) != 64 {
		t.Errorf("hash len = %d, want 64", len(hash))
	}
}

func TestGenerateAPIKey_InvalidType(t *testing.T) {
	_, _, _, err := GenerateAPIKey("bogus")
	if err == nil {
		t.Fatal("expected error for invalid key type")
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		plaintext, _, _, err := GenerateAPIKey(domain.KeyTypeProject)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if seen[plaintext] {
			t.Fatalf("duplicate key at iteration %d: %s", i, plaintext)
		}
		seen[plaintext] = true
	}
}

func TestHashKey_Deterministic(t *testing.T) {
	key := "mj_proj_abcdefghijklmnopqrstuvwxyz123456"
	h1 := HashKey(key)
	h2 := HashKey(key)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}
}

func TestHashKey_DifferentInputs(t *testing.T) {
	h1 := HashKey("mj_proj_aaaa")
	h2 := HashKey("mj_proj_bbbb")
	if h1 == h2 {
		t.Error("different inputs produced same hash")
	}
}

func TestGenerateAPIKey_HashRoundTrip(t *testing.T) {
	plaintext, _, hash, err := GenerateAPIKey(domain.KeyTypeProject)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := HashKey(plaintext); got != hash {
		t.Errorf("HashKey(plaintext) = %q, want %q (from GenerateAPIKey)", got, hash)
	}
}
